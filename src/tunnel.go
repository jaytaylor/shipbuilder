package main

import (
	"bufio"
	"os"
	"os/exec"
	"strings"

	log "github.com/sirupsen/logrus"
)

type Tunnel struct {
	*exec.Cmd
}

func OpenTunnel() (Tunnel, error) {
	log.Infof("Client connecting via %q ..", sshHost)

	sshArgs := append(defaultSshParametersList, "-N", "-L", "9999:127.0.0.1:9999")
	if len(sshKey) > 0 {
		sshArgs = append(sshArgs, "-i", sshKey)
	}
	sshArgs = append(sshArgs, "-v", sshHost)

	cmd := exec.Command("ssh", sshArgs...)
	t := Tunnel{cmd}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return t, err
	}
	defer stderr.Close()
	err = cmd.Start()
	if err != nil {
		return t, err
	}

	wait := make(chan error)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			//log.Debugf("SSH DEBUG: %v", scanner.Text())
			if strings.Contains(scanner.Text(), "Entering interactive session.") {
				wait <- nil
				break
			}
		}
		err := scanner.Err()
		if err != nil {
			wait <- err
		}
	}()
	return t, <-wait
}

func (tun Tunnel) Close() error {
	err := tun.Process.Signal(os.Interrupt)
	if err != nil {
		err = tun.Process.Signal(os.Kill)
	}
	return err
}
