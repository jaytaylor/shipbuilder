package core

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	bashSafeEnvSetup     = `set -o errexit ; set -o pipefail ; set -o nounset ; `
	bashGrepIP           = `grep --only-matching '\([0-9]\{1,3\}\.\)\{3\}[0-9]\{1,3\}'`
	bashLXCIPWaitCommand = `set -o errexit ; set -o nounset ; ` + LXC_BIN + ` list | sed 1,3d | grep '^[|] %s \+[|] ' | awk '{ print $6 }' | (` + bashGrepIP + ` || true) | tr -d $'\n'`
)

var (
	ErrContainerNotFound = errors.New("container not found")

	containerIPWaitTimeout = 10 * time.Second
)

type Executor struct {
	Logger         io.Writer
	SuppressOutput bool
}

func (exe *Executor) Run(name string, args ...string) error {
	if name == "ssh" {
		// Automatically inject ssh parameters.
		args = append(defaultSSHParametersList, args...)
	}

	log.Debugf("Running command: " + name + " " + fmt.Sprint(args))

	if !exe.SuppressOutput {
		io.WriteString(exe.Logger, "$ "+name+" "+strings.Join(args, " ")+"\n")
	}

	cmd := logcmd(exec.Command(name, args...))
	cmd.Stdout = exe.Logger
	cmd.Stderr = exe.Logger
	err := cmd.Run()
	log.Debug("Done with " + name)
	return err
}

// Run a pre-quoted bash command.
func (exe *Executor) BashCmd(cmd string) error {
	return exe.Run("/bin/bash", "-c", bashSafeEnvSetup+cmd)
}

// Run a bash command with fmt args.
func (exe *Executor) BashCmdf(format string, args ...interface{}) error {
	return exe.BashCmd(fmt.Sprintf(format, args...))
}

// Check if an LXC image exists locally.
func (exe *Executor) ImageExists(name string) (bool, error) {
	out, err := exe.lxcImageListJqCmd(fmt.Sprintf(`.[] | .aliases | .[] | select(.name == %q) | .`, name)).Output()
	if err != nil {
		return false, fmt.Errorf("checking if image=%q exists: %s", name, err)
	}

	if len(strings.Trim(string(out), "\r\n")) == 0 {
		return false, nil
	}
	return true, nil
}

// Check if a container exists locally.
func (exe *Executor) ContainerExists(name string) (bool, error) {
	out, err := exe.lxcListJqCmd(fmt.Sprintf(`.[] | select(.name == %q) | .`, name)).Output()
	if err != nil {
		return false, fmt.Errorf("checking if container=%q exists: %s", name, err)
	}

	if len(strings.Trim(string(out), "\r\n")) == 0 {
		return false, nil
	}
	return true, nil
}

func (exe *Executor) ContainerRunning(name string) (bool, error) {
	out, err := exe.lxcListJqCmd(fmt.Sprintf(`.[] | select(.status == "Running") | select(.name == %q) | .`, name)).Output()
	if err != nil {
		return false, fmt.Errorf("checking if container=%q running: %s", name, err)
	}

	if len(strings.Trim(string(out), "\r\n")) == 0 {
		return false, nil
	}
	return true, nil
}

// lxcListJqCmd forms a command which filters `lxc list` JSON output against
// the provided JQ query.
func (exe *Executor) lxcListJqCmd(query string, jqFlags ...string) *exec.Cmd {
	if len(jqFlags) == 0 {
		jqFlags = []string{"-c"}
	}
	cmd := logcmd(exec.Command("/bin/bash", "-c", fmt.Sprintf(`%v%v list --format=json | jq %v '%v'`, bashSafeEnvSetup, LXC_BIN, strings.Join(jqFlags, " "), query)))
	return cmd
}

// lxcImageListJqCmd forms a command which filters `lxc image list` JSON output
// against the provided JQ query.
func (exe *Executor) lxcImageListJqCmd(query string, jqFlags ...string) *exec.Cmd {
	if len(jqFlags) == 0 {
		jqFlags = []string{"-r"}
	}
	cmd := logcmd(exec.Command("/bin/bash", "-c", fmt.Sprintf(`%v%v image list --format=json | jq %v '%v'`, bashSafeEnvSetup, LXC_BIN, strings.Join(jqFlags, " "), query)))
	return cmd
}

// Start a local container.
func (exe *Executor) StartContainer(name string) error {
	exists, err := exe.ContainerExists(name)
	if err != nil {
		return err
	}
	if !exists {
		// Don't attempt operation on non-existent containers.
		return ErrContainerNotFound
	}
	running, err := exe.ContainerRunning(name)
	if err != nil {
		return err
	}
	if !running {
		if err := exe.Run(LXC_BIN, "start", name); err != nil {
			return fmt.Errorf("starting container=%q: %s", name, err)
		}
		if err := exe.waitForContainerIP(name); err != nil {
			return err
		}
	}
	return nil
}

