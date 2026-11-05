// The SPA end to end (C2): a real operator with a worker of the native runtime on a memory-target release,
// pack-ui serving the built SPA and proxying it, a browser (Playwright) driving the deploy — the projects
// page, the release card, a plan, its approval, the applied release.
package e2e

import (
	"io"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v6"
	ggconfig "github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"

	"github.com/horchestra/pack/sdk"
	"github.com/horchestra/pack/sdk/packschema"

	opgit "github.com/horchestra/pack-operator/git"
	opconfig "github.com/horchestra/pack-operator/config"
	opdb "github.com/horchestra/pack-operator/db"
	opserver "github.com/horchestra/pack-operator/server"
	"github.com/horchestra/pack-operator/worker"

	uiconfig "github.com/horchestra/pack-ui/internal/config"
	"github.com/horchestra/pack-ui/internal/edit"
	"github.com/horchestra/pack-ui/internal/operator"
	uiserver "github.com/horchestra/pack-ui/internal/server"
)

// kv is the fixture pack: one setting, no hooks — the smallest pack the sdk writes.
type kv struct {
	Value string `json:"value"`
}

func (*kv) SetDefaults()              {}
func (*kv) Meta() sdk.Meta            { return sdk.Meta{Name: "kv", Version: "1.0.0"} }
func (*kv) PackRoot() packschema.Root { return packschema.Root{} }

