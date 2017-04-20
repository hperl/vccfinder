package main

import "unicode/utf8"

func fixInvalidUtf8(s string) string {
	if utf8.ValidString(s) {
		return s
	} else {
		v := make([]rune, 0, len(s))
		for i, r := range s {
			if r == utf8.RuneError {
				_, size := utf8.DecodeRuneInString(s[i:])
				if size == 1 {
					continue
				}
			}
			v = append(v, r)
		}
		return string(v)
	}
}