// Stop a local container.
func (exe *Executor) StopContainer(name string) error {
	exists, err := exe.ContainerExists(name)
	if err != nil {
		return err
	}
	if !exists {
		// Don't attempt operation on non-existent containers.
		return ErrContainerNotFound
	}
	running, err := exe.ContainerRunning(name)
	if err != nil {
		return err
	}
	if running {
		if err := exe.Run(LXC_BIN, "stop", "--force", name); err != nil {
			return fmt.Errorf("stopping container=%q: %s", name, err)
		}
	}
	return nil
}

func (exe *Executor) RestartContainer(name string) error {
	exists, err := exe.ContainerExists(name)
	if err != nil {
		return err
	}
	if !exists {
		// Don't attempt operation on non-existent containers.
		return ErrContainerNotFound
	}
	if err := exe.Run(LXC_BIN, "restart", "--force", name); err != nil {
		return err
	}
	if err := exe.waitForContainerIP(name); err != nil {
		return err
	}
	return nil
}

// RestoreContainerFromImage verifies that the requested image exists, checks for and
// deletes any existing conflicting destination container, then lxc launches
// and stops to restore the destination container from the contents of the
// image.
func (exe *Executor) RestoreContainerFromImage(image string, container string) error {
	if exists, err := exe.ImageExists(image); err != nil {
		return err
	} else if !exists {
		return fmt.Errorf("restoring image: specified image not found")
	}

	if exists, err := exe.ContainerExists(container); err != nil {
		return err
	} else if exists {
		if err := exe.DestroyContainer(container); err != nil {
			return err
		}
	}

	if err := exe.Run(LXC_BIN, "launch", image, container); err != nil {
		return fmt.Errorf("restoring image from container: %s", err)
	}
	if err := exe.StopContainer(container); err != nil {
		return fmt.Errorf("stopping image after restore: %s", err)
	}
	return nil
}

// waitForContainerIP waits for the specified container to receive an IP
// address.
func (exe *Executor) waitForContainerIP(name string) error {
	since := time.Now()

	for {
		if time.Now().Sub(since) > containerIPWaitTimeout {
			return fmt.Errorf("timed out after %s waiting for container=%v to receive IP", containerIPWaitTimeout, name)
		}

		cmd := logcmd(exec.Command("/bin/bash", "-c", fmt.Sprintf(bashLXCIPWaitCommand, name)))

		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("obtaining stderr pipe to check if container=%v has IP yet: %s", name, err)
		}

		if out, err := cmd.Output(); err != nil {
			stderr, _ := ioutil.ReadAll(stderrPipe)
			return fmt.Errorf("waiting for container=%v to receive IP: %s (stderr=%v)", name, err, string(stderr))
		} else if len(out) > 0 {
			log.Infof("detected IP=%v for container=%v", string(out), name)
			fmt.Fprintf(exe.Logger, "detected IP=%v for container=%v\n", string(out), name)
			break
		}

		time.Sleep(time.Second)
	}
	return nil
}

// Destroy a local container.
// NB: If using zfs, any child snapshot containers will be recursively destroyed to be able to destroy the requested container.
func (exe *Executor) DestroyContainer(name string) error {
	exists, err := exe.ContainerExists(name)
	if err != nil {
		return err
	}
	if exists {
		exe.StopContainer(name)
		// zfs-fuse sometimes takes a few tries to destroy a container.
		if DefaultLXCFS == "zfs" {
			return exe.zfsDestroyContainerAndChildren(name)
		} else {
			return exe.Run(LXC_BIN, "delete", "--force", name)
		}
	}
	return nil // Don't operate on non-existent containers.
}

// Clone a local container.
func (exe *Executor) CloneContainer(oldName, newName string) error {
	return exe.Run(LXC_BIN, "copy", oldName, newName)
}

// Run a command in a local container.
func (exe *Executor) AttachContainer(name string, args ...string) *exec.Cmd {
	// // Add hosts entry for container name to avoid error upon entering shell: "unable to resolve host `name`".
	// err := logcmd(exec.Command("/bin/bash", "-c", `echo "127.0.0.1`+"\t"+name+`" | tee -a `+LXC_DIR+"/"+name+`/rootfs/etc/hosts`)).Run()
	// if err != nil {
	// 	fmt.Fprintf(exe.logger, "warn: host fix command failed for container '%v': %v\n", name, err)
	// }
	// Build command to be run, prefixing any .shipbuilder `bin` directories to the environment $PATH.
	command := `export PATH="$(find /app/.shipbuilder -maxdepth 2 -type d -wholename '*bin'):${PATH}" && /usr/bin/envdir ` + ENV_DIR + " "
	if len(args) == 0 {
		command += "/bin/bash"
	} else {
		command += strings.Join(args, " ")
	}
	prefixedArgs := []string{
		"exec", name, "--",
		"sudo", "-u", "ubuntu", "-n", "-i", "--",
		"/bin/bash", "-c", command,
	}
	log.Infof("AttachContainer name=%v, completeCommand=%v %v", name, LXC_BIN, args)
	return logcmd(exec.Command(LXC_BIN, prefixedArgs...))
}

