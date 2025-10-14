GOCC := "go"
BIN := "./cmd"
TARGET_PATH := "./build/bitswap-sniffer"

GIT_PACKAGE := "github.com/probe-lab/bitswap-sniffer"
REPO_SERVER := "019120760881.dkr.ecr.us-east-1.amazonaws.com"
COMMIT := `git rev-parse --short HEAD`
DATE := `date "+%Y-%m-%dT%H:%M:%SZ"`


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

test:
    go test ./bitswap/...

docker:
	docker build -t probe-lab/bitswap-sniffer:latest -t probe-lab/bitswap-sniffer-{{COMMIT}} -t {{REPO_SERVER}}/probelab:bitswap-sniffer-{{COMMIT}} .

docker-push:
	docker push {{REPO_SERVER}}/probelab:bitswap-sniffer-{{COMMIT}}
	docker rmi {{REPO_SERVER}}/probelab:bitswap-sniffer-{{COMMIT}}

clickhouse-up:
    docker compose up -d clickhouse

clikchouse-down:
    docker compose stop clickhouse

clikchouse-rm:
    docker compose rm clickhouse
