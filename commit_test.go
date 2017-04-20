package main

import (
	"fmt"
	"log"
	"testing"

	git "github.com/libgit2/git2go"
)

type testDatum struct {
	sha                    string
	repo                   string
	additions              int64
	deletions              int64
	hunkcount              int64
	futureChanges          int64
	pastChanges            int64
	futureDifferentAuthors int64
	pastDifferentAuthors   int64
}

/*
func TestMain(m *testing.M) {
	KnownCVEs = NewMitreCves()
	if err := KnownCVEs.Read("data/cve.xml"); err != nil {
		log.Fatal(err)
	}
	os.Exit(m.Run())
}
*/

func TestGetGitMetadata(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	var testData = []testDatum{
		testDatum{"63ab52b7ecdbcaf097862da79efa72e678920725",
			"FFmpeg/FFmpeg", 11, 13, 2, 485, 594, 82, 23},
		testDatum{"0a59a18b4ed63a1d20ac62489e72fb81d4b3c363",
			"FFmpeg/FFmpeg", 166, 134, 14, 1502, 622, 205, 55},
		testDatum{"24dee483e9e925c2ab79dd582f70c9a55ab9ba4d",
			"bagder/curl", 707, 1748, 64, 7471, 710, 609, 63},
	}

	RepoBasePath = "repos/"

	for _, data := range testData {
		r := &Repository{Name: data.repo}
		if err := r.clone(); err != nil {
			t.Errorf("cloning %s: %v", data.repo, err)
		}
		c := &Commit{
			Repository: r,
			Sha:        data.sha,
		}
		if err := c.GetGitMetadata(); err != nil {
			t.Fatalf("GetGitMetadata(): %v", err)
		}
		if c.Additions != data.additions {
			t.Error(c, "should have", data.additions, "additions, has", c.Additions)
		}
		if c.Deletions != data.deletions {
			t.Error(c, "should have", data.deletions, "deletions, has", c.Deletions)
		}
		if c.HunkCount != data.hunkcount {
			t.Error(c, "should have", data.hunkcount, "hunks, has", c.HunkCount)
		}
		if c.FutureChanges != data.futureChanges {
			t.Error(c, "should have", data.futureChanges, "future changes, has", c.FutureChanges)
		}
		if c.PastChanges != data.pastChanges {
			t.Error(c, "should have", data.pastChanges, "past changes, has", c.PastChanges)
		}
		if c.FutureDifferentAuthors != data.futureDifferentAuthors {
			t.Error(c, "should have", data.futureDifferentAuthors, "future different authors, has", c.FutureDifferentAuthors)
		}
		if c.PastDifferentAuthors != data.pastDifferentAuthors {
			t.Error(c, "should have", data.pastDifferentAuthors, "past different authors, has", c.PastDifferentAuthors)
		}
	}
}

func TestFixCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	var testData = []testDatum{
		testDatum{"330d57fb98a916fa8e1363846540dd420e99499a", "torvalds/linux", 0, 0, 0, 0, 0, 0, 0},
	}

	RepoBasePath = "../repos/"

	KnownCVEs = NewMitreCves()
	if err := KnownCVEs.Read("data/cve.xml"); err != nil {
		log.Fatal(err)
	}

	for _, data := range testData {
		r := &Repository{Name: data.repo}
		if err := r.clone(); err != nil {
			t.Errorf("cloning %s: %v", data.repo, err)
		}
		c := &Commit{
			Repository: r,
			Sha:        data.sha,
		}
		if err := c.GetGitMetadata(); err != nil {
			t.Error("GetGitMetadata(): %v", err)
		}
		if err := c.fixCommit(); err != nil {
			t.Error("fixCommit(): %v", err)
		}
		if c.Type != "fixing_commit" {
			t.Errorf("Not marked as fixing commit: %s", c.Message)
		}
	}
}

func TestBlameCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	var testData = []testDatum{
		testDatum{"ae8bb84ecd46d7b6ef557a87725923ac8d09dce0", "libguestfs/libguestfs", 0, 0, 0, 0, 0, 0, 0},
		testDatum{"330d57fb98a916fa8e1363846540dd420e99499a", "torvalds/linux", 0, 0, 0, 0, 0, 0, 0},
	}

	RepoBasePath = "../repos/"

	for _, data := range testData {
		r := &Repository{Name: data.repo}
		if err := r.clone(); err != nil {
			t.Errorf("cloning %s: %v", data.repo, err)
		}
		c := &Commit{
			Repository: r,
			Sha:        data.sha,
		}
		blamedSha, err := c.getBlameCommitSha()
		if err != nil {
			t.Error("blameCommit(): %v", err)
		}
		if blamedSha == "" {
			t.Errorf("No blame for commit: %#v", c)
		}
	}
}

func TestCommitGetFunctions(t *testing.T) {
	var testData = map[string]([]string){
		"7471039d7ed95c5a80338694a9a5c9a03a382232": []string{"modified"},
		"aba1ee55b69a812e74413638bb4c5cc05145633a": []string{"added"},
		"f2291572e5a3bf50103deb6335f6e1554a905f17": []string{"added", "deleted"},
	}
	RepoBasePath = "./testdata/"

	r := &Repository{Name: "testrepo"}
	for sha, funStates := range testData {
		c := &Commit{Repository: r, Sha: sha}
		if err := c.GetGitMetadata(); err != nil {
			t.Error(err)
		}
		if len(c.Functions) != len(funStates) {
			for _, f := range c.Functions {
				t.Logf("%s", f.String())
			}
			t.Fatalf("%s: expected %d functions, got %d", c, len(funStates), len(c.Functions))
		}
		for i, state := range funStates {
			f := c.Functions[i]
			if f.State != state {
				t.Fatalf("%s: expected %s, got %s", f.String(), state, f.State)
			}
		}
		for _, f := range c.Functions {
			t.Logf("%s", f.String())
		}
	}
}

func TestDiffRename(t *testing.T) {
	RepoBasePath = "./repos"

	emptyEachHunkCB := func(hunk git.DiffHunk) (git.DiffForEachLineCallback, error) {
		return func(line git.DiffLine) error {
			return nil
		}, nil
	}

	r := &Repository{Name: "tieto/pidgin"}
	c := &Commit{Repository: r, Sha: "fa502d1211999c1af956490c667f82846c5524af"}
	d, _, err := c.diff()
	checkErr(t, err)
	//opts := git.DefaultDiffFindOptions()
	//opts.Flags = git.DiffFindRenames
	//d.FindSimilar(opts)
	d.ForEach(func(delta git.DiffDelta, num float64) (git.DiffForEachHunkCallback, error) {
		fmt.Println(delta.OldFile.Path)
		return emptyEachHunkCB, nil
	}, git.DiffDetailLines)
	del, _ := d.NumDeltas()
	t.Log("deltas", del)
}

func TestSetPatchKeywords(t *testing.T) {
	c := &Commit{
		Patch: `if then if if catch else foo ?!? ;then`,
	}
	c.SetPatchKeywords()
	if v, ok := c.PatchKeywords.Map["if"]; !ok || v.String != "3" {
		t.Error("expected 3 ifs")
	}
	if v, ok := c.PatchKeywords.Map["auto"]; !ok || v.String != "0" {
		t.Error("expected 0 autos")
	}
	if v, ok := c.PatchKeywords.Map["then"]; !ok || v.String != "2" {
		t.Error("expected 2 then")
	}
	t.Log(c.PatchKeywords)
}

func checkErr(t *testing.T, err error) {
	if err != nil {
		t.Error(err)
	}
}
