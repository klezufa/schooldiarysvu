-- name: GetLessonView :one
SELECT l.id, l.lesson_date, l.starts_at, l.ends_at, l.class_id, c.name AS class_name,
       l.subject_id, s.name AS subject_name, l.teacher_id, l.room_id, r.name AS room_name,
       l.academic_period_id, l.topic, l.status, l.cancellation_reason, l.version, ou.display_name AS original_teacher_name
FROM lessons l JOIN classes c ON c.id = l.class_id JOIN subjects s ON s.id = l.subject_id
LEFT JOIN rooms r ON r.id = l.room_id
LEFT JOIN lesson_substitutions ls ON ls.lesson_id = l.id LEFT JOIN teachers ot ON ot.id = ls.original_teacher_id
LEFT JOIN users ou ON ou.id = ot.user_id WHERE l.id = $1;

-- name: ListLessonViews :many
-- Уроки расписания одним запросом (вместо GetLessonView на каждый урок), в порядке начала.
SELECT l.id, l.lesson_date, l.starts_at, l.ends_at, l.class_id, c.name AS class_name,
       l.subject_id, s.name AS subject_name, l.teacher_id, l.room_id, r.name AS room_name,
       l.academic_period_id, l.topic, l.status, l.cancellation_reason, l.version, ou.display_name AS original_teacher_name
FROM lessons l JOIN classes c ON c.id = l.class_id JOIN subjects s ON s.id = l.subject_id
LEFT JOIN rooms r ON r.id = l.room_id
LEFT JOIN lesson_substitutions ls ON ls.lesson_id = l.id LEFT JOIN teachers ot ON ot.id = ls.original_teacher_id
LEFT JOIN users ou ON ou.id = ot.user_id WHERE l.id = ANY($1::uuid[]) ORDER BY l.starts_at, l.id;

-- name: TeacherAssignmentExists :one
SELECT EXISTS (SELECT 1 FROM teaching_assignments ta
 WHERE ta.teacher_id = $1 AND ta.class_id = $2 AND ta.subject_id = $3 AND ta.academic_period_id = $4
 AND ta.starts_on <= $5::date AND (ta.ends_on IS NULL OR ta.ends_on >= $5::date)) AS allowed;

-- name: AcademicPeriodForDate :one
SELECT id, academic_year_id, name, starts_on, ends_on, status, allow_late_edits, closed_at, closed_by
FROM academic_periods WHERE starts_on <= $1::date AND ends_on >= $1::date ORDER BY starts_on LIMIT 1;

-- name: StudentHasHomework :one
SELECT EXISTS (SELECT 1 FROM homework h
 JOIN enrollments e ON e.class_id = h.class_id AND e.student_id = $2
 AND e.starts_on <= h.due_date AND (e.ends_on IS NULL OR e.ends_on >= h.due_date)
 LEFT JOIN homework_recipients hr ON hr.homework_id = h.id
 WHERE h.id = $1 AND h.status = 'published'
 AND (NOT EXISTS (SELECT 1 FROM homework_recipients all_hr WHERE all_hr.homework_id = h.id)
      OR EXISTS (SELECT 1 FROM homework_recipients own_hr WHERE own_hr.homework_id = h.id AND own_hr.student_id = $2))) AS allowed;

-- name: FileVisibleToTeacher :one
SELECT EXISTS (SELECT 1 FROM files f JOIN homework_files hf ON hf.file_id = f.id
 JOIN homework h ON h.id = hf.homework_id
 LEFT JOIN teachers t ON t.user_id = $2
 LEFT JOIN teaching_assignments ta ON ta.teacher_id = t.id AND ta.class_id = h.class_id AND ta.subject_id = h.subject_id
  AND ta.starts_on <= h.due_date AND (ta.ends_on IS NULL OR ta.ends_on >= h.due_date)
 WHERE f.id = $1 AND f.deleted_at IS NULL AND (h.author_id = $2 OR ta.id IS NOT NULL)) AS allowed;

