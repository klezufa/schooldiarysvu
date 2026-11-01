#!/usr/bin/env bash
# Проверка сертификата профиля https (для cron и мониторинга). Печатает одну строку состояния.
#
#   scripts/check-cert.sh [--days N] [--host АДРЕС:ПОРТ]
#
#   --days N   за сколько дней до окончания считать сертификат требующим продления (по умолчанию 21)
#   --host     дополнительно сверить сертификат, который реально отдаёт сервер, с файлом в TLS_DIR
#              (например dnevnik.school.example:443): расхождение значит, что Nginx не перезагружен
#
# Коды выхода: 0 — в порядке; 1 — истекает в ближайшие N дней или сервер отдаёт другой сертификат;
# 2 — файла нет, он повреждён, ключ не подходит или срок уже истёк.
set -Euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

DAYS=21
HOST=""
while (($#)); do
  case "$1" in
    --days) DAYS="${2:?укажите число дней}"; shift ;;
    --host) HOST="${2:?укажите адрес:порт}"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) printf 'неизвестный параметр %s\n' "$1" >&2; exit 2 ;;
  esac
  shift
done
[[ "$DAYS" =~ ^[0-9]+$ ]] || { printf '--days: нужно целое число\n' >&2; exit 2; }
command -v openssl >/dev/null 2>&1 || { printf 'ОШИБКА: не найдена команда openssl\n' >&2; exit 2; }

TLS_DIR="$(tls_dir)"
CERT="$TLS_DIR/fullchain.pem"
KEY="$TLS_DIR/privkey.pem"
fail() { printf 'ОШИБКА: %s\n' "$*"; exit 2; }

[[ -f "$CERT" ]] || fail "нет файла $CERT"
openssl x509 -in "$CERT" -noout >/dev/null 2>&1 || fail "$CERT повреждён или не в формате PEM"
# Ключ читается только при наличии прав (на сервере он принадлежит uid 101): без прав проверка пропускается.
if [[ -r "$KEY" ]]; then
  [[ "$(openssl x509 -in "$CERT" -noout -pubkey | openssl sha256)" == "$(openssl pkey -in "$KEY" -pubout 2>/dev/null | openssl sha256)" ]] ||
    fail "ключ $KEY не соответствует сертификату"
fi

END="$(openssl x509 -in "$CERT" -noout -enddate | cut -d= -f2)"
END_EPOCH="$(date -d "$END" +%s)"
LEFT=$(((END_EPOCH - $(date +%s)) / 86400))
NAMES="$(openssl x509 -in "$CERT" -noout -ext subjectAltName 2>/dev/null | tail -n +2 | sed -e 's/DNS://g' -e 's/^ *//' | paste -sd' ')"
SUMMARY="${NAMES:-без subjectAltName}, до $(date -u -d "$END" '+%Y-%m-%d'), осталось дней: $LEFT"

openssl x509 -in "$CERT" -noout -checkend 0 >/dev/null || fail "срок сертификата истёк ($SUMMARY)"

STATUS=0
if [[ -n "$HOST" ]]; then
  SERVER="${HOST%:*}"
  SERVED="$(openssl s_client -connect "$HOST" -servername "$SERVER" </dev/null 2>/dev/null | openssl x509 -noout -fingerprint -sha256 2>/dev/null || true)"
  if [[ -z "$SERVED" ]]; then
    printf 'ВНИМАНИЕ: %s не ответил по TLS; ' "$HOST"
    STATUS=1
  elif [[ "$SERVED" != "$(openssl x509 -in "$CERT" -noout -fingerprint -sha256)" ]]; then
    printf 'ВНИМАНИЕ: %s отдаёт другой сертификат — перезагрузите Nginx (scripts/install-cert.sh); ' "$HOST"
    STATUS=1
  fi
fi

if ! openssl x509 -in "$CERT" -noout -checkend $((DAYS * 86400)) >/dev/null; then
  printf 'ВНИМАНИЕ: сертификат скоро истечёт, продлите его: %s\n' "$SUMMARY"
  exit 1
fi
printf '%s: %s\n' "$( ((STATUS)) && echo 'сертификат в файле в порядке' || echo 'в порядке')" "$SUMMARY"
exit "$STATUS"
