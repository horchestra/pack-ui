package server

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v6"
	"golang.org/x/crypto/bcrypt"

	uiconfig "github.com/horchestra/pack-ui/internal/config"
	"github.com/horchestra/pack-ui/internal/edit"
	"github.com/horchestra/pack-ui/internal/operator"
)

// The rig of C9 in pack-ui: auth local, alice's bcrypt password, the operator's service token recorded
// with the user on every call the fake operator sees.
type authRig struct {
	url   string
	op    *httptest.Server
	seen  []map[string]string // the operator's calls: user, auth, path
	users []string
}

func newAuthRig(t *testing.T) *authRig {
	t.Helper()
	rg := &authRig{}
	dir := t.TempDir()
	if _, err := git.PlainInit(dir, false); err != nil {
		t.Fatal(err)
	}
	rg.op = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rg.seen = append(rg.seen, map[string]string{
			"path": r.URL.Path, "user": r.Header.Get("X-Pack-User"), "auth": r.Header.Get("Authorization"),
		})
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(rg.op.Close)
	hash, err := bcrypt.GenerateFromPassword([]byte("alice-pw"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &uiconfig.Config{
		Operator: rg.op.URL, OperatorToken: "pko_service", Auth: "local",
		Users: []uiconfig.User{{Name: "alice", PasswordHash: string(hash)}},
		Tokens: []uiconfig.Token{
			{Name: "ci", Token: "pui_ci", User: "carol"},
		},
		Repos: t.TempDir(),
		Git:   uiconfig.Git{Committer: "pack-ui <pack-ui@localhost>"},
		Projects: []uiconfig.Project{
			{ID: "dev", Repo: "file://" + dir, Branch: "main", Flow: "direct"},
		},
	}
	repos := edit.New(cfg.Repos, nil, cfg.Git.Committer)
	t.Cleanup(repos.Close)
	srv := httptest.NewServer(New(cfg, &operator.Client{Base: cfg.Operator, Token: cfg.OperatorToken}, repos).Handler())
	t.Cleanup(srv.Close)
	rg.url = srv.URL
	return rg
}

func jar(t *testing.T) *http.Client {
	t.Helper()
	j, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: j}
}

// The login and what it opens: nothing before it (401), a wrong password stays out, the session's
// mutation wants the CSRF token; the operator sees the session's user with pack-ui's service token.
func TestAuthLocal(t *testing.T) {
	rg := newAuthRig(t)
	c := jar(t)
	// no session: 401, and whoami says so without one
	resp, err := c.Get(rg.url + "/api/repo/dev/file?path=values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no session: %s", resp.Status)
	}
	resp, err = c.Get(rg.url + "/api/whoami")
	if err != nil {
		t.Fatal(err)
	}
	var who map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&who)
	resp.Body.Close()
	if who["user"] != "" || who["auth"] != "local" {
		t.Fatalf("whoami without a session: %v", who)
	}
	// a wrong password
	resp, err = c.Post(rg.url+"/api/login", "application/json", strings.NewReader(`{"user":"alice","password":"nope"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a wrong password: %s", resp.Status)
	}
	// the login
	resp, err = c.Post(rg.url+"/api/login", "application/json", strings.NewReader(`{"user":"alice","password":"alice-pw"}`))
	if err != nil {
		t.Fatal(err)
	}
	var session map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&session)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(session["csrf"]) == 0 {
		t.Fatalf("the login: %s %v", resp.Status, session)
	}
	// a mutation with the cookie and no CSRF token is refused (PUT file meets a 404 of the repo read
	// only after the CSRF would pass — the refusal is the 403)
	req, _ := http.NewRequest("PUT", rg.url+"/api/repo/dev/file",
		strings.NewReader(`{"path":"a.txt","content":"x","message":"m"}`))
	resp, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("no CSRF: %s", resp.Status)
	}
	// with it, the mutation goes (the write itself fails on the empty repo — after the check)
	req, _ = http.NewRequest("PUT", rg.url+"/api/repo/dev/file",
		strings.NewReader(`{"path":"a.txt","content":"x","message":"m"}`))
	req.Header.Set("X-CSRF-Token", session["csrf"])
	resp, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("with CSRF: %s", resp.Status)
	}
	// whoami names the user and the token
	resp, err = c.Get(rg.url + "/api/whoami")
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewDecoder(resp.Body).Decode(&who)
	resp.Body.Close()
	if who["user"] != "alice" || who["csrf"] != session["csrf"] {
		t.Fatalf("whoami: %v", who)
	}
	// the proxy: the operator saw the session's user with the service token, no cookie
	resp, err = c.Get(rg.url + "/api/op/api/version")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(rg.seen) == 0 {
		t.Fatal("the operator saw nothing")
	}
	last := rg.seen[len(rg.seen)-1]
	if last["user"] != "alice" || last["auth"] != "Bearer pko_service" {
		t.Errorf("the operator saw: %v", last)
	}
	// a CI bearer of the configuration: no session, no CSRF — and the operator sees its user
	req, _ = http.NewRequest("GET", rg.url+"/api/op/api/version", nil)
	req.Header.Set("Authorization", "Bearer pui_ci")
	resp, err = (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("the CI bearer: %s", resp.Status)
	}
	last = rg.seen[len(rg.seen)-1]
	if last["user"] != "carol" {
		t.Errorf("the CI bearer is carol: %v", last)
	}
	// the logout ends it
	resp, err = c.Post(rg.url+"/api/logout", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	resp, err = c.Get(rg.url + "/api/repo/dev/file?path=values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after the logout: %s", resp.Status)
	}
}

// auth: none behaves as before: the configured user, no login, no CSRF.
func TestAuthNoneUnchanged(t *testing.T) {
	rg := newRig(t, "direct")
	resp, err := http.Get(rg.srv.URL + "/api/whoami")
	if err != nil {
		t.Fatal(err)
	}
	var who map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&who)
	resp.Body.Close()
	if who["user"] != "alice" || who["auth"] != "none" {
		t.Errorf("whoami: %v", who)
	}
}
