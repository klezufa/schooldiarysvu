#!/usr/bin/env bash
# Восстановление «Школьного дневника» из копии, созданной scripts/backup.sh.
#
#   scripts/restore.sh КАТАЛОГ_КОПИИ [--yes]
#
# Порядок: проверка контрольных сумм и версии схемы → подтверждение → страховочная копия текущего состояния
# (метка pre-restore) → остановка nginx, worker, backend → база восстанавливается во временную БД и только после
# успешного pg_restore подменяет рабочую (сбой pg_restore не трогает текущие данные) → файлы вложений →
# запуск, ожидание готовности, запись в журнал эксплуатации. Если схема копии старше кода, backend при запуске
# сам применит недостающие миграции; копию с более новой схемой, чем знает этот код, скрипт не восстанавливает.
#
#   --yes   не спрашивать подтверждение (для автоматических проверок восстановления на тестовом стенде)
# shellcheck disable=SC2016 # команды в одинарных кавычках выполняются в контейнере postgres с его переменными
set -Eeuo pipefail
umask 077
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

SOURCE=""
ASSUME_YES=0
while (($#)); do
  case "$1" in
    --yes) ASSUME_YES=1 ;;
    -h|--help) usage; exit 0 ;;
    -*) die "неизвестный параметр $1" ;;
    *) [[ -z "$SOURCE" ]] || die "укажите один каталог копии"; SOURCE="$1" ;;
  esac
  shift
done
[[ -n "$SOURCE" ]] || die "укажите каталог копии: scripts/restore.sh backups/diary-ГГГГММДД-ЧЧММСС"
[[ -d "$SOURCE" ]] || die "каталог $SOURCE не найден"
SOURCE="$(cd "$SOURCE" && pwd)"
BACKUP_DIR="${BACKUP_DIR:-$ROOT/backups}"
mkdir -p "$BACKUP_DIR"
LOG_FILE="$BACKUP_DIR/restore.log"
require docker sha256sum

STOPPED=0
SWAPPED=0
SAFETY=""

cleanup() {
  local code=$?
  trap - ERR
  compose exec -T postgres rm -f /tmp/diary-restore.dump >/dev/null 2>&1 || true
  if ((code != 0)); then
    log "ВОССТАНОВЛЕНИЕ НЕ ЗАВЕРШЕНО"
    if ((SWAPPED)); then
      log "база уже заменена копией, а файлы вложений могли восстановиться не полностью"
    elif ((STOPPED)); then
      log "рабочая база не изменялась"
      compose exec -T postgres sh -c 'psql -X -q -U "$POSTGRES_USER" -d postgres -c "DROP DATABASE IF EXISTS \"${POSTGRES_DB}_restore\""' >/dev/null 2>&1 || true
    fi
    [[ -z "$SAFETY" ]] || log "состояние до восстановления сохранено в $SAFETY — его можно вернуть: scripts/restore.sh $SAFETY"
  fi
  if ((STOPPED)); then
    log "запуск приложения"
    compose start backend worker nginx >/dev/null 2>&1 || log "ОШИБКА: не удалось запустить приложение — выполните: docker compose start"
  fi
  if ((code != 0)); then
    record_event restore failure "{\"reason\": \"$(json_escape "${LAST_ERROR:-код выхода $code}")\", \"name\": \"$(json_escape "$(basename "$SOURCE")")\"}"
  fi
  return $code
}
on_error() {
  LAST_ERROR="шаг «$STEP», код $1"
  log "ОШИБКА восстановления: $LAST_ERROR"
}
trap 'on_error "$?"' ERR
trap cleanup EXIT

step "проверка копии $SOURCE"
for file in manifest.json SHA256SUMS database.dump files.tgz; do
  [[ -f "$SOURCE/$file" ]] || die "в копии нет файла $file — это не каталог scripts/backup.sh или копия неполная"
done
(cd "$SOURCE" && sha256sum -c --quiet SHA256SUMS) || die "контрольные суммы не совпадают — копия повреждена"

