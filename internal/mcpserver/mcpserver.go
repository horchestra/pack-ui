// Package mcpserver is pack-ui's MCP server (C7): the tools a model works a release's values and a pack's
// source with, over pack-ui's own API — questions, answer, diagnose, author_check, author_docs, author_cel,
// author_scaffold, author_ref, author_example. What is a person's is not a tool's: no approval of a deploy
// request (a person signs it), no merge of a change request, no secret's value. The answer tool commits the
// values it writes — every change of the repository is a commit a person reviews.
package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Instructions are what the server tells the model of how to use it.
const Instructions = `Tools of pack-ui for the releases of its projects and the packs they hold. A project
is a repository pack-ui edits (its id), a release its directory in it. Values: questions asks what the
schema still wants for the values as they stand (required, suggested, problems); answer writes the values
— committed as the user, a commit a person reviews, so say what you wrote. diagnose is the case of the
release's failed saga: the class, the cause, the evidence, the ways out (a patch — offer it and let the
person confirm; a command — the person opens the request). Authoring: author_check generates the pack's
source and checks it (problems with where), author_docs the docs skeleton of a language, author_cel works
an expression out as the key's formula would, author_scaffold writes a new pack's source from a spec,
author_ref and author_example are the authoring reference and a ready pack to start from. Approving a
deploy is a person's: no tool approves, no tool merges. Secrets are never read: a password is a reference,
name and key.`

// Client is pack-ui's API as the tools reach it.
type Client struct {
	Base  string       // pack-ui's URL (http://127.0.0.1:8090)
	User  string       // the X-Pack-User header, empty — none
	Token string       // Authorization: Bearer (C9 — a session is a person's; a model carries a token)
	HTTP  *http.Client // nil — http.DefaultClient
}

// call is one API call as a tool's answer: a 2xx is the document, an error the Failure's message.
func (c *Client) call(ctx context.Context, method, path string, body any) (*mcp.CallToolResult, any, error) {
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(c.Base, "/")+path, rd)
	if err != nil {
		return nil, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if len(c.User) > 0 {
		req.Header.Set("X-Pack-User", c.User)
	}
	if len(c.Token) > 0 {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var doc struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &doc) == nil && len(doc.Error.Code) > 0 {
			return nil, nil, fmt.Errorf("%s: %s", doc.Error.Code, doc.Error.Message)
		}
		return nil, nil, fmt.Errorf("%d: %s", resp.StatusCode, bytes.TrimSpace(raw))
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(bytes.TrimSpace(raw))}}}, nil, nil
}

// The tools' inputs. project is pack-ui's project id; path the release's or the pack source's directory
// in the repository.

type questionsIn struct {
	Project string         `json:"project" jsonschema:"pack-ui's project id (dev)"`
	Path    string         `json:"path" jsonschema:"the release's directory in the repository (releases/prod/db)"`
	Values  map[string]any `json:"values" jsonschema:"the values as they stand now"`
	Lang    string         `json:"lang,omitempty" jsonschema:"the language of the help (ru), empty — the base"`
}

type answerIn struct {
	Project string         `json:"project" jsonschema:"pack-ui's project id"`
	Path    string         `json:"path" jsonschema:"the release's directory"`
	Values  map[string]any `json:"values" jsonschema:"the values, whole — they are written as the file is"`
	Message string         `json:"message" jsonschema:"the commit message — say what the values change and why"`
	Lang    string         `json:"lang,omitempty" jsonschema:"the language of the comments, empty — the base"`
}

type diagnoseIn struct {
	Project string `json:"project" jsonschema:"pack-ui's project id"`
	Path    string `json:"path" jsonschema:"the release's directory"`
	Saga    string `json:"saga,omitempty" jsonschema:"the saga to diagnose, empty — the newest"`
	Key     string `json:"key,omitempty" jsonschema:"the failed node, empty — the saga's own"`
	Lang    string `json:"lang,omitempty" jsonschema:"the language of the causes, empty — the base"`
}

type authorPathIn struct {
	Project string `json:"project" jsonschema:"pack-ui's project id"`
	Path    string `json:"path" jsonschema:"the pack's directory in the repository (packs/openbao)"`
	Lang    string `json:"lang,omitempty" jsonschema:"author_docs: the docs' language (ru)"`
}

type authorCELIn struct {
	Project string         `json:"project" jsonschema:"pack-ui's project id"`
	Path    string         `json:"path" jsonschema:"the pack's directory in the repository"`
	Key     string         `json:"key,omitempty" jsonschema:"the key the formula is of, empty — the root's"`
	Expr    string         `json:"expr" jsonschema:"the CEL expression"`
	Values  map[string]any `json:"values,omitempty" jsonschema:"the values the expression reads"`
	Facts   map[string]any `json:"facts,omitempty" jsonschema:"the facts it reads"`
}

