-- name: ListScheduleEntriesAdmin :many
SELECT se.id, se.class_id, c.name AS class_name, se.subject_id, s.name AS subject_name, se.teacher_id, tu.display_name AS teacher_name,
       se.room_id, r.name AS room_name, se.academic_period_id, se.weekday, se.lesson_number,
       se.starts_at, se.ends_at, se.valid_from, se.valid_to, se.week_parity
FROM schedule_entries se
JOIN classes c ON c.id = se.class_id JOIN subjects s ON s.id = se.subject_id
JOIN teachers t ON t.id = se.teacher_id JOIN users tu ON tu.id = t.user_id
LEFT JOIN rooms r ON r.id = se.room_id
WHERE (sqlc.narg(class_id)::uuid IS NULL OR se.class_id = sqlc.narg(class_id))
  AND (sqlc.narg(teacher_id)::uuid IS NULL OR se.teacher_id = sqlc.narg(teacher_id))
  AND (sqlc.narg(period_id)::uuid IS NULL OR se.academic_period_id = sqlc.narg(period_id))
  AND (sqlc.arg(on_date)::date = '0001-01-01' OR (se.valid_from <= sqlc.arg(on_date)::date AND (se.valid_to IS NULL OR se.valid_to >= sqlc.arg(on_date)::date)))
ORDER BY se.weekday, se.lesson_number, c.name;

-- name: GetScheduleEntry :one
SELECT id, class_id, subject_id, teacher_id, room_id, academic_period_id, weekday, lesson_number,
       starts_at, ends_at, valid_from, valid_to, week_parity
FROM schedule_entries WHERE id = $1;

-- name: UpdateScheduleEntry :one
UPDATE schedule_entries SET teacher_id = $2, room_id = $3, weekday = $4, lesson_number = $5, starts_at = $6, ends_at = $7,
       valid_from = $8, valid_to = $9, week_parity = $10
WHERE id = $1
RETURNING id, class_id, subject_id, teacher_id, room_id, academic_period_id, weekday, lesson_number, starts_at, ends_at, valid_from, valid_to, week_parity;

-- name: DeleteScheduleEntry :exec
DELETE FROM schedule_entries WHERE id = $1;

-- name: ScheduleEntryConflictExcluding :one
SELECT EXISTS (SELECT 1 FROM schedule_entries WHERE id <> sqlc.arg(self_id) AND academic_period_id = sqlc.arg(period_id) AND weekday = sqlc.arg(weekday)
 AND starts_at < sqlc.arg(ends_at)::time AND ends_at > sqlc.arg(starts_at)::time
 AND valid_from <= COALESCE(sqlc.narg(valid_to)::date, '9999-12-31') AND (valid_to IS NULL OR valid_to >= sqlc.arg(valid_from)::date)
 AND (week_parity = 0 OR sqlc.arg(week_parity)::smallint = 0 OR week_parity = sqlc.arg(week_parity)::smallint)
 AND (class_id = sqlc.arg(class_id) OR teacher_id = sqlc.arg(teacher_id) OR (sqlc.narg(room_id)::uuid IS NOT NULL AND room_id = sqlc.narg(room_id)))) AS conflicted;

-- name: ListClassLessons :many
SELECT l.id, l.lesson_date, l.starts_at, l.ends_at, l.class_id, c.name AS class_name,
       l.subject_id, s.name AS subject_name, l.topic, l.status, l.version, r.name AS room_name
FROM lessons l JOIN classes c ON c.id = l.class_id JOIN subjects s ON s.id = l.subject_id
LEFT JOIN rooms r ON r.id = l.room_id
WHERE l.class_id = $1 AND l.lesson_date BETWEEN $2::date AND $3::date ORDER BY l.starts_at;

-- name: ListGeneratedLessonDates :many
SELECT schedule_entry_id, lesson_date FROM lessons
WHERE schedule_entry_id IS NOT NULL AND lesson_date BETWEEN sqlc.arg(from_date)::date AND sqlc.arg(to_date)::date;

-- name: DeleteUntouchedFutureLessons :many
-- Будущие уроки строки шаблона, в которых ещё ничего нет: без темы, работ, посещаемости и заданий. Их удаляют при
-- изменении или удалении строки, чтобы в расписании не оставались уроки по старому шаблону. Заполненные остаются.
DELETE FROM lessons l
WHERE l.schedule_entry_id = sqlc.arg(schedule_entry_id) AND l.lesson_date >= school_today()
  AND l.status = 'planned' AND l.topic = ''
  AND NOT EXISTS (SELECT 1 FROM lesson_works w WHERE w.lesson_id = l.id)
  AND NOT EXISTS (SELECT 1 FROM attendance a WHERE a.lesson_id = l.id)
  AND NOT EXISTS (SELECT 1 FROM homework h WHERE h.lesson_id = l.id)
RETURNING l.lesson_date;

-- name: ListCarryOverAssignments :many
-- Назначения, действующие на последний день периода: досрочно завершённые и назначения деактивированных учителей не переносятся.
SELECT ta.teacher_id, ta.class_id, ta.subject_id
FROM teaching_assignments ta
JOIN teachers t ON t.id = ta.teacher_id JOIN users u ON u.id = t.user_id
WHERE ta.academic_period_id = sqlc.arg(period_id)
  AND ta.starts_on <= sqlc.arg(on_date)::date AND (ta.ends_on IS NULL OR ta.ends_on >= sqlc.arg(on_date)::date)
  AND (sqlc.narg(class_id)::uuid IS NULL OR ta.class_id = sqlc.narg(class_id))
  AND u.is_active
ORDER BY ta.class_id, ta.subject_id;
