package bitswap

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type QueryCondition func() string

const (
	CidsTableName = "shared_cids"

	OriginBitswap = "bitswap"
	OriginDHT     = "dht"

	BitswapBlockType    = "block"
	BitswapWantType     = "want"
	BitswapHaveType     = "have"
	BitswapDontHaveType = "dont-have"

	DhtAddProviders = "add-provider-records"
	DhtGetProviders = "get-provider-records"
)

type SharedCid struct {
	// time of when the cid was seen
	Timestamp time.Time `ch:"timestamp" json:"timestamp"`
	// Sent or Recv
	Direction string `ch:"direction" json:"direction"`
	// string representation of the CID
	Cid string `ch:"cid" json:"cid"`
	// PeerID of the producer client
	Producer string `ch:"producer_id" json:"producer_id"`
	// PeerID of the remote node
	By string `ch:"peer_id" json:"peer_id"`
	// Want/Have/DontHave
	Type string `ch:"msg_type" json:"msg_type"`
	// Origin of the message bitswap/dht (simpler filter)
	Origin string `ch:"origin" json:"origin"`
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

func requestCids(ctx context.Context, db driver.Conn, conditions ...string) ([]SharedCid, error) {
	fullCondition := ""
	for i, condition := range conditions {
		if i == 0 {
			fullCondition = "WHERE " + condition
		} else {
			fullCondition = fullCondition + " AND " + condition
		}
	}

	query := fmt.Sprintf(`
		SELECT
			timestamp,
			direction,
			cid,
			producer_id,
			peer_id,
			msg_type,
			origin
		FROM %s
		%s
		ORDER BY timestamp, cid
		`,
		CidsTableName,
		fullCondition,
	)
	var resp []SharedCid
	err := db.Select(ctx, &resp, query)
	return resp, err
}

func RequestCids(ctx context.Context, db driver.Conn, conditions ...string) ([]SharedCid, error) {
	if err := ValidateSharedCidsTableSchema(ctx, db); err != nil {
		return []SharedCid{}, fmt.Errorf("DB schema mismatch: %v", err)
	}
	return requestCids(ctx, db, conditions...)
}

func WithinDates(startDate, endDate time.Time) string {
	return fmt.Sprintf(
		"timestamp BETWEEN '%s' AND '%s'",
		startDate.Format("2006-01-02 15:04:05"),
		endDate.Format("2006-01-02 15:04:05"),
	)
}

func WithMsgType(msgType string) string {
	return fmt.Sprintf("msg_type = '%s'", msgType)
}

func WithOrigin(origin string) string {
	return fmt.Sprintf("origin = '%s'", origin)
}

type columnInfo struct {
	Name string `ch:"name"`
	Type string `ch:"type"`
}

func ValidateSharedCidsTableSchema(ctx context.Context, db driver.Conn) error {
	const schemaQuery = `
		SELECT
			name, type
		FROM system.columns
		WHERE table = ? AND database = currentDatabase()
		ORDER BY position
	` // Adjust filtering if needed.

	var cols []columnInfo
	err := db.Select(ctx, &cols, schemaQuery, CidsTableName)
	if err != nil {
		return fmt.Errorf("schema fetch failed: %w", err)
	}

	// Define the expected schema: field name -> ClickHouse type
	expected := []columnInfo{
		{"timestamp", "DateTime"},
		{"direction", "LowCardinality(String)"},
		{"cid", "String"},
		{"producer_id", "String"},
		{"peer_id", "String"},
		{"msg_type", "LowCardinality(String)"},
		{"origin", "LowCardinality(String)"},
	}

	// Check column count
	if len(cols) != len(expected) {
		return fmt.Errorf("shared_cids schema column count mismatch: got %d want %d", len(cols), len(expected))
	}

	// Check each column’s name and type (normalize types if needed)
	for i, exp := range expected {
		got := cols[i]
		if got.Name != exp.Name || got.Type != exp.Type {
			return fmt.Errorf(
				"shared_cids column mismatch at position %d: got (%s, %s), want (%s, %s)",
				i, got.Name, got.Type, exp.Name, exp.Type,
			)
		}
	}
	return nil
}

func dropAllShardeCidTable(ctx context.Context, con driver.Conn) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE 1`, CidsTableName)
	return con.Exec(ctx, query)
}
