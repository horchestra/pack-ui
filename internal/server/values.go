package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"

	"github.com/horchestra/pack-ui/internal/config"
	"github.com/horchestra/pack-ui/internal/edit"
	"github.com/horchestra/pack-ui/internal/values"
)

// valuesBody is the body of the values endpoints: the release's path in the repository and the values as
// the form holds them.
type valuesBody struct {
	Path    string         `json:"path"`
	Values  map[string]any `json:"values"`
	Lang    string         `json:"lang,omitempty"`
	Message string         `json:"message,omitempty"`
	Rewrite bool           `json:"rewrite,omitempty"` // lose the file's foreign comments
	SHA     string         `json:"sha,omitempty"`
	Branch  string         `json:"branch,omitempty"`
}

// valuesRequest decodes the body and checks out the release it names, or says the HTTP error. The caller
// removes dir.
func (s *Server) valuesRequest(w http.ResponseWriter, r *http.Request) (*values.Release, *config.Project, *valuesBody, string) {
	p := s.project(w, r)
	if p == nil {
		return nil, nil, nil, ""
	}
	var body valuesBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "the body: %v", err)
		return nil, nil, nil, ""
	}
	if len(body.Path) == 0 {
		fail(w, http.StatusBadRequest, "bad_request", "path is required")
		return nil, nil, nil, ""
	}
	branch := p.Branch
	if len(body.Branch) > 0 {
		branch = body.Branch
	}
	repo := s.repo(w, r, p)
	if repo == nil {
		return nil, nil, nil, ""
	}
	dir, err := os.MkdirTemp("", "pack-ui-values-*")
	if err != nil {
		fail(w, http.StatusInternalServerError, "error", "%v", err)
		return nil, nil, nil, ""
	}
	if _, err := repo.Checkout(r.Context(), branch, dir); err != nil {
		os.RemoveAll(dir)
		fail(w, http.StatusInternalServerError, "error", "the checkout: %v", err)
		return nil, nil, nil, ""
	}
	rel, _, err := values.Load(dir, body.Path)
	if err != nil {
		os.RemoveAll(dir)
		if errors.Is(err, os.ErrNotExist) {
			fail(w, http.StatusNotFound, "not_found", "%v", err)
		} else {
			fail(w, http.StatusBadRequest, "bad_request", "%v", err)
		}
		return nil, nil, nil, ""
	}
	return rel, p, &body, dir
}

// valuesCheck answers POST /api/repo/{id}/values/check — the form's live help (the fill pack asks): the
// settings required and missing, the suggestions the pack makes for the values as the form holds them, the
// problems — every one where the schema points.
func (s *Server) valuesCheck(w http.ResponseWriter, r *http.Request) {
	rel, _, body, dir := s.valuesRequest(w, r)
	if rel == nil {
		return
	}
	defer os.RemoveAll(dir)
	questions, err := rel.Ask(body.Values)
	if err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "%v", err)
		return
	}
	answer(w, questions)
}

// valuesPreview answers POST /api/repo/{id}/values/preview — the values.yaml the values would be written
// as (exactly pack values write's render), and the comments of the file it would lose.
func (s *Server) valuesPreview(w http.ResponseWriter, r *http.Request) {
	rel, _, body, dir := s.valuesRequest(w, r)
	if rel == nil {
		return
	}
	defer os.RemoveAll(dir)
	content, foreign, err := rel.Render(body.Values, body.Lang)
	if err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "%v", err)
		return
	}
	answer(w, map[string]any{"content": string(content), "foreign": foreign})
}

// valuesWrite answers POST /api/repo/{id}/values/write: the render committed as the user and pushed, the
// operator asked to sync. Foreign comments are a 409 with them listed unless rewrite is set; a git conflict
// is 409 as a write's is.
func (s *Server) valuesWrite(w http.ResponseWriter, r *http.Request) {
	rel, p, body, dir := s.valuesRequest(w, r)
	if rel == nil {
		return
	}
	defer os.RemoveAll(dir)
	if len(body.Message) == 0 {
		fail(w, http.StatusBadRequest, "bad_request", "message is required")
		return
	}
	content, foreign, err := rel.Render(body.Values, body.Lang)
	if err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "%v", err)
		return
	}
	if len(foreign) > 0 && !body.Rewrite {
		doc := map[string]any{"error": map[string]any{
			"code":    "foreign_comments",
			"message": "the file has comments the render would lose — rewrite them away or put the values back by hand",
			"detail":  map[string]any{"foreign": foreign},
		}}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(doc)
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
	path := body.Path + "/values.yaml"
	sha, err := repo.Write(r.Context(), branch, body.SHA, map[string][]byte{path: content}, author, body.Message)
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
	synced := true
	if err := s.op.Sync(r.Context(), p.Repo, branch); err != nil {
		synced = false
	}
	answer(w, map[string]any{"sha": sha, "branch": branch, "path": path, "synced": synced})
}
