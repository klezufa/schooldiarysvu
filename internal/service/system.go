package service

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"schooldiarysvu/internal/api"
	db "schooldiarysvu/internal/database/sqlc"
	"schooldiarysvu/internal/metrics"
)

// RuntimeInfo — сведения о процессе, которые показывает дашборд системного администратора.
type RuntimeInfo struct {
	Collector   *metrics.Collector
	CPU         *metrics.CPUMeter
	Instance    string
	Version     string
	Environment string
	DiskPath    string
	// NetworkProfile — local-http или https (из конфигурации).
	NetworkProfile string
	// TLSCertFile — публичный сертификат Nginx для контроля срока действия; пусто — не отслеживается.
	TLSCertFile string
}

// SetRuntimeInfo подключает сбор метрик. Без вызова дашборд показывает данные БД и сервисов, но без HTTP-нагрузки.
func (s *Service) SetRuntimeInfo(info RuntimeInfo) { s.runtime = info }

// MetricsStore возвращает хранилище для metrics.Reporter.
func (s *Service) MetricsStore() metrics.Store { return metricsStore{queries: s.queries} }

type metricsStore struct{ queries *db.Queries }

func (m metricsStore) SaveHeartbeat(ctx context.Context, service, instance, version string, startedAt time.Time, stats []byte) error {
	return m.queries.UpsertHeartbeat(ctx, db.UpsertHeartbeatParams{Service: service, Instance: instance, Version: version, StartedAt: startedAt, Stats: stats})
}

func (m metricsStore) SaveSample(ctx context.Context, service, instance string, sample []byte) error {
	return m.queries.InsertMetricSample(ctx, db.InsertMetricSampleParams{Service: service, Instance: instance, Metrics: sample})
}

func (m metricsStore) Prune(ctx context.Context) error {
	if err := m.queries.PruneMetrics(ctx); err != nil {
		return err
	}
	if err := m.queries.PruneMaintenanceEvents(ctx); err != nil {
		return err
	}
	return m.queries.PruneHeartbeats(ctx)
}

// ServerSample собирает точку ряда для процесса server; используется репортёром.
func (s *Service) ServerSample() metrics.Sample {
	http := s.runtime.Collector.HTTP()
	host := metrics.Host(s.runtime.DiskPath)
	runtimeStats := metrics.Runtime(nil, s.runtime.Collector.Started())
	sample := metrics.Sample{RequestsPerMinute: float64(http.RequestsLastMinute), ErrorRate: http.ErrorRate, P95Ms: http.P95Ms,
		Goroutines: runtimeStats.Goroutines, HeapMB: float64(runtimeStats.HeapAllocBytes) / (1 << 20), DBConnections: int(s.pool.Stat().TotalConns()), CPUPercent: host.CPUPercent}
	if host.MemoryTotal != nil && host.MemoryUsed != nil && *host.MemoryTotal > 0 {
		percent := float64(*host.MemoryUsed) / float64(*host.MemoryTotal) * 100
		sample.MemoryUsedPercent = &percent
	}
	return sample
}

// ServerStats — показатели процесса для heartbeat.
func (s *Service) ServerStats() map[string]any {
	http := s.runtime.Collector.HTTP()
	pool := s.pool.Stat()
	runtimeStats := metrics.Runtime(s.runtime.CPU, s.runtime.Collector.Started())
	return map[string]any{
		"goroutines": runtimeStats.Goroutines, "heap_alloc_bytes": runtimeStats.HeapAllocBytes, "heap_sys_bytes": runtimeStats.HeapSysBytes,
		"gc_cycles": runtimeStats.GCCycles, "gc_pause_total_ms": runtimeStats.GCPauseTotalMs, "cpu_percent": runtimeStats.CPUPercent,
		"requests_last_minute": http.RequestsLastMinute, "error_rate": http.ErrorRate, "p95_ms": http.P95Ms, "in_flight": http.InFlight,
		"db_pool_acquired": pool.AcquiredConns(), "db_pool_idle": pool.IdleConns(), "db_pool_max": pool.MaxConns(),
	}
}

func serviceStatus(lastSeen time.Time, now time.Time) api.ServiceStatusStatus {
	switch age := now.Sub(lastSeen); {
	case age <= 45*time.Second:
		return api.Up
	case age <= 5*time.Minute:
		return api.Stale
	default:
		return api.Down
	}
}

