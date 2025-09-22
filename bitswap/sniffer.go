package bitswap

import (
	"context"
	"crypto/rand"
	"fmt"
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
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	rpqm "github.com/ipfs/boxo/routing/providerquerymanager"
	routingdisc "github.com/libp2p/go-libp2p/p2p/discovery/routing"

	"github.com/ipfs/go-datastore"
	mh "github.com/multiformats/go-multihash"
)

type Sniffer struct {
	config *SnifferConfig
	log    *logrus.Logger

	// cid comsumer-related
	cidCache *lru.Cache[string, struct{}]
	cidC     chan []SharedCid
	db       *ClickhouseDB

	// services
	ds        *leveldb.Datastore
	bs        blockstore.Blockstore
	bitswap   *bitswap.Bitswap
	dhtCli    *kaddht.IpfsDHT
	discovery *Discovery

	// metrics
	cidCount          metric.Int64Counter
	uniqueCidCount    metric.Int64Counter
	bitswapStatsCount metric.Int64Counter
	bitswapPeerCount  metric.Int64Gauge
	diskUsageGauge    metric.Float64Gauge
}

func NewSniffer(ctx context.Context, config *SnifferConfig, dhtCli *kaddht.IpfsDHT, db *ClickhouseDB) (*Sniffer, error) {
	log := config.Logger
	cidC := make(chan []SharedCid)

	// create a leveldb-datastore
	ds, err := leveldb.NewDatastore(config.LevelDB, nil)
	if err != nil {
		return nil, fmt.Errorf("leveldb datastore: %w", err)
	}
	log.Infoln("Deleting old datastore...")
	if err := ds.Delete(ctx, datastore.NewKey("/")); err != nil {
		log.WithError(err).Warnln("Couldn't delete old datastore")
	}
	bs := blockstore.NewBlockstore(ds)
	bs = blockstore.NewIdStore(bs)

	// configure reqs for bitswap client
	bitswapLibp2p := bsnet.NewFromIpfsHost(dhtCli.Host())
	bitswapHTTP := httpnet.New(
		dhtCli.Host(),
		httpnet.WithHTTPWorkers(1),
		httpnet.WithUserAgent("probelab-sniffer"),
		httpnet.WithIdleConnTimeout(1*time.Hour),
		httpnet.WithMaxIdleConns(300),
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

	// custom tracer to stream all cids
	tracer, err := NewStreamTracer(log, dhtCli.Host().ID(), cidC)
	if err != nil {
		return nil, err
	}

	// create the bitswap server
	bsServic := bitswap.New(
		ctx,
		bitswapNetworks,
		providerQueryMgr,
		bs,
		bitswap.WithTracer(tracer),
		bitswap.WithServerEnabled(true),
	)

	discv, err := NewDiscovery(
		dhtCli,
		log,
		&DiscoveryConfig{
			Interval: 15 * time.Second,
		},
	)
	if err != nil {
		return nil, err
	}

	cidCache, err := lru.New[string, struct{}](config.CacheSize)
	if err != nil {
		return nil, err
	}
	return &Sniffer{
		log:       log,
		config:    config,
		cidCache:  cidCache,
		cidC:      cidC,
		ds:        ds,
		bs:        bs,
		bitswap:   bsServic,
		dhtCli:    dhtCli,
		db:        db,
		discovery: discv,
	}, nil
}

func (s *Sniffer) Serve(ctx context.Context) error {
	// ensure that we close everything before leaving
	defer func() {
		// close db connections
		s.db.Close()
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
			// list stats
			stats, err := s.bitswap.Stat()
			if err != nil {
				s.log.Warnf("unable to get bitswap server stats, %v", err)
				continue
			}
			s.log.WithFields(logrus.Fields{
				"want-list":    len(stats.Wantlist),
				"peers":        len(stats.Peers),
				"msg-received": stats.MessagesReceived,
				"msg-sent":     stats.DataSent,
			}).Info("bitswap stats...")

			s.bitswapPeerCount.Record(ctx, int64(len(stats.Peers)))
			s.bitswapStatsCount.Add(
				ctx,
				int64(stats.MessagesReceived),
				metric.WithAttributes(
					attribute.String("type", "sent"),
				),
			)
			s.bitswapStatsCount.Add(
				ctx,
				int64(stats.DataSent),
				metric.WithAttributes(
					attribute.String("type", "received"),
				),
			)
			s.bitswapStatsCount.Add(
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

	// debug bootnodes
	for _, bootnode := range succBootnodes {
		attrs := getLibp2pHostInfo(s.dhtCli.Host(), bootnode)
		s.log.WithFields(logrus.Fields{
			"peer_id":           bootnode.String(),
			"agent_version":     attrs["agent_version"],
			"protocols":         attrs["protocols"],
			"protocol_versions": attrs["protocol_versions"],
		}).Debug("bootnode info")
	}

	// init the db
	err = s.db.Init(ctx)
	if err != nil {
		return err
	}

	return s.initMetrics(ctx)
}

func (s *Sniffer) makeSnifferAppealing(ctx context.Context) error {
	// create and add as an IWANT a random Cid
	// this ensures that remote peers still try to fetch stuff from us

	content := make([]byte, 1_024)
	rand.Read(content)

	// configure the type of CID that we want
	pref := cid.Prefix{
		Version:  1,
		Codec:    cid.Raw,
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}

	// get the CID of the content we just generated
	randCid, err := pref.Sum(content)
	if err != nil {
		return err
	}
	s.log.WithField("cid", randCid.String()).Info("Bitswap appealer: pretending to fetch cid from bitswap...")

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(15 * time.Second):
				s.log.WithField("cid", randCid.String()).Info("Bitswap appealer: pretending to fetch cid from bitswap...")
				getBlockCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
				_, err := s.bitswap.GetBlock(getBlockCtx, randCid)
				if err != nil {
					s.log.Warnf("Bitswap appealer: Opps! (as expected) we couldn't find this random CID: %s - %v", randCid.String(), err)
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
						attribute.String("msg_type", sCid.Type),
					),
				)
				present := s.cidCache.Contains(sCid.Cid)
				if present {
					continue
				}
				s.cidCache.Add(sCid.Cid, struct{}{})
				cids = append(cids, sCid)
				s.uniqueCidCount.Add(ctx, int64(1))

			}
			if len(cids) > 0 {
				s.db.PersistCidBatch(ctx, cids)
			}

		case <-ctx.Done():
			return
		}
	}
}

