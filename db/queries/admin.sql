-- name: GetAcademicYear :one
SELECT id, name, starts_on, ends_on, is_current FROM academic_years WHERE id = $1 AND archived_at IS NULL;

-- name: AcademicPeriodOverlaps :one
SELECT EXISTS (SELECT 1 FROM academic_periods WHERE academic_year_id = $1
 AND starts_on <= $3::date AND ends_on >= $2::date) AS overlaps;

-- name: EnrollmentOverlaps :one
SELECT EXISTS (SELECT 1 FROM enrollments WHERE student_id = $1
 AND starts_on <= COALESCE($3::date, '9999-12-31') AND COALESCE(ends_on, '9999-12-31') >= $2::date) AS overlaps;

-- name: TeachingAssignmentOverlaps :one
SELECT EXISTS (SELECT 1 FROM teaching_assignments WHERE class_id = $1 AND subject_id = $2 AND academic_period_id = $3
 AND starts_on <= COALESCE($5::date, '9999-12-31') AND COALESCE(ends_on, '9999-12-31') >= $4::date) AS overlaps;

-- name: ClassTeacherOverlaps :one
SELECT EXISTS (SELECT 1 FROM class_teachers WHERE class_id = $1
 AND starts_on <= COALESCE($3::date, '9999-12-31') AND COALESCE(ends_on, '9999-12-31') >= $2::date) AS overlaps;

-- name: TeacherHasClassOnDate :one
SELECT EXISTS (SELECT 1 FROM teaching_assignments ta JOIN teachers t ON t.id = ta.teacher_id
 WHERE t.user_id = $1 AND ta.class_id = $2 AND ta.starts_on <= $3::date
 AND (ta.ends_on IS NULL OR ta.ends_on >= $3::date)) AS allowed;

-- name: UserHasActiveAdministrator :one
SELECT EXISTS (SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id
 WHERE ur.user_id = $1 AND r.code = 'administrator') AS allowed;
