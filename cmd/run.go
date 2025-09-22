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
	CacheSize         int
	BatcherSize       int
	Flushers          int
	ChDriver          string
	ChHost            string
	ChUser            string
	ChPassword        string
	ChDatabase        string
	ChCluster         string
	ChMigrationEngine string
	ChSecure          bool
}{
	Libp2pHost:        "127.0.0.1",
	Libp2pPort:        9020,
	ConnectionTimeout: 10 * time.Second,
	CacheSize:         65_536, // arbitrary number
	BatcherSize:       1_024,  // arbitrary number
	Flushers:          1,
	ChDriver:          "local",
	ChHost:            "127.0.0.1:9000",
	ChUser:            "username",
	ChPassword:        "password",
	ChDatabase:        "bitswap_sniffer_db",
	ChCluster:         "",
	ChMigrationEngine: "TinyLog",
	ChSecure:          false,
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
		Sources:     cli.EnvVars("BITSWAP_SNIFFER_RUN_LIBP2P_HOST"),
	},
	&cli.IntFlag{
		Name:        "libp2p.port",
		Usage:       "Port for the Libp2p host",
		Value:       runConfig.Libp2pPort,
		Destination: &runConfig.Libp2pPort,
		Sources:     cli.EnvVars("BITSWAP_SNIFFER_RUN_LIBP2P_PORT"),
	},
	&cli.DurationFlag{
		Name:        "connection.timeout",
		Usage:       "Timeout for the connection attempt to the node",
		Value:       runConfig.ConnectionTimeout,
		Destination: &runConfig.ConnectionTimeout,
		Sources:     cli.EnvVars("BITSWAP_SNIFFER_RUN_CONNECTION_TIMEOUT"),
	},
	&cli.IntFlag{
		Name:        "cache.size",
		Usage:       "Size for the CID cache",
		Value:       runConfig.CacheSize,
		Destination: &runConfig.CacheSize,
		Sources:     cli.EnvVars("BITSWAP_SNIFFER_RUN_CACHE_SIZE"),
	},
	&cli.IntFlag{
		Name:        "batcher.size",
		Usage:       "Maximum number of items that will be cached before persisting into the DB",
		Value:       runConfig.BatcherSize,
		Destination: &runConfig.BatcherSize,
		Sources:     cli.EnvVars("BITSWAP_SNIFFER_RUN_BATCHER_SIZE"),
	},
	&cli.IntFlag{
		Name:        "ch.flushers",
		Usage:       "Number of go-routines that will be flushing cids into the DB",
		Value:       runConfig.Flushers,
		Destination: &runConfig.Flushers,
		Sources:     cli.EnvVars("BITSWAP_SNIFFER_RUN_CH_FLUSHERS"),
	},
	&cli.StringFlag{
		Name:        "ch.driver",
		Usage:       "Driver of the Database that will keep all the raw data (local, replicated)",
		Value:       runConfig.ChDriver,
		Destination: &runConfig.ChDriver,
		Sources:     cli.EnvVars("BITSWAP_SNIFFER_RUN_CH_DRIVER"),
	},
	&cli.StringFlag{
		Name:        "ch.host",
		Usage:       "Address of the Database that will keep all the raw data <ip:port>",
		Value:       runConfig.ChHost,
		Destination: &runConfig.ChHost,
		Sources:     cli.EnvVars("BITSWAP_SNIFFER_RUN_CH_HOST"),
	},
	&cli.StringFlag{
		Name:        "ch.user",
		Usage:       "User of the Database that will keep all the raw data",
		Value:       runConfig.ChUser,
		Destination: &runConfig.ChUser,
		Sources:     cli.EnvVars("BITSWAP_SNIFFER_RUN_CH_USER"),
	},
	&cli.StringFlag{
		Name:        "ch.password",
		Usage:       "Password for the user of the given Database",
		Value:       runConfig.ChUser,
		Destination: &runConfig.ChUser,
		Sources:     cli.EnvVars("BITSWAP_SNIFFER_RUN_CH_PASSWORD"),
	},
	&cli.StringFlag{
		Name:        "ch.database",
		Usage:       "Name of the Database that will keep all the raw data",
		Value:       runConfig.ChUser,
		Destination: &runConfig.ChUser,
		Sources:     cli.EnvVars("BITSWAP_SNIFFER_RUN_CH_DATABASE"),
	},
	&cli.StringFlag{
		Name:        "ch.cluster",
		Usage:       "Name of the Cluster that will keep all the raw data",
		Value:       runConfig.ChCluster,
		Destination: &runConfig.ChCluster,
		Sources:     cli.EnvVars("BITSWAP_SNIFFER_RUN_CH_CLUSTER"),
	},
	&cli.BoolFlag{
		Name:        "ch.secure",
		Usage:       "Whether we use or not use of TLS while connecting clickhouse",
		Value:       runConfig.ChSecure,
		Destination: &runConfig.ChSecure,
		Sources:     cli.EnvVars("BITSWAP_SNIFFER_RUN_CH_SECURE"),
	},
}

func scanAction(ctx context.Context, cmd *cli.Command) error {
	log := rootConfig.Logger
	rootConfig.Logger.WithFields(logrus.Fields{
		"libp2p-host":        runConfig.Libp2pHost,
		"libp2p-port":        runConfig.Libp2pPort,
		"connection-timeout": runConfig.ConnectionTimeout,
		"cache-size":         runConfig.CacheSize,
		"batcher-size":       runConfig.BatcherSize,
		"ch-flushers":        runConfig.Flushers,
		"ch-driver":          runConfig.ChDriver,
		"ch-host":            runConfig.ChHost,
		"ch-user":            runConfig.ChUser,
		"ch-database":        runConfig.ChDatabase,
		"ch-cluster":         runConfig.ChCluster,
		"ch-secure":          runConfig.ChSecure,
	}).Info("running run command...")

	snifferConfig := &bitswap.SnifferConfig{
		Libp2pHost:  runConfig.Libp2pHost,
		Libp2pPort:  runConfig.Libp2pPort,
		DialTimeout: runConfig.ConnectionTimeout,
		CacheSize:   runConfig.CacheSize,
		Logger:      log,
		Telemetry:   rootConfig.MetricsProvider,
	}
	err := snifferConfig.Validate()
	if err != nil {
		return err
	}

	dhtCli, err := snifferConfig.CreateDHTClient(ctx)
	if err != nil {
		return err
	}

	conDetails := &bitswap.ChConfig{
		Driver:          runConfig.ChDriver,
		Host:            runConfig.ChHost,
		User:            runConfig.ChUser,
		Password:        runConfig.ChPassword,
		Database:        runConfig.ChDatabase,
		Cluster:         runConfig.ChCluster,
		MigrationEngine: runConfig.ChMigrationEngine,
		Secure:          runConfig.ChSecure,
		BatchSize:       runConfig.BatcherSize,
		Flushers:        runConfig.Flushers,
		Telemetry:       rootConfig.MetricsProvider,
	}
	chCli, err := bitswap.NewClickhouseDB(conDetails, log)
	if err != nil {
		return err
	}

	sniffer, err := bitswap.NewSniffer(ctx, snifferConfig, dhtCli, chCli)
	if err != nil {
		return err
	}

	err = sniffer.Init(ctx)
	if err != nil {
		return err
	}
	return sniffer.Serve(ctx)
}
