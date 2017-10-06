package main

import (
	"fmt"
	"net"
)

func (server *Server) Reset_App(conn net.Conn, applicationName string) error {
	return server.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		titleLogger, dimLogger := server.getTitleAndDimLoggers(conn)
		e := Executor{dimLogger}

		fmt.Fprintf(titleLogger, "=== Resetting %v\n", app.Name)

		err := e.DestroyContainer(app.Name)
		if err != nil {
			return err
		}

		fmt.Fprintf(dimLogger, "Destroyed base image for %v, dependencies will be refreshed upon next deploy\n", app.Name)

		return nil
	})
}
