package main

import (
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/jaytaylor/shipbuilder/pkg/buildpacks"
	"github.com/jaytaylor/shipbuilder/pkg/core"
	"github.com/jaytaylor/shipbuilder/pkg/version"

	log "github.com/sirupsen/logrus"
	"gopkg.in/urfave/cli.v2"
)

func main() {
	app := &cli.App{
		Name:        "shipbuilder",
		Version:     version.Version,
		Description: "Welcome to Shipbuilder!",
		Usage:       "Shipbuilder client",
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
					&cli.BoolFlag{ // TODO: Change to bool.
						Name:        "haproxy-stats",
						EnvVars:     []string{"SB_HAPROXY_STATS"},
						Usage:       "Control whether or not generated HAProxy configs will have statistics enabled",
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
					server := &core.Server{}
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
			&cli.Command{
				Name:        "buildpacks",
				Aliases:     []string{"build-packs"},
				Description: "List available build-packs",
				Action: func(ctx *cli.Context) error {
					var (
						packs    = []string{}
						packsMap = map[string]struct{}{} // For ensuring uniqueness.
					)

					for _, name := range buildpacks.AssetNames() {
						name = strings.Split(name, "/")[0]
						if _, ok := packsMap[name]; !ok {
							packsMap[name] = struct{}{}
							packs = append(packs, name)
						}
					}

					sort.Strings(packs)
					for _, name := range packs {
						fmt.Fprintf(os.Stdout, "%v\n", name)
					}
					return nil
				},
			},
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
