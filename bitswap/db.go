package bitswap

import (
	"context"
	"embed"
	"log/slog"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/pkg/errors"
	"github.com/probe-lab/go-commons/db"
	"go.opentelemetry.io/otel/metric"
)

const (
	ClickhouseLocalDriver      string = "local"
	ClickhouseReplicatedDriver string = "replicated"
	MaxFlushInterval                  = 5 * time.Second
)

//go:embed migrations
var clickhouseMigrations embed.FS

type ChConfig struct {
	db.ClickHouseConfig
	db.ClickHouseMigrationsConfig
	BatchSize int
	Telemetry metric.MeterProvider
}

func (c *ChConfig) Validate() error {
	return c.ClickHouseConfig.BaseConfig.Validate()
}

type ClickhouseDB struct {
	config *ChConfig
	log    *slog.Logger

	conn     driver.Conn
	inserter *db.BatchInserter[SharedCid]
}

func NewClickhouseDB(config *ChConfig, log *slog.Logger) (*ClickhouseDB, error) {
	return &ClickhouseDB{
		config: config,
		log:    log,
	}, nil
}

func (c *ClickhouseDB) Init(ctx context.Context) error {
	opCtx, opCancel := context.WithTimeout(ctx, 15*time.Second)
	defer opCancel()

	var err error
	c.conn, err = c.config.OpenAndPing(opCtx)
	if err != nil {
		return errors.Wrap(err, "connecting clickhouse db")
	}

	err = c.config.Apply(c.config.Options(), clickhouseMigrations)
	if err != nil {
		return errors.Wrap(err, "making clickhouse migrations")
	}

	inserterCfg := db.DefaultBatchInserterConfig[SharedCid]()
	inserterCfg.MaxBatchSize = c.config.BatchSize
	inserterCfg.FlushInterval = MaxFlushInterval
	inserterCfg.Meter = c.config.Telemetry.Meter("clickhouse")

	c.inserter, err = db.NewBatchInserter(c.conn, CidsTableName, inserterCfg)
	if err != nil {
		return errors.Wrap(err, "creating batch inserter")
	}
	c.inserter.Start(ctx)

	return nil
}

func (c *ClickhouseDB) PersistCidBatch(ctx context.Context, cids []SharedCid) {
	for _, cid := range cids {
		if err := c.inserter.Submit(ctx, cid); err != nil {
			c.log.Warn("failed to submit shared cid", "err", err)
		}
	}
}

func (c *ClickhouseDB) Close() error {
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer stopCancel()

	if err := c.inserter.Stop(stopCtx); err != nil {
		c.log.Error("stopping batch inserter", "err", err)
	}
	return c.conn.Close()
}
