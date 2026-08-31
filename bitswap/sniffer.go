package bitswap

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/ipfs/boxo/bitswap"
	"github.com/ipfs/boxo/bitswap/network"
	"github.com/ipfs/boxo/bitswap/network/bsnet"
	"github.com/ipfs/boxo/bitswap/network/httpnet"
	"github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/go-cid"
	leveldb "github.com/ipfs/go-ds-leveldb"
	libp2p "github.com/libp2p/go-libp2p"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	rpqm "github.com/ipfs/boxo/routing/providerquerymanager"
	routingdisc "github.com/libp2p/go-libp2p/p2p/discovery/routing"

	mh "github.com/multiformats/go-multihash"
)

type Sniffer struct {
	config *SnifferConfig

	// cid comsumer-related
	cidCache            *lru.Cache[string, struct{}]
	cidC                chan []SharedCid
	cidConsumerDone     chan struct{}
	bitswapAppealerDone chan struct{}
	db                  *ClickhouseDB

	// services
	ds        *leveldb.Datastore
	bs        blockstore.Blockstore
	bitswap   *bitswap.Bitswap
	dhtCli    *kaddht.IpfsDHT
	discovery *Discovery

	// metrics
	cidCount             metric.Int64Counter
	uniqueCidCount       metric.Int64Counter
	bitswapStatsGauge    metric.Int64Gauge
	bitswapWantListGauge metric.Int64Gauge
	bitswapPeerGauge     metric.Int64Gauge
	diskUsageGauge       metric.Float64Gauge
}

func NewSniffer(
	ctx context.Context,
	config *SnifferConfig,
	ds *leveldb.Datastore,
	db *ClickhouseDB) (*Sniffer, error) {

	cidC := make(chan []SharedCid)

	bs := blockstore.NewBlockstore(ds)
	bs = blockstore.NewIdStore(bs)

	hostOptions, err := config.Libp2pOptions()
	if err != nil {
		return nil, err
	}
	h, err := libp2p.New(hostOptions...)
	if err != nil {
		return nil, err
	}

	cidTracer, err := NewCidTracer(h.ID(), cidC)
	if err != nil {
		return nil, err
	}

	// DHT routing
	dhtOptions, err := config.DHTClientOptions()
	if err != nil {
		return nil, err
	}
	dhtOptions = append(dhtOptions, kaddht.OnRequestHook(cidTracer.dhtRequestTracer))
	if ds != nil {
		dhtOptions = append(dhtOptions, kaddht.Datastore(ds))
	}
	dhtCli, err := kaddht.New(h, dhtOptions...)
	if err != nil {
		return nil, err
	}

	// configure reqs for bitswap client
	bitswapLibp2p := bsnet.NewFromIpfsHost(dhtCli.Host())
	bitswapHTTP := httpnet.New(
		dhtCli.Host(),
		httpnet.WithHTTPWorkers(5),
		httpnet.WithUserAgent("probelab-sniffer"),
		httpnet.WithIdleConnTimeout(1*time.Hour),
		httpnet.WithMaxIdleConns(500),
	)
	bitswapNetworks := network.New(dhtCli.Host().Peerstore(), bitswapLibp2p, bitswapHTTP)

	disc := routingdisc.NewRoutingDiscovery(dhtCli)

	providerQueryMgr, err := rpqm.New(
		bitswapNetworks,
		disc,
		rpqm.WithMaxProviders(20),
	)
	if err != nil {
		return nil, err
	}

	bsServic := bitswap.New(
		ctx,
		bitswapNetworks,
		providerQueryMgr,
		bs,
		bitswap.WithTracer(cidTracer),
		bitswap.WithServerEnabled(true),
		bitswap.SetSendDontHaves(true),
	)

	discv, err := NewDiscovery(
		dhtCli,
		bitswapNetworks,
		&DiscoveryConfig{
			Interval:  config.DiscoveryInterval,
			Telemetry: config.Telemetry,
		},
	)
	if err != nil {
		return nil, err
	}

	var cidCache *lru.Cache[string, struct{}]
	if config.CacheSize > 0 {
		cidCache, err = lru.New[string, struct{}](config.CacheSize)
		if err != nil {
			return nil, err
		}
	}
	return &Sniffer{
		config:              config,
		cidCache:            cidCache,
		cidC:                cidC,
		cidConsumerDone:     make(chan struct{}),
		bitswapAppealerDone: make(chan struct{}),
		ds:                  ds,
		bs:                  bs,
		bitswap:             bsServic,
		dhtCli:              dhtCli,
		db:                  db,
		discovery:           discv,
	}, nil
}

