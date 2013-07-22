package main

import (
	"net"
)

func (this *Server) Rollback(conn net.Conn, applicationName, version string) error {
	return this.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		// Get the next version
		app, cfg, err := this.IncrementAppVersion(app)
		if err != nil {
			return err
		}
		deployment := &Deployment{
			Server:      this,
			Logger:      NewLogger(NewMessageLogger(conn), "[redeploy] "),
			Config:      cfg,
			Application: app,
			Version:     app.LastDeploy,
		}
		err = deployment.extract(version)
		if err != nil {
			return err
		}

		err = deployment.deploy()
		if err != nil {
			return err
		}

		return nil
	})
}
