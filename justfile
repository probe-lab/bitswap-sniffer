# Task runner. `just` lists the recipes; CI runs `just ci`.

# Recipes read the settings in .env, if it exists (cp .env.example .env).
set dotenv-load

# The lint tools, pinned here so a person and CI run the same versions.
gofumpt := "go run mvdan.cc/gofumpt@v0.12.0"
staticcheck := "go run honnef.co/go/tools/cmd/staticcheck@2026.2.2"

# The image CI pushes for a commit: bitswap-sniffer-sha<commit>. A dirty tree gets -dirty.
registry := "019120760881.dkr.ecr.us-east-1.amazonaws.com"
image := registry + "/probelab:bitswap-sniffer-sha" + `git rev-parse --short=7 HEAD` + `git diff --quiet HEAD || echo -dirty`

# List the recipes
default:
    @just --list

# Build the binary to build/bitswap-sniffer
build:
    go build -o build/bitswap-sniffer ./cmd

# Run the tests. They need ClickHouse on 127.0.0.1:9000 (just clickhouse-up)
test *args:
    go test ./... {{ args }}

# Check formatting, go fix, vet, and staticcheck without changing files
lint:
    @test -z "$({{ gofumpt }} -l .)" || { echo "not formatted, run just format:"; {{ gofumpt }} -l .; exit 1; }
    @d=$(go fix -diff ./...) && test -z "$d" || { echo "$d"; echo "go fix changes pending, run just format"; exit 1; }
    go vet ./...
    {{ staticcheck }} ./...

# Format the code and apply go fix in place
format:
    {{ gofumpt }} -w .
    go fix ./...

# Run everything CI runs
ci: lint build test

# Check, build and deploy a commit (default: newest on main) to ECS through ci.yml
deploy commit="" skip-checks="false":
    gh workflow run ci.yml -f commit={{ commit }} -f deploy=true -f skip-checks={{ skip-checks }}

# Run the sniffer with the default settings
run:
    go run ./cmd run

# Remove the build output
clean:
    rm -rf build

# Build the linux/amd64 image with the tag CI uses
docker:
    docker build --platform linux/amd64 -t {{ image }} .

# Start ClickHouse in Docker
clickhouse-up:
    docker compose up -d clickhouse

# Stop ClickHouse
clickhouse-down:
    docker compose stop clickhouse

# Remove the ClickHouse container
clickhouse-rm:
    docker compose rm clickhouse
