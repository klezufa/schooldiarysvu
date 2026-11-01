-- Нагрузочный набор (DEMO_DATASET=load) поверх базового: школа на 360 учеников, 360 родителей и 150 учителей.
-- 12 классов (6А–9В) по 30 учеников, 10 предметов, полная недельная сетка 6 уроков × 5 дней, уроки с начала года
-- до двух недель вперёд, оценки, посещаемость и домашние задания. Только для нагрузочных проверок на локальной машине.
-- Пароли — DEMO_PASSWORD без обязательной смены, чтобы сценарии k6 входили сразу.
-- Учитель пары «класс — предмет» свой (120 учителей ведут уроки, 12 из оставшихся — классные руководители), кабинет
-- у класса свой: так сетка не даёт пересечений по учителю, классу и кабинету. Идентификаторы детерминированы (md5) и
-- не зависят от часового пояса сессии: в хеш идут даты (date), а не timestamptz, чей текст зависит от TimeZone.

INSERT INTO users (id, login, password_hash, display_name, must_change_password)
SELECT md5('load-user:' || kind || n)::uuid, 'load-' || kind || lpad(n::text, 3, '0'), (SELECT password_hash FROM demo_credentials),
 CASE kind WHEN 'teacher' THEN 'Учитель ' WHEN 'student' THEN 'Ученик ' ELSE 'Родитель ' END || lpad(n::text, 3, '0'), false
FROM (SELECT 'teacher' AS kind, generate_series(1, 150) AS n
      UNION ALL SELECT 'student', generate_series(1, 360)
      UNION ALL SELECT 'parent', generate_series(1, 360)) AS people
ON CONFLICT (id) DO UPDATE SET password_hash = EXCLUDED.password_hash, must_change_password = false, is_active = true, deactivated_at = NULL;

INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id FROM users u
JOIN roles r ON r.code = CASE WHEN u.login LIKE 'load-teacher%' THEN 'teacher' WHEN u.login LIKE 'load-student%' THEN 'student' ELSE 'parent' END
WHERE u.login LIKE 'load-%'
ON CONFLICT DO NOTHING;

INSERT INTO teachers (id, user_id) SELECT md5('load-teacher:' || u.id)::uuid, u.id FROM users u WHERE u.login LIKE 'load-teacher%' ON CONFLICT DO NOTHING;
INSERT INTO students (id, user_id, student_code)
SELECT md5('load-student:' || u.id)::uuid, u.id, 'LOAD-' || right(u.login, 3) FROM users u WHERE u.login LIKE 'load-student%' ON CONFLICT DO NOTHING;
INSERT INTO parents (id, user_id) SELECT md5('load-parent:' || u.id)::uuid, u.id FROM users u WHERE u.login LIKE 'load-parent%' ON CONFLICT DO NOTHING;

CREATE TEMP TABLE load_people (n int PRIMARY KEY, teacher_id uuid, student_id uuid, parent_id uuid) ON COMMIT DROP;
INSERT INTO load_people
SELECT n,
 (SELECT t.id FROM teachers t JOIN users u ON u.id = t.user_id WHERE u.login = 'load-teacher' || lpad(n::text, 3, '0')),
 (SELECT s.id FROM students s JOIN users u ON u.id = s.user_id WHERE u.login = 'load-student' || lpad(n::text, 3, '0')),
 (SELECT p.id FROM parents p JOIN users u ON u.id = p.user_id WHERE u.login = 'load-parent' || lpad(n::text, 3, '0'))
FROM generate_series(1, 360) AS n;

-- Классы 6А–9В текущего демонстрационного года.
CREATE TEMP TABLE load_classes (idx int PRIMARY KEY, id uuid, name text) ON COMMIT DROP;
INSERT INTO load_classes
SELECT (g - 6) * 3 + l - 1, md5('load-class:' || g || l)::uuid, g || (ARRAY['А', 'Б', 'В'])[l]
FROM generate_series(6, 9) AS g CROSS JOIN generate_series(1, 3) AS l;
INSERT INTO classes (id, academic_year_id, name)
SELECT id, '00000000-0000-4000-8000-000000002026', name FROM load_classes ON CONFLICT DO NOTHING;

INSERT INTO enrollments (id, student_id, class_id, starts_on)
SELECT md5('load-enrollment:' || p.student_id)::uuid, p.student_id, c.id, '2026-09-01'
FROM load_people p JOIN load_classes c ON c.idx = (p.n - 1) / 30
ON CONFLICT DO NOTHING;
INSERT INTO parent_student (parent_id, student_id, valid_from)
SELECT parent_id, student_id, '2026-09-01' FROM load_people ON CONFLICT DO NOTHING;

