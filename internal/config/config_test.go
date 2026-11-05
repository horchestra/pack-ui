package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "pack-ui.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad(t *testing.T) {
	c, err := Load(write(t, `
listen: ":8081"
operator: "http://operator:9090/"
auth: none
user: alice
static: ./web/dist
repos: /var/lib/pack-ui/repos
secrets:
  provider: openbao
  addr: "https://bao:8200"
  mount: secret
  token_env: PACK_UI_BAO_TOKEN
git:
  committer: "pack-ui <pack-ui@example.org>"
projects:
  - id: dev
    repo: "ssh://git@git.example.org/infra/packs.git"
    credentials: secret:git/dev
    flow: review
  - id: plain
    repo: "https://git.example.org/infra/other.git"
`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != ":8081" || c.Operator != "http://operator:9090" || c.Auth != "none" || c.User != "alice" ||
		c.Static != "./web/dist" || c.Repos != "/var/lib/pack-ui/repos" {
		t.Errorf("the head: %+v", c)
	}
	if c.Secrets.Provider != "openbao" || c.Secrets.TokenEnv != "PACK_UI_BAO_TOKEN" {
		t.Errorf("secrets: %+v", c.Secrets)
	}
	if c.Git.Committer != "pack-ui <pack-ui@example.org>" {
		t.Errorf("git: %+v", c.Git)
	}
	if len(c.Projects) != 2 {
		t.Fatalf("projects: %+v", c.Projects)
	}
	dev, plain := c.Projects[0], c.Projects[1]
	if dev.ID != "dev" || dev.Branch != "main" || dev.Flow != "review" || dev.Credentials != "secret:git/dev" {
		t.Errorf("dev: %+v", dev)
	}
	if plain.Branch != "main" || plain.Flow != "direct" {
		t.Errorf("plain defaults: %+v", plain)
	}
	if c.Project("dev") == nil || c.Project("nope") != nil {
		t.Error("Project")
	}
}

func TestDefaultsAndRefusals(t *testing.T) {
	c, err := Load(write(t, "operator: http://op:9090\nrepos: /tmp/repos\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != ":8080" || c.Auth != "none" || c.User != "dev" || c.Git.Committer != "pack-ui <pack-ui@localhost>" {
		t.Errorf("the defaults: %+v", c)
	}
	for body, want := range map[string]string{
		"repos: /tmp/x\n":                                     "operator is required",
		"operator: http://op\n":                               "repos is required",
		"operator: http://op\nrepos: /x\nauth: oidc\n":        "oidc.issuer",
		"operator: http://op\nrepos: /x\nauth: local\n":      "users",
		"operator: http://op\nrepos: /x\ngit: {committer: x}\n": "is \"Name <address>\"",
		"operator: http://op\nrepos: /x\nprojects: [{id: a, repo: r}, {id: a, repo: r2}]\n": "twice",
		"operator: http://op\nrepos: /x\nprojects: [{id: a, repo: r, flow: sideways}]\n":    "direct or review",
	} {
		if _, err := Load(write(t, body)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: wanted %q, got %v", body, want, err)
		}
	}
}
