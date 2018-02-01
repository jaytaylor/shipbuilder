package core

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/gigawattio/errorlib"
	"github.com/gigawattio/oslib"
	"github.com/jaytaylor/shipbuilder/pkg/domain"

	log "github.com/sirupsen/logrus"
)

type DeploymentOptions struct {
	StartedTs   time.Time
	Server      *Server
	Logger      io.Writer
	Application *Application
	Config      *Config
	Revision    string
	Version     string
	ScalingOnly bool // Flag to indicate whether this is a new release or a scaling activity.
}

type Deployment struct {
	StartedTs        time.Time
	Server           *Server
	Logger           io.Writer
	Application      *Application
	Config           *Config
	Revision         string
	Version          string
	ScalingOnly      bool // Flag to indicate whether this is a new release or a scaling activity.
	exe              *Executor
	ImageFingerprint string
	err              error
}

func NewDeployment(options DeploymentOptions) *Deployment {
	var (
		dimLogger = NewFormatter(options.Logger, DIM)
		d         = &Deployment{
			StartedTs:   options.StartedTs,
			Server:      options.Server,
			Logger:      options.Logger,
			Application: options.Application,
			Config:      options.Config,
			Revision:    options.Revision,
			Version:     options.Version,
			ScalingOnly: options.ScalingOnly,
			exe: &Executor{
				Logger: dimLogger,
			},
		}
	)
	return d
}

type DeployLock struct {
	numStarted  int
	numFinished int
	mutex       sync.RWMutex
}

// start notifies the DeployLock of a newly started deployment.
func (dl *DeployLock) start() {
	dl.mutex.Lock()
	dl.numStarted++
	dl.mutex.Unlock()
}

// finish marks a deployment as completed.
func (dl *DeployLock) finish() {
	dl.mutex.Lock()
	dl.numFinished++
	dl.mutex.Unlock()
}

// value returns the current number of started deploys.  Used as a marker by the
// Dyno cleanup system to protect against taking action with stale data.
func (dl *DeployLock) value() int {
	dl.mutex.RLock()
	defer dl.mutex.RUnlock()
	return dl.numStarted
}

// validateLatest returns true if and only if no deploys are in progress and if
// a possibly out-of-date value matches the current DeployLock.numStarted value.
func (dl *DeployLock) validateLatest(value int) bool {
	dl.mutex.RLock()
	defer dl.mutex.RUnlock()
	return dl.numStarted == dl.numFinished && dl.numStarted == value
}

// deployLock keeps track of deployment run count to avoid cleanup operating on stale data.
var deployLock = DeployLock{
	numStarted:  0,
	numFinished: 0,
}

// applySshPrivateKeyFile is used to enable private github repo access.
func (d *Deployment) applySshPrivateKeyFile() error {
	if d.Application.SshPrivateKey != nil {
		if err := os.Mkdir(d.Application.SshDir(), os.FileMode(int(0700))); err != nil {
			return err
		}
		if err := ioutil.WriteFile(d.Application.SshPrivateKeyFilePath(), []byte(*d.Application.SshPrivateKey), 0500); err != nil {
			return err
		}
	}
	return nil
}

