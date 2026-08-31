package main

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/urfave/cli/v3"

	plcli "github.com/probe-lab/go-commons/cli"
)

var rootCmd = &cli.Command{
	Name:  "bitsniffer",
	Usage: "Connects to the IPFS DHT and sniffs bitswap traffic",
	Commands: []*cli.Command{
		cmdRun,
	},
}

func main() {
	rootApp, rootConfig := plcli.NewRootCommand(rootCmd)

	// preserve this app's existing metrics defaults (always-on, matches
	// prometheus/prometheus.yml's scrape target and current deployments).
	rootConfig.Metrics.Enabled = true
	rootConfig.Metrics.Host = "127.0.0.1"
	rootConfig.Metrics.Port = 9080

	if err := rootApp.Run(); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("terminated abnormally", "err", err)
		os.Exit(1)
	}
}
