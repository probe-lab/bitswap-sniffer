package bitswap

import (
	"context"
	"fmt"
	"time"

	bsmsg "github.com/ipfs/boxo/bitswap/message"
	"github.com/ipfs/boxo/bitswap/tracer"
	"github.com/ipfs/go-cid"
	dht_pb "github.com/libp2p/go-libp2p-kad-dht/pb"
	"github.com/libp2p/go-libp2p/core/network"
	peer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/sirupsen/logrus"
)

type CidTracer struct {
	log      *logrus.Logger
	producer peer.ID
	cidC     chan []SharedCid
}

var _ tracer.Tracer = &CidTracer{}

func NewCidTracer(log *logrus.Logger, producerID peer.ID, cidC chan []SharedCid) (*CidTracer, error) {
	return &CidTracer{
		log:      log,
		producer: producerID,
		cidC:     cidC,
	}, nil
}

func (t *CidTracer) MessageReceived(pid peer.ID, bmsg bsmsg.BitSwapMessage) {
	t.streamCid("received", pid, bmsg)
}

func (t *CidTracer) MessageSent(pid peer.ID, bmsg bsmsg.BitSwapMessage) {
	t.streamCid("sent", pid, bmsg)
}

func (t *CidTracer) streamCid(direction string, pid peer.ID, bmsg bsmsg.BitSwapMessage) {
	if bmsg.Empty() {
		return
	}
	timestamp := time.Now()

	sharedCids := make([]SharedCid, 0, len(bmsg.Wantlist())+len(bmsg.Haves())+len(bmsg.DontHaves())+len(bmsg.Blocks()))
	producerStr := t.producer.String()

	wantCids := make([]string, len(bmsg.Wantlist()))
	for i, wMsg := range bmsg.Wantlist() {
		wantCids[i] = wMsg.Cid.String()
		sharedCids = append(
			sharedCids,
			SharedCid{
				Timestamp: timestamp,
				Direction: direction,
				Cid:       wMsg.Cid.String(),
				Producer:  producerStr,
				By:        pid.String(),
				Type:      BitswapWantType,
				Origin:    OriginBitswap,
			},
		)
	}

	haveCids := make([]string, len(bmsg.Haves()))
	for i, hMsg := range bmsg.Haves() {
		haveCids[i] = hMsg.String()
		sharedCids = append(
			sharedCids,
			SharedCid{
				Timestamp: timestamp,
				Direction: direction,
				Cid:       hMsg.String(),
				Producer:  producerStr,
				By:        pid.String(),
				Type:      BitswapHaveType,
				Origin:    OriginBitswap,
			},
		)
	}

	dontHaveCids := make([]string, len(bmsg.DontHaves()))
	for i, dhMsg := range bmsg.DontHaves() {
		dontHaveCids[i] = dhMsg.String()
		sharedCids = append(
			sharedCids,
			SharedCid{
				Timestamp: timestamp,
				Direction: direction,
				Cid:       dhMsg.String(),
				Producer:  producerStr,
				By:        pid.String(),
				Type:      BitswapDontHaveType,
				Origin:    OriginBitswap,
			},
		)
	}

	blockCids := make([]string, len(bmsg.Blocks()))
	for i, bMsg := range bmsg.Blocks() {
		blockCids[i] = bMsg.Cid().String()
		sharedCids = append(
			sharedCids,
			SharedCid{
				Timestamp: timestamp,
				Direction: direction,
				Cid:       bMsg.String(),
				Producer:  producerStr,
				By:        pid.String(),
				Type:      BitswapBlockType,
				Origin:    OriginBitswap,
			},
		)
	}

	t.log.WithFields(logrus.Fields{
		"peer":      pid.String(),
		"direction": direction,
		"want":      len(bmsg.Wantlist()),
		"have":      len(bmsg.Haves()),
		"dont-have": len(bmsg.DontHaves()),
		"blocks":    len(bmsg.Blocks()),
		"origin":    OriginBitswap,
	}).Debug("more cids tracked from bitswap")

	t.cidC <- sharedCids
}

