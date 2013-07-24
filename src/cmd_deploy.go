package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type (
	Deployment struct {
		Server      *Server
		Logger      io.Writer
		Application *Application
		Config      *Config
		Revision    string
		Version     string
		err         error
	}
	Executor struct {
		logger io.Writer
	}
	DeployLock struct {
		numStarted, numFinished int
		mutex                   sync.Mutex
	}
)

func (this *DeployLock) start() {
	this.mutex.Lock()
	this.numStarted++
	this.mutex.Unlock()
}

func (this *DeployLock) finish() {
	this.mutex.Lock()
	this.numFinished++
	this.mutex.Unlock()
}

func (this *DeployLock) value() int {
	this.mutex.Lock()
	defer this.mutex.Unlock()
	return this.numStarted
}

func (this *DeployLock) validateLatest(value int) bool {
	this.mutex.Lock()
	defer this.mutex.Unlock()
	return this.numStarted == this.numFinished && this.numStarted == value
}

// Keep track of deployment run count to avoid cleanup operating on stale data.
var deployLock = DeployLock{numStarted: 0, numFinished: 0}

func (this *Deployment) createContainer() error {
	dimLogger := NewFormatter(this.Logger, DIM)
	titleLogger := NewFormatter(this.Logger, GREEN)

	e := Executor{dimLogger}

	fmt.Fprintf(titleLogger, "Creating container\n")

	// If there's not already a container.
	_, err := os.Stat(this.Application.RootFsDir())
	if err != nil {
		// Clone the base application.
		this.err = e.Run("sudo", "lxc-clone", "-s", "-B", "btrfs", "-o", "base-"+this.Application.BuildPack, "-n", this.Application.Name)
		if this.err != nil {
			return this.err
		}
	}

	e.Run("sudo", "rm", "-rf", this.Application.AppDir())
	e.Run("mkdir", "-p", this.Application.SrcDir())
	// Copy the binary into the container.
	this.err = e.Run("cp", EXE, this.Application.AppDir()+"/"+BINARY)
	if this.err != nil {
		return this.err
	}
	// Export the source to the container.
	this.err = e.Run("git", "clone", this.Application.GitDir(), this.Application.SrcDir())
	if this.err != nil {
		return this.err
	}
	// Checkout the given revision.
	this.err = e.Run("/bin/bash", "-c", "cd "+this.Application.SrcDir()+" && git checkout -q -f "+this.Revision)
	if this.err != nil {
		return this.err
	}
	// Convert references to submodules to be read-only.
	this.err = e.Run("/bin/bash", "-c",
		"if [ -f '"+this.Application.SrcDir()+"/.gitmodules' ]; then echo 'converting submodule refs to be read-only'; sed -i 's,git@github.com:,git://github.com/,g' '"+this.Application.SrcDir()+"/.gitmodules'; else echo 'project does not appear to have any submodules'; fi")
	if this.err != nil {
		return this.err
	}
	// Update the submodules.
	this.err = e.Run("/bin/bash", "-c", "cd "+this.Application.SrcDir()+" && git submodule init && git submodule update")
	if this.err != nil {
		return this.err
	}
	// Chown the app src & output to default user by grepping the uid+gid from /etc/passwd in the container.
	this.err = e.Run("/bin/bash", "-c",
		"touch "+this.Application.AppDir()+"/out && "+
			"chown $(cat "+this.Application.RootFsDir()+"/etc/passwd | grep '^"+DEFAULT_NODE_USERNAME+":' | cut -d':' -f3,4) "+
			this.Application.AppDir()+" && "+
			"chown -R $(cat "+this.Application.RootFsDir()+"/etc/passwd | grep '^"+DEFAULT_NODE_USERNAME+":' | cut -d':' -f3,4) "+
			this.Application.AppDir()+"/{out,src}",
	)
	if this.err != nil {
		return this.err
	}
	return nil
}

