-- Данные для сценариев k6 из нагрузочной базы (DEMO_DATASET=load): у каждого виртуального учителя свой класс,
-- предмет, уроки и работы I четверти; у ученика и родителя — идентификатор ученика. Результат — один JSON-документ.
--   psql -At -f load/fixtures.sql "$DATABASE_URL" > .tmp/load/fixtures.json
WITH period AS (
  SELECT id FROM academic_periods WHERE academic_year_id = '00000000-0000-4000-8000-000000002026' ORDER BY starts_on LIMIT 1
), teacher_rows AS (
  SELECT u.login, ta.class_id, ta.subject_id,
    (SELECT json_agg(x.id) FROM (SELECT l.id FROM lessons l WHERE l.class_id = ta.class_id AND l.subject_id = ta.subject_id
       AND l.academic_period_id = (SELECT id FROM period) AND l.status = 'completed' ORDER BY l.lesson_date DESC LIMIT 6) x) AS lesson_ids,
    (SELECT json_agg(x.id) FROM (SELECT lw.id FROM lesson_works lw JOIN lessons l ON l.id = lw.lesson_id
       WHERE l.class_id = ta.class_id AND l.subject_id = ta.subject_id AND l.academic_period_id = (SELECT id FROM period)
       ORDER BY l.lesson_date DESC LIMIT 12) x) AS work_ids,
    (SELECT json_agg(e.student_id) FROM enrollments e WHERE e.class_id = ta.class_id AND e.ends_on IS NULL) AS student_ids
  FROM teaching_assignments ta
  JOIN teachers t ON t.id = ta.teacher_id JOIN users u ON u.id = t.user_id
  WHERE u.login LIKE 'load-teacher%' AND ta.academic_period_id = (SELECT id FROM period)
  ORDER BY u.login
)
SELECT json_build_object(
  'period_id', (SELECT id FROM period),
  'teachers', (SELECT json_agg(json_build_object('login', login, 'class_id', class_id, 'subject_id', subject_id,
    'lesson_ids', lesson_ids, 'work_ids', work_ids, 'student_ids', student_ids)) FROM teacher_rows),
  'students', (SELECT json_agg(json_build_object('login', u.login, 'student_id', s.id) ORDER BY u.login)
    FROM students s JOIN users u ON u.id = s.user_id WHERE u.login LIKE 'load-student%'),
  'parents', (SELECT json_agg(json_build_object('login', u.login, 'student_id', ps.student_id) ORDER BY u.login)
    FROM parents p JOIN users u ON u.id = p.user_id JOIN parent_student ps ON ps.parent_id = p.id WHERE u.login LIKE 'load-parent%')
);
