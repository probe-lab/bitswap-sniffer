GOCC := "go"
BIN := "./cmd"
TARGET_PATH := "./build/bitswap-sniffer"


default:
    @just --list --justfile {{ justfile() }}

run:
    {{GOCC}} run {{BIN}} run

build:
	{{GOCC}} build -o {{TARGET_PATH}} {{BIN}}

clean:
	@rm -r $(BIN_PATH)

format:
	{{GOCC}} fmt ./...
	{{GOCC}} mod tidy -v

lint:
	{{GOCC}} mod verify
	{{GOCC}} vet ./...
	{{GOCC}} run honnef.co/go/tools/cmd/staticcheck@latest ./...
	{{GOCC}} test -race -buildvcs -vet=off ./...

# generates clickhouse migrations which work with a local docker deployment
generate-local-clickhouse-migrations:
	#!/usr/bin/env bash
	OUTDIR=bitswap/migrations/local
	mkdir -p $OUTDIR
	for file in $(find bitswap/migrations/replicated -maxdepth 1 -name "*.sql"); do
	  filename=$(basename $file)
	  echo "Generating $OUTDIR/$filename"

	  # The "Replicated" variants don't work with a singular clickhouse deployment
	  # We're stripping that part from the file
	  sed 's/Replicated//' $file > $OUTDIR/$filename.tmp_0

	  # Enabling the JSON type is also different in both environments
	  # allow_experimental_json_type in ClickHouse Cloud
	  # enable_json_type locally
	  sed 's/allow_experimental_json_type/enable_json_type/' $OUTDIR/$filename.tmp_0 > $OUTDIR/$filename.tmp_1

	  # Add a warning message to the top of the file
	  cat <(echo -e "-- DO NOT EDIT: This file was generated with: just generate-local-clickhouse-migrations\n") $OUTDIR/$filename.tmp_1 > $OUTDIR/$filename
	  rm $OUTDIR/$filename.tmp*
	done
