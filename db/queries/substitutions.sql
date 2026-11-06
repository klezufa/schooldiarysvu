-- name: GetLessonSubstitution :one
SELECT lesson_id, original_teacher_id, reason FROM lesson_substitutions WHERE lesson_id = $1;

-- name: UpsertLessonSubstitution :exec
INSERT INTO lesson_substitutions (lesson_id, original_teacher_id, reason, created_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT (lesson_id) DO UPDATE SET reason = EXCLUDED.reason, updated_at = now();

-- name: DeleteLessonSubstitution :exec
DELETE FROM lesson_substitutions WHERE lesson_id = $1;

-- name: SetLessonTeacher :exec
UPDATE lessons SET teacher_id = $2, version = version + 1, updated_at = now(), updated_by = $3 WHERE id = $1;

-- name: GetActiveTeacher :one
-- Учитель, который может вести урок: учётная запись активна и роль учителя не снята.
SELECT t.id, t.user_id, u.display_name FROM teachers t JOIN users u ON u.id = t.user_id AND u.is_active
WHERE t.id = $1 AND EXISTS (SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = u.id AND r.code = 'teacher');

-- name: TeacherSubstitutesInJournal :one
-- Учитель заменял хотя бы на одном уроке этого журнала: ему нужен журнал, чтобы поставить оценки за свой урок.
SELECT EXISTS (SELECT 1 FROM lessons l JOIN lesson_substitutions ls ON ls.lesson_id = l.id
 WHERE l.teacher_id = $1 AND l.class_id = $2 AND l.subject_id = $3 AND l.academic_period_id = $4) AS allowed;

-- name: SubstitutionCandidates :many
-- Учителя, свободные во время урока, с признаками для выбора: ведёт этот предмет, работает с этим классом, сколько
-- уроков у него в этот день. Заменённый и нынешний учитель урока не предлагаются.
WITH target AS (
    SELECT l.id, l.lesson_date, l.starts_at, l.ends_at, l.class_id, l.subject_id, l.teacher_id,
           COALESCE(ls.original_teacher_id, l.teacher_id) AS original_teacher_id
    FROM lessons l LEFT JOIN lesson_substitutions ls ON ls.lesson_id = l.id WHERE l.id = $1
)
SELECT t.id AS teacher_id, u.display_name,
       EXISTS (SELECT 1 FROM teaching_assignments ta WHERE ta.teacher_id = t.id AND ta.subject_id = target.subject_id
               AND ta.starts_on <= target.lesson_date AND (ta.ends_on IS NULL OR ta.ends_on >= target.lesson_date)) AS teaches_subject,
       EXISTS (SELECT 1 FROM teaching_assignments ta WHERE ta.teacher_id = t.id AND ta.class_id = target.class_id
               AND ta.starts_on <= target.lesson_date AND (ta.ends_on IS NULL OR ta.ends_on >= target.lesson_date)) AS knows_class,
       (SELECT count(*) FROM lessons o WHERE o.teacher_id = t.id AND o.lesson_date = target.lesson_date AND o.status <> 'cancelled')::integer AS lessons_that_day
FROM target
CROSS JOIN teachers t
JOIN users u ON u.id = t.user_id AND u.is_active
WHERE t.id <> target.original_teacher_id AND t.id <> target.teacher_id
  AND EXISTS (SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = u.id AND r.code = 'teacher')
  AND NOT EXISTS (SELECT 1 FROM lessons o WHERE o.teacher_id = t.id AND o.id <> target.id AND o.status <> 'cancelled'
                  AND o.starts_at < target.ends_at AND o.ends_at > target.starts_at)
ORDER BY u.display_name;

-- name: ListSubstitutionLessons :many
-- Без учителя — все замены за период; с учителем — его уроки за период, и свои, и те, на которых его заменили.
SELECT l.id, l.lesson_date, l.starts_at, l.ends_at, l.status, c.name AS class_name, s.name AS subject_name,
       r.name AS room_name, l.teacher_id, tu.display_name AS teacher_name,
       ls.original_teacher_id, ou.display_name AS original_teacher_name, COALESCE(ls.reason, '')::text AS reason
FROM lessons l
JOIN classes c ON c.id = l.class_id
JOIN subjects s ON s.id = l.subject_id
LEFT JOIN rooms r ON r.id = l.room_id
JOIN teachers t ON t.id = l.teacher_id JOIN users tu ON tu.id = t.user_id
LEFT JOIN lesson_substitutions ls ON ls.lesson_id = l.id
LEFT JOIN teachers ot ON ot.id = ls.original_teacher_id LEFT JOIN users ou ON ou.id = ot.user_id
WHERE l.lesson_date BETWEEN sqlc.arg(from_date)::date AND sqlc.arg(to_date)::date
  AND CASE WHEN sqlc.narg(teacher_id)::uuid IS NULL THEN ls.lesson_id IS NOT NULL
           ELSE l.teacher_id = sqlc.narg(teacher_id)::uuid OR ls.original_teacher_id = sqlc.narg(teacher_id)::uuid END
ORDER BY l.starts_at, c.name
LIMIT 2000;

-- name: ClassNotificationRecipients :many
-- Ученики класса на дату и их родители, у которых включены уведомления.
SELECT DISTINCT recipient.id FROM enrollments e
JOIN students s ON s.id = e.student_id
LEFT JOIN users su ON su.id = s.user_id AND su.is_active
LEFT JOIN parent_student ps ON ps.student_id = s.id AND ps.valid_from <= sqlc.arg(on_date)::date AND (ps.valid_to IS NULL OR ps.valid_to >= sqlc.arg(on_date)::date)
LEFT JOIN parents p ON p.id = ps.parent_id LEFT JOIN users pu ON pu.id = p.user_id AND pu.is_active
JOIN users recipient ON recipient.id IN (su.id, pu.id)
LEFT JOIN notification_preferences np ON np.user_id = recipient.id
WHERE e.class_id = sqlc.arg(class_id) AND e.starts_on <= sqlc.arg(on_date)::date AND (e.ends_on IS NULL OR e.ends_on >= sqlc.arg(on_date)::date)
  AND COALESCE(np.internal_enabled, true);

-- name: UserNotificationsEnabled :one
SELECT COALESCE((SELECT internal_enabled FROM notification_preferences WHERE user_id = $1), true)::boolean AS enabled;

-- name: IsLessonSubstitute :one
-- Учитель сейчас ведёт этот урок как замена.
SELECT EXISTS (SELECT 1 FROM lesson_substitutions ls JOIN lessons l ON l.id = ls.lesson_id
 WHERE ls.lesson_id = $1 AND l.teacher_id = $2) AS substitute;

-- name: TeacherSubstitutesInClass :one
SELECT EXISTS (SELECT 1 FROM lessons l JOIN lesson_substitutions ls ON ls.lesson_id = l.id
 WHERE l.teacher_id = $1 AND l.class_id = $2) AS allowed;
