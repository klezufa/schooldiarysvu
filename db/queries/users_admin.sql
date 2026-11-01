-- name: GetUserAnyByID :one
SELECT id, login, display_name, is_active, must_change_password, created_at, deactivated_at
FROM users WHERE id = $1;

-- name: SearchUsers :many
-- Поиск и фильтрация пользователей одним запросом: роли, профили и класс собираются подзапросами.
SELECT u.id, u.login, u.display_name, u.is_active, u.must_change_password, u.created_at,
       COALESCE((SELECT array_agg(r.code ORDER BY r.code) FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = u.id), '{}')::text[] AS roles,
       st.id AS student_id, t.id AS teacher_id, p.id AS parent_id,
       u.last_login_at, COALESCE(cl.name, '')::text AS class_name,
       count(*) OVER ()::bigint AS total
FROM users u
LEFT JOIN students st ON st.user_id = u.id
LEFT JOIN teachers t ON t.user_id = u.id
LEFT JOIN parents p ON p.user_id = u.id
LEFT JOIN LATERAL (SELECT c.name FROM enrollments e JOIN classes c ON c.id = e.class_id
  WHERE e.student_id = st.id AND e.starts_on <= school_today() AND (e.ends_on IS NULL OR e.ends_on >= school_today())
  ORDER BY e.starts_on DESC LIMIT 1) cl ON true
WHERE (sqlc.arg(search)::text = '' OR ru_lower(u.display_name) LIKE '%' || sqlc.arg(search) || '%' ESCAPE '\' OR u.login LIKE '%' || sqlc.arg(search) || '%' ESCAPE '\')
  AND (sqlc.arg(role)::text = '' OR EXISTS (SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = u.id AND r.code = sqlc.arg(role)))
  AND (sqlc.arg(status)::text = 'all' OR (sqlc.arg(status) = 'active' AND u.is_active) OR (sqlc.arg(status) = 'inactive' AND NOT u.is_active))
  AND (sqlc.narg(class_id)::uuid IS NULL OR EXISTS (SELECT 1 FROM enrollments e WHERE e.student_id = st.id AND e.class_id = sqlc.narg(class_id)
        AND e.starts_on <= school_today() AND (e.ends_on IS NULL OR e.ends_on >= school_today())))
ORDER BY u.display_name, u.id
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: UpdateUserProfile :one
UPDATE users SET display_name = $2, updated_at = now() WHERE id = $1
RETURNING id, login, display_name, is_active, must_change_password, created_at;

-- name: ReactivateUser :exec
UPDATE users SET is_active = true, deactivated_at = NULL, updated_at = now() WHERE id = $1;

-- name: ResetUserPassword :exec
UPDATE users SET password_hash = $2, must_change_password = true, updated_at = now() WHERE id = $1;

-- name: RemoveRole :exec
DELETE FROM user_roles WHERE user_id = $1 AND role_id = (SELECT id FROM roles WHERE code = $2);

-- name: CountActiveUsersWithRole :one
SELECT count(*)::bigint FROM user_roles ur JOIN roles r ON r.id = ur.role_id JOIN users u ON u.id = ur.user_id
WHERE r.code = $1 AND u.is_active;

-- name: EnsureStudent :exec
INSERT INTO students (user_id, student_code) VALUES ($1, $2) ON CONFLICT (user_id) DO NOTHING;

-- name: EnsureTeacher :exec
INSERT INTO teachers (user_id) VALUES ($1) ON CONFLICT (user_id) DO NOTHING;

-- name: EnsureParent :exec
INSERT INTO parents (user_id) VALUES ($1) ON CONFLICT (user_id) DO NOTHING;

-- name: ListLoginsWithPrefix :many
SELECT login FROM users WHERE login = $1 OR login LIKE $1 || '%' ESCAPE '\';

-- name: FindClassByName :one
SELECT id, name FROM classes WHERE academic_year_id = $1 AND ru_lower(name) = ru_lower($2) AND archived_at IS NULL;

-- name: CurrentAcademicYearID :one
SELECT id FROM academic_years WHERE is_current AND archived_at IS NULL;

-- name: ActiveEnrollmentClass :one
SELECT e.id, e.class_id, e.starts_on FROM enrollments e
WHERE e.student_id = $1 AND e.starts_on <= $2::date AND (e.ends_on IS NULL OR e.ends_on >= $2::date)
ORDER BY e.starts_on DESC LIMIT 1;

-- name: EndEnrollment :exec
UPDATE enrollments SET ends_on = $2 WHERE id = $1;

-- name: ListActiveClassStudentIDs :many
SELECT e.student_id FROM enrollments e
WHERE e.class_id = $1 AND e.starts_on <= $2::date AND (e.ends_on IS NULL OR e.ends_on >= $2::date);

-- name: UnenrolledStudents :many
SELECT st.id AS student_id, u.display_name
FROM students st JOIN users u ON u.id = st.user_id
WHERE u.is_active AND NOT EXISTS (SELECT 1 FROM enrollments e WHERE e.student_id = st.id
  AND e.starts_on <= school_today() AND (e.ends_on IS NULL OR e.ends_on >= school_today()))
ORDER BY u.display_name;

-- name: CreateInvitation :one
INSERT INTO invitations (token_hash, role_codes, note, class_id, student_id, created_by, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, role_codes, note, class_id, student_id, created_at, expires_at;

-- name: ListInvitations :many
SELECT i.id, i.role_codes, i.note, i.class_id, c.name AS class_name, i.student_id, su.display_name AS student_name,
       i.created_at, i.expires_at, i.used_at, i.revoked_at, cu.display_name AS created_by_name, uu.login AS used_by_login
FROM invitations i
LEFT JOIN classes c ON c.id = i.class_id
LEFT JOIN students st ON st.id = i.student_id LEFT JOIN users su ON su.id = st.user_id
JOIN users cu ON cu.id = i.created_by
LEFT JOIN users uu ON uu.id = i.used_by
ORDER BY i.created_at DESC LIMIT $1 OFFSET $2;

-- name: GetInvitationByHash :one
SELECT i.id, i.role_codes, i.note, i.class_id, c.name AS class_name, i.student_id, su.display_name AS student_name,
       i.expires_at, i.used_at, i.revoked_at
FROM invitations i
LEFT JOIN classes c ON c.id = i.class_id
LEFT JOIN students st ON st.id = i.student_id LEFT JOIN users su ON su.id = st.user_id
WHERE i.token_hash = $1;

-- name: ConsumeInvitation :one
UPDATE invitations SET used_at = now(), used_by = $2
WHERE id = $1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at > now()
RETURNING id;

-- name: RevokeInvitation :one
UPDATE invitations SET revoked_at = now() WHERE id = $1 AND used_at IS NULL AND revoked_at IS NULL RETURNING id;

-- name: GetInvitationCreator :one
SELECT created_by FROM invitations WHERE id = $1;

-- name: GetInvitationRoles :one
SELECT role_codes FROM invitations WHERE id = $1;

-- name: GetAcademicYearIDForClass :one
SELECT academic_year_id FROM classes WHERE id = $1 AND archived_at IS NULL;

-- name: GetClassAcademicYear :one
-- Учебный год действующего класса: границы года ограничивают даты зачисления, назначений и шаблона.
SELECT c.academic_year_id, y.name AS year_name, y.starts_on, y.ends_on
FROM classes c JOIN academic_years y ON y.id = c.academic_year_id
WHERE c.id = $1 AND c.archived_at IS NULL AND y.archived_at IS NULL;

-- name: TouchLastLogin :exec
UPDATE users SET last_login_at = now() WHERE id = $1;
