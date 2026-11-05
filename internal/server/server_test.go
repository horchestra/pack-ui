package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"

	uiconfig "github.com/horchestra/pack-ui/internal/config"
	"github.com/horchestra/pack-ui/internal/edit"
	"github.com/horchestra/pack-ui/internal/operator"
)

// rig is pack-ui over a bare file-transport remote and a fake operator that records /api/sync.
type rig struct {
	srv    *httptest.Server
	remote string
	seed   *git.Repository
	seedW  *git.Worktree
	dir    string
	syncs  []map[string]string
	users  []string
}

func newRig(t *testing.T, flow string) *rig {
	t.Helper()
	rg := &rig{}
	root := t.TempDir()
	dir := filepath.Join(root, "seed")
	seed, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, "refs/heads/main")); err != nil {
		t.Fatal(err)
	}
	remote := filepath.Join(root, "remote.git")
	bare, err := git.PlainInit(remote, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := bare.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, "refs/heads/main")); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{remote}}); err != nil {
		t.Fatal(err)
	}
	w, err := seed.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	rg.remote, rg.seed, rg.seedW, rg.dir = remote, seed, w, dir
	rg.commit(t, map[string]string{"values.yaml": "a: 1\n"}, "the beginning")
	op := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/sync" {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			rg.syncs = append(rg.syncs, body)
			rg.users = append(rg.users, r.Header.Get("X-Pack-User"))
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(op.Close)
	cfg := testConfig(t, op.URL)
	cfg.Projects = []uiconfig.Project{{ID: "dev", Repo: remote, Branch: "main", Flow: flow}}
	return rigWith(t, cfg, rg)
}

// testConfig is a configuration over a fake operator at opURL.
func testConfig(t *testing.T, opURL string) *uiconfig.Config {
	t.Helper()
	return &uiconfig.Config{
		Operator: opURL, Auth: "none", User: "alice",
		Repos: filepath.Join(t.TempDir(), "repos"),
		Git:   uiconfig.Git{Committer: "pack-ui <pack-ui@localhost>"},
	}
}

// rigWith serves the configuration; rg nil — one without a remote (the proxy's tests).
func rigWith(t *testing.T, cfg *uiconfig.Config, rg *rig) *rig {
	t.Helper()
	if rg == nil {
		rg = &rig{}
	}
	repos := edit.New(cfg.Repos, nil, cfg.Git.Committer)
	t.Cleanup(repos.Close)
	rg.srv = httptest.NewServer(New(cfg, &operator.Client{Base: cfg.Operator, User: cfg.User}, repos).Handler())
	t.Cleanup(rg.srv.Close)
	return rg
}

