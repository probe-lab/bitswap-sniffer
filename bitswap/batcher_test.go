package bitswap

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Helper to create a SharedCid
func makeSharedCid(n int) SharedCid {
	return SharedCid{
		Direction: "Sent",
		Cid:       fmt.Sprintf("CID-%d", n),
		By:        fmt.Sprintf("Peer-%d", n),
		Type:      "Want",
	}
}

func TestAddCidAndLength(t *testing.T) {
	batcher := newCidBatcher(3)
	require.Equal(t, 0, batcher.Len())

	batcher.AddCid(makeSharedCid(1))
	require.Equal(t, 1, batcher.Len())
}

func TestBatcherIsFull(t *testing.T) {
	batcher := newCidBatcher(2)
	require.False(t, batcher.IsFull())

	batcher.AddCids([]SharedCid{makeSharedCid(1), makeSharedCid(2)})
	require.True(t, batcher.IsFull())
}

func TestBatcherReset(t *testing.T) {
	batcher := newCidBatcher(2)
	batcher.AddCid(makeSharedCid(1))
	oldTime := batcher.GetLastResetTime()

	time.Sleep(time.Millisecond)
	prev := batcher.Reset()
	require.Equal(t, 1, len(prev))
	require.Equal(t, 0, batcher.Len())
	require.True(t, batcher.GetLastResetTime().After(oldTime))
}

func TestAddCidsTable(t *testing.T) {
	tests := []struct {
		name   string
		cids   []SharedCid
		wanted int
	}{
		{"empty", nil, 0},
		{"one", []SharedCid{makeSharedCid(1)}, 1},
		{"many", []SharedCid{makeSharedCid(1), makeSharedCid(2)}, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			batcher := newCidBatcher(10)
			batcher.AddCids(tc.cids)
			require.Equal(t, tc.wanted, batcher.Len())
		})
	}
}
