-- Все средние взвешены весом работы (lesson_works.weight); удалённые оценки не учитываются.
-- Отсутствие данных кодируется значением -1 (sqlc не умеет выводить NULL для приведённых выражений); Go превращает его в null.

-- name: StudentSubjectAverages :many
SELECT s.id AS subject_id, s.name AS subject_name,
       COALESCE(sum(g.value * lw.weight) / NULLIF(sum(lw.weight), 0), -1)::float8 AS average,
       count(*)::int AS grades_count,
       COALESCE(max(tg.value), -1)::float8 AS term_value
FROM grades g
JOIN lesson_works lw ON lw.id = g.lesson_work_id
JOIN lessons l ON l.id = lw.lesson_id
JOIN subjects s ON s.id = l.subject_id
LEFT JOIN term_grades tg ON tg.student_id = g.student_id AND tg.subject_id = s.id AND tg.academic_period_id = g.academic_period_id
WHERE g.student_id = $1 AND g.academic_period_id = $2 AND g.deleted_at IS NULL
GROUP BY s.id, s.name ORDER BY s.name;

-- name: StudentWeeklyDynamics :many
SELECT date_trunc('week', l.lesson_date)::date AS week_start,
       COALESCE(sum(g.value * lw.weight) / NULLIF(sum(lw.weight), 0), -1)::float8 AS average,
       count(*)::int AS grades_count
FROM grades g
JOIN lesson_works lw ON lw.id = g.lesson_work_id
JOIN lessons l ON l.id = lw.lesson_id
WHERE g.student_id = $1 AND g.academic_period_id = $2 AND g.deleted_at IS NULL
GROUP BY 1 ORDER BY 1;

-- name: StudentGradeDistribution :many
SELECT g.value::float8 AS value, count(*)::int AS grades_count
FROM grades g WHERE g.student_id = $1 AND g.academic_period_id = $2 AND g.deleted_at IS NULL
GROUP BY g.value ORDER BY g.value;

-- name: StudentRecentGrades :many
SELECT l.lesson_date, s.name AS subject_name, lw.title, lw.work_type, g.value::float8 AS value
FROM grades g
JOIN lesson_works lw ON lw.id = g.lesson_work_id
JOIN lessons l ON l.id = lw.lesson_id
JOIN subjects s ON s.id = l.subject_id
WHERE g.student_id = $1 AND g.deleted_at IS NULL
ORDER BY l.lesson_date DESC, g.updated_at DESC LIMIT $2;

-- name: StudentAttendanceCounts :many
SELECT a.status, count(*)::int AS lessons_count
FROM attendance a JOIN lessons l ON l.id = a.lesson_id
WHERE a.student_id = $1 AND l.academic_period_id = $2
GROUP BY a.status;

-- name: StudentHomeworkStats :one
SELECT count(*) FILTER (WHERE hc.completed_at IS NOT NULL)::int AS done,
       count(*) FILTER (WHERE hc.completed_at IS NULL AND h.due_date >= sqlc.arg(today)::date)::int AS open,
       count(*) FILTER (WHERE hc.completed_at IS NULL AND h.due_date < sqlc.arg(today)::date)::int AS overdue
FROM homework h
JOIN enrollments e ON e.class_id = h.class_id AND e.student_id = sqlc.arg(student_id)
 AND e.starts_on <= h.due_date AND (e.ends_on IS NULL OR e.ends_on >= h.due_date)
LEFT JOIN homework_completions hc ON hc.homework_id = h.id AND hc.student_id = sqlc.arg(student_id)
WHERE h.status = 'published'
  AND h.due_date BETWEEN sqlc.arg(today)::date - 30 AND sqlc.arg(today)::date + 60
  AND (NOT EXISTS (SELECT 1 FROM homework_recipients r WHERE r.homework_id = h.id)
       OR EXISTS (SELECT 1 FROM homework_recipients r WHERE r.homework_id = h.id AND r.student_id = sqlc.arg(student_id)));

