// Package server is the HTTP server of pack-ui (C1): the SPA from its directory (any path that is not the
// API's falls to index.html — the SPA routes), and the backend API the SPA talks to: who the user is, the
// projects, a file read at a branch, a file written — committed as the user, pushed, the operator told to
// sync. A write that met the other side's change of the same file is 409, a conflict Failure with the files
// and the other side's commit. The errors are Failure documents, the shape the operator's API writes (К4).
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	opauthn "github.com/horchestra/pack/operator/authn"

	"github.com/horchestra/pack-ui/internal/authn"
	"github.com/horchestra/pack-ui/internal/config"
	"github.com/horchestra/pack-ui/internal/edit"
	"github.com/horchestra/pack-ui/internal/operator"
	"github.com/horchestra/pack-ui/web"
)

// Server is pack-ui's backend.
type Server struct {
	cfg      *config.Config
	op       *operator.Client
	repos    *edit.Repos
	sessions *authn.Sessions
	oidc     *opauthn.OIDC
	static   fs.FS
	mux      *http.ServeMux
}

// New is the server on the configuration: op the operator's client, repos the working copies. The SPA is
// the directory static names, the built one embedded into the binary without it — pack-ui is one binary.
func New(cfg *config.Config, op *operator.Client, repos *edit.Repos) *Server {
	s := &Server{cfg: cfg, op: op, repos: repos, sessions: authn.NewSessions(), mux: http.NewServeMux()}
	if cfg.Auth == "oidc" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if o, err := authn.NewOIDC(ctx, cfg.OIDC); err != nil {
			log.Error().Err(err).Msg("oidc: the issuer is not discovered — the login will not work")
		} else {
			s.oidc = o
		}
	}
	if len(cfg.Static) > 0 {
		s.static = os.DirFS(cfg.Static)
	} else if dist, err := fs.Sub(web.Dist, "dist"); err == nil {
		s.static = dist
	}
	s.mux.HandleFunc("GET /api/whoami", s.whoami)
	s.mux.HandleFunc("POST /api/login", s.login)
	s.mux.HandleFunc("POST /api/logout", s.logout)
	s.mux.HandleFunc("GET /api/auth/oidc/login", s.oidcLogin)
	s.mux.HandleFunc("GET /api/auth/oidc/callback", s.oidcCallback)
	s.mux.HandleFunc("GET /api/projects", s.projects)
	s.mux.HandleFunc("GET /api/repo/{id}/file", s.readFile)
	s.mux.HandleFunc("PUT /api/repo/{id}/file", s.writeFile)
	s.mux.HandleFunc("GET /api/repo/{id}/log", s.log)
	s.mux.HandleFunc("GET /api/repo/{id}/diff", s.diff)
	s.mux.HandleFunc("POST /api/repo/{id}/init", s.initRelease)
	s.mux.HandleFunc("GET /api/author/scaffold/schema", s.scaffoldSchema)
	s.mux.HandleFunc("POST /api/repo/{id}/author/scaffold", s.authorScaffold)
	s.mux.HandleFunc("POST /api/repo/{id}/author/check", s.authorCheck)
	s.mux.HandleFunc("POST /api/repo/{id}/author/docs", s.authorDocs)
	s.mux.HandleFunc("POST /api/repo/{id}/author/cel", s.authorCEL)
	s.mux.HandleFunc("GET /api/author/ref", s.authorRef)
	s.mux.HandleFunc("GET /api/author/example", s.authorExample)
	s.mux.HandleFunc("POST /api/repo/{id}/values/check", s.valuesCheck)
	s.mux.HandleFunc("POST /api/repo/{id}/values/preview", s.valuesPreview)
	s.mux.HandleFunc("POST /api/repo/{id}/values/write", s.valuesWrite)
	s.mux.HandleFunc("POST /api/repo/{id}/diagnose", s.diagnose)
	s.mux.Handle("/api/op/", s.proxy())
	s.mux.HandleFunc("/", s.spa)
	return s
}

// Handler is the server as an http.Handler, the API behind the session's middleware (C9).
func (s *Server) Handler() http.Handler { return s.authn(s.mux) }

// userKey is the context's key of the request's user (the session's; auth none — the configured one).
type userKey struct{}

func withUser(ctx context.Context, user string) context.Context {
	return context.WithValue(ctx, userKey{}, user)
}

// user is who the request is for: the commits' author, the X-Pack-User to the operator.
func (s *Server) user(r *http.Request) string {
	if u, ok := r.Context().Value(userKey{}).(string); ok {
		return u
	}
	return s.cfg.User
}

// opFor is the operator's client as the request's user.
func (s *Server) opFor(r *http.Request) *operator.Client { return s.op.As(s.user(r)) }

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

// whoami answers the user the requests are for and the session's CSRF token (auth: none — the configured
// one, X-Pack-User onwards; not logged in — the empty user, the SPA shows its login).
func (s *Server) whoami(w http.ResponseWriter, r *http.Request) {
	_, csrf := s.sessionUser(r)
	answer(w, map[string]string{"user": s.user(r), "auth": s.cfg.Auth, "csrf": csrf})
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

// diff answers GET /api/repo/{id}/diff?base=&head= — the change of the head branch against where it left
// the base one (a change request's page).
func (s *Server) diff(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	repo := s.repo(w, r, p)
	if repo == nil {
		return
	}
	head := r.URL.Query().Get("head")
	if len(head) == 0 {
		fail(w, http.StatusBadRequest, "bad_request", "head is required")
		return
	}
	base := r.URL.Query().Get("base")
	if len(base) == 0 {
		base = p.Branch
	}
	files, err := repo.Diff(r.Context(), base, head)
	if err != nil {
		fail(w, http.StatusInternalServerError, "error", "%v", err)
		return
	}
	answer(w, files)
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
	author := s.user(r) + " <" + s.user(r) + "@localhost>"
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
	if err := s.opFor(r).Sync(r.Context(), p.ID, branch); err != nil {
		log.Error().Err(err).Str("project", p.ID).Msg("the operator's sync after a push")
		// the commit landed; the sync is retried by the operator's interval — say so, do not fail the write
		answer(w, map[string]any{"sha": sha, "branch": branch, "synced": false})
		return
	}
	answer(w, map[string]any{"sha": sha, "branch": branch, "synced": true})
}

// spa serves the SPA; a path that is no file falls to index.html — the SPA routes. With no SPA at all
// (the embedded dist is the placeholder only, no static configured), the API is all there is.
func (s *Server) spa(w http.ResponseWriter, r *http.Request) {
	if s.static == nil {
		fail(w, http.StatusNotFound, "not_found", "no SPA in this run")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if len(path) > 0 {
		if st, err := fs.Stat(s.static, path); err == nil && !st.IsDir() {
			http.ServeFileFS(w, r, s.static, path)
			return
		}
	}
	if _, err := fs.Stat(s.static, "index.html"); err != nil {
		fail(w, http.StatusNotFound, "not_found", "the SPA is not built into this binary (npm run build in web/)")
		return
	}
	http.ServeFileFS(w, r, s.static, "index.html")
}
