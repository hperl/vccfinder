package tools

import (
	"testing"

	"github.com/libgit2/git2go"
)

func TestAnalyzeFlawfinder(t *testing.T) {
	repo, err := git.OpenRepository("../testdata/testrepo")
	handleErr(t, err)
	oid, err := git.NewOid("ff33c940b3dd52f1203029dc09988aba0c93280a")
	handleErr(t, err)
	file := &git.DiffFile{Path: "malloc.c.h", Oid: oid}
	handleErr(t, err)
	results, err := Flawfinder.Analyze(repo, file)
	handleErr(t, err)

	if len(results) != 15 {
		t.Error("Expected 15 results, got", len(results))
	}

	res := ResultsAtLine(results, 2256)
	if len(res) == 0 {
		t.Error("Expected result at line 2256")
	}

	doubleResults := Merge(results, results)
	res = ResultsAtLine(doubleResults, 2256)
	if len(res) != 2 {
		t.Error("Expected double results at line 2256")
	}

	t.Log(results)
}

func TestAnalyzeRats(t *testing.T) {
	repo, err := git.OpenRepository("../testdata/testrepo")
	handleErr(t, err)
	oid, err := git.NewOid("ff33c940b3dd52f1203029dc09988aba0c93280a")
	handleErr(t, err)
	file := &git.DiffFile{Path: "malloc.c.h", Oid: oid}
	handleErr(t, err)
	results, err := Rats.Analyze(repo, file)
	handleErr(t, err)

	if len(results) != 6 {
		t.Error("Expected 6 results, got", len(results))
	}

	res := ResultsAtLine(results, 3372)
	if len(res) == 0 {
		t.Error("Expected result at line 3372")
	}

	t.Log(results)
}

func handleErr(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}
