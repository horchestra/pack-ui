package operator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSync(t *testing.T) {
	var got struct {
		user  string
		repo  string
		ref   string
		calls int
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sync" || r.Method != "POST" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		got.user = r.Header.Get("X-Pack-User")
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		got.repo, got.ref = body["repo"], body["ref"]
		got.calls++
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ts.Close()
	c := &Client{Base: ts.URL, User: "alice"}
	if err := c.Sync(context.Background(), "ssh://git@host/packs.git", "main"); err != nil {
		t.Fatal(err)
	}
	if got.calls != 1 || got.user != "alice" || got.repo != "ssh://git@host/packs.git" || got.ref != "main" {
		t.Errorf("the call: %+v", got)
	}
}

func TestFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
			"code": "conflict", "message": "the branches changed the same files"}})
	}))
	defer ts.Close()
	c := &Client{Base: ts.URL, User: "alice"}
	err := c.Sync(context.Background(), "repo", "main")
	if err == nil {
		t.Fatal("an error was expected")
	}
	if !Is(err, "conflict") {
		t.Errorf("the code: %v", err)
	}
	if err.Error() != "conflict: the branches changed the same files" {
		t.Errorf("the message: %q", err.Error())
	}
}
