package main

import (
	"fmt"
	"io"
	"net"
	"os/exec"

	"github.com/kr/pty"
)

// TODO: Add support for opening a console when the app is scaled to 0.
func (this *Server) Console(conn net.Conn, applicationName string) error {
	err := this.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		process := "web"
		// Find a host running the latest version.
		runningDynos, err := this.getRunningDynos(app.Name, process)
		if err != nil {
			return err
		}
		if len(runningDynos) == 0 {
			return fmt.Errorf("No running web processes found, operation aborted.")
		}

		host := runningDynos[0].Host

		Logf(conn, "Opening SSH session to %v\n", host)
		Send(conn, Message{Hijack, ""})

		e := Executor{conn}

		tempName := applicationName + DYNO_DELIMITER + process + DYNO_DELIMITER + "console"

		err = e.Run("ssh", DEFAULT_NODE_USERNAME+"@"+host, "sudo", "/bin/bash", "-c",
			`"lxc-clone -B `+lxcFs+` -s -o `+runningDynos[0].Application+` -n `+tempName+` && lxc-start -n `+tempName+` -d"`,
		)
		if err != nil {
			return err
		}
		defer e.Run("ssh", DEFAULT_NODE_USERNAME+"@"+host, "sudo", "/bin/bash", "-c",
			`"lxc-stop -k -n `+tempName+`; lxc-destroy -n `+tempName+`"`,
		)

		// Setup a pseudo terminal
		c := exec.Command("ssh", "-t", DEFAULT_NODE_USERNAME+"@"+host, "--", "sudo", "lxc-console", "-n", tempName, "-t", "2")
		f, err := pty.Start(c)
		if err != nil {
			return err
		}
		defer f.Close()

		ec := make(chan error, 1)

		// Read the output
		go func() {
			_, err := io.Copy(conn, f)
			ec <- err
		}()
		// Send the input
		go func() {
			_, err := io.Copy(f, conn)
			ec <- err
		}()

		// Wait for either end to complete
		<-ec
		return nil
	})
	return err
}