type authorScaffoldIn struct {
	Project string          `json:"project" jsonschema:"pack-ui's project id"`
	Spec    json.RawMessage `json:"spec" jsonschema:"the pack's spec (JSON, its schema is GET /api/author/scaffold/schema)"`
	Lang    string          `json:"lang" jsonschema:"the source's language: go | python | java"`
	Dir     string          `json:"dir" jsonschema:"where the source goes in the repository (packs/<name>)"`
	Message string          `json:"message,omitempty" jsonschema:"the commit message"`
}

type langIn struct {
	Lang string `json:"lang,omitempty" jsonschema:"go | python | java, empty — go"`
}

type exampleIn struct {
	Name string `json:"name" jsonschema:"the example's name, as author_ref lists them"`
	Lang string `json:"lang,omitempty" jsonschema:"go | python | java, empty — go"`
}

// New is pack-ui's MCP server (C7).
func New(c *Client, version string) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "pack-ui", Version: version},
		&mcp.ServerOptions{Instructions: Instructions})

	mcp.AddTool(srv, &mcp.Tool{Name: "questions", Description: "What the values of a release still ask: the settings required and missing, the suggestions, the problems — every one where the schema points."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in questionsIn) (*mcp.CallToolResult, any, error) {
			return c.call(ctx, "POST", "/api/repo/"+url.PathEscape(in.Project)+"/values/check",
				map[string]any{"path": in.Path, "values": in.Values, "lang": in.Lang})
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "answer", Description: "Write the release's values: rendered as pack values write renders them, committed as the user, the operator synced. A commit a person reviews — say what changed."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in answerIn) (*mcp.CallToolResult, any, error) {
			return c.call(ctx, "POST", "/api/repo/"+url.PathEscape(in.Project)+"/values/write",
				map[string]any{"path": in.Path, "values": in.Values, "message": in.Message, "lang": in.Lang})
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "diagnose", Description: "The case of the release's failed saga: the class, the cause, the evidence of the journal and the logs, the ways out — a patch the person confirms, a command the person runs as a request."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in diagnoseIn) (*mcp.CallToolResult, any, error) {
			return c.call(ctx, "POST", "/api/repo/"+url.PathEscape(in.Project)+"/diagnose",
				map[string]any{"path": in.Path, "saga": in.Saga, "key": in.Key, "lang": in.Lang})
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "author_check", Description: "Generate the pack's source and check it: the generate problems, the schema check's problems with where, the scenarios' runs."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in authorPathIn) (*mcp.CallToolResult, any, error) {
			return c.call(ctx, "POST", "/api/repo/"+url.PathEscape(in.Project)+"/author/check",
				map[string]any{"path": in.Path})
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "author_docs", Description: "The docs/<lang>.yaml skeleton of the pack's source."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in authorPathIn) (*mcp.CallToolResult, any, error) {
			lang := in.Lang
			if len(lang) == 0 {
				lang = "ru"
			}
			return c.call(ctx, "POST", "/api/repo/"+url.PathEscape(in.Project)+"/author/docs",
				map[string]any{"path": in.Path, "lang": lang})
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "author_cel", Description: "Work a CEL expression out as a formula of the pack's key would read it — on the values, the facts; the target a stub."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in authorCELIn) (*mcp.CallToolResult, any, error) {
			return c.call(ctx, "POST", "/api/repo/"+url.PathEscape(in.Project)+"/author/cel",
				map[string]any{"path": in.Path, "key": in.Key, "expr": in.Expr, "values": in.Values, "facts": in.Facts})
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "author_scaffold", Description: "Write a new pack's source from a spec into the repository, committed. Its schema is GET /api/author/scaffold/schema (the author's page asks it)."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in authorScaffoldIn) (*mcp.CallToolResult, any, error) {
			return c.call(ctx, "POST", "/api/repo/"+url.PathEscape(in.Project)+"/author/scaffold",
				map[string]any{"spec": in.Spec, "lang": in.Lang, "dir": in.Dir, "message": in.Message})
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "author_ref", Description: "The authoring reference of a language: what a pack's source is, the sdk it uses."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in langIn) (*mcp.CallToolResult, any, error) {
			q := ""
			if len(in.Lang) > 0 {
				q = "?lang=" + url.QueryEscape(in.Lang)
			}
			return c.call(ctx, "GET", "/api/author/ref"+q, nil)
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "author_example", Description: "A ready pack of the examples, to start an author's work from."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in exampleIn) (*mcp.CallToolResult, any, error) {
			q := "?name=" + url.QueryEscape(in.Name)
			if len(in.Lang) > 0 {
				q += "&lang=" + url.QueryEscape(in.Lang)
			}
			return c.call(ctx, "GET", "/api/author/example"+q, nil)
		})
	return srv
}