-- name: StudentUpcomingHomework :many
SELECT h.id, s.name AS subject_name, h.text, h.due_date, (hc.completed_at IS NOT NULL)::boolean AS completed
FROM homework h
JOIN subjects s ON s.id = h.subject_id
JOIN enrollments e ON e.class_id = h.class_id AND e.student_id = sqlc.arg(student_id)
 AND e.starts_on <= h.due_date AND (e.ends_on IS NULL OR e.ends_on >= h.due_date)
LEFT JOIN homework_completions hc ON hc.homework_id = h.id AND hc.student_id = sqlc.arg(student_id)
WHERE h.status = 'published' AND h.due_date >= sqlc.arg(today)::date
  AND (NOT EXISTS (SELECT 1 FROM homework_recipients r WHERE r.homework_id = h.id)
       OR EXISTS (SELECT 1 FROM homework_recipients r WHERE r.homework_id = h.id AND r.student_id = sqlc.arg(student_id)))
ORDER BY h.due_date, s.name LIMIT sqlc.arg(row_limit);

-- name: TeacherClassStats :many
SELECT c.id AS class_id, c.name AS class_name, s.id AS subject_id, s.name AS subject_name,
       (SELECT count(*) FROM enrollments e WHERE e.class_id = c.id AND e.starts_on <= current_date
         AND (e.ends_on IS NULL OR e.ends_on >= current_date))::int AS students,
       COALESCE((SELECT sum(g.value * lw.weight) / NULLIF(sum(lw.weight), 0)
          FROM grades g JOIN lesson_works lw ON lw.id = g.lesson_work_id JOIN lessons l ON l.id = lw.lesson_id
         WHERE l.teacher_id = ta.teacher_id AND l.class_id = c.id AND l.subject_id = s.id
           AND g.academic_period_id = ta.academic_period_id AND g.deleted_at IS NULL), -1)::float8 AS average,
       COALESCE((SELECT count(*) FILTER (WHERE a.status IN ('present','late'))::float8 / NULLIF(count(*) FILTER (WHERE a.status <> 'exempt'), 0)
          FROM attendance a JOIN lessons l ON l.id = a.lesson_id
         WHERE l.teacher_id = ta.teacher_id AND l.class_id = c.id AND l.subject_id = s.id
           AND l.academic_period_id = ta.academic_period_id), -1)::float8 AS attendance_rate
FROM teaching_assignments ta
JOIN classes c ON c.id = ta.class_id AND c.archived_at IS NULL
JOIN subjects s ON s.id = ta.subject_id
WHERE ta.teacher_id = $1 AND ta.academic_period_id = $2
  AND ta.starts_on <= current_date AND (ta.ends_on IS NULL OR ta.ends_on >= current_date)
ORDER BY c.name, s.name;

-- name: TeacherGradeDistribution :many
SELECT g.value::float8 AS value, count(*)::int AS grades_count
FROM grades g JOIN lesson_works lw ON lw.id = g.lesson_work_id JOIN lessons l ON l.id = lw.lesson_id
WHERE l.teacher_id = $1 AND g.academic_period_id = $2 AND g.deleted_at IS NULL
GROUP BY g.value ORDER BY g.value;

-- name: TeacherWeeklyDynamics :many
SELECT date_trunc('week', l.lesson_date)::date AS week_start,
       COALESCE(sum(g.value * lw.weight) / NULLIF(sum(lw.weight), 0), -1)::float8 AS average,
       count(*)::int AS grades_count
FROM grades g JOIN lesson_works lw ON lw.id = g.lesson_work_id JOIN lessons l ON l.id = lw.lesson_id
WHERE l.teacher_id = $1 AND g.academic_period_id = $2 AND g.deleted_at IS NULL
GROUP BY 1 ORDER BY 1;

-- name: TeacherLowPerformers :many
SELECT st.id AS student_id, u.display_name, c.name AS class_name,
       COALESCE(sum(g.value * lw.weight) / NULLIF(sum(lw.weight), 0), -1)::float8 AS average, count(*)::int AS grades_count
