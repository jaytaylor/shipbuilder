package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"regexp"
	"strings"
)

func (server *Server) validateAppName(applicationName string) error {
	forbiddenNames := []string{"base"}
	for bp, _ := range BUILD_PACKS {
		forbiddenNames = append(forbiddenNames, "base-"+bp)
	}
	for _, forbiddenName := range forbiddenNames {
		if strings.ToLower(applicationName) == forbiddenName || strings.HasSuffix(strings.ToLower(applicationName), "-maintenance") {
			return fmt.Errorf(`Forbidden application name "` + applicationName + `"`)
		}
	}
	expr := `^[a-z]+([a-z0-9-]*[a-z0-9])?$`
	matcher := regexp.MustCompile(expr)
	if !matcher.MatchString(applicationName) {
		return fmt.Errorf("Application name must match `%v`", expr)
	}
	return nil
}

func (server *Server) validateBuildPack(buildPack string) error {
	_, ok := BUILD_PACKS[buildPack]
	if !ok {
		validChoices := []string{}
		for bp, _ := range BUILD_PACKS {
			validChoices = append(validChoices, bp)
		}
		return fmt.Errorf("unsupported buildpack requested: %v, valid choices are: %v", buildPack, validChoices)
	}
	return nil
}

func (server *Server) Apps_Create(conn net.Conn, applicationName string, buildPack string) error {
	return server.WithPersistentConfig(func(cfg *Config) error {
		applicationName = strings.ToLower(applicationName) // Always lowercase.

		err := server.validateAppName(applicationName)
		if err != nil {
			return err
		}

		// Existing app
		for _, app := range cfg.Applications {
			if app.Name == applicationName {
				return fmt.Errorf("application with name `%v` already exists", applicationName)
			}
		}

		err = server.validateBuildPack(buildPack)
		if err != nil {
			return err
		}

		dimLogger := NewFormatter(NewTimeLogger(NewMessageLogger(conn)), DIM)
		e := Executor{dimLogger}

		for _, command := range []string{
			"git init --bare " + GIT_DIRECTORY + "/" + applicationName,                                               // Create git repo.
			"cd " + GIT_DIRECTORY + "/" + applicationName + " && git symbolic-ref HEAD refs/heads/not-a-real-branch", // Make master deletable.
			"chmod -R 777 " + GIT_DIRECTORY + "/" + applicationName,
		} {
			err = e.Run("sudo", "/bin/bash", "-c", command)
			if err != nil {
				return err
			}
		}

		// Add pre receive hook
		err = ioutil.WriteFile(
			GIT_DIRECTORY+"/"+applicationName+"/hooks/pre-receive",
			[]byte(PRE_RECEIVE),
			0777,
		)
		if err != nil {
			return err
		}
		err = ioutil.WriteFile(
			GIT_DIRECTORY+"/"+applicationName+"/hooks/post-receive",
			[]byte(POST_RECEIVE),
			0777,
		)
		if err != nil {
			return err
		}

		// Save the config
		cfg.Applications = append(cfg.Applications, &Application{
			Name:        applicationName,
			BuildPack:   buildPack,
			Domains:     []string{},
			Environment: map[string]string{},
			Processes:   map[string]int{"web": 1},
			Maintenance: false,
		})
		setReleases(applicationName, []Release{})
		Logf(conn, "Your new application is ready\n")
		return nil
	})
}