func (s *Sniffer) bootstrapDHT(ctx context.Context, bootstrappers []peer.AddrInfo) ([]peer.ID, error) {
	var m sync.Mutex
	var succBootnodes []peer.ID

	// connect to the bootnodes
	var wg sync.WaitGroup

	for _, bnode := range bootstrappers {
		wg.Add(1)
		go func(bn peer.AddrInfo) {
			defer wg.Done()
			err := s.dhtCli.Host().Connect(ctx, bn)
			if err != nil {
				s.log.Warnf("unable to connect bootstrap node: %s - %s", bn.String(), err.Error())
			} else {
				m.Lock()
				succBootnodes = append(succBootnodes, bn.ID)
				m.Unlock()
				s.log.Debug("successful connection to bootstrap node:", bn.String())
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
		s.log.Warnf("unable to bootstrap the dht-node %s", err.Error())
	}
	if routingSize == 0 {
		s.log.Warn("no error, but empty routing table after bootstrapping")
	}
	s.log.WithFields(logrus.Fields{
		"successful-bootnodes": fmt.Sprintf("%d/%d", len(succBootnodes), len(bootstrappers)),
		"peers_in_routing":     routingSize,
	}).Info("dht cli bootstrapped")
	return succBootnodes, nil
}

func getLibp2pHostInfo(h host.Host, pID peer.ID) map[string]any {
	time.Sleep(30 * time.Millisecond)
	attrs := make(map[string]any)
	// read from the local peerstore
	// agent version
	var av any = "unknown"
	av, _ = h.Peerstore().Get(pID, "AgentVersion")
	attrs["agent_version"] = av

	// protocols
	prots, _ := h.Network().Peerstore().GetProtocols(pID)
	attrs["protocols"] = prots

	// protocol version
	var pv any = "unknown"
	pv, _ = h.Peerstore().Get(pID, "ProtocolVersion")
	attrs["protocol_version"] = pv

	return attrs
}

func (s *Sniffer) initMetrics(ctx context.Context) error {
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
	s.bitswapStatsCount, err = meter.Int64Counter("bitswap_stats", metric.WithDescription("Bitswap stats for sent and received msgs"))
	if err != nil {
		return fmt.Errorf("bitswap stats histogram: %w", err)
	}
	s.bitswapPeerCount, err = meter.Int64Gauge("bitswap_peer_count", metric.WithDescription("Number of peers connected at the Bitswap level"))
	if err != nil {
		return fmt.Errorf("bitswap peer count gauge: %w", err)
	s.diskUsageGauge, err = meter.Float64Gauge("datastore_disk_usage", metric.WithDescription("Disk usage of bitswap's datastore"))
	if err != nil {
		return fmt.Errorf("datastore disk usage gauge %w", err)
	}
	return nil
}
