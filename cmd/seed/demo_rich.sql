-- Расширенный демонстрационный набор (DEMO_DATASET=rich): ещё 20 учеников и 3 учителя, 7 предметов,
-- полная недельная сетка двух классов, уроки с начала года до двух недель вперёд, оценки, посещаемость,
-- задания и итоговые отметки за I четверть. Только для локальной проверки интерфейса и графиков.
-- Все идентификаторы детерминированы (md5), «случайность» — хеши: повторный запуск ничего не дублирует.

INSERT INTO users (id, login, password_hash, display_name, must_change_password)
SELECT id, login, (SELECT password_hash FROM demo_credentials), display_name, true FROM (VALUES
 ('00000000-0000-4000-8000-000000000031'::uuid, 'demo-teacher3', 'Смирнова Ольга Викторовна'),
 ('00000000-0000-4000-8000-000000000032', 'demo-teacher4', 'Кузнецов Андрей Павлович'),
 ('00000000-0000-4000-8000-000000000033', 'demo-teacher5', 'Волкова Марина Игоревна')
) AS demo(id, login, display_name)
ON CONFLICT (id) DO UPDATE SET password_hash = EXCLUDED.password_hash, must_change_password = true, is_active = true, deactivated_at = NULL;

INSERT INTO users (id, login, password_hash, display_name, must_change_password)
SELECT ('00000000-0000-4000-8000-0000000003' || lpad(n::text, 2, '0'))::uuid, 'demo-student' || n,
 (SELECT password_hash FROM demo_credentials), name, true
FROM (VALUES
 (6, 'Алексеев Артём Сергеевич'), (7, 'Белова Анна Дмитриевна'), (8, 'Васильев Максим Олегович'), (9, 'Григорьева Софья Ильинична'),
 (10, 'Дмитриев Иван Андреевич'), (11, 'Егорова Полина Максимовна'), (12, 'Жуков Кирилл Романович'), (13, 'Захарова Алиса Павловна'),
 (14, 'Ильин Матвей Викторович'), (15, 'Королёва Варвара Ивановна'), (16, 'Лебедев Тимофей Егорович'), (17, 'Морозова Ева Алексеевна'),
 (18, 'Никитин Лев Денисович'), (19, 'Орлова Милана Сергеевна'), (20, 'Павлов Даниил Николаевич'), (21, 'Романова Вероника Олеговна'),
 (22, 'Соколов Марк Андреевич'), (23, 'Тихонова Злата Игоревна'), (24, 'Устинов Глеб Михайлович'), (25, 'Фёдорова Мария Кирилловна')
) AS demo(n, name)
ON CONFLICT (id) DO UPDATE SET password_hash = EXCLUDED.password_hash, must_change_password = true, is_active = true, deactivated_at = NULL;

INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id FROM users u JOIN roles r ON r.code = CASE WHEN u.login LIKE 'demo-teacher%' THEN 'teacher' ELSE 'student' END
WHERE u.login IN ('demo-teacher3', 'demo-teacher4', 'demo-teacher5') OR u.id::text LIKE '00000000-0000-4000-8000-0000000003%'
ON CONFLICT DO NOTHING;
INSERT INTO teachers (id, user_id) VALUES
 ('00000000-0000-4000-8000-000000001031', '00000000-0000-4000-8000-000000000031'),
 ('00000000-0000-4000-8000-000000001032', '00000000-0000-4000-8000-000000000032'),
 ('00000000-0000-4000-8000-000000001033', '00000000-0000-4000-8000-000000000033')
ON CONFLICT DO NOTHING;
INSERT INTO students (id, user_id, student_code)
SELECT ('00000000-0000-4000-8000-0000000030' || right(u.id::text, 2))::uuid, u.id, 'DEMO-R' || right(u.id::text, 2)
FROM users u WHERE u.id::text LIKE '00000000-0000-4000-8000-0000000003%'
ON CONFLICT DO NOTHING;
INSERT INTO enrollments (id, student_id, class_id, starts_on)
SELECT md5('rich-enrollment:' || s.id)::uuid, s.id,
 CASE WHEN right(s.id::text, 2)::int <= 15 THEN '00000000-0000-4000-8000-000000004001' ELSE '00000000-0000-4000-8000-000000004002' END::uuid,
 '2026-09-01'