// Removes To be invoked after dependency retrieval.
func (d *Deployment) removeSshPrivateKeyFile() error {
	path := d.Application.SshPrivateKeyFilePath()
	exists, err := PathExists(path)
	if err != nil {
		return fmt.Errorf("checking if path=%v exists: %s", path, err)
	}
	if d.Application.SshPrivateKey != nil && exists {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

func (d *Deployment) createContainer() (err error) {
	titleLogger := NewFormatter(d.Logger, GREEN)

	// If there's not already a container.
	var exists bool
	if exists, err = d.exe.ContainerExists(d.Application.Name); err != nil {
		return
	} else if !exists {
		fmt.Fprintf(titleLogger, "Creating container\n")
		// Clone the base application.
		if d.err = d.initContainer(); d.err != nil {
			err = d.err
			return
		}
	} else {
		fmt.Fprintf(titleLogger, "App image container already exists\n")
	}

	// if err = d.exe.MountContainerFS(d.Application.Name); err != nil {
	// 	return
	// }
	defer func() {
		// Housekeeping.
		running, checkErr := d.exe.ContainerRunning(d.Application.Name)
		if checkErr != nil {
			log.Errorf("Unexected problem checking if container already running for app=%q: %s", d.Application.Name, err)
			return
		}
		if running {
			if stopErr := d.exe.StopContainer(d.Application.Name); stopErr != nil && stopErr != ErrContainerNotFound {
				if err == nil {
					err = stopErr
					return
				}
				log.Errorf("Unexpected problem stopping container for app=%q: %s (existing err: %s)", d.Application.Name, stopErr, err)
			}

		}
	}()

	/*
		if d.err = d.exe.BashCmdf("rm -rf %[1]v/* && mkdir -p %[1]v", d.Application.AppDir()); d.err != nil {
			err = d.err
			return
		}
		// if d.err = d.exe.BashCmdf("sudo lxc exec %v -- rm -rf %v/*", d.Application.Name, d.Application.AppDir()); d.err != nil {
		// 	err = d.err
		// 	return
		// }
		// if d.err = d.exe.BashCmdf("sudo lxc exec %v -- mkdir -p %v", d.Application.SrcDir()); d.err != nil {
		// 	err = d.err
		// 	return
		// }

		if d.err = d.b64FileIntoContainer(EXE, oslib.OsPath(string(os.PathSeparator)+"app", BINARY), "755"); d.err != nil {
			err = d.err
			return
		}

		if d.err = d.gitClone(); d.err != nil {
			err = d.err
			return
		}

		// // TEMPORARILY DISABLED
		// // TODO: RESTORE THIS FUNCTIONALITY!
		// // Add the public ssh key for submodule (and later dependency) access.
		// if d.err = d.applySshPrivateKeyFile(); d.err != nil {
		// 	err = d.err
		// 	return
		// }

		if d.err = d.containerCodeInit(); d.err != nil {
			err = d.err
			return
		}

		if d.err = d.sendPreStartScript(); d.err != nil {
			err = d.err
			return
		}*/

	// {
	// 	path := oslib.OsPath(string(os.PathSeparator)+"tmp", "init-"+d.Application.Name+".sh")
	// 	var initFile *os.File
	// 	if initFile, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(int(0755))); err != nil {
	// 		err = fmt.Errorf("opening app code init file %q: %s", path)
	// 		return
	// 	}
	// 	containerCodeTpl.Execute(wr, data)

	// 	var (
	// 		cmd = exec.Command("sudo", "lxc", "exec", d.Application.Name, "--",
	// 			fmt.Sprintf("/bin/bash -c 'set -o errexit ; tee %[1]v && chmod 555 %[1]v'", oslib.OsPath(string(os.PathSeparator)+"app", "init.sh")),
	// 		)
	// 		// r, w   = io.Pipe()
	// 		outBuf = &bytes.Buffer{}
	// 		errBuf = &bytes.Buffer{}
	// 	)
	// 	if d.err = containerCodeTpl.Execute(w, d); d.err != nil {
	// 		err = d.err
	// 		return
	// 	}
	// 	cmd.Stdin = r
	// 	cmd.Stdout = outBuf
	// 	cmd.Stderr = errBuf
	// 	if d.err = cmd.Run(); d.err != nil {
	// 		log.WithField("app", d.Application.Name).Errorf("re")
	// 		err = d.err
	// 		return
	// 	}
	// }

	// // Checkout the given revision.
	// if d.err = d.exe.BashCmdf("sudo lxc exec %v -- /bin/bash -c 'cd %v && git checkout -q -f %v", d.Application.Name, d.Application.SrcDir(), d.Revision); d.err != nil {
	// 	err = d.err
	// 	return
	// }

	// // Convert references to submodules to be read-only.
	// {
	// 	cmdStr := fmt.Sprintf(`test -f '%[1]v/.gitmodules' && echo 'git: converting submodule refs to be read-only' && sed -i 's,git@github.com:,git://github.com/,g' '%[1]v/.gitmodules' || echo 'git: project does not appear to have any submodules'`, d.Application.SrcDir())
	// 	if d.err = d.exe.BashCmd(cmdStr); d.err != nil {
	// 		err = d.err
	// 		return
	// 	}
	// }

	// // Update the submodules.
	// if d.err = d.exe.BashCmd("cd " + d.Application.SrcDir() + " && git submodule init && git submodule update"); d.err != nil {
	// 	err = d.err
	// 	return
	// }

	// // Clear out and remove all git files from the container; they are unnecessary
	// // from this point forward.
	// // NB: If this command fails, don't abort anything, just log the error.
	// {
	// 	cmdStr := fmt.Sprintf(`find %v . -regex '^.*\.git\(ignore\|modules\|attributes\)?$' -exec rm -rf {} \; 1>/dev/null 2>/dev/null`, d.Application.SrcDir())
	// 	if ignorableErr := d.exe.BashCmd(cmdStr); ignorableErr != nil {
	// 		fmt.Fprintf(dimLogger, ".git* cleanup failed: %v\n", ignorableErr)
	// 	}
	// }

	return nil
}

func (d *Deployment) initContainer() error {
	if err := d.exe.CloneContainer("base-"+d.Application.BuildPack, d.Application.Name); err != nil {
		return err
	}
	// Create path mapping as per
	// https://stgraber.org/2017/06/15/custom-user-mappings-in-lxd-containers/.
	// if err := d.addDevice("git", d.Application.GitDir(), string(os.PathSeparator)+"git"); err != nil {
	// 	return err
	// }
	// // TODO: Make into "IF LINE DOESNT EXIST, INSERT IT"
	// if err := d.exe.BashCmdf(`printf "lxd:$(id -u %[1]v):1\nroot:$(id -u %[1]v):1\n" | sudo tee -a /etc/subuid`, DEFAULT_NODE_USERNAME); err != nil {
	// 	return err
	// }
	// // && printf "lxd:$(id -g ubuntu):1\nroot:$(id -g):1\n" | sudo tee -a /etc/subgid, ...)
	return nil
}

func (d *Deployment) hasDevice(name string) (bool, error) {
	// TODO: This is a quick hack, do a proper check for error vs existence / non-existence.
	if err := d.exe.BashCmdf(`test -n "$(%v config device list %v | grep '^%v$')"`, LXC_BIN, d.Application.Name, name); err != nil {
		return false, nil
	}
	return true, nil
}

func (d *Deployment) addDevice(name string, hostPath string, containerPath string) error {
	hasDevice, err := d.hasDevice(name)
	if err != nil {
		return err
	}
	if !hasDevice {
		if err := d.exe.BashCmdf("%v config device add %v %v disk source=%v path=%v", LXC_BIN, d.Application.Name, name, hostPath, containerPath); err != nil {
			return err
		}
	}
	return nil
}

func (d *Deployment) removeDevice(name string) error {
	hasDevice, err := d.hasDevice(name)
	if err != nil {
		return err
	}
	if hasDevice {
		if err := d.exe.BashCmdf("%v config device remove %v %v", LXC_BIN, d.Application.Name, name); err != nil {
			return err
		}
	}
	return nil
}

func (d *Deployment) gitClone() (err error) {
	// LXD compat: map /git/repo to /git in the container.
	if err = d.addDevice("git", d.Application.BareGitDir(), oslib.OsPath(string(os.PathSeparator)+"git")); err != nil {
		return
	}
	defer func() {
		if rmErr := d.removeDevice("git"); rmErr != nil {
			if err == nil {
				err = rmErr
				return
			} else {
				log.WithField("app", d.Application.Name).WithField("device", "git").Errorf("Failed to remove lxc device: %s (pre-existing err=%s)", rmErr, err)
			}
		}
	}()

	// Export the source to the container.  Use `--depth 1` to omit the history
	// which wasn't going to be used anyways.
	// if err = d.exe.BashCmd("git clone --depth 1 --branch master file://" + d.Application.GitDir() + " " + d.Application.SrcDir()); err != nil {
	// if err = d.exe.BashCmd("git clone file://" + d.Application.GitDir() + " " + d.Application.SrcDir()); err != nil {
	// if err = d.exe.BashCmdf("git --git-dir=%q --work-tree=%q checkout -f", d.Application.GitDir(), d.Application.SrcDir()); err != nil {
	if err = d.lxcExecf("git clone --depth 1 file:///git /app/src"); err != nil {
		return
	}
	return
}

// containerCodeInit performs git post-clone operations inside the container.
func (d *Deployment) containerCodeInit() (err error) {
	if err = d.renderTemplateIntoContainer(containerCodeTpl, oslib.OsPath(string(os.PathSeparator)+"app", "init.sh"), "755"); err != nil {
		return
	}
	if err = d.lxcExec("/app/init.sh"); err != nil {
		return
	}
	return
}

// sendPreStartScript renders and copies over the systemd pre-start script into
// the container.
func (d *Deployment) sendPreStartScript() (err error) {
	if err = d.renderTemplateIntoContainer(preStartTpl, oslib.OsPath(string(os.PathSeparator)+"app", "preStart.sh"), "755"); err != nil {
		return
	}
	return
}

func (d *Deployment) renderTemplateIntoContainer(tpl *template.Template, dst string, permissions string) (err error) {
	var (
		filePath = oslib.OsPath(string(os.PathSeparator)+"tmp", tpl.Name()+"-"+d.Application.Name)
		file     *os.File
	)

	if file, err = os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(int(0755))); err != nil {
		err = fmt.Errorf("opening file %q for writing: %s", filePath, err)
		return
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			if err == nil {
				err = closeErr
			} else {
				log.WithField("app", d.Application.Name).Errorf("closing file %q: %s (pre-existing error=%s", filePath, closeErr, err)
			}
		}
		if rmErr := os.Remove(filePath); rmErr != nil {
			if err == nil {
				err = rmErr
			} else {
				log.WithField("app", d.Application.Name).Errorf("removing file %q: %s (pre-existing error=%s", filePath, rmErr, err)
			}
		}
	}()

	if err = tpl.Execute(file, d); err != nil {
		return
	}
	if err = d.b64FileIntoContainer(filePath, dst, permissions); err != nil {
		return
	}
	return
}

