package main

import (
	"path"
	"regexp"
)

var isCodeFileRe = regexp.MustCompile(`^.*\.(fort|c|c\+\+|cpp|h|hpp|h|py|sh|pl|cs\+\+|hh)$`)

// Determines whether the file contains code based on a simple regular expression.
func IsCodeFile(file string) bool {
	if file == "" {
		return false
	}
	return isCodeFileRe.MatchString(path.Base(file))
}
