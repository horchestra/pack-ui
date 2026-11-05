package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Scaffold: the spec's source written in the repository, committed; the operator synced.
func TestAuthorScaffold(t *testing.T) {
	rg := newRig(t, "direct")
	spec := map[string]any{
		"name": "kv", "version": "1.0.0",
		"fields": []map[string]any{{"name": "value", "type": "string", "required": true}},
	}
	status, body := post(t, rg, "/api/repo/dev/author/scaffold", map[string]any{
		"spec": spec, "lang": "go", "dir": "packs/kv",
	})
	if status != http.StatusOK {
		t.Fatalf("status %d: %v", status, body)
	}
	kv := rg.remoteFile(t, "packs/kv/kv.go")
	if !strings.Contains(kv, "Value string") {
		t.Errorf("the source: %s", kv)
	}
	rg.remoteFile(t, "packs/kv/tests/basic.yaml")
	// a spec that is no spec is a 400, nothing committed
	head := rg.head(t)
	status, _ = post(t, rg, "/api/repo/dev/author/scaffold", map[string]any{
		"spec": map[string]any{"name": ""}, "lang": "go", "dir": "packs/none",
	})
	if status != http.StatusBadRequest {
		t.Errorf("a bad spec: status %d", status)
	}
	if rg.head(t) != head {
		t.Error("a bad spec committed")
	}
}

// The schema a spec is constrained by is served.
func TestAuthorScaffoldSchema(t *testing.T) {
	rg := rigWith(t, testConfig(t, "http://127.0.0.1:1"), nil)
	resp, err := http.Get(rg.srv.URL + "/api/author/scaffold/schema")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc["type"] != "object" {
		t.Errorf("the schema: %v", doc["type"])
	}
}

// ref and example: the guide's parts and the example packs' sources.
func TestAuthorRefExample(t *testing.T) {
	rg := rigWith(t, testConfig(t, "http://127.0.0.1:1"), nil)
	resp, err := http.Get(rg.srv.URL + "/api/author/ref?topic=cel&lang=go")
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ref); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !strings.Contains(ref.Text, "CEL") && !strings.Contains(ref.Text, "formula") {
		t.Errorf("the ref: %s", ref.Text[:100])
	}
	resp, err = http.Get(rg.srv.URL + "/api/author/example?name=postgres&lang=go")
	if err != nil {
		t.Fatal(err)
	}
	var ex struct {
		Files map[string]string `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ex); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(ex.Files) == 0 {
		t.Error("no files of the example")
	}
}
