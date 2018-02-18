package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jaytaylor/shipbuilder/pkg/bindata_buildpacks"
	"github.com/jaytaylor/shipbuilder/pkg/cliutil"
	"github.com/jaytaylor/shipbuilder/pkg/core"
	"github.com/jaytaylor/shipbuilder/pkg/domain"
	"github.com/jaytaylor/shipbuilder/pkg/releases"
	"github.com/jaytaylor/shipbuilder/pkg/version"

	"github.com/gigawattio/errorlib"
	lsbase "github.com/jaytaylor/logserver"
	log "github.com/sirupsen/logrus"
	"gopkg.in/urfave/cli.v2"
)

var (
	appFlag = &cli.StringFlag{
		Name:    "app",
		Aliases: []string{"a", "app-name"},
		Usage:   "Name of app",
	}
	deferredFlag = &cli.BoolFlag{
		Name:    "deferred",
		Aliases: []string{"defer", "d"},
		Usage:   "Defer app redeployment",
	}
	suffixes = map[string][]string{
		"set":    []string{"set", "+"},
		"add":    []string{"add", "+"},
		"remove": []string{"remove", "rm", "delete", "del", "-"},
		"list":   []string{"list", "ls"},
		"status": []string{"status", "stat"},
	} // Sets of common flag suffixes.
)

// TODO: Generic command outputs for slice, map[string]interface{}.
//       Then add options like output-format=text,json,yaml, etc..

