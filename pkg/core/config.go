package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gigawattio/errorlib"
	"github.com/gigawattio/oslib"
	lslog "github.com/jaytaylor/logserver"
	log "github.com/sirupsen/logrus"
)

const (
	APP_DIR                            = "/app"
	ENV_DIR                            = APP_DIR + "/env"
	LXC_DIR                            = "/var/lib/lxd/storage-pools/tank/containers/" // "/var/snap/lxd/common/lxd/containers" // "/var/lib/lxd/storage-pools/tank/containers" // "/tank/lxc" // "/var/lib/lxc"
	LXC_BIN                            = "/snap/bin/lxc"
	ZFS_CONTAINER_MOUNT                = "tank/containers"
	DIRECTORY                          = "/etc/shipbuilder"
	BINARY                             = "shipbuilder"
	EXE                                = "/usr/bin/" + BINARY
	CONFIG                             = DIRECTORY + "/config.json"
	GIT_DIRECTORY                      = "/git"
	DEFAULT_NODE_USERNAME              = "ubuntu"
	VERSION                            = "0.1.4"
	NODE_SYNC_TIMEOUT_SECONDS          = 180
	DYNO_START_TIMEOUT_SECONDS         = 120
	LOAD_BALANCER_SYNC_TIMEOUT_SECONDS = 45
	DEPLOY_TIMEOUT_SECONDS             = 240
	STATUS_MONITOR_INTERVAL_SECONDS    = 15
	DEFAULT_SSH_PARAMETERS             = "-o StrictHostKeyChecking=no -o BatchMode=yes -o ConnectTimeout=30" // NB: Notice 30s connect timeout.
)

const (
	bashHAProxyReloadCommand = `/bin/bash -c 'set -o errexit ; set -o pipefail ; if [ "$(sudo systemctl status haproxy | grep --only-matching "Active: [^ ]\+" | cut -d " " -f 2)" = "inactive" ] ; then sudo systemctl start haproxy ; else sudo systemctl reload haproxy ; fi'`
)

var defaultSshParametersList = strings.Split(DEFAULT_SSH_PARAMETERS, " ")

// Global configuration.
var (
	// TODO: Remove "Default" prefix from all these vars.

	// NB: Bools are not settable via ldflags.

	// NB: LDFLAGS can be specified by compiling with `-ldflags '-X main.DefaultSSHHost=.. ...'`.
	DefaultHAProxyEnableNonstandardPorts string
	DefaultHAProxyStats                  string
	DefaultHAProxyCredentials            string
	DefaultAWSKey                        string
	DefaultAWSSecret                     string
	DefaultAWSRegion                     string
	DefaultS3BucketName                  string
	DefaultSSHHost                       string
	DefaultSSHKey                        string
	DefaultLXCFS                         string
	DefaultZFSPool                       string
)

var (
	configLock           sync.Mutex
	syncLoadBalancerLock sync.Mutex
)

var (
	ntpServers     = "0.pool.ntp.org 1.pool.ntp.org time.apple.com time.windows.com"
	ntpSyncCommand = "sudo systemctl stop ntp && sudo /usr/sbin/ntpdate " + ntpServers + " && sudo systemctl start ntp"
)

type Application struct {
	Name          string
	Domains       []string
	BuildPack     string
	Environment   map[string]string
	Processes     map[string]int
	LastDeploy    string
	Maintenance   bool
	Drains        []string
	SshPrivateKey *string
}

type Node struct {
	Host string
}

type Config struct {
	LoadBalancers []string
	Nodes         []*Node
	Port          int
	GitRoot       string
	LxcRoot       string
	Applications  []*Application
}

func (app *Application) LxcDir() string {
	panic("LxcDir is *DECPRECATED*, DISCONTINUE ALL USE")
}
func (app *Application) RootFsDir() string {
	panic("RootFsDir is *DECPRECATED*, DISCONTINUE ALL USE")
}
func (app *Application) AppDir() string {
	panic("AppDir is *DECPRECATED*, DISCONTINUE ALL USE")
}
func (app *Application) SrcDir() string {
	panic("SrcDir is *DECPRECATED*, DISCONTINUE ALL USE")
}
func (app *Application) BareGitDir() string {
	return GIT_DIRECTORY + "/" + app.Name
}
func (app *Application) LocalAppDir() string {
	return APP_DIR
}
func (app *Application) LocalSrcDir() string {
	return APP_DIR + "/src"
}
func (app *Application) SshDir() string {
	panic("SshDir is *DECPRECATED*, DISCONTINUE ALL USE")
}
func (app *Application) SshPrivateKeyFilePath() string {
	panic("SshPrivateKeyFilePath is *DECPRECATED*, DISCONTINUE ALL USE")
}
func (app *Application) BaseContainerName() string {
	return "base-" + app.BuildPack
}
func (app *Application) GitDir() string {
	panic("GitDir is *DECPRECATED*, DISCONTINUE ALL USE")
}
func (app *Application) LastDeployNumber() (int, error) {
	return strconv.Atoi(strings.TrimPrefix(app.LastDeploy, "v"))
}