// SystemOverview собирает состояние системы. Доступно только системному администратору.
func (s *Service) SystemOverview(ctx context.Context, actor Principal) (api.SystemOverview, error) {
	if err := actor.requireSystemAdmin(); err != nil {
		return api.SystemOverview{}, err
	}
	now := time.Now()
	result := api.SystemOverview{GeneratedAt: now, Version: s.runtime.Version, Environment: s.runtime.Environment,
		Services: []api.ServiceStatus{}, Series: []api.MetricPoint{}}
	if collector := s.runtime.Collector; collector != nil {
		result.UptimeSeconds = int(now.Sub(collector.Started()).Seconds())
	}
	beats, err := s.queries.ListHeartbeats(ctx)
	if err != nil {
		return result, err
	}
	alive := map[string]bool{}
	for _, beat := range beats {
		if serviceStatus(beat.LastSeenAt, now) != api.Down {
			alive[beat.Service] = true
		}
	}
	for _, beat := range beats {
		// После перезапуска процесс получает новое имя экземпляра; старую запись «не отвечает»
		// не показываем, если у того же сервиса есть живой экземпляр.
		if serviceStatus(beat.LastSeenAt, now) == api.Down && alive[beat.Service] {
			continue
		}
		stats := map[string]any{}
		_ = json.Unmarshal(beat.Stats, &stats)
		result.Services = append(result.Services, api.ServiceStatus{Service: beat.Service, Instance: beat.Instance, Version: beat.Version, Status: serviceStatus(beat.LastSeenAt, now),
			StartedAt: beat.StartedAt, LastSeenAt: beat.LastSeenAt, UptimeSeconds: int(now.Sub(beat.StartedAt).Seconds()), Stats: stats})
	}
	if result.Database, err = s.databaseOverview(ctx); err != nil {
		return result, err
	}
	host := metrics.Host(s.runtime.DiskPath)
	result.Host = api.HostOverview{Os: runtime.GOOS, Arch: runtime.GOARCH, NumCpu: runtime.NumCPU(), GoVersion: runtime.Version(),
		CpuPercent: float32Ptr(host.CPUPercent), MemoryTotal: host.MemoryTotal, MemoryUsed: host.MemoryUsed, DiskTotal: host.DiskTotal, DiskUsed: host.DiskUsed}
	if host.DiskPath != "" {
		path := host.DiskPath
		result.Host.DiskPath = &path
	}
	result.Http = api.HTTPOverview{Routes: []api.RouteStat{}}
	if collector := s.runtime.Collector; collector != nil {
		snapshot := collector.HTTP()
		result.Http = api.HTTPOverview{RequestsLastMinute: snapshot.RequestsLastMinute, RequestsLastHour: snapshot.RequestsLastHour, ErrorRate: float32(snapshot.ErrorRate),
			P50Ms: float32(snapshot.P50Ms), P95Ms: float32(snapshot.P95Ms), InFlight: snapshot.InFlight, Routes: make([]api.RouteStat, 0, len(snapshot.Routes))}
		for _, route := range snapshot.Routes {
			result.Http.Routes = append(result.Http.Routes, api.RouteStat{Route: route.Route, Count: route.Count, Errors: route.Errors, P95Ms: float32(route.P95Ms)})
		}
	}
	storage, err := s.queries.FileStorageStats(ctx)
	if err != nil {
		return result, err
	}
	result.Storage = api.StorageOverview{FilesCount: storage.FilesCount, FilesBytes: storage.FilesBytes}
	if result.Counts, err = s.schoolCounts(ctx); err != nil {
		return result, err
	}
	if result.Operations, err = s.operationsOverview(ctx, now); err != nil {
		return result, err
	}
	samples, err := s.queries.ListMetricSamples(ctx, db.ListMetricSamplesParams{SampledAt: now.Add(-time.Hour), Service: "server"})
	if err != nil {
		return result, err
	}
	// Ряд строится по всем экземплярам сервиса: после перезапуска история не обрывается.
	for _, sample := range samples {
		var decoded metrics.Sample
		if err := json.Unmarshal(sample.Metrics, &decoded); err != nil {
			continue
		}
		result.Series = append(result.Series, api.MetricPoint{T: sample.SampledAt, CpuPercent: float32Ptr(decoded.CPUPercent), MemoryUsedPercent: float32Ptr(decoded.MemoryUsedPercent),
			RequestsPerMinute: float32(decoded.RequestsPerMinute), ErrorRate: float32(decoded.ErrorRate), P95Ms: float32(decoded.P95Ms), Goroutines: decoded.Goroutines,
			HeapMb: float32(decoded.HeapMB), DbConnections: decoded.DBConnections})
	}
	return result, nil
}

func float32Ptr(value *float64) *float32 {
	if value == nil {
		return nil
	}
	converted := float32(*value)
	return &converted
}

