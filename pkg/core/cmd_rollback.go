package core

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

func (server *Server) Rollback(conn net.Conn, applicationName, version string) error {
	return server.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		deployLock.start()
		defer deployLock.finish()

		if app.LastDeploy == "" {
			return errors.New("Automatic rollback version detection is impossible because this app has not had any releases")
		}
		if app.LastDeploy == "v1" {
			return errors.New("Automatic rollback version detection is impossible because this app has only had 1 release")
		}
		if version == "" {
			// Get release before current.
			var err error = nil
			version, err = app.CalcPreviousVersion()
			if err != nil {
				return err
			}
		}

		if strings.HasPrefix(version, "v") {
			version = strings.TrimLeft(version, "v")
		}

		logger := NewLogger(NewTimeLogger(NewMessageLogger(conn)), "[rollback] ")
		fmt.Fprintf(logger, "Rolling back to v%v\n", version)

		// Get the next version.
		app, cfg, err := server.IncrementAppVersion(app)
		if err != nil {
			return err
		}

		deployment := NewDeployment(DeploymentOptions{
			Server:      server,
			Logger:      logger,
			Config:      cfg,
			Application: app,
			Version:     app.LastDeploy,
			StartedTs:   time.Now(),
		})

		// Cleanup any hanging chads upon error.
		defer func() {
			if err != nil {
				deployment.undoVersionBump()
			}
		}()

		if err := deployment.restore(version); err != nil {
			return err
		}
		if err := deployment.archive(); err != nil {
			return err
		}
		if err := deployment.publish(); err != nil {
			return err
		}
		if err := deployment.deploy(); err != nil {
			return err
		}
		return nil
	})
}
