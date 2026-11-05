// Package server is the HTTP server of pack-ui (C1): the SPA from its directory (any path that is not the
// API's falls to index.html — the SPA routes), and the backend API the SPA talks to: who the user is, the
// projects, a file read at a branch, a file written — committed as the user, pushed, the operator told to
// sync. A write that met the other side's change of the same file is 409, a conflict Failure with the files
// and the other side's commit. The errors are Failure documents, the shape the operator's API writes (К4).
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"

	"github.com/horchestra/pack-ui/internal/config"
	"github.com/horchestra/pack-ui/internal/edit"
	"github.com/horchestra/pack-ui/internal/operator"
)

// Server is pack-ui's backend.
type Server struct {
	cfg   *config.Config
	op    *operator.Client
	repos *edit.Repos
	mux   *http.ServeMux
}

// New is the server on the configuration: op the operator's client, repos the working copies.
func New(cfg *config.Config, op *operator.Client, repos *edit.Repos) *Server {
	s := &Server{cfg: cfg, op: op, repos: repos, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /api/whoami", s.whoami)
	s.mux.HandleFunc("GET /api/projects", s.projects)
	s.mux.HandleFunc("GET /api/repo/{id}/file", s.readFile)
	s.mux.HandleFunc("PUT /api/repo/{id}/file", s.writeFile)
	s.mux.HandleFunc("GET /api/repo/{id}/log", s.log)
	s.mux.HandleFunc("POST /api/repo/{id}/values/check", s.valuesCheck)
	s.mux.HandleFunc("POST /api/repo/{id}/values/preview", s.valuesPreview)
	s.mux.HandleFunc("POST /api/repo/{id}/values/write", s.valuesWrite)
	s.mux.Handle("/api/op/", s.proxy())
	s.mux.HandleFunc("/", s.spa)
	return s
}

// Handler is the server as an http.Handler.
func (s *Server) Handler() http.Handler { return s.mux }

// fail is an error as a Failure document, the API's shape (К4): code one of bad_request, not_found,
// conflict, error.
func fail(w http.ResponseWriter, status int, code, format string, args ...any) {
	doc := map[string]any{"error": map[string]any{"code": code, "message": fmt.Sprintf(format, args...)}}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(doc)
}

// answer is a 200 JSON document.
func answer(w http.ResponseWriter, doc any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(doc)
}

// whoami answers the user the requests are for (auth: none — the configured one, X-Pack-User onwards).
func (s *Server) whoami(w http.ResponseWriter, r *http.Request) {
	answer(w, map[string]string{"user": s.cfg.User, "auth": s.cfg.Auth})
}

// projects answers the projects pack-ui edits (a mirror of the operator's).
func (s *Server) projects(w http.ResponseWriter, r *http.Request) {
	type project struct {
		ID     string `json:"id"`
		Repo   string `json:"repo"`
		Branch string `json:"branch"`
		Flow   string `json:"flow"`
	}
	out := []project{}
	for _, p := range s.cfg.Projects {
		out = append(out, project{ID: p.ID, Repo: p.Repo, Branch: p.Branch, Flow: p.Flow})
	}
	answer(w, out)
}

// project is the configured project of the request's {id}, or a 404.
func (s *Server) project(w http.ResponseWriter, r *http.Request) *config.Project {
	p := s.cfg.Project(r.PathValue("id"))
	if p == nil {
		fail(w, http.StatusNotFound, "not_found", "no project %s", r.PathValue("id"))
	}
	return p
}

// repo is the project's working copy, or a 500.
func (s *Server) repo(w http.ResponseWriter, r *http.Request, p *config.Project) *edit.Repo {
	repo, err := s.repos.Get(r.Context(), p.ID, p.Repo, p.Credentials)
	if err != nil {
		log.Error().Err(err).Str("project", p.ID).Msg("the working copy")
		fail(w, http.StatusInternalServerError, "error", "the working copy: %v", err)
		return nil
	}
	return repo
}

// readFile answers GET /api/repo/{id}/file?path=&ref= — the file at the branch's head (ref: the project's
// main branch), and the head's sha.
func (s *Server) readFile(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	repo := s.repo(w, r, p)
	if repo == nil {
		return
	}
	path := r.URL.Query().Get("path")
	if len(path) == 0 {
		fail(w, http.StatusBadRequest, "bad_request", "path is required")
		return
	}
	branch := r.URL.Query().Get("ref")
	if len(branch) == 0 {
		branch = p.Branch
	}
	content, sha, err := repo.ReadFile(r.Context(), branch, path)
	if err != nil {
		fail(w, http.StatusNotFound, "not_found", "%v", err)
		return
	}
	answer(w, map[string]string{"path": path, "ref": branch, "sha": sha, "content": string(content)})
}

// log answers GET /api/repo/{id}/log?path=&ref=&n= — the commits that touched the path (a release's
// directory), newest first: the release card's "commits of the directory".
func (s *Server) log(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	repo := s.repo(w, r, p)
	if repo == nil {
		return
	}
	branch := r.URL.Query().Get("ref")
	if len(branch) == 0 {
		branch = p.Branch
	}
	commits, err := repo.LogPath(r.Context(), branch, r.URL.Query().Get("path"), 50)
	if err != nil {
		fail(w, http.StatusInternalServerError, "error", "%v", err)
		return
	}
	type commit struct {
		SHA     string `json:"sha"`
		Subject string `json:"subject"`
	}
	out := []commit{}
	for _, c := range commits {
		out = append(out, commit{SHA: c[0], Subject: c[1]})
	}
	answer(w, out)
}

// writeFile is the body of PUT /api/repo/{id}/file.
type writeFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Message string `json:"message"`
	SHA     string `json:"sha,omitempty"`    // the head the user based the edit on (a read answered it)
	Branch  string `json:"branch,omitempty"` // a change request's branch; empty — the project's (its flow says where)
}