FROM students s WHERE s.id::text LIKE '00000000-0000-4000-8000-0000000030%'
ON CONFLICT DO NOTHING;

INSERT INTO subjects (id, name, short_name) VALUES
 ('00000000-0000-4000-8000-000000005004', 'Окружающий мир', 'Окр. мир'),
 ('00000000-0000-4000-8000-000000005005', 'Английский язык', 'Англ.'),
 ('00000000-0000-4000-8000-000000005006', 'Физкультура', 'Физ-ра'),
 ('00000000-0000-4000-8000-000000005007', 'Изобразительное искусство', 'ИЗО')
ON CONFLICT DO NOTHING;
INSERT INTO rooms (id, name) VALUES
 ('00000000-0000-4000-8000-000000006003', '103'),
 ('00000000-0000-4000-8000-000000006004', '204'),
 ('00000000-0000-4000-8000-000000006005', 'Спортзал')
ON CONFLICT DO NOTHING;
INSERT INTO class_teachers (id, teacher_id, class_id, starts_on) VALUES
 ('00000000-0000-4000-8000-000000008102', '00000000-0000-4000-8000-000000001003', '00000000-0000-4000-8000-000000004002', '2026-09-01')
ON CONFLICT DO NOTHING;

-- Предмет → учитель и кабинет (у учителя один кабинет, поэтому кабинеты не пересекаются, если не пересекаются учителя).
CREATE TEMP TABLE rich_subjects (code text PRIMARY KEY, subject_id uuid, teacher_id uuid, room_id uuid) ON COMMIT DROP;
INSERT INTO rich_subjects VALUES
 ('math', '00000000-0000-4000-8000-000000005001', '00000000-0000-4000-8000-000000001002', '00000000-0000-4000-8000-000000006001'),
 ('russian', '00000000-0000-4000-8000-000000005002', '00000000-0000-4000-8000-000000001003', '00000000-0000-4000-8000-000000006002'),
 ('literature', '00000000-0000-4000-8000-000000005003', '00000000-0000-4000-8000-000000001003', '00000000-0000-4000-8000-000000006002'),
 ('world', '00000000-0000-4000-8000-000000005004', '00000000-0000-4000-8000-000000001031', '00000000-0000-4000-8000-000000006003'),
 ('english', '00000000-0000-4000-8000-000000005005', '00000000-0000-4000-8000-000000001032', '00000000-0000-4000-8000-000000006004'),
 ('pe', '00000000-0000-4000-8000-000000005006', '00000000-0000-4000-8000-000000001033', '00000000-0000-4000-8000-000000006005'),
 ('art', '00000000-0000-4000-8000-000000005007', '00000000-0000-4000-8000-000000001031', '00000000-0000-4000-8000-000000006003');

INSERT INTO teaching_assignments (id, teacher_id, class_id, subject_id, academic_period_id, starts_on)
SELECT md5('rich-assignment:' || rs.code || c.id || p.id)::uuid, rs.teacher_id, c.id, rs.subject_id, p.id, p.starts_on
FROM rich_subjects rs
CROSS JOIN (VALUES ('00000000-0000-4000-8000-000000004001'::uuid), ('00000000-0000-4000-8000-000000004002')) AS c(id)
JOIN academic_periods p ON p.academic_year_id = '00000000-0000-4000-8000-000000002026'
ON CONFLICT DO NOTHING;

