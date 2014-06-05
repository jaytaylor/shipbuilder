package main

import (
	"fmt"
	"net"
)

func (this *Server) PrivateKey_Set(conn net.Conn, applicationName string, privateKey string) error {
	return this.WithPersistentApplication(applicationName, func(app *Application, cfg *Config) error {
		app.SshPrivateKey = &privateKey
		return nil
	})
}

func (this *Server) PrivateKey_Get(conn net.Conn, applicationName string) error {
	return this.WithPersistentApplication(applicationName, func(app *Application, cfg *Config) error {
		titleLogger, dimLogger := this.getTitleAndDimLoggers(conn)
		fmt.Fprintf(titleLogger, "=== Getting private SSH key for %v\n", applicationName)
		fmt.Fprintf(dimLogger, "%v\n", *app.SshPrivateKey)
		return nil
	})
}

func (this *Server) PrivateKey_Remove(conn net.Conn, applicationName string) error {
	return this.WithPersistentApplication(applicationName, func(app *Application, cfg *Config) error {
		app.SshPrivateKey = nil
		return nil
	})
}