// Get total requested number of Dynos (based on Processes).
func (app *Application) TotalRequestedDynos() int {
	n := 0
	for _, value := range app.Processes {
		if value > 0 { // Ensure negative values are never added.
			n += value
		}
	}
	return n
}

// Get any valid domain for the app.  HAProxy will use this to formulate checks which are maximally valid, compliant and compatible.
// Note: Not a pointer because this needs to be available for invocation from inside templates.
// Also see: http://stackoverflow.com/questions/10200178/call-a-method-from-a-go-template
func (app *Application) FirstDomain() string {
	if len(app.Domains) > 0 {
		return app.Domains[0]
	} else {
		return "example.com"
	}
}

// Entire maintenance page URL (e.g. "http://example.com/static/maintenance.html").
func (app *Application) MaintenancePageUrl() string {
	maintenanceUrl, ok := app.Environment["MAINTENANCE_PAGE_URL"]
	if ok {
		return maintenanceUrl
	}
	// Fall through to searching for a universal maintenance page URL in an environment variable, and
	// defaulting to a potentially useful page.
	return ConfigFromEnv("SB_DEFAULT_MAINTENANCE_URL", "http://www.downforeveryoneorjustme.com/"+app.FirstDomain())
}

// Maintenance page URL path. (e.g. "/static/maintenance.html").
func (app *Application) MaintenancePageFullPath() string {
	maintenanceUrl := app.MaintenancePageUrl()
	if len(maintenanceUrl) < 3 {
		log.Errorf("Application.MaintenancePageFullPath :: url too short: '%v'\n", maintenanceUrl)
		return maintenanceUrl
	}
	u, err := url.Parse(maintenanceUrl)
	if err != nil {
		log.Errorf("Application.MaintenancePageFullPath :: %v: '%v'\n", err, maintenanceUrl)
		return "/"
	}
	return u.Path
}

// Maintenance page URL without the document name (e.g. "/static/).
func (app *Application) MaintenancePageBasePath() string {
	path := app.MaintenancePageFullPath()
	i := strings.LastIndex(path, "/")
	if i == -1 || len(path) == 1 {
		return path
	}
	return path[0:i]
}

// Maintenance page URL domain-name. (e.g. "example.com").
func (app *Application) MaintenancePageDomain() string {
	maintenanceUrl := app.MaintenancePageUrl()
	if len(maintenanceUrl) < 3 {
		log.Errorf("Application.MaintenancePageDomain :: url too short: '%v'\n", maintenanceUrl)
		return maintenanceUrl
	}
	u, err := url.Parse(maintenanceUrl)
	if err != nil {
		log.Errorf("Application.MaintenancePageDomain :: %v: '%v'\n", err, maintenanceUrl)
		return "domain-parse-failed"
	}
	return u.Host
}

func (app *Application) NextVersion() (string, error) {
	if app.LastDeploy == "" {
		return "v1", nil
	}
	current, err := strconv.Atoi(app.LastDeploy[1:])
	if err != nil {
		return "", err
	}
	return "v" + strconv.Itoa(current+1), nil
}
func (app *Application) CalcPreviousVersion() (string, error) {
	if app.LastDeploy == "" || app.LastDeploy == "v1" {
		return "", nil
	}
	v, err := strconv.Atoi(app.LastDeploy[1:])
	if err != nil {
		return "", err
	}
	return "v" + strconv.Itoa(v-1), nil
}
func (app *Application) CreateBaseContainerIfMissing(e *Executor) error {
	exists, err := e.ContainerExists(app.Name)
	if err != nil {
		return err
	}
	if !exists {
		return e.CloneContainer(app.BaseContainerName(), app.Name)
	}
	return nil
}

