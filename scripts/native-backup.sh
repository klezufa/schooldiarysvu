#!/usr/bin/env bash
# Резервная копия «Школьного дневника», установленного без Docker (docs/native-install.md): база (pg_dump -Fc),
# каталог вложений, версия схемы и контрольные суммы. Копия публикуется атомарно (до конца имеет суффикс
# .partial); итог — успех или ошибка — записывается в журнал эксплуатации и виден на странице «Система».
#
#   sudo scripts/native-backup.sh [--quiesce]
#
#   --quiesce   на время копирования останавливает службы дневника: база и файлы гарантированно согласованы
#               (пользователи 10–60 секунд видят недоступность). Без флага копия «на ходу».
#
# Переменные (обычно из /etc/default/schooldiary): BACKUP_DIR (/var/backups/schooldiary), BACKUP_KEEP (14),
# BACKUP_REMOTE — куда дополнительно скопировать готовую копию: каталог на другом диске или user@host:/path,
# DIARY_DB (diary), DIARY_FILES (/var/lib/schooldiary/files).
# Для установки в Docker используйте scripts/backup.sh.
set -euo pipefail

QUIESCE=0
for arg in "$@"; do
  case "$arg" in
    --quiesce) QUIESCE=1 ;;
    -h|--help) sed -n '2,13p' "$0"; exit 0 ;;
    *) echo "неизвестный параметр $arg" >&2; exit 2 ;;
  esac
done
[[ $EUID -eq 0 ]] || { echo "запускайте через sudo" >&2; exit 1; }

BACKUP_DIR="${BACKUP_DIR:-/var/backups/schooldiary}"
BACKUP_KEEP="${BACKUP_KEEP:-14}"
DIARY_DB="${DIARY_DB:-diary}"
DIARY_FILES="${DIARY_FILES:-/var/lib/schooldiary/files}"
SERVICES=(schooldiary-server schooldiary-worker)
NAME="diary-$(date +%Y%m%d-%H%M%S)"
WORK="$BACKUP_DIR/$NAME.partial"
STOPPED=0

log() { printf '%s %s\n' "$(date '+%F %T')" "$*" | tee -a "$BACKUP_DIR/backup.log" >&2; }
psql_db() { sudo -u postgres psql -d "$DIARY_DB" -v ON_ERROR_STOP=1 -q "$@"; }
json_escape() { local value="${1//\\/\\\\}"; value="${value//\"/\\\"}"; printf '%s' "${value//$'\n'/ }"; }

# Запись в журнал эксплуатации; её сбой не должен скрывать исход самой копии.
record_event() {
  if ! printf "INSERT INTO maintenance_events (kind, status, details) VALUES ('backup', :'status', :'details'::jsonb);\n" |
    psql_db -v status="$1" -v details="$2" >/dev/null 2>&1; then
    log "предупреждение: не удалось записать итог копии в БД (страница «Система» его не покажет)"
  fi
}

resume() {
  if ((STOPPED)); then
    systemctl start "${SERVICES[@]}" || log "ошибка: службы не запустились, проверьте systemctl status"
    STOPPED=0
  fi
}

LAST_ERROR=""
on_exit() {
  local code=$?
  resume
  if ((code != 0)); then
    rm -rf "$WORK"
    log "копия не создана: ${LAST_ERROR:-код выхода $code}"
    record_event failure "{\"reason\": \"$(json_escape "${LAST_ERROR:-код выхода $code}")\", \"name\": \"$NAME\"}"
  fi
}
trap on_exit EXIT

install -d -m 700 "$BACKUP_DIR"
mkdir -m 700 "$WORK"

if ((QUIESCE)); then
  LAST_ERROR="не удалось остановить службы"
  systemctl stop "${SERVICES[@]}"
  STOPPED=1
fi

LAST_ERROR="не удалось прочитать версию схемы"
SCHEMA="$(psql_db -At -c 'SELECT CASE WHEN dirty THEN -1 ELSE version END FROM schema_migrations')"

LAST_ERROR="pg_dump завершился с ошибкой"
sudo -u postgres pg_dump -Fc "$DIARY_DB" > "$WORK/database.dump"

LAST_ERROR="не удалось упаковать вложения"
tar -C "$(dirname "$DIARY_FILES")" -czf "$WORK/files.tar.gz" "$(basename "$DIARY_FILES")"
resume

LAST_ERROR="не удалось посчитать контрольные суммы"
(cd "$WORK" && sha256sum database.dump files.tar.gz > SHA256SUMS)
printf '{"name": "%s", "created": "%s", "schema_version": %s, "consistent": %s, "commit": "%s"}\n' \
  "$NAME" "$(date --iso-8601=seconds)" "$SCHEMA" "$( ((QUIESCE)) && echo true || echo false )" \
  "$(git -C "$(dirname "$0")/.." describe --tags --always 2>/dev/null || echo unknown)" > "$WORK/manifest.json"

LAST_ERROR="не удалось опубликовать копию"
mv "$WORK" "$BACKUP_DIR/$NAME"
SIZE="$(du -sb "$BACKUP_DIR/$NAME" | cut -f1)"

# Храним BACKUP_KEEP последних копий; страховочные копии других меток не трогаем.
mapfile -t OLD < <(find "$BACKUP_DIR" -maxdepth 1 -type d -name 'diary-*' ! -name '*.partial' | sort | head -n "-$BACKUP_KEEP")
((${#OLD[@]})) && rm -rf "${OLD[@]}"

if [[ -n "${BACKUP_REMOTE:-}" ]]; then
  LAST_ERROR="не удалось скопировать в BACKUP_REMOTE"
  rsync -a "$BACKUP_DIR/$NAME" "$BACKUP_REMOTE/"
else
  log "предупреждение: BACKUP_REMOTE не задан — единственная копия лежит на этом сервере"
fi

record_event success "{\"name\": \"$NAME\", \"size_bytes\": $SIZE, \"schema_version\": $SCHEMA, \"consistent\": $( ((QUIESCE)) && echo true || echo false )}"
log "копия готова: $BACKUP_DIR/$NAME ($SIZE байт)"
