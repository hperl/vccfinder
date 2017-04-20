package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/libgit2/git2go"
)

type Blame struct {
	raw string
	dir BlameDirection
}

type ShortBlame struct {
	lines []string
}

type BlameLine struct {
	Sha             string
	Author          string
	AuthorMail      string
	AuthorTimestamp time.Time
	Committer       string
	CommitterMail   string
	PreviousCommit  string
	PreviousPath    string
	OriginalLineNum int
	FinalLineNum    int
}

type BlameLineType uint

const (
	BlameAddition BlameLineType = iota
	BlameDeletion BlameLineType = iota
)

type BlameDirection uint

const (
	BlameBackward BlameDirection = iota
	BlameForward  BlameDirection = iota
)

func NewShortBlame(repo *git.Repository, filepath string) (*ShortBlame, error) {
	blameCmd := exec.Command(
		"git",
		"blame",
		"-let",
		filepath,
	)
	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	blameCmd.Stdout = buf
	blameCmd.Stderr = errBuf
	blameCmd.Dir = repo.Workdir()
	if err := blameCmd.Run(); err != nil {
		log.Print("stderr: ", errBuf)
		return nil, fmt.Errorf("%v failed: %v", blameCmd, err)
	}
	scanner := bufio.NewScanner(buf)
	blame := new(ShortBlame)
	for scanner.Scan() {
		blame.lines = append(blame.lines, scanner.Text())
	}
	return blame, nil
}

func NewBlame(repo *git.Repository, startSha string, filepath string, dir BlameDirection) (*Blame, error) {
	var blameCmd *exec.Cmd
	if dir == BlameForward {
		blameCmd = exec.Command(
			"git",
			"blame",
			//fmt.Sprintf("-L %d,%d", minLine, maxLine),
			"--line-porcelain",
			"--reverse",
			startSha+"..HEAD",
			"--",
			filepath,
		)
	} else {
		blameCmd = exec.Command(
			"git",
			"blame",
			//fmt.Sprintf("-L %d,%d", minLine, maxLine),
			"--line-porcelain",
			startSha,
			"--",
			filepath,
		)
	}
	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	blameCmd.Stdout = buf
	blameCmd.Stderr = errBuf
	blameCmd.Dir = repo.Workdir()
	if err := blameCmd.Run(); err != nil {
		log.Print("stderr: ", errBuf)
		return nil, fmt.Errorf("%v failed: %v", blameCmd, err)
	}
	return &Blame{raw: buf.String()}, nil
}

func (blame *Blame) ForLine(lineNum int) (bl *BlameLine, err error) {
	var (
		matches []string
	)
	re := regexp.MustCompile(fmt.Sprintf(`([[:xdigit:]]{40}) (\d+) %d ?\d*
author ([^\n]*)
author-mail ([^\n]*)
author-time ([[:print:]]*)
author-tz [[:print:]]*
committer ([^\n]*)
committer-mail ([^\n]*)
committer-time [[:print:]]*
committer-tz [[:print:]]*
summary [[:print:]]*
(?:previous ([[:xdigit:]]{40}) ([^\n]*))?(?:[^\t]*?)
\t[[:print:]]*`, lineNum))
	if matches = re.FindStringSubmatch(blame.raw); matches == nil {
		err = fmt.Errorf("line %v not found in blame", lineNum)
	} else {
		origLN, err := strconv.Atoi(matches[2])
		if err != nil {
			return nil, err
		}
		unixTS, err := strconv.ParseInt(matches[5], 0, 64)
		if err != nil {
			return nil, err
		}

		bl = &BlameLine{
			Sha:             matches[1],
			Author:          matches[3],
			AuthorMail:      matches[4],
			AuthorTimestamp: time.Unix(unixTS, 0),
			Committer:       matches[6],
			CommitterMail:   matches[7],
			PreviousCommit:  matches[8],
			PreviousPath:    matches[9],
			FinalLineNum:    lineNum,
			OriginalLineNum: origLN,
		}
	}
	return
}

func (blame *ShortBlame) newestLine(startLine, endLine uint) (bl *BlameLine, err error) {
	var ts int
	bl = new(BlameLine)

	re := regexp.MustCompile(`\^?([[:xdigit:]]+) \(<[[:print:]]+>\W+(\d+)`)
	for l := startLine; l <= endLine; l++ {
		if int(l) >= len(blame.lines) {
			break
		}
		if m := re.FindStringSubmatch(blame.lines[l]); m == nil {
			return nil, fmt.Errorf("no match found in %d %s", l, blame.lines[l])
		} else {
			lineTS, _ := strconv.Atoi(m[2])
			if ts < lineTS {
				ts = lineTS
				bl.Sha = m[1]
			}
		}
	}
	bl.AuthorTimestamp = time.Unix(int64(ts), 0)

	return
}