// commit commits files on the remote's main branch in the seed clone — someone else's change.
func (rg *rig) commit(t *testing.T, files map[string]string, message string) {
	t.Helper()
	for p, content := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(rg.dir, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(rg.dir, p), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := rg.seedW.Add(p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := rg.seedW.Commit(message, &git.CommitOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := rg.seed.Push(&git.PushOptions{RefSpecs: []config.RefSpec{"refs/heads/main:refs/heads/main"}}); err != nil {
		t.Fatal(err)
	}
}

func (rg *rig) head(t *testing.T) string {
	t.Helper()
	r, err := git.PlainOpen(rg.remote)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := r.Reference(plumbing.NewBranchReferenceName("main"), true)
	if err != nil {
		t.Fatal(err)
	}
	return ref.Hash().String()
}

func TestEditPushSync(t *testing.T) {
	rg := newRig(t, "direct")
	// read the file
	resp, err := http.Get(rg.srv.URL + "/api/repo/dev/file?path=values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Content string `json:"content"`
		SHA     string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&file); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if file.Content != "a: 1\n" || file.SHA != rg.head(t) {
		t.Errorf("the read: %+v", file)
	}
	// write it
	body := strings.NewReader(`{"path":"values.yaml","content":"a: 2\n","message":"a is 2"}`)
	req, _ := http.NewRequest("PUT", rg.srv.URL+"/api/repo/dev/file", body)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var wrote struct {
		SHA    string `json:"sha"`
		Branch string `json:"branch"`
		Synced bool   `json:"synced"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrote); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the write: %s", resp.Status)
	}
	if wrote.SHA != rg.head(t) || wrote.Branch != "main" || !wrote.Synced {
		t.Errorf("the write: %+v, the remote at %s", wrote, rg.head(t))
	}
	// the operator was asked to sync, as the user
	if len(rg.syncs) != 1 || rg.syncs[0]["ref"] != "main" || rg.syncs[0]["repo"] != rg.remote {
		t.Errorf("the syncs: %v", rg.syncs)
	}
	if len(rg.users) != 1 || rg.users[0] != "alice" {
		t.Errorf("the users: %v", rg.users)
	}
}

func TestEditConflict(t *testing.T) {
	rg := newRig(t, "direct")
	// the user read the file, then the other side moved it; the write comes with the read's sha
	resp, err := http.Get(rg.srv.URL + "/api/repo/dev/file?path=values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&file); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	rg.commit(t, map[string]string{"values.yaml": "a: 99\n"}, "a is 99")
	body := strings.NewReader(`{"path":"values.yaml","content":"a: 2\n","message":"a is 2","sha":"` + file.SHA + `"}`)
	req, _ := http.NewRequest("PUT", rg.srv.URL+"/api/repo/dev/file", body)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Error struct {
			Code   string `json:"code"`
			Detail struct {
				Files        []string `json:"files"`
				TheirSubject string   `json:"their_subject"`
			} `json:"detail"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict || doc.Error.Code != "conflict" {
		t.Fatalf("%s: %+v", resp.Status, doc)
	}
	if len(doc.Error.Detail.Files) != 1 || doc.Error.Detail.Files[0] != "values.yaml" ||
		doc.Error.Detail.TheirSubject != "a is 99" {
		t.Errorf("the detail: %+v", doc.Error.Detail)
	}
	if len(rg.syncs) != 0 {
		t.Errorf("a conflict syncs nothing: %v", rg.syncs)
	}
}

// The diff of a change request's branch against main: a file changed, a file new, a file gone.
func TestDiff(t *testing.T) {
	rg := newRig(t, "direct")
	rg.commit(t, map[string]string{"gone.txt": "away\n"}, "a file to go")
	// the branch off main's head
	w := rg.seedW
	if err := w.Checkout(&git.CheckoutOptions{Branch: "refs/heads/cr-x", Create: true}); err != nil {
		t.Fatal(err)
	}
	for p, content := range map[string]string{"values.yaml": "a: 2\n", "new.txt": "fresh\n"} {
		if err := os.WriteFile(filepath.Join(rg.dir, p), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Add(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(rg.dir, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Remove("gone.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Commit("the change", &git.CommitOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := rg.seed.Push(&git.PushOptions{RefSpecs: []config.RefSpec{"refs/heads/cr-x:refs/heads/cr-x"}}); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(rg.srv.URL + "/api/repo/dev/diff?head=cr-x")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var files []struct {
		Path string `json:"path"`
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		t.Fatal(err)
	}
	got := map[string][2]string{}
	for _, f := range files {
		got[f.Path] = [2]string{f.From, f.To}
	}
	want := map[string][2]string{
		"values.yaml": {"a: 1\n", "a: 2\n"},
		"new.txt":     {"", "fresh\n"},
		"gone.txt":    {"away\n", ""},
	}
	for p, wv := range want {
		if got[p] != wv {
			t.Errorf("%s: got %q, want %q (all: %v)", p, got[p], wv, got)
		}
	}
}

func TestReviewFlowBranch(t *testing.T) {
	rg := newRig(t, "review")
	// flow: review — a write without the change request's branch is refused
	body := strings.NewReader(`{"path":"values.yaml","content":"a: 2\n","message":"a is 2"}`)
	req, _ := http.NewRequest("PUT", rg.srv.URL+"/api/repo/dev/file", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("no branch: %s", resp.Status)
	}
}
