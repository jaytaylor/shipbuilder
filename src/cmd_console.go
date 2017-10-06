package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"regexp"

	"github.com/kr/pty"
)

// TODO: Add support for opening a console when the app is scaled to 0.
func (server *Server) Console(conn net.Conn, applicationName string, args []string) error {
	return server.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		var err error = nil
		if app.LastDeploy == "" {
			return fmt.Errorf("console not unavailable - application has not yet had a first deploy")
		}

		Send(conn, Message{Hijack, ""})

		e := Executor{conn}

		// If the primary application container is missing for some reason, attempt to create it by
		// pulling the most recent release from S3.
		if !e.ContainerExists(applicationName) {
			err = app.CreateBaseContainerIfMissing(&e)
			if err != nil {
				return err
			}
			err = extractAppFromS3(&e, app, app.LastDeploy)
			if err != nil {
				return err
			}
		}

		containerName := applicationName + DYNO_DELIMITER + "console" + DYNO_DELIMITER + RandomAlphaNumericString(8)

		if e.ContainerExists(containerName) {
			err = e.DestroyContainer(containerName)
			if err != nil {
				return err
			}
		}

		err = e.CloneContainer(applicationName, containerName)
		if err != nil {
			return err
		}

		err = e.StartContainer(containerName)
		if err != nil {
			return err
		}
		defer func() {
			e.StopContainer(containerName)
			e.DestroyContainer(containerName)
		}()

		// Setup a pseudo terminal.
		c := e.AttachContainer(containerName, args...)
		f, err := pty.Start(c)
		if err != nil {
			return err
		}
		defer f.Close()

		ch := make(chan error, 1)

		// Read the output.
		go func() {
			_, err := io.Copy(conn, f)
			ch <- err
		}()
		// Send the input.
		go func() {
			_, err := io.Copy(f, conn)
			ch <- err
		}()

		// Wait for either end to complete
		<-ch
		return nil
	})
}

func RandomAlphaNumericString(numSourceBytes int) string {
	randomBytes := make([]byte, numSourceBytes)
	_, err := rand.Read(randomBytes)
	if err != nil {
		panic("Failed to get random bytes: " + err.Error())
	}
	re := regexp.MustCompile("[^a-zA-Z0-9.]")
	randomString := base64.StdEncoding.EncodeToString(randomBytes)
	return re.ReplaceAllString(randomString, "")
}
