# Bitswap Sniffer

[![ProbeLab](https://img.shields.io/badge/made%20by-ProbeLab-blue.svg)](https://probelab.io)
[![CI](https://img.shields.io/github/actions/workflow/status/probe-lab/bitswap-sniffer/ci.yml?branch=main)](https://github.com/probe-lab/bitswap-sniffer/actions/workflows/ci.yml)

`bitswap-sniffer` records the CIDs that peers of the IPFS network ask for and announce. It runs a libp2p host with a DHT client and Bitswap, connects to as many peers as it can, and logs every CID in the Bitswap messages it sends and receives (`want`, `have`, `dont-have`, `block`) and in DHT provider record requests. The CIDs go to the ClickHouse table `shared_cids`. The tool applies the schema migrations itself.

## Quickstart

You need Go 1.26, [just](https://github.com/casey/just) and Docker.

```sh
cp .env.example .env   # local settings that match docker-compose.yml
just clickhouse-up     # ClickHouse on 127.0.0.1:9000
just run               # go run ./cmd run
```

`just build` writes the binary to `build/bitswap-sniffer`. `./build/bitswap-sniffer run --help` lists the flags.

## Configuration

Every flag has an environment variable with the prefix `BITSNIFFER_`. `.env.example` lists them all.

| Flag | Environment variable | Default | Description |
| --- | --- | --- | --- |
| `--libp2p.host` | `BITSNIFFER_LIBP2P_HOST` | `127.0.0.1` | IP of the libp2p host |
| `--libp2p.port` | `BITSNIFFER_LIBP2P_PORT` | `9020` | Port of the libp2p host |
| `--connection.timeout` | `BITSNIFFER_CONNECTION_TIMEOUT` | `15s` | Timeout of a connection attempt |
| `--connections.low` | `BITSNIFFER_CONNECTIONS_LOW` | `1000` | Low watermark of the connection manager |
| `--connections.high` | `BITSNIFFER_CONNECTIONS_HIGH` | `8000` | High watermark of the connection manager |
| `--discovery.interval` | `BITSNIFFER_DISCOVERY_INTERVAL` | `1m0s` | Interval between DHT peer discovery lookups |
| `--cache.size` | `BITSNIFFER_CACHE_SIZE` | `0` | Size of the CID cache |
| `--batcher.size` | `BITSNIFFER_BATCHER_SIZE` | `1024` | Rows to collect before a write to ClickHouse |
| `--ds.path` | `BITSNIFFER_LEVEL_DB` | `./ds` | Path of the LevelDB datastore |
| `--clickhouse.host` | `BITSNIFFER_CLICKHOUSE_HOST` | `127.0.0.1` | ClickHouse host |
| `--clickhouse.port` | `BITSNIFFER_CLICKHOUSE_PORT` | `9000` | ClickHouse native port |
| `--clickhouse.user` | `BITSNIFFER_CLICKHOUSE_USER` | `bitswap_sniffer_db` | ClickHouse user |
| `--clickhouse.password` | `BITSNIFFER_CLICKHOUSE_PASSWORD` | `password` | ClickHouse password |
| `--clickhouse.database` | `BITSNIFFER_CLICKHOUSE_DATABASE` | `bitswap_sniffer_db` | ClickHouse database |
| `--clickhouse.ssl` | `BITSNIFFER_CLICKHOUSE_SSL` | `false` | Connect to ClickHouse with TLS |
| `--clickhouse.cluster` | `BITSNIFFER_CLICKHOUSE_CLUSTER` | | ClickHouse cluster name |
| `--clickhouse.migrationsTable` | `BITSNIFFER_CLICKHOUSE_MIGRATIONS_TABLE` | `schema_migrations` | Table that records the applied migrations |
| `--clickhouse.migrationsTableEngine` | `BITSNIFFER_CLICKHOUSE_MIGRATIONS_TABLE_ENGINE` | `TinyLog` | Engine of the migrations table |
| `--clickhouse.migrations.multiStatement` | `BITSNIFFER_CLICKHOUSE_MIGRATIONS_MULTI_STATEMENT` | `false` | Apply migrations in multi-statement mode |
| `--clickhouse.migrations.multiStatementMaxSize` | `BITSNIFFER_CLICKHOUSE_MIGRATIONS_MULTI_STATEMENT_MAX_SIZE` | `10485760` | Maximum size of a multi-statement |
| `--clickhouse.migrations.replicatedTableEngines` | `BITSNIFFER_CLICKHOUSE_MIGRATIONS_REPLICATED_TABLE_ENGINES` | `false` | Use `Replicated*` table engines. Production sets `true`. |
| `--log.level` | `BITSNIFFER_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `--log.format` | `BITSNIFFER_LOG_FORMAT` | `text` | `text` or `json` |
| `--log.source` | `BITSNIFFER_LOG_SOURCE` | `false` | Add the source position to each log line |
| `--metrics.enabled` | `BITSNIFFER_METRICS_ENABLED` | `false` | Serve Prometheus metrics |
| `--metrics.host` | `BITSNIFFER_METRICS_HOST` | `localhost` | Interface of the metrics endpoint |
| `--metrics.port` | `BITSNIFFER_METRICS_PORT` | `6060` | Port of the metrics endpoint |
| `--metrics.path` | `BITSNIFFER_METRICS_PATH` | `/metrics` | Path of the metrics endpoint |
| `--tracing.enabled` | `BITSNIFFER_TRACING_ENABLED` | `false` | Emit traces |
| `--aws.region` | `AWS_REGION` | | AWS region the service runs in |

## Metrics

With `BITSNIFFER_METRICS_ENABLED=true`, the sniffer serves Prometheus metrics on `BITSNIFFER_METRICS_HOST:BITSNIFFER_METRICS_PORT` at `/metrics`. The names carry the prefix `bitsniffer_`. Units are not set yet, and some names do not follow the ProbeLab naming rules.

| Metric | Type | Unit | Labels | Description |
| --- | --- | --- | --- | --- |
| `bitsniffer_total_seen_cids_total` | counter | none | `direction`: `sent`, `received`; `origin`: `bitswap`, `dht`; `msg_type`: `want`, `have`, `dont-have`, `block`, `add-provider-records`, `get-provider-records` | CIDs seen |
| `bitsniffer_unique_cids_by_msg_type_total` | counter | none | none | CIDs not yet in the CID cache |
| `bitsniffer_bitswap_peer_count` | gauge | none | none | Peers connected at the Bitswap level |
| `bitsniffer_bitswap_stats` | gauge | none | `type`: `sent`, `received` | Bitswap message stats. `type="sent"` holds the received message count and `type="received"` the sent data size (`bitswap/sniffer.go`). |
| `bitsniffer_bitswap_want_list` | gauge | none | `type`: `cid-want-list` | CIDs in the sniffer's own want-list |
| `bitsniffer_datastore_disk_usage` | gauge | bytes, not declared | none | Disk usage of the LevelDB datastore |
| `bitsniffer_lookups_total` | counter | none | `success`: `true`, `false` | DHT peer discovery lookups |
| `bitsniffer_batch_inserter_rows_flushed_total` | counter | none | `table` | Rows written to ClickHouse |
| `bitsniffer_batch_inserter_rows_dropped_total` | counter | none | `table` | Rows dropped by a failed write |
| `bitsniffer_batch_inserter_flush_duration_seconds` | histogram | seconds | `table`, `trigger` | Duration of a write |
| `bitsniffer_batch_inserter_flush_size` | histogram | none | `table`, `trigger` | Rows in a write |

The `batch_inserter` metrics come from the ClickHouse batch inserter of go-commons. No Grafana dashboard is linked here yet.

## Development

| Command | Does |
| --- | --- |
| `just` | Lists the recipes |
| `just build` | Builds `build/bitswap-sniffer` |
| `just test` | Runs the tests. They need ClickHouse on `127.0.0.1:9000` with user `username`, password `password` and database `bitswap_sniffer_ipfs`. |
| `just lint` | Checks formatting, `go fix`, `go vet` and `staticcheck`. The `justfile` pins the tool versions. |
| `just format` | Formats the code and applies `go fix` |
| `just ci` | Runs `lint`, `build` and `test`, as CI does |
| `just run` | Runs the sniffer with the settings in `.env` |
| `just docker` | Builds the linux/amd64 image with the tag CI uses |
| `just clickhouse-up`, `just clickhouse-down`, `just clickhouse-rm` | Start, stop and remove the local ClickHouse |

`docker compose up -d prometheus` starts a local Prometheus that scrapes `127.0.0.1:9080`, the metrics port in `.env.example`.

## Deployment

ProbeLab runs one instance for the IPFS mainnet as the ECS service `prod-use1-cmi-bitsniffer` in the cluster `prod-use1-cmi-ecs-cluster` (us-east-1). It writes to the ClickHouse database `bitswap_sniffer_ipfs`. The service and its task definition are in [probelab-infra](https://github.com/probe-lab/probelab-infra): `tf/modules/bitswap_sniffer` and `tf/bitsniffer.tf`.

`ci.yml` builds the image `bitswap-sniffer-sha<commit>` of every merge to `main`. A merge deploys when its pull request has the label `deploy`. To deploy any commit by hand, run `just deploy <commit>` or start the CI workflow from the Actions tab with the flag `deploy`.

## Maintainers

[@cortze](https://github.com/cortze) from [@probe-lab](https://github.com/probe-lab)

## License

[PolyForm Strict License 1.0.0](LICENSE)
