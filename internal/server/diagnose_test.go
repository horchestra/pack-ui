package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// The diagnosis' rig: the seeded release, the operator's journal of a failed saga (an SSE replay) and
// logs, the pack's schema carrying an x-pack-diagnose that matches the failure. The case comes back with
// the pack's own diagnosis, the evidence of the journal and the logs, and the ways out.
func TestDiagnose(t *testing.T) {
	rg := newRig(t, "direct")
	// the schema with the pack's own diagnosis on its root
	schema := map[string]any{}
	if err := json.Unmarshal([]byte(mustRead(t, packFixture+"/schema.json")), &schema); err != nil {
		t.Fatal(err)
	}
	schema["x-pack-diagnose"] = []map[string]any{{
		"id":    "port-taken",
		"match": "address already in use",
		"cause": "the port the pack wants is taken",
		"fix": []map[string]any{
			{"command": "pack discover", "why": "see what listens on the hosts"},
			{"patch": "/openbao/ui", "expr": "true", "why": "a patched setting"},
		},
	}}
	raw, _ := json.Marshal(schema)
	files := map[string]string{
		"releases/prod/db/release.yaml": "pack: ../../../packs/openbao\nstate: db\nid: 11111111-2222-3333-4444-555555555555\n",
		"packs/openbao/pack.yaml":       mustRead(t, packFixture+"/pack.yaml"),
		"packs/openbao/schema.json":     string(raw),
		"packs/tembo/pack.yaml":         mustRead(t, packFixture+"/../tembo/pack.yaml"),
		"packs/tembo/schema.json":       mustRead(t, packFixture+"/../tembo/schema.json"),
	}
	rg.commit(t, files, "the openbao pack with a diagnosis")

	rg.opExtra = func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sagas"):
			_ = json.NewEncoder(w).Encode([]map[string]string{{"id": "s-1", "outcome": "failed"}})
		case strings.HasSuffix(r.URL.Path, "/events"):
			w.Header().Set("Content-Type", "text/event-stream")
			for _, ev := range []map[string]string{
				{"key": "openbao", "status": "running", "message": "starting the container"},
				{"key": "openbao", "status": "failed", "error": "start failed"},
			} {
				b, _ := json.Marshal(ev)
				fmt.Fprintf(w, "id: 1\ndata: %s\n\n", b)
			}
		case strings.HasSuffix(r.URL.Path, "/logs"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"logs": map[string]map[string][]string{
					"openbao": {"inst-1": {"listen tcp :8200: bind: address already in use"}},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	code, out := post(t, rg, "/api/repo/dev/diagnose", map[string]any{"path": "releases/prod/db"})
	if code != http.StatusOK {
		t.Fatalf("%d: %v", code, out)
	}
	if out["class"] != "port-taken" || out["diagnosis"] != "port-taken" || out["key"] != "openbao" || out["saga"] != "s-1" {
		t.Errorf("the case: %v", out)
	}
	evidence, _ := out["evidence"].([]any)
	var joined string
	for _, e := range evidence {
		joined += fmt.Sprint(e) + "\n"
	}
	if !strings.Contains(joined, "address already in use") {
		t.Errorf("the evidence: %v", evidence)
	}
	remedies, _ := out["remedies"].([]any)
	if len(remedies) != 2 {
		t.Fatalf("the remedies: %v", remedies)
	}
	r1, _ := remedies[1].(map[string]any)
	if r1["patch"] != "/openbao/ui" || r1["value"] != true {
		t.Errorf("the patched remedy: %v", r1)
	}
}

// Without the pack's own diagnosis matching, the case is the kind every pack shares.
func TestDiagnoseBuiltinKind(t *testing.T) {
	rg := newRig(t, "direct")
	seedPack(t, rg)
	rg.opExtra = func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sagas"):
			_ = json.NewEncoder(w).Encode([]map[string]string{{"id": "s-9", "outcome": "failed"}})
		case strings.HasSuffix(r.URL.Path, "/events"):
			w.Header().Set("Content-Type", "text/event-stream")
			b, _ := json.Marshal(map[string]string{"key": "openbao", "status": "failed", "error": "dial tcp: address already in use"})
			fmt.Fprintf(w, "data: %s\n\n", b)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
	code, out := post(t, rg, "/api/repo/dev/diagnose", map[string]any{"path": "releases/prod/db", "logs": false})
	if code != http.StatusOK {
		t.Fatalf("%d: %v", code, out)
	}
	if out["class"] != "port_conflict" {
		t.Errorf("the case: %v", out)
	}
}

// A release that ran no saga yet has nothing to diagnose.
func TestDiagnoseNoSaga(t *testing.T) {
	rg := newRig(t, "direct")
	seedPack(t, rg)
	rg.opExtra = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]string{})
	}
	code, out := post(t, rg, "/api/repo/dev/diagnose", map[string]any{"path": "releases/prod/db"})
	if code != http.StatusNotFound {
		t.Errorf("%d: %v", code, out)
	}
}
