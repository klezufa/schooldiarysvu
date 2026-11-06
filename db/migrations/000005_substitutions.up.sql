-- Замена учителя на конкретном уроке. Урок ведёт lessons.teacher_id (заменяющий), здесь — кого заменили и почему:
-- так расписание, проверка пересечений и «мои уроки» работают без изменений, а замену можно отменить.
CREATE TABLE lesson_substitutions (
    lesson_id uuid PRIMARY KEY REFERENCES lessons(id) ON DELETE RESTRICT,
    original_teacher_id uuid NOT NULL REFERENCES teachers(id) ON DELETE RESTRICT,
    reason text NOT NULL DEFAULT '' CHECK (length(reason) <= 300),
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX lesson_substitutions_original_idx ON lesson_substitutions (original_teacher_id);

ALTER TABLE notifications DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check
    CHECK (kind IN ('grade','homework','attendance','announcement','digest','schedule'));

-- Код подлинности выданного архива журнала ищут по журналу аудита.
CREATE INDEX audit_journal_archive_code_idx ON audit_logs ((new_values->>'code')) WHERE action = 'journal_archive.issued';
