package bitswap

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/metric"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"

	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	ma "github.com/multiformats/go-multiaddr"
)

type SnifferConfig struct {
	Libp2pHost        string
	Libp2pPort        int
	DialTimeout       time.Duration
	CacheSize         int
	LevelDB           string
	DiscoveryInterval time.Duration

	Logger    *logrus.Logger
	Telemetry metric.MeterProvider
}

func (c *SnifferConfig) Validate() error {
	// parameters
	ip := net.ParseIP(c.Libp2pHost)
	if ip == nil {
		return fmt.Errorf("invalid libp2p-host: %s", c.Libp2pHost)
	}
	if c.Libp2pPort <= 0 && c.Libp2pPort > (2^16) {
		return fmt.Errorf("invalid libp2p-port: %d", c.Libp2pPort)
	}
	if c.DialTimeout == time.Duration(0) {
		return fmt.Errorf("invlaid dial timeout: %s", c.DialTimeout)
	}
	if c.CacheSize <= 0 {
		return fmt.Errorf("invalid cache size: %d", c.CacheSize)
	}
	if c.DiscoveryInterval == time.Duration(0) {
		return fmt.Errorf("invalid discovery interval: %s", c.DiscoveryInterval)
	}
	if len(c.LevelDB) == 0 {
		return fmt.Errorf("invalid level-db path: %s", c.LevelDB)
	}

	// extra services related stuff
	if c.Logger == nil {
		return fmt.Errorf("no logger on sniffer config")
	}
	if c.Telemetry == nil {
		return fmt.Errorf("no metrics-service on sniffer config")
	}
	return nil
}

func (c *SnifferConfig) Libp2pOptions() ([]libp2p.Option, error) {
	// transport protocols
	mAddrs := make([]ma.Multiaddr, 0, 2)
	tcpAddr, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", c.Libp2pHost, c.Libp2pPort))
	if err != nil {
		c.Logger.Error(err)
		return nil, err
	}
	quicAddr, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/udp/%d/quic", c.Libp2pHost, c.Libp2pPort))
	if err != nil {
		c.Logger.Error(err)
		return nil, err
	}

	mAddrs = append(mAddrs, tcpAddr, quicAddr)
	cm, err := connmgr.NewConnManager(500, 2_000)
	if err != nil {
		c.Logger.Error(err)
		return nil, err
	}

	return []libp2p.Option{
		libp2p.WithDialTimeout(c.DialTimeout),
		libp2p.ListenAddrs(mAddrs...),
		libp2p.Security(noise.ID, noise.New),
		libp2p.UserAgent("probelab-sniffer"),
		libp2p.ResourceManager(&network.NullResourceManager{}),
		libp2p.ConnectionManager(cm),
		libp2p.DisableRelay(),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(quic.NewTransport),
		libp2p.Muxer(yamux.ID, yamux.DefaultTransport),
	}, nil
}

func (c *SnifferConfig) DHTClientOptions() ([]kaddht.Option, error) {
	return []kaddht.Option{
		kaddht.Mode(kaddht.ModeServer),
		kaddht.BootstrapPeers(kaddht.GetDefaultBootstrapPeerAddrInfos()...),
	}, nil
}

func (c *SnifferConfig) CreateDHTServer(ctx context.Context) (*kaddht.IpfsDHT, error) {
	// generate the libp2p host
	hostOptions, err := c.Libp2pOptions()
	if err != nil {
		return nil, err
	}
	h, err := libp2p.New(hostOptions...)
	if err != nil {
		return nil, err
	}

	// init the dht host
	// DHT routing
	dhtOptions, err := c.DHTClientOptions()
	if err != nil {
		return nil, err
	}
	dhtCli, err := kaddht.New(ctx, h, dhtOptions...)
	if err != nil {
		return nil, err
	}
	return dhtCli, nil
}