-- Недельная сетка (сгенерирована без пересечений классов, учителей и кабинетов, с учётом базовых строк 5А в понедельник).
CREATE TEMP TABLE rich_timetable (class_code text, subject_code text, weekday smallint, lesson_number smallint, starts_at time, ends_at time) ON COMMIT DROP;
INSERT INTO rich_timetable VALUES
 ('5A', 'math', 1, 4, '11:35', '12:20'),
 ('5A', 'russian', 1, 5, '12:30', '13:15'),
 ('5A', 'math', 2, 1, '08:30', '09:15'),
 ('5A', 'russian', 2, 2, '09:25', '10:10'),
 ('5A', 'english', 2, 3, '10:30', '11:15'),
 ('5A', 'world', 2, 4, '11:35', '12:20'),
 ('5A', 'pe', 2, 5, '12:30', '13:15'),
 ('5A', 'literature', 3, 1, '08:30', '09:15'),
 ('5A', 'math', 3, 2, '09:25', '10:10'),
 ('5A', 'russian', 3, 3, '10:30', '11:15'),
 ('5A', 'english', 3, 4, '11:35', '12:20'),
 ('5A', 'world', 3, 5, '12:30', '13:15'),
 ('5A', 'pe', 4, 1, '08:30', '09:15'),
 ('5A', 'literature', 4, 2, '09:25', '10:10'),
 ('5A', 'art', 4, 3, '10:30', '11:15'),
 ('5A', 'math', 4, 4, '11:35', '12:20'),
 ('5A', 'russian', 4, 5, '12:30', '13:15'),
 ('5A', 'english', 5, 1, '08:30', '09:15'),
 ('5A', 'world', 5, 2, '09:25', '10:10'),
 ('5A', 'pe', 5, 3, '10:30', '11:15'),
 ('5A', 'literature', 5, 4, '11:35', '12:20'),
 ('5A', 'art', 5, 5, '12:30', '13:15'),
 ('5B', 'russian', 1, 1, '08:30', '09:15'),
 ('5B', 'english', 1, 2, '09:25', '10:10'),
 ('5B', 'math', 1, 3, '10:30', '11:15'),
 ('5B', 'russian', 1, 4, '11:35', '12:20'),
 ('5B', 'math', 1, 5, '12:30', '13:15'),
 ('5B', 'russian', 2, 1, '08:30', '09:15'),
 ('5B', 'math', 2, 2, '09:25', '10:10'),
 ('5B', 'world', 2, 3, '10:30', '11:15'),
 ('5B', 'pe', 2, 4, '11:35', '12:20'),
 ('5B', 'literature', 2, 5, '12:30', '13:15'),
 ('5B', 'art', 3, 1, '08:30', '09:15'),
 ('5B', 'russian', 3, 2, '09:25', '10:10'),
 ('5B', 'math', 3, 3, '10:30', '11:15'),
 ('5B', 'world', 3, 4, '11:35', '12:20'),
 ('5B', 'english', 3, 5, '12:30', '13:15'),
 ('5B', 'literature', 4, 1, '08:30', '09:15'),
 ('5B', 'pe', 4, 2, '09:25', '10:10'),
 ('5B', 'math', 4, 3, '10:30', '11:15'),
 ('5B', 'art', 4, 4, '11:35', '12:20'),
 ('5B', 'english', 4, 5, '12:30', '13:15'),
 ('5B', 'russian', 5, 1, '08:30', '09:15'),
 ('5B', 'pe', 5, 2, '09:25', '10:10'),
 ('5B', 'world', 5, 3, '10:30', '11:15'),
 ('5B', 'art', 5, 4, '11:35', '12:20'),
 ('5B', 'literature', 5, 5, '12:30', '13:15');

INSERT INTO schedule_entries (id, class_id, subject_id, teacher_id, room_id, academic_period_id, weekday, lesson_number, starts_at, ends_at, valid_from, valid_to)
SELECT md5('rich-entry:' || t.class_code || t.weekday || t.lesson_number || p.id)::uuid,
 CASE t.class_code WHEN '5A' THEN '00000000-0000-4000-8000-000000004001' ELSE '00000000-0000-4000-8000-000000004002' END::uuid,
 rs.subject_id, rs.teacher_id, rs.room_id, p.id, t.weekday, t.lesson_number, t.starts_at, t.ends_at, p.starts_on, p.ends_on
FROM rich_timetable t JOIN rich_subjects rs ON rs.code = t.subject_code
JOIN academic_periods p ON p.academic_year_id = '00000000-0000-4000-8000-000000002026'
ON CONFLICT DO NOTHING;
-- Базовые строки 5А (понедельник, математика и русский) продолжаются во II четверти.
INSERT INTO schedule_entries (id, class_id, subject_id, teacher_id, room_id, academic_period_id, weekday, lesson_number, starts_at, ends_at, valid_from)
SELECT md5('rich-entry-copy:' || se.id)::uuid, se.class_id, se.subject_id, se.teacher_id, se.room_id,
 '00000000-0000-4000-8000-000000003002', se.weekday, se.lesson_number, se.starts_at, se.ends_at, '2026-11-01'
