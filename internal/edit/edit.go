// Package edit is the git client of a user's edits (C1, К5): the working copies of the projects'
// repositories, a read of a file at a branch, a commit of the user's changes — the author the user, the
// committer pack-ui — and a push: to the main branch (flow: direct) or to a change request's branch
// (flow: review). A push the remote refuses is rebased once; a rebase that meets the other side's change of
// the same file is a Conflict, the user's to resolve. Only the git protocol — no git binary: go-git (the
// operator's git package carries the credentials and the transport).
package edit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	git "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"

	opgit "github.com/horchestra/pack-operator/git"
)

// Conflict is a push that met the other side's change of the same files: the rebase pack-ui tried could not
// be made. The user rebases — the files and the other side's commit say where.
type Conflict struct {
	Files        []string `json:"files"`
	TheirSHA     string   `json:"their_sha"`
	TheirSubject string   `json:"their_subject"`
}

func (e *Conflict) Error() string {
	return fmt.Sprintf("conflict: %s changed %s too", e.TheirSHA[:7], strings.Join(e.Files, ", "))
}

// Repos is the working copies, one per project, each used under its lock (a working copy serializes its
// edits — a worktree is one).
type Repos struct {
	dir       string
	store     opgit.Store // nil — the repositories ask nothing
	committer string
	mu        sync.Mutex
	repos     map[string]*Repo
	locks     map[string]*sync.Mutex
}

// New is the working copies in dir, the credentials read from store, the commits signed by committer
// ("pack-ui <address>", К5).
func New(dir string, store opgit.Store, committer string) *Repos {
	return &Repos{dir: dir, store: store, committer: committer, repos: map[string]*Repo{}, locks: map[string]*sync.Mutex{}}
}

// Repo is one project's working copy.
type Repo struct {
	Dir       string
	transport *opgit.Repo // fetch and push with the credentials
	r         *git.Repository
	committer *object.Signature
	lock      *sync.Mutex
}

// Get is the project's working copy, cloned on first use, fetched on every.
func (rs *Repos) Get(ctx context.Context, id, url, credsRef string) (*Repo, error) {
	lock := rs.lockOf(id)
	lock.Lock()
	defer lock.Unlock()
	rs.mu.Lock()
	repo, ok := rs.repos[id]
	rs.mu.Unlock()
	if ok {
		if err := repo.refetch(ctx); err != nil {
			return nil, err
		}
		return repo, nil
	}
	var creds *opgit.Credentials
	if len(credsRef) > 0 {
		if rs.store == nil {
			return nil, fmt.Errorf("project %s: credentials %s, but no secrets store is configured", id, credsRef)
		}
		c, err := rs.store.Credentials(ctx, credsRef)
		if err != nil {
			return nil, fmt.Errorf("project %s: %w", id, err)
		}
		creds = c
	}
	dir := filepath.Join(rs.dir, id)
	var tr *opgit.Repo
	var err error
	if _, statErr := os.Stat(filepath.Join(dir, ".git")); statErr == nil {
		tr, err = opgit.Open(dir, creds)
	} else {
		if err := os.MkdirAll(rs.dir, 0o755); err != nil {
			return nil, err
		}
		tr, err = opgit.Clone(ctx, url, dir, creds)
	}
	if err != nil {
		return nil, fmt.Errorf("project %s: %w", id, err)
	}
	if err := tr.Fetch(ctx); err != nil {
		tr.Close()
		return nil, fmt.Errorf("project %s: %w", id, err)
	}
	repo = &Repo{Dir: dir, transport: tr, committer: signature(rs.committer), lock: rs.lockOf(id)}
	if err := repo.reopen(); err != nil {
		tr.Close()
		return nil, fmt.Errorf("project %s: %w", id, err)
	}
	rs.mu.Lock()
	rs.repos[id] = repo
	rs.mu.Unlock()
	return repo, nil
}

// reopen opens the working copy's own handle again: the fetch goes through the transport's handle, and this
// handle's storer caches the objects it knows — a reopened one sees what the fetch brought.
func (r *Repo) reopen() error {
	repo, err := git.PlainOpen(r.Dir)
	if err != nil {
		return err
	}
	r.r = repo
	return nil
}

// refetch is a fetch and a reopened handle.
func (r *Repo) refetch(ctx context.Context) error {
	if err := r.transport.Fetch(ctx); err != nil {
		return err
	}
	return r.reopen()
}

func (rs *Repos) lockOf(id string) *sync.Mutex {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	l, ok := rs.locks[id]
	if !ok {
		l = &sync.Mutex{}
		rs.locks[id] = l
	}
	return l
}

// Close gives every repo's temp files up.
func (rs *Repos) Close() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for _, r := range rs.repos {
		r.transport.Close()
	}
	rs.repos = map[string]*Repo{}
}

// signature parses "Name <address>".
func signature(who string) *object.Signature {
	name, email, ok := strings.Cut(who, " <")
	if !ok {
		return &object.Signature{Name: who, Email: who + "@localhost"}
	}
	return &object.Signature{Name: name, Email: strings.TrimSuffix(email, ">")}
}

