package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"

	"github.com/horchestra/pack/pkg/bundle"
	"github.com/horchestra/pack/pkg/valuesfile"
)

// packFixture is the golden pack the sdk's tests write, vendored here — the pack-ui repository stands
// alone, there is no sdk checkout beside it.
const packFixture = "testdata/openbao"

// seedPack copies the fixture pack and a release using it into the remote.
func seedPack(t *testing.T, rg *rig) {
	t.Helper()
	files := map[string]string{
		"releases/prod/db/release.yaml": "pack: ../../../packs/openbao\nstate: db\nid: 11111111-2222-3333-4444-555555555555\n",
	}
	// the pack and the one it holds (resolved from the sibling directory)
	for _, pack := range []string{"openbao", "tembo"} {
		dir := filepath.Join(packFixture, "..", pack)
		for _, name := range []string{"pack.yaml", "schema.json"} {
			files["packs/"+pack+"/"+name] = mustRead(t, filepath.Join(dir, name))
		}
	}
	rg.commit(t, files, "the openbao pack and its release")
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// post is a POST of doc to the rig's server; the status and the decoded body.
func post(t *testing.T, rg *rig, path string, doc any) (int, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(rg.srv.URL+path, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	return resp.StatusCode, out
}

// remoteFile is the file at the remote's main head.
func (rg *rig) remoteFile(t *testing.T, path string) string {
	t.Helper()
	content, err := rg.remoteFileE(t, path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

// remoteFileE is remoteFile's error, for a caller that expects a file missing.
func (rg *rig) remoteFileE(t *testing.T, path string) (string, error) {
	t.Helper()
	r, err := git.PlainOpen(rg.remote)
	if err != nil {
		return "", err
	}
	ref, err := r.Reference(plumbing.NewBranchReferenceName("main"), true)
	if err != nil {
		return "", err
	}
	c, err := r.CommitObject(ref.Hash())
	if err != nil {
		return "", err
	}
	tree, err := c.Tree()
	if err != nil {
		return "", err
	}
	f, err := tree.File(path)
	if err != nil {
		return "", err
	}
	return f.Contents()
}

var givenValues = map[string]any{
	"openbao":  map[string]any{"api_addr": "https://bao:8200", "ui": true},
	"postgres": map[string]any{"pg_port": float64(5440), "node": map[string]any{"nofailover": true}},
}

// The values the form writes land in the repository byte for byte as pack values write would write them
// (pkg/valuesfile — the same call cmd/values.go's writeValues makes), and the operator is asked to sync.
func TestValuesWriteMatchesPack(t *testing.T) {
	rg := newRig(t, "direct")
	seedPack(t, rg)
	status, body := post(t, rg, "/api/repo/dev/values/write", map[string]any{
		"path": "releases/prod/db", "values": givenValues, "message": "set the values", "lang": "en",
	})
	if status != http.StatusOK {
		t.Fatalf("status %d: %v", status, body)
	}
	if body["synced"] != true {
		t.Fatalf("not synced: %v", body)
	}
	b, err := bundle.Load(packFixture)
	if err != nil {
		t.Fatal(err)
	}
	want, err := valuesfile.Write(b, givenValues, valuesfile.Sources{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := rg.remoteFile(t, "releases/prod/db/values.yaml"); got != string(want) {
		t.Errorf("the landed values.yaml is not pack values write's:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	if len(rg.syncs) == 0 {
		t.Error("the operator was not asked to sync")
	}
}

// A file with comments of the operator's own is refused (409 foreign_comments) until rewrite is set.
func TestValuesWriteForeign(t *testing.T) {
	rg := newRig(t, "direct")
	seedPack(t, rg)
	rg.commit(t, map[string]string{
		"releases/prod/db/values.yaml": "# a note the operator wrote by hand\nopenbao:\n  api_addr: https://bao:8200\n",
	}, "a hand-written note")
	doc := map[string]any{"path": "releases/prod/db", "values": givenValues, "message": "set the values"}
	status, body := post(t, rg, "/api/repo/dev/values/write", doc)
	if status != http.StatusConflict {
		t.Fatalf("status %d: %v", status, body)
	}
	if body["error"].(map[string]any)["code"] != "foreign_comments" {
		t.Fatalf("not foreign_comments: %v", body)
	}
	doc["rewrite"] = true
	status, body = post(t, rg, "/api/repo/dev/values/write", doc)
	if status != http.StatusOK {
		t.Fatalf("rewrite: status %d: %v", status, body)
	}
}

// Check is the form's live help: the values as they stand ask questions or report problems, never an error
// for an empty form.
func TestValuesCheck(t *testing.T) {
	rg := newRig(t, "direct")
	seedPack(t, rg)
	status, body := post(t, rg, "/api/repo/dev/values/check", map[string]any{
		"path": "releases/prod/db", "values": map[string]any{},
	})
	if status != http.StatusOK {
		t.Fatalf("status %d: %v", status, body)
	}
	if _, ok := body["questions"]; !ok {
		if _, ok := body["problems"]; !ok {
			t.Fatalf("neither questions nor problems: %v", body)
		}
	}
}

// A release whose pack is an OCI reference is a 400 that says why.
func TestValuesOCIRefused(t *testing.T) {
	rg := newRig(t, "direct")
	rg.commit(t, map[string]string{
		"releases/prod/db/release.yaml": "pack: oci://registry.example.com/packs/openbao:1.0.0\nstate: db\nid: 11111111-2222-3333-4444-555555555555\n",
	}, "a release of a referenced pack")
	status, body := post(t, rg, "/api/repo/dev/values/check", map[string]any{
		"path": "releases/prod/db", "values": map[string]any{},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status %d: %v", status, body)
	}
}
