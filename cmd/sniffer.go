package main

import (
	"context"

	"github.com/probe-lab/bitswap-sniffer/bitswap"
	"github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v3"
)

var runConfig = struct {
	Libp2pHost string
	Libp2pPort int
}{
	Libp2pHost: "127.0.0.1",
	Libp2pPort: 9020,
}

var cmdRun = &cli.Command{
	Name:                  "run",
	Usage:                 "Connects and scans a given node for its custody and network status",
	EnableShellCompletion: true,
	Action:                scanAction,
	Flags:                 scanFlags,
}

var scanFlags = []cli.Flag{
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
}

func scanAction(ctx context.Context, cmd *cli.Command) error {
	log := rootConfig.Logger
	rootConfig.Logger.WithFields(logrus.Fields{
		"libp2p-host":  runConfig.Libp2pHost,
		"libp2p-port":  runConfig.Libp2pPort,
		"metrics-host": rootConfig.MetricsHost,
		"metrics-port": rootConfig.MetricsPort,
	}).Info("running bitswap-sniffer")

	snifferConfig := &bitswap.SnifferConfig{
		Logger:     log,
		Meter:      rootConfig.MetricsProvider,
		Libp2pHost: runConfig.Libp2pHost,
		Libp2pPort: runConfig.Libp2pPort,
	}
	err := snifferConfig.Validate()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dhtCli, err := snifferConfig.DHTClient(ctx)
	if err != nil {
		return err
	}

	sniffer, err := bitswap.NewSniffer(ctx, log, dhtCli)
	if err != nil {
		return err
	}

	err = sniffer.Init(ctx)
	if err != nil {
		return err
	}
	return sniffer.Serve(ctx)
}
