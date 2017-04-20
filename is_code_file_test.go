package main

import "testing"

func TestIsCodeFile(t *testing.T) {
	codePaths := []string{
		"a/code.c",
		"a/code.c++",
		"apache2/apache2_config.c",
		"apache2/msc_remote_rules.h",
		"a/b/c/d/e.fort",
	}
	docPaths := []string{
		"README",
		"",
		"/foo.txt",
		"/foo.textile",
		"/foo.rdoc",
		"/foo.md",
		"doc/src/sgml/perform.sgml",
		"/foo.markdown",
	}
	for _, f := range codePaths {
		if !IsCodeFile(f) {
			t.Errorf("%s should be a code file", f)
		}
	}
	for _, f := range docPaths {
		if IsCodeFile(f) {
			t.Errorf("%s should not be a code file", f)
		}
	}
}