-- name: StudentNotificationRecipients :many
SELECT DISTINCT u.id FROM students s JOIN users u ON u.id = s.user_id AND u.is_active
LEFT JOIN parent_student ps ON ps.student_id = s.id AND ps.valid_from <= $2::date AND (ps.valid_to IS NULL OR ps.valid_to >= $2::date)
LEFT JOIN parents p ON p.id = ps.parent_id LEFT JOIN users pu ON pu.id = p.user_id AND pu.is_active
LEFT JOIN notification_preferences np ON np.user_id = u.id
WHERE s.id = $1 AND COALESCE(np.internal_enabled, true)
UNION
SELECT DISTINCT pu.id FROM students s
JOIN parent_student ps ON ps.student_id = s.id AND ps.valid_from <= $2::date AND (ps.valid_to IS NULL OR ps.valid_to >= $2::date)
JOIN parents p ON p.id = ps.parent_id JOIN users pu ON pu.id = p.user_id AND pu.is_active
LEFT JOIN notification_preferences np ON np.user_id = pu.id
WHERE s.id = $1 AND COALESCE(np.internal_enabled, true);

-- name: HomeworkNotificationRecipients :many
SELECT DISTINCT recipient.id FROM homework h
JOIN enrollments e ON e.class_id = h.class_id AND e.starts_on <= h.due_date AND (e.ends_on IS NULL OR e.ends_on >= h.due_date)
JOIN students s ON s.id = e.student_id
LEFT JOIN users su ON su.id = s.user_id AND su.is_active
LEFT JOIN parent_student ps ON ps.student_id = s.id AND ps.valid_from <= h.due_date AND (ps.valid_to IS NULL OR ps.valid_to >= h.due_date)
LEFT JOIN parents p ON p.id = ps.parent_id LEFT JOIN users pu ON pu.id = p.user_id AND pu.is_active
JOIN users recipient ON recipient.id IN (su.id, pu.id)
LEFT JOIN notification_preferences np ON np.user_id = recipient.id
WHERE h.id = $1 AND COALESCE(np.internal_enabled, true)
AND (NOT EXISTS (SELECT 1 FROM homework_recipients all_hr WHERE all_hr.homework_id = h.id)
     OR EXISTS (SELECT 1 FROM homework_recipients own_hr WHERE own_hr.homework_id = h.id AND own_hr.student_id = s.id));

-- name: AnnouncementNotificationRecipients :many
SELECT DISTINCT recipient.id FROM announcements a
JOIN announcement_audiences aa ON aa.announcement_id = a.id
LEFT JOIN enrollments e ON e.class_id = aa.class_id AND e.starts_on <= school_today() AND (e.ends_on IS NULL OR e.ends_on >= school_today())
LEFT JOIN lessons l ON l.subject_id = aa.subject_id AND l.lesson_date <= school_today()
LEFT JOIN enrollments se ON se.class_id = l.class_id AND se.starts_on <= l.lesson_date AND (se.ends_on IS NULL OR se.ends_on >= l.lesson_date)
LEFT JOIN students s ON s.id = COALESCE(e.student_id, se.student_id)
LEFT JOIN users su ON su.id = s.user_id AND su.is_active
LEFT JOIN parent_student ps ON ps.student_id = s.id AND ps.valid_from <= school_today() AND (ps.valid_to IS NULL OR ps.valid_to >= school_today())
LEFT JOIN parents p ON p.id = ps.parent_id LEFT JOIN users pu ON pu.id = p.user_id AND pu.is_active
JOIN users recipient ON recipient.id IN (su.id, pu.id)
LEFT JOIN notification_preferences np ON np.user_id = recipient.id
WHERE a.id = $1 AND COALESCE(np.internal_enabled, true)
AND (aa.audience_type <> 'school' OR s.id IS NOT NULL);

-- name: ListTeacherAnnouncements :many
SELECT DISTINCT a.id, a.title, a.body, a.published_at FROM announcements a
LEFT JOIN announcement_audiences aa ON aa.announcement_id = a.id
LEFT JOIN teaching_assignments ta ON ta.teacher_id = $1 AND ta.starts_on <= school_today() AND (ta.ends_on IS NULL OR ta.ends_on >= school_today())
 AND (ta.class_id = aa.class_id OR ta.subject_id = aa.subject_id)
