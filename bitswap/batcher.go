package bitswap

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

var DefaultBatchLimit int = 128

type cidBatcher struct {
	sync.RWMutex
	lastResetT time.Time
	sharedCids []SharedCid
	limit      int
}

func newCidBatcher(limit int) *cidBatcher {
	if limit <= 0 {
		logrus.Warnf("no limit was set, setting it to the default %d", limit)
	}
	return &cidBatcher{
		limit:      limit,
		sharedCids: make([]SharedCid, 0),
		lastResetT: time.Now(),
	}
}

func (b *cidBatcher) AddCid(c SharedCid) {
	b.Lock()
	defer b.Unlock()
	b.addCid(c)
}

func (b *cidBatcher) AddCids(cs []SharedCid) {
	b.Lock()
	defer b.Unlock()
	for _, c := range cs {
		b.addCid(c)
	}
}

func (b *cidBatcher) addCid(c SharedCid) {
	b.sharedCids = append(b.sharedCids, c)
}

func (b *cidBatcher) Len() int {
	b.RLock()
	defer b.RUnlock()
	return b.len()
}

func (b *cidBatcher) len() int {
	return len(b.sharedCids)
}

func (b *cidBatcher) IsFull() bool {
	b.RLock()
	defer b.RUnlock()
	return b.isFull()
}

func (b *cidBatcher) isFull() bool {
	return b.len() >= b.limit
}

func (b *cidBatcher) Reset() []SharedCid {
	b.Lock()
	defer b.Unlock()
	return b.reset()
}

func (b *cidBatcher) reset() []SharedCid {
	prevCids := b.sharedCids
	b.sharedCids = make([]SharedCid, 0)
	b.lastResetT = time.Now()
	return prevCids
}

func (b *cidBatcher) GetLastResetTime() time.Time {
	b.RLock()
	defer b.RUnlock()
	return b.lastResetT
}
