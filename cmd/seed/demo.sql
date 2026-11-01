UPDATE school_settings SET school_name = 'Демонстрационная школа', updated_at = now();

INSERT INTO users (id, login, password_hash, display_name, must_change_password)
SELECT id, login, (SELECT password_hash FROM demo_credentials), display_name, true FROM (VALUES
 ('00000000-0000-4000-8000-000000000001'::uuid, 'demo-admin', 'Демо Администратор'),
 ('00000000-0000-4000-8000-000000000004', 'demo-sysadmin', 'Демо Системный администратор'),
 ('00000000-0000-4000-8000-000000000002', 'demo-teacher1', 'Демо Учитель 1'),
 ('00000000-0000-4000-8000-000000000003', 'demo-teacher2', 'Демо Учитель 2'),
 ('00000000-0000-4000-8000-000000000011', 'demo-student1', 'Демо Ученик 1'),
 ('00000000-0000-4000-8000-000000000012', 'demo-student2', 'Демо Ученик 2'),
 ('00000000-0000-4000-8000-000000000013', 'demo-student3', 'Демо Ученик 3'),
 ('00000000-0000-4000-8000-000000000014', 'demo-student4', 'Демо Ученик 4'),
 ('00000000-0000-4000-8000-000000000015', 'demo-student5', 'Демо Ученик 5'),
 ('00000000-0000-4000-8000-000000000021', 'demo-parent1', 'Демо Родитель 1'),
 ('00000000-0000-4000-8000-000000000022', 'demo-parent2', 'Демо Родитель 2')
) AS demo(id, login, display_name)
ON CONFLICT (id) DO UPDATE SET password_hash = EXCLUDED.password_hash,
 must_change_password = true, is_active = true, deactivated_at = NULL;

INSERT INTO user_roles (user_id, role_id)
SELECT demo.user_id, roles.id FROM roles JOIN (VALUES
 ('00000000-0000-4000-8000-000000000001'::uuid, 'administrator'),
 ('00000000-0000-4000-8000-000000000004', 'system_admin'),
 ('00000000-0000-4000-8000-000000000002', 'teacher'),
 ('00000000-0000-4000-8000-000000000002', 'class_teacher'),
 ('00000000-0000-4000-8000-000000000003', 'teacher'),
 ('00000000-0000-4000-8000-000000000011', 'student'),
 ('00000000-0000-4000-8000-000000000012', 'student'),
 ('00000000-0000-4000-8000-000000000013', 'student'),
 ('00000000-0000-4000-8000-000000000014', 'student'),
 ('00000000-0000-4000-8000-000000000015', 'student'),
 ('00000000-0000-4000-8000-000000000021', 'parent'),
 ('00000000-0000-4000-8000-000000000022', 'parent')
) AS demo(user_id, role_code) ON roles.code = demo.role_code
ON CONFLICT DO NOTHING;

INSERT INTO teachers (id, user_id) VALUES
 ('00000000-0000-4000-8000-000000001002', '00000000-0000-4000-8000-000000000002'),
 ('00000000-0000-4000-8000-000000001003', '00000000-0000-4000-8000-000000000003')
ON CONFLICT DO NOTHING;
INSERT INTO students (id, user_id, student_code)
SELECT ('00000000-0000-4000-8000-0000000010' || substr(id::text, 35))::uuid, id, 'DEMO-' || row_number FROM (
 SELECT id, row_number() OVER (ORDER BY login) FROM users WHERE login ~ '^demo-student[1-5]$'
) AS ordered
ON CONFLICT DO NOTHING;
INSERT INTO parents (id, user_id)
SELECT ('00000000-0000-4000-8000-0000000020' || substr(id::text, 35))::uuid, id FROM users WHERE login ~ '^demo-parent[1-2]$'
ON CONFLICT DO NOTHING;

INSERT INTO academic_years (id, name, starts_on, ends_on, is_current) VALUES
 ('00000000-0000-4000-8000-000000002026', '2026/2027', '2026-09-01', '2027-05-31', true)