WHERE a.archived_at IS NULL AND (aa.audience_type = 'school' OR a.author_id = $2 OR ta.id IS NOT NULL)
ORDER BY a.published_at DESC LIMIT 50;

-- name: ListAllAnnouncements :many
SELECT id, title, body, published_at FROM announcements WHERE archived_at IS NULL ORDER BY published_at DESC LIMIT 50;

-- name: GetHomeworkView :one
SELECT h.id, h.class_id, c.name AS class_name, h.subject_id, s.name AS subject_name,
       h.lesson_id, h.text, h.due_date, h.status, h.published_at, h.version
FROM homework h JOIN classes c ON c.id = h.class_id JOIN subjects s ON s.id = h.subject_id WHERE h.id = $1;

-- name: StudentAttendanceReport :many
SELECT subj.id AS subject_id, subj.name AS subject_name, a.status, count(a.id)::integer AS count
FROM attendance a JOIN lessons l ON l.id = a.lesson_id JOIN subjects subj ON subj.id = l.subject_id
WHERE a.student_id = $1 AND l.academic_period_id = $2
GROUP BY subj.id, subj.name, a.status ORDER BY subj.name, a.status;

-- name: ClassAttendanceReport :many
SELECT s.id AS student_id, u.display_name, a.status, count(a.id)::integer AS count
FROM attendance a JOIN lessons l ON l.id = a.lesson_id
JOIN students s ON s.id = a.student_id JOIN users u ON u.id = s.user_id
WHERE l.class_id = $1 AND l.academic_period_id = $2
GROUP BY s.id, u.display_name, a.status ORDER BY u.display_name, a.status;

-- name: UnfilledLessonsReport :many
SELECT l.id, l.lesson_date, c.name AS class_name, subj.name AS subject_name,
       count(DISTINCT lw.id)::integer AS work_count, count(DISTINCT a.id)::integer AS attendance_count
FROM lessons l JOIN classes c ON c.id = l.class_id JOIN subjects subj ON subj.id = l.subject_id
LEFT JOIN lesson_works lw ON lw.lesson_id = l.id LEFT JOIN attendance a ON a.lesson_id = l.id
WHERE l.academic_period_id = $1 AND l.status = 'completed'
 AND ($2::uuid = '00000000-0000-0000-0000-000000000000' OR l.teacher_id = $2)
GROUP BY l.id, l.lesson_date, c.name, subj.name
HAVING count(DISTINCT lw.id) = 0 OR count(DISTINCT a.id) = 0
ORDER BY l.lesson_date LIMIT 500;

-- name: ListStudentGradesForSubject :many
SELECT g.id, subj.id AS subject_id, subj.name AS subject_name, l.lesson_date, lw.work_type,
       g.value, g.comment, g.version, lw.weight
FROM grades g JOIN lesson_works lw ON lw.id = g.lesson_work_id JOIN lessons l ON l.id = lw.lesson_id
JOIN subjects subj ON subj.id = l.subject_id
WHERE g.student_id = $1 AND l.subject_id = $2 AND l.academic_period_id = $3 AND g.deleted_at IS NULL
ORDER BY l.lesson_date, lw.position;

-- name: DeleteExpiredSessionsNow :exec
DELETE FROM sessions WHERE expires_at < now() - interval '30 days';

-- name: TeacherCanGradeTerm :one
SELECT EXISTS (SELECT 1 FROM teaching_assignments ta JOIN teachers t ON t.id = ta.teacher_id
 JOIN academic_periods ap ON ap.id = ta.academic_period_id
 JOIN enrollments e ON e.class_id = ta.class_id AND e.student_id = $2
 AND e.starts_on <= ap.ends_on AND (e.ends_on IS NULL OR e.ends_on >= ap.starts_on)
 AND ta.starts_on <= ap.ends_on AND (ta.ends_on IS NULL OR ta.ends_on >= ap.starts_on)
 WHERE t.user_id = $1 AND ta.subject_id = $3 AND ta.academic_period_id = $4) AS allowed;