-- Предметы: существующие по имени используются повторно.
CREATE TEMP TABLE load_subjects (idx int PRIMARY KEY, name text, id uuid) ON COMMIT DROP;
INSERT INTO load_subjects (idx, name)
SELECT ord - 1, name FROM unnest(ARRAY['Математика', 'Русский язык', 'Литература', 'История', 'Обществознание', 'География',
 'Биология', 'Физика', 'Английский язык', 'Физкультура']) WITH ORDINALITY AS item(name, ord);
INSERT INTO subjects (id, name)
SELECT md5('load-subject:' || ls.name)::uuid, ls.name FROM load_subjects ls
WHERE NOT EXISTS (SELECT 1 FROM subjects s WHERE s.name = ls.name)
ON CONFLICT DO NOTHING;
UPDATE load_subjects ls SET id = s.id FROM subjects s WHERE s.name = ls.name;

INSERT INTO rooms (id, name)
SELECT md5('load-room:' || idx)::uuid, 'Каб. ' || (301 + idx) FROM load_classes ON CONFLICT DO NOTHING;

-- Учитель пары «класс c — предмет s»: № s·12 + c + 1 (1–120); классные руководители — № 121–132.
CREATE TEMP TABLE load_pairs (class_idx int, subject_idx int, class_id uuid, subject_id uuid, teacher_id uuid, room_id uuid) ON COMMIT DROP;
INSERT INTO load_pairs
SELECT c.idx, s.idx, c.id, s.id, p.teacher_id, (SELECT r.id FROM rooms r WHERE r.name = 'Каб. ' || (301 + c.idx))
FROM load_classes c CROSS JOIN load_subjects s JOIN load_people p ON p.n = s.idx * 12 + c.idx + 1;

INSERT INTO teaching_assignments (id, teacher_id, class_id, subject_id, academic_period_id, starts_on)
SELECT md5('load-assignment:' || lp.class_id || lp.subject_id || p.id)::uuid, lp.teacher_id, lp.class_id, lp.subject_id, p.id, p.starts_on
FROM load_pairs lp JOIN academic_periods p ON p.academic_year_id = '00000000-0000-4000-8000-000000002026'
ON CONFLICT DO NOTHING;
INSERT INTO class_teachers (id, teacher_id, class_id, starts_on)
SELECT md5('load-class-teacher:' || c.id)::uuid, p.teacher_id, c.id, '2026-09-01'
FROM load_classes c JOIN load_people p ON p.n = 121 + c.idx
ON CONFLICT DO NOTHING;

-- Недельная сетка: 30 слотов, предмет слота k у класса c — (k + c) mod 10, каждый предмет трижды в неделю.
CREATE TEMP TABLE load_bells (lesson_number int PRIMARY KEY, starts_at time, ends_at time) ON COMMIT DROP;
INSERT INTO load_bells VALUES (1, '08:30', '09:15'), (2, '09:25', '10:10'), (3, '10:30', '11:15'), (4, '11:35', '12:20'),
 (5, '12:30', '13:15'), (6, '13:25', '14:10');
INSERT INTO schedule_entries (id, class_id, subject_id, teacher_id, room_id, academic_period_id, weekday, lesson_number, starts_at, ends_at, valid_from, valid_to)
SELECT md5('load-entry:' || lp.class_id || k || p.id)::uuid, lp.class_id, lp.subject_id, lp.teacher_id, lp.room_id, p.id,
 k / 6 + 1, k % 6 + 1, b.starts_at, b.ends_at, p.starts_on, p.ends_on
FROM generate_series(0, 29) AS k
JOIN load_pairs lp ON lp.subject_idx = (k + lp.class_idx) % 10
JOIN load_bells b ON b.lesson_number = k % 6 + 1
JOIN academic_periods p ON p.academic_year_id = '00000000-0000-4000-8000-000000002026'
ON CONFLICT DO NOTHING;

