package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/horchestra/pack-ui/internal/values"
	"github.com/horchestra/pack/pkg/diagnosis"
	"github.com/horchestra/pack/pkg/render"
)

// diagnoseBody is the body of POST /api/repo/{id}/diagnose.
type diagnoseBody struct {
	Path string `json:"path"`
	Lang string `json:"lang,omitempty"`
	Saga string `json:"saga,omitempty"` // "" — the newest
	Key  string `json:"key,omitempty"`  // "" — the saga's failed node
	Logs *bool  `json:"logs,omitempty"` // nil — with logs
}

// diagnose answers POST: the case of the release's failure as pkg/diagnosis works it — the saga and the
// failed node out of the operator's journal (its API), the journal's lines of the node and the tail of its
// logs, the pack's own diagnoses of the checkout's pack with the fixes worked out on the values as the file
// holds them. The ways out come back as data: a patch the user confirms before the values are committed
// (the values' endpoints), a command a request of the operator's runs — nothing here runs anything.
func (s *Server) diagnose(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	var body diagnoseBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "the body: %v", err)
		return
	}
	if len(body.Path) == 0 {
		fail(w, http.StatusBadRequest, "bad_request", "path is required")
		return
	}
	repo := s.repo(w, r, p)
	if repo == nil {
		return
	}
	dir, err := os.MkdirTemp("", "pack-ui-diagnose-*")
	if err != nil {
		fail(w, http.StatusInternalServerError, "error", "%v", err)
		return
	}
	defer os.RemoveAll(dir)
	if _, err := repo.Checkout(r.Context(), p.Branch, dir); err != nil {
		fail(w, http.StatusInternalServerError, "error", "the checkout: %v", err)
		return
	}
	rel, vals, err := values.Load(dir, body.Path)
	if err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "%v", err)
		return
	}
	releaseID := p.ID + ":" + body.Path
	in := diagnosis.Input{Lang: body.Lang}
	sagas, err := s.op.Sagas(r.Context(), releaseID)
	if err != nil {
		fail(w, http.StatusBadGateway, "error", "the operator's sagas: %v", err)
		return
	}
	saga := body.Saga
	if len(saga) == 0 {
		if len(sagas) == 0 {
			fail(w, http.StatusNotFound, "not_found", "%s ran no saga yet — nothing to diagnose", body.Path)
			return
		}
		saga = sagas[0].ID
	}
	in.Saga = saga
	events, err := s.op.SagaEvents(r.Context(), releaseID, saga)
	if err != nil {
		fail(w, http.StatusBadGateway, "error", "the saga's journal: %v", err)
		return
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Status == "failed" && len(events[i].Key) > 0 {
			in.Key, in.Error = events[i].Key, events[i].Error
			break
		}
	}
	if len(body.Key) > 0 {
		in.Key = body.Key
	}
	if len(in.Key) == 0 {
		fail(w, http.StatusNotFound, "not_found", "the saga %s has no failed node", saga)
		return
	}
	for _, e := range events {
		if e.Key == in.Key {
			if line := strings.TrimSpace(e.Message + " " + e.Error); len(line) > 0 {
				in.Lines = append(in.Lines, line)
			}
		}
	}
	if body.Logs == nil || *body.Logs {
		if logs, err := s.op.Logs(r.Context(), releaseID, in.Key, 50); err == nil && logs != nil {
			for inst, lines := range logs.Logs[in.Key] {
				for _, line := range lines {
					in.Lines = append(in.Lines, inst+": "+line)
				}
			}
		}
	}
	if b, err := rel.Bundle(); err == nil {
		key := in.Key
		if pk := b.Packs[b.KeyPack[key]]; pk != nil {
			in.Diagnoses = pk.Schema.Root.Diagnoses
		}
		rc := render.Context{Installation: rel.Installation(), Target: values.StubTarget{}}
		in.Eval = func(expr string) (any, error) { return render.Eval(b, render.Values{Values: vals}, rc, key, expr) }
		in.Pointer = func(ptr string) string { return b.External("/" + key + ptr) }
	}
	answer(w, diagnosis.Make(in))
}