ON CONFLICT DO NOTHING;
INSERT INTO academic_periods (id, academic_year_id, name, starts_on, ends_on) VALUES
 ('00000000-0000-4000-8000-000000003001', '00000000-0000-4000-8000-000000002026', 'I четверть', '2026-09-01', '2026-10-31'),
 ('00000000-0000-4000-8000-000000003002', '00000000-0000-4000-8000-000000002026', 'II четверть', '2026-11-01', '2026-12-31')
ON CONFLICT DO NOTHING;
INSERT INTO classes (id, academic_year_id, name) VALUES
 ('00000000-0000-4000-8000-000000004001', '00000000-0000-4000-8000-000000002026', '5А'),
 ('00000000-0000-4000-8000-000000004002', '00000000-0000-4000-8000-000000002026', '5Б')
ON CONFLICT DO NOTHING;
INSERT INTO subjects (id, name, short_name) VALUES
 ('00000000-0000-4000-8000-000000005001', 'Математика', 'Мат.'),
 ('00000000-0000-4000-8000-000000005002', 'Русский язык', 'Рус.'),
 ('00000000-0000-4000-8000-000000005003', 'Литература', 'Лит.')
ON CONFLICT DO NOTHING;
INSERT INTO rooms (id, name) VALUES
 ('00000000-0000-4000-8000-000000006001', '101'),
 ('00000000-0000-4000-8000-000000006002', '102')
ON CONFLICT DO NOTHING;

INSERT INTO enrollments (id, student_id, class_id, starts_on)
SELECT ('00000000-0000-4000-8000-0000000070' || substr(s.id::text, 35))::uuid, s.id,
 CASE WHEN u.login IN ('demo-student1', 'demo-student2', 'demo-student3')
 THEN '00000000-0000-4000-8000-000000004001' ELSE '00000000-0000-4000-8000-000000004002' END::uuid,
 '2026-09-01'
FROM students s JOIN users u ON u.id = s.user_id WHERE u.login ~ '^demo-student[1-5]$'
ON CONFLICT DO NOTHING;
INSERT INTO parent_student (parent_id, student_id, valid_from)
SELECT p.id, s.id, '2026-09-01' FROM parents p JOIN users pu ON pu.id = p.user_id
JOIN users su ON su.login = CASE pu.login
 WHEN 'demo-parent1' THEN 'demo-student1'
 WHEN 'demo-parent2' THEN 'demo-student3' END
JOIN students s ON s.user_id = su.id
ON CONFLICT DO NOTHING;
INSERT INTO parent_student (parent_id, student_id, valid_from)
SELECT p.id, s.id, '2026-09-01' FROM parents p JOIN users pu ON pu.id = p.user_id AND pu.login = 'demo-parent1'
JOIN students s ON s.id = (SELECT id FROM students WHERE user_id = '00000000-0000-4000-8000-000000000012')
ON CONFLICT DO NOTHING;

INSERT INTO teaching_assignments (id, teacher_id, class_id, subject_id, academic_period_id, starts_on) VALUES
 ('00000000-0000-4000-8000-000000008001', '00000000-0000-4000-8000-000000001002', '00000000-0000-4000-8000-000000004001', '00000000-0000-4000-8000-000000005001', '00000000-0000-4000-8000-000000003001', '2026-09-01'),
 ('00000000-0000-4000-8000-000000008002', '00000000-0000-4000-8000-000000001003', '00000000-0000-4000-8000-000000004001', '00000000-0000-4000-8000-000000005002', '00000000-0000-4000-8000-000000003001', '2026-09-01')
ON CONFLICT DO NOTHING;
INSERT INTO class_teachers (id, teacher_id, class_id, starts_on) VALUES
 ('00000000-0000-4000-8000-000000008101', '00000000-0000-4000-8000-000000001002', '00000000-0000-4000-8000-000000004001', '2026-09-01')
ON CONFLICT DO NOTHING;
INSERT INTO schedule_entries (id, class_id, subject_id, teacher_id, room_id, academic_period_id, weekday, lesson_number, starts_at, ends_at, valid_from) VALUES
 ('00000000-0000-4000-8000-000000009001', '00000000-0000-4000-8000-000000004001', '00000000-0000-4000-8000-000000005001', '00000000-0000-4000-8000-000000001002', '00000000-0000-4000-8000-000000006001', '00000000-0000-4000-8000-000000003001', 1, 1, '09:00', '09:45', '2026-09-01'),
 ('00000000-0000-4000-8000-000000009002', '00000000-0000-4000-8000-000000004001', '00000000-0000-4000-8000-000000005002', '00000000-0000-4000-8000-000000001003', '00000000-0000-4000-8000-000000006002', '00000000-0000-4000-8000-000000003001', 1, 2, '10:00', '10:45', '2026-09-01')
