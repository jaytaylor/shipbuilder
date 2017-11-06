package core

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

func (exe *Executor) ContainerRunning(name string) (bool, error) {
	cmd := exec.Command("bash", "-c", fmt.Sprintf(`set -o errexit ; set -o pipefail ; lxc list --format=json | jq -c '.[] | select(.status == "Running") | select(.name == %q) | .'`, name))
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("checking if container=%q running: %s", name, err)
	}

	if len(strings.Trim(string(out), "\r\n")) == 0 {
		return false, nil
	}
	return true, nil
}

// Start a local container.
func (exe *Executor) StartContainer(name string) error {
	if exe.ContainerExists(name) {
		if err := exe.UnmountContainerFS(name); err != nil {
			return err
		}
		running, err := exe.ContainerRunning(name)
		if err != nil {
			return fmt.Errorf("checking if container %q running: %s", name, err)
		}
		if !running {
			return exe.Run("sudo", "lxc", "start", name)
		}
	}
	return nil // Don't operate on non-existent containers.
}

// Stop a local container.
func (exe *Executor) StopContainer(name string) error {
	if exe.ContainerExists(name) {
		running, err := exe.ContainerRunning(name)
		if err != nil {
			return fmt.Errorf("checking if container %q running: %s", name, err)
		}
		if running {
			return exe.Run("sudo", "lxc", "stop", "--force", name)
		}
	}
	return nil // Don't operate on non-existent containers.
}

// Destroy a local container.
// NB: If using zfs, any child snapshot containers will be recursively destroyed to be able to destroy the requested container.
func (exe *Executor) DestroyContainer(name string) error {
	if exe.ContainerExists(name) {
		exe.StopContainer(name)
		// zfs-fuse sometimes takes a few tries to destroy a container.
		if DefaultLXCFS == "zfs" {
			return exe.zfsDestroyContainerAndChildren(name)
		} else {
			return exe.Run("sudo", "lxc", "delete", "--force", name)
		}
	}
	return nil // Don't operate on non-existent containers.
}

// Clone a local container.
func (exe *Executor) CloneContainer(oldName, newName string) error {
	return exe.Run("sudo", "lxc", "copy", oldName, newName)
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
		"lxc", "exec", name, "--",
		"sudo", "-u", "ubuntu", "-n", "-i", "--",
		"/bin/bash", "-c", command,
	}
	log.Infof("AttachContainer name=%v, completeCommand=sudo %v", name, args)
	return exec.Command("sudo", prefixedArgs...)
}

func (exe *Executor) ContainerFSMountpoint(name string) (string, error) {
	if DefaultLXCFS != "zfs" {
		return "", opNotSupportedOnFSErr()
	}
	var (
		path = "/" + exe.ZFSContainerName(name)
		cmd  = exec.Command("sudo", "zfs", "list", "-H", "-o", "mountpoint", path)
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("locating zfs mountpoint for %q: %s (out=%v)", path, err, out)
	}
	mountpoint := strings.Trim(string(out), "\r\n")
	return mountpoint, nil
}

func (exe *Executor) MountContainerFS(name string) error {
	if DefaultLXCFS != "zfs" {
		return opNotSupportedOnFSErr()
	}

	mounted, err := exe.ContainerFSMounted(name)
	if err != nil {
		return err
	}
	if !mounted {
		path := exe.ZFSContainerName(name)
		if err = exe.Run("sudo", "zfs", "mount", path); err != nil {
			return fmt.Errorf("mounting zfs path %q: %s", path, err)
		}
	}
	return nil
}

// UnmountContainerFS check for existing mount and, if found, attempt to
// unmount it.
func (exe *Executor) UnmountContainerFS(name string) error {
	running, err := exe.ContainerRunning(name)
	if err != nil {
		return err
	}
	if running {
		return fmt.Errorf("refusing to unmount container filesystem for running container %q", name)
	}
	mounted, err := exe.ContainerFSMounted(name)
	if err != nil {
		return err
	}
	if mounted {
		exe.zfsRunAndResistDatasetIsBusy("sudo", "zfs", "umount", exe.ZFSContainerName(name))
	}
	return nil
}

