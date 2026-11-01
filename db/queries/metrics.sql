-- name: UpsertHeartbeat :exec
INSERT INTO service_heartbeats (service, instance, version, started_at, last_seen_at, stats)
VALUES ($1, $2, $3, $4, now(), $5)
ON CONFLICT (service, instance) DO UPDATE SET version = EXCLUDED.version, started_at = EXCLUDED.started_at,
    last_seen_at = now(), stats = EXCLUDED.stats;

-- name: InsertMetricSample :exec
INSERT INTO system_metric_samples (service, instance, sampled_at, metrics)
VALUES ($1, $2, date_trunc('second', now()), $3)
ON CONFLICT DO NOTHING;

-- name: ListHeartbeats :many
SELECT service, instance, version, started_at, last_seen_at, stats FROM service_heartbeats ORDER BY service, instance;

-- name: ListMetricSamples :many
SELECT service, instance, sampled_at, metrics FROM system_metric_samples
WHERE sampled_at >= $1 AND service = $2 ORDER BY sampled_at;

-- name: PruneMetrics :exec
DELETE FROM system_metric_samples WHERE sampled_at < now() - interval '48 hours';

-- name: PruneHeartbeats :exec
DELETE FROM service_heartbeats WHERE last_seen_at < now() - interval '7 days';

-- name: FileStorageStats :one
SELECT count(*)::bigint AS files_count, COALESCE(sum(size_bytes), 0)::bigint AS files_bytes FROM files WHERE deleted_at IS NULL;

-- name: LatestMaintenanceEvent :one
SELECT created_at, details FROM maintenance_events WHERE kind = $1 AND status = $2 ORDER BY created_at DESC LIMIT 1;

-- name: PruneMaintenanceEvents :exec
DELETE FROM maintenance_events WHERE created_at < now() - interval '400 days';
