package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"strings"

	lru "github.com/hashicorp/golang-lru"
	"github.com/juju/utils/set"
	"github.com/libgit2/git2go"
)

type ChangeStatistic struct {
	PastChanges   int64
	FutureChanges int64
	PastAuthors   int64
	FutureAuthors int64
}

func (cs *ChangeStatistic) Add(other *ChangeStatistic) {
	cs.PastChanges += other.PastChanges
	cs.PastAuthors += other.PastAuthors
	cs.FutureChanges += other.FutureChanges
	cs.FutureAuthors += other.FutureAuthors
}

var cache *lru.Cache

func init() {
	cache, _ = lru.New(2000)
}

//var cache = struct {
//sync.RWMutex
//m map[string]bytes.Buffer
//}{m: make(map[string]bytes.Buffer)}

func FileChanges(repo *git.Repository, commit *git.Commit, filepath string) (cs *ChangeStatistic, err error) {
	var buf bytes.Buffer
	PastAuthors := new(set.Strings)
	FutureAuthors := new(set.Strings)
	future := true
	sha := commit.Id().String()
	cs = new(ChangeStatistic)
	key := repo.Path() + "|" + filepath
	val, ok := cache.Get(key)
	if ok {
		buf = val.(bytes.Buffer)
	} else {
		logCmd := exec.Command(
			"git",
			"log",
			"--follow",
			`--format="%H$$$%an$$$%aE$$$%cn$$$%cE"`,
			"--",
			filepath,
		)
		b := new(bytes.Buffer)
		errBuf := new(bytes.Buffer)
		logCmd.Stdout = b
		logCmd.Stderr = errBuf
		logCmd.Dir = repo.Workdir()
		if err := logCmd.Run(); err != nil {
			log.Print("stderr: ", errBuf)
			return nil, fmt.Errorf("%v failed: %v", logCmd, err)
		}
		cache.Add(key, *b)
		buf = *b
	}
	scanner := bufio.NewScanner(&buf)
	for scanner.Scan() {
		line := strings.SplitN(strings.Trim(scanner.Text(), `"`), "$$$", 5)
		if len(line) != 5 {
			panic("Could not parse line")
		}
		// check if we have seen the commit
		if future && line[0] == sha {
			future = false
		}
		if future {
			cs.FutureChanges++
			FutureAuthors.Add(line[1])
		} else {
			cs.PastChanges++
			PastAuthors.Add(line[3])
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	cs.FutureAuthors = int64(FutureAuthors.Size())
	cs.PastAuthors = int64(PastAuthors.Size())

	return
}

func LinesInBlob(blob *git.Blob) (lines int, err error) {
	buf := bytes.NewBuffer(blob.Contents())
	scanner := bufio.NewScanner(buf)
	for scanner.Scan() {
		scanner.Text()
		lines += 1
	}
	return lines, scanner.Err()
}