func (s *Service) databaseOverview(ctx context.Context) (api.DatabaseOverview, error) {
	var (
		overview                                       api.DatabaseOverview
		longest                                        float64
		hit, read                                      int64
		startedAt                                      pgtype.Timestamptz
		size, commits, rollbacks, deadlocks, tempFiles int64
		connections, active, maxConnections            int
	)
	err := s.pool.QueryRow(ctx, `
SELECT pg_database_size(current_database()), version(),
       (SELECT setting::int FROM pg_settings WHERE name = 'max_connections'),
       (SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()),
       (SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND state = 'active'),
       COALESCE((SELECT extract(epoch FROM max(now() - query_start)) FROM pg_stat_activity
                 WHERE datname = current_database() AND state = 'active' AND pid <> pg_backend_pid()), 0)::float8,
       d.xact_commit, d.xact_rollback, d.deadlocks, d.blks_hit, d.blks_read, d.temp_files, pg_postmaster_start_time()
FROM pg_stat_database d WHERE d.datname = current_database()`).
		Scan(&size, &overview.Version, &maxConnections, &connections, &active, &longest, &commits, &rollbacks, &deadlocks, &hit, &read, &tempFiles, &startedAt)
	if err != nil {
		return overview, err
	}
	overview.SizeBytes, overview.MaxConnections, overview.Connections, overview.ActiveConnections = size, maxConnections, connections, active
	overview.LongestQuerySeconds = float32(longest)
	overview.XactCommit, overview.XactRollback, overview.Deadlocks, overview.TempFiles = commits, rollbacks, deadlocks, tempFiles
	overview.StartedAt = startedAt.Time
	if hit+read > 0 {
		ratio := float32(float64(hit) / float64(hit+read))
		overview.CacheHitRatio = &ratio
	}
	pool := s.pool.Stat()
	overview.Pool = api.PoolStats{Max: int(pool.MaxConns()), Acquired: int(pool.AcquiredConns()), Idle: int(pool.IdleConns()), Total: int(pool.TotalConns())}
	overview.Tables = []api.TableSize{}
	rows, err := s.pool.Query(ctx, `SELECT relname::text, pg_total_relation_size(relid)::bigint, n_live_tup::bigint
FROM pg_stat_user_tables ORDER BY pg_total_relation_size(relid) DESC LIMIT 8`)
	if err != nil {
		return overview, err
	}
	defer rows.Close()
	for rows.Next() {
		var table api.TableSize
		if err := rows.Scan(&table.Name, &table.SizeBytes, &table.RowsEstimate); err != nil {
			return overview, err
		}
		overview.Tables = append(overview.Tables, table)
	}
	return overview, rows.Err()
}

// operationsOverview собирает эксплуатационные сведения: последние резервные копии и восстановление (их пишут
// скрипты резервного копирования в maintenance_events) и срок действия TLS-сертификата Nginx.
func (s *Service) operationsOverview(ctx context.Context, now time.Time) (api.OperationsOverview, error) {
	result := api.OperationsOverview{NetworkProfile: api.LocalHttp, TlsNames: []string{}}
	if s.runtime.NetworkProfile == "https" {
		result.NetworkProfile = api.Https
	}
	type eventDetails struct {
		SizeBytes *int64 `json:"size_bytes"`
		Reason    string `json:"reason"`
	}
	latest := func(kind, status string) (*time.Time, eventDetails, error) {
		row, err := s.queries.LatestMaintenanceEvent(ctx, db.LatestMaintenanceEventParams{Kind: kind, Status: status})
		if isNoRows(err) {
			return nil, eventDetails{}, nil
		}
		if err != nil {
			return nil, eventDetails{}, fmt.Errorf("latest %s %s event: %w", kind, status, err)
		}
		var details eventDetails
		_ = json.Unmarshal(row.Details, &details)
		at := row.CreatedAt
		return &at, details, nil
	}
	var details eventDetails
	var err error
	if result.LastBackupAt, details, err = latest("backup", "success"); err != nil {
		return result, err
	}
	result.LastBackupSizeBytes = details.SizeBytes
	if result.LastBackupFailureAt, details, err = latest("backup", "failure"); err != nil {
		return result, err
	}
	if details.Reason != "" {
		reason := details.Reason
		result.LastBackupFailureReason = &reason
	}
	if result.LastRestoreAt, _, err = latest("restore", "success"); err != nil {
		return result, err
	}
	if s.runtime.TLSCertFile != "" {
		certificate, err := readCertificate(s.runtime.TLSCertFile)
		if err != nil {
			message := err.Error()
			result.TlsError = &message
		} else {
			notAfter := certificate.NotAfter
			days := int(math.Floor(notAfter.Sub(now).Hours() / 24))
			result.TlsNotAfter, result.TlsDaysLeft = &notAfter, &days
			result.TlsNames = append(result.TlsNames, certificate.DNSNames...)
		}
	}
	return result, nil
}

// readCertificate читает первый сертификат PEM-файла (цепочка fullchain начинается с сертификата сервера).
func readCertificate(path string) (*x509.Certificate, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("сертификат недоступен: %w", err)
	}
	for block, rest := pem.Decode(raw); block != nil; block, rest = pem.Decode(rest) {
		if block.Type == "CERTIFICATE" {
			certificate, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("сертификат не разобран: %w", err)
			}
			return certificate, nil
		}
	}
	return nil, errors.New("в файле нет сертификата PEM")
}
