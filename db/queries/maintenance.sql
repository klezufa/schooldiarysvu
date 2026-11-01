-- name: UpdateAcademicYear :one
UPDATE academic_years SET name = $2, starts_on = $3, ends_on = $4, is_current = $5, updated_at = now()
WHERE id = $1 AND archived_at IS NULL RETURNING id, name, starts_on, ends_on, is_current;

-- name: ClearCurrentAcademicYear :exec
UPDATE academic_years SET is_current = false, updated_at = now() WHERE is_current AND academic_years.id <> $1;

-- name: ArchiveAcademicYear :execrows
UPDATE academic_years SET archived_at = now(), is_current = false, updated_at = now()
WHERE academic_years.id = $1 AND academic_years.archived_at IS NULL AND NOT EXISTS (SELECT 1 FROM classes c WHERE c.academic_year_id = $1 AND c.archived_at IS NULL);

-- name: UpdateAcademicPeriod :one
UPDATE academic_periods SET name = $2, starts_on = $3, ends_on = $4, updated_at = now()
WHERE id = $1 RETURNING id, academic_year_id, name, starts_on, ends_on, status, allow_late_edits;

-- name: UpdateClass :one
UPDATE classes SET name = $2, updated_at = now() WHERE id = $1 AND archived_at IS NULL RETURNING id, academic_year_id, name;

-- name: ArchiveClass :execrows
UPDATE classes SET archived_at = now(), updated_at = now() WHERE id = $1 AND archived_at IS NULL;

-- name: UpdateSubject :one
UPDATE subjects SET name = $2, short_name = $3, updated_at = now() WHERE id = $1 AND archived_at IS NULL RETURNING id, name, short_name;

-- name: ArchiveSubject :execrows
UPDATE subjects SET archived_at = now(), updated_at = now() WHERE id = $1 AND archived_at IS NULL;

-- name: UpdateRoom :one
UPDATE rooms SET name = $2, updated_at = now() WHERE id = $1 AND archived_at IS NULL RETURNING id, name;

-- name: ArchiveRoom :execrows
UPDATE rooms SET archived_at = now(), updated_at = now() WHERE id = $1 AND archived_at IS NULL;

-- name: GetEnrollment :one
SELECT id, student_id, class_id, starts_on, ends_on FROM enrollments WHERE id = $1;

-- name: EnrollmentOverlapsExcept :one
SELECT EXISTS (SELECT 1 FROM enrollments WHERE id <> $1 AND student_id = $2
 AND starts_on <= $4::date AND (ends_on IS NULL OR ends_on >= $3::date)) AS overlaps;

-- name: UpdateEnrollmentEnd :one
UPDATE enrollments SET ends_on = $2 WHERE id = $1 RETURNING id, student_id, class_id, starts_on, ends_on;

-- name: GetTeachingAssignment :one
SELECT id, teacher_id, class_id, subject_id, academic_period_id, starts_on, ends_on FROM teaching_assignments WHERE id = $1;

-- name: TeachingAssignmentOverlapsExcept :one
SELECT EXISTS (SELECT 1 FROM teaching_assignments WHERE id <> $1 AND class_id = $2 AND subject_id = $3
 AND academic_period_id = $4 AND starts_on <= $6::date AND (ends_on IS NULL OR ends_on >= $5::date)) AS overlaps;

-- name: UpdateTeachingAssignmentEnd :one
UPDATE teaching_assignments SET ends_on = $2 WHERE id = $1
RETURNING id, teacher_id, class_id, subject_id, academic_period_id, starts_on, ends_on;

-- name: GetClassTeacher :one
SELECT id, teacher_id, class_id, starts_on, ends_on FROM class_teachers WHERE id = $1;

-- name: ClassTeacherOverlapsExcept :one
SELECT EXISTS (SELECT 1 FROM class_teachers WHERE id <> $1 AND class_id = $2
 AND starts_on <= $4::date AND (ends_on IS NULL OR ends_on >= $3::date)) AS overlaps;

-- name: UpdateClassTeacherEnd :execrows
UPDATE class_teachers SET ends_on = $2 WHERE id = $1;

-- name: UpdateParentStudentEnd :execrows
UPDATE parent_student SET valid_to = $4 WHERE parent_id = $1 AND student_id = $2 AND valid_from = $3::date;

-- name: GetSchoolSettings :one
SELECT school_name, timezone FROM school_settings WHERE singleton;

-- name: ListGradeScales :many
SELECT id, name, allowed_values, is_active FROM grade_scales ORDER BY name;

-- name: CreateGradeScale :one
INSERT INTO grade_scales (name, allowed_values, is_active) VALUES ($1,$2,$3) RETURNING id, name, allowed_values, is_active;

-- name: UpdateGradeScale :one
UPDATE grade_scales SET name = $2, allowed_values = $3, is_active = $4 WHERE id = $1 RETURNING id, name, allowed_values, is_active;

-- name: ClearActiveGradeScales :exec
UPDATE grade_scales SET is_active = false WHERE id <> $1;