func TestE2E(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("no node — the browser run wants it")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := t.TempDir()

	// the pack binary the operator's jobs run
	bin := filepath.Join(root, "pack")
	build := exec.Command("go", "build", "-o", bin, "./cmd/pack")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("the pack build: %v\n%s", err, out)
	}
	// the SPA pack-ui serves
	web := exec.Command("npm", "run", "build")
	web.Dir = filepath.Join("..", "web")
	if out, err := web.CombinedOutput(); err != nil {
		t.Fatalf("the web build: %v\n%s", err, out)
	}

	// the project repository: packs/kv, releases/prod/db on the memory target, an approval on plan
	bare := filepath.Join(root, "repo.git")
	dir := filepath.Join(root, "seed")
	seed, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := sdk.Generate(filepath.Join(dir, "packs"), &kv{}); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("releases/prod/db/release.yaml",
		"pack: ../../../packs/kv\nstate: db\nid: 11111111-2222-3333-4444-555555555555\n")
	write("releases/prod/db/values.yaml", "value: one\n")
	write("releases/prod/db/env.yaml", `{"target": "memory", "run": {"approve": "plan"}}`+"\n")
	wt, err := seed.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("the start", &git.CommitOptions{Author: &object.Signature{Name: "alice", Email: "alice@example.org"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := git.PlainInit(bare, true); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.CreateRemote(&ggconfig.RemoteConfig{Name: "origin", URLs: []string{bare}}); err != nil {
		t.Fatal(err)
	}
	if err := seed.Push(&git.PushOptions{RefSpecs: []ggconfig.RefSpec{"refs/heads/master:refs/heads/main"}}); err != nil {
		t.Fatal(err)
	}
	b, err := git.PlainOpen(bare)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, "refs/heads/main")); err != nil {
		t.Fatal(err)
	}

	// the operator
	dataDir := t.TempDir()
	opCfg := &opconfig.Config{
		DB:               "sqlite://" + filepath.Join(t.TempDir(), "operator.db"),
		Auth:             "none",
		DevUser:          "dev",
		Pack:             bin,
		DataDir:          dataDir,
		StateDSN:         "sqlite://" + filepath.Join(t.TempDir(), "state.db"),
		EncryptionKeyEnv: "PACK_UI_E2E_KEY",
		Git:              opconfig.Git{FetchInterval: opconfig.Duration{Duration: time.Hour}},
		Projects:         []opconfig.Project{{ID: "dev", Repo: "file://" + bare, Branch: "main", Runtime: "native"}},
	}
	t.Setenv("PACK_UI_E2E_KEY", "test-key")
	opSrv, err := opserver.NewServer(ctx, opCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer opSrv.Close()
	opLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = opSrv.Serve(ctx, opLn) }()
	opURL := "http://" + opLn.Addr().String()
	d, err := opdb.Open(ctx, opCfg.DB)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	w := worker.New(opCfg, opSrv.Bus(), d, nil,
		opgit.NewRepos(filepath.Join(dataDir, "repos"), nil), "native", nil,
		worker.Native{Pack: bin, Log: func(line string) { t.Log(line) }})
	go func() { _ = w.Run(ctx) }()
	post(t, opURL+"/api/sync", `{"repo":"dev","ref":"main"}`)
	waitReleases(t, opURL)

	// pack-ui
	uiCfg := &uiconfig.Config{
		Operator: opURL, Auth: "none", User: "dev", // auth: none — dev is an admin in every domain
		Static: filepath.Join("..", "web", "dist"),
		Repos:  filepath.Join(t.TempDir(), "repos"),
		Pack:   bin,
		Git:    uiconfig.Git{Committer: "pack-ui <pack-ui@localhost>"},
		Projects: []uiconfig.Project{{
			ID: "dev", Repo: "file://" + bare, Branch: "main", Flow: "direct",
		}},
	}
	repos := edit.New(uiCfg.Repos, nil, uiCfg.Git.Committer)
	defer repos.Close()
	uiLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	uiSrv := &http.Server{Handler: uiserver.New(uiCfg, &operator.Client{Base: opURL, User: uiCfg.User}, repos).Handler()}
	go func() { _ = uiSrv.Serve(uiLn) }()
	defer uiSrv.Close()

	// the browser
	spec := exec.Command("npx", "playwright", "test")
	spec.Dir = "."
	spec.Env = append(os.Environ(), "PACK_UI_URL=http://"+uiLn.Addr().String())
	spec.Stdout = os.Stdout
	spec.Stderr = os.Stderr
	if err := spec.Run(); err != nil {
		t.Fatalf("the browser: %v", err)
	}

	// C5: a release is made through pack-ui — init's files committed; the values the pack asks written; the
	// operator's refresh done, the release listed, unfilled
	post(t, "http://"+uiLn.Addr().String()+"/api/repo/dev/init",
		`{"pack":"packs/kv","path":"releases/prod/cache","target":"memory","message":"the cache release"}`)
	req, _ := http.NewRequest("PUT", "http://"+uiLn.Addr().String()+"/api/repo/dev/file",
		strings.NewReader(`{"path":"releases/prod/cache/values.yaml","content":"value: two\n","message":"the values"}`))
	req.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil || resp2.StatusCode != http.StatusOK {
		t.Fatalf("the values write: %v %v", err, resp2)
	}
	resp2.Body.Close()
	post(t, opURL+"/api/releases/dev:releases%2Fprod%2Fcache/requests", `{"action":"refresh"}`)
	deadline := time.Now().Add(120 * time.Second)
	seen := map[string]bool{}
	for {
		if time.Now().After(deadline) {
			t.Fatalf("the new release did not land (seen: %v)", seen)
		}
		// a webhook would: the operator fetches what the refresh committed and says the states again
		if resp, err := http.Post(opURL+"/api/sync", "application/json", strings.NewReader(`{"repo":"dev","ref":"main"}`)); err == nil {
			resp.Body.Close()
		}
		resp, err := http.Get(opURL + "/api/projects/dev/releases")
		if err == nil && resp.StatusCode == http.StatusOK {
			var rels []struct {
				Path  string `json:"path"`
				State string `json:"state"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&rels)
			resp.Body.Close()
			for _, r := range rels {
				if r.Path == "releases/prod/cache" {
					seen[r.State] = true
					if r.State == "unfilled" || r.State == "planned" || r.State == "applied" {
						fmt.Println("the release is made:", r.Path, r.State)
						return
					}
				}
			}
		} else if err == nil {
			resp.Body.Close()
		}
		// the jobs the release's requests made: a refresh that succeeded is the release brought
		if resp, err := http.Get(opURL + "/api/jobs"); err == nil && resp.StatusCode == http.StatusOK {
			var jobs []struct {
				State string   `json:"state"`
				Argv  []string `json:"argv"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&jobs)
			resp.Body.Close()
			for _, j := range jobs {
				argv := strings.Join(j.Argv, " ")
				if j.State == "succeeded" && strings.Contains(argv, "releases/prod/cache") && strings.Contains(argv, "refresh") {
					seen["refreshed"] = true
					fmt.Println("the release is made: refreshed")
					return
				}
			}
		} else if err == nil {
			resp.Body.Close()
		}
		time.Sleep(time.Second)
	}
}

func post(t *testing.T, url, body string) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST %s: %s — %s", url, resp.Status, body2)
	}
}

func waitReleases(t *testing.T, opURL string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(opURL + "/api/projects/dev/releases")
		if err == nil && resp.StatusCode == http.StatusOK {
			var rels []struct {
				Path string `json:"path"`
				SHA  string `json:"sha"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&rels)
			resp.Body.Close()
			if len(rels) == 1 && len(rels[0].SHA) == 40 {
				fmt.Println("the release is synced:", rels[0].Path)
				return
			}
		} else if err == nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("the releases did not sync")
}