func (t *CidTracer) dhtRequestTracer(ctx context.Context, s network.Stream, req *dht_pb.Message) {
	// unwrap the record and get the cid
	providerId := s.Conn().RemotePeer()

	var sharedCid SharedCid
	var cid cid.Cid
	var err error

	// we are only interested into the AddProviders or GetProviders messages
	switch req.Type {
	case dht_pb.Message_ADD_PROVIDER:
		cid, err = handleAddProvider(providerId, req)
		if err != nil {
			t.log.WithFields(logrus.Fields{
				"key":         string(req.Key),
				"remote-peer": providerId.String(),
			}).Errorf("dht: unable to extract the cid from given key - %s", err.Error())
			return
		}
		sharedCid = SharedCid{
			Timestamp: time.Now(),
			Direction: "received",
			Cid:       cid.String(),
			Producer:  t.producer.String(),
			By:        providerId.String(),
			Type:      DhtAddProviders,
			Origin:    OriginDHT,
		}

	case dht_pb.Message_GET_PROVIDERS:
		cid, err = handleGetProvider(req)
		if err != nil {
			t.log.WithFields(logrus.Fields{
				"key":         string(req.Key),
				"remote-peer": providerId.String(),
			}).Errorf("dht: unable to extract the cid from given key - %s", err.Error())
			return
		}
		sharedCid = SharedCid{
			Timestamp: time.Now(),
			Direction: "received",
			Cid:       cid.String(),
			Producer:  t.producer.String(),
			By:        providerId.String(),
			Type:      DhtGetProviders,
			Origin:    OriginDHT,
		}

	default:
		t.log.WithField("type", dht_pb.Message_MessageType_name[int32(req.Type)]).Trace("dropping not relevant dht message...")
		return
	}

	t.log.WithFields(logrus.Fields{
		"peer":      providerId.String(),
		"key":       sharedCid.Cid,
		"op":        sharedCid.Type,
		"direction": "received",
		"providers": len(req.ProviderPeers),
		"origin":    OriginDHT,
	}).Debug("more cids tracked from the DHT server")

	sharedCids := make([]SharedCid, 1)
	sharedCids[0] = sharedCid
	t.cidC <- sharedCids
}

func handleAddProvider(p peer.ID, pmes *dht_pb.Message) (cid.Cid, error) {
	key := pmes.GetKey()
	if len(key) > 80 {
		return cid.Cid{}, fmt.Errorf("provider_records: key size too large")
	} else if len(key) == 0 {
		return cid.Cid{}, fmt.Errorf("provider_records: key is empty")
	}

	pinfos := dht_pb.PBPeersToPeerInfos(pmes.GetProviderPeers())
	success := false
	for _, pi := range pinfos {
		if pi.ID != p {
			continue
		}
		if len(pi.Addrs) < 1 {
			continue
		}
		success = true
	}
	if !success {
		return cid.Cid{}, fmt.Errorf("provider_record: no valid provider")
	}

	// conver key to multihash
	keyMH, err := multihash.Cast(key)
	if err != nil {
		return cid.Cid{}, fmt.Errorf("provider_record: no valid multihash for key - %s", err.Error())
	}

	// conver the key into a CID
	return cid.NewCidV1(uint64(multicodec.Raw), keyMH), nil
}

func handleGetProvider(pmes *dht_pb.Message) (cid.Cid, error) {
	key := pmes.GetKey()
	if len(key) > 80 {
		return cid.Cid{}, fmt.Errorf("provider_records: key size too large")
	} else if len(key) == 0 {
		return cid.Cid{}, fmt.Errorf("provider_records: key is empty")
	}

	// conver key to multihash
	keyMH, err := multihash.Cast(key)
	if err != nil {
		return cid.Cid{}, fmt.Errorf("provider_record: no valid multihash for key - %s", err.Error())
	}
	decodedKeyMH, err := multihash.Decode(key)
	if err != nil {
		return cid.Cid{}, fmt.Errorf("provider_record: no valid decoded multihash for key - %s", err.Error())
	}

	// conver the key into a CID
	return cid.NewCidV1(decodedKeyMH.Code, keyMH), nil
}