# manifest_value КАТАЛОГ_КОПИИ КЛЮЧ — значение верхнего уровня из manifest.json (без зависимости от jq).
manifest_value() {
  sed -n "s/^ *\"$2\": *\"\{0,1\}\([^\",]*\)\"\{0,1\},\{0,1\}$/\1/p" "$1/manifest.json" | head -n 1
}
BACKUP_SCHEMA="$(manifest_value "$SOURCE" schema_version)"
BACKUP_CREATED="$(manifest_value "$SOURCE" created_at)"
BACKUP_PROJECT="$(manifest_value "$SOURCE" project)"
[[ "$BACKUP_SCHEMA" =~ ^[0-9]+$ ]] || die "в manifest.json нет корректной schema_version"
LATEST_SCHEMA=0
for migration in "$ROOT"/db/migrations/*.up.sql; do
  number="$(basename "$migration")"
  number=$((10#${number%%_*}))
  if ((number > LATEST_SCHEMA)); then LATEST_SCHEMA=$number; fi
done
((BACKUP_SCHEMA <= LATEST_SCHEMA)) ||
  die "схема копии ($BACKUP_SCHEMA) новее, чем знает этот код ($LATEST_SCHEMA): сначала обновите код до версии, с которой сделана копия"

compose ps --status running --services | grep -qx postgres || die "контейнер postgres не запущен: docker compose up -d postgres"

cat >&2 <<EOF

  Восстановление заменит ВСЕ текущие данные дневника (база и файлы вложений) содержимым копии.
    копия:          $(basename "$SOURCE")
    создана (UTC):  ${BACKUP_CREATED:-неизвестно}
    проект копии:   ${BACKUP_PROJECT:-неизвестно}, текущий проект: $PROJECT
    схема копии:    $BACKUP_SCHEMA, схема кода: $LATEST_SCHEMA
  Всё, что изменено после создания копии, будет потеряно. Перед восстановлением
  будет сделана страховочная копия текущего состояния (метка pre-restore).

EOF
if ((!ASSUME_YES)); then
  [[ -t 0 ]] || die "нет терминала для подтверждения: запустите интерактивно или с --yes"
  read -r -p "Для продолжения введите YES: " answer
  [[ "$answer" == "YES" ]] || die "восстановление отменено пользователем"
fi
log "начало восстановления из $(basename "$SOURCE") (проект $PROJECT)"

step "страховочная копия текущего состояния"
SAFETY="$(bash "$ROOT/scripts/backup.sh" --label pre-restore)" || die "страховочная копия не создана — восстановление отменено"

step "остановка nginx, worker, backend"
STOPPED=1
compose stop nginx worker backend >/dev/null

step "восстановление базы данных"
compose cp "$(host_path "$SOURCE")/database.dump" postgres:/tmp/diary-restore.dump >/dev/null
# Временная БД: пока pg_restore не завершился успешно, рабочая база остаётся нетронутой.
compose exec -T postgres sh -eu -c '
  admin() { psql -X -q -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d postgres "$@"; }
  admin -c "DROP DATABASE IF EXISTS \"${POSTGRES_DB}_restore\""
  admin -c "CREATE DATABASE \"${POSTGRES_DB}_restore\""
  pg_restore -U "$POSTGRES_USER" -d "${POSTGRES_DB}_restore" --no-owner --exit-on-error --single-transaction /tmp/diary-restore.dump
'
step "подмена рабочей базы"
compose exec -T postgres sh -eu -c '
  admin() { psql -X -q -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d postgres "$@"; }
  admin -c "DROP DATABASE IF EXISTS \"${POSTGRES_DB}_previous\""
  admin -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '\''$POSTGRES_DB'\'' AND pid <> pg_backend_pid()" >/dev/null
  admin -c "ALTER DATABASE \"$POSTGRES_DB\" RENAME TO \"${POSTGRES_DB}_previous\""
  admin -c "ALTER DATABASE \"${POSTGRES_DB}_restore\" RENAME TO \"$POSTGRES_DB\""
  admin -c "DROP DATABASE \"${POSTGRES_DB}_previous\""
'
SWAPPED=1
# Сессии из копии могли быть отозваны уже после её создания (например, украденная): завершаем все, вход заново.
db_psql -c 'UPDATE sessions SET revoked_at = now() WHERE revoked_at IS NULL'

step "восстановление файлов вложений"
# 65532 — пользователь nonroot в образе backend: файлы должны остаться доступны приложению на запись.
docker run --rm -v "$FILES_VOLUME:/data" -v "$(host_path "$SOURCE"):/backup:ro" "$HELPER_IMAGE" sh -eu -c '
  find /data -mindepth 1 -delete
  tar -xzf /backup/files.tgz -C /data
  chown -R 65532:65532 /data
'

step "запуск приложения"
compose start backend worker nginx >/dev/null
STOPPED=0
wait_ready 120 || die "backend не стал готов за 120 секунд — смотрите docker compose logs backend"

# Событие о страховочной копии записывалось в прежнюю базу, которую заменила копия: повторяем его, иначе
# страница «Система» покажет устаревшую дату последней резервной копии.
record_event backup success "{\"name\": \"$(json_escape "$(basename "$SAFETY")")\", \"schema_version\": $(manifest_value "$SAFETY" schema_version), \"consistent\": false}"
record_event restore success "{\"name\": \"$(json_escape "$(basename "$SOURCE")")\", \"schema_version\": $BACKUP_SCHEMA, \"safety_backup\": \"$(json_escape "$(basename "$SAFETY")")\"}"
log "восстановление успешно завершено"
cat >&2 <<EOF

  Проверьте после восстановления:
    1. Вход администратора и системного администратора (все сессии завершены — входят заново все пользователи).
    2. Страница «Система»: сервисы в норме, в «Эксплуатации» есть запись о восстановлении.
    3. Журнал и расписание класса за последние дни до даты копии; открывается любое вложение.
    4. Пароли, сменённые после даты копии, снова прежние: предупредите пользователей, при утечке — сбросьте пароли.
    5. Страховочная копия $(basename "$SAFETY") хранится в $BACKUP_DIR — удалите её, когда убедитесь в результате.
EOF
