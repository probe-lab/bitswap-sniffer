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
	Interval  time.Duration
	Telemetry metric.MeterProvider
}

type Discovery struct {
	cfg       *DiscoveryConfig
	log       *logrus.Logger
	dhtCli    *kaddht.IpfsDHT
	bsNetwork network.BitSwapNetwork

	randCid cid.Cid

	// Metrics
	MeterLookups metric.Int64Counter
}

func NewDiscovery(dhtCli *kaddht.IpfsDHT, bsNet network.BitSwapNetwork, log *logrus.Logger, cfg *DiscoveryConfig) (*Discovery, error) {
	log.Info("Initialize Discovery service")

	d := &Discovery{
		cfg:       cfg,
		log:       log,
		dhtCli:    dhtCli,
		randCid:   randCid,
		bsNetwork: bsNet,
	}

	err = d.initMetrics()
	if err != nil {
		return nil, err
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
	d.log.WithField("interval", d.cfg.Interval).Info("Starting DHT Discovery Service")
	defer d.log.Info("Stopped DHT Discovery Service")

	for {

		k, err := d.dhtCli.RoutingTable().GenRandomKey(0)
		if err != nil {
			return fmt.Errorf("failed to generate random key: %w", err)
		}

		start := time.Now()
		timeoutCtx, timeoutCancel := context.WithTimeout(ctx, time.Minute)
		d.log.WithField("key", hex.EncodeToString(k)).Info("DHT discovery: looking up random key")
		peers, err := d.dhtCli.GetClosestPeers(timeoutCtx, string(k))
		d.log.WithFields(logrus.Fields{
			"count": len(peers),
			"err":   err,
			"took":  time.Since(start).String(),
		}).Info("DHT discovery: finished lookup")
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
