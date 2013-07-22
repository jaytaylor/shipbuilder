package main

import (
	"net"
	"strconv"
)

func (this *Server) Ps_List(conn net.Conn, applicationName string) error {
	return this.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		str := ""
		for process, numDynos := range app.Processes {
			dynos, err := this.getRunningDynos(app.Name, process)
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

func (this *Server) Rescale(args map[string]string, applicationName string) error {
	return this.WithPersistentApplication(applicationName, func(app *Application, cfg *Config) error {
		for processType, numDynosStr := range args {
			numDynos, err := strconv.Atoi(numDynosStr)
			if err != nil {
				return err
			}
			app.Processes[processType] = numDynos
		}
		return nil
	})
}

// ps:scale web=12 worker=12 scheduler=1
func (this *Server) Ps_Scale(conn net.Conn, applicationName string, args map[string]string) error {
	err := this.Rescale(args, applicationName)
	if err != nil {
		return err
	}
	return this.Redeploy(conn, applicationName)
}
