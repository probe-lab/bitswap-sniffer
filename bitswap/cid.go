package bitswap

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

const (
	CidsTableName = "shared_cids"
)

type SharedCid struct {
	// time of when the cid was seen
	Timestamp time.Time `ch:"timestamp" json:"timestamp"`
	// Sent or Recv
	Direction string `ch:"direction" json:"direction"`
	// string representation of the CID
	Cid string `ch:"cid" json:"cid"`
	// PeerID of the remote node
	By string `ch:"peer_id" json:"peer_id"`
	// Want/Have/DontHave
	Type string `ch:"type" json:"type"`
}

func PrepareSharedCidsBatch(ctx context.Context, db driver.Conn, cids []SharedCid) (driver.Batch, error) {
	query := fmt.Sprintf("INSERT INTO %s", CidsTableName)
	b, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return nil, err
	}
	for _, cid := range cids {
		err = b.AppendStruct(&cid)
		if err != nil {
			return nil, err
		}
	}
	return b, nil
}