func (exe *Executor) ContainerFSMounted(name string) (bool, error) {
	if DefaultLXCFS != "zfs" {
		return false, opNotSupportedOnFSErr()
	}

	cmd := exec.Command("sudo", "zfs", "mount")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("checking for existing zfs mount for %q: %s (out=%v)", name, err, out)
	}
	want := exe.ZFSContainerName(name)
	for _, line := range strings.Split(string(out), "\n") {
		zfsPath := strings.Split(line, " ")[0]
		if zfsPath == want {
			return true, nil
		}
	}
	return false, nil
}

// zfsDestroyContainerAndChildren is used internally when the filesystem type if
// zfs.  Recursively destroys children of the requested container before
// destroying.  This should only be invoked by an Executor to destroy containers.
func (exe *Executor) zfsDestroyContainerAndChildren(name string) error {
	if DefaultLXCFS != "zfs" {
		return opNotSupportedOnFSErr()
	}

	// NB: This is not working yet, and may not be required.
	/* fmt.Fprintf(exe.logger, "sudo /bin/bash -c \""+`zfs list -t snapshot | grep --only-matching '^`+DefaultZFSPool+`/`+name+`@[^ ]\+' | sed 's/^`+DefaultZFSPool+`\/`+name+`@//'`+"\"\n")
	childrenBytes, err := exec.Command("sudo", "/bin/bash", "-c", `zfs list -t snapshot | grep --only-matching '^`+DefaultZFSPool+`/`+name+`@[^ ]\+' | sed 's/^`+DefaultZFSPool+`\/`+name+`@//'`).Output()
	if err != nil {
		// Allude to one possible cause and rememdy for the failure.
		return fmt.Errorf("zfs snapshot listing failed- check that 'listsnapshots' is enabled for "+DefaultZFSPool+" ('zpool set listsnapshots=on "+DefaultZFSPool+"'), error=%v", err)
	}
	if len(strings.TrimSpace(string(childrenBytes))) > 0 {
		fmt.Fprintf(exe.logger, "Found some children for parent=%v: %v\n", name, strings.Split(strings.TrimSpace(string(childrenBytes)), "\n"))
	}
	for _, child := range strings.Split(strings.TrimSpace(string(childrenBytes)), "\n") {
		if len(child) > 0 {
			exe.StopContainer(child)
			exe.zfsDestroyContainerAndChildren(child)
			exe.zfsRunAndResistDatasetIsBusy("sudo", "zfs", "destroy", "-R", DefaultZFSPool+"/"+name+"@"+child)
			err = exe.zfsRunAndResistDatasetIsBusy("sudo", "lxc", "delete", "--force", child)
			//err := exe.zfsDestroyContainerAndChildren(child)
			if err != nil {
				return err
			}
		}
		//exe.Run("sudo", "zfs", "destroy", DefaultZFSPool+"/"+name+"@"+child)
	}*/
	//exe.zfsRunAndResistDatasetIsBusy("sudo", "zfs", "destroy", "-R", DefaultZFSPool+"/"+name)
	if err := exe.zfsRunAndResistDatasetIsBusy("sudo", "lxc", "delete", name); err != nil {
		return err
	}

	return nil
}

// zfsRunAndResistDatasetIsBusy zfs-fuse sometimes requires several attempts to
// destroy a container before the operation goes through successfully.
// Expected error messages follow the form of:
//     cannot destroy 'tank/app_vXX': dataset is busy
func (exe *Executor) zfsRunAndResistDatasetIsBusy(cmd string, args ...string) error {
	if DefaultLXCFS != "zfs" {
		return opNotSupportedOnFSErr()
	}

	var err error = nil
	for i := 0; i < 30; i++ {
		err = exe.Run(cmd, args...)
		if err == nil || (!strings.Contains(err.Error(), "dataset is busy") && !strings.Contains(err.Error(), "target is busy")) {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	return err
}

func (exe *Executor) ZFSContainerName(name string) string {
	zfsName := strings.TrimLeft(ZFS_CONTAINER_MOUNT+"/"+name, "/")
	return zfsName
}

func opNotSupportedOnFSErr() error {
	return fmt.Errorf("operation not supported for fs-type=%q", DefaultLXCFS)
}
