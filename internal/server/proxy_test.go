package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProxy(t *testing.T) {
	var gotUser, gotPath string
	op := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPath = r.Header.Get("X-Pack-User"), r.URL.Path
		if strings.HasSuffix(r.URL.Path, "/events") {
			// an SSE stream of two events
			w.Header().Set("Content-Type", "text/event-stream")
			fl, _ := w.(http.Flusher)
			fmt.Fprintf(w, "id: 1\ndata: {\"type\":\"log\"}\n\n")
			fl.Flush()
			fmt.Fprintf(w, "id: 2\ndata: {\"type\":\"result\"}\n\n")
			fl.Flush()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "1"})
	}))
	defer op.Close()
	cfg := testConfig(t, op.URL)
	rg := rigWith(t, cfg, nil)
	defer rg.srv.Close()

	// a JSON call proxied, the user asserted
	resp, err := http.Get(rg.srv.URL + "/api/op/version")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotPath != "/api/version" || gotUser != "alice" {
		t.Errorf("the operator saw %s as %q", gotPath, gotUser)
	}

	// a release id is one segment, its path's slashes encoded — the encoding reaches the operator
	resp, err = http.Get(rg.srv.URL + "/api/op/releases/dev%3Areleases%2Fprod%2Fdb")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotPath != "/api/releases/dev:releases/prod/db" {
		t.Errorf("the operator saw %q", gotPath)
	}

	// an SSE stream proxied as a stream: the first event arrives before the handler returns
	req, _ := http.NewRequest("GET", rg.srv.URL+"/api/op/jobs/j1/events", nil)
	req.Header.Set("Last-Event-ID", "0")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("the stream: %s", resp.Header.Get("Content-Type"))
	}
	lines := bufio.NewScanner(resp.Body)
	var events []string
	deadline := time.After(5 * time.Second)
	done := make(chan bool)
	go func() {
		for lines.Scan() {
			if strings.HasPrefix(lines.Text(), "data:") {
				events = append(events, lines.Text())
				if len(events) == 2 {
					break
				}
			}
		}
		done <- true
	}()
	select {
	case <-done:
	case <-deadline:
		t.Fatal("the stream did not deliver")
	}
	if len(events) != 2 || events[0] != "data: {\"type\":\"log\"}" || events[1] != "data: {\"type\":\"result\"}" {
		t.Errorf("the events: %v", events)
	}
}
