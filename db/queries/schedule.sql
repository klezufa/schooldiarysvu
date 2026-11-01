-- name: CreateScheduleEntry :one
INSERT INTO schedule_entries (class_id, subject_id, teacher_id, room_id, academic_period_id, weekday, lesson_number, starts_at, ends_at, valid_from, valid_to, week_parity)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
RETURNING id, class_id, subject_id, teacher_id, room_id, academic_period_id, weekday, lesson_number, starts_at, ends_at, valid_from, valid_to, week_parity;

-- name: CreateLesson :one
INSERT INTO lessons (lesson_date, starts_at, ends_at, class_id, subject_id, teacher_id, room_id, academic_period_id, schedule_entry_id, topic, created_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING id, lesson_date, starts_at, ends_at, class_id, subject_id, teacher_id, room_id, academic_period_id, topic, status, version;

-- name: GetLesson :one
SELECT id, lesson_date, starts_at, ends_at, class_id, subject_id, teacher_id, room_id, academic_period_id,
       schedule_entry_id, topic, status, cancellation_reason, version
FROM lessons WHERE id = $1;

-- name: UpdateLesson :one
UPDATE lessons SET topic = $2, room_id = $3, status = $4, cancellation_reason = $5,
       version = version + 1, updated_at = now(), updated_by = $6
WHERE id = $1 AND version = $7
RETURNING id, topic, room_id, status, cancellation_reason, version;

-- name: LessonTimeConflict :one
SELECT EXISTS (SELECT 1 FROM lessons WHERE id <> $1 AND status <> 'cancelled' AND starts_at < $3::timestamptz AND ends_at > $2::timestamptz
 AND (class_id = $4 OR teacher_id = $5 OR ($6::uuid IS NOT NULL AND room_id = $6))) AS conflicted;

-- name: ListTeacherLessons :many
SELECT l.id, l.lesson_date, l.starts_at, l.ends_at, l.class_id, c.name AS class_name,
       l.subject_id, s.name AS subject_name, l.topic, l.status, l.version, r.name AS room_name
FROM lessons l JOIN classes c ON c.id = l.class_id JOIN subjects s ON s.id = l.subject_id
LEFT JOIN rooms r ON r.id = l.room_id
WHERE l.teacher_id = $1 AND l.lesson_date BETWEEN $2::date AND $3::date ORDER BY l.starts_at;

-- name: ListStudentLessons :many
SELECT l.id, l.lesson_date, l.starts_at, l.ends_at, l.class_id, c.name AS class_name,
       l.subject_id, s.name AS subject_name, l.topic, l.status, l.version, r.name AS room_name
FROM lessons l JOIN classes c ON c.id = l.class_id JOIN subjects s ON s.id = l.subject_id
LEFT JOIN rooms r ON r.id = l.room_id
JOIN enrollments e ON e.class_id = l.class_id AND e.student_id = $1
 AND e.starts_on <= l.lesson_date AND (e.ends_on IS NULL OR e.ends_on >= l.lesson_date)
WHERE l.lesson_date BETWEEN $2::date AND $3::date ORDER BY l.starts_at;

-- name: ListTeacherWeekSchedule :many
SELECT se.id, se.weekday, se.lesson_number, se.starts_at, se.ends_at, c.name AS class_name,
       s.name AS subject_name, r.name AS room_name
FROM schedule_entries se JOIN classes c ON c.id = se.class_id JOIN subjects s ON s.id = se.subject_id
LEFT JOIN rooms r ON r.id = se.room_id
WHERE se.teacher_id = $1 AND se.valid_from <= $2::date AND (se.valid_to IS NULL OR se.valid_to >= $2::date)
ORDER BY se.weekday, se.lesson_number;

-- name: ListStudentWeekSchedule :many
SELECT se.id, se.weekday, se.lesson_number, se.starts_at, se.ends_at, c.name AS class_name,
       s.name AS subject_name, r.name AS room_name
FROM schedule_entries se JOIN classes c ON c.id = se.class_id JOIN subjects s ON s.id = se.subject_id
LEFT JOIN rooms r ON r.id = se.room_id
JOIN enrollments e ON e.class_id = se.class_id AND e.student_id = $1
 AND e.starts_on <= $2::date AND (e.ends_on IS NULL OR e.ends_on >= $2::date)
WHERE se.valid_from <= $2::date AND (se.valid_to IS NULL OR se.valid_to >= $2::date)
ORDER BY se.weekday, se.lesson_number;