func main() {
	app := &cli.App{
		Name:        "shipbuilder",
		Version:     version.Version,
		Description: "Welcome to Shipbuilder!",
		Usage:       "Shipbuilder client",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "quiet",
				Aliases: []string{"q"},
				EnvVars: []string{"SB_QUIET_LOGGING"},
				Usage:   "Turn down logging to warnings and errors only",
			},
			&cli.BoolFlag{
				Name:    "silent",
				Aliases: []string{"s"},
				EnvVars: []string{"SB_SILENT_LOGGING"},
				Usage:   "Turn down logging to errors only",
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"vv", "debug", "d"},
				EnvVars: []string{"SB_VERBOSE_LOGGING", "SB_DEBUG_LOGGING"},
				Usage:   "Enable verbose debug logging messages",
			},
			&cli.StringFlag{
				Name:        "ssh-host",
				EnvVars:     []string{"SB_SSH_HOST"},
				Usage:       "Address of the server host for the client to connect to",
				Value:       core.DefaultSSHHost,
				Destination: &core.DefaultSSHHost,
			},
			&cli.StringFlag{
				Name:        "ssh-key",
				EnvVars:     []string{"SB_SSH_KEY"},
				Usage:       "Location of SSH key for the client to use",
				Value:       core.DefaultSSHKey,
				Destination: &core.DefaultSSHKey,
			},
		},
		Before: func(ctx *cli.Context) error {
			if err := initLogging(ctx); err != nil {
				return err
			}
			return nil
		},
		Action: func(ctx *cli.Context) error {
			client := &core.Client{}
			client.Do(os.Args) // ctx.Args().Slice())
			return nil
		},
		Commands: []*cli.Command{
			&cli.Command{
				Name:    "client",
				Aliases: []string{"c"},
				Action: func(ctx *cli.Context) error {
					client := &core.Client{}
					client.Do(ctx.Args().Slice())
					return nil
				},
			},
			&cli.Command{
				Name:    "server",
				Aliases: []string{"s"},
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "haproxy-enable-nonstandard-ports",
						EnvVars:     []string{"SB_HAPROXY_ENABLE_NONSTANDARD_PORTS"},
						Usage:       "Set to '1' to enable support for non-standard HAProxy load-balancer ports; should only be enabled for testing and development purposes because it's a little less precise about domain name matching",
						Value:       core.DefaultHAProxyEnableNonstandardPorts,
						Destination: &core.DefaultHAProxyEnableNonstandardPorts,
					},
					&cli.StringFlag{
						Name:        "haproxy-stats",
						EnvVars:     []string{"SB_HAPROXY_STATS"},
						Usage:       "Set to '1' to enable statistics for generated HAProxy configs will have statistics enabled",
						Value:       core.DefaultHAProxyStats,
						Destination: &core.DefaultHAProxyStats,
					},
					&cli.StringFlag{
						Name:        "haproxy-credentials",
						EnvVars:     []string{"SB_HAPROXY_CREDENTIALS"},
						Usage:       "HAProxy user:secret",
						Value:       core.DefaultHAProxyCredentials,
						Destination: &core.DefaultHAProxyCredentials,
					},
					&cli.StringFlag{
						Name:        "aws-key",
						EnvVars:     []string{"SB_AWS_KEY"},
						Usage:       "AWS key",
						Value:       core.DefaultAWSKey,
						Destination: &core.DefaultAWSKey,
					},
					&cli.StringFlag{
						Name:        "aws-secret",
						EnvVars:     []string{"SB_AWS_SECRET"},
						Usage:       "AWS secret",
						Value:       core.DefaultAWSSecret,
						Destination: &core.DefaultAWSSecret,
					},
					&cli.StringFlag{
						Name:        "aws-region",
						EnvVars:     []string{"SB_AWS_REGION"},
						Usage:       "AWS region to use",
						Value:       core.DefaultAWSRegion,
						Destination: &core.DefaultAWSRegion,
					},
					&cli.StringFlag{
						Name:        "s3-bucket",
						EnvVars:     []string{"SB_S3_BUCKET"},
						Usage:       "Name of S3 bucket where app releases will be stored",
						Value:       core.DefaultS3BucketName,
						Destination: &core.DefaultS3BucketName,
					},
					&cli.StringFlag{
						Name:        "lxc-fs",
						EnvVars:     []string{"SB_LXC_FS"},
						Usage:       "LXC filesystem type",
						Value:       core.DefaultLXCFS,
						Destination: &core.DefaultLXCFS,
					},
					&cli.StringFlag{
						Name:        "zfs-pool",
						EnvVars:     []string{"SB_ZFS_POOL"},
						Usage:       "ZFS pool name",
						Value:       core.DefaultZFSPool,
						Destination: &core.DefaultZFSPool,
					},
					&cli.StringFlag{
						Name:    "releases-provider",
						EnvVars: []string{"SB_RELEASES_PROVIDER"},
						Usage:   "Release persistence backend, must be one of: 'aws', 'fs'",
						Value:   "aws",
					},
					&cli.StringFlag{
						Name:    "fs-releases-provider-path",
						Aliases: []string{"fs-path"},
						EnvVars: []string{"SB_FS_RELEASES_PROVIDER_PATH"},
						Usage:   "Storage path for FS releases provider",
					},
					&cli.StringFlag{
						Name:    "listen",
						Aliases: []string{"l", "listen-addr"},
						EnvVars: []string{"SB_SERVER_LISTEN_ADDR", "SB_SERVER_LISTEN_ADDRESS"},
						Usage:   "addr:port for Shipbuilder to listen for TCP connections on",
						Value:   core.DefaultListenAddr,
					},
					&cli.StringFlag{
						Name:    "logserver-listen",
						Aliases: []string{"lsl", "logserver-listen-addr"},
						EnvVars: []string{"SB_LOGSERVER_LISTEN_ADDR", "SB_LOGSERVER_LISTEN_ADDRESS"},
						Usage:   "addr:port for Logserver to listen for TCP connections on",
						Value:   fmt.Sprintf(":%v", lsbase.DefaultPort),
					},
				},
				Before: func(ctx *cli.Context) error {
					if ctx.Args().Len() == 0 {
						if err := core.ValidateConfig(); err != nil {
							return err
						}
					}
					return nil
				},
				Action: func(ctx *cli.Context) error {
					releasesProvider, err := releasesProvider(ctx)
					if err != nil {
						return err
					}

					server := &core.Server{
						ListenAddr:          ctx.String("listen"),
						LogServerListenAddr: ctx.String("logserver-listen"),
						BuildpacksProvider:  bindata_buildpacks.NewProvider(),
						ReleasesProvider:    releasesProvider,
					}
					if err := server.Start(); err != nil {
						return err
					}
					if err := sigWait(); err != nil {
						return err
					}
					return nil
				},
				Subcommands: []*cli.Command{
					&cli.Command{
						Name:        "showconfig",
						Aliases:     []string{"show-config"},
						Description: "Print current configuration",
						Action: func(ctx *cli.Context) error {
							type pair struct {
								key   string
								value interface{}
							}
							pairs := []pair{
								{"DefaultHAProxyEnableNonstandardPorts", core.DefaultHAProxyEnableNonstandardPorts},
								{"DefaultHAProxyStats", core.DefaultHAProxyStats},
								{"DefaultHAProxyCredentials", core.DefaultHAProxyCredentials},
								{"DefaultAWSKey", core.DefaultAWSKey},
								{"DefaultAWSSecret", core.DefaultAWSSecret},
								{"DefaultAWSRegion", core.DefaultAWSRegion},
								{"DefaultS3BucketName", core.DefaultS3BucketName},
								{"DefaultSSHHost", core.DefaultSSHHost},
								{"DefaultSSHKey", core.DefaultSSHKey},
								{"DefaultLXCFS", core.DefaultLXCFS},
								{"DefaultZFSPool", core.DefaultZFSPool},
							}
							for _, p := range pairs {
								fmt.Fprintf(os.Stdout, "%v: %v\n", p.key, p.value)
							}
							return nil
						},
					},
				},
			},

			////////////////////////////////////////////////////////////////////
			// Container meta-data commands

			&cli.Command{
				Name:        "container",
				Aliases:     []string{"containers"},
				Description: "Provides access to server's desired container configuration",
				Subcommands: []*cli.Command{
					&cli.Command{
						Name:        "list-disable-services",
						Aliases:     []string{"show-disable-services"},
						Description: "Print out list of ubuntu 16.04 system services to disable in app containers",
						Action: func(ctx *cli.Context) error {
							fmt.Fprintf(os.Stdout, "%v\n", strings.Join(core.DisableServices, "\n"))
							return nil
						},
					},
					&cli.Command{
						Name:        "list-purge-packages",
						Aliases:     []string{"show-purge-packages"},
						Description: "Print out list of packages to purge from ubuntu 16.04 app containers",
						Action: func(ctx *cli.Context) error {
							fmt.Fprintf(os.Stdout, "%v\n", strings.Join(core.PurgePackages, "\n"))
							return nil
						},
					},
				},
			},

			////////////////////////////////////////////////////////////////////
			// Embedded script printer commands

			&cli.Command{
				Name:        "script",
				Aliases:     []string{"scripts"},
				Description: "Script printer (for accessing embdedded scripts",
				Action: func(ctx *cli.Context) error {
					if ctx.Args().Len() > 0 {
						fmt.Printf("Error: Unrecognized script %q specified\n\n", ctx.Args().First())
					}
					fmt.Println("Available scripts:")
					longestName := 0
					for _, scriptCommand := range scriptSubcommands() {
						if l := len(scriptCommand.Name); l > 0 {
							longestName = l
						}
					}
					for _, scriptCommand := range scriptSubcommands() {
						aliases := ""
						if len(scriptCommand.Aliases) > 0 {
							aliases = fmt.Sprintf(" (aliases: %v)", strings.Join(scriptCommand.Aliases, ", "))
						}
						fmt.Printf(fmt.Sprintf("    %%-%vv%%v\n", longestName), scriptCommand.Name, aliases)
					}
					return nil
				},
				Subcommands: scriptSubcommands(),
			},

			////////////////////////////////////////////////////////////////////
			// Client commands

			&cli.Command{
				Name:        "buildpacks",
				Aliases:     []string{"build-packs"},
				Description: "List available build-packs",
				Action: func(ctx *cli.Context) error {
					for _, name := range bindata_buildpacks.NewProvider().Available() {
						fmt.Fprintf(os.Stdout, "%v\n", name)
					}
					return nil
				},
				Subcommands: buildpackSubcommands(),
			},

			// &cli.Command{
			// 	Name:        "buildpacks",
			// 	Aliases:     []string{"build-packs"},
			// 	Description: "List available build-packs",
			// 	Action: func(ctx *cli.Context) error {
			// 		for _, name := range bindata_buildpacks.NewProvider().Available() {
			// 			fmt.Fprintf(os.Stdout, "%v\n", name)
			// 		}
			// 		return nil
			// 	},
			// },

			////////////////////////////////////////////////////////////////////
			// App management commands                                        //
			////////////////////////////////////////////////////////////////////

			////////////////////////////////////////////////////////////////////
			// apps:*
			command(
				[]string{"apps", "apps:list", "Apps_List"},
				"List shipbuilder-managed apps",
			),

			appCommand(
				[]string{"create", "apps:create", "Apps_Create"},
				"Create a new app",
				flagSpec{
					names:       []string{"buildpack", "b"},
					usage:       "Desired buildpack for app type",
					allowedVals: bindata_buildpacks.NewProvider().Available(),
					required:    true,
				},
			),

			appCommand(
				[]string{"destroy", "apps:destroy", "Apps_Destroy"},
				"Destroy an app",
				flagSpec{
					names: []string{"force", "f"},
					usage: "Force removal without confirmation prompt",
					typ:   "bool",
				},
			),

			command(
				[]string{"clone", "apps:clone", "Apps_Clone"},
				"Clone an app",
				flagSpec{
					names:    []string{"old-app", "o"},
					usage:    "Name of old app",
					required: true,
				},
				flagSpec{
					names:    []string{"new-app", "n"},
					usage:    "Name of new app",
					required: true,
				},
			),

			// HERE

			////////////////////////////////////////////////////////////////////
			// config:*
			appCommand(
				cliutil.PermuteCmds([]string{"config", "cfg"}, []string{"list", "ls"}, true, "Config_List"),
				"Displays the entire configuration for an app",
			),
			&cli.Command{
				Name:        cliutil.PermuteCmds([]string{"config", "cfg"}, []string{"get", "show"}, false, "Config_Get")[0],
				Aliases:     cliutil.PermuteCmds([]string{"config", "cfg"}, []string{"get", "show"}, false, "Config_Get")[1:],
				Description: "Get one or more configuration parameter values for an app (displays all if none specified)",
				Flags: []cli.Flag{
					appFlag,
					&cli.StringFlag{
						Name:    "key",
						Aliases: []string{"k", "parameter", "p"},
						Usage:   "Configuration parameter to lookup",
					},
				},
				Action: func(ctx *cli.Context) error {
					var (
						app = ctx.String("app")
						key = ctx.String("key")
					)
					if len(app) == 0 {
						return errors.New("app flag is required")
					}
					if len(key) == 0 {
						return (&core.Client{}).RemoteExec("Config_List", app)
					}
					return (&core.Client{}).RemoteExec("Config_Get", app, key)
				},
			},
			deferredMappedAppCommand(
				append([]string{"set"}, cliutil.PermuteCmds([]string{"config", "cfg"}, suffixes["set"], false, "Config_Set")...),
				"Set the value of one or more configuration parameters for an app in the form of FOO=bar BAZ=xy",
			),
			&cli.Command{
				Name:        cliutil.PermuteCmds([]string{"config", "cfg"}, suffixes["remove"], false, "Config_Remove")[0],
				Aliases:     cliutil.PermuteCmds([]string{"config", "cfg"}, suffixes["remove"], false, "Config_Remove")[1:],
				Description: "Remove one or more configuration keys from an app",
				Flags: []cli.Flag{
					appFlag,
					&cli.BoolFlag{
						Name:    "deferred",
						Aliases: []string{"defer", "d"},
						Usage:   "Defer app redeployment",
					},
					&cli.StringSliceFlag{
						Name:    "key",
						Aliases: []string{"k", "parameter", "p"},
						Usage:   "Pass multiple time for multiple keys",
					},
				},
				Action: func(ctx *cli.Context) error {
					var (
						app      = ctx.String("app")
						deferred = ctx.Bool("deferred")
						keys     = ctx.StringSlice("key")
					)
					if len(app) == 0 {
						return errors.New("app flag is required")
					}
					if len(keys) == 0 {
						keys = ctx.Args().Slice()
						if len(keys) == 0 {
							return errors.New("key flag(s) or args are required")
						}
					}
					return (&core.Client{}).RemoteExec("Config_Remove", app, deferred, keys)
				},
			},

			////////////////////////////////////////////////////////////////////
			// domains:*
			appCommand(
				cliutil.PermuteCmds([]string{"domains", "domain"}, suffixes["list"], true, "Domains_List"),
				"Show domain names associated with an app",
				// TODO: Add sub-commands instead of ':' delimited pair
				// clusters.
			),
			&cli.Command{
				Name:        cliutil.PermuteCmds([]string{"domains"}, suffixes["add"], false, "Domains_Add")[0],
				Aliases:     cliutil.PermuteCmds([]string{"domains"}, suffixes["add"], false, "Domains_Add")[1:],
				Description: "Associate one or more domain names to an app",
				Flags: []cli.Flag{
					appFlag,
					deferredFlag,
				},
				Action: func(ctx *cli.Context) error {
					var (
						app      = ctx.String("app")
						deferred = ctx.Bool("deferred")
						domains  = ctx.Args().Slice()
					)
					if len(app) == 0 {
						return errors.New("app flag is required")
					}
					if len(domains) == 0 {
						return errors.New("cannot add empty list of domains to app")
					}
					return (&core.Client{}).RemoteExec("Domains_Add", app, deferred, domains)
				},
			},
			&cli.Command{
				Name:        cliutil.PermuteCmds([]string{"domains"}, suffixes["remove"], false, "Domains_Remove")[0],
				Aliases:     cliutil.PermuteCmds([]string{"domains"}, suffixes["remove"], false, "Domains_Remove")[1:],
				Description: "Remove one or more domain names from an app",
				Flags: []cli.Flag{
					appFlag,
					deferredFlag,
				},
				Action: func(ctx *cli.Context) error {
					var (
						app      = ctx.String("app")
						deferred = ctx.Bool("deferred")
						domains  = ctx.Args().Slice()
					)
					if len(app) == 0 {
						return errors.New("app flag is required")
					}
					if len(domains) == 0 {
						return errors.New("cannot remove empty list of domains from app")
					}
					return (&core.Client{}).RemoteExec("Domains_Remove", app, deferred, domains)
				},
			},

			////////////////////////////////////////////////////////////////////
			// drains:*
			appCommand(
				cliutil.PermuteCmds([]string{"drains", "drain"}, suffixes["list"], true, "Drains_List"),
				"Show drains for an app",
			),
			&cli.Command{
				Name:        cliutil.PermuteCmds([]string{"drains", "drain"}, suffixes["add"], false, "Drains_Add")[0],
				Aliases:     cliutil.PermuteCmds([]string{"drains", "drain"}, suffixes["add"], false, "Drains_Add")[1:],
				Description: "Add one or more drains to an app",
				Flags: []cli.Flag{
					appFlag,
				},
				Action: func(ctx *cli.Context) error {
					var (
						app    = ctx.String("app")
						drains = ctx.Args().Slice()
					)
					if len(app) == 0 {
						return errors.New("app flag is required")
					}
					if len(drains) == 0 {
						return errors.New("cannot add empty list of drains to app")
					}
					return (&core.Client{}).RemoteExec("Drains_Add", app, drains)
				},
			},
			&cli.Command{
				Name:        cliutil.PermuteCmds([]string{"drains", "drain"}, suffixes["remove"], false, "Drains_Remove")[0],
				Aliases:     cliutil.PermuteCmds([]string{"drains", "drain"}, suffixes["remove"], false, "Drains_Remove")[1:],
				Description: "Remove one or more drains from an app",
				Flags: []cli.Flag{
					appFlag,
				},
				Action: func(ctx *cli.Context) error {
					var (
						app    = ctx.String("app")
						drains = ctx.Args().Slice()
					)
					if len(app) == 0 {
						return errors.New("app flag is required")
					}
					if len(drains) == 0 {
						return errors.New("cannot remove empty list of drains from app")
					}
					return (&core.Client{}).RemoteExec("Drains_Remove", app, drains)
				},
			},

			////////////////////////////////////////////////////////////////////
			// redeploy
			appCommand(
				[]string{"redeploy", "apps:redeploy", "Redeploy_App"},
				"Redeploy the current running version of an app",
			),
			// TODO: Verify that this alwaays redeploys the current version and not the latest version.

			////////////////////////////////////////////////////////////////////
			// deploy
			appCommand(
				[]string{"deploy", "Deploy"},
				"Deploy an app at a specific version",
				flagSpec{
					names:    []string{"version", "v", "revision", "r"},
					usage:    "Version to use",
					required: true,
				},
			),

			////////////////////////////////////////////////////////////////////
			// reset
			appCommand(
				[]string{"reset", "apps:reset", "Reset_App"},
				"Reset all build artifacts for an app so the next deployment will build from scratch",
			),

			////////////////////////////////////////////////////////////////////
			// logs:*
			appCommand(
				[]string{"logs", "logs:get", "Logs_Get"},
				"Get logs for an app",
				flagSpec{
					names: []string{"process", "p"},
					usage: "App process name",
				},
				flagSpec{
					names: []string{"filter"},
					usage: "Golang regular exression to filter log lines on",
				},
			),

			////////////////////////////////////////////////////////////////////
			// logger
			&cli.Command{
				Name:        "logger",
				Aliases:     []string{"Logger"},
				Description: "Logger command for apps to send logs back to shipbuilder",
				Flags: []cli.Flag{
					appFlag,
					&cli.StringFlag{
						Name:  "host",
						Usage: "Logserver hostname",
					},
					&cli.StringFlag{
						Name:    "process",
						Aliases: []string{"p"},
						Usage:   "App process ID name",
					},
				},
				Action: func(ctx *cli.Context) error {
					var (
						host    = ctx.String("host")
						app     = ctx.String("app")
						process = ctx.String("process")
					)

					if len(host) == 0 {
						return errors.New("host flag is required and must not be empty!")
					}
					if len(app) == 0 {
						return errors.New("app flag is required and must not be empty!")
					}
					if len(process) == 0 {
						return errors.New("process flag is required and must not be empty!")
					}

					(&core.Local{}).Logger(host, app, process)
					return nil
				},
			},

			////////////////////////////////////////////////////////////////////
			// run
			appCommand(
				[]string{"run", "shell", "console", "Console"},
				"Run a command in an app container image",
				flagSpec{
					names:    []string{"command", "c"},
					usage:    "Command to use",
					required: true,
				},
			),
			// TODO: consider adding command to attach to a running container.

			////////////////////////////////////////////////////////////////////
			// maint:*
			appCommand(
				cliutil.PermuteCmds([]string{"maintenance", "maint"}, []string{"url"}, false, "Maintenance_Url"),
				"Set the maintenance redirect URL for an app",
				flagSpec{
					names:    []string{"url", "u"},
					usage:    "Maintenance URL",
					required: true,
				},
			),
			appCommand(
				cliutil.PermuteCmds([]string{"maintenance", "maint"}, suffixes["status"], true, "Maintenance_Status"),
				"Show maintenance mode status for an app",
			),
			appCommand(
				cliutil.PermuteCmds([]string{"maintenance", "maint"}, []string{"on", "+"}, false, "Maintenance_On"),
				"Activates maintenance mode for an app",
			),
			appCommand(
				cliutil.PermuteCmds([]string{"maintenance", "maint"}, []string{"off", "-"}, false, "Maintenance_Off"),
				"Deactivates maintenance mode for an app",
			),

			////////////////////////////////////////////////////////////////////
			// privatekey:*
			appCommand(
				cliutil.PermuteCmds([]string{"privatekey", "privkey"}, []string{"get"}, true, "PrivateKey_Get"),
				"Show SSH private key to use for accessing and cloning protected repositories when checking out git submodules for app",
			),
			appCommand(
				cliutil.PermuteCmds([]string{"privatekey", "privkey"}, suffixes["set"], false, "PrivateKey_Set"),
				"Set the maintenance redirect URL for an app",
				flagSpec{
					names:    []string{"private-key"},
					usage:    "Private SSH keystring",
					required: true,
				},
			),
			appCommand(
				cliutil.PermuteCmds([]string{"privatekey", "privkey"}, suffixes["remove"], false, "PrivateKey_Remove"),
				"Remove existing SSH private key from app",
			),

			////////////////////////////////////////////////////////////////////
			// ps:*
			appCommand(
				cliutil.PermuteCmds([]string{"ps"}, suffixes["list"], true, "Ps_List"),
				"Show running container processes for app",
			),
			deferredMappedAppCommand(
				[]string{"ps:scale", "scale", "Ps_Scale"},
				"Scale app processes up or down",
			),
			argsOrFlagAppCommand(
				cliutil.PermuteCmds([]string{"ps"}, suffixes["status"], false, "Ps_Status"),
				"Get the status of one or more container processes for an app",
				[]string{"process-types"},
				"Specify flag multiple times for multiple process types",
			),
			argsOrFlagAppCommand(
				[]string{"ps:restart", "restart", "Ps_Restart"},
				"Restart one or more container processes for an app",
				[]string{"process-types"},
				"Specify flag multiple times for multiple process types",
			),
			argsOrFlagAppCommand(
				[]string{"ps:stop", "stop", "Ps_Stop"},
				"Stop one or more container processes for an app",
				[]string{"process-types"},
				"Specify flag multiple times for multiple process types",
			),
			argsOrFlagAppCommand(
				[]string{"ps:start", "start", "Ps_Start"},
				"Start one or more container processes for an app",
				[]string{"process-types"},
				"Specify flag multiple times for multiple process types",
			),

			////////////////////////////////////////////////////////////////////
			// rollback
			appCommand(
				[]string{"rollback", "Rollback"},
				"Roll app back to previous release",
				flagSpec{
					names: []string{"version", "v"},
					usage: "Version to rollback to - if omitted, then the previous version will be used",
				},
			),

			////////////////////////////////////////////////////////////////////
			// releases:*
			appCommand(
				cliutil.PermuteCmds([]string{"releases", "release", "rls"}, suffixes["list"], true, "Releases_List"),
				"Show app release history",
			),
			appCommand(
				cliutil.PermuteCmds([]string{"releases", "release", "rls"}, []string{"info", "detail", "details"}, false, "Releases_Info"),
				"Show detailed release history information for a specific version",
				flagSpec{
					names: []string{"version", "v"},
					usage: "Version to rollback to - if omitted, then the previous version will be used",
				},
			),

			////////////////////////////////////////////////////////////////////
			// Global system management commands                              //
			////////////////////////////////////////////////////////////////////

			// TODO: Not yet implemented.
			command(
				[]string{"health", "apps:health", "App_Health"},
				"Show health report for all apps",
			),
			////////////////////////////////////////////////////////////////////
			// lb:*
			command(
				cliutil.PermuteCmds([]string{"lb", "lbs"}, suffixes["list"], true, "LoadBalancer_List"),
				"Show server load-balancers",
			),
			command(
				cliutil.PermuteCmds([]string{"lb", "lbs"}, suffixes["add"], false, "LoadBalancer_Add"),
				"Add one or more load-balancers to shipbuilder instance",
				flagSpec{
					names:    []string{"hostname", "hostnames"},
					usage:    "Specify flag multiple times for multiple load-balancer hostnames",
					required: true,
					args:     true,
					typ:      "slice",
				},
			),
			command(
				cliutil.PermuteCmds([]string{"lb", "lbs"}, suffixes["remove"], false, "LoadBalancer_Remove"),
				"Disaassociate one or more load-balancers from the shipbuilder instance.",
				flagSpec{
					names:    []string{"hostname", "hostnames"},
					usage:    "Specify flag multiple times for multiple load-balancer hostnames",
					required: true,
					args:     true,
					typ:      "slice",
				},
			),
			command(
				cliutil.PermuteCmds([]string{"lb", "lbs"}, []string{"sync"}, false, "LoadBalancer_Sync"),
				"Sync internal apps and domains state to physical LB configuration",
			),

			////////////////////////////////////////////////////////////////////
			// nodes:*
			command(
				cliutil.PermuteCmds([]string{"nodes", "node", "slaves", "slave"}, suffixes["list"], true, "Node_List"),
				"Show server slave nodes",
			),
			command(
				cliutil.PermuteCmds([]string{"nodes", "node", "slaves", "slave"}, suffixes["add"], false, "Node_Add"),
				"Associate one or more slave nodes to the shipbuilder instance",
				flagSpec{
					names:    []string{"hostname", "hostnames"},
					usage:    "Specify flag multiple times for multiple slave node hostnames",
					required: true,
					args:     true,
					typ:      "slice",
				},
			),
			command(
				cliutil.PermuteCmds([]string{"nodes", "node", "slaves", "slave"}, suffixes["remove"], false, "Node_Remove"),
				"Disassociate one or more slave nodes from the shipbuilder instance",
				flagSpec{
					names:    []string{"hostname", "hostnames"},
					usage:    "Specify flag multiple times for multiple slave node hostnames",
					required: true,
					args:     true,
					typ:      "slice",
				},
			),

			////////////////////////////////////////////////////////////////////
			// runtime:*
			// DISABLED:
			// global("runtime:tests", "runtimetests", "LocalRuntimeTests"),

			////////////////////////////////////////////////////////////////////
			// sys:*
			command(
				[]string{"system:zfscleanup", "sys:zfscleanup", "System_ZfsCleanup"},
				"Cleans up old app build versions on the shipbuilder build box; IMPORTANT: this is automated via cron, so it shoud not need to be run manually",
			),
			command(
				[]string{"system:snapshotscleanup", "sys:snapshotscleanup", "System_SnapshotsCleanup"},
				"Prune orphaned ZFS snapshots; IMPORTANT: this is automated via cron, so it shoud not need to be run manually",
			),
			command(
				[]string{"system:ntpsync", "sys:ntpsync", "System_NtpSync"},
				"Sync system clock via NTP; IMPORTANT: this is automated via cron, so it shoud not need to be run manually",
			),

			////////////////////////////////////////////////////////////////////
			// Backend functions                                              //
			////////////////////////////////////////////////////////////////////

			////////////////////////////////////////////////////////////////////
			// pre/post-receive hooks
			command(
				[]string{"pre-receive", "PreReceive"},
				"Shipbuilder server git pre-receive hook function",
				flagSpec{
					names:    []string{"directory", "d"},
					usage:    "Path to directory",
					required: true,
				},
				flagSpec{
					names:    []string{"oldrev", "o"},
					usage:    "old git revision",
					required: true,
				},
				flagSpec{
					names:    []string{"newrev", "n"},
					usage:    "new git revision",
					required: true,
				},
				flagSpec{
					names:    []string{"ref", "r"},
					usage:    "git ref",
					required: true,
				},
			),
			command(
				[]string{"post-receive", "PostReceive"},
				"Shipbuilder server git post-receive hook function",
				flagSpec{
					names:    []string{"directory", "d"},
					usage:    "Path to directory",
					required: true,
				},
				flagSpec{
					names:    []string{"oldrev", "o"},
					usage:    "old git revision",
					required: true,
				},
				flagSpec{
					names:    []string{"newrev", "n"},
					usage:    "new git revision",
					required: true,
				},
				flagSpec{
					names:    []string{"ref", "r"},
					usage:    "git ref",
					required: true,
				},
			),
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Errorf("%s", err)
		os.Exit(1)
	}
}

