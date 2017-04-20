package main

// #cgo LDFLAGS: -lclang
// #cgo CFLAGS: -std=c11
// #include <stdlib.h>
// #include <errno.h>
// #include "function.h"
import "C"

import (
	"fmt"
	"path"
	"strings"
	"unsafe"

	"github.com/libgit2/git2go"
	"github.com/sbinet/go-clang"
)

var DisableFunctionAnalysis = false

type Function struct {
	Id        int64  `db:"id" table:"functions"`
	CommitId  int64  `db:"commit_id"`
	Name      string `db:"name"`
	FileName  string `db:"file_name"`
	StartLine uint   `db:"start_line"`
	EndLine   uint   `db:"end_line"`
	State     string `db:"state"` // can be "added", "modified", "deleted"
}

type Functions struct {
	data      map[string](*Function)
	functions [](*Function)
}

type EmtyFunctionIterator func(line int) (f *Function, ok bool)

func NewFunctions() *Functions {
	fs := &Functions{
		data:      make(map[string]*Function),
		functions: make([](*Function), 0, 10),
	}
	return fs
}

func NewFunctionsFromC(fa *C.functions_array) *Functions {
	fs := NewFunctions()
	faLen := int(fa.len)
	for i := 0; i < faLen; i++ {
		f := C.fa_at(fa, C.size_t(i))
		if f.start_line+1 >= f.end_line { // ignore declarations
			continue
		}
		fs.Add(&Function{
			Name:      C.GoString(f.name),
			StartLine: uint(f.start_line),
			EndLine:   uint(f.end_line),
		})
	}

	return fs
}

func (fs *Functions) NewEmptyFunctionIterator() EmtyFunctionIterator {
	var (
		i = 0
	)
	return func(line int) (f *Function, ok bool) {
		for ; i < len(fs.functions); i++ {
			f := fs.functions[i]
			if f.State == "" && line <= int(f.EndLine) {
				return f, true
			}
		}
		return nil, false
	}
}

func (fs *Functions) Functions() (fa []*Function) {
	return fs.functions
}

func (fs *Functions) Add(f *Function) {
	fs.data[f.Name] = f
	fs.functions = append(fs.functions, f)
}

func (fs *Functions) Data() map[string]*Function {
	return fs.data
}

func AddedAndDeletedFunctions(newFs, oldFs *Functions) []*Function {
	fs := make([]*Function, 0, len(newFs.functions))
	for name, f := range newFs.data {
		if _, found := oldFs.data[name]; !found {
			f.State = "added"
			fs = append(fs, f)
		}
	}
	for name, f := range oldFs.data {
		if _, found := newFs.data[name]; !found {
			f.State = "deleted"
			fs = append(fs, f)
		}
	}
	return fs
}

func (fs *Functions) String() string {
	var ss []string

	for _, f := range fs.functions {
		ss = append(ss, f.String())
	}
	return "\n" + strings.Join(ss, "\n")
}

func (f *Function) GetId() int64 {
	return f.Id
}

func (f *Function) String() string {
	return fmt.Sprintf("%s (%s): %s:%d:%d", f.Name, f.State, f.FileName, f.StartLine, f.EndLine)
}

func (f *Function) ContainsLine(line int) bool {
	return int(f.StartLine) <= line && line <= int(f.EndLine)
}

func functionsForFilename(fname string, unsaved clang.UnsavedFiles) (functions *Functions, err error) {
	cfname := C.CString(fname)
	defer C.free(unsafe.Pointer(cfname))

	contents := C.CString(unsaved[fname])
	defer C.free(unsafe.Pointer(contents))

	fa, err := C.get_functions(cfname, contents, C.ulong(len(unsaved[fname])))
	if err != nil {
		return nil, err
	}
	defer C.fa_free(fa)

	functions = NewFunctionsFromC(fa)
	return
}

func FunctionsForFile(repo *git.Repository, file *git.DiffFile) (functions *Functions, err error) {
	if DisableFunctionAnalysis {
		return NewFunctions(), nil
	}

	blob, err := repo.LookupBlob(file.Oid)
	if err != nil {
		return
	}
	fname := path.Join(repo.Workdir(), file.Path)
	functions, err = functionsForFilename(
		fname,
		map[string]string{fname: string(blob.Contents())},
	)
	if err != nil {
		return nil, err
	}
	for _, f := range functions.functions {
		f.FileName = file.Path
	}
	return
}