// ReadFile is the file at the remote branch's head, and the head's sha.
func (r *Repo) ReadFile(ctx context.Context, branch, path string) ([]byte, string, error) {
	sha, err := r.transport.Resolve(ctx, "refs/remotes/origin/"+branch)
	if err != nil {
		return nil, "", fmt.Errorf("the branch %s: %w", branch, err)
	}
	c, err := r.r.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		return nil, "", err
	}
	tree, err := c.Tree()
	if err != nil {
		return nil, "", err
	}
	f, err := tree.File(path)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	content, err := f.Contents()
	if err != nil {
		return nil, "", err
	}
	return []byte(content), sha, nil
}

// LogPath is the commits of the remote branch that touched path (a release's directory), newest first,
// each its sha and subject, at most n.
func (r *Repo) LogPath(ctx context.Context, branch, path string, n int) ([][2]string, error) {
	sha, err := r.transport.Resolve(ctx, "refs/remotes/origin/"+branch)
	if err != nil {
		return nil, fmt.Errorf("the branch %s: %w", branch, err)
	}
	iter, err := r.r.Log(&git.LogOptions{From: plumbing.NewHash(sha), PathFilter: func(p string) bool {
		return p == path || strings.HasPrefix(p, strings.TrimSuffix(path, "/")+"/")
	}})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var commits [][2]string
	err = iter.ForEach(func(c *object.Commit) error {
		if n > 0 && len(commits) >= n {
			return io.EOF
		}
		subject, _, _ := strings.Cut(c.Message, "\n")
		commits = append(commits, [2]string{c.Hash.String(), strings.TrimRight(subject, "\r")})
		return nil
	})
	if err == io.EOF {
		err = nil
	}
	return commits, err
}

// Checkout is the tree of the remote branch's head written in dest — a copy for reading (the values work
// renders a pack out of it), with no .git of its own. The caller removes it.
func (r *Repo) Checkout(ctx context.Context, branch, dest string) (string, error) {
	sha, err := r.transport.Resolve(ctx, "refs/remotes/origin/"+branch)
	if err != nil {
		return "", fmt.Errorf("the branch %s: %w", branch, err)
	}
	if err := r.transport.Checkout(ctx, sha, dest); err != nil {
		return "", err
	}
	return sha, nil
}

// Write commits files (path → content) on branch as author (the user — "Name <address>"), the committer
// pack-ui (К5), and pushes. baseSHA is the head the user saw when they started editing ("" — not known):
// when the branch moved since, a file of the user's among the moved ones is a *Conflict with the other
// side's commit — the user rebases, nothing is silently overwritten. The commit goes onto the remote head
// the working copy knows; a push the remote refuses — it moved in the meantime — is fetched and rebased
// once: our commit onto the new head when the files each side changed are disjoint, a *Conflict when they
// are not. The answer is the commit that landed on the remote.
func (r *Repo) Write(ctx context.Context, branch, baseSHA string, files map[string][]byte, author, message string) (string, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	if len(baseSHA) > 0 {
		if err := r.stale(ctx, branch, baseSHA, files); err != nil {
			return "", err
		}
	}
	sha, err := r.commit(ctx, branch, files, author, message)
	if err != nil {
		return "", err
	}
	for attempt := 0; ; attempt++ {
		err := r.push(ctx, branch)
		if err == nil {
			r.landed(branch, sha)
			return sha, nil
		}
		if attempt > 0 || !nonFastForward(err) {
			return "", err
		}
		// the remote moved under the commit: fetch and rebase onto the new head
		if err := r.refetch(ctx); err != nil {
			return "", err
		}
		sha, err = r.rebase(ctx, branch, sha, author, message)
		if err != nil {
			return "", err
		}
	}
}

// stale is the base-sha check of Write: the branch's head the working copy knows against what the user saw.
func (r *Repo) stale(ctx context.Context, branch, baseSHA string, files map[string][]byte) error {
	remote, err := r.r.Reference(plumbing.NewRemoteReferenceName("origin", branch), true)
	if err == plumbing.ErrReferenceNotFound {
		return fmt.Errorf("the branch %s is not on the remote", branch)
	}
	if err != nil {
		return err
	}
	head := remote.Hash().String()
	if head == baseSHA {
		return nil
	}
	theirs, err := r.transport.ChangedFiles(ctx, baseSHA, head)
	if err != nil {
		return err
	}
	moved := map[string]bool{}
	for _, f := range theirs {
		moved[f] = true
	}
	var both []string
	for p := range files {
		if moved[p] {
			both = append(both, p)
		}
	}
	if len(both) == 0 {
		return nil
	}
	info, err := r.transport.Inspect(ctx, head)
	if err != nil {
		return err
	}
	subject, _, _ := strings.Cut(info.Message, "\n")
	return &Conflict{Files: both, TheirSHA: head, TheirSubject: subject}
}

// nonFastForward says whether a push error is the remote refusing a non-fast-forward.
func nonFastForward(err error) bool {
	return errors.Is(err, git.ErrNonFastForwardUpdate) || strings.Contains(err.Error(), "non-fast-forward")
}

