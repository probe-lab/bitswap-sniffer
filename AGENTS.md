# bitswap-sniffer

ProbeLab repository. Follow the `probelab` skill: it holds the company rules. Load it before you start any task here.

`bitswap-sniffer` records the CIDs that peers of the IPFS network ask for and announce. It runs a libp2p host with a DHT client and Bitswap, connects to as many peers as it can (connection manager watermarks 1000 to 8000), and traces every Bitswap message (`want`, `have`, `dont-have`, `block`) and DHT provider record request. Each CID it sees goes to the ClickHouse table `shared_cids`. ProbeLab runs one instance for the IPFS mainnet, and tiros and data-platform read its table.

## Layout

- `cmd/` — CLI: the root command `bitsniffer` (`cmd.go`) and the `run` command with its flags (`run.go`)
- `bitswap/` — package `bitswap`: the sniffer and its libp2p host (`sniffer.go`, `config.go`), DHT peer discovery (`discovery.go`), the Bitswap and DHT message tracer (`tracer.go`), the `SharedCid` row and its queries (`cid.go`), the ClickHouse writer (`db.go`), metrics (`metrics.go`)
- `bitswap/migrations/` — ClickHouse migrations, embedded in the binary
- `docker-compose.yml`, `prometheus/` — local ClickHouse and Prometheus
- `ds/` — the LevelDB datastore that a local run creates (`BITSNIFFER_LEVEL_DB`); not in git

## Commands

- `just build`, `just test`, `just lint`, `just format`, `just ci` (the checks CI runs)
- `just test` needs ClickHouse on `127.0.0.1:9000`, user `username`, password `password`, database `bitswap_sniffer_ipfs`. CI starts it as a service container. Locally, `just clickhouse-up` starts ClickHouse from `docker-compose.yml`.
- `cp .env.example .env`, then `just run` runs the sniffer against the local ClickHouse
- `just docker` builds the image as `bitswap-sniffer-sha<commit>`
- `just deploy [commit]` deploys a commit to ECS through `ci.yml`

## Conventions

- Environment variables have the prefix `BITSNIFFER_`, from the command name `bitsniffer`, not from the repo name.
- A schema change is a new numbered `up`/`down` pair in `bitswap/migrations/`. The sniffer applies migrations on startup. Production sets `BITSNIFFER_CLICKHOUSE_MIGRATIONS_REPLICATED_TABLE_ENGINES=true`.
- The package is not yet under `pkg/`, and `bitswap/cid_test.go` uses testify. Keep new code in `bitswap/` until it moves.
- The image tag is `bitswap-sniffer-sha<commit>`. `tf/bitsniffer.tf` in `probelab-infra` pins it.

## Related repositories

- `tiros` — reads `bitswap_sniffer_ipfs.shared_cids` to pick CIDs to fetch (`pkg/cid_provider.go`)
- `data-platform` — analyses the table in `src/etl/tools/cidsniffer`
- `probelab-infra` — the ECS service and task definition (`tf/modules/bitswap_sniffer`, `tf/bitsniffer.tf`)
- `go-commons` — CLI setup, logging, metrics, ClickHouse connection, migrations and batch inserts

## Owner

@probe-lab/probelab-core
