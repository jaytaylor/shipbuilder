package main

import (
	"os"

	"github.com/jaytaylor/shipbuilder/pkg/core"
	"github.com/jaytaylor/shipbuilder/pkg/version"

	log "github.com/sirupsen/logrus"
	"gopkg.in/urfave/cli.v2"
)

func main() {
	args := os.Args
	if len(args) < 2 {
		log.Fatalln("expected at least one argument")
		return
	}
	switch args[1] {
	case "server":
		server()
	default:
		client := &core.Client{}
		client.Do(args)
	}

}

func server() {
	app := &cli.App{
		Name:    "shipbuilder",
		Version: version.Version,
		Usage:   "Shipbuilder server",
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
				EnvVars:     []string{},
				Usage:       "Name of S3 bucket where app releases will be stored",
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
				EnvVars:     []string{"LXC_FS"},
				Usage:       "LXC filesystem type",
				Value:       core.DefaultLXCFS,
				Destination: &core.DefaultLXCFS,
			},
			&cli.StringFlag{
				Name:        "zfs-pool",
				EnvVars:     []string{"ZFS_POOL"},
				Usage:       "ZFS pool name",
				Value:       core.DefaultZFSPool,
				Destination: &core.DefaultZFSPool,
			},
		},
		Action: func(ctx *cli.Context) error {
			server := &core.Server{}
			if err := server.Start(); err != nil {
				return err
			}
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Errorf("%s", err)
		os.Exit(1)
	}
}
