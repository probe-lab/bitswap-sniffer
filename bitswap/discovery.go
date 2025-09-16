package bitswap

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	kaddht "github.com/libp2p/go-libp2p-kad-dht"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/metric"
)

type DiscoveryConfig struct {
	Interval time.Duration
	Meter    metric.Meter
}

type Discovery struct {
	cfg    *DiscoveryConfig
	log    *logrus.Logger
	dhtCli *kaddht.IpfsDHT

	// Metrics
	MeterLookups metric.Int64Counter
}

func NewDiscovery(dhtCli *kaddht.IpfsDHT, log *logrus.Logger, cfg *DiscoveryConfig) (*Discovery, error) {
	log.Info("Initialize Discovery service")

	d := &Discovery{
		cfg:    cfg,
		log:    log,
		dhtCli: dhtCli,
	}

	/*
		var err error
		d.MeterLookups, err = cfg.Meter.Int64Counter("lookups", metric.WithDescription("Total number of performed lookups"))
		if err != nil {
			return nil, fmt.Errorf("lookups counter: %w", err)
		}
	*/
	return d, nil
}

func (d *Discovery) Serve(ctx context.Context) (err error) {
	d.log.Info("Starting discv5 Discovery Service", "interval", d.cfg.Interval)
	defer d.log.Info("Stopped discv5 Discovery Service")

	for {

		k, err := d.dhtCli.RoutingTable().GenRandomKey(0)
		if err != nil {
			return fmt.Errorf("failed to generate random key: %w", err)
		}

		start := time.Now()
		timeoutCtx, timeoutCancel := context.WithTimeout(ctx, time.Minute)
		d.log.Info("Looking up random key", "key", hex.EncodeToString(k))
		peers, err := d.dhtCli.GetClosestPeers(timeoutCtx, string(k))
		d.log.Info("Finished lookup", "count", len(peers), "err", err, "took", time.Since(start).String())
		timeoutCancel()

		// d.MeterLookups.Add(ctx, 1, metric.WithAttributes(attribute.Bool("success", err == nil)))
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
