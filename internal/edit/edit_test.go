package edit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

// rig is a bare remote, a seed clone standing in for everyone else, and the pack-ui working copies — all on
// the filesystem: the git protocol's file transport is enough for the commit/push/rebase logic (the
// credentials and the SSH/HTTPS transports are the operator's tests').
type rig struct {
	remote string
	seed   *git.Repository
	seedW  *git.Worktree
	dir    string
	repos  *Repos
}

func newRig(t *testing.T) *rig {
	t.Helper()
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
	rg := &rig{remote: remote, seed: seed, seedW: w, dir: dir,
		repos: New(filepath.Join(root, "repos"), nil, "pack-ui <pack-ui@localhost>")}
	rg.commit(t, "main", map[string]string{"values.yaml": "a: 1\n", "notes.txt": "notes\n"}, "the beginning")
	return rg
}

// commit commits files on the branch in the seed clone and pushes — someone else's change, the one a push
// meets.
func (rg *rig) commit(t *testing.T, branch string, files map[string]string, message string) string {
	t.Helper()
	if branch != "main" {
		head, err := rg.seed.Head()
		if err != nil {
			t.Fatal(err)
		}
		ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branch), head.Hash())
		if err := rg.seed.Storer.SetReference(ref); err != nil {
			t.Fatal(err)
		}
		if err := rg.seedW.Checkout(&git.CheckoutOptions{Branch: ref.Name()}); err != nil {
			t.Fatal(err)
		}
	}
	for p, content := range files {
		full := filepath.Join(rg.dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := rg.seedW.Add(p); err != nil {
			t.Fatal(err)
		}
	}
	who := &object.Signature{Name: "other", Email: "other@localhost", When: time.Now()}
	h, err := rg.seedW.Commit(message, &git.CommitOptions{Author: who, Committer: who})
	if err != nil {
		t.Fatal(err)
	}
	refspec := plumbing.NewBranchReferenceName(branch).String() + ":" + plumbing.NewBranchReferenceName(branch).String()
	if err := rg.seed.Push(&git.PushOptions{RefSpecs: []config.RefSpec{config.RefSpec(refspec)}}); err != nil {
		t.Fatal(err)
	}
	return h.String()
}

// branch makes a branch of the bare remote at the main branch's head — what the operator does when a change
// request opens.
func (rg *rig) branch(t *testing.T, name string) {
	t.Helper()
	head, err := rg.seed.Reference(plumbing.NewBranchReferenceName("main"), true)
	if err != nil {
		t.Fatal(err)
	}
	refspec := config.RefSpec(head.Hash().String() + ":" + plumbing.NewBranchReferenceName(name).String())
	if err := rg.seed.Push(&git.PushOptions{RefSpecs: []config.RefSpec{refspec}}); err != nil {
		t.Fatal(err)
	}
}

// head is the sha the remote's branch is at.
func (rg *rig) head(t *testing.T, branch string) string {
	t.Helper()
	r, err := git.PlainOpen(rg.remote)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := r.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err != nil {
		t.Fatal(err)
	}
	return ref.Hash().String()
}

// file is a file of a commit on the remote.
func (rg *rig) file(t *testing.T, sha, path string) string {
	t.Helper()
	r, err := git.PlainOpen(rg.remote)
	if err != nil {
		t.Fatal(err)
	}
	c, err := r.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		t.Fatal(err)
	}
	tree, err := c.Tree()
	if err != nil {
		t.Fatal(err)
	}
	f, err := tree.File(path)
	if err != nil {
		t.Fatal(err)
	}
	content, err := f.Contents()
	if err != nil {
		t.Fatal(err)
	}
	return content
}

