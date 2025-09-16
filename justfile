GOCC := "go"
BIN := "./cmd"
TARGET_PATH := "./build/bitswap-sniffer"


default:
    @just --list --justfile {{ justfile() }}

run:
    {{GOCC}} run {{BIN}}/root.go

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