// b64FileIntoContainer copies a file into the app container via b64 encoding it.
func (d *Deployment) b64FileIntoContainer(src string, dst string, permissions string) error {
	log.WithField("app", d.Application.Name).WithField("src", src).WithField("dst", dst).WithField("permissions", permissions).Debugf("Sending file into container via base64 encoding")

	cmd := exec.Command("sudo",
		"/bin/bash", "-c",
		fmt.Sprintf(
			`set -o errexit ; set -o pipefail ; base64 < %[1]v | `+LXC_BIN+` exec -T %[2]v -- /bin/bash -c 'set -o errexit ; set -o pipefail ; base64 -d > %[3]v && chmod %[4]v %[3]v'`,
			src,
			d.Application.Name,
			dst,
			permissions,
		),
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sending %v -> %v via b64 into container: %s (out=%v)", src, dst, err, string(out))
	}
	return nil

	// NB: This way of doing it makes things hang, isn't yet working right.
	// // Copy the ShipBuilder binary into the container.
	// var (
	// 	cmd = exec.Command("sudo", "lxc", "exec", d.Application.Name, "--",
	// 		fmt.Sprintf("/bin/bash -c 'set -o errexit ; base64 -d > %[1]v && chmod %[2]v %[1]v'", oslib.OsPath(string(os.PathSeparator)+"app", BINARY), permissions),
	// 	)
	// 	r, w = io.Pipe()
	// 	enc  = base64.NewEncoder(base64.StdEncoding, w)
	// 	file *os.File
	// 	out  []byte
	// )

	// cmd.Stdin = r

	// defer enc.Close()

	// if file, err = os.Open(EXE); d.err != nil {
	// 	err = fmt.Errorf("opening file %q: %s", EXE, d.err)
	// 	return
	// }
	// defer func() {
	// 	if closeErr := file.Close(); closeErr != nil {
	// 		if err == nil {
	// 			err = closeErr
	// 		} else {
	// 			log.WithField("app", d.Application.Name).Errorf("closing file %q: %s (pre-existing error=%s", EXE, closeErr, err)
	// 		}
	// 	}
	// }()

	// if _, err = io.Copy(enc, file); err != nil {
	// 	return
	// }

	// if out, err = cmd.CombinedOutput(); err != nil {
	// 	err = fmt.Errorf("sending SB binary: %s (out=%v)", err, string(out))
	// 	return
	// }
	// return

	// // if d.err = d.exe.BashCmdf("sudo lxc exec %v cp " + EXE + " " + d.Application.AppDir() + "/" + BINARY); d.err != nil {
	// // 	err = d.err
	// // 	return
	// // }
}

func (d *Deployment) prepareEnvironmentVariables() (err error) {
	if len(d.Application.Environment) == 0 {
		log.WithField("app", d.Application.Name).Debug("App has no environment variables set, skipping preparation")
		return
	}

	// Write out the environmental variables and transfer into container.
	var (
		tempDirBytes []byte
		tempDir      string
	)

	if tempDirBytes, err = exec.Command("mktemp", "--directory").CombinedOutput(); err != nil {
		return fmt.Errorf("creating temp directory for env setup: %s", err)
	}
	tempDir = string(bytes.Trim(tempDirBytes, "\r\n"))
	defer func() {
		if rmErr := os.RemoveAll(tempDir); rmErr != nil {
			if err == nil {
				err = fmt.Errorf("removing tmp dir %q: %s", tempDir, err)
			} else {
				log.WithField("app", d.Application.Name).Errorf("Problem removing tmp dir %q: %s (pre-existing err=%s)", tempDir, rmErr, err)
			}
		}
	}()

	for key, value := range d.Application.Environment {
		path := oslib.OsPath(tempDir, key)
		if err = ioutil.WriteFile(path, []byte(value), os.FileMode(int(0444))); err != nil {
			err = fmt.Errorf("writing path %q: %s", path, err)
			return
		}
	}

	// Create env tar.
	if err = d.exe.BashCmdf("cd %[1]v && tar -czvf env.tar.gz *", tempDir); err != nil {
		err = fmt.Errorf("creating env tar: %s", err)
		return
	}

	// Send to container.
	if err = d.b64FileIntoContainer(oslib.OsPath(tempDir, "env.tar.gz"), "/tmp/env.tar.gz", "444"); err != nil {
		err = fmt.Errorf("sending env tar to container: %s", err)
		return
	}

	if err = d.lxcExec(`bash -c 'set -o errexit ; tar --directory /app/env -xzvf /tmp/env.tar.gz && rm -f /tmp/env.tar.gz'`); err != nil {
		err = fmt.Errorf("extracting env tar: %s", err)
		return
	}

	return

	// if err := d.exe.BashCmd("rm -rf " + oslib.OsPath(d.Application.AppDir(), "env")); err != nil {
	// 	return err
	// }
	// if err := d.exe.BashCmd("mkdir -p " + oslib.OsPath(d.Application.AppDir(), "env")); err != nil {
	// 	return err
	// }
	// for key, value := range d.Application.Environment {
	// 	if err := ioutil.WriteFile(oslib.OsPath(d.Application.AppDir(), "env", key), []byte(value), os.FileMode(int(0444))); err != nil {
	// 		return err
	// 	}
	// }
	// return nil
}

func (d *Deployment) prepareShellEnvironment() error {
	// Update the container's /etc/passwd file to use the `envdirbash` script and
	// /app/src as the user's home directory.
	if err := d.lxcExecf("mkdir -p /app/src && ps -ef && usermod -d /app/src %v", DEFAULT_NODE_USERNAME); err != nil {
		return err
	}
	// TODO: TRACK DOWN THE ENVDIR THING - IT DOESN'T APPEAR TO HAVE EVER EXISTED???

	// escapedAppSrc := strings.Replace(d.Application.LocalSrcDir(), "/", `\/`, -1)
	// err := d.exe.Run("sudo",
	// 	"lxc", "exec", d.Application.Name, "--",
	// 	"sed", "-i",
	// 	fmt.Sprintf(`s/^\(%[1]v:.*:\):\/home\/%[1]v:\/bin\/bash$/\1:%[2]v:\/bin\/bash/g`, DEFAULT_NODE_USERNAME, escapedAppSrc),
	// 	"/etc/passwd",
	// )
	// if err != nil {
	// 	return err
	// }

	// Move /home/<user>/.ssh to the new home directory in /app/src
	if err := d.lxcExecf("test ! -d /home/%[1]v/.ssh || cp -a /home/%[1]v/.ssh /app/src/", DEFAULT_NODE_USERNAME); err != nil {
		return err
	}
	return nil
}

func (d *Deployment) prepareAppFilePermissions() error {
	// Chown the app env, src, and output to default node user.
	return d.lxcExecf(`bash -c "touch /app/out && chown $(id -u %[1]v):$(id -g %[1]v) /app && chown -R $(id -u %[1]v):$(id -g %[1]v) /app/{env,out,src}"`, DEFAULT_NODE_USERNAME)
	// return d.exe.BashCmd(
	// 	"touch " + d.Application.AppDir() + "/out && " +
	// 		"chown $(cat " + d.Application.RootFsDir() + "/etc/passwd | grep '^" + DEFAULT_NODE_USERNAME + ":' | cut -d':' -f3,4) " +
	// 		d.Application.AppDir() + " && " +
	// 		"chown -R $(cat " + d.Application.RootFsDir() + "/etc/passwd | grep '^" + DEFAULT_NODE_USERNAME + ":' | cut -d':' -f3,4) " +
	// 		d.Application.AppDir() + "/{out,src}",
	// )
}

// installAppPPAs processes and acts upon an apps ".ppas" file.
func (d *Deployment) installAppPPAs() error {
	script := `
function installPPAs() {
    if [ -r .ppas ] ; then
        while read line ; do
            if [[ "${line}" =~ ^[#\;] ]] ; then
                continue
            fi

            name=$(echo "${line}" | sed 's/^ppa://')
            found=$(grep "${name}" /etc/apt/sources.list /etc/apt/sources.list.d/*)
            if [ -z "${found}" ] ; then
                echo "info: Installing PPA: ${name}"
                sudo --non-interactive add-apt-repository -y "${line}"
                rc=$?
                if [ ${rc} -ne 0 ] ; then
                    echo "error: Installing PPA ${name} exited with non-zero status code ${rc}"
                    exit ${rc}
                fi
            fi
        done < .ppas
    fi
}

set -o errexit
cd /app/src
set +o errexit

installPPAs
`
	file := "/tmp/ppas.sh"
	if err := ioutil.WriteFile(file, []byte(script), os.FileMode(int(0777))); err != nil {
		return err
	}
	if err := d.b64FileIntoContainer(file, file, "777"); err != nil {
		return err
	}
	if err := d.lxcExec(file); err != nil {
		return fmt.Errorf("installing app PPAs: %s", err)
	}
	if err := d.lxcExec("rm -f /tmp/ppas.sh"); err != nil {
		return fmt.Errorf("cleaning up app PPAs script: %s", err)
	}
	return nil
}