func sigWait() error {
	sigCh := make(chan os.Signal, 1)

	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	<-sigCh

	return nil
}

// releasesProvider performs runtime resolution for which releases provider to
// use.
func releasesProvider(ctx *cli.Context) (provider domain.ReleasesProvider, err error) {
	requested := ctx.String("releases-provider")

	switch requested {
	case "s3", "aws":
		provider = releases.NewAWSS3ReleasesProvider(core.DefaultAWSKey, core.DefaultAWSSecret, core.DefaultS3BucketName, core.DefaultAWSRegion)
		return

	case "fs":
		storagePath := ctx.String("fs-releases-provider-path")
		if len(storagePath) == 0 {
			err = errors.New("missing required parameter: fs-releases-provider-path")
			return
		}
		provider = releases.NewFSReleasesProvider(storagePath)
		return
	}

	err = fmt.Errorf("unrecognized releases-provider %q", requested)
	return
}

type flagSpec struct {
	names       []string // NB: pos[0] = name, pos[1:] = aliases.
	usage       string
	required    bool
	allowedVals []string // Optional.
	args        bool     // Use as os.Args based parameter instead of a flag.
	typ         string   // NB: one of "", "bool", or "slice"; "" signifies a string flag.
}

func (spec flagSpec) flag() cli.Flag {
	if spec.args {
		panic(fmt.Sprintf("flagSpec %v has args enabled and thus is not eligible to be translated to a flag", spec.names[0]))
	}

	allowedValsUsage := ""
	if len(spec.allowedVals) > 0 {
		allowedValsUsage = fmt.Sprintf("; allowed values: %v", strings.Join(spec.allowedVals, ", "))
	}

	var flag cli.Flag

	switch spec.typ {
	case "":
		flag = &cli.StringFlag{
			Name:    spec.names[0],
			Aliases: spec.names[1:],
			Usage:   spec.usage + allowedValsUsage,
		}

	case "bool":
		flag = &cli.BoolFlag{
			Name:    spec.names[0],
			Aliases: spec.names[1:],
			Usage:   spec.usage,
		}

	case "slice":
		flag = &cli.StringSliceFlag{
			Name:    spec.names[0],
			Aliases: spec.names[1:],
			Usage:   spec.usage + allowedValsUsage,
		}

	default:
		panic(fmt.Sprintf("unrecognized spec.typ: %v", spec.typ))
	}

	return flag
}

