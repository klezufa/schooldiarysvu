-- «Сегодня» по часовому поясу школы, а не сессии PostgreSQL (в контейнере это UTC). Записи о зачислении, связях
-- родителей и назначениях получают дату по школьному времени, и запросы должны сравнивать их с той же датой —
-- иначе с полуночи по Москве до 03:00 только что созданная связь «ещё не началась».
CREATE FUNCTION school_today() RETURNS date
LANGUAGE sql STABLE
AS $$
    SELECT (now() AT TIME ZONE COALESCE((SELECT timezone FROM school_settings LIMIT 1), 'UTC'))::date
$$;

ALTER TABLE parent_student ALTER COLUMN valid_from SET DEFAULT school_today();
