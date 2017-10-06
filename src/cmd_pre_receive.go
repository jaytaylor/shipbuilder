package main

import (
	"net"
	"strings"
)

func (server *Server) PreReceive(conn net.Conn, dir, oldrev, newrev, ref string) error {
	// We only care about master
	if ref != "refs/heads/master" {
		return nil
	}
	return server.Deploy(conn, dir[strings.LastIndex(dir, "/")+1:], newrev)
}
