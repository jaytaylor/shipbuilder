package main

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type (
	Executor struct {
		logger io.Writer
	}
)

func (this *Executor) Run(name string, args ...string) error {
	if name == "ssh" {
		//fmt.Fprint(this.logger, "debug: injecting ssh args\n")
		args = append([]string{"-o", "StrictHostKeyChecking no", "-o", "BatchMode yes"}, args...)
	}
	io.WriteString(this.logger, "$ "+name+" "+strings.Join(args, " ")+"\n")
	cmd := exec.Command(name, args...)
	cmd.Stdout = this.logger
	cmd.Stderr = this.logger
	err := cmd.Run()
	return err
}

func (this *Executor) ContainerExists(name string) bool {
	_, err := os.Stat(LXC_DIR + "/" + name)
	return err == nil
}
func (this *Executor) StartContainer(name string) error {
	if this.ContainerExists(name) {
		return this.Run("sudo", "lxc-start", "-d", "-n", name)
	}
	return nil // Don't operate on non-existent containers.
}
func (this *Executor) StopContainer(name string) error {
	if this.ContainerExists(name) {
		return this.Run("sudo", "lxc-stop", "-k", "-n", name)
	}
	return nil // Don't operate on non-existent containers.
}
func (this *Executor) DestroyContainer(name string) error {
	if this.ContainerExists(name) {
		this.StopContainer(name)
		err := this.Run("sudo", "lxc-destroy", "-n", name)
		// zfs-fuse sometimes takes a few tries to destroy a container.
		if lxcFs == "zfs" {
			if err != nil {
				for i := 0; i <= 10; i++ {
					time.Sleep(1 * time.Second)
					err = this.Run("sudo", "lxc-destroy", "-n", name)
					if err == nil {
						return err
					}
				}
			}
		}
		return err
	}
	return nil // Don't operate on non-existent containers.
}
func (this *Executor) CloneContainer(oldName, newName string) error {
	return this.Run("sudo", "lxc-clone", "-s", "-B", lxcFs, "-o", oldName, "-n", newName)
}

func (this *Executor) BashCmd(cmd string) error {
	return this.Run("sudo", "/bin/bash", "-c", cmd)
}