func (spec flagSpec) val(ctx *cli.Context, argsConsumed *int) (interface{}, error) {
	switch spec.typ {
	case "":
		var val string
		if spec.args {
			val = ctx.Args().First()
		} else {
			val = ctx.String(spec.names[0])
		}
		if spec.required && len(val) == 0 {
			if ctx.Args().Len() > *argsConsumed {
				val = ctx.Args().Slice()[*argsConsumed]
				*argsConsumed++
			}
			if len(val) == 0 {
				return nil, spec.requiredError()
			}
		}
		return val, nil

	case "bool":
		var val = ctx.Bool(spec.names[0])
		return val, nil

	case "slice":
		var val []string
		if spec.args {
			val = ctx.Args().Slice()
		} else {
			val = ctx.StringSlice(spec.names[0])
		}
		if spec.required && len(val) == 0 {
			if ctx.Args().Len() > *argsConsumed {
				val = ctx.Args().Slice()[*argsConsumed:]
				*argsConsumed += len(val)
			}
			if len(val) == 0 {
				return nil, spec.requiredError()
			}
		}
		return val, nil

	default:
		return nil, fmt.Errorf("unrecognized spec.typ: %v", spec.typ)
	}

}

// requiredError returns the "flag required" error message to present to user.
func (spec flagSpec) requiredError() error {
	if spec.required {
		var plural string
		if spec.typ == "slice" {
			plural = "one or more "
		}
		if spec.args {
			if spec.typ == "slice" {
				return fmt.Errorf("one or more %v arguments are required", spec.names[0])
			}
			return fmt.Errorf("%v argument is required", spec.names[0], plural)
		}
		if spec.typ == "slice" {
			return fmt.Errorf("one or more %v flags are required", spec.names[0])
		}
		return fmt.Errorf("%v flag is required", spec.names[0])
	}
	return nil
}

