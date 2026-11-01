-- Роль системного администратора, приглашения для саморегистрации и метрики сервисов.

ALTER TABLE roles DROP CONSTRAINT roles_code_check;
ALTER TABLE roles ADD CONSTRAINT roles_code_check CHECK (code IN
    ('system_admin','administrator','teacher','class_teacher','student','parent','principal'));
INSERT INTO roles (code, name) VALUES ('system_admin', 'Системный администратор');

-- Время последнего входа хранится в пользователе: сессии удаляются через 30 дней после истечения,
-- и по ним нельзя отличить «давно не входил» от «никогда не входил».
ALTER TABLE users ADD COLUMN last_login_at timestamptz;
UPDATE users u SET last_login_at = (SELECT max(s.created_at) FROM sessions s WHERE s.user_id = u.id);

-- Регистронезависимое сравнение, не зависящее от локали кластера: lower() в локали C не понижает кириллицу.
CREATE FUNCTION ru_lower(value text) RETURNS text LANGUAGE sql IMMUTABLE PARALLEL SAFE STRICT
AS $$ SELECT lower(translate(value, 'АБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯ', 'абвгдеёжзийклмнопрстуфхцчшщъыьэюя')) $$;

-- Одноразовое приглашение: токен хранится только как SHA-256, как и токены сессий.
CREATE TABLE invitations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    role_codes text[] NOT NULL CHECK (cardinality(role_codes) > 0),
    note text NOT NULL DEFAULT '',
    class_id uuid REFERENCES classes(id) ON DELETE RESTRICT,
    student_id uuid REFERENCES students(id) ON DELETE RESTRICT,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    used_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    revoked_at timestamptz,
    CHECK (expires_at > created_at),
    CHECK ((used_at IS NULL) = (used_by IS NULL))
);
CREATE INDEX invitations_open_idx ON invitations (expires_at) WHERE used_at IS NULL AND revoked_at IS NULL;

-- Каждый процесс (server, worker) периодически записывает свою строку; дашборд системного
-- администратора читает их, поэтому видит и сервисы, которые не отвечают по HTTP.
CREATE TABLE service_heartbeats (
    service text NOT NULL,
    instance text NOT NULL,
    version text NOT NULL DEFAULT '',
    started_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    stats jsonb NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (service, instance)
);
CREATE TABLE system_metric_samples (
    service text NOT NULL,
    instance text NOT NULL,
    sampled_at timestamptz NOT NULL,
    metrics jsonb NOT NULL,
    PRIMARY KEY (service, instance, sampled_at)
);
CREATE INDEX system_metric_samples_time_idx ON system_metric_samples (sampled_at);
