package core

import (
	"fmt"
	"net"
)

func (server *Server) Ps_List(conn net.Conn, applicationName string) error {
	return server.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		str := ""
		procfile, err := Procfile(applicationName)
		if err != nil {
			Logf(conn, "Warn: app=%v failed getting or parsing Procfile: %s", app.Name, err)
		}
		for process, numDynos := range app.Processes {
			Logf(conn, "=== %v: ", process)
			//if len(procfile) > 0 {
			if _, ok := procfile[process]; !ok {
				Logf(conn, "\nWarn: app=%[1]v currently has no Procfile entry for process=%[2]v\n=== %[2]v", app.Name, process)
			}
			//}
			dynos, err := server.GetRunningDynos(app.Name, process)
			if err != nil {
				Logf(conn, "Error: %v (app=%q process=%q)", err, app.Name, process)
				continue
			}
			Logf(conn, "dyno scale=%v, actual=%v\n", numDynos, len(dynos))
			for _, dyno := range dynos {
				Logf(conn, "%v @ %v [%v:%v]\n", process, dyno.Version, dyno.Host, dyno.Port)
			}
			Logf(conn, "\n")
		}
		return Send(conn, Message{Log, str})
	})
}

// e.g. ps:scale web=12 worker=12 scheduler=1
func (server *Server) Ps_Scale(conn net.Conn, applicationName string, deferred bool, args map[string]string) error {
	return server.Rescale(conn, applicationName, deferred, args)
}

// Wrapper used by ps:[start|stop|restart|status].
func (server *Server) Ps_Manage(action string, conn net.Conn, applicationName string, processTypes []string) error {
	if len(processTypes) == 0 {
		return fmt.Errorf("list of process types must not be empty")
	}
	return server.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		// Validate client-submitted list of process types.
		for _, processType := range processTypes {
			if _, ok := app.Processes[processType]; !ok {
				return fmt.Errorf("unrecognized process type: %v", processType)
			}
		}
		for _, processType := range processTypes {
			err := server.ManageProcessState(action, conn, app, processType)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// Restart all dynos for a particular process type.
// e.g. ps:restart web -amyApp
func (server *Server) Ps_Restart(conn net.Conn, applicationName string, processTypes []string) error {
	return server.Ps_Manage("restart", conn, applicationName, processTypes)
}

// Stop all dynos for a particular process type.
// e.g. ps:stop web -amyApp
func (server *Server) Ps_Stop(conn net.Conn, applicationName string, processTypes []string) error {
	return server.Ps_Manage("stop", conn, applicationName, processTypes)
}

// Start all dynos for a particular process type.
// e.g. ps:start web -amyApp
func (server *Server) Ps_Start(conn net.Conn, applicationName string, processTypes []string) error {
	return server.Ps_Manage("start", conn, applicationName, processTypes)
}

// Get app service status for all dynos of a particular process type.
// e.g. ps:status web -amyApp
func (server *Server) Ps_Status(conn net.Conn, applicationName string, processTypes []string) error {
	return server.Ps_Manage("status", conn, applicationName, processTypes)
}
