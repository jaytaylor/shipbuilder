package main

import (
	"fmt"
	"net"
)

func (this *Server) Ps_List(conn net.Conn, applicationName string) error {
	return this.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		str := ""
		for process, numDynos := range app.Processes {
			dynos, err := this.GetRunningDynos(app.Name, process)
			if err != nil {
				Logf(conn, "Error: %v (process was '%v')", err, process)
				continue
			}
			Logf(conn, "=== %v: dyno scale=%v, actual=%v\n", process, numDynos, len(dynos))
			for _, dyno := range dynos {
				Logf(conn, "%v @ %v [%v:%v]\n", process, dyno.Version, dyno.Host, dyno.Port)
			}
			Logf(conn, "\n")
		}
		return Send(conn, Message{Log, str})
	})
}

// e.g. ps:scale web=12 worker=12 scheduler=1
func (this *Server) Ps_Scale(conn net.Conn, applicationName string, args map[string]string) error {
	return this.Rescale(conn, applicationName, args)
}

// Restart all dynos for a particular process type.
// e.g. ps:restart web -amyApp
func (this *Server) Ps_Restart(conn net.Conn, applicationName string, processTypes []string) error {
	if len(processTypes) == 0 {
		return fmt.Errorf("list of process types must not be empty")
	}
	return this.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		// Validate client-submitted list of process types.
		for _, processType := range processTypes {
			if _, ok := app.Processes[processType]; !ok {
				return fmt.Errorf("unrecognized process type: %v", processType)
			}
		}
		for _, processType := range processTypes {
			err := this.RestartProcessType(conn, app, processType)
			if err != nil {
				return err
			}
		}
		return nil
	})
}
