# shellcheck shell=bash
# Переменные ниже читают подключающие скрипты; переменные в одинарных кавычках раскрываются внутри контейнера.
# shellcheck disable=SC2034,SC2016
# Общие функции скриптов эксплуатации (подключается через source). Linux-сервер с Docker Compose v2.24+.
# Настройки берутся из окружения (их удобно положить в /etc/default/schooldiary или экспортировать в cron):
#   COMPOSE_PROJECT_NAME  имя проекта Compose (по умолчанию schooldiarysvu — как в docker-compose.yml)
#   COMPOSE_FILES         файлы Compose через пробел; без неё берётся COMPOSE_FILE из .env (через двоеточие),
#                         иначе docker-compose.yml. Для https: "docker-compose.yml docker-compose.https.yml"
#   ENV_FILE              файл переменных Compose (по умолчанию .env в корне репозитория)

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT="${COMPOSE_PROJECT_NAME:-schooldiarysvu}"
COMPOSE_FILES="${COMPOSE_FILES:-}"
ENV_FILE="${ENV_FILE:-$ROOT/.env}"
FILES_VOLUME="${PROJECT}_files-data"
HELPER_IMAGE="${HELPER_IMAGE:-alpine:3.22}"
# Git Bash на Windows (только для проверки скриптов): не переписывать пути вида /tmp внутри аргументов docker.
export MSYS_NO_PATHCONV=1

# usage — справка: начальный комментарий вызвавшего скрипта до первой команды или директивы shellcheck.
usage() {
  awk 'NR == 1 { next } /^# shellcheck/ || !/^#/ { exit } { sub(/^# ?/, ""); print }' "$0"
}

log() {
  local line
  line="$(date '+%Y-%m-%d %H:%M:%S') $*"
  printf '%s\n' "$line" >&2
  if [[ -n "${LOG_FILE:-}" ]]; then printf '%s\n' "$line" >>"$LOG_FILE"; fi
}

LAST_ERROR=""
STEP="подготовка"
# step — текущий шаг: его название попадает в сообщение об ошибке и в журнал эксплуатации.
step() {
  STEP="$*"
  log "$*"
}

die() {
  LAST_ERROR="$*"
  log "ОШИБКА: $*"
  exit 1
}

compose() {
  local args=(-p "$PROJECT")
  [[ -f "$ENV_FILE" ]] && args+=(--env-file "$ENV_FILE")
  local file files="$COMPOSE_FILES"
  # COMPOSE_FILE в .env (через двоеточие) — тот же набор файлов, который видит обычная команда docker compose.
  [[ -n "$files" ]] || files="$(env_value COMPOSE_FILE | tr ':' ' ')"
  for file in ${files:-docker-compose.yml}; do args+=(-f "$ROOT/$file"); done
  docker compose "${args[@]}" "$@"
}

# Путь каталога хоста для -v. На Linux — как есть; в Git Bash переводится в C:/... для Docker Desktop.
host_path() {
  if command -v cygpath >/dev/null 2>&1; then cygpath -m "$1"; else printf '%s' "$1"; fi
}

# env_value KEY — значение из окружения, иначе из ENV_FILE (как его увидит Compose); пусто, если не задано.
env_value() {
  if [[ -n "${!1:-}" ]]; then printf '%s' "${!1}"; return; fi
  [[ -f "$ENV_FILE" ]] || return 0
  sed -n "s/^$1=//p" "$ENV_FILE" | tail -n 1 | sed -e 's/^"\(.*\)"$/\1/' -e "s/^'\(.*\)'\$/\1/" | tr -d '\r'
}

# Каталог сертификатов профиля https; относительный путь в .env Compose считает от корня проекта.
tls_dir() {
  local dir
  dir="$(env_value TLS_DIR)"
  dir="${dir:-./deploy/tls}"
  [[ "$dir" == /* ]] || dir="$ROOT/${dir#./}"
  host_path "$dir"
}

require() {
  local tool
  for tool in "$@"; do command -v "$tool" >/dev/null 2>&1 || die "не найдена команда $tool"; done
  docker info >/dev/null 2>&1 || die "Docker недоступен (служба не запущена или нет прав на сокет)"
}

# psql внутри контейнера postgres: учётные данные берутся из переменных самого контейнера, на хост они не попадают.
db_psql() {
  compose exec -T postgres sh -c 'psql -X -q -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" "$@"' psql "$@"
}

# record_event backup|restore success|failure '{"json": "details"}' — запись в журнал эксплуатации (страница «Система»).
# Ошибка записи не должна маскировать исход самой операции, поэтому она только логируется.
record_event() {
  local kind="$1" status="$2" details="${3:-}"
  [[ -n "$details" ]] || details='{}'
  if ! printf "INSERT INTO maintenance_events (kind, status, details) VALUES (:'kind', :'status', :'details'::jsonb);\n" |
    db_psql -v kind="$kind" -v status="$status" -v details="$details" >/dev/null 2>&1; then
    log "предупреждение: не удалось записать событие $kind/$status в БД (страница «Система» его не покажет)"
  fi
}

json_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/ }"
  printf '%s' "$value"
}

wait_ready() {
  local deadline=$((SECONDS + ${1:-90}))
  while ((SECONDS < deadline)); do
    if compose exec -T backend /app/healthcheck >/dev/null 2>&1; then return 0; fi
    sleep 2
  done
  return 1
}
