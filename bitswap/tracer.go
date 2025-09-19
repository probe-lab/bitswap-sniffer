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
	timestamp := time.Now()
	sharedCids := make([]SharedCid, 0)

	wantList := bmsg.Wantlist()
	haveList := bmsg.Haves()
	dontHaveList := bmsg.DontHaves()
	blockList := bmsg.Blocks()

	wantCids := make([]string, len(wantList))
	for i, wMsg := range wantList {
		wantCids[i] = wMsg.Cid.String()
		sharedCids = append(sharedCids, SharedCid{timestamp, direction, wMsg.Cid.String(), t.producer.String(), pid.String(), "want"})
	}

	haveCids := make([]string, len(haveList))
	for i, hMsg := range haveCids {
		haveCids[i] = string(hMsg)
		sharedCids = append(sharedCids, SharedCid{timestamp, direction, string(hMsg), t.producer.String(), pid.String(), "have"})
	}

	dontHaveCids := make([]string, len(dontHaveList))
	for i, dhMsg := range dontHaveCids {
		dontHaveCids[i] = string(dhMsg)
		sharedCids = append(sharedCids, SharedCid{timestamp, direction, string(dhMsg), t.producer.String(), pid.String(), "dont-have"})
	}

	blockCids := make([]string, len(blockList))
	for i, bMsg := range blockList {
		blockCids[i] = bMsg.Cid().String()
		sharedCids = append(sharedCids, SharedCid{timestamp, direction, bMsg.Cid().String(), t.producer.String(), pid.String(), "block"})
	}

	t.log.WithFields(logrus.Fields{
		"peer":      pid.String(),
		"direction": direction,
		"want":      len(wantList),
		"have":      len(haveList),
		"dont-have": len(dontHaveList),
		"blocks":    len(blockList),
	}).Debug("more cids tracked")

	t.cidC <- sharedCids
}
