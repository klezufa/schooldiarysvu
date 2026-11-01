-- name: GetUserByLogin :one
SELECT id, login, password_hash, display_name, is_active, must_change_password, created_at, updated_at, deactivated_at
FROM users WHERE login = $1 AND is_active;

-- name: GetUserByID :one
SELECT id, login, password_hash, display_name, is_active, must_change_password, created_at, updated_at, deactivated_at
FROM users WHERE id = $1 AND is_active;

-- name: ListUsers :many
SELECT id, login, display_name, is_active, must_change_password, created_at
FROM users WHERE is_active ORDER BY display_name LIMIT $1 OFFSET $2;

-- name: CreateUser :one
INSERT INTO users (login, password_hash, display_name, must_change_password)
VALUES ($1, $2, $3, $4)
RETURNING id, login, display_name, is_active, must_change_password, created_at;

-- name: AssignRole :exec
INSERT INTO user_roles (user_id, role_id)
SELECT $1, id FROM roles WHERE code = $2;

-- name: GetUserRoles :many
SELECT r.code FROM roles r JOIN user_roles ur ON ur.role_id = r.id WHERE ur.user_id = $1 ORDER BY r.code;

-- name: UpdatePassword :exec
UPDATE users SET password_hash = $2, must_change_password = false, updated_at = now() WHERE id = $1 AND is_active;

-- name: DeactivateUser :exec
UPDATE users SET is_active = false, deactivated_at = now(), updated_at = now() WHERE id = $1;

-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, csrf_hash, expires_at, user_agent, ip)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, expires_at;

-- name: GetSessionByTokenHash :one
SELECT s.id AS session_id, s.user_id, s.csrf_hash, s.expires_at,
       u.login, u.display_name, u.must_change_password
FROM sessions s JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > now() AND u.is_active;

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = now() WHERE id = $1;

-- name: RevokeSession :exec
UPDATE sessions SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeUserSessions :exec
UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at < now() - interval '30 days';

-- name: CreateStudent :one
INSERT INTO students (user_id, student_code) VALUES ($1, $2) RETURNING id, user_id, student_code;

-- name: CreateTeacher :one
INSERT INTO teachers (user_id) VALUES ($1) RETURNING id, user_id;

-- name: CreateParent :one
INSERT INTO parents (user_id) VALUES ($1) RETURNING id, user_id;

-- name: GetStudentByUserID :one
SELECT id, user_id, student_code FROM students WHERE user_id = $1;

-- name: GetTeacherByUserID :one
SELECT id, user_id FROM teachers WHERE user_id = $1;

-- name: GetParentByUserID :one
SELECT id, user_id FROM parents WHERE user_id = $1;

-- name: ParentHasStudent :one
SELECT EXISTS (
    SELECT 1 FROM parent_student ps
    JOIN parents p ON p.id = ps.parent_id
    WHERE p.user_id = $1 AND ps.student_id = $2 AND ps.valid_from <= $3::date
      AND (ps.valid_to IS NULL OR ps.valid_to >= $3::date)
) AS allowed;

-- name: ListChildren :many
SELECT s.id, u.display_name, s.student_code
FROM parent_student ps
JOIN parents p ON p.id = ps.parent_id
JOIN students s ON s.id = ps.student_id
JOIN users u ON u.id = s.user_id
WHERE p.user_id = $1 AND ps.valid_from <= current_date AND (ps.valid_to IS NULL OR ps.valid_to >= current_date)
ORDER BY u.display_name;
