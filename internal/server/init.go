package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/horchestra/pack-ui/internal/edit"
)

// initBody is the body of POST /api/repo/{id}/init (C5): the pack — an oci:// reference or a directory of
// the repository (an authored project) — where the release lives, and its environment.
type initBody struct {
	Pack         string `json:"pack"` // oci://registry/repo:tag or a directory in the repository
	Path         string `json:"path"` // the release's directory in the repository
	Target       string `json:"target"`
	TargetSource string `json:"target_source,omitempty"`
	TargetImage  string `json:"target_image,omitempty"`
	Runtime      string `json:"runtime,omitempty"`
	Lang         string `json:"lang,omitempty"`
	Message      string `json:"message,omitempty"`
	Branch       string `json:"branch,omitempty"` // a change request's branch, flow: review
}

// initRelease answers POST /api/repo/{id}/init: pack init in a checkout of the branch (no terminal — the
// target the form names), the files it made committed as the user and pushed, the operator asked to refresh
// the release (the lock is the operator's commit, B10).
func (s *Server) initRelease(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	var body initBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "the body: %v", err)
		return
	}
	body.Path = strings.Trim(body.Path, "/")
	if len(body.Pack) == 0 || len(body.Path) == 0 || len(body.Target) == 0 {
		fail(w, http.StatusBadRequest, "bad_request", "pack, path and target are required")
		return
	}
	if strings.Contains(body.Path, "..") {
		fail(w, http.StatusBadRequest, "bad_request", "the path leaves the repository: %q", body.Path)
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
	dir, err := os.MkdirTemp("", "pack-ui-init-*")
	if err != nil {
		fail(w, http.StatusInternalServerError, "error", "%v", err)
		return
	}
	defer os.RemoveAll(dir)
	sha, err := repo.Checkout(r.Context(), branch, dir)
	if err != nil {
		fail(w, http.StatusInternalServerError, "error", "the checkout: %v", err)
		return
	}
	// a pack of the repository is given absolute — init records it from the release's directory
	pack := body.Pack
	if !strings.Contains(pack, "://") {
		pack = filepath.Join(dir, filepath.Clean("/"+filepath.FromSlash(pack)))
	}
	args := []string{"init", pack, filepath.Join(dir, filepath.FromSlash(body.Path)), "--target", body.Target}
	if len(body.TargetSource) > 0 {
		args = append(args, "--target-source", body.TargetSource)
	}
	if len(body.TargetImage) > 0 {
		args = append(args, "--target-image", body.TargetImage)
	}
	if len(body.Runtime) > 0 {
		args = append(args, "--runtime", body.Runtime)
	}
	if len(body.Lang) > 0 {
		args = append(args, "--lang", body.Lang)
	}
	cmd := exec.CommandContext(r.Context(), s.cfg.Pack, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stdout = &stderr
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "pack init: %v — %s", err, strings.TrimSpace(stderr.String()))
		return
	}
	// the files init made, into the commit
	files := map[string][]byte{}
	root := filepath.Join(dir, filepath.FromSlash(body.Path))
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = raw
		return nil
	})
	if err != nil {
		fail(w, http.StatusInternalServerError, "error", "%v", err)
		return
	}
	if len(files) == 0 {
		fail(w, http.StatusInternalServerError, "error", "pack init made no files in %s", body.Path)
		return
	}
	// init names a pack of the repository absolute when the release is deeper than two ".." from it; the
	// release names it from where it is, so that the two move together
	if !strings.Contains(body.Pack, "://") {
		relYAML := body.Path + "/release.yaml"
		if raw, ok := files[relYAML]; ok {
			var doc map[string]any
			if err := yaml.Unmarshal(raw, &doc); err == nil {
				rel, err := filepath.Rel(filepath.Clean("/"+filepath.FromSlash(body.Path)), filepath.Clean("/"+filepath.FromSlash(body.Pack)))
				if err == nil {
					doc["pack"] = filepath.ToSlash(rel)
					if patched, err := yaml.Marshal(doc); err == nil {
						files[relYAML] = patched
					}
				}
			}
		}
	}
	message := body.Message
	if len(message) == 0 {
		message = fmt.Sprintf("init %s", body.Path)
	}
	author := s.user(r) + " <" + s.user(r) + "@localhost>"
	landed, err := repo.Write(r.Context(), branch, sha, files, author, message)
	if err != nil {
		var conflict *edit.Conflict
		if errors.As(err, &conflict) {
			fail(w, http.StatusConflict, "conflict", "%v", conflict)
			return
		}
		fail(w, http.StatusInternalServerError, "error", "%v", err)
		return
	}
	// the operator syncs the push — it knows the release — then refreshes: the pack locked and brought,
	// the lock its commit
	if err := s.opFor(r).Sync(r.Context(), p.ID, branch); err != nil {
		fail(w, http.StatusBadGateway, "error", "committed as %s, but the sync was not asked: %v", landed[:12], err)
		return
	}
	// the sync is the operator's own work: wait until it knows the release, then ask the refresh
	rel := p.ID + ":" + body.Path
	deadline := time.Now().Add(30 * time.Second)
	var out map[string]any
	for {
		if err := s.opFor(r).Get(r.Context(), "/api/releases/"+url.PathEscape(rel), &out); err == nil {
			break
		}
		if time.Now().After(deadline) {
			fail(w, http.StatusBadGateway, "error", "committed as %s, but the operator does not know %s after the sync", landed[:12], rel)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
	req, err := s.opFor(r).Request(r.Context(), rel, "refresh", landed, nil)
	if err != nil {
		fail(w, http.StatusBadGateway, "error", "committed as %s, but the refresh was not asked: %v", landed[:12], err)
		return
	}
	answer(w, map[string]any{"sha": landed, "branch": branch, "files": len(files), "request": req})
}