-- name: ScheduleEntryConflict :one
SELECT EXISTS (SELECT 1 FROM schedule_entries WHERE academic_period_id = $1 AND weekday = $2
 AND starts_at < $4::time AND ends_at > $3::time AND valid_from <= $6::date AND (valid_to IS NULL OR valid_to >= $5::date)
 AND (week_parity = 0 OR $7 = 0 OR week_parity = $7)
 AND (class_id = $8 OR teacher_id = $9 OR ($10::uuid IS NOT NULL AND room_id = $10))) AS conflicted;

-- name: ListTeacherScheduleEntries :many
SELECT se.id, se.class_id, c.name AS class_name, se.subject_id, s.name AS subject_name, se.teacher_id,
       se.room_id, r.name AS room_name, se.academic_period_id, se.weekday, se.lesson_number,
       se.starts_at, se.ends_at, se.valid_from, se.valid_to, se.week_parity
FROM schedule_entries se JOIN classes c ON c.id = se.class_id JOIN subjects s ON s.id = se.subject_id
LEFT JOIN rooms r ON r.id = se.room_id
WHERE se.teacher_id = $1 AND se.valid_from <= $2::date AND (se.valid_to IS NULL OR se.valid_to >= $2::date)
ORDER BY se.weekday, se.lesson_number;

-- name: ListStudentScheduleEntries :many
SELECT se.id, se.class_id, c.name AS class_name, se.subject_id, s.name AS subject_name, se.teacher_id,
       se.room_id, r.name AS room_name, se.academic_period_id, se.weekday, se.lesson_number,
       se.starts_at, se.ends_at, se.valid_from, se.valid_to, se.week_parity
FROM schedule_entries se JOIN classes c ON c.id = se.class_id JOIN subjects s ON s.id = se.subject_id
LEFT JOIN rooms r ON r.id = se.room_id
JOIN enrollments e ON e.class_id = se.class_id AND e.student_id = $1
 AND e.starts_on <= $2::date AND (e.ends_on IS NULL OR e.ends_on >= $2::date)
WHERE se.valid_from <= $2::date AND (se.valid_to IS NULL OR se.valid_to >= $2::date)
ORDER BY se.weekday, se.lesson_number;

-- name: RescheduleLesson :one
UPDATE lessons SET lesson_date = $2, starts_at = $3, ends_at = $4, teacher_id = $5, room_id = $6,
 topic = $7, status = $8, cancellation_reason = $9, updated_by = $10, version = version + 1, updated_at = now()
WHERE id = $1 AND version = $11 RETURNING id, version;

-- name: ArchiveFile :one
UPDATE files SET deleted_at = now() WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL RETURNING id;

-- name: ArchiveAnnouncement :one
UPDATE announcements SET archived_at = now() WHERE id = $1 AND author_id = $2 AND archived_at IS NULL RETURNING id;

-- name: ListDigestNotifications :many
SELECT id, kind, title, body, entity_type, entity_id, read_at, created_at FROM notifications
WHERE user_id = $1 AND created_at > $2::timestamptz AND created_at <= $3::timestamptz AND kind <> 'digest'
ORDER BY created_at DESC LIMIT 200;

-- name: SubjectReport :many
SELECT s.id AS student_id, u.display_name, count(g.id)::integer AS grade_count,
 COALESCE(avg(g.value),0)::float8 AS average,
 COALESCE(sum(g.value * lw.weight) / NULLIF(sum(lw.weight), 0), 0)::float8 AS weighted_average
FROM enrollments e JOIN students s ON s.id = e.student_id JOIN users u ON u.id = s.user_id
JOIN academic_periods ap ON ap.id = $3 AND e.starts_on <= ap.ends_on AND (e.ends_on IS NULL OR e.ends_on >= ap.starts_on)
LEFT JOIN lessons l ON l.class_id = e.class_id AND l.subject_id = $2 AND l.academic_period_id = $3
 AND l.lesson_date BETWEEN e.starts_on AND COALESCE(e.ends_on, ap.ends_on)
LEFT JOIN lesson_works lw ON lw.lesson_id = l.id
LEFT JOIN grades g ON g.lesson_work_id = lw.id AND g.student_id = s.id AND g.deleted_at IS NULL
WHERE e.class_id = $1 GROUP BY s.id, u.display_name ORDER BY u.display_name;

-- name: UpdateUserDisplayName :one
UPDATE users SET display_name = $2, updated_at = now() WHERE id = $1 AND is_active RETURNING id;

-- name: ListClassTeachers :many
SELECT id, teacher_id, class_id, starts_on, ends_on FROM class_teachers ORDER BY starts_on DESC;

-- name: ListParentStudentLinks :many
SELECT parent_id, student_id, valid_from, valid_to FROM parent_student ORDER BY valid_from DESC;

-- name: ListDigestRecipients :many
SELECT u.id, np.digest_frequency, np.quiet_from, np.quiet_to
FROM notification_preferences np JOIN users u ON u.id = np.user_id AND u.is_active
WHERE np.internal_enabled AND np.digest_frequency IN ('daily', 'weekly');
