CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE roles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL UNIQUE CHECK (code IN ('administrator','teacher','class_teacher','student','parent','principal')),
    name text NOT NULL
);
INSERT INTO roles (code, name) VALUES
    ('administrator','Администратор'),('teacher','Учитель'),('class_teacher','Классный руководитель'),
    ('student','Ученик'),('parent','Родитель'),('principal','Руководитель школы');

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    login text NOT NULL UNIQUE CHECK (login = lower(btrim(login)) AND length(login) >= 3),
    password_hash text NOT NULL,
    display_name text NOT NULL CHECK (length(btrim(display_name)) > 0),
    is_active boolean NOT NULL DEFAULT true,
    must_change_password boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deactivated_at timestamptz
);
CREATE TABLE user_roles (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    role_id uuid NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    PRIMARY KEY (user_id, role_id)
);
CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    csrf_hash bytea NOT NULL CHECK (octet_length(csrf_hash) = 32),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    user_agent text,
    ip inet
);
CREATE INDEX sessions_active_idx ON sessions (token_hash, expires_at) WHERE revoked_at IS NULL;
CREATE INDEX sessions_user_idx ON sessions (user_id) WHERE revoked_at IS NULL;
CREATE TABLE students (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL UNIQUE REFERENCES users(id) ON DELETE RESTRICT,
    student_code text UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE teachers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL UNIQUE REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE parents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL UNIQUE REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE parent_student (
    parent_id uuid NOT NULL REFERENCES parents(id) ON DELETE RESTRICT,
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
    valid_from date NOT NULL DEFAULT current_date,
    valid_to date,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (parent_id, student_id, valid_from),
    CHECK (valid_to IS NULL OR valid_to >= valid_from)
);
CREATE INDEX parent_student_active_idx ON parent_student (parent_id, student_id, valid_from, valid_to);

CREATE TABLE school_settings (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    school_name text NOT NULL DEFAULT 'Школа',
    timezone text NOT NULL DEFAULT 'Europe/Moscow',
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO school_settings DEFAULT VALUES;
CREATE TABLE grade_scales (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE,
    allowed_values jsonb NOT NULL CHECK (jsonb_typeof(allowed_values) = 'array'),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO grade_scales (name, allowed_values) VALUES ('Пятибалльная', '[1,2,3,4,5]');
CREATE TABLE academic_years (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE,
    starts_on date NOT NULL,
    ends_on date NOT NULL,
    is_current boolean NOT NULL DEFAULT false,
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_on > starts_on)
);
CREATE UNIQUE INDEX academic_one_current_idx ON academic_years (is_current) WHERE is_current AND archived_at IS NULL;
CREATE TABLE academic_periods (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    academic_year_id uuid NOT NULL REFERENCES academic_years(id) ON DELETE RESTRICT,
    name text NOT NULL,
    starts_on date NOT NULL,
    ends_on date NOT NULL,
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open','closed')),
    allow_late_edits boolean NOT NULL DEFAULT false,
    closed_at timestamptz,
    closed_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (academic_year_id, name),
    CHECK (ends_on > starts_on),
    CHECK ((status = 'open' AND closed_at IS NULL) OR (status = 'closed' AND closed_at IS NOT NULL))
);
CREATE TABLE classes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    academic_year_id uuid NOT NULL REFERENCES academic_years(id) ON DELETE RESTRICT,
    name text NOT NULL,
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (academic_year_id, name)
);
CREATE TABLE subjects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE,
    short_name text,
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE rooms (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE,
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE enrollments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
    class_id uuid NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
    starts_on date NOT NULL,
    ends_on date,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_on IS NULL OR ends_on >= starts_on)
);
CREATE INDEX enrollments_student_date_idx ON enrollments (student_id, starts_on, ends_on);
CREATE INDEX enrollments_class_date_idx ON enrollments (class_id, starts_on, ends_on);
CREATE TABLE teaching_assignments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    teacher_id uuid NOT NULL REFERENCES teachers(id) ON DELETE RESTRICT,
    class_id uuid NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
    subject_id uuid NOT NULL REFERENCES subjects(id) ON DELETE RESTRICT,
    academic_period_id uuid NOT NULL REFERENCES academic_periods(id) ON DELETE RESTRICT,
    starts_on date NOT NULL,
    ends_on date,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (teacher_id, class_id, subject_id, academic_period_id, starts_on),
    CHECK (ends_on IS NULL OR ends_on >= starts_on)
);
CREATE INDEX teaching_assignments_lookup_idx ON teaching_assignments (teacher_id, class_id, subject_id, starts_on, ends_on);
CREATE TABLE class_teachers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    teacher_id uuid NOT NULL REFERENCES teachers(id) ON DELETE RESTRICT,
    class_id uuid NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
    starts_on date NOT NULL,
    ends_on date,
    UNIQUE (teacher_id, class_id, starts_on),
    CHECK (ends_on IS NULL OR ends_on >= starts_on)
);