FROM grades g JOIN lesson_works lw ON lw.id = g.lesson_work_id JOIN lessons l ON l.id = lw.lesson_id
JOIN students st ON st.id = g.student_id JOIN users u ON u.id = st.user_id JOIN classes c ON c.id = l.class_id
WHERE l.teacher_id = $1 AND g.academic_period_id = $2 AND g.deleted_at IS NULL
GROUP BY st.id, u.display_name, c.name
HAVING count(*) >= 3
ORDER BY average ASC, u.display_name LIMIT $3;

-- name: TeacherLessonCounts :one
SELECT count(*) FILTER (WHERE l.lesson_date = sqlc.arg(today)::date AND l.status <> 'cancelled')::int AS today_lessons,
       count(*) FILTER (WHERE l.lesson_date < sqlc.arg(today)::date AND l.status = 'planned')::int AS unmarked_past_lessons
FROM lessons l WHERE l.teacher_id = sqlc.arg(teacher_id) AND l.academic_period_id = sqlc.arg(period_id);

-- name: TeacherHomeworkCounts :one
SELECT count(*) FILTER (WHERE h.status = 'draft')::int AS drafts,
       count(*) FILTER (WHERE h.status = 'published' AND h.due_date >= sqlc.arg(today)::date)::int AS active
FROM homework h WHERE h.author_id = sqlc.arg(user_id);

-- name: SchoolCounts :one
SELECT (SELECT count(*) FROM students st JOIN users u ON u.id = st.user_id WHERE u.is_active)::int AS students,
       (SELECT count(*) FROM teachers t JOIN users u ON u.id = t.user_id WHERE u.is_active)::int AS teachers,
       (SELECT count(*) FROM parents p JOIN users u ON u.id = p.user_id WHERE u.is_active)::int AS parents,
       (SELECT count(*) FROM classes WHERE archived_at IS NULL)::int AS classes,
       (SELECT count(*) FROM subjects WHERE archived_at IS NULL)::int AS subjects,
       (SELECT count(*) FROM users WHERE is_active)::int AS users_active,
       (SELECT count(*) FROM users WHERE NOT is_active)::int AS users_inactive,
       (SELECT count(*) FROM users WHERE is_active AND must_change_password)::int AS pending_password_change,
       (SELECT count(*) FROM sessions WHERE revoked_at IS NULL AND expires_at > now())::int AS active_sessions,
       (SELECT count(*) FROM invitations WHERE used_at IS NULL AND revoked_at IS NULL AND expires_at > now())::int AS open_invitations;

-- name: UsersByRole :many
SELECT r.code AS role, count(*)::int AS users_count
FROM user_roles ur JOIN roles r ON r.id = ur.role_id JOIN users u ON u.id = ur.user_id
WHERE u.is_active GROUP BY r.code ORDER BY r.code;

-- name: SchoolAttendanceByClass :many
SELECT c.id AS class_id, c.name AS class_name,
       (SELECT count(*) FROM enrollments e WHERE e.class_id = c.id AND e.starts_on <= sqlc.arg(on_date)::date
         AND (e.ends_on IS NULL OR e.ends_on >= sqlc.arg(on_date)::date))::int AS students,
       count(a.id) FILTER (WHERE a.status IN ('present','late'))::int AS present,
       count(a.id) FILTER (WHERE a.status IN ('absent','excused'))::int AS absent
FROM classes c
LEFT JOIN lessons l ON l.class_id = c.id AND l.lesson_date = sqlc.arg(on_date)::date AND l.status <> 'cancelled'
LEFT JOIN attendance a ON a.lesson_id = l.id
WHERE c.archived_at IS NULL
GROUP BY c.id, c.name ORDER BY c.name;

