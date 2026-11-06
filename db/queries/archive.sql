-- name: GetArchiveClass :one
SELECT c.id, c.name AS class_name, y.id AS academic_year_id, y.name AS year_name, y.starts_on, y.ends_on,
       COALESCE((SELECT school_name FROM school_settings LIMIT 1), '')::text AS school_name
FROM classes c JOIN academic_years y ON y.id = c.academic_year_id WHERE c.id = $1;

-- name: ListArchivePeriods :many
SELECT id, name, starts_on, ends_on, status FROM academic_periods WHERE academic_year_id = $1 ORDER BY starts_on, name;

-- name: ListArchiveStudents :many
-- Все, кто учился в классе в этом году, включая выбывших: в архиве остаются и их оценки.
SELECT s.id, u.display_name, min(e.starts_on)::date AS starts_on,
       bool_or(e.ends_on IS NULL)::boolean AS active, COALESCE(max(e.ends_on), min(e.starts_on))::date AS left_on
FROM enrollments e JOIN students s ON s.id = e.student_id JOIN users u ON u.id = s.user_id
WHERE e.class_id = $1
GROUP BY s.id, u.display_name
ORDER BY u.display_name, s.id;

-- name: ListArchiveLessons :many
SELECT l.id, l.subject_id, s.name AS subject_name, l.academic_period_id, l.lesson_date, l.starts_at, l.topic, l.status,
       COALESCE(l.cancellation_reason, '')::text AS cancellation_reason, tu.display_name AS teacher_name,
       COALESCE(ou.display_name, '')::text AS original_teacher_name,
       COALESCE((SELECT string_agg(h.text, ' / ' ORDER BY h.created_at) FROM homework h
                 WHERE h.lesson_id = l.id AND h.status = 'published'), '')::text AS homework
FROM lessons l
JOIN subjects s ON s.id = l.subject_id
JOIN teachers t ON t.id = l.teacher_id JOIN users tu ON tu.id = t.user_id
LEFT JOIN lesson_substitutions ls ON ls.lesson_id = l.id
LEFT JOIN teachers ot ON ot.id = ls.original_teacher_id LEFT JOIN users ou ON ou.id = ot.user_id
WHERE l.class_id = $1
ORDER BY s.name, l.subject_id, l.starts_at, l.id;

-- name: ListArchiveGrades :many
SELECT lw.lesson_id, g.student_id, g.value
FROM grades g JOIN lesson_works lw ON lw.id = g.lesson_work_id JOIN lessons l ON l.id = lw.lesson_id
WHERE l.class_id = $1 AND g.deleted_at IS NULL
ORDER BY lw.lesson_id, g.student_id, lw.position, g.id;

-- name: ListArchiveAttendance :many
SELECT a.lesson_id, a.student_id, a.status FROM attendance a JOIN lessons l ON l.id = a.lesson_id
WHERE l.class_id = $1 AND a.status <> 'present'
ORDER BY a.lesson_id, a.student_id;

-- name: ListArchiveTermGrades :many
SELECT tg.student_id, tg.subject_id, s.name AS subject_name, tg.academic_period_id, tg.value
FROM term_grades tg JOIN subjects s ON s.id = tg.subject_id
WHERE tg.academic_period_id IN (SELECT ap.id FROM academic_periods ap WHERE ap.academic_year_id = sqlc.arg(academic_year_id))
  AND tg.student_id IN (SELECT e.student_id FROM enrollments e WHERE e.class_id = sqlc.arg(class_id))
ORDER BY s.name, tg.subject_id, tg.student_id, tg.academic_period_id;

-- name: FindJournalArchiveIssue :one
SELECT a.created_at, a.new_values, COALESCE(u.display_name, '')::text AS issued_by
FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id
WHERE a.action = 'journal_archive.issued' AND a.new_values->>'code' = sqlc.arg(code)::text
ORDER BY a.created_at LIMIT 1;
