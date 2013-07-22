package main

import (
	"net"
	"os"
)

func (this *Server) PostReceive(conn net.Conn, dir, oldrev, newrev, ref string) error {
	// We only care about master
	if ref != "refs/heads/master" {
		return nil
	}
	e := Executor{NewLogger(os.Stdout, "[post-receive]")}
	return e.Run("sudo", "/bin/bash", "-c", "cd "+dir+" && git symbolic-ref HEAD refs/heads/tmp && git branch -D master")
}