func (this *Deployment) build() error {
	dimLogger := NewFormatter(this.Logger, DIM)
	titleLogger := NewFormatter(this.Logger, GREEN)

	e := Executor{dimLogger}

	fmt.Fprintf(titleLogger, "Building image\n")

	f, err := os.OpenFile(this.Application.RootFsDir()+"/etc/init/app.conf", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
	if err != nil {
		return err
	}
	err = UPSTART.Execute(f, nil)
	f.Close()
	if err != nil {
		return err
	}
	// Create the build script.
	f, err = os.OpenFile(this.Application.RootFsDir()+"/app/run", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
	if err != nil {
		return err
	}
	err = BUILD_PACKS[this.Application.BuildPack].Execute(f, nil)
	f.Close()
	if err != nil {
		return err
	}

	// Create a file to store the output.
	f, err = os.Create(this.Application.AppDir() + "/out")
	if err != nil {
		return err
	}
	defer f.Close()

	c := make(chan error)
	go func() {
		buf := make([]byte, 8192)
		var err error
		for {
			n, _ := f.Read(buf)
			if n > 0 {
				dimLogger.Write(buf[:n])
				if bytes.Contains(buf, []byte("RETURN_CODE")) {
					if !bytes.Contains(buf, []byte("RETURN_CODE: 0")) {
						err = fmt.Errorf("build failed")
					}
					break
				}
			} else {
				time.Sleep(time.Millisecond * 100)
			}
		}
		c <- err
	}()
	err = e.Run("sudo", "lxc-start", "-d", "-n", this.Application.Name)
	if err != nil {
		return err
	}

	select {
	case err = <-c:
	case <-time.After(30 * 60 * time.Second):
		err = fmt.Errorf("timeout")
	}
	e.Run("sudo", "lxc-stop", "-k", "-n", this.Application.Name)
	if err != nil {
		return err
	}

	// Write out the environmental variables.
	err = e.Run("sudo", "rm", "-rf", this.Application.AppDir()+"/env")
	if err != nil {
		return err
	}
	err = e.Run("sudo", "mkdir", "-p", this.Application.AppDir()+"/env")
	if err != nil {
		return err
	}
	for k, v := range this.Application.Environment {
		err = ioutil.WriteFile(this.Application.AppDir()+"/env/"+k, []byte(v), 0777)
		if err != nil {
			return err
		}
	}

	return nil
}

func (this *Deployment) archive() error {
	dimLogger := NewFormatter(this.Logger, DIM)

	e := Executor{dimLogger}
	e.Run("sudo", "lxc-clone",
		"-B", "btrfs",
		"-s",
		"-o", this.Application.Name,
		"-n", this.Application.Name+DYNO_DELIMITER+this.Version,
	)
	// Upload the image to S3.
	go func() {
		e = Executor{NewLogger(os.Stdout, "[archive]")}
		archiveName := "/tmp/" + this.Application.Name + DYNO_DELIMITER + this.Version + ".tar.gz"
		err := e.Run("sudo", "tar", "-czf", archiveName, this.Application.LxcDir())
		if err != nil {
			return
		}
		defer e.Run("sudo", "rm", "-rf", archiveName)
		h, err := os.Open(archiveName)
		if err != nil {
			return
		}
		defer h.Close()
		stat, err := h.Stat()
		if err != nil {
			return
		}
		getS3Bucket().PutReader(
			"/releases/"+this.Application.Name+"/"+this.Version+".tar.gz",
			h,
			stat.Size(),
			"application/x-tar-gz",
			"private",
		)
	}()
	return nil
}

func (this *Deployment) extract(version string) error {
	e := Executor{this.Logger}

	_, err := os.Stat(LXC_DIR + "/" + this.Application.Name + DYNO_DELIMITER + version)
	if err == nil {
		err = e.Run("sudo", "lxc-destroy", "-n", this.Application.Name)
		if err != nil {
			return err
		}
		err = e.Run("sudo", "lxc-clone", "-s", "-B", "btrfs", "-o", this.Application.Name+DYNO_DELIMITER+version, "-n", this.Application.Name)
		if err != nil {
			return err
		}
		return nil
	}

	// Pull from S3
	fmt.Fprintln(this.Logger, "Downloading from S3")
	r, err := getS3Bucket().GetReader("/releases/" + this.Application.Name + "/" + version + ".tar.gz")
	if err != nil {
		return err
	}
	defer r.Close()
	h, err := os.Create("/tmp/" + version + ".tar.gz")
	if err != nil {
		return err
	}
	defer h.Close()
	defer os.Remove("/tmp/" + version + ".tar.gz")
	_, err = io.Copy(h, r)
	if err != nil {
		return err
	}

	fmt.Fprintln(this.Logger, "Extracting..")
	err = e.Run("sudo", "tar", "-C", this.Application.LxcDir(), "-xzf", "/tmp/"+version+".tar.gz")
	if err != nil {
		return err
	}

	return nil
}

func (this *Deployment) syncNode(node *Node) error {
	logger := NewLogger(this.Logger, "["+node.Host+"] ")
	e := Executor{logger}

	// TODO: Maybe add fail check to clone operation.
	err := e.Run("ssh", DEFAULT_NODE_USERNAME+"@"+node.Host,
		"sudo", "/bin/bash", "-c",
		`"if ! [ -d '/var/lib/lxc/`+this.Application.Name+`' ]; then lxc-clone -B btrfs -s -o base-`+this.Application.BuildPack+` -n `+this.Application.Name+`; else echo 'app image already exists'; fi"`,
	)
	if err != nil {
		fmt.Fprintf(logger, "error cloning base container: %v\n", err)
		return err
	}
	// Rsync the application container over.
	err = e.Run("sudo", "rsync",
		"--recursive",
		"--links",
		"--perms",
		"--times",
		"--devices",
		"--specials",
		"--hard-links",
		"--acls",
		"--delete",
		"--xattrs",
		"--numeric-ids",
		"-e", "ssh -o 'StrictHostKeyChecking no' -o 'BatchMode yes'",
		this.Application.LxcDir()+"/rootfs/",
		"root@"+node.Host+":"+this.Application.LxcDir()+"/rootfs/",
	)
	if err != nil {
		return err
	}
	err = e.Run("rsync",
		"-azve", "ssh -o 'StrictHostKeyChecking no' -o 'BatchMode yes'",
		"/tmp/postdeploy.py", "/tmp/shutdown_container.py",
		"root@"+node.Host+":/tmp/",
	)
	if err != nil {
		return err
	}
	return nil
}

func (this *Deployment) startDyno(dynoGenerator *DynoGenerator, process string) (Dyno, error) {
	dyno := dynoGenerator.next(process)

	logger := NewLogger(this.Logger, "["+dyno.Host+"] ")
	e := Executor{logger}

	var err error
	done := make(chan bool)
	go func() {
		fmt.Fprint(logger, "Starting dyno")
		err = e.Run("ssh", DEFAULT_NODE_USERNAME+"@"+dyno.Host, "sudo", "/tmp/postdeploy.py", dyno.Container)
		done <- true
	}()
	select {
	case <-done: // implicitly break.
	case <-time.After(60 * time.Second):
		err = fmt.Errorf("Timed out for dyno host %v", dyno.Host)
	}
	return dyno, err
}

// Deploy the container to the hosts
func (this *Deployment) deploy() error {
	if len(this.Application.Processes) == 0 {
		return fmt.Errorf("No processes scaled up, adjust with `ps:scale proc=#` before deploying")
	}

	titleLogger := NewFormatter(this.Logger, GREEN)
	dimLogger := NewFormatter(this.Logger, DIM)

	e := Executor{dimLogger}

	err := ioutil.WriteFile("/tmp/postdeploy.py", []byte(POSTDEPLOY), 0777)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile("/tmp/shutdown_container.py", []byte(SHUTDOWN_CONTAINER), 0777)
	if err != nil {
		return err
	}

	// TODO: Remove this once the automatic resource reclaimer exists.
	// Build list of running dynos to be deactivated upon successful deployment.
	removeDynos := []Dyno{}
	for process, _ := range this.Application.Processes {
		runningDynos, err := this.Server.getRunningDynos(this.Application.Name, process)
		if err != nil {
			return err
		}
		removeDynos = append(removeDynos, runningDynos...)
	}

	type SyncResult struct {
		node *Node
		err  error
	}

	syncStep := make(chan SyncResult)
	for _, node := range this.Config.Nodes {
		go func(node *Node) {
			c := make(chan error, 1)
			go func() { c <- this.syncNode(node) }()
			go func() {
				time.Sleep(180 * time.Second)
				c <- fmt.Errorf("Sync operation to node '%v' timed out after 180 seconds", node.Host)
			}()
			// Block until c has something, at which point syncStep will be notified.
			syncStep <- SyncResult{node, <-c}
		}(node)
	}

	availableNodes := []*Node{}

	// Wait for all the syncs to finish or timeout, and collect available nodes.
	for _ = range this.Config.Nodes {
		syncResult := <-syncStep
		if syncResult.err == nil {
			availableNodes = append(availableNodes, syncResult.node)
		}
	}

	if len(availableNodes) == 0 {
		return fmt.Errorf("No available nodes. This is probably very bad for all apps running on this deployment system.")
	}

	// Now we've successfully sync'd and we have a list of nodes available to deploy to.
	addDynos := []Dyno{}

	dynoGenerator, err := this.Server.newDynoGenerator(availableNodes, this.Application.Name, this.Version)
	if err != nil {
		return err
	}

	type StartResult struct {
		dyno Dyno
		err  error
	}
	startedChannel := make(chan StartResult)

	startDynoWrapper := func(dynoGenerator *DynoGenerator, process string) {
		dyno, err := this.startDyno(dynoGenerator, process)
		startedChannel <- StartResult{dyno, err}
	}

	numDesiredDynos := 0

	// First deploy the changes and start the new dynos.
	for process, numDynos := range this.Application.Processes {
		for i := 0; i < numDynos; i++ {
			go startDynoWrapper(dynoGenerator, process)
			numDesiredDynos++
		}
	}

	timeout := time.After(180 * time.Second)
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
			return fmt.Errorf("Start operation timed out after 3 minutes")
		}
	}

	err = this.Server.SyncLoadBalancers(e, addDynos, removeDynos)
	if err != nil {
		return err
	}

	// TODO: Maybe add delay to let things finish?

	// Next destroy the old dynos.
	/*for _, removeDyno := range removeDynos {
		fmt.Fprintf(titleLogger, "Destroying old dyno %v\n", removeDyno.Container)
		e.Run("ssh", DEFAULT_NODE_USERNAME+"@"+removeDyno.Host, "sudo", "/tmp/shutdown_container.py", removeDyno.Container)
	}*/

	releases, err := getReleases(this.Application.Name)
	if err != nil {
		return err
	}
	// Prepend the release (releases are in descending order)
	releases = append([]Release{{
		Version:  this.Version,
		Revision: this.Revision,
		Date:     time.Now(),
		Config:   this.Application.Environment,
	}}, releases...)
	// Only keep around the latest 15 (older ones are still in S3)
	if len(releases) > 15 {
		releases = releases[:15]
	}
	err = setReleases(this.Application.Name, releases)
	if err != nil {
		return err
	}

	return nil
}

