# HTTPS: сертификаты, переход и HSTS

Профиль `https` (`docker-compose.https.yml`): Nginx принимает TLS 1.2 и 1.3 на порту 443, порт 80 только
перенаправляет на HTTPS (код 301), cookie сессии получают флаг `Secure`. Backend отказывается запускаться
в профиле `https` с `SESSION_COOKIE_SECURE=false` — ошибиться и получить «HTTPS без защиты cookie» нельзя.

Сертификат лежит в `TLS_DIR` (по умолчанию `deploy/tls`, на сервере — `/etc/schooldiary/tls`):

| Файл | Содержимое | Права |
| --- | --- | --- |
| `fullchain.pem` | сертификат сервера, затем промежуточные (корневой — не нужен) | 644 |
| `privkey.pem` | закрытый ключ без пароля | 600, владелец uid 101 (Nginx в образе) |

Устанавливайте сертификат только через `scripts/install-cert.sh`: он проверяет, что ключ подходит к сертификату,
сертификат действует и содержит имя дневника, перезаписывает файлы на месте (Compose монтирует их по одному,
и замена через `mv` осталась бы невидимой для контейнера), проверяет конфигурацию Nginx и перезагружает его без
остановки. При ошибке проверки возвращается прежний сертификат (`*.prev`).

## Какой способ выбрать

