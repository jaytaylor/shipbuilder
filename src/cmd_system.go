package main

import (
	"net"
)

func (this *Server) System_CleanupZfs(conn net.Conn) error {
	err := this.sysPerformZfsMaintenance(NewLogger(NewMessageLogger(conn), "[zfsMaintenance] "))
	return err
}

func (this *Server) System_CleanupSnapshots(conn net.Conn) error {
	err := this.sysRemoveOrphanedReleaseSnapshots(NewLogger(NewMessageLogger(conn), "[orphanedSnapshots] "))
	return err
}
