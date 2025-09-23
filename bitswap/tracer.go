package bitswap

import (
	"time"

	bsmsg "github.com/ipfs/boxo/bitswap/message"
	"github.com/ipfs/boxo/bitswap/tracer"
	peer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

type CidStreamTracer struct {
	log      *logrus.Logger
	producer peer.ID
	cidC     chan []SharedCid
}

var _ tracer.Tracer = &CidStreamTracer{}

func NewStreamTracer(log *logrus.Logger, producerID peer.ID, cidC chan []SharedCid) (*CidStreamTracer, error) {
	return &CidStreamTracer{
		log:      log,
		producer: producerID,
		cidC:     cidC,
	}, nil
}

func (t *CidStreamTracer) MessageReceived(pid peer.ID, bmsg bsmsg.BitSwapMessage) {
	t.streamCid("received", pid, bmsg)
}

func (t *CidStreamTracer) MessageSent(pid peer.ID, bmsg bsmsg.BitSwapMessage) {
	t.streamCid("sent", pid, bmsg)
}

func (t *CidStreamTracer) streamCid(direction string, pid peer.ID, bmsg bsmsg.BitSwapMessage) {
	if bmsg.Empty() {
		return
	}
	timestamp := time.Now()

	sharedCids := make([]SharedCid, len(bmsg.Wantlist())+len(bmsg.Haves())+len(bmsg.DontHaves())+len(bmsg.Blocks()))
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
				Type:      "want",
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
				Type:      "have",
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
				Type:      "dont-have",
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
				Type:      "block",
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
	}).Debug("more cids tracked")

	t.cidC <- sharedCids
}