func (this *Deployment) Deploy() error {
	var err error

	// Cleanup any hanging chads upon error.
	defer func() {
		if err != nil {
			this.Server.destroyContainer(Executor{this.Logger}, this.Application.Name+DYNO_DELIMITER+this.Version)
			this.Server.WithPersistentApplication(this.Application.Name, func(app *Application, cfg *Config) error {
				// If the version hasn't been messed with since we incremented it, go ahead and decrement it because
				// this deploy has failed.
				if app.LastDeploy == this.Version {
					prev, err := app.CalcPreviousVersion()
					if err != nil {
						return err
					}
					app.LastDeploy = prev
				}
				return nil
			})
		}
	}()

	err = this.createContainer()
	if err != nil {
		return err
	}

	err = this.build()
	if err != nil {
		return err
	}

	err = this.archive()
	if err != nil {
		return err
	}

	err = this.deploy()
	if err != nil {
		return err
	}

	return nil
}

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

func (this *Server) Deploy(conn net.Conn, applicationName, revision string) error {
	fmt.Fprintf(this.getLogger(conn), "Deploying revision %v\n", revision)
	deployLock.start()
	defer deployLock.finish()

	return this.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		// Bump version.
		app, cfg, err := this.IncrementAppVersion(app)
		if err != nil {
			return err
		}
		deployment := &Deployment{
			Server:      this,
			Logger:      NewTimeLogger(NewMessageLogger(conn)),
			Config:      cfg,
			Application: app,
			Revision:    revision,
			Version:     app.LastDeploy,
		}
		err = deployment.Deploy()
		if err != nil {
			return err
		}
		app.LastDeploy = deployment.Version
		return nil
	})
}

func (this *Server) Redeploy(conn net.Conn, applicationName string) error {
	deployLock.start()
	defer deployLock.finish()

	return this.WithApplication(applicationName, func(app *Application, cfg *Config) error {
		// Nothing to redeploy.
		if app.LastDeploy == "" {
			fmt.Fprintf(this.getLogger(conn), "Redeploy is not going to happen because this app has not yet had a first deploy\n")
			return nil
		}
		previousVersion := app.LastDeploy
		// Bump version.
		app, cfg, err := this.IncrementAppVersion(app)
		if err != nil {
			return err
		}
		deployment := &Deployment{
			Server:      this,
			Logger:      NewTimeLogger(NewMessageLogger(conn)),
			Config:      cfg,
			Application: app,
			Version:     app.LastDeploy,
		}
		// Find the release that corresponds with the latest deploy
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
			err = this.WithPersistentApplication(applicationName, func(app *Application, cfg *Config) error {
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