// command generates a cli.Command with 0 or more string flags.
//
// In the generated function, if args count matches required flags count and
// flag values are empty, positional args will used.
//
// Important: names must be a non-empty slice and end with a value which
// corresponds to a valid shipbuilder server method.
func command(names []string, description string, flagSpecs ...flagSpec) *cli.Command {
	// TODO: Consider real validation via reflection for names[-1].
	if len(names) == 0 {
		panic("name / aliases slice must not be empty!")
	}

	cliFlags := []cli.Flag{}
	for _, spec := range flagSpecs {
		if spec.args {
			continue
		}
		if len(spec.names) == 0 {
			panic("flag name / aliases slice must not be empty!")
		}
		cliFlags = append(cliFlags, spec.flag())
	}

	return &cli.Command{
		Name:        names[0],
		Aliases:     names[1:],
		Description: description,
		Flags:       cliFlags,
		Action: func(ctx *cli.Context) error {
			var (
				funcArgs     = []interface{}{}
				errs         = []error{}
				argsConsumed = 0
			)
			for _, spec := range flagSpecs {
				val, err := spec.val(ctx, &argsConsumed)
				if err != nil {
					errs = append(errs, err)
					continue
				}
				if len(spec.allowedVals) > 0 {
					var found bool
					for _, allowed := range spec.allowedVals {
						if val == allowed {
							found = true
							break
						}
					}
					if !found {
						errs = append(errs, fmt.Errorf("%v must be one of: %v", spec.names[0], strings.Join(spec.allowedVals, ", ")))
					}
				}
				funcArgs = append(funcArgs, val)
			}
			if err := errorlib.Merge(errs); err != nil {
				return err
			}
			log.WithField("remote-func", names[len(names)-1]).WithField("args", funcArgs).Debug("invoking remote-exec")
			return (&core.Client{}).RemoteExec(names[len(names)-1], funcArgs...)
		},
	}
}

