package bitswap

import (
	"context"
	"crypto/tls"
	"embed"
	"fmt"
	"os"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/golang-migrate/migrate/v4"
	mch "github.com/golang-migrate/migrate/v4/database/clickhouse"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	ClickhouseLocalDriver      string = "local"
	ClickhouseReplicatedDriver string = "replicated"
)

type ChConfig struct {
	Driver          string
	Host            string
	User            string
	Password        string
	Database        string
	Secure          bool
	BatchSize       int
	Flushers        int
	Cluster         string
	MigrationEngine string
	Telemetry       metric.MeterProvider
}

func (c *ChConfig) Options() *clickhouse.Options {
	opts := &clickhouse.Options{
		Addr: []string{c.Host},
		Auth: clickhouse.Auth{
			Database: c.Database,
			Username: c.User,
			Password: c.Password,
		},
	}
	if c.Secure {
		opts.TLS = &tls.Config{}
	}

	return opts
}

var MaxFlushInterval = 5 * time.Second

type ClickhouseDB struct {
	config *ChConfig
	log    *logrus.Logger

	conn         driver.Conn
	cidC         chan []SharedCid
	cidBatcher   *cidBatcher
	closeFlusher chan struct{}

	// metrics
	insertRowCount         metric.Int64Counter
	insertLatencyHistogram metric.Int64Histogram
}

func NewClickhouseDB(config *ChConfig, log *logrus.Logger) (*ClickhouseDB, error) {
	db := &ClickhouseDB{
		config:       config,
		log:          log,
		cidBatcher:   newCidBatcher(config.BatchSize),
		cidC:         make(chan []SharedCid, config.Flushers),
		closeFlusher: make(chan struct{}),
	}
	return db, nil
}

func (db *ClickhouseDB) Init(ctx context.Context) error {
	opCtx, opCancel := context.WithTimeout(ctx, 15*time.Second)
	defer opCancel()
	err := db.openConnection(opCtx)
	if err != nil {
		return errors.Wrap(err, "connecting clickhouse db")
	}

	err = db.ensureMigrations()
	if err != nil {
		return errors.Wrap(err, "making clickhouse migrations")
	}

	go db.internalFlushingLoop(ctx, db.config.Flushers)

	return db.initMetrics(ctx)
}

func (db *ClickhouseDB) openConnection(ctx context.Context) (err error) {
	db.conn, err = clickhouse.Open(db.config.Options())
	if err != nil {
		return fmt.Errorf("open clickhouse database: %w", err)
	}

	return db.conn.Ping(ctx)
}

func (db *ClickhouseDB) internalFlushingLoop(ctx context.Context, workers int) {
	flusherT := time.NewTicker(MaxFlushInterval)
	for w := range workers {
		go func(wId int) {
			db.log.WithField("worker", wId).Info("new db worker")
			for {
				select {
				case cids := <-db.cidC:
					db.cidBatcher.AddCids(cids)
					if !db.cidBatcher.IsFull() {
						continue
					}
					persistCids := db.cidBatcher.Reset()

					opCtx, opCancel := context.WithTimeout(ctx, 5*time.Second)
					batch, err := PrepareSharedCidsBatch(opCtx, db.conn, persistCids)
					if err != nil {
						db.log.Errorf("batching shared cids %v", err)
						opCancel()
						continue
					}
					db.send(opCtx, batch, CidsTableName)
					flusherT.Reset(MaxFlushInterval)
					opCancel()

				case <-db.closeFlusher:
					db.log.WithField("worker", wId).Debug("closing worker, control close")
					return

				case <-ctx.Done():
					db.log.WithField("worker", wId).Debug("closing worker, ctx died")
					return
				}
			}
		}(w)
	}

	for {
		select {
		case <-flusherT.C:
			// flush all the non-empty batches
			// flush cids
			if db.cidBatcher.Len() > 0 {
				db.log.WithField("rows", db.cidBatcher.Len()).Debug("flusher triggered")
				persistCids := db.cidBatcher.Reset()

				opCtx, opCancel := context.WithTimeout(ctx, 5*time.Second)
				batch, err := PrepareSharedCidsBatch(opCtx, db.conn, persistCids)
				if err != nil {
					db.log.Errorf("batching shared cids %v", err)
				}
				db.send(opCtx, batch, CidsTableName)
				flusherT.Reset(MaxFlushInterval)
				opCancel()
			}

		case <-db.closeFlusher:
			db.log.Warn("closing flusher, control close")
			return

		case <-ctx.Done():
			db.log.Warn("closing flusher, ctx died")
			return
		}
	}
}

func (db *ClickhouseDB) send(ctx context.Context, batch driver.Batch, table string) {
	start := time.Now()
	err := batch.Send()
	if err != nil {
		db.log.WithFields(logrus.Fields{
			"rows":  batch.Rows(),
			"table": table,
			"error": err.Error(),
		}).Error("Failed to send ch batch")
	}
	duration := time.Since(start)
	db.log.WithFields(logrus.Fields{
		"rows":     batch.Rows(),
		"table":    table,
		"duration": duration,
	}).Info("Ch batch sent")
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
	case <-time.After(15 * time.Second): // arbitrary number
		db.log.Warnf("attempt to queue Shared CIDs timed out -> lack of workers/resources?")
	}
}

func (db *ClickhouseDB) Close() error {
	close(db.closeFlusher)
	return db.conn.Close()
}

//go:embed migrations/replicated
var clickhouseClusterMigrations embed.FS

//go:embed migrations/local
var clickhouseLocalMigrations embed.FS

func (db *ClickhouseDB) ensureMigrations() error {
	db.log.Infof("applying database migrations...")

	tmpDir, err := os.MkdirTemp("", "sniffer")
	if err != nil {
		return fmt.Errorf("create tmp directory for migrations: %w", err)
	}
	defer func() {
		err = os.RemoveAll(tmpDir)
		if err != nil {
			db.log.WithFields(logrus.Fields{
				"tmpDir": tmpDir,
				"error":  err,
			}).Warn("could not clean up tmp directory")
		}
	}()
	db.log.WithField("dir", tmpDir).Debug("Created temporary directory")

	// point to the rigth migrations folder
	var migrations embed.FS
	var path string
	switch db.config.Driver {
	case "local":
		migrations = clickhouseLocalMigrations
		path = "migrations/" + db.config.Driver

	case "replicated":
		migrations = clickhouseClusterMigrations
		path = "migrations/" + db.config.Driver
	default:
		return fmt.Errorf("clickhouse doesn't support %s migrations", db.config.Driver)
	}

	migrationsDir, err := iofs.New(migrations, path)
	if err != nil {
		return fmt.Errorf("create iofs migrations source: %w", err)
	}

	conn := clickhouse.OpenDB(db.config.Options())
	mdriver, err := mch.WithInstance(conn, &mch.Config{
		DatabaseName:          db.config.Database,
		ClusterName:           db.config.Cluster,
		MigrationsTableEngine: db.config.MigrationEngine,
	})
	if err != nil {
		return fmt.Errorf("create migrate driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", migrationsDir, db.config.Database, mdriver)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}

	if err = m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate database: %w", err)
	}

	return nil
}

func (db *ClickhouseDB) initMetrics(ctx context.Context) error {
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
