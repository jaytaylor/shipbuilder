package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"launchpad.net/goamz/aws"
)

type (
	Application struct {
		Name        string
		Domains     []string
		BuildPack   string
		Environment map[string]string
		Processes   map[string]int
		LastDeploy  string
		Maintenance bool
		Drains      []string
	}
	Node struct {
		Host string
	}
	Config struct {
		LoadBalancers []string
		Nodes         []*Node
		Port          int
		GitRoot       string
		LxcRoot       string
		Applications  []*Application
	}
)

const (
	APP_DIR                    = "/app"
	LXC_DIR                    = "/var/lib/lxc"
	DIRECTORY                  = "/mnt/build"
	BINARY                     = "shipbuilder"
	EXE                        = DIRECTORY + "/" + BINARY
	CONFIG                     = DIRECTORY + "/config.json"
	GIT_DIRECTORY              = "/git"
	DEFAULT_NODE_USERNAME      = "ubuntu"
	VERSION                    = "0.1.0"
	NODE_SYNC_TIMEOUT_SECONDS  = 180
	DYNO_START_TIMEOUT_SECONDS = 120
	DEPLOY_TIMEOUT_SECONDS     = 240
)

// LDFLAGS can be specified by compiling with `-ldflags '-X main.defaultSshHost=.. ...'`.
var (
	build                     string
	defaultHaProxyStats       bool
	defaultHaProxyCredentials string
	defaultAwsKey             string
	defaultAwsSecret          string
	defaultAwsRegion          string
	defaultS3BucketName       string
	defaultSshHost            string
	defaultSshKey             string
	defaultLxcFs              string
)

// Global configuration.
var (
	sshHost      = OverridableByEnv("SB_SSH_HOST", defaultSshHost)
	sshKey       = OverridableByEnv("SB_SSH_KEY", defaultSshKey)
	awsKey       = OverridableByEnv("SB_AWS_KEY", defaultAwsKey)
	awsSecret    = OverridableByEnv("SB_AWS_SECRET", defaultAwsSecret)
	awsRegion    = getAwsRegion("SB_AWS_REGION", defaultAwsRegion)
	s3BucketName = OverridableByEnv("SB_S3_BUCKET", defaultS3BucketName)
	lxcFs        = OverridableByEnv("LXC_FS", defaultLxcFs)
)

var (
	configLock           sync.Mutex
	syncLoadBalancerLock sync.Mutex
)

func (this *Application) LxcDir() string {
	return LXC_DIR + "/" + this.Name
}
func (this *Application) RootFsDir() string {
	return LXC_DIR + "/" + this.Name + "/rootfs"
}
func (this *Application) AppDir() string {
	return this.RootFsDir() + APP_DIR
}
func (this *Application) SrcDir() string {
	return this.AppDir() + "/src"
}
func (this *Application) GitDir() string {
	return GIT_DIRECTORY + "/" + this.Name
}

// Entire maintenance page URL (e.g. "http://example.com/static/maintenance.html").
func (this *Application) MaintenancePageUrl() string {
	maintenanceUrl, ok := this.Environment["MAINTENANCE_PAGE_URL"]
	if ok {
		return maintenanceUrl
	}
	// Fall through to searching for a universal maintenance page URL in an environment variable, and
	// defaulting to a potentially useful page.
	var firstDomain string
	if len(this.Domains) > 0 {
		firstDomain = this.Domains[0]
	} else {
		firstDomain = "example.com"
	}
	return ConfigFromEnv("SB_DEFAULT_MAINTENANCE_URL", "http://www.downforeveryoneorjustme.com/"+firstDomain)
}

// Maintenance page URL path. (e.g. "/static/maintenance.html").
func (this *Application) MaintenancePageFullPath() string {
	maintenanceUrl := this.MaintenancePageUrl()
	if len(maintenanceUrl) < 3 {
		fmt.Printf("error :: Application.MaintenancePageFullPath :: url too short: '%v'\n", maintenanceUrl)
		return maintenanceUrl
	}
	u, err := url.Parse(maintenanceUrl)
	if err != nil {
		fmt.Printf("error :: Application.MaintenancePageFullPath :: %v: '%v'\n", err, maintenanceUrl)
		return "/"
	}
	return u.Path
}

// Maintenance page URL without the document name (e.g. "/static/).
func (this *Application) MaintenancePageBasePath() string {
	path := this.MaintenancePageFullPath()
	i := strings.LastIndex(path, "/")
	if i == -1 || len(path) == 1 {
		return path
	}
	return path[0:i]
}