ON CONFLICT DO NOTHING;
INSERT INTO lessons (id, lesson_date, starts_at, ends_at, class_id, subject_id, teacher_id, room_id, academic_period_id, schedule_entry_id, topic, status, created_by) VALUES
 ('00000000-0000-4000-8000-000000010001', '2026-09-07', '2026-09-07 09:00:00+03', '2026-09-07 09:45:00+03', '00000000-0000-4000-8000-000000004001', '00000000-0000-4000-8000-000000005001', '00000000-0000-4000-8000-000000001002', '00000000-0000-4000-8000-000000006001', '00000000-0000-4000-8000-000000003001', '00000000-0000-4000-8000-000000009001', 'Демонстрационный урок математики', 'completed', '00000000-0000-4000-8000-000000000002'),
 ('00000000-0000-4000-8000-000000010002', '2026-09-07', '2026-09-07 10:00:00+03', '2026-09-07 10:45:00+03', '00000000-0000-4000-8000-000000004001', '00000000-0000-4000-8000-000000005002', '00000000-0000-4000-8000-000000001003', '00000000-0000-4000-8000-000000006002', '00000000-0000-4000-8000-000000003001', '00000000-0000-4000-8000-000000009002', 'Демонстрационный урок русского языка', 'planned', '00000000-0000-4000-8000-000000000003')
ON CONFLICT DO NOTHING;
INSERT INTO lesson_works (id, lesson_id, work_type, title, weight, is_important, position) VALUES
 ('00000000-0000-4000-8000-000000011001', '00000000-0000-4000-8000-000000010001', 'classwork', 'Работа в классе', 1, false, 1)
ON CONFLICT DO NOTHING;
INSERT INTO grades (id, student_id, lesson_work_id, academic_period_id, value, comment, created_by)
SELECT ('00000000-0000-4000-8000-0000000120' || substr(s.id::text, 35))::uuid, s.id,
 '00000000-0000-4000-8000-000000011001', '00000000-0000-4000-8000-000000003001',
 2 + row_number() OVER (ORDER BY u.login), 'Демонстрационная оценка', '00000000-0000-4000-8000-000000000002'
FROM students s JOIN users u ON u.id = s.user_id
WHERE u.login IN ('demo-student1', 'demo-student2', 'demo-student3')
ON CONFLICT DO NOTHING;
INSERT INTO attendance (id, lesson_id, student_id, status, created_by)
SELECT ('00000000-0000-4000-8000-0000000130' || substr(e.student_id::text, 35))::uuid,
 '00000000-0000-4000-8000-000000010001', e.student_id, 'present', '00000000-0000-4000-8000-000000000002'
FROM enrollments e WHERE e.class_id = '00000000-0000-4000-8000-000000004001'
ON CONFLICT DO NOTHING;

INSERT INTO homework (id, class_id, subject_id, lesson_id, author_id, text, due_date, status, published_at) VALUES
 ('00000000-0000-4000-8000-000000014001', '00000000-0000-4000-8000-000000004001', '00000000-0000-4000-8000-000000005001', '00000000-0000-4000-8000-000000010001', '00000000-0000-4000-8000-000000000002', 'Демонстрационное домашнее задание', '2026-09-08', 'published', now())
ON CONFLICT DO NOTHING;
INSERT INTO announcements (id, author_id, title, body) VALUES
 ('00000000-0000-4000-8000-000000015001', '00000000-0000-4000-8000-000000000001', 'Демонстрационное объявление', 'Данные предназначены только для локальной разработки.')
ON CONFLICT DO NOTHING;
INSERT INTO announcement_audiences (id, announcement_id, audience_type) VALUES
 ('00000000-0000-4000-8000-000000015101', '00000000-0000-4000-8000-000000015001', 'school')
ON CONFLICT DO NOTHING;
