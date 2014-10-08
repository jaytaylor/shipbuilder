package main

import (
	"net"
)

func (this *Server) Zfs_Cleanup(conn net.Conn) error {
	err := this.performZfsMaintenance(NewLogger(NewMessageLogger(conn), "[zfsMaintenance] "))
	return err
}
