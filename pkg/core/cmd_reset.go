package core

import (
	"fmt"
	"net"
)

func (server *Server) Reset_App(conn net.Conn, applicationName string) error {
	return server.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		titleLogger, dimLogger := server.getTitleAndDimLoggers(conn)
		e := Executor{logger: dimLogger}

		fmt.Fprintf(titleLogger, "=== Resetting %v\n", app.Name)

		if err := e.DestroyContainer(app.Name); err != nil {
			return err
		}

		if err := e.BashCmdf("rm -rf %v/%v", GIT_DIRECTORY, app.Name); err != nil {
			return fmt.Errorf("removing %v/%v: %s", GIT_DIRECTORY, app.Name, err)
		}

		if err := server.initAppGitRepo(conn, app.Name); err != nil {
			return err
		}

		fmt.Fprintf(dimLogger, "Destroyed local git repository and image for %q, dependencies will be refreshed upon next deploy\n", app.Name)

		return nil
	})
}