func (server *Server) IncrementAppVersion(app *Application) (*Application, *Config, error) {
	var updatedApp *Application
	var updatedCfg *Config

	err := server.WithPersistentApplication(app.Name, func(app *Application, cfg *Config) error {
		nextVersion, err := app.NextVersion()
		log.Infof("NEXT VERSION OF %v -> %v\n", app.Name, nextVersion)
		if err != nil {
			return err
		}
		app.LastDeploy = nextVersion
		updatedApp = app
		updatedCfg = cfg
		return nil
	})
	return updatedApp, updatedCfg, err
}

// Only to be invoked by safe locking getters/setters, never externally!!!
func (server *Server) getConfig(lock bool) (*Config, error) {
	if lock {
		configLock.Lock()
		defer configLock.Unlock()
	}

	var config Config

	// Create config if it doesn't exist.
	if exists, err := oslib.PathExists(server.ConfigFile); err != nil {
		return nil, fmt.Errorf("checking if config file at location %q exists: %s", server.ConfigFile, err)
	} else if !exists {
		if err := ioutil.WriteFile(server.ConfigFile, []byte("{}"), os.FileMode(int(0600))); err != nil {
			return nil, fmt.Errorf("writing to new config file at location %q: %s", server.ConfigFile, err)
		}
		log.Infof("Created new shipbuilder config file at location %q", server.ConfigFile)
	}

	f, err := os.Open(server.ConfigFile)
	if err == nil {
		defer f.Close()
		err = json.NewDecoder(f).Decode(&config)
		if err != nil {
			return nil, err
		}
	}

	if config.Applications == nil {
		config.Applications = []*Application{}
	}

	if config.LoadBalancers == nil {
		config.LoadBalancers = []string{}
	}
	if config.Nodes == nil {
		config.Nodes = []*Node{}
	}

	return &config, nil
}

// IMPORTANT: Only to be invoked by `WithPersistentConfig`.
// TODO: Check for ignored errors.
func (server *Server) writeConfig(config *Config) error {
	f, err := os.Create(server.ConfigFile)
	if err != nil {
		return err
	}
	defer f.Close()
	err = json.NewEncoder(f).Encode(config)
	if err != nil {
		return err
	}
	return nil
}

// Obtains the config lock, then applies the passed function which can mutate the config, then writes out the changes.
func (server *Server) WithPersistentConfig(fn func(*Config) error) error {
	configLock.Lock()
	defer configLock.Unlock()

	cfg, err := server.getConfig(false)
	if err != nil {
		return err
	}
	if err := fn(cfg); err != nil {
		return err
	}
	if err := server.writeConfig(cfg); err != nil {
		return err
	}
	return nil
}

// Reads the config and invokes the passed function with it.  Does not store any config changes.
func (server *Server) WithConfig(fn func(*Config) error) error {
	cfg, err := server.getConfig(true)
	if err != nil {
		return err
	}
	if err := fn(cfg); err != nil {
		return err
	}
	return nil
}

func (server *Server) WithPersistentApplication(name string, fn func(*Application, *Config) error) error {
	return server.WithPersistentConfig(func(cfg *Config) error {
		for _, app := range cfg.Applications {
			if app.Name == name {
				return fn(app, cfg)
			}
		}
		return fmt.Errorf("unknown application: %v", name)
	})
}

func (server *Server) WithApplication(name string, fn func(*Application, *Config) error) error {
	return server.WithConfig(func(cfg *Config) error {
		for _, app := range cfg.Applications {
			if app.Name == name {
				return fn(app, cfg)
			}
		}
		return fmt.Errorf("unknown application: %v", name)
	})
}

// ResolveLogServerIpAndPortr eturns the ShipBuilder log server ip:port to send
// HAProxy UDP logs to.  Autmatically takes care of transforming ssh hostname
// into just a hostname.
func (server *Server) ResolveLogServerIpAndPort() (string, error) {
	var (
		hostname = DefaultSSHHost[int(math.Max(float64(strings.Index(DefaultSSHHost, "@")+1), 0)):]
		port     = fmt.Sprint(lslog.DefaultPort)
	)

	if server.LogServerListenAddr != "" && !strings.HasPrefix(server.LogServerListenAddr, ":") {
		pieces := strings.Split(server.LogServerListenAddr, ":")
		hostname = pieces[0]
		if len(pieces) > 1 && len(pieces[1]) > 0 {
			port = pieces[1]
		}
	}

	ipAddr, err := net.ResolveIPAddr("ip", hostname)
	if err != nil {
		return "", err
	}

	ip := ipAddr.IP.String()
	return ip + ":" + port, nil
}