// installAppPackages processes and acts upon an apps ".packages" file.
func (d *Deployment) installAppPackages() error {
	script := `
function installPackages() {
    if [ -r .packages ] ; then
        needed=''

        while read line ; do
            if [[ "${line}" =~ ^[#\;] ]] ; then
                continue
            fi
            installed=$(dpkg -s "${line}" 2>/dev/null | grep '^Status' | sed 's/^Status: //')
            if [ "${installed}" != 'install ok installed' ] ; then
                needed="${needed} ${line}"
            fi
        done < .packages

        if [ -n "${needed}" ] ; then
            echo "info: Installing packages: ${needed}"
            sudo --non-interactive apt install -y ${needed}

            rc=$?
            if [ ${rc} -ne 0 ] ; then
                echo "error: Installing packages \"${needed}\" exited with non-zero status code ${rc}"
                exit ${rc}
            fi
        fi
    fi
}

set -o errexit
cd /app/src
set +o errexit

installPackages
`
	file := "/tmp/packages.sh"
	if err := ioutil.WriteFile(file, []byte(script), os.FileMode(int(0777))); err != nil {
		return err
	}
	if err := d.b64FileIntoContainer(file, file, "777"); err != nil {
		return err
	}
	if err := d.lxcExec(file); err != nil {
		return fmt.Errorf("installing app packages: %s", err)
	}
	if err := d.lxcExec("rm -f /tmp/packages.sh"); err != nil {
		return fmt.Errorf("cleaning up app packages script: %s", err)
	}
	return nil
}

// PurgePackages is the list of packages to be purged from app containers.
var PurgePackages = []string{
	"dbus",
}

// DisableServices is the list of unnecessary system services to disable in app containers.
var DisableServices = []string{
	"atd",
	"autovt@",
	"cloud-config",
	"cloud-final",
	"cloud-init",
	"cloud-init-local",
	"cron",
	"friendly-recovery",
	"getty@",
	"iscsid",
	"lvm2-monitor",
	"lxcfs",
	"lxd-containers",
	"open-iscsi",
	"open-vm-tools",
	"pollinate",
	"rsyslog",
	"snapd",
	"snapd.autoimport",
	"snapd.core-fixup",
	"snapd.system-shutdown",
	"ssh",
	"systemd-timesyncd",
	"udev",
	"ufw",
	"unattended-upgrades",
	"ureadahead",
}

// Disable unnecessary services in container.
func (d *Deployment) prepareDisabledServices() error {
	// Disable `ondemand` cpu scalaing power-saving service.
	if err := d.lxcExec("update-rc.d ondemand disable"); err != nil {
		return err
	}
	// NB: No longer needed in ubuntu 16.04.
	// // Disable `ntpdate` client from being triggered when networking comes up.
	// if err := d.lxcExec("chmod a-x /etc/network/if-up.d/ntpdate", d.Application.Name); err != nil {
	// 	return err
	// }

	// TODO: COME BACK TO THIS LATER, IT BREAKS B/C NETWORKING SERVICE NOT YET STARTED.
	// if len(PurgePackages) > 0 {
	// 	if err := d.lxcExecf("apt-get purge -y --no-upgrade %v", strings.Join(PurgePackages, " ")); err != nil {
	// 		return err
	// 	}
	// }

	if len(DisableServices) > 0 {
		// Disable auto-start for unnecessary services, such as:
		// SSH, rsyslog, cron, tty1-6, and udev.

		// NB: The ` || :' is because sometimes this exits with unhappy exit status code even when nothing is wrong.
		if err := d.lxcExecf(`/bin/bash -c "echo '%v' | xargs -n1 -IX /bin/bash -c 'systemctl is-enabled X 1>/dev/null && ( systemctl stop X ; systemctl disable X )' || :"`, strings.Join(DisableServices, "\n")); err != nil {
			return err
		}
	}
	return nil
}