var appFlagSpec = flagSpec{
	names:    []string{"app", "a"},
	usage:    "Name of app",
	required: true,
}

// appCommand generates a simple app command.
//
// The names parameter must be non-empty and end with a value which corresponds
// to a valid shipbuilder command function.
func appCommand(names []string, description string, flagSpecs ...flagSpec) *cli.Command {
	return command(names, description, append([]flagSpec{appFlagSpec}, flagSpecs...)...)
}

// argsOrFlagAppCommand generates an app command with a single flag of type
// string slice which can also be passed as unnamed arguments.
//
// The names and flagNames parameters must be non-empty and end with a value
// which corresponds to a valid shipbuilder command function.
func argsOrFlagAppCommand(names []string, description string, flagNames []string, flagUsage string) *cli.Command {
	// TODO: Consider real validation via reflection for names[-1].
	if len(names) == 0 {
		panic("name / aliases slice must not be empty!")
	}
	if len(flagNames) == 0 {
		panic("flag name / aliases slice must not be empty!")
	}
	return &cli.Command{
		Name:        names[0],
		Aliases:     names[1:],
		Description: description,
		Flags: []cli.Flag{
			appFlag,
			&cli.StringSliceFlag{
				Name:    flagNames[0],
				Aliases: flagNames[1:],
				Usage:   flagUsage,
			},
		},
		Action: func(ctx *cli.Context) error {
			var (
				app         = ctx.String("app")
				dynamicFlag = ctx.StringSlice(flagNames[0])
			)
			if len(app) == 0 {
				return errors.New("app flag is required")
			}
			// NB: Notice the precedence here - flag is respected above args.
			if ctx.Args().Present() && len(dynamicFlag) == 0 {
				dynamicFlag = ctx.Args().Slice()
			}
			if len(dynamicFlag) == 0 {
				return fmt.Errorf("%v flag or args values are required", flagNames[0])
			}
			return (&core.Client{}).RemoteExec(names[len(names)-1], app, dynamicFlag)
		},
	}
}