-- name: SchoolGradeAverageByClass :many
SELECT c.id AS class_id, c.name AS class_name, COALESCE(sum(g.value * lw.weight) / NULLIF(sum(lw.weight), 0), -1)::float8 AS average,
       count(*)::int AS grades_count
FROM grades g JOIN lesson_works lw ON lw.id = g.lesson_work_id JOIN lessons l ON l.id = lw.lesson_id
JOIN classes c ON c.id = l.class_id
WHERE g.academic_period_id = $1 AND g.deleted_at IS NULL
GROUP BY c.id, c.name ORDER BY c.name;

-- name: SchoolGradeAverageBySubject :many
SELECT s.id AS subject_id, s.name AS subject_name, COALESCE(sum(g.value * lw.weight) / NULLIF(sum(lw.weight), 0), -1)::float8 AS average,
       count(*)::int AS grades_count
FROM grades g JOIN lesson_works lw ON lw.id = g.lesson_work_id JOIN lessons l ON l.id = lw.lesson_id
JOIN subjects s ON s.id = l.subject_id
WHERE g.academic_period_id = $1 AND g.deleted_at IS NULL
GROUP BY s.id, s.name ORDER BY s.name;

-- name: SchoolGradeDistribution :many
SELECT g.value::float8 AS value, count(*)::int AS grades_count
FROM grades g WHERE g.academic_period_id = $1 AND g.deleted_at IS NULL
GROUP BY g.value ORDER BY g.value;

-- name: SchoolAttendanceTrend :many
SELECT l.lesson_date AS day,
       COALESCE(count(a.id) FILTER (WHERE a.status IN ('present','late'))::float8 / NULLIF(count(a.id) FILTER (WHERE a.status <> 'exempt'), 0), -1)::float8 AS rate
FROM lessons l JOIN attendance a ON a.lesson_id = l.id
WHERE l.lesson_date BETWEEN sqlc.arg(from_date)::date AND sqlc.arg(to_date)::date
GROUP BY l.lesson_date ORDER BY l.lesson_date;

-- name: SchoolAlertCounts :one
SELECT (SELECT count(*) FROM students st JOIN users u ON u.id = st.user_id WHERE u.is_active AND NOT EXISTS (
          SELECT 1 FROM enrollments e WHERE e.student_id = st.id AND e.starts_on <= current_date AND (e.ends_on IS NULL OR e.ends_on >= current_date)))::int AS unenrolled_students,
       (SELECT count(*) FROM classes c WHERE c.archived_at IS NULL AND NOT EXISTS (
          SELECT 1 FROM class_teachers ct WHERE ct.class_id = c.id AND ct.starts_on <= current_date AND (ct.ends_on IS NULL OR ct.ends_on >= current_date)))::int AS classes_without_class_teacher,
       (SELECT count(*) FROM users u WHERE u.is_active AND u.created_at < now() - interval '7 days'
          AND u.last_login_at IS NULL)::int AS never_signed_in,
       (SELECT count(*) FROM parents p JOIN users u ON u.id = p.user_id WHERE u.is_active AND NOT EXISTS (
          SELECT 1 FROM parent_student ps WHERE ps.parent_id = p.id AND ps.valid_from <= current_date AND (ps.valid_to IS NULL OR ps.valid_to >= current_date)))::int AS parents_without_children,
       (SELECT count(*) FROM teachers t JOIN users u ON u.id = t.user_id WHERE u.is_active AND NOT EXISTS (
          SELECT 1 FROM teaching_assignments ta WHERE ta.teacher_id = t.id AND ta.starts_on <= current_date AND (ta.ends_on IS NULL OR ta.ends_on >= current_date)))::int AS teachers_without_assignments,
       (SELECT count(*) FROM lessons l WHERE l.lesson_date < current_date AND l.status = 'planned')::int AS unmarked_past_lessons;

-- name: RecentAuditWithActor :many
SELECT a.id, a.action, a.entity_type, a.entity_id, a.created_at, u.display_name AS actor_name
FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id
ORDER BY a.created_at DESC LIMIT $1;