func (d *Deployment) build() (err error) {
	var (
		dimLogger   = NewFormatter(d.Logger, DIM)
		titleLogger = NewFormatter(d.Logger, GREEN)
	)

	fmt.Fprint(titleLogger, "Building image\n")

	// To be sure we are starting with a container in the stopped state.
	if stopErr := d.exe.StopContainer(d.Application.Name); stopErr != nil && stopErr != ErrContainerNotFound {
		log.WithField("app", d.Application.Name).WithField("err", stopErr).Errorf("Problem stopping container (this can likely be ignored)")
	}

	// if d.err = d.exe.MountContainerFS(d.Application.Name); d.err != nil {
	// 	err = d.err
	// 	return
	// }

	if d.err = d.exe.StartContainer(d.Application.Name); d.err != nil {
		err = d.err
		return
	}

	// if d.err = d.exe.BashCmdf("rm -rf %[1]v/* && mkdir -p %[1]v", d.Application.AppDir()); d.err != nil {
	// 	err = d.err
	// 	return
	// }
	if d.err = d.lxcExec("bash -c 'rm -rf /app/src /app/env && mkdir -p /app /app/env'"); d.err != nil {
		err = d.err
		return
	}

	log.Debugf("SENDING SB BIN...")
	if d.err = d.b64FileIntoContainer(EXE, oslib.OsPath(string(os.PathSeparator)+"app", BINARY), "755"); d.err != nil {
		err = d.err
		return
	}

	if d.err = d.gitClone(); d.err != nil {
		err = d.err
		return
	}

	// // TEMPORARILY DISABLED
	// // TODO: RESTORE THIS FUNCTIONALITY!
	// // Add the public ssh key for submodule (and later dependency) access.
	// if d.err = d.applySshPrivateKeyFile(); d.err != nil {
	// 	err = d.err
	// 	return
	// }

	if d.err = d.containerCodeInit(); d.err != nil {
		err = d.err
		return
	}

	if d.err = d.Validate(); d.err != nil {
		err = d.err
		return
	}

	// // TEMPORARILY DISABLED
	// // TODO: RESTORE THIS FUNCTIONALITY!
	// // Defer removal of the ssh private key file.
	// defer func() {
	// 	if rmErr := d.removeSshPrivateKeyFile(); rmErr != nil {
	// 		if err == nil {
	// 			err = rmErr
	// 		} else {
	// 			log.Warnf("found pre-existing err=%q and encountered a problem removing ssh private key for app=%q: %s", err, d.Application.Name, rmErr)
	// 		}
	// 	}
	// }()

	prepErr := func() error {
		if err := d.prepareShellEnvironment(); err != nil {
			return err
		}
		if err := d.prepareAppFilePermissions(); err != nil {
			return err
		}
		if err := d.prepareDisabledServices(); err != nil {
			return err
		}
		if err := d.installAppPPAs(); err != nil {
			return err
		}
		if err := d.installAppPackages(); err != nil {
			return err
		}
		return nil
	}()
	if prepErr != nil {
		err = prepErr
		d.err = err
		if err := d.exe.StopContainer(d.Application.Name); err != nil && err != ErrContainerNotFound {
			log.WithField("app", d.Application.Name).Errorf("Unexpected error stopping container after prep failure: %s", err)
		}
		return
	}

	// Prepare /app/env now so that the app env vars are available to the pre-hook
	// script.
	if err = d.prepareEnvironmentVariables(); err != nil {
		return
	}

	// Create app system service.
	if d.err = d.renderTemplateIntoContainer(systemdAppTpl, oslib.OsPath(string(os.PathSeparator)+"etc", "systemd", "system", "app.service"), "644"); d.err != nil {
		d.err = fmt.Errorf("rendering app.service systemd template into container: %s", d.err)
		err = d.err
		return
	}

	// Enable app system service.
	if d.err = d.lxcExec("systemctl enable app"); d.err != nil {
		d.err = fmt.Errorf("enabling app system service: %s", err)
		err = d.err
		return
	}

	if d.err = d.sendPreStartScript(); d.err != nil {
		err = d.err
		return
	}

	var bp domain.Buildpack
	if bp, d.err = d.Server.BuildpacksProvider.New(d.Application.BuildPack); d.err != nil {
		err = d.err
		return
	}

	var tpl *template.Template
	if tpl, d.err = template.New(d.Application.BuildPack).Parse(bp.PreHook()); d.err != nil {
		d.err = fmt.Errorf("compiling pre-hook template: %s", d.err)
		err = d.err
		return
	}

	if d.err = d.renderTemplateIntoContainer(tpl, oslib.OsPath(string(os.PathSeparator)+"app", "run"), "777"); d.err != nil {
		d.err = fmt.Errorf("rendering /app/run template: %s", d.err)
		err = d.err
		return
	}

	// Resart container to trigger the build.
	if d.err = d.exe.RestartContainer(d.Application.Name); d.err != nil {
		err = d.err
		return
	}

	// Connect to build output file.
	cmd := exec.Command(LXC_BIN, "exec", d.Application.Name, "--", "/bin/bash", "-c", "tail --follow --lines=+0 /app/out")

	r, err := cmd.StdoutPipe()
	if err != nil {
		d.err = fmt.Errorf("getting stdout monitor pipe: %s", err)
		err = d.err
		return
	}

	if err = cmd.Start(); err != nil {
		d.err = fmt.Errorf("monitoring /app/out: %s", err)
		err = d.err
		return
	}
	defer func() {
		if killErr := cmd.Process.Kill(); killErr != nil {
			if err != nil {
				log.Errorf("Problem killing process pid=%v: %s", cmd.Process.Pid, killErr)
				return
			}
			d.err = fmt.Errorf("killing out monitor pid=%v: %s", cmd.Process.Pid, killErr)
			err = d.err
		}
	}()

	// Run the pre-hook with a timeout.
	var (
		errCh        = make(chan error)
		cancelCh     = make(chan struct{}, 1)
		waitDuration = 30 * time.Minute
	)

	go func() {
		buf := make([]byte, 8192)
		var waitErr error
		for {
			select {
			case <-cancelCh:
				log.WithField("app", d.Application.Name).Debug("Received cancel request, pre-hook waiter bailing out")
				return
			default:
			}

			n, readErr := r.Read(buf)
			if readErr != nil {
				log.Errorf("Unexpected read error reading pre-hook script output: %s", readErr)
				time.Sleep(time.Second) // TODO: Use exponential backoff.
			}
			if n > 0 {
				dimLogger.Write(buf[:n])
				if bytes.Contains(buf, []byte("RETURN_CODE")) || bytes.Contains(buf, []byte("/app/run: line ")) {
					if !bytes.Contains(buf, []byte("RETURN_CODE: 0")) {
						waitErr = fmt.Errorf("build failed")
					}
					break
				}
			} else {
				time.Sleep(time.Millisecond * 100)
			}
		}
		errCh <- waitErr
	}()

	// if err = d.exe.StartContainer(d.Application.Name); err != nil {
	// 	cancelCh <- struct{}{}
	// 	err = fmt.Errorf("starting container: %s", err)
	// 	return
	// }

	fmt.Fprintf(titleLogger, "Waiting for container pre-hook\n")

	select {
	case err = <-errCh:
		// if err == nil {
		// 	prepErr := func() error {
		// 		if err := d.prepareShellEnvironment(); err != nil {
		// 			return err
		// 		}
		// 		if err := d.prepareAppFilePermissions(); err != nil {
		// 			return err
		// 		}
		// 		if err := d.prepareDisabledServices(); err != nil {
		// 			return err
		// 		}
		// 		return nil
		// 	}()
		// 	if prepErr != nil {
		// 		err = prepErr
		// 		d.exe.StopContainer(d.Application.Name)
		// 		return
		// 	}
		// }

	case <-time.After(waitDuration):
		err = fmt.Errorf("timed out after %v", waitDuration)
		cancelCh <- struct{}{}
	}

	stopErr := d.exe.StopContainer(d.Application.Name)
	if err != nil {
		return
	}
	if stopErr != nil {
		err = fmt.Errorf("stopping container: %s", stopErr)
		return
	}

	return
}

func (d *Deployment) lxcExec(cmd string) error {
	c := exec.Command(LXC_BIN, "exec", "-T", d.Application.Name, "--", "/bin/bash", "-c", fmt.Sprintf("set -o errexit ; set -o pipefail ; %v", cmd))
	if output, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("%s (output=%v)", err, string(output))
	}
	return nil
}

func (d *Deployment) lxcExecf(cmd string, args ...interface{}) error {
	c := exec.Command(LXC_BIN, "exec", "-T", d.Application.Name, "--", "/bin/bash", "-c", fmt.Sprintf("set -o errexit ; set -o pipefail ; %v", fmt.Sprintf(cmd, args...)))
	if output, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("%s (output=%v)", err, string(output))
	}
	return nil
}

func (d *Deployment) archive() error {
	versionedContainerName := d.Application.Name + DYNO_DELIMITER + d.Version

	// Compress & persist the container image.
	go func() {
		var (
			archive    = fmt.Sprintf("/tmp/%v.tar.gz", versionedContainerName)
			logContext = log.WithField("app", d.Application).WithField("version", d.Version).WithField("archive", archive)
			e          = Executor{
				Logger: NewLogger(os.Stdout, "[archiver] "),
			}
		)

		err := func() error {
			// NB: LXC (rather annoyingly) automatically appends '.tar.gz' to whatever
			// argument is specified for the output filename.
			if err := e.BashCmdf(LXC_BIN+" image export local:%v-%v %v", d.Application.Name, d.Version, strings.TrimSuffix(archive, ".tar.gz")); err != nil {
				return fmt.Errorf("exporting image: %s; operation aborted", err)
			}

			h, err := os.Open(archive)
			if err != nil {
				return fmt.Errorf("unable to open archive: %s; operation aborted", err)
			}
			defer func() {
				fmt.Fprintf(e.Logger, "[archiver] Closing filehandle and removing archive file %q\n", archive)
				logContext.Debugf("[archiver] Closing filehandle and removing archive file %q\n", archive)
				if closeErr := h.Close(); closeErr != nil {
					logContext.Errorf("[archiver] Problem closing archive filehandle: %s", closeErr)
				}
				if rmErr := e.BashCmd("rm -f " + archive); rmErr != nil {
					logContext.Errorf("[archiver] Problem removing archive: %s", rmErr)
				}
			}()

			stat, err := h.Stat()
			if err != nil {
				return fmt.Errorf("unable to stat archive: %s; operation aborted", err)
			}
			if err := d.Server.ReleasesProvider.Store(d.Application.Name, d.Version, h, stat.Size()); err != nil {
				return fmt.Errorf("storing release - operation failed: %s", err)
			}

			return nil
		}()

		if err == nil {
			logContext.Info("[archiver] Successfully stored release")
		} else {
			logContext.Errorf("[archiver] Archival failed: %s", err)
		}
		// TODO: CLEAN UP OLD LOCAL AND REMOTE LXC IMAGES.
		// TODO: Also ensure that old / unused images slave nodes get purged
		//       regularly.
	}()

	return nil
}