CREATE TABLE schedule_entries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    class_id uuid NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
    subject_id uuid NOT NULL REFERENCES subjects(id) ON DELETE RESTRICT,
    teacher_id uuid NOT NULL REFERENCES teachers(id) ON DELETE RESTRICT,
    room_id uuid REFERENCES rooms(id) ON DELETE RESTRICT,
    academic_period_id uuid NOT NULL REFERENCES academic_periods(id) ON DELETE RESTRICT,
    weekday smallint NOT NULL CHECK (weekday BETWEEN 1 AND 7),
    lesson_number smallint NOT NULL CHECK (lesson_number BETWEEN 1 AND 12),
    starts_at time NOT NULL,
    ends_at time NOT NULL,
    valid_from date NOT NULL,
    valid_to date,
    week_parity smallint NOT NULL DEFAULT 0 CHECK (week_parity IN (0,1,2)),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at),
    CHECK (valid_to IS NULL OR valid_to >= valid_from)
);
CREATE INDEX schedule_class_weekday_idx ON schedule_entries (class_id, weekday, valid_from, valid_to);
CREATE INDEX schedule_teacher_weekday_idx ON schedule_entries (teacher_id, weekday, valid_from, valid_to);
CREATE TABLE lessons (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    lesson_date date NOT NULL,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    class_id uuid NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
    subject_id uuid NOT NULL REFERENCES subjects(id) ON DELETE RESTRICT,
    teacher_id uuid NOT NULL REFERENCES teachers(id) ON DELETE RESTRICT,
    room_id uuid REFERENCES rooms(id) ON DELETE RESTRICT,
    academic_period_id uuid NOT NULL REFERENCES academic_periods(id) ON DELETE RESTRICT,
    schedule_entry_id uuid REFERENCES schedule_entries(id) ON DELETE SET NULL,
    topic text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'planned' CHECK (status IN ('planned','completed','cancelled')),
    cancellation_reason text,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    updated_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at),
    CHECK (status <> 'cancelled' OR cancellation_reason IS NOT NULL)
);
-- UNIQUE cannot detect overlapping time ranges. Class/teacher/room overlaps are
-- checked inside a serializable domain transaction with advisory locks.
CREATE INDEX lessons_class_period_date_idx ON lessons (class_id, subject_id, academic_period_id, lesson_date);
CREATE INDEX lessons_teacher_date_idx ON lessons (teacher_id, lesson_date, starts_at);
CREATE INDEX lessons_room_time_idx ON lessons (room_id, starts_at, ends_at);

