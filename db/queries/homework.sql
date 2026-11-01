-- name: CreateHomework :one
INSERT INTO homework (class_id, subject_id, lesson_id, author_id, text, due_date, status, published_at)
VALUES ($1,$2,$3,$4,$5,$6,$7, CASE WHEN $7 = 'published' THEN now() ELSE NULL END)
RETURNING id, class_id, subject_id, lesson_id, text, due_date, status, published_at, version;

-- name: GetHomework :one
SELECT id, class_id, subject_id, lesson_id, author_id, text, due_date, status, published_at, version
FROM homework WHERE id = $1;

-- name: UpdateHomework :one
UPDATE homework SET text = $2, due_date = $3, version = version + 1, updated_at = now()
WHERE id = $1 AND version = $4 RETURNING id, text, due_date, status, version;

-- name: PublishHomework :one
UPDATE homework SET status = 'published', published_at = now(), version = version + 1, updated_at = now()
WHERE id = $1 AND version = $2 AND status = 'draft' RETURNING id, status, published_at, version;

-- name: AddHomeworkRecipient :exec
INSERT INTO homework_recipients (homework_id, student_id) VALUES ($1,$2) ON CONFLICT DO NOTHING;

-- name: ListStudentHomework :many
SELECT h.id, h.class_id, h.subject_id, s.name AS subject_name, h.lesson_id, h.text, h.due_date,
       h.status, h.version, COALESCE(hc.completed_at IS NOT NULL, false) AS completed
FROM homework h JOIN subjects s ON s.id = h.subject_id
JOIN enrollments e ON e.class_id = h.class_id AND e.student_id = $1
 AND e.starts_on <= h.due_date AND (e.ends_on IS NULL OR e.ends_on >= h.due_date)
LEFT JOIN homework_recipients hr ON hr.homework_id = h.id
LEFT JOIN homework_completions hc ON hc.homework_id = h.id AND hc.student_id = $1
WHERE h.status = 'published' AND (hr.student_id IS NULL OR hr.student_id = $1)
ORDER BY h.due_date, s.name;

-- name: ListTeacherHomework :many
SELECT h.id, h.class_id, c.name AS class_name, h.subject_id, s.name AS subject_name,
       h.lesson_id, h.text, h.due_date, h.status, h.version
FROM homework h JOIN classes c ON c.id = h.class_id JOIN subjects s ON s.id = h.subject_id
WHERE h.author_id = $1 ORDER BY h.due_date DESC;

-- name: SetHomeworkCompletion :one
INSERT INTO homework_completions (homework_id, student_id, completed_at)
VALUES ($1,$2, CASE WHEN $3 THEN now() ELSE NULL END)
ON CONFLICT (homework_id, student_id) DO UPDATE SET completed_at = EXCLUDED.completed_at, updated_at = now()
RETURNING homework_id, student_id, completed_at;

-- name: CreateFile :one
INSERT INTO files (storage_key, original_name, size_bytes, mime_type, owner_id)
VALUES ($1,$2,$3,$4,$5) RETURNING id, storage_key, original_name, size_bytes, mime_type, created_at;

-- name: GetFile :one
SELECT id, storage_key, original_name, size_bytes, mime_type, owner_id, created_at, deleted_at
FROM files WHERE id = $1 AND deleted_at IS NULL;

-- name: AttachHomeworkFile :exec
INSERT INTO homework_files (homework_id, file_id) VALUES ($1,$2);

-- name: FileVisibleToStudent :one
SELECT EXISTS (SELECT 1 FROM files f LEFT JOIN homework_files hf ON hf.file_id = f.id
 LEFT JOIN homework h ON h.id = hf.homework_id
 LEFT JOIN enrollments e ON e.class_id = h.class_id AND e.student_id = $2
 LEFT JOIN homework_recipients hr ON hr.homework_id = h.id
 WHERE f.id = $1 AND f.deleted_at IS NULL AND h.status = 'published'
 AND e.student_id IS NOT NULL AND e.starts_on <= h.due_date AND (e.ends_on IS NULL OR e.ends_on >= h.due_date)
 AND (hr.student_id IS NULL OR hr.student_id = $2)) AS allowed;

-- name: ListHomeworkFiles :many
SELECT f.id, f.original_name, f.size_bytes, f.mime_type FROM files f
JOIN homework_files hf ON hf.file_id = f.id WHERE hf.homework_id = $1 AND f.deleted_at IS NULL;