// extract returns the resolved image name.  This is necessary because LXC will
// refuse to import duplicate fingerprints.  So in cases where the image
// fingerprint already exists in the LXC image store, that image version must be
// used.
//
// TODO: check for ignored errors.
func (d *Deployment) extract(version string) error {
	if err := d.Application.CreateBaseContainerIfMissing(d.exe); err != nil {
		return err
	}

	// Detect if the container is already present locally.
	versionedAppContainer := d.Application.Name + DYNO_DELIMITER + version
	exists, err := d.exe.ContainerExists(versionedAppContainer)
	if err != nil {
		return err
	}
	if exists {
		fmt.Fprintf(d.Logger, "Image %v already exists locally\n", version)
		return nil
	}

	// The requested app version doesn't exist locally, attempt to download it from
	// S3.
	if err := extractAppFromS3(d.exe, d.Application, version); err != nil {
		return err
	}
	return nil
}

// TODO: check for ignored errors.
func extractAppFromS3(e *Executor, app *Application, version string) error {
	fmt.Fprintf(e.Logger, "Downloading release %v from S3\n", version)
	r, err := getS3Bucket().GetReader("/releases/" + app.Name + "/" + version + ".tar.gz")
	if err != nil {
		return err
	}
	defer r.Close()

	localArchive := "/tmp/" + app.Name + DYNO_DELIMITER + version + ".tar.gz"
	h, err := os.Create(localArchive)
	if err != nil {
		return err
	}
	defer h.Close()
	defer os.Remove(localArchive)

	if _, err := io.Copy(h, r); err != nil {
		return err
	}

	fmt.Fprintf(e.Logger, "Importing %v\n", localArchive)

	if err := e.BashCmdf("%v image import %v --public --alias %v", LXC_BIN, localArchive, app.Name+DYNO_DELIMITER+version); err != nil {
		return err
	}
	return nil
}

func (d *Deployment) syncNode(node *Node) error {
	logger := NewLogger(d.Logger, "["+node.Host+"] ")
	fmt.Fprint(logger, "Syncing slave node..\n")
	// NB: The leading ":" below is a no-op to prevent extraneous useless bash
	// output.
	bashCmds := fmt.Sprintf(`:
set -o errexit
set -o pipefail
test -n "$(%[1]v remote list | sed '1,3d' | grep -v '^+' | awk '{print $2}' | grep %[2]v)" || %[1]v remote add --accept-certificate --public %[2]v https://%[2]v:8443
%[1]v image copy --copy-aliases %[2]v:%[3]v local:`,
		LXC_BIN,
		DefaultSSHHost,
		d.lxcImageName(),
	)
	if err := d.exe.Run("ssh", "root@"+node.Host, "/bin/bash", "-c", bashCmds); err != nil {
		fmt.Fprintf(logger, "Problem sending image from host %v to %v: %s\n", DefaultSSHHost, node.Host, err)
		return fmt.Errorf("sending image from host %v to %v: %s", DefaultSSHHost, node.Host, err)
	}

	if err := d.exe.SyncContainerScripts("root@" + node.Host + ":/tmp/"); err != nil {
		return err
	}

	fmt.Fprint(logger, "Sync succesful\n")
	return nil
}

func (d *Deployment) lxcImageName() string {
	name := fmt.Sprintf("%v%v%v", d.Application.Name, DYNO_DELIMITER, d.Version)
	return name
}

func (d *Deployment) startDyno(dynoGenerator *DynoGenerator, process string) (Dyno, error) {
	var (
		dyno, err = dynoGenerator.Next(process)
		logger    = NewLogger(d.Logger, "["+dyno.Host+"] ")
		e         = Executor{
			Logger: logger,
		}
		done = make(chan struct{})
		mu   sync.Mutex
	)

	if err != nil {
		return dyno, err
	}

	go func() {
		fmt.Fprint(logger, "Starting dyno")
		mu.Lock()
		err = e.Run("ssh", DEFAULT_NODE_USERNAME+"@"+dyno.Host, "sudo", "/tmp/postdeploy.py", dyno.Container)
		mu.Unlock()
		done <- struct{}{}
	}()

	select {
	case <-done: // implicitly break.
	case <-time.After(DYNO_START_TIMEOUT_SECONDS * time.Second):
		mu.Lock()
		err = fmt.Errorf("Timed out for dyno host %v", dyno.Host)
		mu.Unlock()
	}

	return dyno, err
}

func (d *Deployment) autoDetectRevision() error {
	if len(d.Revision) == 0 {
		revision, err := ioutil.ReadFile(d.Application.BareGitDir() + "/.git/refs/heads/master")
		if err != nil {
			return err
		}
		d.Revision = strings.Trim(string(revision), "\n")
	}
	return nil
}

func (d *Deployment) calculateDynosToDestroy() ([]Dyno, bool, error) {
	var (
		// Track whether or not new dynos will be allocated.  If no new allocations
		// are necessary, no rsync'ing will be necessary.
		allocatingNewDynos = false
		// Build list of running dynos to be deactivated in the LB config upon
		// successful deployment.
		removeDynos = []Dyno{}
	)
	for process, numDynos := range d.Application.Processes {
		runningDynos, err := d.Server.GetRunningDynos(d.Application.Name, process)
		if err != nil {
			return removeDynos, allocatingNewDynos, err
		}
		if !d.ScalingOnly {
			removeDynos = append(removeDynos, runningDynos...)
			allocatingNewDynos = true
		} else if numDynos < 0 {
			// Scaling down this type of process.
			if len(runningDynos) >= -1*numDynos {
				// NB: -1*numDynos in this case == positive number of dynos to remove.
				removeDynos = append(removeDynos, runningDynos[0:-1*numDynos]...)
			} else {
				removeDynos = append(removeDynos, runningDynos...)
			}
		} else {
			allocatingNewDynos = true
		}
	}
	fmt.Fprintf(d.Logger, "calculateDynosToDestroy :: calculated to remove the following dynos: %v\n", removeDynos)
	return removeDynos, allocatingNewDynos, nil
}

func (d *Deployment) syncNodes() ([]*Node, error) {
	type NodeSyncResult struct {
		node *Node
		err  error
	}

	d.exe.SuppressOutput = true
	defer func() { d.exe.SuppressOutput = false }()

	syncStep := make(chan NodeSyncResult)
	for _, node := range d.Config.Nodes {
		go func(node *Node) {
			c := make(chan error, 1)
			go func() { c <- d.syncNode(node) }()
			go func() {
				time.Sleep(NODE_SYNC_TIMEOUT_SECONDS * time.Second)
				c <- fmt.Errorf("Sync operation to node '%v' timed out after %v seconds", node.Host, NODE_SYNC_TIMEOUT_SECONDS)
			}()
			// Block until chan has something, at which point syncStep will be notified.
			syncStep <- NodeSyncResult{node, <-c}
		}(node)
	}

	availableNodes := []*Node{}

	// Wait for all the syncs to finish or timeout, and collect available nodes.
	for _ = range d.Config.Nodes {
		syncResult := <-syncStep
		if syncResult.err == nil {
			availableNodes = append(availableNodes, syncResult.node)
		}
	}

	if len(availableNodes) == 0 {
		return availableNodes, fmt.Errorf("No available nodes. This is probably very bad for all apps running on this PaaS.")
	}
	return availableNodes, nil
}

