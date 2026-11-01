ALTER TABLE parent_student ALTER COLUMN valid_from SET DEFAULT current_date;
DROP FUNCTION IF EXISTS school_today();