// Maintenance page URL domain-name. (e.g. "example.com").
func (this *Application) MaintenancePageDomain() string {
	maintenanceUrl := this.MaintenancePageUrl()
	if len(maintenanceUrl) < 3 {
		fmt.Printf("error :: Application.MaintenancePageDomain :: url too short: '%v'\n", maintenanceUrl)
		return maintenanceUrl
	}
	u, err := url.Parse(maintenanceUrl)
	if err != nil {
		fmt.Printf("error :: Application.MaintenancePageDomain :: %v: '%v'\n", err, maintenanceUrl)
		return "domain-parse-failed"
	}
	return u.Host
}

func (this *Application) NextVersion() (string, error) {
	if this.LastDeploy == "" {
		return "v1", nil
	}
	current, err := strconv.Atoi(this.LastDeploy[1:])
	if err != nil {
		return "", err
	}
	return "v" + strconv.Itoa(current+1), nil
}
func (this *Application) CalcPreviousVersion() (string, error) {
	if this.LastDeploy == "" || this.LastDeploy == "v1" {
		return "", nil
	}
	v, err := strconv.Atoi(this.LastDeploy[1:])
	if err != nil {
		return "", err
	}
	return "v" + strconv.Itoa(v-1), nil
}

func (this *Server) IncrementAppVersion(app *Application) (*Application, *Config, error) {
	var updatedApp *Application
	var updatedCfg *Config

	err := this.WithPersistentApplication(app.Name, func(app *Application, cfg *Config) error {
		nextVersion, err := app.NextVersion()
		fmt.Printf("NEXT VERSION OF %v -> %v\n", app.Name, nextVersion)
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
func (this *Server) getConfig(lock bool) (*Config, error) {
	if lock {
		configLock.Lock()
		defer configLock.Unlock()
	}

	var config Config

	f, err := os.Open(CONFIG)
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

// // This function exists to adhere to D.R.Y. WRT config updates.
// // Only to be invoked by safe locking getters/setters, never externally!!!
// func (this *Server) saveConfig(lock bool, fn func(*Config) error) error {
// 	config, err := this.getConfig(lock)
// 	if err != nil {
// 		return err
// 	}

// 	err = fn(config)
// 	if err != nil {
// 		return err
// 	}

// 	if lock {
// 		configLock.Lock()
// 		defer configLock.Unlock()
// 	}

// 	f, err := os.Create(CONFIG)
// 	if err != nil {
// 		return err
// 	}
// 	defer f.Close()
// 	err = json.NewEncoder(f).Encode(config)
// 	if err != nil {
// 		return err
// 	}
// 	return nil
// }

// // Only to be invoked by safe locking getters/setters, never externally!!!
// func (this *Server) saveApplicationConfig(lock bool, app *Application) error {
// 	return saveConfig(lock, func(config *Config) error {
// 		// Update or append app.
// 		found := false
// 		for i, a := range config.Applications {
// 			if a.Name == app.Name {
// 				found = true
// 				config.Applications[i] = app
// 			}
// 		}
// 		if !found {
// 			config.Applications = append(config.Applications, app)
// 		}
// 		return nil
// 	})
// }

// // Only to be invoked by safe locking getters/setters, never externally!!!
// func (this *Server) saveSystemConfig(lock bool, sys *System) error {
// 	return saveConfig(lock, func(config *Config) error {
// 		// Update System configuration.
// 		config.System = sys
// 		return nil
// 	})
// }

// func (this *Server) withConfig(fn func(*Config) error) error {
// 	cfg, err := this.getConfig()
// 	if err != nil {
// 		return err
// 	}
// 	err = fn(cfg)
// 	if err != nil {
// 		return err
// 	}
// 	return this.saveConfig(cfg)
// }

func (this *Server) writeConfig(config *Config) error {
	f, err := os.Create(CONFIG)
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
func (this *Server) WithPersistentConfig(fn func(*Config) error) error {
	configLock.Lock()
	defer configLock.Unlock()

	cfg, err := this.getConfig(false)
	if err != nil {
		return err
	}
	err = fn(cfg)
	if err != nil {
		return err
	}
	err = this.writeConfig(cfg)
	if err != nil {
		return err
	}
	return nil
}

// Reads the config and invokes the passed function with it.  Does not store any config changes.
func (this *Server) WithConfig(fn func(*Config) error) error {
	cfg, err := this.getConfig(true)
	if err != nil {
		return err
	}
	err = fn(cfg)
	if err != nil {
		return err
	}
	return nil
}

func (this *Server) WithPersistentApplication(name string, fn func(*Application, *Config) error) error {
	return this.WithPersistentConfig(func(cfg *Config) error {
		for _, app := range cfg.Applications {
			if app.Name == name {
				return fn(app, cfg)
			}
		}
		return fmt.Errorf("Unknown application: %v", name)
	})
}

func (this *Server) WithApplication(name string, fn func(*Application, *Config) error) error {
	return this.WithConfig(func(cfg *Config) error {
		for _, app := range cfg.Applications {
			if app.Name == name {
				return fn(app, cfg)
			}
		}
		return fmt.Errorf("Unknown application: %v", name)
	})
}

func (this *Server) SyncLoadBalancers(e Executor, addDynos []Dyno, removeDynos []Dyno) error {
	syncLoadBalancerLock.Lock()
	defer syncLoadBalancerLock.Unlock()

	cfg, err := this.getConfig(true)
	if err != nil {
		return err
	}

	type Server struct {
		Host string
		Port int
	}
	type App struct {
		Name                    string
		Domains                 []string
		Servers                 []*Server
		Maintenance             bool
		MaintenancePageFullPath string
		MaintenancePageBasePath string
		MaintenancePageDomain   string
		HaProxyStatsEnabled     bool
		HaProxyCredentials      string
	}
	type Lb struct {
		Applications []*App
	}
	lb := &Lb{[]*App{}}

	for _, app := range cfg.Applications {
		a := &App{
			Name:                    app.Name,
			Domains:                 app.Domains,
			Servers:                 []*Server{},
			Maintenance:             app.Maintenance,
			MaintenancePageFullPath: app.MaintenancePageFullPath(),
			MaintenancePageBasePath: app.MaintenancePageBasePath(),
			MaintenancePageDomain:   app.MaintenancePageDomain(),
			HaProxyStatsEnabled:     HaProxyStatsEnabled(),
			HaProxyCredentials:      HaProxyCredentials(),
		}
		for proc, _ := range app.Processes {
			if proc == "web" {
				// Find and don't add `removeDynos`.
				runningDynos, err := this.getRunningDynos(app.Name, proc)
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
					a.Servers = append(a.Servers, &Server{
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

						candidateServer := &Server{
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
		lb.Applications = append(lb.Applications, a)
	}

	// Save it to the load balancer
	f, err := os.OpenFile("/tmp/haproxy.cfg", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0x666)
	if err != nil {
		return err
	}
	defer os.Remove("/tmp/haproxy.cfg")
	err = HAPROXY_CONFIG.Execute(f, lb)
	f.Close()
	if err != nil {
		return err
	}

	for _, lb := range cfg.LoadBalancers {
		err := e.Run("rsync",
			"-azve", "ssh -o 'StrictHostKeyChecking no' -o 'BatchMode yes'",
			"/tmp/haproxy.cfg", "root@"+lb+":/etc/haproxy/",
		)
		if err != nil {
			return err
		}
		err = e.Run("ssh", DEFAULT_NODE_USERNAME+"@"+lb,
			`sudo /bin/bash -c 'if [ "$(sudo service haproxy status)" = "haproxy not running." ]; then sudo service haproxy start; else sudo service haproxy reload; fi'`,
		)
		if err != nil {
			return err
		}
	}

	time.Sleep(time.Second * 1)

	return nil
}

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

func OverridableByEnv(key string, ldflagsValue string) string {
	envValue := os.Getenv(key)
	if len(envValue) > 0 {
		fmt.Printf("info: environmental override detected for %v: %v\n", key, envValue)
		return envValue
	}
	if len(ldflagsValue) == 0 {
		fmt.Printf("fatal: missing required configuration value for '%v'\n", key)
		os.Exit(1)
	}
	//fmt.Printf("info: ldflags value detected for %v: %v\n", key, ldflagsValue)
	return ldflagsValue
}

// Validate that the configured key exists in the provided options.
func getAwsRegion(key string, ldflagsValue string) aws.Region {
	regionKey := OverridableByEnv(key, ldflagsValue)
	region, ok := aws.Regions[regionKey]
	if !ok {
		validRegions := []string{}
		for _, r := range aws.Regions {
			validRegions = append(validRegions, r.Name)
		}
		fmt.Printf("fatal: invalid option '%v' for parameter '%v', acceptable values are: %v\n", regionKey, key, validRegions)
		os.Exit(1)
	}
	return region
}

func GetSystemIp() string {
	name, err := os.Hostname()
	if err != nil {
		fmt.Printf("error: GetSystemIp: Oops-1: %v\n", err)
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

func HaProxyStatsEnabled() bool {
	maybeValue := strings.TrimSpace(strings.ToLower(os.Getenv("SB_HAPROXY_STATS")))
	if maybeValue != "" {
		return maybeValue == "1" || maybeValue == "true" || maybeValue == "yes"
	}
	return defaultHaProxyStats
}

func HaProxyCredentials() string {
	maybeValue := os.Getenv("SB_HAPROXY_CREDENTIALS")
	if len(maybeValue) > 2 && strings.Contains(maybeValue, ":") {
		return maybeValue
	}
	if defaultHaProxyCredentials != "" && len(defaultHaProxyCredentials) > 2 && strings.Contains(defaultHaProxyCredentials, ":") {
		return defaultHaProxyCredentials
	}
	return "admin:password"
}
