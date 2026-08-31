package main

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/probe-lab/bitswap-sniffer/bitswap"
	plcli "github.com/probe-lab/go-commons/cli"
	"github.com/probe-lab/go-commons/db"
	commonlog "github.com/probe-lab/go-commons/log"
	cli "github.com/urfave/cli/v3"
	"go.opentelemetry.io/otel"
)

const envPrefix = "BITSNIFFER_"

var runConfig = struct {
	Libp2pHost        string
	Libp2pPort        int
	ConnectionTimeout time.Duration
	CacheSize         int
	BatcherSize       int
	LevelDB           string
	DiscoveryInterval time.Duration
	ConnectionsLow    int
	ConnectionsHigh   int
	ClickhouseConfig  *db.ClickHouseConfig
	MigrationsConfig  *db.ClickHouseMigrationsConfig
}{
	Libp2pHost:        "127.0.0.1",
	Libp2pPort:        9020,
	ConnectionTimeout: 15 * time.Second,
	CacheSize:         0,
	BatcherSize:       1_024,
	LevelDB:           "./ds",
	DiscoveryInterval: 1 * time.Minute,
	ConnectionsLow:    1_000,
	ConnectionsHigh:   8_000,
	ClickhouseConfig:  db.DefaultClickHouseConfig("bitswap_sniffer_db"),
	MigrationsConfig:  db.DefaultClickHouseMigrationsConfig(),
}

var cmdRun = &cli.Command{
	Name:   "run",
	Usage:  "Connects and scans a given node for its custody and network status",
	Action: scanAction,
	Flags: slices.Concat(
		plcli.ClickHouseFlags(envPrefix, runConfig.ClickhouseConfig),
		plcli.ClickHouseMigrationsFlags(envPrefix, runConfig.MigrationsConfig),
		runFlags,
	),
}

var runFlags = []cli.Flag{
	&cli.StringFlag{
		Name:        "libp2p.host",
		Usage:       "IP for the Libp2p host",
		Value:       runConfig.Libp2pHost,
		Destination: &runConfig.Libp2pHost,
		Sources:     cli.EnvVars(envPrefix + "LIBP2P_HOST"),
	},
	&cli.IntFlag{
		Name:        "libp2p.port",
		Usage:       "Port for the Libp2p host",
		Value:       runConfig.Libp2pPort,
		Destination: &runConfig.Libp2pPort,
		Sources:     cli.EnvVars(envPrefix + "LIBP2P_PORT"),
	},
	&cli.DurationFlag{
		Name:        "connection.timeout",
		Usage:       "Timeout for the connection attempt to the node",
		Value:       runConfig.ConnectionTimeout,
		Destination: &runConfig.ConnectionTimeout,
		Sources:     cli.EnvVars(envPrefix + "CONNECTION_TIMEOUT"),
	},
	&cli.IntFlag{
		Name:        "cache.size",
		Usage:       "Size for the CID cache",
		Value:       runConfig.CacheSize,
		Destination: &runConfig.CacheSize,
		Sources:     cli.EnvVars(envPrefix + "CACHE_SIZE"),
	},
	&cli.StringFlag{
		Name:        "ds.path",
		Usage:       "Path to the LevelDB datastore",
		Value:       runConfig.LevelDB,
		Destination: &runConfig.LevelDB,
		Sources:     cli.EnvVars(envPrefix + "LEVEL_DB"),
	},
	&cli.DurationFlag{
		Name:        "discovery.interval",
		Usage:       "Interval between dht peer discovery lookups",
		Value:       runConfig.DiscoveryInterval,
		Destination: &runConfig.DiscoveryInterval,
		Sources:     cli.EnvVars(envPrefix + "DISCOVERY_INTERVAL"),
	},
	&cli.IntFlag{
		Name:        "batcher.size",
		Usage:       "Maximum number of items that will be cached before persisting into the DB",
		Value:       runConfig.BatcherSize,
		Destination: &runConfig.BatcherSize,
		Sources:     cli.EnvVars(envPrefix + "BATCHER_SIZE"),
	},
	&cli.IntFlag{
		Name:        "connections.low",
		Usage:       "The low water mark for the connection manager.",
		Value:       runConfig.ConnectionsLow,
		Destination: &runConfig.ConnectionsLow,
		Sources:     cli.EnvVars(envPrefix + "CONNECTIONS_LOW"),
	},
	&cli.IntFlag{
		Name:        "connections.high",
		Usage:       "The high water mark for the connection manager.",
		Value:       runConfig.ConnectionsHigh,
		Destination: &runConfig.ConnectionsHigh,
		Sources:     cli.EnvVars(envPrefix + "CONNECTIONS_HIGH"),
	},
}

func scanAction(ctx context.Context, cmd *cli.Command) error {

	if err := commonlog.SetGlobalLogger(rootConfig.Log); err != nil {
		return err
	}

	slog.Info("running run command...",
		"libp2p-host", runConfig.Libp2pHost,
		"libp2p-port", runConfig.Libp2pPort,
		"connection-timeout", runConfig.ConnectionTimeout,
		"cache-size", runConfig.CacheSize,
		"batcher-size", runConfig.BatcherSize,
		"level-db", runConfig.LevelDB,
		"discv-interval", runConfig.DiscoveryInterval,
		"ch-host", runConfig.ClickhouseConfig.BaseConfig.Host,
		"ch-port", runConfig.ClickhouseConfig.BaseConfig.Port,
		"ch-user", runConfig.ClickhouseConfig.BaseConfig.User,
		"ch-database", runConfig.ClickhouseConfig.Database,
		"ch-cluster", runConfig.MigrationsConfig.ClusterName,
		"ch-secure", runConfig.ClickhouseConfig.BaseConfig.SSL,
		"ch-engine", runConfig.MigrationsConfig.MigrationsTableEngine,
	)

	snifferConfig := &bitswap.SnifferConfig{
		Libp2pHost:        runConfig.Libp2pHost,
		Libp2pPort:        runConfig.Libp2pPort,
		ConnectionsLow:    runConfig.ConnectionsLow,
		ConnectionsHigh:   runConfig.ConnectionsHigh,
		DialTimeout:       runConfig.ConnectionTimeout,
		DiscoveryInterval: runConfig.DiscoveryInterval,
		CacheSize:         runConfig.CacheSize,
		LevelDB:           runConfig.LevelDB,
		Telemetry:         otel.GetMeterProvider(),
	}
	err := snifferConfig.Validate()
	if err != nil {
		return errors.Wrap(err, "validating conf")
	}

	ds, err := snifferConfig.CreateDatastore(ctx)
	if err != nil {
		return errors.Wrap(err, "creating leveldb datastore")
	}

	conDetails := &bitswap.ChConfig{
		ClickHouseConfig:           *runConfig.ClickhouseConfig,
		ClickHouseMigrationsConfig: *runConfig.MigrationsConfig,
		BatchSize:                  runConfig.BatcherSize,
		Telemetry:                  otel.GetMeterProvider(),
	}
	chCli, err := bitswap.NewClickhouseDB(conDetails)
	if err != nil {
		return errors.Wrap(err, "opening ch db")

	}

	sniffer, err := bitswap.NewSniffer(ctx, snifferConfig, ds, chCli)
	if err != nil {
		return errors.Wrap(err, "creating bitswap sniffer")
	}

	err = sniffer.Init(ctx)
	if err != nil {
		return errors.Wrap(err, "init bitswap sniffer")
	}
	return sniffer.Serve(ctx)
}