INSERT INTO lessons (id, lesson_date, starts_at, ends_at, class_id, subject_id, teacher_id, room_id, academic_period_id, schedule_entry_id, topic, status, created_by)
SELECT md5('load-lesson:' || se.id || d::date)::uuid, d::date,
 (d::date + se.starts_at) AT TIME ZONE 'Europe/Moscow', (d::date + se.ends_at) AT TIME ZONE 'Europe/Moscow',
 se.class_id, se.subject_id, se.teacher_id, se.room_id, se.academic_period_id, se.id,
 CASE WHEN d::date < current_date THEN 'Тема урока ' || (1 + (d::date - p.starts_on) / 7) ELSE '' END,
 CASE WHEN d::date < current_date THEN 'completed' ELSE 'planned' END,
 '00000000-0000-4000-8000-000000000001'
FROM schedule_entries se
JOIN load_classes c ON c.id = se.class_id
JOIN academic_periods p ON p.id = se.academic_period_id
CROSS JOIN LATERAL generate_series(p.starts_on, LEAST(p.ends_on, current_date + 13), interval '1 day') AS d
WHERE extract(isodow FROM d) = se.weekday AND d::date >= se.valid_from AND (se.valid_to IS NULL OR d::date <= se.valid_to)
  AND NOT EXISTS (SELECT 1 FROM lessons l WHERE l.schedule_entry_id = se.id AND l.lesson_date = d::date)
ON CONFLICT DO NOTHING;

CREATE TEMP TABLE load_lessons ON COMMIT DROP AS
SELECT l.* FROM lessons l JOIN load_classes c ON c.id = l.class_id;

INSERT INTO lesson_works (id, lesson_id, work_type, title, weight, is_important, position)
SELECT md5('load-work:' || l.id)::uuid, l.id, 'classwork', 'Работа на уроке', 1, false, 1
FROM load_lessons l WHERE l.status = 'completed'
ON CONFLICT DO NOTHING;
INSERT INTO lesson_works (id, lesson_id, work_type, title, weight, is_important, position)
SELECT md5('load-test:' || l.id)::uuid, l.id, 'test', 'Контрольная работа', 2, true, 2
FROM load_lessons l WHERE l.status = 'completed' AND extract(week FROM l.lesson_date)::int % 4 = 0 AND extract(isodow FROM l.lesson_date) = 4
ON CONFLICT DO NOTHING;

INSERT INTO grades (id, student_id, lesson_work_id, academic_period_id, value, created_by)
SELECT md5('load-grade:' || lw.id || e.student_id)::uuid, e.student_id, lw.id, l.academic_period_id,
 greatest(2, least(5, round(3.0 + (abs(hashtext(e.student_id::text)) % 20) / 10.0
   + ((abs(hashtext(lw.id::text || e.student_id::text)) % 25) - 12) / 10.0))),
 t.user_id
FROM lesson_works lw
JOIN load_lessons l ON l.id = lw.lesson_id
JOIN teachers t ON t.id = l.teacher_id
JOIN enrollments e ON e.class_id = l.class_id AND e.starts_on <= l.lesson_date AND (e.ends_on IS NULL OR e.ends_on >= l.lesson_date)
WHERE lw.work_type = 'test' OR abs(hashtext('pick' || lw.id::text || e.student_id::text)) % 100 < 30
ON CONFLICT DO NOTHING;

INSERT INTO attendance (id, lesson_id, student_id, status, created_by)
SELECT md5('load-attendance:' || l.id || e.student_id)::uuid, l.id, e.student_id,
 CASE WHEN abs(hashtext('att' || l.id::text || e.student_id::text)) % 100 < 4 THEN 'absent'
      WHEN abs(hashtext('att' || l.id::text || e.student_id::text)) % 100 < 7 THEN 'late' ELSE 'present' END,
 t.user_id
FROM load_lessons l
JOIN teachers t ON t.id = l.teacher_id
JOIN enrollments e ON e.class_id = l.class_id AND e.starts_on <= l.lesson_date AND (e.ends_on IS NULL OR e.ends_on >= l.lesson_date)
WHERE l.status = 'completed'
ON CONFLICT DO NOTHING;

INSERT INTO homework (id, class_id, subject_id, lesson_id, author_id, text, due_date, status, published_at)
SELECT md5('load-homework:' || l.id)::uuid, l.class_id, l.subject_id, l.id, t.user_id,
 'Задание к уроку «' || COALESCE(NULLIF(l.topic, ''), 'по расписанию') || '»', l.lesson_date + 2, 'published',
 LEAST((l.lesson_date + time '15:00') AT TIME ZONE 'Europe/Moscow', now())
FROM load_lessons l JOIN teachers t ON t.id = l.teacher_id
WHERE l.lesson_date <= current_date + 3 AND abs(hashtext('hw' || l.id::text)) % 100 < 40
ON CONFLICT DO NOTHING;
