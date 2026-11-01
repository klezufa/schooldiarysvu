-- name: CreateLessonWork :one
INSERT INTO lesson_works (lesson_id, work_type, title, weight, is_important, position)
VALUES ($1,$2,$3,$4,$5,$6) RETURNING id, lesson_id, work_type, title, weight, is_important, position;

-- name: ListLessonWorks :many
SELECT id, lesson_id, work_type, title, weight, is_important, position FROM lesson_works WHERE lesson_id = $1 ORDER BY position;

-- name: GetLessonWork :one
SELECT lw.id, lw.lesson_id, l.class_id, l.subject_id, l.lesson_date, l.academic_period_id, l.teacher_id
FROM lesson_works lw JOIN lessons l ON l.id = lw.lesson_id WHERE lw.id = $1;

-- name: CreateGrade :one
INSERT INTO grades (student_id, lesson_work_id, academic_period_id, value, comment, created_by)
VALUES ($1,$2,$3,$4,$5,$6)
RETURNING id, student_id, lesson_work_id, academic_period_id, value, comment, version;

-- name: GetGrade :one
SELECT id, student_id, lesson_work_id, academic_period_id, value, comment, version, created_by, deleted_at
FROM grades WHERE id = $1;

-- name: UpdateGrade :one
UPDATE grades SET value = $2, comment = $3, version = version + 1, updated_at = now()
WHERE id = $1 AND version = $4 AND deleted_at IS NULL
RETURNING id, value, comment, version;

-- name: SoftDeleteGrade :one
UPDATE grades SET deleted_at = now(), version = version + 1, updated_at = now()
WHERE id = $1 AND version = $2 AND deleted_at IS NULL RETURNING id, version;

-- name: ListJournalGrades :many
SELECT s.id AS student_id, u.display_name AS student_name, l.id AS lesson_id, l.lesson_date,
       lw.id AS lesson_work_id, lw.work_type, lw.title AS work_title, lw.weight,
       g.id AS grade_id, g.value AS grade_value, g.comment AS grade_comment, g.version AS grade_version
FROM lessons l
JOIN lesson_works lw ON lw.lesson_id = l.id
JOIN enrollments e ON e.class_id = l.class_id AND e.starts_on <= l.lesson_date AND (e.ends_on IS NULL OR e.ends_on >= l.lesson_date)
JOIN students s ON s.id = e.student_id JOIN users u ON u.id = s.user_id
LEFT JOIN grades g ON g.lesson_work_id = lw.id AND g.student_id = s.id AND g.deleted_at IS NULL
WHERE l.class_id = $1 AND l.subject_id = $2 AND l.academic_period_id = $3
ORDER BY u.display_name, l.lesson_date, lw.position;

-- name: ListStudentGrades :many
SELECT g.id, subj.id AS subject_id, subj.name AS subject_name, l.lesson_date, lw.work_type,
       g.value, g.comment, g.version, lw.weight
FROM grades g JOIN lesson_works lw ON lw.id = g.lesson_work_id JOIN lessons l ON l.id = lw.lesson_id
JOIN subjects subj ON subj.id = l.subject_id
WHERE g.student_id = $1 AND g.deleted_at IS NULL AND ($2::uuid IS NULL OR l.academic_period_id = $2)
ORDER BY l.lesson_date DESC, subj.name;

-- name: GradeAverages :one
SELECT COALESCE(avg(g.value), 0)::float8 AS average,
       COALESCE(sum(g.value * lw.weight) / NULLIF(sum(lw.weight), 0), 0)::float8 AS weighted_average,
       count(g.id)::integer AS grade_count
FROM grades g JOIN lesson_works lw ON lw.id = g.lesson_work_id JOIN lessons l ON l.id = lw.lesson_id
WHERE g.student_id = $1 AND l.subject_id = $2 AND l.academic_period_id = $3 AND g.deleted_at IS NULL;

-- name: UpsertTermGrade :one
INSERT INTO term_grades (student_id, subject_id, academic_period_id, value, created_by)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (student_id, subject_id, academic_period_id) DO UPDATE
SET value = EXCLUDED.value, version = term_grades.version + 1, updated_at = now()
RETURNING id, student_id, subject_id, academic_period_id, value, version;

-- name: UpsertAttendance :one
INSERT INTO attendance (lesson_id, student_id, status, reason, comment, created_by)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (lesson_id, student_id) DO UPDATE SET status = EXCLUDED.status,
 reason = EXCLUDED.reason, comment = EXCLUDED.comment, version = attendance.version + 1, updated_at = now()
RETURNING id, lesson_id, student_id, status, reason, comment, version;

-- name: MarkAllPresent :many
INSERT INTO attendance (lesson_id, student_id, status, created_by)
SELECT $1, e.student_id, 'present', $2 FROM lessons l
JOIN enrollments e ON e.class_id = l.class_id AND e.starts_on <= l.lesson_date AND (e.ends_on IS NULL OR e.ends_on >= l.lesson_date)
WHERE l.id = $1
ON CONFLICT (lesson_id, student_id) DO UPDATE SET status = 'present', reason = NULL,
 comment = NULL, version = attendance.version + 1, updated_at = now()
RETURNING student_id, status, version;

-- name: ListAttendance :many
SELECT a.id, a.lesson_id, a.student_id, u.display_name AS student_name, a.status, a.reason, a.comment, a.version
FROM attendance a JOIN students s ON s.id = a.student_id JOIN users u ON u.id = s.user_id
WHERE a.lesson_id = $1 ORDER BY u.display_name;