// writeFile answers PUT: the file committed as the user (the committer pack-ui), pushed to the branch the
// flow names, the operator asked to sync. A conflict is 409 with the files and the other side's commit.
func (s *Server) writeFile(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	var body writeFile
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "the body: %v", err)
		return
	}
	if len(body.Path) == 0 || len(body.Message) == 0 {
		fail(w, http.StatusBadRequest, "bad_request", "path and message are required")
		return
	}
	branch := p.Branch
	if p.Flow == "review" {
		if len(body.Branch) == 0 {
			fail(w, http.StatusBadRequest, "bad_request", "the project %s is flow: review — name the change request's branch", p.ID)
			return
		}
		branch = body.Branch
	}
	repo := s.repo(w, r, p)
	if repo == nil {
		return
	}
	author := s.cfg.User + " <" + s.cfg.User + "@localhost>"
	sha, err := repo.Write(r.Context(), branch, body.SHA, map[string][]byte{body.Path: []byte(body.Content)}, author, body.Message)
	if err != nil {
		var conflict *edit.Conflict
		if errors.As(err, &conflict) {
			doc := map[string]any{"error": map[string]any{
				"code":    "conflict",
				"message": conflict.Error(),
				"detail":  conflict,
			}}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(doc)
			return
		}
		fail(w, http.StatusInternalServerError, "error", "%v", err)
		return
	}
	if err := s.op.Sync(r.Context(), p.Repo, branch); err != nil {
		log.Error().Err(err).Str("project", p.ID).Msg("the operator's sync after a push")
		// the commit landed; the sync is retried by the operator's interval — say so, do not fail the write
		answer(w, map[string]any{"sha": sha, "branch": branch, "synced": false})
		return
	}
	answer(w, map[string]any{"sha": sha, "branch": branch, "synced": true})
}

// spa serves the SPA's directory; a path that is no file falls to index.html — the SPA routes. With no
// static directory configured, the API is all there is.
func (s *Server) spa(w http.ResponseWriter, r *http.Request) {
	if len(s.cfg.Static) == 0 {
		fail(w, http.StatusNotFound, "not_found", "the SPA is not built into this run (static: is empty)")
		return
	}
	path := filepath.Join(s.cfg.Static, filepath.Clean("/"+r.URL.Path))
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	http.ServeFile(w, r, filepath.Join(s.cfg.Static, "index.html"))
}
