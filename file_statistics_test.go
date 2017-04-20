package main

import "testing"
import "github.com/libgit2/git2go"

func TestFileChanges(t *testing.T) {
	repo, err := git.OpenRepository("./testdata/testrepo")
	handleErr(t, err)
	oid, err := git.NewOid("7471039d7ed95c5a80338694a9a5c9a03a382232")
	handleErr(t, err)
	commit, err := repo.LookupCommit(oid)
	handleErr(t, err)
	cs1, err := FileChanges(repo, commit, "main.c")
	handleErr(t, err)

	t.Log(cs1)

	cs2, err := FileChanges(repo, commit, "main.c")
	handleErr(t, err)

	t.Log(cs2)
}

func handleErr(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}
