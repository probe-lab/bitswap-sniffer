package bitswap

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/ipfs/go-datastore"
	leveldb "github.com/ipfs/go-ds-leveldb"
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
	ConnectionsLow    int
	ConnectionsHigh   int
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
	cm, err := connmgr.NewConnManager(c.ConnectionsLow, c.ConnectionsHigh)
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

func (c *SnifferConfig) CreateDatastore(ctx context.Context) (*leveldb.Datastore, error) {
	// create a leveldb-datastore
	_ = os.Mkdir(c.LevelDB, 0777) // create the folder and ignore if there was an issue
	ds, err := leveldb.NewDatastore(c.LevelDB, nil)
	if err != nil {
		return nil, err
	}
	// We don't store the priv key of the host, thus, delete any existing files
	c.Logger.Info("Deleting old datastore...")
	if err := ds.Delete(ctx, datastore.NewKey("/")); err != nil {
		c.Logger.WithError(err).Warnln("Couldn't delete old datastore")
	}
	return ds, nil
}