FROM schedule_entries se WHERE se.id IN ('00000000-0000-4000-8000-000000009001', '00000000-0000-4000-8000-000000009002')
ON CONFLICT DO NOTHING;

-- Уроки по шаблону: с начала периода до двух недель вперёд от сегодняшнего дня, будни, без дублей.
CREATE TEMP TABLE rich_topics (subject_id uuid, n int, topic text) ON COMMIT DROP;
INSERT INTO rich_topics
SELECT rs.subject_id, ord::int, topic FROM rich_subjects rs
CROSS JOIN LATERAL unnest(CASE rs.code
  WHEN 'math' THEN ARRAY['Натуральные числа','Сложение и вычитание','Умножение и деление','Деление с остатком','Степень числа','Числовые выражения','Уравнения','Формулы','Площади фигур','Объём параллелепипеда','Обыкновенные дроби','Сравнение дробей']
  WHEN 'russian' THEN ARRAY['Звуки и буквы','Орфограммы','Состав слова','Словосочетание','Предложение','Главные члены','Второстепенные члены','Однородные члены','Обращение','Прямая речь','Фонетика','Лексика']
  WHEN 'literature' THEN ARRAY['Мифы и легенды','Устное народное творчество','Сказки','Басни Крылова','Пушкин. Сказки','Лермонтов. «Бородино»','Гоголь','Тургенев. «Муму»','Некрасов','Толстой. «Кавказский пленник»','Чехов','Поэзия о природе']
  WHEN 'world' THEN ARRAY['Природа осенью','Вода и её свойства','Воздух','Горные породы','Почва','Растения','Грибы','Животные леса','Экосистемы','Охрана природы','Карта и план','Наша страна']
  WHEN 'english' THEN ARRAY['My family','School subjects','Present Simple','Daily routine','My house','Present Continuous','Animals','Food and drinks','Can / can''t','Weather','Holidays','Past Simple']
  WHEN 'pe' THEN ARRAY['Лёгкая атлетика','Бег на короткие дистанции','Прыжки в длину','Метание мяча','Подвижные игры','Баскетбол','Волейбол','Гимнастика','Акробатика','Эстафеты','Футбол','Общая физическая подготовка']
  ELSE ARRAY['Осенний пейзаж','Натюрморт','Цветоведение','Орнамент','Народные промыслы','Гжель','Хохлома','Портрет','Графика','Аппликация','Лепка','Плакат'] END)
  WITH ORDINALITY AS item(topic, ord);

INSERT INTO lessons (id, lesson_date, starts_at, ends_at, class_id, subject_id, teacher_id, room_id, academic_period_id, schedule_entry_id, topic, status, created_by)
SELECT md5('rich-lesson:' || se.id || d)::uuid, d::date,
 (d::date + se.starts_at) AT TIME ZONE 'Europe/Moscow', (d::date + se.ends_at) AT TIME ZONE 'Europe/Moscow',
 se.class_id, se.subject_id, se.teacher_id, se.room_id, se.academic_period_id, se.id,
 CASE WHEN d::date < current_date THEN COALESCE((SELECT topic FROM rich_topics rt WHERE rt.subject_id = se.subject_id
   AND rt.n = 1 + ((d::date - p.starts_on) / 7) % 12), '') ELSE '' END,
 CASE WHEN d::date < current_date THEN 'completed' ELSE 'planned' END,
 '00000000-0000-4000-8000-000000000001'
FROM schedule_entries se
JOIN academic_periods p ON p.id = se.academic_period_id
CROSS JOIN LATERAL generate_series(p.starts_on, LEAST(p.ends_on, current_date + 13), interval '1 day') AS d
WHERE se.class_id IN ('00000000-0000-4000-8000-000000004001', '00000000-0000-4000-8000-000000004002')
  AND extract(isodow FROM d) = se.weekday
  AND d::date >= se.valid_from AND (se.valid_to IS NULL OR d::date <= se.valid_to)
  AND NOT EXISTS (SELECT 1 FROM lessons l WHERE l.schedule_entry_id = se.id AND l.lesson_date = d::date)
