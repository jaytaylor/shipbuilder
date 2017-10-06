package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

type Executor struct {
	logger io.Writer
}

func (exe *Executor) Run(name string, args ...string) error {
	if name == "ssh" {
		// Automatically inject ssh parameters.
		args = append(defaultSshParametersList, args...)
	}
	io.WriteString(exe.logger, "$ "+name+" "+strings.Join(args, " ")+"\n")
	cmd := exec.Command(name, args...)
	cmd.Stdout = exe.logger
	cmd.Stderr = exe.logger
	err := cmd.Run()
	return err
}

// Run a pre-quoted bash command.
func (exe *Executor) BashCmd(cmd string) error {
	return exe.Run("sudo", "/bin/bash", "-c", cmd)
}

// Check if a container exists locally.
func (exe *Executor) ContainerExists(name string) bool {
	_, err := os.Stat(LXC_DIR + "/" + name)
	return err == nil
}

// Start a local container.
func (exe *Executor) StartContainer(name string) error {
	if exe.ContainerExists(name) {
		return exe.Run("sudo", "lxc-start", "-d", "-n", name)
	}
	return nil // Don't operate on non-existent containers.
}

// Stop a local container.
func (exe *Executor) StopContainer(name string) error {
	if exe.ContainerExists(name) {
		return exe.Run("sudo", "lxc-stop", "-k", "-n", name)
	}
	return nil // Don't operate on non-existent containers.
}

// Destroy a local container.
// NB: If using zfs, any child snapshot containers will be recursively destroyed to be able to destroy the requested container.
func (exe *Executor) DestroyContainer(name string) error {
	if exe.ContainerExists(name) {
		exe.StopContainer(name)
		// zfs-fuse sometimes takes a few tries to destroy a container.
		if lxcFs == "zfs" {
			return exe.zfsDestroyContainerAndChildren(name)
		} else {
			return exe.Run("sudo", "lxc-destroy", "-n", name)
		}
	}
	return nil // Don't operate on non-existent containers.
}

// This is used internally when the filesystem type if zfs.
// Recursively destroys children of the requested container before destroying.  This should only be invoked by an Executor to destroy containers.
func (exe *Executor) zfsDestroyContainerAndChildren(name string) error {
	// NB: This is not working yet, and may not be required.
	/* fmt.Fprintf(exe.logger, "sudo /bin/bash -c \""+`zfs list -t snapshot | grep --only-matching '^`+zfsPool+`/`+name+`@[^ ]\+' | sed 's/^`+zfsPool+`\/`+name+`@//'`+"\"\n")
	childrenBytes, err := exec.Command("sudo", "/bin/bash", "-c", `zfs list -t snapshot | grep --only-matching '^`+zfsPool+`/`+name+`@[^ ]\+' | sed 's/^`+zfsPool+`\/`+name+`@//'`).Output()
	if err != nil {
		// Allude to one possible cause and rememdy for the failure.
		return fmt.Errorf("zfs snapshot listing failed- check that 'listsnapshots' is enabled for "+zfsPool+" ('zpool set listsnapshots=on "+zfsPool+"'), error=%v", err)
	}
	if len(strings.TrimSpace(string(childrenBytes))) > 0 {
		fmt.Fprintf(exe.logger, "Found some children for parent=%v: %v\n", name, strings.Split(strings.TrimSpace(string(childrenBytes)), "\n"))
	}
	for _, child := range strings.Split(strings.TrimSpace(string(childrenBytes)), "\n") {
		if len(child) > 0 {
			exe.StopContainer(child)
			exe.zfsDestroyContainerAndChildren(child)
			exe.zfsRunAndResistDatasetIsBusy("sudo", "zfs", "destroy", "-R", zfsPool+"/"+name+"@"+child)
			err = exe.zfsRunAndResistDatasetIsBusy("sudo", "lxc-destroy", "-n", child)
			//err := exe.zfsDestroyContainerAndChildren(child)
			if err != nil {
				return err
			}
		}
		//exe.Run("sudo", "zfs", "destroy", zfsPool+"/"+name+"@"+child)
	}*/
	exe.zfsRunAndResistDatasetIsBusy("sudo", "zfs", "destroy", "-R", zfsPool+"/"+name)
	err := exe.zfsRunAndResistDatasetIsBusy("sudo", "lxc-destroy", "-n", name)
	if err != nil {
		return err
	}

	return nil
}

// zfs-fuse sometimes requires several attempts to destroy a container before the operation goes through successfully.
// Expected error messages follow the form of:
//     cannot destroy 'tank/app_vXX': dataset is busy
func (exe *Executor) zfsRunAndResistDatasetIsBusy(cmd string, args ...string) error {
	var err error = nil
	for i := 0; i < 30; i++ {
		err = exe.Run(cmd, args...)
		if err == nil || !strings.Contains(err.Error(), "dataset is busy") {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	return err
}

// Clone a local container.
func (exe *Executor) CloneContainer(oldName, newName string) error {
	return exe.Run("sudo", "lxc-clone", "-s", "-B", lxcFs, "-o", oldName, "-n", newName)
}

// Run a command in a local container.
func (exe *Executor) AttachContainer(name string, args ...string) *exec.Cmd {
	// Add hosts entry for container name to avoid error upon entering shell: "sudo: unable to resolve host `name`".
	err := exec.Command("sudo", "/bin/bash", "-c", `echo "127.0.0.1`+"\t"+name+`" | sudo tee -a `+LXC_DIR+"/"+name+`/rootfs/etc/hosts`).Run()
	if err != nil {
		fmt.Fprintf(exe.logger, "warn: host fix command failed for container '%v': %v\n", name, err)
	}
	// Build command to be run, prefixing any .shipbuilder `bin` directories to the environment $PATH.
	command := `export PATH="$(find /app/.shipbuilder -maxdepth 2 -type d -wholename '*bin'):${PATH}" && /usr/bin/envdir ` + ENV_DIR + " "
	if len(args) == 0 {
		command += "/bin/bash"
	} else {
		command += strings.Join(args, " ")
	}
	prefixedArgs := []string{
		"lxc-attach", "-n", name, "--",
		"sudo", "-u", "ubuntu", "-n", "-i", "--",
		"/bin/bash", "-c", command,
	}
	log.Infof("AttachContainer name=%v, completeCommand=sudo %v", name, args)
	return exec.Command("sudo", prefixedArgs...)
}