func (d *Deployment) startDynos(availableNodes []*Node, titleLogger io.Writer) ([]Dyno, error) {
	// Now we've successfully sync'd and we have a list of nodes available to deploy to.
	addDynos := []Dyno{}

	dynoGenerator, err := d.Server.NewDynoGenerator(availableNodes, d.Application.Name, d.Version)
	if err != nil {
		return addDynos, err
	}

	type StartResult struct {
		dyno Dyno
		err  error
	}
	startedChannel := make(chan StartResult)

	startDynoWrapper := func(dynoGenerator *DynoGenerator, process string) {
		dyno, err := d.startDyno(dynoGenerator, process)
		startedChannel <- StartResult{
			dyno: dyno,
			err:  err,
		}
	}

	numDesiredDynos := 0

	// First deploy the changes and start the new dynos.
	for process, numDynos := range d.Application.Processes {
		for i := 0; i < numDynos; i++ {
			go startDynoWrapper(dynoGenerator, process)
			numDesiredDynos++
		}
	}

	if numDesiredDynos > 0 {
		timeout := time.After(DEPLOY_TIMEOUT_SECONDS * time.Second)
	OUTER:
		for {
			select {
			case result := <-startedChannel:
				if result.err != nil {
					// Then attempt to start it again.
					fmt.Fprintf(titleLogger, "Retrying starting app dyno %v on host %v, failure reason: %v\n", result.dyno.Process, result.dyno.Host, result.err)
					go startDynoWrapper(dynoGenerator, result.dyno.Process)
				} else {
					addDynos = append(addDynos, result.dyno)
					if len(addDynos) == numDesiredDynos {
						fmt.Fprintf(titleLogger, "Successfully started app on %v total dynos\n", numDesiredDynos)
						break OUTER
					}
				}
			case <-timeout:
				return addDynos, fmt.Errorf("Start operation timed out after %v seconds", DEPLOY_TIMEOUT_SECONDS)
			}
		}
	}
	return addDynos, nil
}

// Validate performs high-level validation of content contained within in the
// Deployment.
func (d *Deployment) Validate() error {
	errs := []error{
		d.validateProcfile(),
		d.validatePackages(),
		d.validatePPAs(),
	}

	if err := errorlib.Merge(errs); err != nil {
		return err
	}
	return nil
}

// validateProcfile performs validation on an apps Procfile.
// TODO: check for ignored errors.
func (d *Deployment) validateProcfile() error {
	r, err := d.bareRepoContent("Procfile")
	if err != nil {
		if err == os.ErrNotExist {
			return errors.New("missing required file: Procfile")
		}
		return err
	}

	var (
		processExpr = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]:.*$`)
		lineFilter  = func(line string) bool {
			return len(line) > 0 && strings.Index(line, "#") != 0 && strings.Index(line, ";") != 0
		}
	)

	if err := allLinesMatch(r, processExpr, lineFilter); err != nil {
		return fmt.Errorf("Procfile %s", err)
	}

	return nil
}

// validatePackages validates an apps '.packages' file, if one exists.
func (d *Deployment) validatePackages() error {
	r, err := d.bareRepoContent(".packages")
	if err != nil {
		if err == os.ErrNotExist {
			return nil
		}
		return err
	}

	var (
		packageExpr = regexp.MustCompile(`^[a-zA-Z0-9:_.=~-]+$`)
		lineFilter  = func(line string) bool {
			return len(line) > 0 && strings.Index(line, "#") != 0 && strings.Index(line, ";") != 0
		}
	)

	if err := allLinesMatch(r, packageExpr, lineFilter); err != nil {
		return fmt.Errorf(".packages %s", err)
	}

	return nil
}

// validatePPAs validates an apps '.ppas' file, if one exists.
func (d *Deployment) validatePPAs() error {
	r, err := d.bareRepoContent(".ppas")
	if err != nil {
		if err == os.ErrNotExist {
			return nil
		}
		return err
	}

	var (
		packageExpr = regexp.MustCompile(`^ppa:[a-zA-Z0-9]+\/[a-zA-Z0-9:_.=~-]+$`)
		lineFilter  = func(line string) bool {
			return len(line) > 0 && strings.Index(line, "#") != 0 && strings.Index(line, ";") != 0
		}
	)

	if err := allLinesMatch(r, packageExpr, lineFilter); err != nil {
		return fmt.Errorf(".ppas %s", err)
	}

	return nil
}

// allLinesMatch is a helper for determining if each line contained in a reader
// matches the given regular expression.
//
// lineFilterFuncs may optionally be used for additional control over which
// lines are eligible for inspection or not.  If the all functions return true, the
// line will be inspected, otherwise it will be skipped.
func allLinesMatch(r io.Reader, expr *regexp.Regexp, lineFilterFuncs ...func(line string) bool) error {
	var (
		scanner = bufio.NewScanner(r)
		lineNo  = 1
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Only proceed if line isn't empty or commented out.
		if len(lineFilterFuncs) > 0 {
			var skip bool
			for _, filterFn := range lineFilterFuncs {
				if !filterFn(line) {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
		}
		if !expr.MatchString(line) {
			return fmt.Errorf("content validation failed at line %v: \"%v\", must match regular expression \"%v\"", lineNo, line, expr.String())
		}
		lineNo++
	}

	return nil
}

// bareRepoContent gets a file from the deployment apps' bare git repository.
func (d *Deployment) bareRepoContent(file string) (io.Reader, error) {
	cmd := exec.Command("/bin/bash", "-c", fmt.Sprintf("set -o errexit ; cd %v/%v ; git show HEAD:%v", GIT_DIRECTORY, d.Application.Name, file))

	r, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("getting stdout pipe for app=%v %q content: %s", d.Application.Name, file, err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting command to check if app=%v git repo contains file %q: %s", d.Application.Name, file, err)
	}

	if err := cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			// The program has exited with a non-zero exit status code.
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				if status.ExitStatus() == 128 {
					// File was not found.
					return nil, os.ErrNotExist
				}
			}
		}
		return nil, fmt.Errorf("retrieving app=%v %q content: %s", d.Application.Name, file, err)
	}

	return r, nil
}

// Deploy and launch the container to nodes.
func (d *Deployment) deploy() error {
	if len(d.Application.Processes) == 0 {
		return fmt.Errorf("No processes scaled up, adjust with `ps:scale procType=#` before deploying")
	}

	var (
		titleLogger = NewFormatter(d.Logger, GREEN)
		dimLogger   = NewFormatter(d.Logger, DIM)
		e           = Executor{Logger: dimLogger}
	)

	d.autoDetectRevision()

	removeDynos, allocatingNewDynos, err := d.calculateDynosToDestroy()
	if err != nil {
		return err
	}

	if allocatingNewDynos {
		availableNodes, err := d.syncNodes()
		if err != nil {
			return err
		}

		// Now we've successfully sync'd and we have a list of nodes available to deploy to.
		addDynos, err := d.startDynos(availableNodes, titleLogger)
		if err != nil {
			return err
		}

		if err := d.Server.SyncLoadBalancers(&e, addDynos, removeDynos); err != nil {
			return err
		}
	}

	if !d.ScalingOnly {
		// Update releases.
		releases, err := d.Server.ReleasesProvider.List(d.Application.Name)
		if err != nil {
			return err
		}
		// Prepend the release (releases are in descending order).
		releases = append([]domain.Release{d.release()}, releases...)
		// Only keep around the latest 15 (older ones are still in S3).
		if len(releases) > 15 {
			releases = releases[:15]
		}
		if err := d.Server.ReleasesProvider.Set(d.Application.Name, releases); err != nil {
			log.WithField("app", d.Application.Name).Errorf("Problem setting releases: %s", err)
			return err
		}
		log.WithField("app", d.Application.Name).Debug("Successfully set releases")
	} else {
		// Trigger old dynos to shutdown.
		for _, removeDyno := range removeDynos {
			fmt.Fprintf(titleLogger, "Shutting down dyno: %v\n", removeDyno.Container)
			go func(rd Dyno) {
				e := &Executor{
					Logger: os.Stdout,
				}
				rd.Shutdown(e)
			}(removeDyno)
		}
	}

	return nil
}

