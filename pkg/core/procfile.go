package core

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/scanner"

	log "github.com/sirupsen/logrus"
)

//type ProcEntry struct {
//	Name string
//	Cmd string
//}

// Profile selects and parses the latest Procfile from the git repository of
// the named app.
//
// Returns map[NAME] => "COMMAND (+args..)".
func Procfile(appName string) (map[string]string, error) {
	cmd := exec.Command(
		"git", "--no-pager", "--git-dir",
		fmt.Sprintf("%[1]s%[2]s%[3]s show master:Procfile", GIT_DIRECTORY, string(os.PathSeparator), appName),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("problem getting Procfile from git: %s (out=%v)", err, string(out))
	}
	var (
		s scanner.Scanner
		m = map[string]string{}
	)
	s.Init(bytes.NewReader(out))
	s.Filename = "Procfile"
	for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
		pieces := strings.SplitN(string(tok), ":", 2)
		if len(pieces) != 2 {
			log.Debugf("Malformed Procfile entry detected for app=%v: %v", appName, tok)
			continue
		}
		// n.b. m[NAME] => CMD.
		m[strings.TrimSpace(pieces[0])] = strings.TrimSpace(pieces[1])
	}
	return m, nil
}
