// Package operator is the client of the operator's API (К4): REST, JSON, /api, the user in X-Pack-User until
// authentication lands (C9). Errors come back as the operator writes them — a Failure document, its code the
// one thing a caller branches on.
package operator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is one operator API, for one user.
type Client struct {
	Base   string // http://operator:9090
	User   string // X-Pack-User
	HTTP   *http.Client
}

// Failure is an error as the API writes it (pkg/api.Failure, mirrored — pack-ui does not import pack).
type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  any    `json:"detail,omitempty"`
}

func (f *Failure) Error() string { return f.Code + ": " + f.Message }

// Is says whether err is an API failure of the code (unauthorized, forbidden, busy, stale, conflict,
// not_found — К4).
func Is(err error, code string) bool {
	f, ok := err.(*Failure)
	return ok && f.Code == code
}

func (c *Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// Do runs one call: body (nil for none) JSON-encoded, the answer JSON-decoded into out (nil for none),
// a non-2xx read as a Failure.
func (c *Client) Do(ctx context.Context, method, path string, body, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, rd)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if len(c.User) > 0 {
		req.Header.Set("X-Pack-User", c.User)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return 0, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var doc struct {
			Error Failure `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil || len(doc.Error.Code) == 0 {
			return resp.StatusCode, &Failure{Code: "error", Message: resp.Status}
		}
		return resp.StatusCode, &doc.Error
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, fmt.Errorf("%s %s: the answer: %w", method, path, err)
		}
	} else {
		_, _ = io.Copy(io.Discard, resp.Body)
	}
	return resp.StatusCode, nil
}

// Get is a GET decoded into out.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	_, err := c.Do(ctx, "GET", path, nil, out)
	return err
}

// Post is a POST of body, the answer decoded into out.
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	_, err := c.Do(ctx, "POST", path, body, out)
	return err
}

// Sync asks the operator to fetch a repository now (К4: 202).
func (c *Client) Sync(ctx context.Context, repo, ref string) error {
	code, err := c.Do(ctx, "POST", "/api/sync", map[string]string{"repo": repo, "ref": ref}, nil)
	if err != nil {
		return err
	}
	if code != http.StatusAccepted && code != http.StatusOK {
		return &Failure{Code: "error", Message: fmt.Sprintf("sync: %d", code)}
	}
	return nil
}

// The typed reads (К4). The documents of pack itself (status, history, logs, actions, check, template, graph,
// schema, explain) come back as json.RawMessage — pack-ui passes them through, it does not read them.

// Project is an operator project.
type Project struct {
	ID      string `json:"id"`
	Repo    string `json:"repo"`
	Branch  string `json:"branch"`
	Flow    string `json:"flow"`
	Runtime string `json:"runtime"`
}

// Release is a release as the list shows it.
type Release struct {
	Path         string `json:"path"`
	Next         string `json:"next,omitempty"`
	SHA          string `json:"sha,omitempty"`
	InstalledSHA string `json:"installed_sha,omitempty"`
	State        string `json:"state,omitempty"`
	Health       string `json:"health,omitempty"`
	LastJob      string `json:"last_job,omitempty"`
}

// Request is a deploy request (К3) as the API returns it.
type Request struct {
	ID      string `json:"id"`
	Release string `json:"release"`
	Action  string `json:"action"`
	SHA     string `json:"sha"`
	State   string `json:"state"`
}

// Job is a job as the API lists it.
type Job struct {
	ID      string `json:"id"`
	Release string `json:"release"`
	Command string `json:"command"`
	State   string `json:"state"`
}

// Change is a change request.
type Change struct {
	ID     string `json:"id"`
	Branch string `json:"branch"`
	Author string `json:"author"`
	Title  string `json:"title"`
	State  string `json:"state"`
}

// Secret is a secret's name and keys — never a value (К4).
type Secret struct {
	Name string   `json:"name"`
	Keys []string `json:"keys"`
}

func (c *Client) Projects(ctx context.Context) ([]Project, error) {
	var out []Project
	return out, c.Get(ctx, "/api/projects", &out)
}

// Releases is the releases of a project.
func (c *Client) Releases(ctx context.Context, project string) ([]Release, error) {
	var out []Release
	return out, c.Get(ctx, "/api/projects/"+project+"/releases", &out)
}

// relEsc is a release id ("<project>:<path>") as one URL segment — the slashes of the path %2F, as the
// operator's routes want them (К4).
func relEsc(release string) string {
	return url.PathEscape(release)
}

// Raw is a synchronous pack document (К4): /api/releases/{rel}/{status|history|logs|actions|check|template|
// graph|schema|explain}, the query as the caller built it ("sha=…&lang=…").
func (c *Client) Raw(ctx context.Context, release, op, query string) (json.RawMessage, error) {
	path := "/api/releases/" + relEsc(release) + "/" + op
	if len(query) > 0 {
		path += "?" + query
	}
	var out json.RawMessage
	return out, c.Get(ctx, path, &out)
}

// Request opens a deploy request (К3): {action, sha, params}, idempotent.
func (c *Client) Request(ctx context.Context, release, action, sha string, params map[string]string) (*Request, error) {
	var out Request
	err := c.Post(ctx, "/api/releases/"+relEsc(release)+"/requests",
		map[string]any{"action": action, "sha": sha, "params": params}, &out)
	return &out, err
}

// Approve approves a request: the digest and the sha it was seen on.
func (c *Client) Approve(ctx context.Context, id, digest, sha string) error {
	return c.Post(ctx, "/api/requests/"+id+"/approve", map[string]string{"digest": digest, "sha": sha}, nil)
}

// Jobs is the jobs, newest first; release and state narrow it ("" — any).
func (c *Client) Jobs(ctx context.Context, release, state string) ([]Job, error) {
	var q []string
	if len(release) > 0 {
		q = append(q, "release="+release)
	}
	if len(state) > 0 {
		q = append(q, "state="+state)
	}
	path := "/api/jobs"
	if len(q) > 0 {
		path += "?" + strings.Join(q, "&")
	}
	var out []Job
	return out, c.Get(ctx, path, &out)
}

// CancelJob asks the job to stop.
func (c *Client) CancelJob(ctx context.Context, id string) error {
	return c.Post(ctx, "/api/jobs/"+id+"/cancel", nil, nil)
}

// Changes is the change requests, of a project when project is not "".
func (c *Client) Changes(ctx context.Context, project string) ([]Change, error) {
	path := "/api/changes"
	if len(project) > 0 {
		path += "?project=" + project
	}
	var out []Change
	return out, c.Get(ctx, path, &out)
}

// OpenChange opens a change request on a branch of a project; the operator makes the branch at the main
// branch's head when the remote has it not.
func (c *Client) OpenChange(ctx context.Context, project, branch, title string) (*Change, error) {
	var out Change
	err := c.Post(ctx, "/api/changes", map[string]string{"project": project, "branch": branch, "title": title}, &out)
	return &out, err
}

// MergeChange merges a change request; CloseChange closes it.
func (c *Client) MergeChange(ctx context.Context, id string) error {
	return c.Post(ctx, "/api/changes/"+id+"/merge", nil, nil)
}
func (c *Client) CloseChange(ctx context.Context, id string) error {
	return c.Post(ctx, "/api/changes/"+id+"/close", nil, nil)
}

// Secrets is the names and keys, never a value.
func (c *Client) Secrets(ctx context.Context, prefix string) ([]Secret, error) {
	path := "/api/secrets"
	if len(prefix) > 0 {
		path += "?prefix=" + prefix
	}
	var out []Secret
	return out, c.Get(ctx, path, &out)
}
