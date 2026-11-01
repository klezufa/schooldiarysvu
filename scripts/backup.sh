#!/usr/bin/env bash
# Резервная копия «Школьного дневника» (Docker Compose): база (pg_dump -Fc), файлы вложений, версия схемы,
# manifest и контрольные суммы. Готовая копия публикуется атомарно; при любой ошибке — сообщение об ошибке,
# запись в журнал эксплуатации и ненулевой код выхода (успех не сообщается при частичной копии).
#
#   scripts/backup.sh [--quiesce] [--label NAME]
#
#   --quiesce   на время копирования останавливает nginx, worker и backend: БД и файлы гарантированно
#               согласованы (пользователи 10–60 секунд видят недоступность). Без флага копия «на ходу»:
#               база согласована внутри себя, но файлы могут оказаться чуть новее базы.
#   --label     префикс имени копии (по умолчанию diary; restore.sh делает страховочную копию с меткой pre-restore)
#
# Переменные: BACKUP_DIR (по умолчанию ./backups), BACKUP_KEEP (сколько копий хранить, 14),
# BACKUP_REMOTE — куда дополнительно скопировать готовую копию: каталог на другом диске/NFS или user@host:/path
# (rsync по SSH с ключом). Без BACKUP_REMOTE скрипт предупреждает, что единственная копия лежит на этом сервере.
# shellcheck disable=SC2016 # команды в одинарных кавычках выполняются в контейнере postgres с его переменными
set -Eeuo pipefail
umask 077
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

QUIESCE=0
LABEL=diary
while (($#)); do
  case "$1" in
    --quiesce) QUIESCE=1 ;;
    --label) LABEL="${2:?укажите метку}"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "неизвестный параметр $1" ;;
  esac
  shift
done
[[ "$LABEL" =~ ^[a-z0-9-]+$ ]] || die "метка: только латиница в нижнем регистре, цифры и дефис"

BACKUP_DIR="${BACKUP_DIR:-$ROOT/backups}"
BACKUP_KEEP="${BACKUP_KEEP:-14}"
mkdir -p "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR"
LOG_FILE="$BACKUP_DIR/backup.log"
require docker sha256sum tar

STAMP="$(date '+%Y%m%d-%H%M%S')"
NAME="$LABEL-$STAMP"
PARTIAL="$BACKUP_DIR/.$NAME.partial"
FINAL="$BACKUP_DIR/$NAME"
STOPPED=0

cleanup() {
  local code=$?
  trap - ERR
  if ((code != 0)); then
    record_event backup failure "{\"reason\": \"$(json_escape "${LAST_ERROR:-код выхода $code}")\", \"name\": \"$NAME\"}"
    log "РЕЗЕРВНАЯ КОПИЯ НЕ СОЗДАНА"
  fi
  rm -rf "$PARTIAL"
  compose exec -T postgres rm -f /tmp/diary-backup.dump >/dev/null 2>&1 || true
  if ((STOPPED)); then
    log "запуск приложения после копирования"
    compose start backend worker nginx >/dev/null 2>&1 || log "ОШИБКА: не удалось снова запустить приложение — выполните: docker compose start"
  fi
  return $code
}
on_error() {
  LAST_ERROR="шаг «$STEP», код $1"
  log "ОШИБКА резервного копирования: $LAST_ERROR"
}
trap 'on_error "$?"' ERR
trap cleanup EXIT

step "начало резервного копирования $NAME (проект $PROJECT)"
compose ps --status running --services | grep -qx postgres || die "контейнер postgres не запущен"
mkdir "$PARTIAL"

if ((QUIESCE)); then
  step "остановка nginx, worker, backend для согласованной копии"
  STOPPED=1
  compose stop nginx worker backend >/dev/null
fi

step "дамп базы данных"
# Дамп снимается внутри контейнера и копируется файлом: двоичные данные не проходят через конвейеры оболочки.
compose exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc -f /tmp/diary-backup.dump'
compose cp postgres:/tmp/diary-backup.dump "$(host_path "$PARTIAL")/database.dump" >/dev/null
# -1 — миграция прервана (dirty): такую базу нельзя считать надёжной копией.
SCHEMA="$(db_psql -At -c 'SELECT CASE WHEN dirty THEN -1 ELSE version END FROM schema_migrations')"
[[ "$SCHEMA" =~ ^[0-9]+$ ]] || die "миграции БД в незавершённом состоянии ($SCHEMA) — копирование отменено"

step "архив файлов вложений"
docker run --rm -v "$FILES_VOLUME:/data:ro" -v "$(host_path "$PARTIAL"):/backup" "$HELPER_IMAGE" \
  tar -czf /backup/files.tgz -C /data .

if ((STOPPED)); then
  step "запуск приложения"
  compose start backend worker nginx >/dev/null
  STOPPED=0
fi

step "manifest и контрольные суммы"
COMMIT="$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)"
(cd "$PARTIAL" && sha256sum database.dump files.tgz >SHA256SUMS)
SIZE=$(($(stat -c %s "$PARTIAL/database.dump") + $(stat -c %s "$PARTIAL/files.tgz")))
cat >"$PARTIAL/manifest.json" <<EOF
{
  "kind": "docker-compose",
  "name": "$NAME",
  "created_at": "$(date -u '+%Y-%m-%dT%H:%M:%SZ')",
  "project": "$PROJECT",
  "schema_version": $SCHEMA,
  "git_commit": "$COMMIT",
  "consistent": $( ((QUIESCE)) && echo true || echo false ),
  "size_bytes": $SIZE
}
EOF
chmod 600 "$PARTIAL"/*
(cd "$PARTIAL" && sha256sum -c --quiet SHA256SUMS)
mv "$PARTIAL" "$FINAL"
log "копия готова: $FINAL ($SIZE байт, схема $SCHEMA)"

# Хранение: удаляются только самые старые копии с той же меткой; страховочные pre-restore считаются отдельно.
mapfile -t OLD < <(find "$BACKUP_DIR" -mindepth 1 -maxdepth 1 -type d -name "$LABEL-[0-9]*" | sort)
if ((${#OLD[@]} > BACKUP_KEEP)); then
  for dir in "${OLD[@]:0:${#OLD[@]}-BACKUP_KEEP}"; do
    log "удаление устаревшей копии $(basename "$dir")"
    rm -rf -- "$dir"
  done
fi

if [[ -n "${BACKUP_REMOTE:-}" ]]; then
  step "передача копии в $BACKUP_REMOTE"
  if [[ "$BACKUP_REMOTE" == *:* ]]; then
    command -v rsync >/dev/null 2>&1 || die "для BACKUP_REMOTE=$BACKUP_REMOTE нужен rsync"
    rsync -a --chmod=D700,F600 "$FINAL" "$BACKUP_REMOTE/"
  else
    mkdir -p "$BACKUP_REMOTE"
    cp -a "$FINAL" "$BACKUP_REMOTE/"
    (cd "$BACKUP_REMOTE/$NAME" && sha256sum -c --quiet SHA256SUMS)
  fi
  log "копия передана в $BACKUP_REMOTE"
else
  log "предупреждение: BACKUP_REMOTE не задан — единственная копия лежит на этом сервере; копируйте её на другой носитель"
fi

record_event backup success "{\"name\": \"$NAME\", \"size_bytes\": $SIZE, \"schema_version\": $SCHEMA, \"consistent\": $( ((QUIESCE)) && echo true || echo false )}"
log "резервное копирование успешно завершено"
printf '%s\n' "$FINAL"
