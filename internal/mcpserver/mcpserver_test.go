package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// rig is the MCP server over a fake pack-ui API that records what the tools asked of it.
type rig struct {
	t    *testing.T
	cs   *mcp.ClientSession
	seen []string
}

func newRig(t *testing.T) *rig {
	t.Helper()
	rg := &rig{t: t}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rg.seen = append(rg.seen, r.Method+" "+r.URL.Path)
		// a Failure document for an unknown path, an empty object otherwise
		if strings.HasSuffix(r.URL.Path, "/author/ref") {
			_ = json.NewEncoder(w).Encode(map[string]string{"text": "the reference"})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/values/check") {
			_ = json.NewEncoder(w).Encode(map[string]any{"questions": []any{}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "not_found", "message": "no such path"}})
	}))
	t.Cleanup(api.Close)
	st, ct := mcp.NewInMemoryTransports()
	msrv := New(&Client{Base: api.URL, User: "alice"}, "test")
	if _, err := msrv.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	rg.cs = cs
	return rg
}

func (rg *rig) call(name string, args map[string]any) (string, error) {
	rg.t.Helper()
	res, err := rg.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return "", err
	}
	return res.Content[0].(*mcp.TextContent).Text, nil
}

// The tools of C7: the list is whole, and nothing in it approves or merges — that is a person's.
func TestTools(t *testing.T) {
	rg := newRig(t)
	tools, err := rg.cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"questions", "answer", "diagnose", "author_check", "author_docs", "author_cel",
		"author_scaffold", "author_ref", "author_example"}
	if len(tools.Tools) != len(want) {
		t.Fatalf("the tools: %v", tools.Tools)
	}
	got := map[string]bool{}
	for _, tool := range tools.Tools {
		got[tool.Name] = true
		if strings.Contains(tool.Name, "approve") || strings.Contains(tool.Name, "merge") {
			t.Errorf("a person's act is no tool: %s", tool.Name)
		}
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("no tool %s", name)
		}
	}
}

// A reading tool passes through what the API says; a writing one carries the project into the path.
func TestCalls(t *testing.T) {
	rg := newRig(t)
	out, err := rg.call("author_ref", map[string]any{"lang": "go"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "the reference") {
		t.Errorf("author_ref: %s", out)
	}
	out, err = rg.call("questions", map[string]any{"project": "dev", "path": "releases/prod/db", "values": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "questions") {
		t.Errorf("questions: %s", out)
	}
	if rg.seen[0] != "GET /api/author/ref" || rg.seen[1] != "POST /api/repo/dev/values/check" {
		t.Errorf("the calls: %v", rg.seen)
	}
}

// An API error is the tool's error, the Failure's own words.
func TestFailure(t *testing.T) {
	rg := newRig(t)
	res, err := rg.cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "diagnose",
		Arguments: map[string]any{"project": "dev", "path": "releases/none"},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !res.IsError || !strings.Contains(text, "no such path") {
		t.Errorf("diagnose: isError=%v %s", res.IsError, text)
	}
}
