package main

import "testing"
import "github.com/libgit2/git2go"

/*
func TestFunctionsForFile(t *testing.T) {
	repo, err := git.OpenRepository(path.Join("repos", "torvalds", "linux"))
	if err != nil {
		panic(err)
	}
	oid, err := git.NewOid("2127a7b9dc3a6ad2bb214536bf23e83d5005441b")
	if err != nil {
		panic(err)
	}
	blob, err := repo.LookupBlob(oid)
	if err != nil {
		panic(err)
	}

	fn := "repos/torvalds/linux/fs/file.c"
	fs := FunctionsForFile(fn, map[string]string{
		fn: string(blob.Contents()),
	})

	//oid := git.Oid("ab2af1f50")
	t.Logf(fs.String())
}
*/

func TestFunctionsForFileString(t *testing.T) {
	fn := "testdata/foo.c"
	content := `
#include "bar.h"

static void foo() {
	missing();
	int a = 0;
}

void bar() {
	int b = 0;
}

baz() {
	int c = 42;
}

	`

	fs, err := functionsForFilename(fn, map[string]string{
		fn: content,
	})
	if err != nil {
		t.Error(err)
	}
	t.Log(fs.String())
}

func TestFunctionsForFileOpensslSServer(t *testing.T) {
	fn := "/home/perl/Dokumente/code/openssl/apps/s_server.c"
	fs, err := functionsForFilename(fn, nil)
	if err != nil {
		t.Error(err)
	}
	t.Log(fs.String())
}

func TestEmptyFunctionIterator(t *testing.T) {
	fs := NewFunctions()
	fs.Add(&Function{
		Name:      "foo",
		StartLine: 10,
		EndLine:   20,
		State:     "",
	})
	iter := fs.NewEmptyFunctionIterator()
	f, ok := iter(10)
	if !ok {
		t.Error("iter failed")
	}
	f.State = "modified"
	funs := fs.Data()
	if funs["foo"].State != "modified" {
		t.Error("not the same function")
	}
}

func TestNewFunctions(t *testing.T) {
	fs := NewFunctions()
	fs.Add(&Function{})
	fs.Add(&Function{})
	t.Logf("%+v", fs.data)
	t.Logf("%+v", fs.functions)
}

func TestGetFunctionsForBlobOid(t *testing.T) {
	RepoBasePath = "./testdata/"

	r := &Repository{Name: "testrepo"}
	repo, err := r.GitRepository()
	if err != nil {
		t.Error(err)
	}
	oid, err := git.NewOid("a4f6af262f1f527715eb91891de1a4fd2a11089b")
	if err != nil {
		t.Error(err)
	}
	file := &git.DiffFile{
		Path: "main.c",
		Oid:  oid,
	}
	funs, err := FunctionsForFile(repo, file)
	if err != nil {
		t.Error(err)
	}
	if _, found := funs.Data()["main"]; !found {
		t.Fatalf("Expected 'main' to be found in %v", funs)
	}
}

/* Test for
   - Functions that use types not defined
   - Use of undefined makros
*/
func TestFunctionsForIncompleteFile(t *testing.T) {
	fn := "some/file.c"
	content := `
#define custom static

int func_with_invalid_syntax(???) {
	this does not even make sense !!!
}

custom my_return_type funktion(type_t *t) {
	my_array_t *a = new_my_array();
	void *v;
	ITER_MAKRO(a, v) DO
		puts(v);
	END
}

#ifdef __MS_WINDOZE
static void windoze_only() {
	missing_t *wtf = malloc(sizeof(missing_t));
	baz omfg = oh(wtf);
	return 42;
}
#endif

clang ignores stuff at the end
`

	fs, err := functionsForFilename(fn, map[string]string{
		fn: content,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs.Functions()) != 2 {
		t.Error("Should have length 2, had length", len(fs.Functions()))
	}
	for _, name := range []string{"funktion", "func_with_invalid_syntax"} {
		if _, ok := fs.Data()[name]; !ok {
			t.Errorf("Should have found `%s'.", name)
		}
	}
	t.Log(fs.String())
}
