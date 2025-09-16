package main

import (
	"context"
	"time"

	"github.com/probe-lab/bitswap-sniffer/bitswap"
	"github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v3"
)

var runConfig = struct {
	Libp2pHost        string
	Libp2pPort        int
	ConnectionTimeout time.Duration
}{
	Libp2pHost:        "127.0.0.1",
	Libp2pPort:        9020,
	ConnectionTimeout: 10 * time.Second,
}

var cmdRun = &cli.Command{
	Name:                  "run",
	Usage:                 "Connects and scans a given node for its custody and network status",
	EnableShellCompletion: true,
	Action:                scanAction,
	Flags:                 runFlags,
}

var runFlags = []cli.Flag{
	&cli.StringFlag{
		Name:        "libp2p.host",
		Usage:       "IP for the Libp2p host",
		Value:       runConfig.Libp2pHost,
		Destination: &runConfig.Libp2pHost,
	},
	&cli.IntFlag{
		Name:        "metrics.port",
		Usage:       "Port for the Libp2p host",
		Value:       runConfig.Libp2pPort,
		Destination: &runConfig.Libp2pPort,
	},
	&cli.DurationFlag{
		Name:        "connection.timeout",
		Usage:       "Timeout for the connection attempt to the node",
		Value:       runConfig.ConnectionTimeout,
		Destination: &runConfig.ConnectionTimeout,
	},
}

func scanAction(ctx context.Context, cmd *cli.Command) error {
	log := rootConfig.Logger
	rootConfig.Logger.WithFields(logrus.Fields{
		"libp2p-host":        runConfig.Libp2pHost,
		"libp2p-port":        runConfig.Libp2pPort,
		"connection-timeout": runConfig.ConnectionTimeout,
	}).Info("running bitswap-sniffer")

	snifferConfig := &bitswap.SnifferConfig{
		Libp2pHost:  runConfig.Libp2pHost,
		Libp2pPort:  runConfig.Libp2pPort,
		DialTimeout: runConfig.ConnectionTimeout,
		Logger:      log,
		Meter:       rootConfig.MetricsProvider,
	}
	err := snifferConfig.Validate()
	if err != nil {
		return err
	}

	dhtCli, err := snifferConfig.CreateDHTClient(ctx)
	if err != nil {
		return err
	}

	sniffer, err := bitswap.NewSniffer(ctx, dhtCli, snifferConfig)
	if err != nil {
		return err
	}

	err = sniffer.Init(ctx)
	if err != nil {
		return err
	}
	return sniffer.Serve(ctx)
}
