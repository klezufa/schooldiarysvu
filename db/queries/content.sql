-- name: CreateAnnouncement :one
INSERT INTO announcements (author_id, title, body) VALUES ($1,$2,$3)
RETURNING id, title, body, published_at;

-- name: AddAnnouncementAudience :exec
INSERT INTO announcement_audiences (announcement_id, audience_type, class_id, subject_id) VALUES ($1,$2,$3,$4);

-- name: ListAnnouncementsForStudent :many
SELECT DISTINCT a.id, a.title, a.body, a.published_at FROM announcements a
LEFT JOIN announcement_audiences aa ON aa.announcement_id = a.id
LEFT JOIN enrollments e ON e.class_id = aa.class_id AND e.student_id = $1
LEFT JOIN lessons l ON l.subject_id = aa.subject_id AND l.class_id = e.class_id
WHERE a.archived_at IS NULL AND (aa.audience_type = 'school' OR e.student_id IS NOT NULL OR l.id IS NOT NULL)
ORDER BY a.published_at DESC LIMIT 50;

-- name: CreateNotification :exec
INSERT INTO notifications (user_id, kind, title, body, entity_type, entity_id) VALUES ($1,$2,$3,$4,$5,$6);

-- name: ListNotifications :many
SELECT id, kind, title, body, entity_type, entity_id, read_at, created_at
FROM notifications WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: MarkNotificationRead :execrows
UPDATE notifications SET read_at = now() WHERE id = $1 AND user_id = $2;

-- name: UpsertNotificationPreferences :one
INSERT INTO notification_preferences (user_id, internal_enabled, digest_frequency, quiet_from, quiet_to)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (user_id) DO UPDATE SET internal_enabled = EXCLUDED.internal_enabled,
 digest_frequency = EXCLUDED.digest_frequency, quiet_from = EXCLUDED.quiet_from, quiet_to = EXCLUDED.quiet_to, updated_at = now()
RETURNING user_id, internal_enabled, digest_frequency, quiet_from, quiet_to;

-- name: GetNotificationPreferences :one
SELECT user_id, internal_enabled, digest_frequency, quiet_from, quiet_to FROM notification_preferences WHERE user_id = $1;

-- name: CreateAuditLog :exec
INSERT INTO audit_logs (actor_id, action, entity_type, entity_id, old_values, new_values, request_id)
VALUES ($1,$2,$3,$4,$5,$6,$7);

-- name: ListAuditLogs :many
SELECT id, actor_id, action, entity_type, entity_id, old_values, new_values, request_id, created_at
FROM audit_logs WHERE (NULLIF($1::text, '') IS NULL OR entity_type = $1)
 AND ($2::uuid = '00000000-0000-0000-0000-000000000000' OR entity_id = $2)
 AND ($3::uuid = '00000000-0000-0000-0000-000000000000' OR actor_id = $3)
 AND (NULLIF($6::text, '') IS NULL OR action = $6)
 AND created_at >= $7::timestamptz AND created_at <= $8::timestamptz
ORDER BY created_at DESC LIMIT $4 OFFSET $5;

-- name: ListEntityAudit :many
SELECT id, actor_id, action, entity_type, entity_id, old_values, new_values, request_id, created_at
FROM audit_logs WHERE entity_type = $1 AND entity_id = $2 ORDER BY created_at DESC LIMIT 100;

-- name: GetIdempotencyResponse :one
SELECT request_hash, response FROM idempotency_keys WHERE actor_id = $1 AND key = $2 AND operation = $3;

-- name: SaveIdempotencyResponse :exec
INSERT INTO idempotency_keys (actor_id, key, operation, request_hash, response) VALUES ($1,$2,$3,$4,$5);

-- name: ClassReport :many
SELECT s.id AS student_id, u.display_name, subj.name AS subject_name,
       count(g.id)::integer AS grade_count, COALESCE(avg(g.value),0)::float8 AS average
FROM enrollments e JOIN students s ON s.id = e.student_id JOIN users u ON u.id = s.user_id
JOIN academic_periods ap ON ap.id = $2 AND e.starts_on <= ap.ends_on AND (e.ends_on IS NULL OR e.ends_on >= ap.starts_on)
JOIN lessons l ON l.class_id = e.class_id AND l.academic_period_id = $2
 AND l.lesson_date BETWEEN e.starts_on AND COALESCE(e.ends_on, ap.ends_on)
JOIN subjects subj ON subj.id = l.subject_id
LEFT JOIN lesson_works lw ON lw.lesson_id = l.id
LEFT JOIN grades g ON g.lesson_work_id = lw.id AND g.student_id = s.id AND g.deleted_at IS NULL
WHERE e.class_id = $1
GROUP BY s.id, u.display_name, subj.name ORDER BY u.display_name, subj.name;

-- name: ListGradeHistory :many
SELECT a.id, a.action, u.display_name AS actor_name, a.old_values, a.new_values, a.created_at
FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id
WHERE a.entity_type = 'grade' AND a.entity_id = $1
ORDER BY a.created_at DESC LIMIT 100;
