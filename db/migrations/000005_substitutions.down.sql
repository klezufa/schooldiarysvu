DROP INDEX IF EXISTS audit_journal_archive_code_idx;
DELETE FROM notifications WHERE kind = 'schedule';
ALTER TABLE notifications DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check
    CHECK (kind IN ('grade','homework','attendance','announcement','digest'));
DROP TABLE IF EXISTS lesson_substitutions;