ON CONFLICT DO NOTHING;

-- Работы: классная работа на каждом проведённом уроке, раз в три недели — контрольная по математике и русскому.
INSERT INTO lesson_works (id, lesson_id, work_type, title, weight, is_important, position)
SELECT md5('rich-work:' || l.id)::uuid, l.id, 'classwork', 'Работа на уроке', 1, false, 1
FROM lessons l WHERE l.status = 'completed' AND l.id::text NOT LIKE '00000000-0000-4000-8000-%'
ON CONFLICT DO NOTHING;
INSERT INTO lesson_works (id, lesson_id, work_type, title, weight, is_important, position)
SELECT md5('rich-test:' || l.id)::uuid, l.id, 'test', 'Контрольная работа', 2, true, 2
FROM lessons l
WHERE l.status = 'completed' AND l.id::text NOT LIKE '00000000-0000-4000-8000-%'
  AND l.subject_id IN ('00000000-0000-4000-8000-000000005001', '00000000-0000-4000-8000-000000005002')
  AND extract(week FROM l.lesson_date)::int % 3 = 0 AND extract(isodow FROM l.lesson_date) = 4
ON CONFLICT DO NOTHING;

-- Оценки: у каждого ученика свой «уровень» 3,0–4,9; к нему добавляется разброс ±1,2. Классную работу оценивают выборочно.
INSERT INTO grades (id, student_id, lesson_work_id, academic_period_id, value, created_by)
SELECT md5('rich-grade:' || lw.id || e.student_id)::uuid, e.student_id, lw.id, l.academic_period_id,
 greatest(2, least(5, round(3.0 + (abs(hashtext(e.student_id::text)) % 20) / 10.0
   + ((abs(hashtext(lw.id::text || e.student_id::text)) % 25) - 12) / 10.0))),
 t.user_id
FROM lesson_works lw
JOIN lessons l ON l.id = lw.lesson_id
JOIN teachers t ON t.id = l.teacher_id
JOIN enrollments e ON e.class_id = l.class_id AND e.starts_on <= l.lesson_date AND (e.ends_on IS NULL OR e.ends_on >= l.lesson_date)
WHERE lw.id::text NOT LIKE '00000000-0000-4000-8000-%'
  AND (lw.work_type = 'test' OR abs(hashtext('pick' || lw.id::text || e.student_id::text)) % 100 < 45)
ON CONFLICT DO NOTHING;

INSERT INTO attendance (id, lesson_id, student_id, status, reason, created_by)
SELECT md5('rich-attendance:' || l.id || e.student_id)::uuid, l.id, e.student_id, s.att_status,
 CASE s.att_status WHEN 'excused' THEN 'По болезни' END, t.user_id
FROM lessons l
JOIN teachers t ON t.id = l.teacher_id
JOIN enrollments e ON e.class_id = l.class_id AND e.starts_on <= l.lesson_date AND (e.ends_on IS NULL OR e.ends_on >= l.lesson_date)
CROSS JOIN LATERAL (SELECT CASE
   WHEN abs(hashtext('att' || l.id::text || e.student_id::text)) % 100 < 3 THEN 'absent'
   WHEN abs(hashtext('att' || l.id::text || e.student_id::text)) % 100 < 5 THEN 'excused'
   WHEN abs(hashtext('att' || l.id::text || e.student_id::text)) % 100 < 8 THEN 'late'
   ELSE 'present' END AS att_status) s
WHERE l.status = 'completed' AND l.id::text NOT LIKE '00000000-0000-4000-8000-%'
ON CONFLICT DO NOTHING;