CREATE TABLE lesson_works (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    lesson_id uuid NOT NULL REFERENCES lessons(id) ON DELETE RESTRICT,
    work_type text NOT NULL CHECK (work_type IN ('classwork','homework','test','quiz','project','oral')),
    title text NOT NULL,
    weight numeric(6,2) NOT NULL DEFAULT 1 CHECK (weight > 0),
    is_important boolean NOT NULL DEFAULT false,
    position integer NOT NULL DEFAULT 1 CHECK (position > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (lesson_id, position)
);
CREATE TABLE grades (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
    lesson_work_id uuid NOT NULL REFERENCES lesson_works(id) ON DELETE RESTRICT,
    academic_period_id uuid NOT NULL REFERENCES academic_periods(id) ON DELETE RESTRICT,
    value numeric(5,2) NOT NULL CHECK (value >= 0),
    comment text,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    deleted_at timestamptz
);
CREATE UNIQUE INDEX grades_active_unique_idx ON grades (student_id, lesson_work_id) WHERE deleted_at IS NULL;
CREATE INDEX grades_student_period_idx ON grades (student_id, academic_period_id) WHERE deleted_at IS NULL;
CREATE INDEX grades_work_idx ON grades (lesson_work_id) WHERE deleted_at IS NULL;
CREATE TABLE term_grades (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
    subject_id uuid NOT NULL REFERENCES subjects(id) ON DELETE RESTRICT,
    academic_period_id uuid NOT NULL REFERENCES academic_periods(id) ON DELETE RESTRICT,
    value numeric(5,2) NOT NULL CHECK (value >= 0),
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (student_id, subject_id, academic_period_id)
);
CREATE TABLE attendance (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    lesson_id uuid NOT NULL REFERENCES lessons(id) ON DELETE RESTRICT,
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
    status text NOT NULL CHECK (status IN ('present','absent','late','excused','exempt')),
    reason text,
    comment text,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE (lesson_id, student_id)
);
CREATE INDEX attendance_student_idx ON attendance (student_id, lesson_id);

CREATE TABLE homework (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    class_id uuid NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
    subject_id uuid NOT NULL REFERENCES subjects(id) ON DELETE RESTRICT,
    lesson_id uuid REFERENCES lessons(id) ON DELETE RESTRICT,
    author_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    text text NOT NULL CHECK (length(btrim(text)) > 0),
    due_date date NOT NULL,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published')),
    published_at timestamptz,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (status <> 'published' OR published_at IS NOT NULL)
);
CREATE INDEX homework_class_due_idx ON homework (class_id, due_date, status);
CREATE TABLE homework_recipients (
    homework_id uuid NOT NULL REFERENCES homework(id) ON DELETE RESTRICT,
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
    PRIMARY KEY (homework_id, student_id)
);
-- No homework_recipients rows means the whole class is the audience.
CREATE TABLE homework_completions (
    homework_id uuid NOT NULL REFERENCES homework(id) ON DELETE RESTRICT,
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
    completed_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (homework_id, student_id)
);
CREATE TABLE files (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    storage_key text NOT NULL UNIQUE,
    original_name text NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
    mime_type text NOT NULL,
    owner_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE TABLE homework_files (
    homework_id uuid NOT NULL REFERENCES homework(id) ON DELETE RESTRICT,
    file_id uuid NOT NULL UNIQUE REFERENCES files(id) ON DELETE RESTRICT,
    PRIMARY KEY (homework_id, file_id)
);

CREATE TABLE announcements (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    title text NOT NULL,
    body text NOT NULL,
    published_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE announcement_audiences (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    announcement_id uuid NOT NULL REFERENCES announcements(id) ON DELETE RESTRICT,
    audience_type text NOT NULL CHECK (audience_type IN ('school','class','subject')),
    class_id uuid REFERENCES classes(id) ON DELETE RESTRICT,
    subject_id uuid REFERENCES subjects(id) ON DELETE RESTRICT,
    CHECK ((audience_type = 'school' AND class_id IS NULL AND subject_id IS NULL) OR
           (audience_type = 'class' AND class_id IS NOT NULL AND subject_id IS NULL) OR
           (audience_type = 'subject' AND subject_id IS NOT NULL AND class_id IS NULL)),
    UNIQUE (announcement_id, audience_type, class_id, subject_id)
);
CREATE INDEX announcement_audience_class_idx ON announcement_audiences (class_id, announcement_id);
CREATE INDEX announcement_audience_subject_idx ON announcement_audiences (subject_id, announcement_id);
CREATE TABLE notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    kind text NOT NULL CHECK (kind IN ('grade','homework','attendance','announcement','digest')),
    title text NOT NULL,
    body text NOT NULL,
    entity_type text,
    entity_id uuid,
    read_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notifications_user_unread_idx ON notifications (user_id, created_at DESC) WHERE read_at IS NULL;
CREATE TABLE notification_preferences (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE RESTRICT,
    internal_enabled boolean NOT NULL DEFAULT true,
    digest_frequency text NOT NULL DEFAULT 'daily' CHECK (digest_frequency IN ('never','daily','weekly')),
    quiet_from time,
    quiet_to time,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE audit_logs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id uuid REFERENCES users(id) ON DELETE RESTRICT,
    action text NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    old_values jsonb,
    new_values jsonb,
    request_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_entity_idx ON audit_logs (entity_type, entity_id, created_at DESC);
CREATE INDEX audit_actor_time_idx ON audit_logs (actor_id, created_at DESC);
CREATE FUNCTION prevent_audit_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'audit log is append-only'; END;
$$;
CREATE TRIGGER audit_logs_immutable BEFORE UPDATE OR DELETE ON audit_logs
FOR EACH ROW EXECUTE FUNCTION prevent_audit_mutation();

CREATE TABLE idempotency_keys (
    actor_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    key text NOT NULL CHECK (length(key) BETWEEN 8 AND 128),
    operation text NOT NULL,
    request_hash text NOT NULL,
    response jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (actor_id, key, operation)
);
