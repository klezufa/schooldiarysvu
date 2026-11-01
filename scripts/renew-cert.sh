#!/usr/bin/env bash
# Выпуск и продление публичного сертификата через ACME (Let's Encrypt) с проверкой DNS-01: серверу не нужен
# входящий доступ из интернета, нужен только API DNS-провайдера зоны. Клиент — lego в контейнере.
# Запускайте из cron раз в сутки: lego продлевает сертификат, только когда до окончания меньше RENEW_DAYS дней,
# а Nginx перезагружается, только если сертификат действительно сменился.
#
#   scripts/renew-cert.sh
#
# Переменные:
#   LEGO_ENV_FILE  файл настроек lego вне репозитория, права 600 (по умолчанию /etc/schooldiary/lego.env):
#                    LEGO_EMAIL=admin@school.example
#                    LEGO_DOMAINS=dnevnik.school.example
#                    LEGO_DNS=cloudflare            (код провайдера: docker run --rm goacme/lego:v5.5.2 dnshelp)
#                    CF_DNS_API_TOKEN=...           (переменные доступа выбранного провайдера)
#   LEGO_DATA      каталог учётной записи ACME и выпущенных сертификатов (по умолчанию /var/lib/schooldiary/lego)
#   LEGO_SERVER    другой ACME-сервер, например https://acme-staging-v02.api.letsencrypt.org/directory для проверки
#   RENEW_DAYS     за сколько дней до окончания продлевать (по умолчанию 30)
set -Eeuo pipefail
umask 077
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
  "") ;;
  *) die "неизвестный параметр $1" ;;
esac

LEGO_ENV_FILE="${LEGO_ENV_FILE:-/etc/schooldiary/lego.env}"
LEGO_DATA="${LEGO_DATA:-/var/lib/schooldiary/lego}"
LEGO_IMAGE="${LEGO_IMAGE:-goacme/lego:v5.5.2}"
RENEW_DAYS="${RENEW_DAYS:-30}"
CERT_NAME=diary
require docker openssl

step "проверка настроек $LEGO_ENV_FILE"
[[ -f "$LEGO_ENV_FILE" ]] || die "нет файла $LEGO_ENV_FILE — создайте его по образцу из docs/https.md"
if [[ "$(uname -s)" == Linux ]]; then
  MODE="$(stat -c %a "$LEGO_ENV_FILE")"
  [[ "$MODE" == 600 || "$MODE" == 400 ]] || die "права $LEGO_ENV_FILE — $MODE: в файле ключ API DNS, выполните chmod 600"
fi
for name in LEGO_EMAIL LEGO_DOMAINS LEGO_DNS; do
  grep -q "^$name=." "$LEGO_ENV_FILE" || die "в $LEGO_ENV_FILE не задан $name"
done
! grep -q '^LEGO_PATH=' "$LEGO_ENV_FILE" || die "уберите LEGO_PATH из $LEGO_ENV_FILE: каталог задаёт переменная LEGO_DATA"
[[ "$RENEW_DAYS" =~ ^[0-9]+$ ]] || die "RENEW_DAYS: нужно целое число"
mkdir -p "$LEGO_DATA"
chmod 700 "$LEGO_DATA"

step "запрос сертификата у ACME (lego run, продление при остатке меньше $RENEW_DAYS дней)"
ARGS=(run --accept-tos --path /acme --cert.name "$CERT_NAME" --renew-days "$RENEW_DAYS")
[[ -z "${LEGO_SERVER:-}" ]] || ARGS+=(--server "$LEGO_SERVER")
docker run --rm --env-file "$LEGO_ENV_FILE" -v "$(host_path "$LEGO_DATA"):/acme" "$LEGO_IMAGE" "${ARGS[@]}" ||
  die "lego завершился с ошибкой — причина в его выводе выше (чаще всего: ключ API, имя провайдера, домен не в этой зоне); установленный сертификат не изменён"

ISSUED_CERT="$LEGO_DATA/certificates/$CERT_NAME.crt"
ISSUED_KEY="$LEGO_DATA/certificates/$CERT_NAME.key"
[[ -f "$ISSUED_CERT" && -f "$ISSUED_KEY" ]] ||
  die "lego не сохранил $ISSUED_CERT и $ISSUED_KEY — содержимое каталога: $(find "$LEGO_DATA/certificates" -maxdepth 1 -type f -printf '%f ' 2>/dev/null)"
# Сертификату тестового сервера браузеры не доверяют: он только подтверждает, что настройка DNS-01 работает.
if [[ "${LEGO_SERVER:-}" == *staging* ]]; then
  log "тестовый сертификат получен: $ISSUED_CERT — настройка верна; он не устанавливается, запустите без LEGO_SERVER"
  exit 0
fi

TLS_DIR="$(tls_dir)"
if cmp -s "$ISSUED_CERT" "$TLS_DIR/fullchain.pem"; then
  log "сертификат не менялся — установка не нужна"
  exit 0
fi
step "установка нового сертификата"
bash "$ROOT/scripts/install-cert.sh" "$ISSUED_CERT" "$ISSUED_KEY"
log "сертификат обновлён"