-- Домашние задания примерно к половине уроков (кроме физкультуры и ИЗО), срок — следующий урок по предмету.
INSERT INTO homework (id, class_id, subject_id, lesson_id, author_id, text, due_date, status, published_at)
SELECT md5('rich-homework:' || l.id)::uuid, l.class_id, l.subject_id, l.id, t.user_id,
 CASE l.subject_id
   WHEN '00000000-0000-4000-8000-000000005001' THEN 'Решить задачи № ' || (100 + abs(hashtext(l.id::text)) % 600) || ', ' || (101 + abs(hashtext(l.id::text)) % 600)
   WHEN '00000000-0000-4000-8000-000000005002' THEN 'Упражнение ' || (10 + abs(hashtext(l.id::text)) % 300) || ', выучить правило'
   WHEN '00000000-0000-4000-8000-000000005003' THEN 'Прочитать и подготовить пересказ, стр. ' || (20 + abs(hashtext(l.id::text)) % 200)
   WHEN '00000000-0000-4000-8000-000000005005' THEN 'Workbook p. ' || (5 + abs(hashtext(l.id::text)) % 90) || ', ex. ' || (1 + abs(hashtext(l.id::text)) % 6)
   ELSE 'Подготовить сообщение по теме урока' END,
 COALESCE((SELECT min(n.lesson_date) FROM lessons n WHERE n.class_id = l.class_id AND n.subject_id = l.subject_id AND n.lesson_date > l.lesson_date), l.lesson_date + 2),
 'published', LEAST((l.lesson_date + time '15:00') AT TIME ZONE 'Europe/Moscow', now())
FROM lessons l JOIN teachers t ON t.id = l.teacher_id
WHERE l.id::text NOT LIKE '00000000-0000-4000-8000-%'
  AND l.subject_id NOT IN ('00000000-0000-4000-8000-000000005006', '00000000-0000-4000-8000-000000005007')
  AND l.lesson_date <= current_date + 3
  AND abs(hashtext('hw' || l.id::text)) % 100 < 50
ON CONFLICT DO NOTHING;
INSERT INTO homework_completions (homework_id, student_id, completed_at)
SELECT h.id, e.student_id, (h.due_date - 1 + time '19:00') AT TIME ZONE 'Europe/Moscow'
FROM homework h
JOIN enrollments e ON e.class_id = h.class_id AND e.starts_on <= h.due_date AND (e.ends_on IS NULL OR e.ends_on >= h.due_date)
WHERE h.id::text NOT LIKE '00000000-0000-4000-8000-%' AND h.due_date <= current_date
  AND abs(hashtext('done' || h.id::text || e.student_id::text)) % 100 < 78
ON CONFLICT DO NOTHING;

-- Итоговые отметки за завершённую I четверть: округлённый средневзвешенный балл.
INSERT INTO term_grades (id, student_id, subject_id, academic_period_id, value, created_by)
SELECT md5('rich-term:' || g.student_id || l.subject_id)::uuid, g.student_id, l.subject_id, g.academic_period_id,
 round(sum(g.value * lw.weight) / sum(lw.weight)), min(t.user_id::text)::uuid
FROM grades g JOIN lesson_works lw ON lw.id = g.lesson_work_id JOIN lessons l ON l.id = lw.lesson_id JOIN teachers t ON t.id = l.teacher_id
WHERE g.academic_period_id = '00000000-0000-4000-8000-000000003001' AND g.deleted_at IS NULL AND current_date > '2026-10-31'
GROUP BY g.student_id, l.subject_id, g.academic_period_id
ON CONFLICT DO NOTHING;

INSERT INTO announcements (id, author_id, title, body) VALUES
 ('00000000-0000-4000-8000-000000015002', '00000000-0000-4000-8000-000000000001', 'Родительское собрание', 'Собрание для родителей 5-х классов пройдёт в актовом зале в четверг в 18:00.'),
 ('00000000-0000-4000-8000-000000015003', '00000000-0000-4000-8000-000000000001', 'Школьная олимпиада по математике', 'Регистрация участников открыта до пятницы, обращайтесь к учителю математики.'),
 ('00000000-0000-4000-8000-000000015004', '00000000-0000-4000-8000-000000000001', 'Неделя английского языка', 'Конкурс чтецов и викторина для 5–7 классов. Приходите поддержать участников!')
ON CONFLICT DO NOTHING;
INSERT INTO announcement_audiences (id, announcement_id, audience_type) VALUES
 ('00000000-0000-4000-8000-000000015102', '00000000-0000-4000-8000-000000015002', 'school'),
 ('00000000-0000-4000-8000-000000015103', '00000000-0000-4000-8000-000000015003', 'school'),
 ('00000000-0000-4000-8000-000000015104', '00000000-0000-4000-8000-000000015004', 'school')
ON CONFLICT DO NOTHING;