func (s *Sniffer) Serve(ctx context.Context) error {
	// ensure that we close everything before leaving
	defer func() {
		var err error
		err = s.bitswap.Close()
		if err != nil {
			slog.Error("closing bitswap", "err", err)
		}

		err = s.dhtCli.Close()
		if err != nil {
			slog.Error("closing dht client", "err", err)
		}

		err = s.dhtCli.Host().Close()
		if err != nil {
			slog.Error("closing libp2p host", "err", err)
		}

		err = s.ds.Close()
		if err != nil {
			slog.Error("closing datastore", "err", err)
		}

		<-s.cidConsumerDone
		<-s.bitswapAppealerDone

		err = s.db.Close()
		if err != nil {
			slog.Error("closing db", "err", err)
		}

	}()

	go s.discovery.Serve(ctx)
	go s.cidConsumer(ctx)
	go s.measureDiskUsage(ctx)

	err := s.makeSnifferAppealing(ctx)
	if err != nil {
		return err
	}

	for {
		select {
		case <-time.After(10 * time.Second):
			stats, err := s.bitswap.Stat()
			if err != nil {
				slog.Warn("unable to get bitswap server stats", "err", err)
				continue
			}
			slog.Info("bitswap stats...",
				"want-list", len(stats.Wantlist),
				"peers", len(stats.Peers),
				"msg-received", stats.MessagesReceived,
				"msg-sent", stats.DataSent,
			)

			s.bitswapPeerGauge.Record(ctx, int64(len(stats.Peers)))
			s.bitswapStatsGauge.Record(
				ctx,
				int64(stats.MessagesReceived),
				metric.WithAttributes(
					attribute.String("type", "sent"),
				),
			)
			s.bitswapStatsGauge.Record(
				ctx,
				int64(stats.DataSent),
				metric.WithAttributes(
					attribute.String("type", "received"),
				),
			)
			s.bitswapWantListGauge.Record(
				ctx,
				int64(len(stats.Wantlist)),
				metric.WithAttributes(
					attribute.String("type", "cid-want-list"),
				),
			)

		case <-ctx.Done():
			return nil
		}
	}
}

func (s *Sniffer) Init(ctx context.Context) error {
	// prevent dial backoffs
	succBootnodes, err := s.bootstrapDHT(ctx, kaddht.GetDefaultBootstrapPeerAddrInfos())
	if err != nil {
		return err
	}

	for _, bootnode := range succBootnodes {
		attrs := getLibp2pHostInfo(s.dhtCli.Host(), bootnode)
		slog.Debug("bootnode info",
			"peer_id", bootnode.String(),
			"agent_version", attrs["agent_version"],
			"protocols", attrs["protocols"],
			"protocol_versions", attrs["protocol_versions"],
		)
	}

	err = s.db.Init(ctx)
	if err != nil {
		return err
	}

	return s.initMetrics()
}

func (s *Sniffer) makeSnifferAppealing(ctx context.Context) error {
	// create and add as an IWANT a random Cid
	// this ensures that remote peers still try to fetch stuff from us

	content := make([]byte, 1_024)
	rand.Read(content)

	pref := cid.Prefix{
		Version:  1,
		Codec:    cid.Raw,
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}

	randCid, err := pref.Sum(content)
	if err != nil {
		return err
	}
	slog.Info("Bitswap appealer: pretending to fetch cid from bitswap...", "cid", randCid.String())

	go func() {
		for {
			select {
			case <-ctx.Done():
				close(s.bitswapAppealerDone)
				return
			case <-time.After(15 * time.Second):
				slog.Info("Bitswap appealer: pretending to fetch cid from bitswap...", "cid", randCid.String())
				getBlockCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
				_, err := s.bitswap.GetBlock(getBlockCtx, randCid)
				if err != nil {
					slog.Warn("Bitswap appealer: Opps! (as expected) we couldn't find this random CID", "cid", randCid.String(), "err", err)
				}
				cancel()
			}
		}
	}()
	return nil
}

