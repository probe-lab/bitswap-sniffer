FROM golang:1.24 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-w -s" -o bitswap-sniffer ./cmd

FROM debian:bookworm-slim
WORKDIR /app
COPY --from=builder /app/bitswap-sniffer /app/bitswap-sniffer
ENTRYPOINT ["./bitswap-sniffer"]
CMD ["run --help"]