// TODO: Check for ignored errors.
func (server *Server) SyncLoadBalancers(e *Executor, addDynos []Dyno, removeDynos []Dyno) error {
	syncLoadBalancerLock.Lock()
	defer syncLoadBalancerLock.Unlock()

	cfg, err := server.getConfig(true)
	if err != nil {
		return err
	}

	logServerIpAndPort, err := server.ResolveLogServerIpAndPort()
	if err != nil {
		return err
	}

	lbSpec := &LBSpec{
		LogServerIpAndPort:  logServerIpAndPort,
		Applications:        []*LBApp{},
		LoadBalancers:       cfg.LoadBalancers,
		HaProxyStatsEnabled: isTruthy(DefaultHAProxyStats),
		HaProxyCredentials:  HaProxyCredentials(),
	}

	for _, app := range cfg.Applications {
		a := &LBApp{
			Name:                    app.Name,
			Domains:                 app.Domains,
			FirstDomain:             app.FirstDomain(),
			Servers:                 []*LBAppDyno{},
			Maintenance:             app.Maintenance,
			MaintenancePageFullPath: app.MaintenancePageFullPath(),
			MaintenancePageBasePath: app.MaintenancePageBasePath(),
			MaintenancePageDomain:   app.MaintenancePageDomain(),
			SSL:           !isTruthy(app.Environment["SB_DISABLE_SSL"]),
			SSLForwarding: !isTruthy(app.Environment["SB_DISABLE_SSL_FORWARDING"]),
		}
		for proc, _ := range app.Processes {
			if proc == "web" {
				// Find and don't add `removeDynos`.
				runningDynos, err := server.GetRunningDynos(app.Name, proc)
				if err != nil {
					return err
				}
				for _, dyno := range runningDynos {
					found := false
					for _, remove := range removeDynos {
						if remove == dyno {
							found = true
							break
						}
					}
					if found {
						continue
					}
					port, err := strconv.Atoi(dyno.Port)
					if err != nil {
						return err
					}
					a.Servers = append(a.Servers, &LBAppDyno{
						Host: dyno.Host,
						Port: port,
					})
				}
				// Add `addDynos` if type is "web" and it matches the current application and process.
				for _, addDyno := range addDynos {
					if addDyno.Application == app.Name && addDyno.Process == proc {
						port, err := strconv.Atoi(addDyno.Port)
						if err != nil {
							return err
						}

						candidateServer := &LBAppDyno{
							Host: addDyno.Host,
							Port: port,
						}

						// Check if the server has already been added.
						alreadyAdded := false
						for _, existingServer := range a.Servers {
							if existingServer == candidateServer {
								alreadyAdded = true
							}
						}
						if !alreadyAdded {
							a.Servers = append(a.Servers, candidateServer)
						}
					}
				}
			}
		}
		lbSpec.Applications = append(lbSpec.Applications, a)
	}

	// Save it to the load balancer
	hapFileLoc := "/tmp/haproxy.cfg"
	f, err := os.OpenFile(hapFileLoc, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(int(0666)))
	if err != nil {
		return err
	}

	defer func() {
		if rmErr := os.Remove(hapFileLoc); rmErr != nil {
			log.Warnf("Unexpected problem during cleanup removal of %q: %s", hapFileLoc, rmErr)
		}
	}()

	err = HAPROXY_CONFIG.Execute(f, lbSpec)

	if closeErr := f.Close(); closeErr != nil {
		log.Warnf("Unexpected problem closing %q: %s", hapFileLoc, closeErr)
	}

	if err != nil {
		return err
	}

	type LBSyncResult struct {
		lbHost string
		err    error
	}

	syncChannel := make(chan LBSyncResult)
	for _, host := range cfg.LoadBalancers {
		go func(host string) {
			c := make(chan error, 1)
			go func() {
				err := e.Run("rsync",
					"-azve", "ssh "+DEFAULT_SSH_PARAMETERS,
					"/tmp/haproxy.cfg", "root@"+host+":/etc/haproxy/",
				)
				if err != nil {
					c <- err
					return
				}
				if err = e.Run("ssh", DEFAULT_NODE_USERNAME+"@"+host, bashHAProxyReloadCommand); err != nil {
					c <- err
					return
				}
				c <- nil
			}()
			go func() {
				time.Sleep(LOAD_BALANCER_SYNC_TIMEOUT_SECONDS * time.Second)
				c <- fmt.Errorf("LB sync operation to %q timed out after %v seconds", host, LOAD_BALANCER_SYNC_TIMEOUT_SECONDS)
			}()
			// Block until chan has something, at which point syncChannel will be notified.
			syncChannel <- LBSyncResult{host, <-c}
		}(host)
	}

	nLoadBalancers := len(cfg.LoadBalancers)
	errors := []error{}
	for i := 1; i <= nLoadBalancers; i++ {
		syncResult := <-syncChannel
		if syncResult.err != nil {
			errors = append(errors, syncResult.err)
		}
		fmt.Fprintf(e.Logger, "%v/%v load-balancer sync finished (%v succeeded, %v failed, %v outstanding)\n", i, nLoadBalancers, i-len(errors), len(errors), nLoadBalancers-i)
	}

	// If all LB updates failed, abort with error.
	if nLoadBalancers > 0 && len(errors) == nLoadBalancers {
		err = fmt.Errorf("error: all load-balancer updates failed: %v", errors)
		fmt.Fprintf(e.Logger, "%s\n", err)
		return err
	}

	// Uddate `currentLoadBalancerConfig` with updated HAProxy configuration.
	cfgBuffer := bytes.Buffer{}
	if err = HAPROXY_CONFIG.Execute(&cfgBuffer, lbSpec); err != nil {
		return err
	}
	server.currentLoadBalancerConfig = cfgBuffer.String()

	return nil
}

