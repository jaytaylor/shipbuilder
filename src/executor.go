package main

import (
	"fmt"
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
		args = injectsshParameters(args...)
	}
	io.WriteString(this.logger, "$ "+name+" "+strings.Join(args, " ")+"\n")
	cmd := exec.Command(name, args...)
	cmd.Stdout = this.logger
	cmd.Stderr = this.logger
	err := cmd.Run()
	return err
}
func injectsshParameters(args ...string) []string {
	return append([]string{"-o", "StrictHostKeyChecking no", "-o", "BatchMode yes"}, args...)
}
func (this *Executor) BashCmd(cmd string) error {
	return this.Run("sudo", "/bin/bash", "-c", cmd)
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

// NB: If using zfs, any child snapshot containers will be recursively destroyed to be able to destroy the requested container.
func (this *Executor) DestroyContainer(name string) error {
	if this.ContainerExists(name) {
		this.StopContainer(name)
		var err error = nil
		// zfs-fuse sometimes takes a few tries to destroy a container.
		if lxcFs == "zfs" {
			err = this.zfsDestroyContainerAndChildren(name)
		} else {
			err = this.Run("sudo", "lxc-destroy", "-n", name)
		}
		return err
	}
	return nil // Don't operate on non-existent containers.
}

// Recursively destroys children of the requested container before destroying.  This should only be invoked by an Executor to destroy containers.
func (this *Executor) zfsDestroyContainerAndChildren(name string) error {
	childrenBytes, err := exec.Command("sudo", "/bin/bash", "-c", `zfs list -t snapshot | grep --only-matching '^.*/`+name+`@[^ ]\+' | sed 's/^.*@//'`).Output()
	if err != nil {
		// Allude to one possible cause and rememdy for the failure.
		return fmt.Errorf("zfs snapshot listing failed- check that 'listsnapshots' is enabled for "+zfsPool+" ('zpool set listsnapshots=on "+zfsPool+"'), error=%v", err)
	}
	for _, child := range strings.Split(string(childrenBytes), "\n") {
		err := this.zfsDestroyContainerAndChildren(child)
		if err != nil {
			return err
		}
		//this.Run("sudo", "zfs", "destroy", zfsPool+"/"+name+"@"+child)
	}
	err = this.zfsRunAndResistDatasetIsBusy("sudo", "zfs", "destroy", "-R", zfsPool+"/"+name)
	if err != nil {
		return err
	}
	err = this.zfsRunAndResistDatasetIsBusy("sudo", "lxc-destroy", "-n", name)
	if err != nil {
		return err
	}

	return nil
}

// zfs-fuse sometimes requires several attempts to destroy a container before the operation goes through successfully.
// Expected error messages follow the form of:
//     cannot destroy 'tank/app_vXX': dataset is busy
func (this *Executor) zfsRunAndResistDatasetIsBusy(cmd string, args ...string) error {
	var err error = nil
	for i := 0; i < 30; i++ {
		err = this.Run(cmd, args...)
		if err == nil || !strings.Contains(err.Error(), "dataset is busy") {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	return err
}

func (this *Executor) CloneContainer(oldName, newName string) error {
	return this.Run("sudo", "lxc-clone", "-s", "-B", lxcFs, "-o", oldName, "-n", newName)
}
