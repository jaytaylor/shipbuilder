package core

import (
	"net"

	log "github.com/sirupsen/logrus"
)

func (server *Server) PostReceive(conn net.Conn, dir, oldrev, newrev, ref string) error {
	log.WithField("dir", dir).WithField("oldrev", oldrev).WithField("newrev", newrev).WithField("ref", ref).Debug("PostReceive invoked")

	// We only care about master
	if ref != "refs/heads/master" {
		log.WithFields(log.Fields{"dir": dir, "oldrev": oldrev, "newrev": newrev, "ref": ref}).Warn("PostReceive with non-master branch ignored")
		return nil
	}
	// // NB: REPOSITORY CLEARING IS DISABLED
	// //e := Executor{NewLogger(os.Stdout, "[post-receive]")}
	// return nil //e.Run("sudo", "/bin/bash", "-c", "cd "+dir+" && git symbolic-ref HEAD refs/heads/tmp && git branch -D master")
	return nil
}
