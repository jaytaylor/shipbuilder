package main

import (
	"fmt"
	"net"
	"strings"

	. "github.com/jaytaylor/logserver"
	lserver "github.com/jaytaylor/logserver/server"
)

var activeDrains = []*lserver.Drainer{}

func initDrains(server *Server) {
	server.WithConfig(func(cfg *Config) error {
		for _, app := range cfg.Applications {
			for _, address := range app.Drains {
				drain := server.LogServer.StartDrainer(address, EntryFilter{
					Application: app.Name,
				})
				activeDrains = append(activeDrains, drain)
			}
		}
		return nil
	})
}

func (server *Server) Drains_Add(conn net.Conn, applicationName string, addresses []string) error {
	return server.WithPersistentApplication(applicationName, func(app *Application, cfg *Config) error {
		app.Drains = server.UniqueStringsAppender(conn, app.Drains, addresses, "drain",
			func(addItem string) {
				// Open a new drain.
				drain := server.LogServer.StartDrainer(addItem, EntryFilter{Application: applicationName})
				activeDrains = append(activeDrains, drain)
			},
		)
		return nil
	})
}

func (server *Server) Drains_List(conn net.Conn, applicationName string) error {
	titleLogger, dimLogger := server.getTitleAndDimLoggers(conn)

	fmt.Fprintf(titleLogger, "=== Listing drains for %v\n", applicationName)

	return server.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		for _, address := range app.Drains {
			fmt.Fprintf(dimLogger, "%v\n", address)
		}
		return nil
	})
}

func (server *Server) Drains_Remove(conn net.Conn, applicationName string, addresses []string) error {
	err := server.WithPersistentApplication(applicationName, func(app *Application, cfg *Config) error {
		app.Drains = server.UniqueStringsRemover(conn, app.Drains, addresses, "drain", nil)
		return nil
	})
	if err != nil {
		return err
	}

	// Close and remove any matching active drains.

	newDrains := []*lserver.Drainer{}
	for _, drain := range activeDrains {
		keep := true
		if drain.Filter.Application == applicationName {
			for _, removeAddress := range addresses {
				if strings.ToLower(removeAddress) == strings.ToLower(drain.Address) {
					keep = false
				}
			}
		}
		if !keep {
			drain.Close()
		} else {
			newDrains = append(newDrains, drain)
		}
	}
	activeDrains = newDrains
	return err
}
