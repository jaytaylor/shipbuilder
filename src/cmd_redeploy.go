package main

import (
	"fmt"
	"net"
)

func (server *Server) Redeploy_App(conn net.Conn, applicationName string) error {
	return server.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		titleLogger, _ := server.getTitleAndDimLoggers(conn)
		fmt.Fprintf(titleLogger, "=== Redeploying %v\n", app.Name)
		return server.Redeploy(conn, applicationName)
	})
}
