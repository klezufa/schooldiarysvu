-- Служебные события эксплуатации: результаты резервного копирования и восстановления.
-- Пишут скрипты scripts/backup.sh, scripts/restore.sh и их Windows-аналоги; читает страница «Система».
CREATE TABLE maintenance_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind text NOT NULL CHECK (kind IN ('backup', 'restore')),
    status text NOT NULL CHECK (status IN ('success', 'failure')),
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX maintenance_events_kind_time_idx ON maintenance_events (kind, status, created_at DESC);