func (s *Sniffer) cidConsumer(ctx context.Context) {
	for {
		select {
		case cidList := <-s.cidC:
			cids := make([]SharedCid, 0)
			for _, sCid := range cidList {
				s.cidCount.Add(
					ctx,
					1,
					metric.WithAttributes(
						attribute.String("direction", sCid.Direction),
						attribute.String("origin", sCid.Origin),
						attribute.String("msg_type", sCid.Type),
					),
				)
				if s.cidCache != nil {
					present := s.cidCache.Contains(sCid.Cid)
					if present {
						continue
					}
					s.cidCache.Add(sCid.Cid, struct{}{})
				}
				cids = append(cids, sCid)
				s.uniqueCidCount.Add(ctx, int64(1))
			}
			if len(cids) > 0 {
				s.db.PersistCidBatch(ctx, cids)
			}

		case <-ctx.Done():
			close(s.cidConsumerDone)
			return
		}
	}
}

func (s *Sniffer) bootstrapDHT(ctx context.Context, bootstrappers []peer.AddrInfo) ([]peer.ID, error) {
	var m sync.Mutex
	var succBootnodes []peer.ID

	var wg sync.WaitGroup

	for _, bnode := range bootstrappers {
		wg.Add(1)
		go func(bn peer.AddrInfo) {
			defer wg.Done()
			err := s.dhtCli.Host().Connect(ctx, bn)
			if err != nil {
				slog.Warn("unable to connect bootstrap node", "bootnode", bn.String(), "err", err)
			} else {
				m.Lock()
				succBootnodes = append(succBootnodes, bn.ID)
				m.Unlock()
				slog.Debug("successful connection to bootstrap node", "bootnode", bn.String())
			}
		}(bnode)
	}

	// bootstrap from existing connections
	wg.Wait()
	err := s.dhtCli.Bootstrap(ctx)

	// force waiting a little bit to let the bootstrap work
	bootstrapTicker := time.NewTicker(5 * time.Second)
	select {
	case <-bootstrapTicker.C:
	case <-ctx.Done():
	}

	routingSize := s.dhtCli.RoutingTable().Size()
	if err != nil {
		slog.Warn("unable to bootstrap the dht-node", "err", err)
	}
	if routingSize == 0 {
		slog.Warn("no error, but empty routing table after bootstrapping")
	}
	slog.Info("dht cli bootstrapped",
		"successful-bootnodes", fmt.Sprintf("%d/%d", len(succBootnodes), len(bootstrappers)),
		"peers_in_routing", routingSize,
	)
	return succBootnodes, nil
}

func getLibp2pHostInfo(h host.Host, pID peer.ID) map[string]any {
	time.Sleep(30 * time.Millisecond)
	attrs := make(map[string]any)
	// read from the local peerstore
	var av any = "unknown"
	av, _ = h.Peerstore().Get(pID, "AgentVersion")
	attrs["agent_version"] = av

	prots, _ := h.Network().Peerstore().GetProtocols(pID)
	attrs["protocols"] = prots

	var pv any = "unknown"
	pv, _ = h.Peerstore().Get(pID, "ProtocolVersion")
	attrs["protocol_version"] = pv

	return attrs
}

func (s *Sniffer) initMetrics() error {
	var err error
	meter := s.config.Telemetry.Meter("sniffer")

	s.cidCount, err = meter.Int64Counter("total_seen_cids", metric.WithDescription("Number of cids seen during runtime"))
	if err != nil {
		return fmt.Errorf("total seen cids histogram: %w", err)
	}
	s.uniqueCidCount, err = meter.Int64Counter("unique_cids_by_msg_type", metric.WithDescription("Total number of unique CIDs"))
	if err != nil {
		return fmt.Errorf("unique seen cids counter: %w", err)
	}
	s.bitswapStatsGauge, err = meter.Int64Gauge("bitswap_stats", metric.WithDescription("Bitswap stats for sent and received msgs"))
	if err != nil {
		return fmt.Errorf("bitswap stats histogram: %w", err)
	}
	s.bitswapPeerGauge, err = meter.Int64Gauge("bitswap_peer_count", metric.WithDescription("Number of peers connected at the Bitswap level"))
	if err != nil {
		return fmt.Errorf("bitswap peer gauge: %w", err)
	}
	s.bitswapWantListGauge, err = meter.Int64Gauge("bitswap_want_list", metric.WithDescription("Number of CIDs in our want-list"))
	if err != nil {
		return fmt.Errorf("bitswap want-list gauge %w", err)
	}
	s.diskUsageGauge, err = meter.Float64Gauge("datastore_disk_usage", metric.WithDescription("Disk usage of bitswap's datastore"))
	if err != nil {
		return fmt.Errorf("datastore disk usage gauge %w", err)
	}
	return nil
}
