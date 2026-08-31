package main

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/urfave/cli/v3"

	plcli "github.com/probe-lab/go-commons/cli"
)

// rootConfig is populated by plcli.NewRootCommand before rootCmd.Before runs.
var rootConfig *plcli.RootCommandConfig

var rootCmd = &cli.Command{
	Name:  "bitsniffer",
	Usage: "Connects to the IPFS DHT and sniffs bitswap traffic",
	Commands: []*cli.Command{
		cmdRun,
	},
}

func main() {
	var rootApp *plcli.RootCommand
	rootApp, rootConfig = plcli.NewRootCommand(rootCmd)

	if err := rootApp.Run(); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("terminated abnormally", "err", err)
		os.Exit(1)
	}
}
