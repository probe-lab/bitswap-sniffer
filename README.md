# Bitswap Sniffer

[![ProbeLab](https://img.shields.io/badge/made%20by-ProbeLab-blue.svg)](https://probelab.io)
[![Build status](https://img.shields.io/github/actions/workflow/status/probe-lab/bitswap-sniffer/go-check.yml?branch=main)](https://github.com/probe-lab/bitswap-sniffer/actions)
[![Docker Image](https://img.shields.io/github/actions/workflow/status/probe-lab/bitswap-sniffer/push.yml?branch=main)](https://github.com/probe-lab/bitswap-sniffer/actions)

The `bitswap-sniffer` is a tool that, as its name describes, sniffs CIDs in the IPFS network using the Bitswap protocol. The tool attempts to connect to as many peers as possible, listening and then listing CIDs requested through IWANT messages by remote nodes.

## Requirements
- `Go >=1.24`
- (Recommended) [Just](https://github.com/casey/just)
- Access to a Clickhouse Database (the tool takes care of the schema migrations)

## Installation

We provide a `Justfile` that simplifies installation and building. Use the following commands:
```bash
# To build the tool locally -> binary at ./build/bitswap-sniffer
$ just build
```

## Usage

The tool exposes a single CLI command. Basic usage:
```bash
$ ./build/bitswap-sniffer --help

NAME:
   bitsniffer - Connects to the IPFS DHT and sniffs bitswap traffic

USAGE:
   bitsniffer [global options] [command [command options]]

COMMANDS:
   run      Connects and scans a given node for its custody and network status
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --help, -h  show help

   Logging Configuration:

   --log.format string  Sets the format to output the log statements in: text, json (default: "text") [$BITSNIFFER_LOG_FORMAT]
   --log.level string   Sets an explicit logging level: debug, info, warn, error. (default: "info") [$BITSNIFFER_LOG_LEVEL]
   --log.source         Compute the source code position of a log statement and add a SourceKey attribute to the output. [$BITSNIFFER_LOG_SOURCE]

   Telemetry Configuration:

   --aws.region string    The AWS region that this service runs in. [$AWS_REGION]
   --metrics.enabled      Whether to expose metrics information [$BITSNIFFER_METRICS_ENABLED]
   --metrics.host string  Which network interface should the metrics endpoint bind to (default: "127.0.0.1") [$BITSNIFFER_METRICS_HOST]
   --metrics.path string  On which path should the metrics endpoint listen (default: "/metrics") [$BITSNIFFER_METRICS_PATH]
   --metrics.port int     On which port should the metrics endpoint listen (default: 9080) [$BITSNIFFER_METRICS_PORT]
   --tracing.enabled      Whether to emit trace data [$BITSNIFFER_TRACING_ENABLED]
```

Run command:
```bash
NAME:
   bitsniffer run - Connects and scans a given node for its custody and network status

USAGE:
   bitsniffer run [options]

OPTIONS:
   --batcher.size int             Maximum number of items that will be cached before persisting into the DB (default: 1024) [$BITSNIFFER_BATCHER_SIZE]
   --cache.size int               Size for the CID cache (default: 0) [$BITSNIFFER_CACHE_SIZE]
   --ch.flushers int              Number of go-routines that will be flushing cids into the DB (default: 5) [$BITSNIFFER_CH_FLUSHERS]
   --connection.timeout duration  Timeout for the connection attempt to the node (default: 15s) [$BITSNIFFER_CONNECTION_TIMEOUT]
   --connections.high int         The high water mark for the connection manager. (default: 8000) [$BITSNIFFER_CONNECTIONS_HIGH]
   --connections.low int          The low water mark for the connection manager. (default: 1000) [$BITSNIFFER_CONNECTIONS_LOW]
   --discovery.interval duration  Interval between dht peer discovery lookups (default: 1m0s) [$BITSNIFFER_DISCOVERY_INTERVAL]
   --ds.path string               Path to the LevelDB datastore (default: "./ds") [$BITSNIFFER_LEVEL_DB]
   --help, -h                     show help
   --libp2p.host string           IP for the Libp2p host (default: "127.0.0.1") [$BITSNIFFER_LIBP2P_HOST]
   --libp2p.port int              Port for the Libp2p host (default: 9020) [$BITSNIFFER_LIBP2P_PORT]

   Database Configuration:

   --clickhouse.cluster string                        The cluster name of the Clickhouse service. [$BITSNIFFER_CLICKHOUSE_CLUSTER]
   --clickhouse.database string                       The ClickHouse database name to connect to (default: "bitswap_sniffer_db") [$BITSNIFFER_CLICKHOUSE_DATABASE]
   --clickhouse.host string                           The address where ClickHouse is hosted (default: "127.0.0.1") [$BITSNIFFER_CLICKHOUSE_HOST]
   --clickhouse.migrations.multiStatement             Whether to use multi-statement mode when applying migrations. [$BITSNIFFER_CLICKHOUSE_MIGRATIONS_MULTI_STATEMENT]
   --clickhouse.migrations.multiStatementMaxSize int  The maximum size of a multi-statement. (default: 10485760) [$BITSNIFFER_CLICKHOUSE_MIGRATIONS_MULTI_STATEMENT_MAX_SIZE]
   --clickhouse.migrations.replicatedTableEngines     Whether to use replicated table engines. [$BITSNIFFER_CLICKHOUSE_MIGRATIONS_REPLICATED_TABLE_ENGINES]
   --clickhouse.migrationsTable string                The name of the migrations table. (default: "schema_migrations") [$BITSNIFFER_CLICKHOUSE_MIGRATIONS_TABLE]
   --clickhouse.migrationsTableEngine string          The engine of the migrations table. (default: "TinyLog") [$BITSNIFFER_CLICKHOUSE_MIGRATIONS_TABLE_ENGINE]
   --clickhouse.password string                       The password for the ClickHouse user (default: "password") [$BITSNIFFER_CLICKHOUSE_PASSWORD]
   --clickhouse.port int                              Port at which the ClickHouse database is accessible (default: 9000) [$BITSNIFFER_CLICKHOUSE_PORT]
   --clickhouse.ssl                                   Whether to use SSL to connect to ClickHouse [$BITSNIFFER_CLICKHOUSE_SSL]
   --clickhouse.user string                           The ClickHouse user that has the right privileges (default: "username") [$BITSNIFFER_CLICKHOUSE_USER]

GLOBAL OPTIONS:
   --log.level string     Sets an explicit logging level: debug, info, warn, error. (default: "info") [$BITSNIFFER_LOG_LEVEL]
   --log.format string    Sets the format to output the log statements in: text, json (default: "text") [$BITSNIFFER_LOG_FORMAT]
   --log.source           Compute the source code position of a log statement and add a SourceKey attribute to the output. [$BITSNIFFER_LOG_SOURCE]
   --metrics.enabled      Whether to expose metrics information [$BITSNIFFER_METRICS_ENABLED]
   --metrics.host string  Which network interface should the metrics endpoint bind to (default: "127.0.0.1") [$BITSNIFFER_METRICS_HOST]
   --metrics.port int     On which port should the metrics endpoint listen (default: 9080) [$BITSNIFFER_METRICS_PORT]
   --metrics.path string  On which path should the metrics endpoint listen (default: "/metrics") [$BITSNIFFER_METRICS_PATH]
   --tracing.enabled      Whether to emit trace data [$BITSNIFFER_TRACING_ENABLED]
   --aws.region string    The AWS region that this service runs in. [$AWS_REGION]
```

> Note: `clickhouse.migrations.replicatedTableEngines` replaces the old `ch.driver local|replicated` flag (`--clickhouse.migrations.replicatedTableEngines` is equivalent to `ch.driver=replicated`).

Example of how to use it:
```bash
# easy run with just its default values
just run
```

## Maintainers
[@cortze](https://github.com/cortze) from [@probe-lab](https://github.com/probe-lab)

## Contributing
Due to the debugging and research nature of the project, feedback and feature suggestions are very welcome. Feel free to open an issue or submit a pull request.
