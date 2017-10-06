package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type Deployment struct {
	StartedTs   time.Time
	Server      *Server
	Logger      io.Writer
	Application *Application
	Config      *Config
	Revision    string
	Version     string
	ScalingOnly bool // Flag to indicate whether this is a new release or a scaling activity.
	err         error
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

func (server *Deployment) createContainer() error {
	var (
		dimLogger   = NewFormatter(server.Logger, DIM)
		titleLogger = NewFormatter(server.Logger, GREEN)
		e           = Executor{
			logger: dimLogger,
		}
	)

	// If there's not already a container.
	if _, err := os.Stat(server.Application.RootFsDir()); err != nil {
		fmt.Fprintf(titleLogger, "Creating container\n")
		// Clone the base application.
		if server.err = e.CloneContainer("base-"+server.Application.BuildPack, server.Application.Name); server.err != nil {
			return server.err
		}
	} else {
		fmt.Fprintf(titleLogger, "App image container already exists\n")
	}

	if err := e.BashCmd("rm -rf " + server.Application.AppDir() + "/*"); err != nil {
		return err
	}
	if err := e.BashCmd("mkdir -p " + server.Application.SrcDir()); err != nil {
		return err
	}
	// Copy the ShipBuilder binary into the container.
	if server.err = e.BashCmd("cp " + EXE + " " + server.Application.AppDir() + "/" + BINARY); server.err != nil {
		return server.err
	}
	// Export the source to the container.  Use `--depth 1` to omit the history which wasn't going to be used anyways.
	if server.err = e.BashCmd("git clone --depth 1 " + server.Application.GitDir() + " " + server.Application.SrcDir()); server.err != nil {
		return server.err
	}

	// Add the public ssh key for submodule (and later dependency) access.
	if err := server.applySshPrivateKeyFile(); err != nil {
		return err
	}

	// Checkout the given revision.
	if server.err = e.BashCmd("cd " + server.Application.SrcDir() + " && git checkout -q -f " + server.Revision); server.err != nil {
		return server.err
	}

	// Convert references to submodules to be read-only.
	{
		cmdStr := fmt.Sprintf(`test -f '%[1]v/.gitmodules' && echo 'git: converting submodule refs to be read-only' && sed -i 's,git@github.com:,git://github.com/,g' '%[1]v/.gitmodules' || echo 'git: project does not appear to have any submodules'`, server.Application.SrcDir())
		if server.err = e.BashCmd(cmdStr); server.err != nil {
			return server.err
		}
	}

	// Update the submodules.
	if server.err = e.BashCmd("cd " + server.Application.SrcDir() + " && git submodule init && git submodule update"); server.err != nil {
		return server.err
	}

	// Clear out and remove all git files from the container; they are unnecessary from this point forward.
	// NB: If this command fails, don't abort anything, just log the error.
	{
		cmdStr := fmt.Sprintf(`find %v . -regex '^.*\.git\(ignore\|modules\|attributes\)?$' -exec rm -rf {} \; 1>/dev/null 2>/dev/null`, server.Application.SrcDir())
		if ignorableErr := e.BashCmd(cmdStr); ignorableErr != nil {
			fmt.Fprintf(dimLogger, ".git* cleanup failed: %v\n", ignorableErr)
		}
	}

	return nil
}

func (server *Deployment) prepareEnvironmentVariables(e *Executor) error {
	// Write out the environmental variables.
	if err := e.BashCmd("rm -rf " + server.Application.AppDir() + "/env"); err != nil {
		return err
	}
	if err := e.BashCmd("mkdir -p " + server.Application.AppDir() + "/env"); err != nil {
		return err
	}
	for key, value := range server.Application.Environment {
		if err := ioutil.WriteFile(server.Application.AppDir()+"/env/"+key, []byte(value), os.FileMode(int(0444))); err != nil {
			return err
		}
	}
	return nil
}

func (server *Deployment) prepareShellEnvironment(e *Executor) error {
	// Update the container's /etc/passwd file to use the `envdirbash` script and /app/src as the user's home directory.
	escapedAppSrc := strings.Replace(server.Application.LocalSrcDir(), "/", `\/`, -1)
	err := e.Run("sudo",
		"sed", "-i",
		`s/^\(`+DEFAULT_NODE_USERNAME+`:.*:\):\/home\/`+DEFAULT_NODE_USERNAME+`:\/bin\/bash$/\1:`+escapedAppSrc+`:\/bin\/bash/g`,
		server.Application.RootFsDir()+"/etc/passwd",
	)
	if err != nil {
		return err
	}
	// Move /home/<user>/.ssh to the new home directory in /app/src
	{
		cmdStr := fmt.Sprintf("cp -a /home/%v/.[a-zA-Z0-9]* %v/", DEFAULT_NODE_USERNAME, server.Application.SrcDir())
		if err := e.BashCmd(cmdStr); err != nil {
			return err
		}
	}
	return nil
}

func (server *Deployment) prepareAppFilePermissions(e *Executor) error {
	// Chown the app src & output to default user by grepping the uid+gid from /etc/passwd in the container.
	return e.BashCmd(
		"touch " + server.Application.AppDir() + "/out && " +
			"chown $(cat " + server.Application.RootFsDir() + "/etc/passwd | grep '^" + DEFAULT_NODE_USERNAME + ":' | cut -d':' -f3,4) " +
			server.Application.AppDir() + " && " +
			"chown -R $(cat " + server.Application.RootFsDir() + "/etc/passwd | grep '^" + DEFAULT_NODE_USERNAME + ":' | cut -d':' -f3,4) " +
			server.Application.AppDir() + "/{out,src}",
	)
}

// Disable unnecessary services in container.
func (server *Deployment) prepareDisabledServices(e *Executor) error {
	// Disable `ondemand` power-saving service by unlinking it from /etc/rc*.d.
	if err := e.BashCmd(`find ` + server.Application.RootFsDir() + `/etc/rc*.d/ -wholename '*/S*ondemand' -exec unlink {} \;`); err != nil {
		return err
	}
	// Disable `ntpdate` client from being triggered when networking comes up.
	if err := e.BashCmd(`chmod a-x ` + server.Application.RootFsDir() + `/etc/network/if-up.d/ntpdate`); err != nil {
		return err
	}
	// Disable auto-start for unnecessary services in /etc/init/*.conf, such as: SSH, rsyslog, cron, tty1-6, and udev.
	services := []string{
		"ssh",
		"rsyslog",
		"cron",
		"tty1",
		"tty2",
		"tty3",
		"tty4",
		"tty5",
		"tty6",
		"udev",
	}
	for _, service := range services {
		if err := e.BashCmd("echo 'manual' > " + server.Application.RootFsDir() + "/etc/init/" + service + ".override"); err != nil {
			return err
		}
	}
	if err := e.BashCmd(`sed -i 's/^NTPSERVERS=".*"$/NTPSERVERS=""/' /etc/default/ntpdate`); err != nil {
		return err
	}
	return nil
}

func (server *Deployment) build() (err error) {
	var (
		dimLogger   = NewFormatter(server.Logger, DIM)
		titleLogger = NewFormatter(server.Logger, GREEN)
		e           = &Executor{
			logger: dimLogger,
		}
	)

	fmt.Fprint(titleLogger, "Building image\n")

	// To be sure we are starting with a container in the stopped state.
	if stopErr := e.StopContainer(server.Application.Name); stopErr != nil {
		log.Debugf("Error stopping container for app=%v: %s (this can likely be ignored)", server.Application.Name, stopErr)
	}

	// Defer removal of the ssh private key file.
	defer func() {
		if rmErr := server.removeSshPrivateKeyFile(); rmErr != nil {
			if err == nil {
				err = rmErr
			} else {
				log.Warnf("found pre-existing err=%q and encountered a problem removing ssh private key for app=%q: %s", err, server.Application.Name, rmErr)
			}
		}
	}()

	// Prepare /app/env now so that the app env vars are available to the pre-hook script.
	if err = server.prepareEnvironmentVariables(e); err != nil {
		return
	}

	var f *os.File

	// Create upstart script.
	if f, err = os.OpenFile(server.Application.RootFsDir()+"/etc/init/app.conf", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0444); err != nil {
		return
	}
	if err = UPSTART.Execute(f, nil); err != nil {
		err = fmt.Errorf("applying upstart template: %s", err)
		return
	}
	if err = f.Close(); err != nil {
		err = fmt.Errorf("closing file=%q: %s", f.Name(), err)
		return
	}

	// Create the build script.
	if f, err = os.OpenFile(server.Application.RootFsDir()+APP_DIR+"/run", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777); err != nil {
		return
	}
	if err = BUILD_PACKS[server.Application.BuildPack].Execute(f, nil); err != nil {
		err = fmt.Errorf("applying build-pack template: %s", err)
		return
	}
	if err = f.Close(); err != nil {
		err = fmt.Errorf("closing file=%q: %s", f.Name(), err)
		return
	}

	// Create a file to store container launch output in.
	if f, err = os.Create(server.Application.AppDir() + "/out"); err != nil {
		return
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			if err == nil {
				err = fmt.Errorf("closing file=%q: %s", f.Name(), closeErr)
			} else {
				log.Warnf("found pre-existing err=%q and encountered a problem closing file=%q: %s", err, f.Name(), closeErr)
			}
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
				log.Debugf("Received cancel request, pre-hook waiter bailing out for app=%v", server.Application.Name)
				return
			default:
			}

			n, readErr := f.Read(buf)
			if readErr != nil {
				log.Debugf("Unexpected read error while running pre-hook: %s", readErr)
			}
			if n > 0 {
				dimLogger.Write(buf[:n])
				if bytes.Contains(buf, []byte("RETURN_CODE")) || bytes.Contains(buf, []byte("exited with non-zero status")) {
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

	if err = e.StartContainer(server.Application.Name); err != nil {
		cancelCh <- struct{}{}
		err = fmt.Errorf("starting container: %s", err)
		return
	}

	fmt.Fprintf(titleLogger, "Waiting for container pre-hook\n")

	select {
	case err = <-errCh:
	case <-time.After(waitDuration):
		err = fmt.Errorf("timed out after %v", waitDuration)
		cancelCh <- struct{}{}
	}
	stopErr := e.StopContainer(server.Application.Name)
	if err != nil {
		return
	}
	if stopErr != nil {
		err = fmt.Errorf("stopping container: %s", stopErr)
		return
	}

	if err = server.prepareShellEnvironment(e); err != nil {
		return
	}
	if err = server.prepareAppFilePermissions(e); err != nil {
		return
	}
	if err = server.prepareDisabledServices(e); err != nil {
		return
	}

	return
}

// TODO: check for ignored errors.
func (server *Deployment) archive() error {
	var (
		dimLogger = NewFormatter(server.Logger, DIM)
		e         = Executor{
			logger: dimLogger,
		}
		versionedContainerName = server.Application.Name + DYNO_DELIMITER + server.Version
	)

	if err := e.CloneContainer(server.Application.Name, versionedContainerName); err != nil {
		return err
	}

	// Compress & upload the image to S3.
	go func() {
		e = Executor{
			logger: NewLogger(os.Stdout, "[archive] "),
		}
		archiveName := "/tmp/" + versionedContainerName + ".tar.gz"
		if err := e.BashCmd("tar --create --gzip --preserve-permissions --file " + archiveName + " " + server.Application.RootFsDir()); err != nil {
			return
		}

		h, err := os.Open(archiveName)
		if err != nil {
			return
		}
		defer func(archiveName string, e Executor) {
			fmt.Fprintf(e.logger, "Closing filehandle and removing archive file \"%v\"\n", archiveName)
			h.Close()
			e.BashCmd("rm -f " + archiveName)
		}(archiveName, e)
		stat, err := h.Stat()
		if err != nil {
			return
		}
		getS3Bucket().PutReader(
			"/releases/"+server.Application.Name+"/"+server.Version+".tar.gz",
			h,
			stat.Size(),
			"application/x-tar-gz",
			"private",
		)
	}()
	return nil
}

// TODO: check for ignored errors.
func (server *Deployment) extract(version string) error {
	e := Executor{
		logger: server.Logger,
	}

	if err := server.Application.CreateBaseContainerIfMissing(&e); err != nil {
		return err
	}

	// Detect if the container is already present locally.
	versionedAppContainer := server.Application.Name + DYNO_DELIMITER + version
	if e.ContainerExists(versionedAppContainer) {
		fmt.Fprintf(server.Logger, "Syncing local copy of %v\n", version)
		// Rsync to versioned container to base app container.
		rsyncCommand := "rsync --recursive --links --hard-links --devices --specials --acls --owner --group --perms --times --delete --xattrs --numeric-ids "
		return e.BashCmd(rsyncCommand + LXC_DIR + "/" + versionedAppContainer + "/rootfs/ " + server.Application.RootFsDir())
	}

	// The requested app version doesn't exist locally, attempt to download it from S3.
	if err := extractAppFromS3(&e, server.Application, version); err != nil {
		return err
	}
	return nil
}

// TODO: check for ignored errors.
func extractAppFromS3(e *Executor, app *Application, version string) error {
	fmt.Fprintf(e.logger, "Downloading release %v from S3\n", version)
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

	fmt.Fprintf(e.logger, "Extracting %v\n", localArchive)
	if err := e.BashCmd("rm -rf " + app.RootFsDir() + "/*"); err != nil {
		return err
	}
	if err := e.BashCmd("tar -C / --extract --gzip --preserve-permissions --file " + localArchive); err != nil {
		return err
	}
	return nil
}

func (server *Deployment) syncNode(node *Node) error {
	var (
		logger = NewLogger(server.Logger, "["+node.Host+"] ")
		e      = Executor{
			logger: logger,
		}
	)

	// TODO: Maybe add fail check to clone operation.
	err := e.Run("ssh", DEFAULT_NODE_USERNAME+"@"+node.Host,
		"sudo", "/bin/bash", "-c",
		`"test ! -d '`+LXC_DIR+`/`+server.Application.Name+`' && lxc-clone -B `+lxcFs+` -s -o base-`+server.Application.BuildPack+` -n `+server.Application.Name+` || echo 'app image already exists'"`,
	)
	if err != nil {
		fmt.Fprintf(logger, "error cloning base container: %v\n", err)
		return err
	}
	// Rsync the application container over.
	//rsync --recursive --links --hard-links --devices --specials --owner --group --perms --times --acls --delete --xattrs --numeric-ids
	err = e.Run("sudo", "rsync",
		"--recursive",
		"--links",
		"--hard-links",
		"--devices",
		"--specials",
		"--owner",
		"--group",
		"--perms",
		"--times",
		"--acls",
		"--delete",
		"--xattrs",
		"--numeric-ids",
		"-e", "ssh "+DEFAULT_SSH_PARAMETERS,
		server.Application.LxcDir()+"/rootfs/",
		"root@"+node.Host+":"+server.Application.LxcDir()+"/rootfs/",
	)
	if err != nil {
		return err
	}
	err = e.Run("rsync",
		"-azve", "ssh "+DEFAULT_SSH_PARAMETERS,
		"/tmp/postdeploy.py", "/tmp/shutdown_container.py",
		"root@"+node.Host+":/tmp/",
	)
	if err != nil {
		return err
	}
	return nil
}

func (server *Deployment) startDyno(dynoGenerator *DynoGenerator, process string) (Dyno, error) {
	var (
		dyno   = dynoGenerator.Next(process)
		logger = NewLogger(server.Logger, "["+dyno.Host+"] ")
		e      = Executor{
			logger: logger,
		}
		done = make(chan struct{})
		err  error
	)
	go func() {
		fmt.Fprint(logger, "Starting dyno")
		err = e.Run("ssh", DEFAULT_NODE_USERNAME+"@"+dyno.Host, "sudo", "/tmp/postdeploy.py", dyno.Container)
		done <- struct{}{}
	}()
	select {
	case <-done: // implicitly break.
	case <-time.After(DYNO_START_TIMEOUT_SECONDS * time.Second):
		err = fmt.Errorf("Timed out for dyno host %v", dyno.Host)
	}
	return dyno, err
}

func (server *Deployment) autoDetectRevision() error {
	if len(server.Revision) == 0 {
		revision, err := ioutil.ReadFile(server.Application.SrcDir() + "/.git/refs/heads/master")
		if err != nil {
			return err
		}
		server.Revision = strings.Trim(string(revision), "\n")
	}
	return nil
}

func writeDeployScripts() error {
	if err := ioutil.WriteFile("/tmp/postdeploy.py", []byte(POSTDEPLOY), os.FileMode(int(0777))); err != nil {
		return err
	}
	if err := ioutil.WriteFile("/tmp/shutdown_container.py", []byte(SHUTDOWN_CONTAINER), os.FileMode(int(0777))); err != nil {
		return err
	}
	return nil
}

func (server *Deployment) calculateDynosToDestroy() ([]Dyno, bool, error) {
	var (
		// Track whether or not new dynos will be allocated.  If no new allocations are necessary, no rsync'ing will be necessary.
		allocatingNewDynos = false
		// Build list of running dynos to be deactivated in the LB config upon successful deployment.
		removeDynos = []Dyno{}
	)
	for process, numDynos := range server.Application.Processes {
		runningDynos, err := server.Server.GetRunningDynos(server.Application.Name, process)
		if err != nil {
			return removeDynos, allocatingNewDynos, err
		}
		if !server.ScalingOnly {
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
	fmt.Fprintf(server.Logger, "calculateDynosToDestroy :: calculated to remove the following dynos: %v\n", removeDynos)
	return removeDynos, allocatingNewDynos, nil
}

func (server *Deployment) syncNodes() ([]*Node, error) {
	type NodeSyncResult struct {
		node *Node
		err  error
	}

	syncStep := make(chan NodeSyncResult)
	for _, node := range server.Config.Nodes {
		go func(node *Node) {
			c := make(chan error, 1)
			go func() { c <- server.syncNode(node) }()
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
	for _ = range server.Config.Nodes {
		syncResult := <-syncStep
		if syncResult.err == nil {
			availableNodes = append(availableNodes, syncResult.node)
		}
	}

	if len(availableNodes) == 0 {
		return availableNodes, fmt.Errorf("No available nodes. This is probably very bad for all apps running on this deployment system.")
	}
	return availableNodes, nil
}

func (server *Deployment) startDynos(availableNodes []*Node, titleLogger io.Writer) ([]Dyno, error) {
	// Now we've successfully sync'd and we have a list of nodes available to deploy to.
	addDynos := []Dyno{}

	dynoGenerator, err := server.Server.NewDynoGenerator(availableNodes, server.Application.Name, server.Version)
	if err != nil {
		return addDynos, err
	}

	type StartResult struct {
		dyno Dyno
		err  error
	}
	startedChannel := make(chan StartResult)

	startDynoWrapper := func(dynoGenerator *DynoGenerator, process string) {
		dyno, err := server.startDyno(dynoGenerator, process)
		startedChannel <- StartResult{dyno, err}
	}

	numDesiredDynos := 0

	// First deploy the changes and start the new dynos.
	for process, numDynos := range server.Application.Processes {
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
					// Then attempt start it again.
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

// Validate application's Procfile.
// TODO: check for ignored errors.
func (server *Deployment) validateProcfile() error {
	f, err := os.Open(server.Application.SrcDir() + "/Procfile")
	if err != nil {
		return fmt.Errorf(err.Error() + ", does this application have a \"Procfile\"?")
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	processRe := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]:.*`)
	lineNo := 1
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Line not empty or commented out
		if len(line) > 0 && strings.Index(line, "#") != 0 && strings.Index(line, ";") != 0 {
			if !processRe.MatchString(line) {
				return fmt.Errorf("Procfile validation failed on line %v: \"%v\", must match regular expression \"%v\"", lineNo, line, processRe.String())
			}
		}
		lineNo++
	}
	return nil
}

// Deploy and launch the container to nodes.
func (server *Deployment) deploy() error {
	if len(server.Application.Processes) == 0 {
		return fmt.Errorf("No processes scaled up, adjust with `ps:scale procType=#` before deploying")
	}

	if err := server.validateProcfile(); err != nil {
		return err
	}

	var (
		titleLogger = NewFormatter(server.Logger, GREEN)
		dimLogger   = NewFormatter(server.Logger, DIM)
		e           = Executor{dimLogger}
	)

	server.autoDetectRevision()

	if err := writeDeployScripts(); err != nil {
		return err
	}

	removeDynos, allocatingNewDynos, err := server.calculateDynosToDestroy()
	if err != nil {
		return err
	}

	if allocatingNewDynos {
		availableNodes, err := server.syncNodes()
		if err != nil {
			return err
		}

		// Now we've successfully sync'd and we have a list of nodes available to deploy to.
		addDynos, err := server.startDynos(availableNodes, titleLogger)
		if err != nil {
			return err
		}

		if err := server.Server.SyncLoadBalancers(&e, addDynos, removeDynos); err != nil {
			return err
		}
	}

	if !server.ScalingOnly {
		// Update releases.
		releases, err := getReleases(server.Application.Name)
		if err != nil {
			return err
		}
		// Prepend the release (releases are in descending order)
		releases = append([]Release{{
			Version:  server.Version,
			Revision: server.Revision,
			Date:     time.Now(),
			Config:   server.Application.Environment,
		}}, releases...)
		// Only keep around the latest 15 (older ones are still in S3)
		if len(releases) > 15 {
			releases = releases[:15]
		}
		err = setReleases(server.Application.Name, releases)
		if err != nil {
			return err
		}
	} else {
		// Trigger old dynos to shutdown.
		for _, removeDyno := range removeDynos {
			fmt.Fprintf(titleLogger, "Shutting down dyno: %v\n", removeDyno.Container)
			go func(rd Dyno) {
				rd.Shutdown(&Executor{os.Stdout})
			}(removeDyno)
		}
	}

	return nil
}

func (server *Deployment) postDeployHooks(err error) {
	var (
		message  string
		notify   = "0"
		color    = "green"
		revision = "."
	)

	if len(server.Revision) > 0 {
		revision = " (" + server.Revision[0:7] + ")."
	}

	durationFractionStripper, _ := regexp.Compile(`^(.*)\.[0-9]*(s)?$`)
	duration := durationFractionStripper.ReplaceAllString(time.Since(server.StartedTs).String(), "$1$2")

	hookUrl, ok := server.Application.Environment["DEPLOYHOOKS_HTTP_URL"]
	if !ok {
		log.Errorf("app %q doesn't have a DEPLOYHOOKS_HTTP_URL", server.Application.Name)
		return
	} else if err != nil {
		task := "Deployment"
		if server.ScalingOnly {
			task = "Scaling"
		}
		message = server.Application.Name + ": " + task + " operation failed after " + duration + ": " + err.Error() + revision
		notify = "1"
		color = "red"
	} else if err == nil && server.ScalingOnly {
		procInfo := ""
		err := server.Server.WithApplication(server.Application.Name, func(app *Application, cfg *Config) error {
			for proc, val := range app.Processes {
				procInfo += " " + proc + "=" + strconv.Itoa(val)
			}
			return nil
		})
		if err != nil {
			log.Warnf("PostDeployHooks scaling caught: %v", err)
		}
		if len(procInfo) > 0 {
			message = "Scaled " + server.Application.Name + " to" + procInfo + " in " + duration + revision
		} else {
			message = "Scaled down all " + server.Application.Name + " processes down to 0"
		}
	} else {
		message = "Deployed " + server.Application.Name + " " + server.Version + " in " + duration + revision
	}

	if strings.HasPrefix(hookUrl, "https://api.hipchat.com/v1/rooms/message") {
		hookUrl += "&notify=" + notify + "&color=" + color + "&from=ShipBuilder&message_format=text&message=" + url.QueryEscape(message)
		log.Infof("Dispatching app deployhook url, app=%v url=%v", server.Application.Name, hookUrl)
		go http.Get(hookUrl)
	} else {
		log.Errorf("Unrecognized app deployhook url, app=%v url=%v", server.Application.Name, hookUrl)
	}
}

func (server *Deployment) undoVersionBump() {
	e := Executor{
		logger: server.Logger,
	}
	e.DestroyContainer(server.Application.Name + DYNO_DELIMITER + server.Version)
	server.Server.WithPersistentApplication(server.Application.Name, func(app *Application, cfg *Config) error {
		// If the version hasn't been messed with since we incremented it, go ahead and decrement it because
		// this deploy has failed.
		if app.LastDeploy == server.Version {
			prev, err := app.CalcPreviousVersion()
			if err != nil {
				return err
			}
			app.LastDeploy = prev
		}
		return nil
	})
}

func (server *Deployment) Deploy() error {
	var err error

	// Cleanup any hanging chads upon error.
	defer func() {
		if err != nil {
			server.undoVersionBump()
		}
		server.postDeployHooks(err)
	}()

	if !server.ScalingOnly {
		if err = server.createContainer(); err != nil {
			return err
		}

		if err = server.build(); err != nil {
			return err
		}

		if err = server.archive(); err != nil {
			return err
		}
	}

	if err = server.deploy(); err != nil {
		return err
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
		deployment := &Deployment{
			Server:      server,
			Logger:      logger,
			Config:      cfg,
			Application: app,
			Revision:    revision,
			Version:     app.LastDeploy,
			StartedTs:   time.Now(),
		}
		err = deployment.Deploy()
		if err != nil {
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
		deployment := &Deployment{
			Server:      server,
			Logger:      logger,
			Config:      cfg,
			Application: app,
			Version:     app.LastDeploy,
			StartedTs:   time.Now(),
		}
		// Find the release that corresponds with the latest deploy.
		releases, err := getReleases(applicationName)
		if err != nil {
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
			err = server.WithPersistentApplication(applicationName, func(app *Application, cfg *Config) error {
				app.LastDeploy = previousVersion
				return nil
			})
			if err != nil {
				return err
			}
			return fmt.Errorf("failed to find previous deploy: %v", previousVersion)
		}
		Logf(conn, "redeploying\n")
		return deployment.Deploy()
	})
}

func (server *Server) Rescale(conn net.Conn, applicationName string, args map[string]string) error {
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

	// Apply the changes.
	return server.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		if app.LastDeploy == "" {
			// Nothing to redeploy.
			return fmt.Errorf("Rescaling will apply only to future deployments because this app has not yet had a first deploy")
		}

		fmt.Fprintf(logger, "will make the following scale adjustments: %v\n", changes)

		// Temporarily replace Processes with the diff.
		app.Processes = changes
		deployment := &Deployment{
			Server:      server,
			Logger:      logger,
			Config:      cfg,
			Application: app,
			Version:     app.LastDeploy,
			StartedTs:   time.Now(),
			ScalingOnly: true,
		}
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
		logger: logger,
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