func (server *Server) Apps_Destroy(conn net.Conn, applicationName string) error {
	err := server.validateAppName(applicationName)
	if err != nil {
		return err
	}

	Send(conn, Message{ReadLineRequest, "/!\\ Warning! This is a destructive action which cannot be undone /!\\\nPlease enter your app name if you are sure you want to continue: "})
	message, err := Receive(conn)
	if err != nil {
		return err
	}
	if message.Type != ReadLineResponse {
		return fmt.Errorf("Got unexpected message reponse type `%v`, wanted a `ReadLineResponse`", message.Type)
	}
	if strings.TrimSpace(message.Body) != applicationName {
		return fmt.Errorf("Incorrect application name entered. Operation aborted.")
	}

	return server.WithPersistentConfig(func(cfg *Config) error {
		titleLogger, dimLogger := server.getTitleAndDimLoggers(conn)
		e := Executor{dimLogger}

		if len(applicationName) == 0 {
			return fmt.Errorf("Cannot delete application with empty name")
		}

		nApps := make([]*Application, 0, len(cfg.Applications))
		for _, app := range cfg.Applications {
			if app.Name == applicationName {
				fmt.Fprintf(titleLogger, "Destroying application `%v`..\n", applicationName)
			} else {
				nApps = append(nApps, app)
			}
		}
		cfg.Applications = nApps

		gitPath := GIT_DIRECTORY + "/" + applicationName
		gitPathExists, err := PathExists(gitPath)
		if err != nil {
			return err
		}
		if gitPathExists {
			fmt.Fprint(dimLogger, "Removing git path: %v\n", gitPath)
			e.Run("sudo", "rm", "-r", gitPath)
		}

		lxcContainerExists, err := PathExists(LXC_DIR + "/" + applicationName)
		if err != nil {
			return err
		}
		if lxcContainerExists {
			// Remove LXC base app image + version snapshots.
			// NB: BTRFS has restrictions on how subvolumes may be removed (in this case <path>/rootfs).
			fmt.Fprint(dimLogger, "Removing app LXC container(s)\n")
			err := e.DestroyContainer(applicationName)
			relatedVersionedContainerPaths, err := filepath.Glob(LXC_DIR + "/" + applicationName + DYNO_DELIMITER + "v*")
			if err != nil {
				return err
			}
			for _, path := range relatedVersionedContainerPaths {
				tokens := strings.Split(path, "/")
				container := tokens[len(tokens)-1]
				err = e.DestroyContainer(container)
				if err != nil {
					fmt.Fprintf(dimLogger, "warn: Encountered error while destroying container '%v': %v\n", container, err)
				}
			}
		}

		fmt.Fprint(dimLogger, "Deleting archived app releases from S3\n")
		err = delReleases(applicationName, dimLogger)
		if err != nil {
			return err
		}

		return Send(conn, Message{Log, "Application destroyed\n"})
	})
}

func (server *Server) Apps_Clone(conn net.Conn, oldApplicationName, newApplicationName string) error {
	var oldApp *Application
	err := server.WithApplication(oldApplicationName, func(app *Application, cfg *Config) error {
		oldApp = app
		return nil
	})
	if err != nil {
		return err
	}
	err = server.Apps_Create(conn, newApplicationName, oldApp.BuildPack)
	if err != nil {
		return err
	}
	return server.WithPersistentApplication(newApplicationName, func(newApp *Application, cfg *Config) error {
		newApp.Environment = oldApp.Environment
		newApp.Processes = oldApp.Processes
		return nil
	})
}

func (server *Server) Apps_List(conn net.Conn) error {
	return server.WithConfig(func(cfg *Config) error {
		for _, app := range cfg.Applications {
			Logf(conn, "%v\n", app.Name)
		}
		return nil
	})
}

func (server *Server) Apps_Health(conn net.Conn) error {
	return server.WithConfig(func(cfg *Config) error {
		for _, app := range cfg.Applications {
			for process, numDynos := range app.Processes {
				dynos, err := server.GetRunningDynos(app.Name, process)
				status := "passed"
				message := ""
				if err != nil {
					status = "error"
					message = fmt.Sprintf(" error=%v", err)
				}
				if numDynos != 0 && len(dynos) != numDynos {
					if len(dynos) > numDynos {
						message = fmt.Sprintf(" detail=%v_too_many_dynos", len(dynos)-numDynos)
					} else if len(dynos) < numDynos {
						message = fmt.Sprintf(" detail=%v_too_few_dynos", numDynos-len(dynos))
					}
					status = "failed"
					message = fmt.Sprintf(" actual=%v%v", len(dynos), message)
				}
				Logf(conn, "%v appName=%v processType=%v numDynos=%v%v\n", status, app.Name, process, numDynos, message)
			}
		}
		return nil
	})
}