// commitOf is a commit of the remote.
func (rg *rig) commitOf(t *testing.T, sha string) *object.Commit {
	t.Helper()
	r, err := git.PlainOpen(rg.remote)
	if err != nil {
		t.Fatal(err)
	}
	c, err := r.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (rg *rig) get(t *testing.T) *Repo {
	t.Helper()
	repo, err := rg.repos.Get(context.Background(), "dev", rg.remote, "")
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestWritePush(t *testing.T) {
	rg := newRig(t)
	repo := rg.get(t)
	content, sha, err := repo.ReadFile(context.Background(), "main", "values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "a: 1\n" {
		t.Fatalf("the file: %q", content)
	}
	landed, err := repo.Write(context.Background(), "main", "",
		map[string][]byte{"values.yaml": []byte("a: 2\n")}, "alice <alice@localhost>", "a is 2")
	if err != nil {
		t.Fatal(err)
	}
	if landed == sha {
		t.Fatal("the head did not move")
	}
	if got := rg.head(t, "main"); got != landed {
		t.Fatalf("the remote is at %s, the write landed %s", got, landed)
	}
	if got := rg.file(t, landed, "values.yaml"); got != "a: 2\n" {
		t.Fatalf("the remote file: %q", got)
	}
	// the author is the user, the committer pack-ui (К5)
	c := rg.commitOf(t, landed)
	if c.Author.Name != "alice" || c.Author.Email != "alice@localhost" {
		t.Errorf("the author: %v", c.Author)
	}
	if c.Committer.Name != "pack-ui" {
		t.Errorf("the committer: %v", c.Committer)
	}
}

func TestWriteRebase(t *testing.T) {
	rg := newRig(t)
	repo := rg.get(t) // fetches: the copy knows the remote's head
	// the other side moves a file we do not touch
	rg.commit(t, "main", map[string]string{"notes.txt": "their notes\n"}, "the notes")
	landed, err := repo.Write(context.Background(), "main", "",
		map[string][]byte{"values.yaml": []byte("a: 2\n")}, "alice <alice@localhost>", "a is 2")
	if err != nil {
		t.Fatal(err)
	}
	if got := rg.head(t, "main"); got != landed {
		t.Fatalf("the remote is at %s, the write landed %s", got, landed)
	}
	if got := rg.file(t, landed, "values.yaml"); got != "a: 2\n" {
		t.Errorf("our change: %q", got)
	}
	if got := rg.file(t, landed, "notes.txt"); got != "their notes\n" {
		t.Errorf("their change kept: %q", got)
	}
	// the rebased commit's parent is the other side's head — a straight line, no merge
	c := rg.commitOf(t, landed)
	if c.NumParents() != 1 {
		t.Fatalf("the rebased commit's parents: %d", c.NumParents())
	}
	parent, _ := c.Parents().Next()
	subject, _, _ := strings.Cut(parent.Message, "\n")
	if subject != "the notes" {
		t.Errorf("the parent: %q", subject)
	}
	if c.Author.Name != "alice" {
		t.Errorf("the author kept: %v", c.Author)
	}
}

func TestWriteConflict(t *testing.T) {
	rg := newRig(t)
	repo := rg.get(t)
	// the other side moves the very file we edit
	rg.commit(t, "main", map[string]string{"values.yaml": "a: 99\n"}, "a is 99")
	_, err := repo.Write(context.Background(), "main", "",
		map[string][]byte{"values.yaml": []byte("a: 2\n")}, "alice <alice@localhost>", "a is 2")
	var conflict *Conflict
	if !errors.As(err, &conflict) {
		t.Fatalf("a Conflict was expected, got %v", err)
	}
	if len(conflict.Files) != 1 || conflict.Files[0] != "values.yaml" {
		t.Errorf("the files: %v", conflict.Files)
	}
	if conflict.TheirSubject != "a is 99" || len(conflict.TheirSHA) != 40 {
		t.Errorf("their commit: %+v", conflict)
	}
	// the remote kept the other side's change — nothing of ours landed
	if got := rg.file(t, rg.head(t, "main"), "values.yaml"); got != "a: 99\n" {
		t.Errorf("the remote file: %q", got)
	}
}

func TestWriteStaleBase(t *testing.T) {
	rg := newRig(t)
	repo := rg.get(t)
	base := rg.head(t, "main")
	// the branch moved since the user read the file, and moved the very file
	rg.commit(t, "main", map[string]string{"values.yaml": "a: 99\n"}, "a is 99")
	if err := repo.refetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := repo.Write(context.Background(), "main", base,
		map[string][]byte{"values.yaml": []byte("a: 2\n")}, "alice <alice@localhost>", "a is 2")
	var conflict *Conflict
	if !errors.As(err, &conflict) || conflict.TheirSubject != "a is 99" {
		t.Fatalf("a Conflict on a is 99 was expected, got %v", err)
	}
	// a file the move did not touch may still be written on the same base
	landed, err := repo.Write(context.Background(), "main", base,
		map[string][]byte{"notes.txt": []byte("my notes\n")}, "alice <alice@localhost>", "the notes")
	if err != nil {
		t.Fatal(err)
	}
	if got := rg.file(t, landed, "values.yaml"); got != "a: 99\n" {
		t.Errorf("their change kept: %q", got)
	}
	if got := rg.file(t, landed, "notes.txt"); got != "my notes\n" {
		t.Errorf("our change: %q", got)
	}
}

func TestWriteNewBranch(t *testing.T) {
	rg := newRig(t)
	repo := rg.get(t)
	// a change request's branch the operator made at the main branch's head (openChange does)
	rg.branch(t, "cr-1")
	landed, err := repo.Write(context.Background(), "cr-1", "",
		map[string][]byte{"values.yaml": []byte("a: 2\n")}, "alice <alice@localhost>", "a is 2")
	if err != nil {
		t.Fatal(err)
	}
	if got := rg.head(t, "cr-1"); got != landed {
		t.Fatalf("the branch is at %s, the write landed %s", got, landed)
	}
	// the main branch is untouched
	if got := rg.file(t, rg.head(t, "main"), "values.yaml"); got != "a: 1\n" {
		t.Errorf("main: %q", got)
	}
}