// publish pushes the built image to the LXC image repository.
//
// As a side-effect, when successful it sets the Deployment.ImageFingerprint.
func (d *Deployment) publish() error {
	var (
		cmd    = logcmd(exec.Command(LXC_BIN, "publish", "--force", "--force-local", "--public", d.Application.Name, "--alias", d.lxcImageName()))
		stderr = &bytes.Buffer{}
	)

	cmd.Stderr = stderr

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("publishing image: %s (stdout=%v stderr=%v)", err, string(output), stderr.String())
	}

	// Extract the image fingerprint from the output.
	var (
		expr    = regexp.MustCompile(`Container published with fingerprint: (.+)`)
		matches = expr.FindSubmatch(output)
	)

	if len(matches) < 2 {
		return fmt.Errorf("fingerprint not found in lxc publish output %q", string(output))
	}

	d.ImageFingerprint = string(matches[1])

	return nil
}

func (d *Deployment) undoVersionBump() {
	d.exe.DestroyContainer(d.Application.Name + DYNO_DELIMITER + d.Version)
	d.Server.WithPersistentApplication(d.Application.Name, func(app *Application, cfg *Config) error {
		// If the version hasn't been messed with since we incremented it, go ahead and decrement it because
		// this deploy has failed.
		if app.LastDeploy == d.Version {
			prev, err := app.CalcPreviousVersion()
			if err != nil {
				return err
			}
			app.LastDeploy = prev
		}
		return nil
	})
}

func (d *Deployment) release() domain.Release {
	r := domain.Release{
		Version:          d.Version,
		Revision:         d.Revision,
		ImageFingerprint: d.ImageFingerprint,
		Date:             time.Now(),
		Config:           d.Application.Environment,
	}
	return r
}

func (d *Deployment) Deploy() error {
	var err error

	// Cleanup any hanging chads upon error.
	defer func() {
		if err != nil {
			d.undoVersionBump()
		}
		d.postDeployHooks(err)
	}()

	if !d.ScalingOnly {
		if err = d.createContainer(); err != nil {
			return fmt.Errorf("initializing: %s", err)
		}

		if err = d.build(); err != nil {
			return fmt.Errorf("building: %s", err)
		}

		if err = d.publish(); err != nil {
			return fmt.Errorf("publishing: %s", err)
		}

		if err = d.archive(); err != nil {
			return fmt.Errorf("archiving: %s", err)
		}
	}

	if err = d.deploy(); err != nil {
		return fmt.Errorf("deploying: %s", err)
	}

	return nil
}

func (server *Server) Deploy(conn net.Conn, applicationName, revision string) error {
	deployLock.start()
	defer deployLock.finish()

	logger := NewTimeLogger(NewMessageLogger(conn))
	fmt.Fprintf(logger, "Deploying revision %v\n", revision)

	return server.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		// Bump version.
		app, cfg, err := server.IncrementAppVersion(app)
		if err != nil {
			return err
		}
		deployment := NewDeployment(DeploymentOptions{
			Server:      server,
			Logger:      logger,
			Config:      cfg,
			Application: app,
			Revision:    revision,
			Version:     app.LastDeploy,
			StartedTs:   time.Now(),
		})
		if err = deployment.Deploy(); err != nil {
			return err
		}
		return nil
	})
}

func (server *Server) Redeploy(conn net.Conn, applicationName string) error {
	deployLock.start()
	defer deployLock.finish()

	logger := NewTimeLogger(NewMessageLogger(conn))

	return server.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		if app.LastDeploy == "" {
			// Nothing to redeploy.
			return fmt.Errorf("Redeploy is not going to happen because this app has not yet had a first deploy")
		}
		previousVersion := app.LastDeploy
		// Bump version.
		app, cfg, err := server.IncrementAppVersion(app)
		if err != nil {
			return err
		}

		restore := func() error {
			pErr := server.WithPersistentApplication(applicationName, func(app *Application, cfg *Config) error {
				app.LastDeploy = previousVersion
				return nil
			})
			if pErr != nil {
				return fmt.Errorf("failed to restore version=%v for app=%v: %s", previousVersion, app.Name, pErr)
			}
			return nil
		}

		deployment := NewDeployment(DeploymentOptions{
			Server:      server,
			Logger:      logger,
			Config:      cfg,
			Application: app,
			Version:     app.LastDeploy,
			StartedTs:   time.Now(),
		})
		// Find the release that corresponds with the latest deploy.
		releases, err := server.ReleasesProvider.List(applicationName)
		if err != nil {
			if rErr := restore(); rErr != nil {
				return errorlib.Merge([]error{err, rErr})
			}
			return err
		}
		found := false
		for _, r := range releases {
			if r.Version == previousVersion {
				deployment.Revision = r.Revision
				found = true
				break
			}
		}
		if !found {
			// Roll back the version bump.
			if rErr := restore(); rErr != nil {
				return rErr
			}
			return fmt.Errorf("failed to find previous deploy: %v", previousVersion)
		}
		Logf(conn, "redeploying\n")
		return deployment.Deploy()
	})
}

func (server *Server) Rescale(conn net.Conn, applicationName string, deferred bool, args map[string]string) error {
	deployLock.start()
	defer deployLock.finish()

	logger := NewLogger(NewTimeLogger(NewMessageLogger(conn)), "[scale] ")

	// Calculate scale changes to make.
	changes := map[string]int{}

	err := server.WithPersistentApplication(applicationName, func(app *Application, cfg *Config) error {
		for processType, newNumDynosStr := range args {
			newNumDynos, err := strconv.Atoi(newNumDynosStr)
			if err != nil {
				return err
			}

			oldNumDynos, ok := app.Processes[processType]
			if !ok {
				// Add new dyno type to changes.
				changes[processType] = newNumDynos
			} else if newNumDynos != oldNumDynos {
				// Take note of difference.
				changes[processType] = newNumDynos - oldNumDynos
			}

			if newNumDynos == 0 {
				delete(app.Processes, processType)
			} else {
				app.Processes[processType] = newNumDynos
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		return fmt.Errorf("No scaling changes were detected")
	}

	if deferred {
		fmt.Fprint(logger, "Rescaling will apply to the next deployment because deferral was requested\n")
		return nil
	}

	// Apply the changes.
	return server.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		if app.LastDeploy == "" {
			// Nothing to redeploy.
			return fmt.Errorf("Rescaling will apply only to future deployments because this app has not yet had a first deploy")
		}

		fmt.Fprintf(logger, "will make the following scale adjustments: %v\n", changes)

		// Temporarily replace Processes with the diff.
		app.Processes = changes
		deployment := NewDeployment(DeploymentOptions{
			Server:      server,
			Logger:      logger,
			Config:      cfg,
			Application: app,
			Version:     app.LastDeploy,
			StartedTs:   time.Now(),
			ScalingOnly: true,
		})
		return deployment.Deploy()
	})
}

// Stop, start, restart, or get the status for the service for a particular dyno process type for an app.
// @param action One of "stop", "start" "restart", or "status".
func (server *Server) ManageProcessState(action string, conn net.Conn, app *Application, processType string) error {
	// Require that the process type exist in the applications processes map.
	if _, ok := app.Processes[processType]; !ok {
		return fmt.Errorf("unrecognized process type: %v", processType)
	}
	dynos, err := server.GetRunningDynos(app.Name, processType)
	if err != nil {
		return err
	}
	logger := NewLogger(NewTimeLogger(NewMessageLogger(conn)), fmt.Sprintf("[ps:%v] ", action))
	executor := &Executor{
		Logger: logger,
	}
	for _, dyno := range dynos {
		if dyno.Process == processType {
			if action == "stop" {
				err = dyno.StopService(executor)
			} else if action == "start" {
				err = dyno.StartService(executor)
			} else if action == "restart" {
				err = dyno.RestartService(executor)
			} else if action == "status" {
				err = dyno.GetServiceStatus(executor)
			} else {
				err = fmt.Errorf("unrecognized action: %v", action)
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}
