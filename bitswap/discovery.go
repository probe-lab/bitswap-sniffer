package bitswap

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ipfs/boxo/bitswap/network"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type DiscoveryConfig struct {
	Interval  time.Duration
	Telemetry metric.MeterProvider
}

type Discovery struct {
	cfg       *DiscoveryConfig
	log       *slog.Logger
	dhtCli    *kaddht.IpfsDHT
	bsNetwork network.BitSwapNetwork

	// Metrics
	MeterLookups metric.Int64Counter
}

func NewDiscovery(dhtCli *kaddht.IpfsDHT, bsNet network.BitSwapNetwork, log *slog.Logger, cfg *DiscoveryConfig) (*Discovery, error) {
	log.Info("Initialize Discovery service")

	d := &Discovery{
		cfg:       cfg,
		log:       log,
		dhtCli:    dhtCli,
		bsNetwork: bsNet,
	}

	err := d.initMetrics()
	if err != nil {
		return nil, err
	}

	return d, nil
}

func (d *Discovery) Serve(ctx context.Context) (err error) {
	d.log.Info("Starting DHT Discovery Service", "interval", d.cfg.Interval)
	defer d.log.Info("Stopped DHT Discovery Service")

	for {

		k, err := d.dhtCli.RoutingTable().GenRandomKey(0)
		if err != nil {
			return fmt.Errorf("failed to generate random key: %w", err)
		}

		start := time.Now()
		timeoutCtx, timeoutCancel := context.WithTimeout(ctx, time.Minute)
		d.log.Info("DHT discovery: looking up random key", "key", hex.EncodeToString(k))
		peers, err := d.dhtCli.GetClosestPeers(timeoutCtx, string(k))
		d.log.Info("DHT discovery: finished lookup",
			"count", len(peers),
			"err", err,
			"took", time.Since(start).String(),
		)
		timeoutCancel()

		d.MeterLookups.Add(ctx, 1, metric.WithAttributes(attribute.Bool("success", err == nil)))
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil
		} else if err != nil || len(peers) == 0 {
			// could be that we don't have any DHT peers in our peer store
			// -> bootstrap again
			for _, addrInfo := range kaddht.GetDefaultBootstrapPeerAddrInfos() {
				timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 5*time.Second)
				_ = d.dhtCli.Host().Connect(timeoutCtx, addrInfo)
				timeoutCancel()
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(d.cfg.Interval - time.Since(start)):
			continue
		}
	}
}
func (d *Discovery) initMetrics() error {
	var err error
	meter := d.cfg.Telemetry.Meter("discovery")
	d.MeterLookups, err = meter.Int64Counter("lookups", metric.WithDescription("Total number of performed lookups"))
	if err != nil {
		return fmt.Errorf("lookups counter: %w", err)
	}
	return nil
}