// The first time this method is invoked the current config will read from a load-balancer, if one is available.
// Subsequent invocations will use the current version.
// After a deployment, the `SyncLoadBalancers` method automatically updates `currentLoadBalancerConfig`.
func (server *Server) GetActiveLoadBalancerConfig() (string, error) {
	if len(server.currentLoadBalancerConfig) == 0 {
		cfg, err := server.getConfig(true)
		if err != nil {
			return server.currentLoadBalancerConfig, err
		}
		if len(cfg.LoadBalancers) == 0 {
			return server.currentLoadBalancerConfig, fmt.Errorf("There are currently no load-balancers configured to pull LB config from")
		}

		syncLoadBalancerLock.Lock()
		defer syncLoadBalancerLock.Unlock()
		server.currentLoadBalancerConfig, err = RemoteCommand(cfg.LoadBalancers[0], "sudo cat /etc/haproxy/haproxy.cfg")
		if err != nil {
			return server.currentLoadBalancerConfig, err
		}
	} else {
		syncLoadBalancerLock.Lock()
		defer syncLoadBalancerLock.Unlock()
	}
	return server.currentLoadBalancerConfig, nil
}

// TODO: Replace with gigawattio/oslib.
func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// TODO: This should probably be replaced with os.Mkdirs.
func MkdirIfNotExists(path string, perm os.FileMode) error {
	exists, err := PathExists(path)
	if err != nil {
		return err
	}
	if !exists {
		err = os.Mkdir(path, perm)
		if err != nil {
			return err
		}
	}
	return nil
}

func ConfigFromEnv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		value = defaultValue
	}
	return value
}

// ValidateConfig validates global configuration.
func ValidateConfig() error {
	errs := []error{}

	if DefaultAWSRegion == "" {
		errs = append(errs, errors.New("AWS region cannot be empty"))
	}

	if err := errorlib.Merge(errs); err != nil {
		return err
	}
	return nil
}

func GetSystemIp() string {
	name, err := os.Hostname()
	if err != nil {
		log.Errorf("GetSystemIp: Oops-1: %v", err)
	} else {
		addrs, err := net.LookupHost(name)
		if err != nil {
			fmt.Printf("error: GetSystemIp: Oops-2: %v\n", err)
		} else if len(addrs) > 0 {
			return addrs[0]
		}
	}
	fmt.Printf("warning: GetSystemIp: system address discovery failed, defaulting to '127.0.0.1'\n")
	return "127.0.0.1"
}

func HaProxyCredentials() string {
	maybeValue := os.Getenv("SB_HAPROXY_CREDENTIALS")
	if len(maybeValue) > 2 && strings.Contains(maybeValue, ":") {
		return maybeValue
	}
	if DefaultHAProxyCredentials != "" && len(DefaultHAProxyCredentials) > 2 && strings.Contains(DefaultHAProxyCredentials, ":") {
		return DefaultHAProxyCredentials
	}
	return "admin:password"
}
