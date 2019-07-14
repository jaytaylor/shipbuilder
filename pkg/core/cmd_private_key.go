package core

import (
	"fmt"
	"net"
)

func (server *Server) PrivateKey_Set(conn net.Conn, applicationName string, privateKey string) error {
	return server.WithPersistentApplication(applicationName, func(app *Application, cfg *Config) error {
		app.SSHPrivateKey = &privateKey
		return nil
	})
}

func (server *Server) PrivateKey_Get(conn net.Conn, applicationName string) error {
	return server.WithPersistentApplication(applicationName, func(app *Application, cfg *Config) error {
		titleLogger, dimLogger := server.getTitleAndDimLoggers(conn)
		fmt.Fprintf(titleLogger, "=== Getting private SSH key for %v\n", applicationName)
		fmt.Fprintf(dimLogger, "%v\n", *app.SSHPrivateKey)
		return nil
	})
}

func (server *Server) PrivateKey_Remove(conn net.Conn, applicationName string) error {
	return server.WithPersistentApplication(applicationName, func(app *Application, cfg *Config) error {
		app.SSHPrivateKey = nil
		return nil
	})
}
