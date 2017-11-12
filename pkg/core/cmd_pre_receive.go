package core

import (
	"net"
	"strings"

	log "github.com/sirupsen/logrus"
)

func (server *Server) PreReceive(conn net.Conn, dir, oldrev, newrev, ref string) error {
	log.WithField("dir", dir).WithField("oldrev", oldrev).WithField("newrev", newrev).WithField("ref", ref).Debug("PreReceive invoked")

	// We only care about master.
	if ref != "refs/heads/master" {
		log.WithFields(log.Fields{"dir": dir, "oldrev": oldrev, "newrev": newrev, "ref": ref}).Warn("PreReceive with non-master branch ignored")
		return nil
	}

	if err := server.Deploy(conn, dir[strings.LastIndex(dir, "/")+1:], newrev); err != nil {
		log.WithFields(log.Fields{"dir": dir, "oldrev": oldrev, "newrev": newrev, "ref": ref}).Errorf("Problem deploying from PreReceive: %s", err)
		return err
	}
	return nil
}
