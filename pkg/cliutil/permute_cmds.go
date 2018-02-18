package cliutil

import (
	"fmt"
)

// PermuteCmds takes a set of prefixes and suffixes and generates all
// combinations.
func PermuteCmds(prefixes []string, suffixes []string, suffixOptional bool, funcName string) []string {
	out := []string{}

	if suffixOptional {
		out = append(out, prefixes...)
	}

	for _, suffix := range suffixes {
		for _, prefix := range prefixes {
			out = append(out, fmt.Sprintf("%v:%v", prefix, suffix))
		}
	}

	if len(funcName) > 0 {
		out = append(out, funcName)
	}

	return out
}