// deferredMappedAppCommand generates a deferred mapped app command.
//
// The names parameter must be non-empty and end with a value which corresponds
// to a valid shipbuilder command function.
func deferredMappedAppCommand(names []string, description string) *cli.Command {
	// TODO: Consider real validation via reflection for names[-1].
	if len(names) == 0 {
		panic("name / aliases slice must not be empty!")
	}
	return &cli.Command{
		Name:        names[0],
		Aliases:     names[1:],
		Description: description,
		Flags: []cli.Flag{
			appFlag,
			deferredFlag,
		},
		Action: func(ctx *cli.Context) error {
			var (
				app      = ctx.String("app")
				deferred = ctx.Bool("deferred")
				mapped   = map[string]string{}
				errs     = []error{}
			)
			if len(app) == 0 {
				return errors.New("app flag is required")
			}
			for _, arg := range ctx.Args().Slice() {
				if pieces := strings.SplitN(arg, "=", 2); len(pieces) == 2 {
					mapped[pieces[0]] = pieces[1]
				} else {
					errs = append(errs, fmt.Errorf("malformed arg %q; must be of the form key=value", arg))
				}
			}
			if err := errorlib.Merge(errs); err != nil {
				return err
			}
			if len(mapped) == 0 {
				return errors.New("invalid due to empty map of key/value parameters")
			}
			return (&core.Client{}).RemoteExec(names[len(names)-1], app, deferred, mapped)
		},
	}
}