func (exe *Executor) ContainerFSMountpoint(name string) (string, error) {
	if DefaultLXCFS != "zfs" {
		return "", opNotSupportedOnFSErr()
	}
	var (
		path = "/" + exe.ZFSContainerName(name)
		cmd  = logcmd(exec.Command("zfs", "list", "-H", "-o", "mountpoint", path))
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
		if err = exe.Run("zfs", "mount", path); err != nil {
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
		return fmt.Errorf("refusing to unmount container filesystem for running container=%q", name)
	}
	mounted, err := exe.ContainerFSMounted(name)
	if err != nil {
		return err
	}
	if mounted {
		exe.zfsRunAndResistDatasetIsBusy("zfs", "umount", exe.ZFSContainerName(name))
	}
	return nil
}

func (exe *Executor) ContainerFSMounted(name string) (bool, error) {
	if DefaultLXCFS != "zfs" {
		return false, opNotSupportedOnFSErr()
	}

	cmd := logcmd(exec.Command("zfs", "mount"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("checking for existing zfs mount for container=%q: %s (out=%v)", name, err, out)
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
	/* fmt.Fprintf(exe.logger, "/bin/bash -c \""+`zfs list -t snapshot | grep --only-matching '^`+DefaultZFSPool+`/`+name+`@[^ ]\+' | sed 's/^`+DefaultZFSPool+`\/`+name+`@//'`+"\"\n")
	childrenBytes, err := logcmd(exec.Command("/bin/bash", "-c", `zfs list -t snapshot | grep --only-matching '^`+DefaultZFSPool+`/`+name+`@[^ ]\+' | sed 's/^`+DefaultZFSPool+`\/`+name+`@//'`)).Output()
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
			exe.zfsRunAndResistDatasetIsBusy("zfs", "destroy", "-R", DefaultZFSPool+"/"+name+"@"+child)
			err = exe.zfsRunAndResistDatasetIsBusy(LXC_BIN, "delete", "--force", child)
			//err := exe.zfsDestroyContainerAndChildren(child)
			if err != nil {
				return err
			}
		}
		//exe.Run("zfs", "destroy", DefaultZFSPool+"/"+name+"@"+child)
	}*/
	//exe.zfsRunAndResistDatasetIsBusy("zfs", "destroy", "-R", DefaultZFSPool+"/"+name)
	if err := exe.zfsRunAndResistDatasetIsBusy(LXC_BIN, "delete", "--force", name); err != nil {
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

// SyncContainerScripts sends the container control scripts to a destination of
// the form username@target:/some/path/.
func (exe *Executor) SyncContainerScripts(target string) error {
	if err := exe.WriteDeployScripts("/tmp"); err != nil {
		return err
	}
	if err := exe.Rsync(target, "/tmp/postdeploy.py", "/tmp/shutdown_container.py"); err != nil {
		return err
	}
	return nil
}

// Rsync one or more files to a destination.
func (exe *Executor) Rsync(target string, files ...string) error {
	if len(files) == 0 {
		return fmt.Errorf("cannot rsync empty file set to %q", target)
	}
	err := exe.Run("rsync",
		"-azve", "ssh "+DEFAULT_SSH_PARAMETERS,
		"/tmp/postdeploy.py", "/tmp/shutdown_container.py",
		target,
	)
	if err != nil {
		return fmt.Errorf("rsync'ing file set=%+v to %q: %s", files, target, err)
	}
	return nil
}

// WriteDeployScripts writes out python container control scripts to the
// specified local path.
func (_ *Executor) WriteDeployScripts(path string) error {
	if err := ioutil.WriteFile(fmt.Sprintf("%v/postdeploy.py", path), []byte(POSTDEPLOY), os.FileMode(int(0777))); err != nil {
		return err
	}
	if err := ioutil.WriteFile(fmt.Sprintf("%v/shutdown_container.py", path), []byte(SHUTDOWN_CONTAINER), os.FileMode(int(0777))); err != nil {
		return err
	}
	return nil
}

func opNotSupportedOnFSErr() error {
	return fmt.Errorf("operation not supported for fs-type=%q", DefaultLXCFS)
}

func logcmd(cmd *exec.Cmd) *exec.Cmd {
	log.WithField("args", cmd.Args).WithField("dir", cmd.Dir).WithField("path", cmd.Path).Debug("Command")
	return cmd
}
