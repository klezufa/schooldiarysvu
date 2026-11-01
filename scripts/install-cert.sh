#!/usr/bin/env bash
# Установка сертификата для профиля https: проверка пары сертификат/ключ, замена файлов в TLS_DIR, проверка
# конфигурации и перезагрузка Nginx без остановки. Подходит и для сертификата внутреннего центра сертификации,
# и для публичного (его вызывает scripts/renew-cert.sh).
#
#   scripts/install-cert.sh FULLCHAIN.pem PRIVKEY.pem [--hostname dnevnik.school.example]
#
#   FULLCHAIN   сертификат сервера и промежуточные сертификаты (без корневого), PEM
#   PRIVKEY     закрытый ключ, PEM; после установки — права 600, владелец uid 101 (nginx в образе)
#   --hostname  имя, по которому открывают дневник: сертификат обязан его содержать (по умолчанию DIARY_HOSTNAME)
set -Eeuo pipefail
umask 077
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

CERT=""
KEY=""
HOSTNAME_CHECK="$(env_value DIARY_HOSTNAME)"
while (($#)); do
  case "$1" in
    --hostname) HOSTNAME_CHECK="${2:?укажите имя}"; shift ;;
    -h|--help) usage; exit 0 ;;
    -*) die "неизвестный параметр $1" ;;
    *)
      if [[ -z "$CERT" ]]; then CERT="$1"
      elif [[ -z "$KEY" ]]; then KEY="$1"
      else die "лишний аргумент $1"; fi ;;
  esac
  shift
done
[[ -n "$CERT" && -n "$KEY" ]] || die "укажите файлы: scripts/install-cert.sh fullchain.pem privkey.pem"
[[ -f "$CERT" ]] || die "файл сертификата $CERT не найден"
[[ -f "$KEY" ]] || die "файл ключа $KEY не найден"
command -v openssl >/dev/null 2>&1 || die "не найдена команда openssl"

step "проверка сертификата $CERT"
openssl x509 -in "$CERT" -noout >/dev/null 2>&1 || die "$CERT — не сертификат в формате PEM"
openssl pkey -in "$KEY" -noout >/dev/null 2>&1 || die "$KEY — не закрытый ключ в формате PEM (или ключ зашифрован паролем)"
openssl x509 -in "$CERT" -noout -checkend 0 >/dev/null || die "срок действия сертификата уже истёк"
CERT_PUB="$(openssl x509 -in "$CERT" -noout -pubkey | openssl sha256)"
KEY_PUB="$(openssl pkey -in "$KEY" -pubout | openssl sha256)"
[[ "$CERT_PUB" == "$KEY_PUB" ]] || die "ключ не соответствует сертификату — проверьте, что файлы из одного выпуска"
if [[ -n "$HOSTNAME_CHECK" ]]; then
  openssl x509 -in "$CERT" -noout -checkhost "$HOSTNAME_CHECK" | grep -q 'does match' ||
    die "сертификат не содержит имя $HOSTNAME_CHECK — браузеры покажут ошибку"
fi
if ! openssl x509 -in "$CERT" -noout -checkend $((21 * 86400)) >/dev/null; then
  log "предупреждение: сертификат истекает менее чем через 21 день"
fi
CHAIN_LENGTH="$(grep -c 'BEGIN CERTIFICATE' "$CERT")"
if ((CHAIN_LENGTH == 1)) &&
  [[ "$(openssl x509 -in "$CERT" -noout -subject)" != "subject=$(openssl x509 -in "$CERT" -noout -issuer | cut -d= -f2-)" ]]; then
  log "в $CERT только сертификат сервера: это верно, если его подписал сам корневой ЦС; если выпускал промежуточный ЦС — допишите его сертификат после серверного, иначе часть устройств не доверит серверу"
fi

TLS_DIR="$(tls_dir)"
FULLCHAIN="$TLS_DIR/fullchain.pem"
PRIVKEY="$TLS_DIR/privkey.pem"
mkdir -p "$TLS_DIR"
for target in "$FULLCHAIN" "$PRIVKEY"; do
  [[ ! -d "$target" ]] || die "$target — каталог: его создал Docker, когда контейнер запускался без сертификата. Остановите nginx, удалите каталог и повторите"
done

step "установка в $TLS_DIR"
# Compose монтирует файлы по отдельности. Замена через mv сменила бы inode, и контейнеры продолжили бы видеть
# старый файл даже после перезагрузки Nginx, поэтому содержимое перезаписывается на месте; прежнее — в *.prev.
HAD_PREVIOUS=0
if [[ -f "$FULLCHAIN" && -f "$PRIVKEY" ]]; then
  cp -p "$FULLCHAIN" "$FULLCHAIN.prev"
  cp -p "$PRIVKEY" "$PRIVKEY.prev"
  HAD_PREVIOUS=1
fi
rollback() {
  ((HAD_PREVIOUS)) || return 0
  cat "$FULLCHAIN.prev" >"$FULLCHAIN"
  cat "$PRIVKEY.prev" >"$PRIVKEY"
  log "возвращён прежний сертификат"
}
cat "$CERT" >"$FULLCHAIN"
cat "$KEY" >"$PRIVKEY"
chmod 644 "$FULLCHAIN"
chmod 600 "$PRIVKEY"
if [[ "$(id -u)" == 0 ]]; then
  chown 101:101 "$PRIVKEY"
elif [[ "$(uname -s)" == Linux && "$(stat -c %u "$PRIVKEY")" != 101 ]]; then
  log "предупреждение: владелец $PRIVKEY не uid 101 — Nginx не прочитает ключ; выполните: sudo chown 101:101 $PRIVKEY"
fi

if compose ps --status running --services 2>/dev/null | grep -qx nginx; then
  step "проверка конфигурации и перезагрузка Nginx"
  if ! compose exec -T nginx nginx -t >/dev/null 2>&1; then
    rollback
    die "Nginx не принял новый сертификат (docker compose exec nginx nginx -t покажет причину)"
  fi
  compose exec -T nginx nginx -s reload >/dev/null 2>&1 || die "Nginx не перезагрузился: docker compose logs nginx"
  log "Nginx перезагружен без остановки"
else
  log "Nginx не запущен — сертификат применится при запуске (docker compose -f docker-compose.yml -f docker-compose.https.yml up -d)"
fi
log "установлен сертификат: $(openssl x509 -in "$FULLCHAIN" -noout -subject), действует до $(openssl x509 -in "$FULLCHAIN" -noout -enddate | cut -d= -f2)"
