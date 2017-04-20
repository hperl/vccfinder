package tools

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"os/exec"
	"regexp"
	"sort"
	"strconv"

	"github.com/libgit2/git2go"
)

type Tool struct {
	name    string
	command string
	args    []string
	regex   *regexp.Regexp
}

type Result struct {
	FileName string
	Line     uint
	Reason   string
	FoundBy  string
}

type ByLine []Result

func (r ByLine) Len() int           { return len(r) }
func (r ByLine) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r ByLine) Less(i, j int) bool { return r[i].Line < r[j].Line }

var Flawfinder, Rats *Tool

func init() {
	script, err := Asset("data/flawfinder.py")
	if err != nil {
		panic(err)
	}
	flawfinderScript, err := ioutil.TempFile("", "flawfinder")
	if err != nil {
		panic(err)
	}
	defer flawfinderScript.Close()
	_, err = flawfinderScript.Write(script)
	if err != nil {
		panic(err)
	}

	script, err = Asset("data/rats_wrapper.py")
	if err != nil {
		panic(err)
	}
	ratsWrapperScript, err := ioutil.TempFile("", "rats_wrapper")
	if err != nil {
		panic(err)
	}
	defer ratsWrapperScript.Close()
	_, err = ratsWrapperScript.Write(script)
	if err != nil {
		panic(err)
	}

	Flawfinder = &Tool{
		name:    "flawfinder",
		command: "/usr/bin/python",
		args:    []string{flawfinderScript.Name(), "-SQD", "-"},
		regex:   regexp.MustCompile(`^-:(\d+):  (.*)$`),
	}

	Rats = &Tool{
		name:    "rats",
		command: "/usr/bin/python",
		args:    []string{ratsWrapperScript.Name()},
		regex:   regexp.MustCompile(`.+:(\d+): (.*)$`),
	}
}

func (t *Tool) Analyze(repo *git.Repository, file *git.DiffFile) (res []Result, err error) {
	blob, err := repo.LookupBlob(file.Oid)
	if err != nil {
		return
	}
	fname := file.Path

	return t.run(blob.Contents(), fname)
}

func ResultsAtLine(results []Result, line uint) (r []Result) {
	var idx, end int
	idx = sort.Search(len(results), func(i int) bool {
		return results[i].Line >= line
	})
	for end = idx; end < len(results) && results[end].Line == line; end++ {
	}
	return results[idx:end]
}

func Merge(a, b []Result) []Result {
	rs := append(a, b...)
	sort.Sort(ByLine(rs))
	return rs
}

func (t *Tool) run(input []byte, fname string) (results []Result, err error) {
	cmd := exec.Command(t.command, t.args...)
	cmd.Stdin = bytes.NewReader(input)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(&out)
	for scanner.Scan() {
		line := scanner.Text()
		if m := t.regex.FindStringSubmatch(line); len(m) == 3 {
			line, err := strconv.Atoi(m[1])
			if err != nil {
				line = 0
			}
			result := Result{
				FoundBy:  t.name,
				FileName: fname,
				Line:     uint(line),
				Reason:   m[2],
			}
			results = append(results, result)
		}
	}
	sort.Sort(ByLine(results))
	return
}
