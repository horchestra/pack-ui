package edit

import (
	"fmt"
	"sort"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage"
)

// The tree plumbing of a rebase: a commit of ours moved onto the remote's head keeps the head's tree with the
// files we touched replaced by ours (insert) or struck out (remove). A tree is rewritten pure-plumbing, no
// worktree — the same way the operator's CommitFile works.

// writeBlob stores content as a blob.
func writeBlob(st storage.Storer, content []byte) (plumbing.Hash, error) {
	obj := st.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	obj.SetSize(int64(len(content)))
	w, err := obj.Writer()
	if err != nil {
		return plumbing.Hash{}, err
	}
	if _, err := w.Write(content); err != nil {
		w.Close()
		return plumbing.Hash{}, err
	}
	if err := w.Close(); err != nil {
		return plumbing.Hash{}, err
	}
	return st.SetEncodedObject(obj)
}

// treeEntry is the blob (or its absence) a path ends up with.
func treeEntry(st storage.Storer, tree *object.Tree, path string) (plumbing.Hash, bool, error) {
	f, err := tree.File(path)
	if err == object.ErrFileNotFound || err == object.ErrDirectoryNotFound {
		return plumbing.Hash{}, false, nil
	}
	if err != nil {
		return plumbing.Hash{}, false, err
	}
	return f.Hash, true, nil
}

// setBlob is the tree's hash with the blob at path (split on /) set — or, content nil, the entry removed.
// The subtrees on the way are made or rebuilt; git has no directories of its own, a path is a chain of trees.
func setBlob(st storage.Storer, tree *object.Tree, parts []string, blob plumbing.Hash, present bool) (plumbing.Hash, error) {
	entries := make([]object.TreeEntry, 0, len(tree.Entries)+1)
	if len(parts) == 1 {
		for _, e := range tree.Entries {
			if e.Name == parts[0] {
				continue
			}
			entries = append(entries, e)
		}
		if present {
			entries = append(entries, object.TreeEntry{Name: parts[0], Mode: filemode.Regular, Hash: blob})
		}
	} else {
		sub := &object.Tree{}
		for _, e := range tree.Entries {
			if e.Name == parts[0] {
				if e.Mode != filemode.Dir {
					return plumbing.Hash{}, fmt.Errorf("%s is a file, not a directory", parts[0])
				}
				t, err := object.GetTree(st, e.Hash)
				if err != nil {
					return plumbing.Hash{}, err
				}
				sub = t
				continue
			}
			entries = append(entries, e)
		}
		subHash, err := setBlob(st, sub, parts[1:], blob, present)
		if err != nil {
			return plumbing.Hash{}, err
		}
		entries = append(entries, object.TreeEntry{Name: parts[0], Mode: filemode.Dir, Hash: subHash})
	}
	// git orders a tree's entries by name, a directory as its name with a trailing /
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i].Name, entries[j].Name
		if entries[i].Mode == filemode.Dir {
			a += "/"
		}
		if entries[j].Mode == filemode.Dir {
			b += "/"
		}
		return a < b
	})
	out := object.Tree{Entries: entries}
	obj := st.NewEncodedObject()
	if err := out.Encode(obj); err != nil {
		return plumbing.Hash{}, err
	}
	return st.SetEncodedObject(obj)
}

// applyFiles is the tree of onto with the files of from at paths applied: a file present in from replaces,
// one absent there is removed. The rebase of a commit whose files the other branch did not touch is exactly
// this — the trees differ only at paths.
func applyFiles(st storage.Storer, onto, from *object.Tree, paths []string) (plumbing.Hash, error) {
	h := onto.Hash
	current := onto
	for _, p := range paths {
		blob, present, err := treeEntry(st, from, p)
		if err != nil {
			return plumbing.Hash{}, err
		}
		h, err = setBlob(st, current, strings.Split(p, "/"), blob, present)
		if err != nil {
			return plumbing.Hash{}, err
		}
		current, err = object.GetTree(st, h)
		if err != nil {
			return plumbing.Hash{}, err
		}
	}
	return h, nil
}
