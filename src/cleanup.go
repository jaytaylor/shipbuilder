package main

import (
	"fmt"
	"os"
)

func (this *Server) PruneDynos(nodeStatus NodeStatus) error {
	cfg, err := this.getConfig(true)
	if err != nil {
		return err
	}

	logger := NewLogger(os.Stdout, "[dyno-cleanup] ")
	//fmt.Fprint(logger, "Pruning inactive dynos\n")

	type Key struct {
		application string
		version     string
		process     string
	}

	appMap := map[Key]bool{}

	// Build mapping of current expected app-process-versions.
	for _, app := range cfg.Applications {
		for process, _ := range app.Processes {
			//fmt.Fprintf(logger, "Existing app found, name=%v version=%v\n", app.Name, app.LastDeploy)
			appMap[Key{app.Name, app.LastDeploy, process}] = true
		}
	}

	e := Executor{logger}

	for _, container := range nodeStatus.Containers {
		// appName-process-version-port
		dyno, err := ContainerToDyno(nodeStatus.Host, container)
		if err != nil {
			return err
		}
		key := Key{dyno.Application, dyno.Version, dyno.Process}
		_, ok := appMap[key]
		if !ok {
			fmt.Fprintf(logger, "Cleaning up trash name=%v version=%v\n", dyno.Application, dyno.Version)
			go dyno.shutdown(e)
		}
	}

	return nil
}