| | Внутренний центр сертификации | Публичный сертификат (Let's Encrypt) |
| --- | --- | --- |
| Имя | любое внутреннее, например `dnevnik.svu` | имя в домене школы, например `dnevnik.school.example` |
| Устройства | только управляемые: корневой сертификат ставится через MDM / групповые политики | любые, включая личные телефоны родителей и учеников |
| Что нужно | безопасное хранение корневого ключа, выпуск раз в год | домен и DNS-провайдер с API |
| Продление | вручную, раз в 397 дней | автоматически, `scripts/renew-cert.sh` по cron |

Публичный сертификат на чисто внутреннее имя (`.svu`, `.local`, `.lan`) получить нельзя. Открывать дневник
по IP-адресу или «соглашаться с предупреждением» нельзя ни в каком варианте: пользователь, приученный нажимать
«Всё равно перейти», не заметит настоящую подмену сервера.

## Вариант 1. Внутренний центр сертификации

Если в школе уже есть ЦС (Active Directory Certificate Services и т. п.), выпустите в нём серверный сертификат
по запросу из шага 2 с назначением «Проверка подлинности сервера» и именем дневника в SAN, и переходите к шагу 4.

### 1. Корневой ЦС (один раз, не на сервере дневника)

Корневой ключ создаётся и хранится **вне сервера** — на отдельном компьютере без доступа в сеть или на
зашифрованном носителе в сейфе. Кто владеет этим ключом, может выдать себя за любой сайт из списка разрешённых
имён на всех школьных устройствах.

```bash
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-384 \
  -keyout school-root.key -out school-root.crt -days 3650 \
  -subj "/CN=School Diary Root CA" \
  -addext "basicConstraints=critical,CA:TRUE,pathlen:0" \
  -addext "keyUsage=critical,keyCertSign,cRLSign" \
  -addext "nameConstraints=critical,permitted;DNS:dnevnik.svu"
```

OpenSSL спросит пароль ключа — придумайте длинный и храните отдельно от носителя. `nameConstraints`
ограничивает ЦС именем дневника: даже при утечке ключа им нельзя подделать сертификат банка или почты.
Для нескольких школьных сервисов перечислите их: `permitted;DNS:dnevnik.svu,permitted;DNS:wiki.svu`.

`school-root.crt` (не ключ!) распространите на управляемые устройства: групповая политика «Доверенные корневые
центры сертификации» для Windows, профиль конфигурации MDM для iPad/macOS/Android.

### 2. Запрос на сервере дневника

Закрытый ключ сервера создаётся на самом сервере и никуда не передаётся:

```bash
sudo install -d -m 700 /etc/schooldiary/tls-request && cd /etc/schooldiary/tls-request
sudo openssl req -new -newkey ec -pkeyopt ec_paramgen_curve:P-256 -noenc \
  -keyout privkey.pem -out request.csr \
  -subj "/CN=dnevnik.svu" -addext "subjectAltName=DNS:dnevnik.svu"
```

На компьютер с корневым ЦС переносится только `request.csr`.

### 3. Подпись запроса

```bash
cat > server.ext <<'EOF'
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature
extendedKeyUsage=serverAuth
subjectAltName=DNS:dnevnik.svu
EOF
openssl x509 -req -in request.csr -CA school-root.crt -CAkey school-root.key -CAcreateserial \
  -days 397 -sha256 -extfile server.ext -out fullchain.pem
openssl verify -CAfile school-root.crt fullchain.pem    # fullchain.pem: OK
```

397 дней — предел, который принимают все современные браузеры и устройства Apple. Имя в `subjectAltName`
обязательно: поле CN браузеры давно не проверяют.

### 4. Установка

Перенесите `fullchain.pem` на сервер (это публичный файл) и установите:

```bash
cd /opt/schooldiary
sudo bash scripts/install-cert.sh /etc/schooldiary/tls-request/fullchain.pem /etc/schooldiary/tls-request/privkey.pem --hostname dnevnik.svu
```

Поставьте в календарь напоминание о продлении за месяц до окончания; страница «Система» предупредит за 21 день,
`scripts/check-cert.sh` в мониторинге — тоже.

## Вариант 2. Публичный сертификат через DNS-01

Let's Encrypt проверяет владение доменом по TXT-записи `_acme-challenge.<имя>` в публичной DNS-зоне.
Серверу не нужен входящий доступ из интернета, а адрес дневника в публичной зоне можно вообще не публиковать:
внутри школы имя направляется на локальный IP через AdGuard Home ([adguard-home.md](adguard-home.md)).

Клиент — [lego](https://go-acme.github.io/lego/) в контейнере `goacme/lego:v5.5.2`; он поддерживает больше
150 DNS-провайдеров, в том числе `yandexcloud`, `selectelv2`, `regru`, `nicru`, `cloudru`, `timewebcloud`,
`beget`, `cloudflare`. Переменные доступа своего провайдера смотрите командой
`sudo docker run --rm goacme/lego:v5.5.2 dnshelp -c <код>`.

### 1. Ключ API

Создайте у DNS-провайдера ключ с **минимальными** правами: изменение записей только в зоне школы (у Cloudflare —
API Token с правом `Zone.DNS:Edit` на одну зону, у Yandex Cloud — сервисный аккаунт с ролью `dns.editor`
на одну папку). Ключ с полным доступом к аккаунту — это доступ ко всем доменам школы.

### 2. Настройки lego

```bash
sudo install -d -m 700 /etc/schooldiary
sudo install -m 600 /dev/null /etc/schooldiary/lego.env
sudo tee /etc/schooldiary/lego.env >/dev/null <<'EOF'
LEGO_EMAIL=it@school.example
LEGO_DOMAINS=dnevnik.school.example
LEGO_DNS=cloudflare
CF_DNS_API_TOKEN=<токен>
EOF
```

Файл вне репозитория и с правами 600: `renew-cert.sh` откажется работать с файлом, который могут читать другие.
На адрес `LEGO_EMAIL` Let's Encrypt пишет о проблемах с сертификатом — укажите общий ящик ИТ-службы.

### 3. Проверка на тестовом сервере Let's Encrypt

```bash
cd /opt/schooldiary
sudo LEGO_SERVER=https://acme-staging-v02.api.letsencrypt.org/directory LEGO_DATA=/var/lib/schooldiary/lego-staging \
  bash scripts/renew-cert.sh
```

Итог — «тестовый сертификат получен … настройка верна». Такой сертификат браузеры не примут, поэтому скрипт
его не устанавливает; важно, что lego создал TXT-запись и получил ответ. Ошибки на этом шаге не тратят лимиты
рабочего сервера Let's Encrypt (5 неудачных проверок в час на имя).

### 4. Рабочий сертификат и продление

```bash
sudo bash scripts/renew-cert.sh
```

Сертификат действует 90 дней. Задание cron из [installation.md](installation.md#7-расписание-обслуживания)
запускает `renew-cert.sh` ежедневно: lego продлевает сертификат, когда до окончания остаётся меньше 30 дней
(`RENEW_DAYS`), а Nginx перезагружается, только если сертификат действительно сменился.

## Переход с local-http на https

1. Имя дневника разрешается во внутренний IP на всех устройствах (`nslookup dnevnik.school.example`) —
   [adguard-home.md](adguard-home.md).
2. Сертификат установлен (`sudo bash scripts/check-cert.sh` — «в порядке»).
3. В `.env`: `COMPOSE_FILE=docker-compose.yml:docker-compose.https.yml`; затем `sudo docker compose up -d`.
   Все пользователи один раз войдут заново: прежние cookie без флага `Secure` не подходят.
4. Проверки: `http://` → 301 на `https://`; вход и выход; скачивание вложения; на «Системе» профиль `https` и срок
   сертификата; `sudo bash scripts/check-cert.sh --host dnevnik.school.example:443` — «в порядке».

Откат: уберите строку `COMPOSE_FILE` из `.env` и выполните `sudo docker compose up -d` — вернётся профиль
`local-http`. Если HSTS уже включён, откат для браузеров, запомнивших его, невозможен — см. ниже.

## HSTS

Заголовок `Strict-Transport-Security` заставляет браузер открывать сайт только по HTTPS и **запрещает**
продолжить при ошибке сертификата. Защищает от подмены через HTTP, но любая проблема с сертификатом
(истёк, не продлился, новое устройство без корневого ЦС) делает дневник недоступным без обходного пути на весь
срок `max-age`, который браузер помнит.

По умолчанию HSTS выключен. Включайте только когда:

- HTTPS работает без предупреждений на всех типах устройств школы не меньше двух недель;
- продление сертификата автоматическое (вариант 2) или закреплено за ответственным (вариант 1);
- мониторинг (`check-cert.sh`, страница «Система») проверяется регулярно.

Порядок: раскомментируйте в `deploy/nginx/profile-headers-https.conf` строку с `max-age=86400`,
`sudo docker compose up -d nginx`, неделя наблюдения; затем `max-age=31536000`. Не добавляйте
`includeSubDomains` и `preload`: они распространяют требование на весь домен школы, включая сайты, которые
дневник не контролирует, а исключение из preload-списка браузеров занимает месяцы.

Отключить HSTS можно только отдав `max-age=0` по **работающему** HTTPS: браузеры, которые не успели получить
новый заголовок, будут соблюдать старый срок.
