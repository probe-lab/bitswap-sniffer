package bitswap

import (
	"context"
	"log/slog"
	"time"
)

func (s *Sniffer) measureDiskUsage(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		usage, err := s.ds.DiskUsage(ctx)
		if err != nil {
			slog.Warn("Failed getting disk usage", "err", err)
			continue
		}

		s.diskUsageGauge.Record(ctx, float64(usage))
	}
}
