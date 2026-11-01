-- name: CreateAcademicYear :one
INSERT INTO academic_years (name, starts_on, ends_on, is_current) VALUES ($1, $2, $3, $4)
RETURNING id, name, starts_on, ends_on, is_current;

-- name: ListAcademicYears :many
SELECT id, name, starts_on, ends_on, is_current FROM academic_years WHERE archived_at IS NULL ORDER BY starts_on DESC;

-- name: CreateAcademicPeriod :one
INSERT INTO academic_periods (academic_year_id, name, starts_on, ends_on) VALUES ($1, $2, $3, $4)
RETURNING id, academic_year_id, name, starts_on, ends_on, status, allow_late_edits;

-- name: ListAcademicPeriods :many
SELECT id, academic_year_id, name, starts_on, ends_on, status, allow_late_edits
FROM academic_periods WHERE academic_year_id = $1 ORDER BY starts_on;

-- name: ListAllAcademicPeriods :many
-- Периоды всех неархивных лет: при переходе на новый год администратор готовит его заранее, до смены текущего года.
SELECT p.id, p.academic_year_id, p.name, p.starts_on, p.ends_on, p.status, p.allow_late_edits
FROM academic_periods p JOIN academic_years y ON y.id = p.academic_year_id
WHERE y.archived_at IS NULL ORDER BY p.starts_on;

-- name: GetAcademicPeriod :one
SELECT id, academic_year_id, name, starts_on, ends_on, status, allow_late_edits, closed_at, closed_by
FROM academic_periods WHERE id = $1;

-- name: CloseAcademicPeriod :exec
UPDATE academic_periods SET status = 'closed', closed_at = now(), closed_by = $2, updated_at = now() WHERE id = $1 AND status = 'open';

-- name: SetLateEdits :exec
UPDATE academic_periods SET allow_late_edits = $2, updated_at = now() WHERE id = $1;

-- name: CreateClass :one
INSERT INTO classes (academic_year_id, name) VALUES ($1, $2) RETURNING id, academic_year_id, name;

-- name: ListClasses :many
SELECT id, academic_year_id, name FROM classes WHERE archived_at IS NULL ORDER BY name;

-- name: CreateSubject :one
INSERT INTO subjects (name, short_name) VALUES ($1, $2) RETURNING id, name, short_name;

-- name: ListSubjects :many
SELECT id, name, short_name FROM subjects WHERE archived_at IS NULL ORDER BY name;

-- name: CreateRoom :one
INSERT INTO rooms (name) VALUES ($1) RETURNING id, name;

-- name: ListRooms :many
SELECT id, name FROM rooms WHERE archived_at IS NULL ORDER BY name;

-- name: CreateEnrollment :one
INSERT INTO enrollments (student_id, class_id, starts_on, ends_on) VALUES ($1, $2, $3, $4)
RETURNING id, student_id, class_id, starts_on, ends_on;

-- name: ListClassStudentsOnDate :many
SELECT s.id, u.display_name, s.student_code
FROM enrollments e JOIN students s ON s.id = e.student_id JOIN users u ON u.id = s.user_id
WHERE e.class_id = $1 AND e.starts_on <= $2::date AND (e.ends_on IS NULL OR e.ends_on >= $2::date)
ORDER BY u.display_name;

-- name: StudentInClassOnDate :one
SELECT EXISTS (SELECT 1 FROM enrollments e WHERE e.student_id = $1 AND e.class_id = $2
 AND e.starts_on <= $3::date AND (e.ends_on IS NULL OR e.ends_on >= $3::date)) AS allowed;

-- name: CreateTeachingAssignment :one
INSERT INTO teaching_assignments (teacher_id, class_id, subject_id, academic_period_id, starts_on, ends_on)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, teacher_id, class_id, subject_id, academic_period_id, starts_on, ends_on;

-- name: TeacherHasAssignment :one
SELECT EXISTS (SELECT 1 FROM teaching_assignments ta JOIN teachers t ON t.id = ta.teacher_id
 WHERE t.user_id = $1 AND ta.class_id = $2 AND ta.subject_id = $3 AND ta.academic_period_id = $4
 AND ta.starts_on <= $5::date AND (ta.ends_on IS NULL OR ta.ends_on >= $5::date)) AS allowed;

-- name: ListTeachingAssignments :many
SELECT ta.id, t.id AS teacher_id, u.display_name AS teacher_name, ta.class_id, c.name AS class_name,
       ta.subject_id, s.name AS subject_name, ta.academic_period_id, ta.starts_on, ta.ends_on
FROM teaching_assignments ta JOIN teachers t ON t.id = ta.teacher_id JOIN users u ON u.id = t.user_id
JOIN classes c ON c.id = ta.class_id JOIN subjects s ON s.id = ta.subject_id
ORDER BY c.name, s.name, u.display_name;

-- name: AssignClassTeacher :exec
INSERT INTO class_teachers (teacher_id, class_id, starts_on, ends_on) VALUES ($1, $2, $3, $4);

-- name: ClassTeacherHasClass :one
SELECT EXISTS (SELECT 1 FROM class_teachers ct JOIN teachers t ON t.id = ct.teacher_id
 WHERE t.user_id = $1 AND ct.class_id = $2 AND ct.starts_on <= $3::date
 AND (ct.ends_on IS NULL OR ct.ends_on >= $3::date)) AS allowed;

-- name: LinkParentStudent :exec
INSERT INTO parent_student (parent_id, student_id, valid_from, valid_to) VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- name: GetGradeScale :one
SELECT id, name, allowed_values FROM grade_scales WHERE is_active ORDER BY name LIMIT 1;

-- name: UpdateSchoolSettings :exec
UPDATE school_settings SET school_name = $1, timezone = $2, updated_at = now() WHERE singleton;

-- name: ListEnrollments :many
SELECT e.id, e.student_id, u.display_name AS student_name, e.class_id, c.name AS class_name, e.starts_on, e.ends_on
FROM enrollments e
JOIN students s ON s.id = e.student_id JOIN users u ON u.id = s.user_id
JOIN classes c ON c.id = e.class_id
WHERE ($1::uuid = '00000000-0000-0000-0000-000000000000' OR e.class_id = $1)
ORDER BY c.name, u.display_name, e.starts_on
LIMIT 1000;