func scriptSubcommands() []*cli.Command {
	// genPrintAction generates a cli action function which prints the passed
	// content.
	genPrintAction := func(content string) func(*cli.Context) error {
		return func(_ *cli.Context) error {
			fmt.Fprint(os.Stdout, content)
			return nil
		}
	}

	return []*cli.Command{
		&cli.Command{
			Name:        "auto-iptables",
			Aliases:     []string{"auto-iptables.py", "iptables", "iptables.py"},
			Description: "Automatic IPTables fixer (should be cron'd to allow containers running on nodes to become accessible again after a reboot)",
			Action:      genPrintAction(core.AutoIPTablesScript),
		},
		&cli.Command{
			Name:        "postdeploy",
			Aliases:     []string{"postdeploy.py"},
			Description: "Container launcher",
			Action:      genPrintAction(core.POSTDEPLOY),
		},
		&cli.Command{
			Name:        "shutdown",
			Aliases:     []string{"shutdown.py"},
			Description: "Container terminator",
			Action:      genPrintAction(core.SHUTDOWN_CONTAINER),
		},
		&cli.Command{
			Name:        "lxd-systemd-patch",
			Aliases:     []string{"lxd-systemd-patch.py", "lxd-compat", "lxd-compat.py"},
			Description: "LXDCompatScript updates the LXD systemd service definition to protect against /var/lib/lxd path conflicts between LXD and shipbuilder",
			Action:      genPrintAction(core.LXDCompatScript),
		},
	}
}

func buildpackSubcommands() []*cli.Command {
	var (
		cmds     = []*cli.Command{}
		provider = bindata_buildpacks.NewProvider()
	)

	for _, bp := range provider.All() {
		subCmds := []*cli.Command{
			&cli.Command{
				Name:    "container-custom-commands",
				Aliases: []string{"ContainerCustomCommands"},
				Action: func(ctx *cli.Context) error {
					fmt.Fprintf(os.Stdout, "%v\n", bp.ContainerCustomCommands())
					return nil
				},
			},
			&cli.Command{
				Name:    "container-packages",
				Aliases: []string{"ContainerPackages"},
				Action: func(ctx *cli.Context) error {
					fmt.Fprintf(os.Stdout, "%v\n", strings.Join(bp.ContainerPackages(), "\n"))
					return nil
				},
			},
			&cli.Command{
				Name:    "pre-hook",
				Aliases: []string{"PreHook"},
				Action: func(ctx *cli.Context) error {
					fmt.Fprintf(os.Stdout, "%v\n", bp.PreHook())
					return nil
				},
			},
		}
		cmd := &cli.Command{
			Name: bp.Name(),
			Action: func(ctx *cli.Context) error {
				fmt.Fprint(os.Stdout, "container-custom-commands\ncontainer-packages\npre-hook\n")
				return nil
			},
			Subcommands: subCmds,
		}
		cmds = append(cmds, cmd)
	}

	return cmds
}

func initLogging(ctx *cli.Context) error {
	var (
		silent  = ctx.Bool("silent")
		quiet   = ctx.Bool("quiet")
		verbose = ctx.Bool("verbose")
	)
	if (silent && quiet) || (silent && verbose) || (quiet && verbose) {
		return errors.New("only one of silent, quiet, or verbose log output flags may be specified at a time")
	}
	if silent {
		log.SetLevel(log.ErrorLevel)
	}
	if quiet {
		log.SetLevel(log.WarnLevel)
	}
	if verbose {
		log.SetLevel(log.DebugLevel)
		log.Debug("Verbose debug logging enabled")
	}
	return nil
}
