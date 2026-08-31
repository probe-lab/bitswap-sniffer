package bitswap

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/pkg/errors"
	"github.com/probe-lab/go-commons/db"
	"go.opentelemetry.io/otel/attribute"
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
	Flushers  int
	Telemetry metric.MeterProvider
}

func (c *ChConfig) Validate() error {
	return c.ClickHouseConfig.BaseConfig.Validate()
}

type ClickhouseDB struct {
	config *ChConfig
	log    *slog.Logger

	conn           driver.Conn
	cidC           chan []SharedCid
	cidBatcher     *cidBatcher
	closeFlusher   chan struct{}
	flusherClosedC chan struct{}

	// metrics
	insertRowCount         metric.Int64Counter
	insertLatencyHistogram metric.Int64Histogram
}

func NewClickhouseDB(config *ChConfig, log *slog.Logger) (*ClickhouseDB, error) {
	db := &ClickhouseDB{
		config:         config,
		log:            log,
		cidBatcher:     newCidBatcher(config.BatchSize),
		cidC:           make(chan []SharedCid, config.Flushers),
		closeFlusher:   make(chan struct{}),
		flusherClosedC: make(chan struct{}),
	}
	return db, nil
}

func (db *ClickhouseDB) Init(ctx context.Context) error {
	opCtx, opCancel := context.WithTimeout(ctx, 15*time.Second)
	defer opCancel()

	var err error
	db.conn, err = db.config.OpenAndPing(opCtx)
	if err != nil {
		return errors.Wrap(err, "connecting clickhouse db")
	}

	err = db.config.Apply(db.config.Options(), clickhouseMigrations)
	if err != nil {
		return errors.Wrap(err, "making clickhouse migrations")
	}

	go db.internalFlushingLoop(ctx, db.config.Flushers)

	return db.initMetrics()
}

func (db *ClickhouseDB) internalFlushingLoop(ctx context.Context, workers int) {
	flusherT := time.NewTicker(MaxFlushInterval)
	var flusherWg sync.WaitGroup
	flushersDoneC := make(chan struct{})

	for w := range workers {
		flusherWg.Add(1)
		go func(wId int) {
			defer flusherWg.Done()
			db.log.Info("new db worker", "worker", wId)
			for {
				select {
				case cids := <-db.cidC:
					persistCids := db.cidBatcher.addCidsAndDrainIfFull(cids)
					if persistCids == nil {
						continue
					}

					opCtx, opCancel := context.WithTimeout(ctx, 5*time.Second)
					batch, err := PrepareSharedCidsBatch(opCtx, db.conn, persistCids)
					if err != nil {
						db.log.Error("batching shared cids", "err", err)
						opCancel()
						continue
					}
					db.send(opCtx, batch, CidsTableName)
					flusherT.Reset(MaxFlushInterval)
					opCancel()

				case <-db.closeFlusher:
					db.log.Debug("closing worker, control close", "worker", wId)
					persistCids := db.cidBatcher.Reset()
					opCtx, opCancel := context.WithTimeout(ctx, 5*time.Second)
					defer opCancel()
					batch, err := PrepareSharedCidsBatch(opCtx, db.conn, persistCids)
					if err != nil {
						db.log.Error("batching shared cids", "err", err)
						return
					}
					db.send(opCtx, batch, CidsTableName)
					return

				case <-ctx.Done():
					db.log.Debug("closing worker, ctx died", "worker", wId)
					return
				}
			}
		}(w)
	}
	defer func() {
		flusherWg.Wait()
		close(flushersDoneC)
	}()

periodicFlushLoop:
	for {
		select {
		case <-flusherT.C:
			if db.cidBatcher.Len() > 0 {
				db.log.Debug("flusher triggered", "rows", db.cidBatcher.Len())
				persistCids := db.cidBatcher.Reset()

				opCtx, opCancel := context.WithTimeout(ctx, 5*time.Second)
				batch, err := PrepareSharedCidsBatch(opCtx, db.conn, persistCids)
				if err != nil {
					db.log.Error("batching shared cids", "err", err)
				}
				db.send(opCtx, batch, CidsTableName)
				flusherT.Reset(MaxFlushInterval)
				opCancel()
			}

		case <-db.closeFlusher:
			db.log.Warn("closing flusher, control close")
			break periodicFlushLoop

		case <-ctx.Done():
			db.log.Warn("closing flusher, ctx died")
			break periodicFlushLoop
		}
	}
	<-flushersDoneC
	close(db.flusherClosedC)
}

func (db *ClickhouseDB) send(ctx context.Context, batch driver.Batch, table string) {
	start := time.Now()
	err := batch.Send()
	if err != nil {
		db.log.Error("Failed to send ch batch", "rows", batch.Rows(), "table", table, "error", err.Error())
	}
	duration := time.Since(start)
	db.log.Info("Ch batch sent", "rows", batch.Rows(), "table", table, "duration", duration)
	db.insertRowCount.Add(ctx, int64(batch.Rows()))
	db.insertLatencyHistogram.Record(
		ctx,
		duration.Milliseconds(),
		metric.WithAttributes(
			attribute.String("type", table),
			attribute.Bool("success", err == nil),
		),
	)

}

func (db *ClickhouseDB) PersistCidBatch(ctx context.Context, cids []SharedCid) {
	select {
	case db.cidC <- cids:
	case <-ctx.Done():
	case <-time.After(15 * time.Second):
		db.log.Warn("attempt to queue Shared CIDs timed out -> lack of workers/resources?")
	}
}

func (db *ClickhouseDB) Close() error {
	close(db.closeFlusher)
	select {
	case <-db.flusherClosedC:
		db.log.Debug("flushing routines were shutdown")
	case <-time.After(15 * time.Second):
		db.log.Error("flushing db before shutting down took more than 15 secs, something went wrong!")
	}
	return db.conn.Close()
}

func (db *ClickhouseDB) initMetrics() error {
	var err error
	meter := db.config.Telemetry.Meter("clickhouse")
	db.insertRowCount, err = meter.Int64Counter("inserts", metric.WithDescription("Number of written rows"))
	if err != nil {
		return fmt.Errorf("inserts counter: %w", err)
	}

	db.insertLatencyHistogram, err = meter.Int64Histogram("insert_latency", metric.WithDescription("Histogram of database query times for insertions"), metric.WithUnit("milliseconds"))
	if err != nil {
		return fmt.Errorf("insert_latency histogram: %w", err)
	}
	return nil
}
