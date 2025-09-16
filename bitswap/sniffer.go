package bitswap

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ipfs/boxo/bitswap"
	"github.com/ipfs/boxo/bitswap/network"
	"github.com/ipfs/boxo/bitswap/network/bsnet"
	"github.com/ipfs/boxo/bitswap/network/httpnet"
	"github.com/ipfs/boxo/blockstore"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"

	rpqm "github.com/ipfs/boxo/routing/providerquerymanager"
	routingdisc "github.com/libp2p/go-libp2p/p2p/discovery/routing"

	"github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
)

type Sniffer struct {
	config *SnifferConfig
	log    *logrus.Logger

	// services
	bitswap   *bitswap.Bitswap
	dhtCli    *kaddht.IpfsDHT
	discovery *Discovery
}

func NewSniffer(ctx context.Context, dhtCli *kaddht.IpfsDHT, config *SnifferConfig) (*Sniffer, error) {
	log := config.Logger
	ds := dsync.MutexWrap(datastore.NewMapDatastore())
	bs := blockstore.NewBlockstore(ds)
	bs = blockstore.NewIdStore(bs)

	bitswapLibp2p := bsnet.NewFromIpfsHost(dhtCli.Host())
	bitswapHTTP := httpnet.New(
		dhtCli.Host(),
		httpnet.WithHTTPWorkers(1),
		httpnet.WithUserAgent("probelab-sniffer"),
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

	localTracer, err := NewTracer(log)
	if err != nil {
		return nil, err
	}

	bsServic := bitswap.New(
		ctx,
		bitswapNetworks,
		providerQueryMgr,
		bs,
		bitswap.WithTracer(localTracer),
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

	return &Sniffer{
		log:       log,
		config:    config,
		bitswap:   bsServic,
		dhtCli:    dhtCli,
		discovery: discv,
	}, nil
}

func (s *Sniffer) Serve(ctx context.Context) error {
	go func() {
		s.discovery.Serve(ctx)
	}()

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
	return nil
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
