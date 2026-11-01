package metrics

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"
)

// Store — то, куда репортёр складывает показатели. Реализуется слоем сервисов поверх БД.
type Store interface {
	SaveHeartbeat(ctx context.Context, service, instance, version string, startedAt time.Time, stats []byte) error
	SaveSample(ctx context.Context, service, instance string, sample []byte) error
	Prune(ctx context.Context) error
}

// Sample — одна точка временного ряда для графиков дашборда.
type Sample struct {
	CPUPercent        *float64 `json:"cpu_percent,omitempty"`
	MemoryUsedPercent *float64 `json:"memory_used_percent,omitempty"`
	RequestsPerMinute float64  `json:"requests_per_minute"`
	ErrorRate         float64  `json:"error_rate"`
	P95Ms             float64  `json:"p95_ms"`
	Goroutines        int      `json:"goroutines"`
	HeapMB            float64  `json:"heap_mb"`
	DBConnections     int      `json:"db_connections"`
}

// Reporter раз в 15 секунд обновляет heartbeat, раз в 30 — пишет точку ряда, раз в час чистит старые данные.
type Reporter struct {
	Store    Store
	Service  string
	Instance string
	Version  string
	Started  time.Time
	// Stats возвращает произвольные показатели сервиса для heartbeat.
	Stats func() map[string]any
	// Sample возвращает точку временного ряда; nil — ряд для сервиса не ведётся.
	Sample func() Sample
	Logger *slog.Logger

	HeartbeatEvery time.Duration
	SampleEvery    time.Duration
}

func (r *Reporter) Run(ctx context.Context) {
	heartbeat, sample := r.HeartbeatEvery, r.SampleEvery
	if heartbeat <= 0 {
		heartbeat = 15 * time.Second
	}
	if sample <= 0 {
		sample = 30 * time.Second
	}
	logger := r.Logger
	if logger == nil {
		logger = slog.Default()
	}
	r.beat(ctx, logger)
	beatTicker, sampleTicker, pruneTicker := time.NewTicker(heartbeat), time.NewTicker(sample), time.NewTicker(time.Hour)
	defer beatTicker.Stop()
	defer sampleTicker.Stop()
	defer pruneTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-beatTicker.C:
			r.beat(ctx, logger)
		case <-sampleTicker.C:
			if r.Sample == nil {
				continue
			}
			payload, err := json.Marshal(r.Sample())
			if err == nil {
				err = r.Store.SaveSample(ctx, r.Service, r.Instance, payload)
			}
			if err != nil && ctx.Err() == nil {
				logger.Warn("metrics sample not saved", "service", r.Service, "error", err)
			}
		case <-pruneTicker.C:
			if err := r.Store.Prune(ctx); err != nil && ctx.Err() == nil {
				logger.Warn("metrics prune failed", "error", err)
			}
		}
	}
}

func (r *Reporter) beat(ctx context.Context, logger *slog.Logger) {
	stats := map[string]any{}
	if r.Stats != nil {
		stats = r.Stats()
	}
	payload, err := json.Marshal(stats)
	if err == nil {
		err = r.Store.SaveHeartbeat(ctx, r.Service, r.Instance, r.Version, r.Started, payload)
	}
	if err != nil && ctx.Err() == nil {
		logger.Warn("heartbeat not saved", "service", r.Service, "error", err)
	}
}