// landed records the pushed sha as the remote branch's head.
func (r *Repo) landed(branch string, sha string) {
	ref := plumbing.NewHashReference(plumbing.NewRemoteReferenceName("origin", branch), plumbing.NewHash(sha))
	_ = r.r.Storer.SetReference(ref)
}

// commit is the user's commit on the local branch: the worktree at the remote branch's head, the files
// written, author the user, committer pack-ui.
func (r *Repo) commit(ctx context.Context, branch string, files map[string][]byte, author, message string) (string, error) {
	var head plumbing.Hash
	remote, err := r.r.Reference(plumbing.NewRemoteReferenceName("origin", branch), true)
	if err == plumbing.ErrReferenceNotFound {
		// a change request's branch opened since the last fetch: one more look
		if err := r.refetch(ctx); err != nil {
			return "", err
		}
		remote, err = r.r.Reference(plumbing.NewRemoteReferenceName("origin", branch), true)
	}
	if err == nil {
		head = remote.Hash()
	} else if err == plumbing.ErrReferenceNotFound {
		return "", fmt.Errorf("the branch %s is not on the remote", branch)
	} else {
		return "", err
	}
	w, err := r.r.Worktree()
	if err != nil {
		return "", err
	}
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branch), head)
	if err := r.r.Storer.SetReference(ref); err != nil {
		return "", err
	}
	if err := w.Checkout(&git.CheckoutOptions{Branch: ref.Name(), Force: true}); err != nil {
		return "", fmt.Errorf("checkout %s: %w", branch, err)
	}
	paths := make([]string, 0, len(files))
	for p, content := range files {
		if strings.Contains(p, "..") || strings.HasPrefix(p, "/") {
			return "", fmt.Errorf("the path stays in the repository: %q", p)
		}
		full := filepath.Join(r.Dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(full, content, 0o644); err != nil {
			return "", err
		}
		paths = append(paths, p)
	}
	for _, p := range paths {
		if _, err := w.Add(p); err != nil {
			return "", err
		}
	}
	who := signature(author)
	who.When = time.Now()
	cmt := *r.committer
	cmt.When = time.Now()
	h, err := w.Commit(message, &git.CommitOptions{Author: who, Committer: &cmt})
	if err != nil {
		return "", err
	}
	return h.String(), nil
}

// push pushes the local branch to the remote; a refusal of a non-fast-forward is for Write to rebase on.
func (r *Repo) push(ctx context.Context, branch string) error {
	refspec := plumbing.NewBranchReferenceName(branch).String() + ":" + plumbing.NewBranchReferenceName(branch).String()
	return r.transport.Push(ctx, refspec)
}

// rebase moves the commit onto the remote branch's head: the files each side changed must be disjoint,
// otherwise a *Conflict with the other side's commit. The rebased commit keeps the tree differences, the
// author and the message; the committer is pack-ui again.
func (r *Repo) rebase(ctx context.Context, branch, sha, author, message string) (string, error) {
	remote, err := r.r.Reference(plumbing.NewRemoteReferenceName("origin", branch), true)
	if err != nil {
		return "", err
	}
	head := remote.Hash().String()
	base, err := r.transport.MergeBase(ctx, head, sha)
	if err != nil {
		return "", err
	}
	mine, err := r.transport.ChangedFiles(ctx, base, sha)
	if err != nil {
		return "", err
	}
	theirs, err := r.transport.ChangedFiles(ctx, base, head)
	if err != nil {
		return "", err
	}
	touched := map[string]bool{}
	for _, f := range theirs {
		touched[f] = true
	}
	var both []string
	for _, f := range mine {
		if touched[f] {
			both = append(both, f)
		}
	}
	if len(both) > 0 {
		info, err := r.transport.Inspect(ctx, head)
		if err != nil {
			return "", err
		}
		subject, _, _ := strings.Cut(info.Message, "\n")
		return "", &Conflict{Files: both, TheirSHA: head, TheirSubject: subject}
	}
	ours, err := r.r.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		return "", err
	}
	onto, err := r.r.CommitObject(remote.Hash())
	if err != nil {
		return "", err
	}
	ourTree, err := ours.Tree()
	if err != nil {
		return "", err
	}
	ontoTree, err := onto.Tree()
	if err != nil {
		return "", err
	}
	tree, err := applyFiles(r.r.Storer, ontoTree, ourTree, mine)
	if err != nil {
		return "", err
	}
	who := signature(author)
	who.When = time.Now()
	cmt := *r.committer
	cmt.When = time.Now()
	commit := object.Commit{
		Author:       *who,
		Committer:    cmt,
		Message:      message,
		TreeHash:     tree,
		ParentHashes: []plumbing.Hash{remote.Hash()},
	}
	obj := r.r.Storer.NewEncodedObject()
	if err := commit.Encode(obj); err != nil {
		return "", err
	}
	h, err := r.r.Storer.SetEncodedObject(obj)
	if err != nil {
		return "", err
	}
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branch), h)
	if err := r.r.Storer.SetReference(ref); err != nil {
		return "", err
	}
	return h.String(), nil
}
