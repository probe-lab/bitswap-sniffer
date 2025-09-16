package bitswap

import (
	bsmsg "github.com/ipfs/boxo/bitswap/message"
	"github.com/ipfs/boxo/bitswap/tracer"
	peer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

// Tracer provides methods to access all messages sent and received by Bitswap.
// This interface can be used to implement various statistics (this is original intent).
type LogTracer struct {
	log *logrus.Logger
}

var _ tracer.Tracer = &LogTracer{}

func NewTracer(log *logrus.Logger) (*LogTracer, error) {
	return &LogTracer{log: log}, nil
}

func (t *LogTracer) MessageReceived(pid peer.ID, bmsg bsmsg.BitSwapMessage) {
	wantList := bmsg.Wantlist()
	haveList := bmsg.Haves()
	dontHaveList := bmsg.DontHaves()

	t.log.WithFields(logrus.Fields{
		"peer-id":    pid.String(),
		"wants":      len(wantList),
		"haves":      len(haveList),
		"dont-haves": len(dontHaveList),
	}).Info("new received message")

	logBitswap(t.log, "new received message", pid, bmsg)
}

func (t *LogTracer) MessageSent(pid peer.ID, bmsg bsmsg.BitSwapMessage) {
	logBitswap(t.log, "sent message", pid, bmsg)
}

func logBitswap(logger *logrus.Logger, lmsg string, pid peer.ID, bmsg bsmsg.BitSwapMessage) {
	wantList := bmsg.Wantlist()
	haveList := bmsg.Haves()
	dontHaveList := bmsg.DontHaves()

	logger.WithFields(logrus.Fields{
		"peer-id":    pid.String(),
		"wants":      len(wantList),
		"haves":      len(haveList),
		"dont-haves": len(dontHaveList),
	}).Info("sent message")

	wantCids := make([]string, len(wantList))
	for i, wMsg := range wantList {
		wantCids[i] = wMsg.Cid.String()
	}
	logger.WithFields(logrus.Fields{
		"cids": wantCids,
	}).Info(" * want Cids")

	haveCids := make([]string, len(haveList))
	for i, hMsg := range haveCids {
		haveCids[i] = string(hMsg)
	}
	logger.WithFields(logrus.Fields{
		"cids": haveCids,
	}).Info(" * have Cids")

	dontHaveCids := make([]string, len(dontHaveList))
	for i, dhMsg := range dontHaveCids {
		dontHaveCids[i] = string(dhMsg)
	}
	logger.WithFields(logrus.Fields{
		"cids": dontHaveCids,
	}).Info(" * dont have Cids")
}
