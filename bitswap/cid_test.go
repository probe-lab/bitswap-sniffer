package bitswap

import (
	"context"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/probe-lab/go-commons/db"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	sdkmetrics "go.opentelemetry.io/otel/sdk/metric"
)

// assumes that the db is freshly started from scratch
func TestCidQueries(t *testing.T) {
	chCli := createTestDB(t)

	err := chCli.Init(context.Background())
	require.NoError(t, err)

	err = dropAllShardeCidTable(context.Background(), chCli.conn)
	require.NoError(t, err)

	cids, batch := createSharedCidsbatch(t, context.Background(), chCli.conn)

	// test the schema of the db
	err = ValidateSharedCidsTableSchema(context.Background(), chCli.conn)
	require.NoError(t, err)

	chCli.send(context.Background(), batch, CidsTableName)

	// do the requests
	// get all the cids
	respCids, err := RequestCids(context.Background(), chCli.conn)
	require.NoError(t, err)
	require.Equal(t, 3, len(respCids))
	for i, cid := range respCids {
		require.Equal(t, cids[i].Timestamp.Unix(), cid.Timestamp.Unix())
		require.Equal(t, cids[i].Cid, cid.Cid)
		require.Equal(t, cids[i].Direction, cid.Direction)
		require.Equal(t, cids[i].Producer, cid.Producer)
		require.Equal(t, cids[i].By, cid.By)
		require.Equal(t, cids[i].Type, cid.Type)
		require.Equal(t, cids[i].Origin, cid.Origin)
	}

	// get between dates cids
	respCids, err = RequestCids(
		context.Background(),
		chCli.conn,
		WithinDates(time.Now().Add(-23*time.Hour), time.Now()),
	)
	require.NoError(t, err)
	require.Equal(t, 0, len(respCids))

	// get bitswap cids
	respCids, err = RequestCids(context.Background(), chCli.conn, WithOrigin(OriginBitswap))
	require.NoError(t, err)
	require.Equal(t, 1, len(respCids))
	require.Equal(t, cids[0].Timestamp.Unix(), respCids[0].Timestamp.Unix())
	require.Equal(t, cids[0].Cid, respCids[0].Cid)
	require.Equal(t, cids[0].Direction, respCids[0].Direction)
	require.Equal(t, cids[0].Producer, respCids[0].Producer)
	require.Equal(t, cids[0].By, respCids[0].By)
	require.Equal(t, cids[0].Type, respCids[0].Type)
	require.Equal(t, cids[0].Origin, respCids[0].Origin)

	// get dht cids
	respCids, err = RequestCids(context.Background(), chCli.conn, WithOrigin(OriginDHT))
	require.NoError(t, err)
	require.Equal(t, 2, len(respCids))
	for i, cid := range respCids {
		require.Equal(t, cids[i+1].Timestamp.Unix(), cid.Timestamp.Unix())
		require.Equal(t, cids[i+1].Cid, cid.Cid)
		require.Equal(t, cids[i+1].Direction, cid.Direction)
		require.Equal(t, cids[i+1].Producer, cid.Producer)
		require.Equal(t, cids[i+1].By, cid.By)
		require.Equal(t, cids[i+1].Type, cid.Type)
		require.Equal(t, cids[i+1].Origin, cid.Origin)
	}

	// get dht add-providers cids
	respCids, err = RequestCids(context.Background(), chCli.conn, WithOrigin(OriginDHT), WithMsgType(DhtAddProviders))
	require.NoError(t, err)
	require.Equal(t, 1, len(respCids))
	require.Equal(t, cids[1].Timestamp.Unix(), respCids[0].Timestamp.Unix())
	require.Equal(t, cids[1].Cid, respCids[0].Cid)
	require.Equal(t, cids[1].Direction, respCids[0].Direction)
	require.Equal(t, cids[1].Producer, respCids[0].Producer)
	require.Equal(t, cids[1].By, respCids[0].By)
	require.Equal(t, cids[1].Type, respCids[0].Type)
	require.Equal(t, cids[1].Origin, respCids[0].Origin)

	dropAllShardeCidTable(context.Background(), chCli.conn)
	require.NoError(t, err)
}

func createTestDB(t *testing.T) *ClickhouseDB {
	// init the db
	config := &ChConfig{
		ClickHouseConfig: db.ClickHouseConfig{
			BaseConfig: &db.ClickHouseBaseConfig{
				Host: "127.0.0.1",
				Port: 9000,
				User: "username",
				Pass: "password",
				SSL:  false,
			},
			Database: "bitswap_sniffer_ipfs",
		},
		ClickHouseMigrationsConfig: db.ClickHouseMigrationsConfig{
			MigrationsTableEngine:  "TinyLog",
			MultiStatementEnabled:  false,
			ReplicatedTableEngines: false,
		},
		BatchSize: 1,
		Flushers:  1,
		Telemetry: sdkmetrics.NewMeterProvider(),
	}

	clickhouse, err := NewClickhouseDB(config, logrus.New())
	require.NoError(t, err)
	return clickhouse
}

func createSharedCidsbatch(t *testing.T, ctx context.Context, db driver.Conn) ([]SharedCid, driver.Batch) {
	cids := []SharedCid{
		SharedCid{
			Timestamp: time.Now().Add(-24 * time.Hour).UTC(),
			Direction: "received",
			Cid:       "cid_1",
			Producer:  "ProducerPeer",
			By:        "RemotePeerID",
			Type:      BitswapWantType,
			Origin:    OriginBitswap,
		},
		SharedCid{
			Timestamp: time.Now().Add(-24 * time.Hour).UTC(),
			Direction: "received",
			Cid:       "cid_2",
			Producer:  "ProducerPeer",
			By:        "RemotePeerID",
			Type:      DhtAddProviders,
			Origin:    OriginDHT,
		},
		SharedCid{
			Timestamp: time.Now().Add(-24 * time.Hour).UTC(),
			Direction: "received",
			Cid:       "cid_3",
			Producer:  "ProducerPeer",
			By:        "RemotePeerID",
			Type:      DhtGetProviders,
			Origin:    OriginDHT,
		},
	}

	batch, err := PrepareSharedCidsBatch(ctx, db, cids)
	require.NoError(t, err)
	return cids, batch
}
