# Bitswap Sniffer

[![ProbeLab](https://img.shields.io/badge/made%20by-ProbeLab-blue.svg)](https://probelab.io)
[![Build status](https://img.shields.io/github/actions/workflow/status/probe-lab/bitswap-sniffer/go-test.yml?branch=main)](https://github.com/probe-lab/bitswap-sniffer/actions)
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
   bitswap-sniffer

USAGE:
   bitswap-sniffer [global options] [command [command options]]

COMMANDS:
   run      Connects and scans a given node for its custody and network status
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --log.level string     Level of the logs (default: "info") [$BITSWAP_SNIFFER_LOG_LEVEL]
   --log.format string    Format of the logs [text, json] (default: "text") [$BITSWAP_SNIFFER_LOG_FORMAT]
   --metrics.host string  IP for the metrics OP host (default: "127.0.0.1") [$BITSWAP_SNIFFER_METRICS_HOST]
   --metrics.port int     Port for the metrics OP host (default: 9080) [$BITSWAP_SNIFFER_METRICS_PORT]
   --help, -h             show help
```

Run command:
```bash
NAME:
   bitswap-sniffer run - Connects and scans a given node for its custody and network status

USAGE:
   bitswap-sniffer run [options]

OPTIONS:
   --libp2p.host string           IP for the Libp2p host (default: "127.0.0.1") [$BITSWAP_SNIFFER_RUN_LIBP2P_HOST]
   --libp2p.port int              Port for the Libp2p host (default: 9020) [$BITSWAP_SNIFFER_RUN_LIBP2P_PORT]
   --connection.timeout duration  Timeout for the connection attempt to the node (default: 15s) [$BITSWAP_SNIFFER_RUN_CONNECTION_TIMEOUT]
   --cache.size int               Size for the CID cache (default: 65536) [$BITSWAP_SNIFFER_RUN_CACHE_SIZE]
   --ds.path string               Path to the LevelDB datastore (default: "./ds") [$BITSWAP_SNIFFER_RUN_LEVEL_DB]
   --discovery.interval duration  Interval between DHT peer discovery lookups (default: 1m0s) [$BITSWAP_SNIFFER_RUN_DISCOVERY_INTERVAL]
   --batcher.size int             Maximum number of items that will be cached before persisting into the DB (default: 1024) [$BITSWAP_SNIFFER_RUN_BATCHER_SIZE]
   --ch.flushers int              Number of go-routines that will be flushing cids into the DB (default: 5) [$BITSWAP_SNIFFER_RUN_CH_FLUSHERS]
   --ch.driver string             Driver of the Database that will keep all the raw data (local, replicated) (default: "local") [$BITSWAP_SNIFFER_RUN_CH_DRIVER]
   --ch.host string               Address of the Database that will keep all the raw data <ip:port> (default: "127.0.0.1:9000") [$BITSWAP_SNIFFER_RUN_CH_HOST]
   --ch.user string               User of the Database that will keep all the raw data (default: "username") [$BITSWAP_SNIFFER_RUN_CH_USER]
   --ch.password string           Password for the user of the given Database (default: "password") [$BITSWAP_SNIFFER_RUN_CH_PASSWORD]
   --ch.database string           Name of the Database that will keep all the raw data (default: "bitswap_sniffer_ipfs") [$BITSWAP_SNIFFER_RUN_CH_DATABASE]
   --ch.cluster string            Name of the Cluster that will keep all the raw data [$BITSWAP_SNIFFER_RUN_CH_CLUSTER]
   --ch.secure                    Whether we use or not use TLS while connecting clickhouse (default: false) [$BITSWAP_SNIFFER_RUN_CH_SECURE]
   --ch.engine string             CH engine that will be used for the migrations (default: "TinyLog") [$BITSWAP_SNIFFER_RUN_CH_ENGINE]
   --connections.low int          The low water mark for the connection manager. (default: 1000) [$BITSWAP_SNIFFER_RUN_CONNECTIONS_LOW]
   --connections.high int         The high water mark for the connection manager. (default: 8000) [$BITSWAP_SNIFFER_RUN_CONNECTIONS_HIGH]
   --help, -h                     show help

GLOBAL OPTIONS:
   --log.level string     Level of the logs (default: "info") [$BITSWAP_SNIFFER_LOG_LEVEL]
   --log.format string    Format of the logs [text, json] (default: "text") [$BITSWAP_SNIFFER_LOG_FORMAT]
   --metrics.host string  IP for the metrics OP host (default: "127.0.0.1") [$BITSWAP_SNIFFER_METRICS_HOST]
   --metrics.port int     Port for the metrics OP host (default: 9080) [$BITSWAP_SNIFFER_METRICS_PORT]
```

Example of how to use it:
```bash
# easy run with just its default values
just run
```

## Maintainers
[@cortze](https://github.com/cortze) from [@probe-lab](https://github.com/probe-lab)

## Contributing
Due to the debugging and research nature of the project, feedback and feature suggestions are very welcome. Feel free to open an issue or submit a pull request.
