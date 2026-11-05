package server

import (
	"bytes"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// packBin is the pack binary init runs (C5), built once for the package's tests.
var (
	packBin     string
	packBinOnce sync.Once
)

func buildPack(t *testing.T) string {
	t.Helper()
	packBinOnce.Do(func() {
		bin := filepath.Join(os.TempDir(), "pack-ui-test-pack")
		build := exec.Command("go", "build", "-o", bin, "./cmd/pack")
		build.Dir = filepath.Join("..", "..", "..")
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("the pack build: %v\n%s", err, out)
		}
		packBin = bin
	})
	return packBin
}

// kvPack is an authored pack of the repository (C5): one setting, nothing to bring.
var kvPack = map[string]string{
	"packs/kv/pack.yaml": `type: application/vnd.pack.pack.v1+json
repository: packs/kv:1.0.0
annotations:
  pack.name: kv
  pack.version: 1.0.0
items:
- from: file://schema.json
  type: application/vnd.pack.schema.v1+json
`,
	"packs/kv/schema.json": `{"type":"object","title":"KV","properties":{"key":{"type":"string","title":"Key"}},"additionalProperties":false}`,
}

// A release is made: pack init in the checkout, its files committed, the operator asked the refresh.
func TestInitRelease(t *testing.T) {
	rg := newRig(t, "direct")
	rg.cfg.Pack = buildPack(t)
	rg.commit(t, kvPack, "the kv pack")
	status, body := post(t, rg, "/api/repo/dev/init", map[string]any{
		"pack": "packs/kv", "path": "releases/prod/db", "target": "memory", "message": "the db release",
	})
	if status != http.StatusOK {
		t.Fatalf("status %d: %v", status, body)
	}
	// the files landed
	rel, err := rg.remoteFileE(t, "releases/prod/db/release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(rel), []byte("pack: ../../../packs/kv")) {
		t.Errorf("release.yaml names the pack from the release: %s", rel)
	}
	env := rg.remoteFile(t, "releases/prod/db/env.yaml")
	if !bytes.Contains([]byte(env), []byte("memory")) {
		t.Errorf("env.yaml: %s", env)
	}
	rg.remoteFile(t, "releases/prod/db/values.yaml")
	// the operator was asked the refresh of the new release
	if len(rg.requests) != 1 || rg.requests[0]["release"] != "dev:releases/prod/db" || rg.requests[0]["action"] != "refresh" {
		t.Errorf("the requests: %v", rg.requests)
	}
}

// A pack that is not there is init's error, said as a 400 with init's words.
func TestInitNoPack(t *testing.T) {
	rg := newRig(t, "direct")
	rg.cfg.Pack = buildPack(t)
	status, body := post(t, rg, "/api/repo/dev/init", map[string]any{
		"pack": "packs/none", "path": "releases/prod/db", "target": "memory",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status %d: %v", status, body)
	}
	if len(rg.requests) != 0 {
		t.Errorf("a failed init asks nothing: %v", rg.requests)
	}
	_ = body
}
