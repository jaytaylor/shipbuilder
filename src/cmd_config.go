package main

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

func (server *Server) Config_Get(conn net.Conn, applicationName, configName string) error {
	return server.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		val, _ := app.Environment[configName]
		return Send(conn, Message{Log, val + "\n"})
	})
}

func (server *Server) Config_List(conn net.Conn, applicationName string) error {
	return server.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		titleLogger, dimLogger := server.getTitleAndDimLoggers(conn)

		fmt.Fprintf(titleLogger, "=== Environment variables for application: %v\n\n", applicationName)

		// Sort the keys alphabetically.
		keys := []string{}
		longestKey := 0
		for k, _ := range app.Environment {
			keys = append(keys, k)
			if len(k) > longestKey {
				longestKey = len(k)
			}
		}
		sort.Strings(keys)

		for _, k := range keys {
			fmt.Fprintf(dimLogger, "%v:%v%v\n", k, strings.Repeat(" ", longestKey-len(k)+1), app.Environment[k])
		}
		return nil
	})
}

func (server *Server) Config_Set(conn net.Conn, applicationName, deferred string, args map[string]string) error {
	titleLogger, dimLogger := server.getTitleAndDimLoggers(conn)

	err := server.WithPersistentApplication(applicationName, func(app *Application, cfg *Config) error {
		fmt.Fprintf(titleLogger, "=== Setting environment variables..\n\n")

		for key, value := range args {
			fmt.Fprintf(dimLogger, "    Setting %v=%v\n", key, value)
			app.Environment[key] = value
		}
		return Logf(conn, "Finished setting environment variables.\n")
	})
	if err != nil {
		return err
	}
	if deferred != "" {
		fmt.Fprintf(titleLogger, "NOTICE: Redeploy deferred, changes will not be active until next deploy is triggered\n")
		return nil
	} else {
		return server.Redeploy(conn, applicationName)
	}
}

func (server *Server) Config_Remove(conn net.Conn, applicationName, deferred string, configNames []string) error {
	titleLogger, dimLogger := server.getTitleAndDimLoggers(conn)

	err := server.WithPersistentApplication(applicationName, func(app *Application, cfg *Config) error {
		fmt.Fprintf(titleLogger, "=== Removing environment variables..\n\n")
		for _, key := range configNames {
			fmt.Fprintf(dimLogger, "    Removing '%v'\n", key)
			delete(app.Environment, key)
		}
		return Logf(conn, "Finished removing environment variables.\n")
	})
	if err != nil {
		return err
	}
	if deferred != "" {
		fmt.Fprintf(titleLogger, "NOTICE: Redeploy deferred, changes will not be active until next deploy is triggered\n")
		return nil
	} else {
		return server.Redeploy(conn, applicationName)
	}
}
