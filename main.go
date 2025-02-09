//go:generate go run pkg/codegen/cleanup/main.go
//go:generate /bin/rm -rf pkg/generated
//go:generate go run pkg/codegen/main.go
//go:generate /bin/bash scripts/generate-manifest

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/rancher/wrangler/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/server"
)

var (
	Version    = "v0.0.0-dev"
	GitCommit  = "HEAD"
	KubeConfig string
)

func main() {
	app := cli.NewApp()
	app.Name = "rancher-harvester"
	app.Version = fmt.Sprintf("%s (%s)", Version, GitCommit)
	app.Usage = ""
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "kubeconfig",
			EnvVar:      "KUBECONFIG",
			Destination: &KubeConfig,
		},
		cli.IntFlag{
			Name:        "threadiness",
			EnvVar:      "THREADINESS",
			Value:       10,
			Destination: &config.Threadiness,
		},
		cli.IntFlag{
			Name:        "http-port",
			EnvVar:      "HARVESTER_SERVER_HTTP_PORT",
			Value:       8080,
			Destination: &config.HTTPListenPort,
		},
		cli.IntFlag{
			Name:        "https-port",
			EnvVar:      "HARVESTER_SERVER_HTTPS_PORT",
			Value:       8443,
			Destination: &config.HTTPSListenPort,
		},
		cli.StringFlag{
			Name:        "namespace",
			EnvVar:      "NAMESPACE",
			Destination: &config.Namespace,
			Usage:       "The default namespace to store management resources",
			Required:    true,
		},
		cli.StringFlag{
			Name:        "image-storage-endpoint",
			Usage:       "S3 compatible storage endpoint(format: http://example.com:9000). It should be accessible across the cluster",
			EnvVar:      "IMAGE_STORAGE_ENDPOINT",
			Destination: &config.ImageStorageEndpoint,
			Required:    true,
		},
		cli.StringFlag{
			Name:        "image-storage-access-key",
			EnvVar:      "IMAGE_STORAGE_ACCESS_KEY",
			Destination: &config.ImageStorageAccessKey,
			Required:    true,
		},
		cli.StringFlag{
			Name:        "image-storage-secret-key",
			EnvVar:      "IMAGE_STORAGE_SECRET_KEY",
			Destination: &config.ImageStorageSecretKey,
			Required:    true,
		},
		cli.BoolFlag{
			Name:        "skip-authentication",
			EnvVar:      "SKIP_AUTHENTICATION",
			Destination: &config.SkipAuthentication,
			Usage:       "Define whether to skip auth login or not, default to false",
		},
		cli.StringFlag{
			Name:   "authentication-mode",
			EnvVar: "HARVESTER_AUTHENTICATION_MODE",
			Usage:  "Define authentication mode, kubernetesCredentials and localUser are supported, could config more than one mode, separated by comma",
		},
	}
	app.Action = run

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func run(c *cli.Context) {
	logrus.Info("Starting controller")
	ctx := signals.SetupSignalHandler(context.Background())

	cfg, err := server.GetConfig(KubeConfig)
	if err != nil {
		logrus.Fatalf("failed to find kubeconfig: %v", err)
	}

	harv, err := server.New(ctx, cfg)
	if err != nil {
		logrus.Fatalf("failed to create harvester server: %v", err)
	}
	if err := harv.ListenAndServe(nil); err != nil {
		logrus.Fatalf("harvester server stop, %v", err)
	}
}
