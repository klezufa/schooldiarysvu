# Установка на Ubuntu Server без Docker

Дневник ставится прямо в систему: PostgreSQL 17 из официального репозитория PostgreSQL, два исполняемых файла
под отдельным пользователем и две службы systemd. Сервер сам отдаёт и интерфейс, и API, поэтому для пилота
по HTTP Nginx не нужен. Проверено на Ubuntu Server 24.04 LTS.

**Когда выбирать этот способ.** Когда Docker на школьном сервере ставить нельзя или не хочется: меньше слоёв,
привычные `systemctl` и `journalctl`, PostgreSQL как обычная служба. Цена — сборка и обновление вручную:
нужны Go и Node.js, а изоляцию, которую давали контейнеры, заменяют настройки systemd. Если ограничений
нет, проще [установка в Docker](pilot.md).

Руководство написано для пилота по HTTP. Правила пилота, план, обратная связь и завершение — те же, что
в [pilot.md](pilot.md) (разделы 1, 2, 6, 8, 9, 12). Здесь — то, что отличается: установка, сеть, копии,
обслуживание.

| Что | Где |
| --- | --- |
| Код и сборка | `/opt/schooldiary` (владелец root) |
| Исполняемые файлы | `/opt/schooldiary/bin/server`, `worker`, `bootstrap-admin` |
| Интерфейс | `/opt/schooldiary/web/dist` |
| Настройки и пароль базы | `/etc/schooldiary/diary.env` (root:diary, 640) |
| Вложения | `/var/lib/schooldiary/files` (diary, 700) |
| База | PostgreSQL 17, база и роль `diary`, только `127.0.0.1` |
| Службы | `schooldiary-server` (порт 80), `schooldiary-worker` |
| Журналы | `journalctl -u schooldiary-server -u schooldiary-worker` |
| Копии | `/var/backups/schooldiary`, скрипт `scripts/native-backup.sh` |

## 1. Система

```bash
sudo apt-get update && sudo apt-get -y upgrade
sudo apt-get install -y ca-certificates curl git openssl rsync xz-utils
sudo timedatectl set-timezone Europe/Moscow
timedatectl status          # «System clock synchronized: yes»
```

Серверу на время установки нужен интернет: `github.com`, `apt.postgresql.org`, `go.dev`/`dl.google.com`,
`nodejs.org`, `registry.npmjs.org`, `proxy.golang.org`. После сборки дневник работает без интернета.

## 2. PostgreSQL 17

В Ubuntu 24.04 по умолчанию PostgreSQL 16; дневник проверяется на 17, поэтому ставим её из официального
репозитория PostgreSQL:

```bash
sudo apt-get install -y postgresql-common
sudo /usr/share/postgresql-common/pgdg/apt.postgresql.org.sh -y
sudo apt-get install -y postgresql-17
sudo systemctl enable --now postgresql
sudo -u postgres psql -c 'SELECT version();'      # PostgreSQL 17.x
```

По умолчанию PostgreSQL слушает только `localhost` и требует пароль для сетевых подключений
(`scram-sha-256`) — так и оставьте: снаружи базу никто не видит, дневник подключается к ней локально.

**База — пустая, с нуля.** Создаются только роль и база; таблицы дневник создаст сам при первом запуске
(миграции встроены в программу). Демо-данных и тестовых учётных записей нет.

```bash
DB_PASSWORD="$(openssl rand -hex 24)"     # только шестнадцатеричные символы: пароль войдёт в адрес базы
sudo -u postgres psql -v ON_ERROR_STOP=1 -v pw="$DB_PASSWORD" <<'SQL'
CREATE ROLE diary LOGIN PASSWORD :'pw';
CREATE DATABASE diary OWNER diary ENCODING 'UTF8' TEMPLATE template0;
SQL
sudo -u postgres psql -d diary -c 'CREATE EXTENSION IF NOT EXISTS pgcrypto;'
```

Не закрывайте терминал до шага 6: переменная `DB_PASSWORD` понадобится для файла настроек. Если закрыли —
задайте новый пароль: `sudo -u postgres psql -c "ALTER ROLE diary PASSWORD 'новый-hex-пароль'"`.

## 3. Инструменты сборки: Go и Node.js

Версии те же, что в образах Docker и CI. Пакеты Ubuntu для этого слишком старые, поэтому — официальные архивы:

```bash
cd /tmp
curl -fsSLO https://go.dev/dl/go1.26.10.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.26.10.linux-amd64.tar.gz
/usr/local/go/bin/go version        # go1.26.10

curl -fsSLO https://nodejs.org/dist/v24.14.0/node-v24.14.0-linux-x64.tar.xz
sudo rm -rf /usr/local/lib/nodejs && sudo mkdir -p /usr/local/lib/nodejs
sudo tar -C /usr/local/lib/nodejs --strip-components=1 -xJf node-v24.14.0-linux-x64.tar.xz
sudo ln -sf /usr/local/lib/nodejs/bin/node /usr/local/lib/nodejs/bin/npm /usr/local/lib/nodejs/bin/npx /usr/local/bin/
node --version && npm --version     # v24.14.0
```

Контрольные суммы архивов опубликованы рядом с ними (`go.dev/dl`, `nodejs.org/dist/v24.14.0/SHASUMS256.txt`) —
сверьте `sha256sum`, если в школе это принято. Сервер на ARM — замените `amd64`/`x64` на `arm64`.

## 4. Код и сборка

```bash
sudo git clone https://github.com/klezufa/schooldiarysvu.git /opt/schooldiary
cd /opt/schooldiary
sudo git checkout pilot-2                          # выпуск для пилота, не случайный коммит из main
```

Интерфейс (2–5 минут):

```bash
cd /opt/schooldiary/web
sudo npm ci
sudo npm run build                                 # результат — /opt/schooldiary/web/dist
```

Сервер, фоновый обработчик и утилита первого входа. Версия попадёт на страницу «Система»:

```bash
cd /opt/schooldiary
VERSION="$(sudo git describe --tags --always)"   # от root: репозиторий принадлежит root
for cmd in server worker bootstrap-admin; do
  sudo env CGO_ENABLED=0 /usr/local/go/bin/go build -trimpath \
    -ldflags="-s -w -X schooldiarysvu/internal/buildinfo.Version=$VERSION" -o bin/$cmd ./cmd/$cmd || break
done
ls -l bin/                                         # server, worker, bootstrap-admin
```

## 5. Пользователь и каталоги

Дневник работает от отдельного системного пользователя без входа в систему:

```bash
sudo useradd --system --home-dir /var/lib/schooldiary --shell /usr/sbin/nologin diary
sudo install -d -o diary -g diary -m 700 /var/lib/schooldiary /var/lib/schooldiary/files
sudo install -d -o root -g diary -m 750 /etc/schooldiary
```

## 6. Настройки

`192.168.10.20` замените на IP сервера в школьной сети:

```bash
sudo install -o root -g diary -m 640 /dev/null /etc/schooldiary/diary.env
sudo tee /etc/schooldiary/diary.env >/dev/null <<EOF
APP_ENV=production
# Профиль без шифрования — только для пилота (docs/pilot.md, раздел 1).
NETWORK_PROFILE=local-http
SESSION_COOKIE_SECURE=false
# Адрес и порт: только школьный интерфейс сервера, порт 80 — адрес без номера порта.
HTTP_ADDR=192.168.10.20:80
DATABASE_URL=postgres://diary:${DB_PASSWORD}@127.0.0.1:5432/diary?sslmode=disable
SCHOOL_TIMEZONE=Europe/Moscow
FILE_STORAGE_PATH=/var/lib/schooldiary/files
STATIC_DIR=/opt/schooldiary/web/dist
# Запросы приходят напрямую, без Nginx: адрес клиента берётся из соединения (важно для защиты от перебора).
TRUST_PROXY_HEADERS=false
EOF
sudo grep DATABASE_URL /etc/schooldiary/diary.env    # пароль подставился, а не пустой
unset DB_PASSWORD
```

## 7. Службы systemd

```bash
sudo tee /etc/systemd/system/schooldiary-server.service >/dev/null <<'EOF'
[Unit]
Description=School Diary server (API and interface)
After=network-online.target postgresql.service
Wants=network-online.target postgresql.service

[Service]
User=diary
Group=diary
EnvironmentFile=/etc/schooldiary/diary.env
ExecStart=/opt/schooldiary/bin/server
Restart=on-failure
RestartSec=5
# Порт 80 без прав root: только эта возможность и ничего больше.
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/var/lib/schooldiary
ProtectHome=true
PrivateTmp=true
PrivateDevices=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
MemoryMax=1G

[Install]
WantedBy=multi-user.target
EOF

sudo tee /etc/systemd/system/schooldiary-worker.service >/dev/null <<'EOF'
[Unit]
Description=School Diary background worker (notifications, digests, session cleanup)
After=schooldiary-server.service
Wants=schooldiary-server.service

[Service]
User=diary
Group=diary
EnvironmentFile=/etc/schooldiary/diary.env
ExecStart=/opt/schooldiary/bin/worker
Restart=on-failure
RestartSec=5
CapabilityBoundingSet=
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/var/lib/schooldiary
ProtectHome=true
PrivateTmp=true
PrivateDevices=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
MemoryMax=512M

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now schooldiary-server schooldiary-worker
systemctl status schooldiary-server schooldiary-worker --no-pager
curl -fsS http://192.168.10.20/health/ready ; echo           # {"status":"ready"}
```

При первом запуске сервер создаёт таблицы в пустой базе (миграции) и только потом начинает принимать
запросы. Если `status` показывает ошибку — `journalctl -u schooldiary-server -n 50 --no-pager` (раздел 12).

Проверьте перезагрузку один раз: `sudo reboot`, затем снова `curl …/health/ready`.

## 8. Первый вход

Системный администратор создаётся утилитой; пароль вводится без отображения:

```bash
read -rsp 'Пароль системного администратора (от 12 символов): ' BOOTSTRAP_ADMIN_PASSWORD; echo
export BOOTSTRAP_ADMIN_PASSWORD
sudo --preserve-env=BOOTSTRAP_ADMIN_PASSWORD -u diary bash -c \
  'set -a; . /etc/schooldiary/diary.env; set +a; BOOTSTRAP_ADMIN_LOGIN=sysadmin BOOTSTRAP_ADMIN_DISPLAY_NAME="Системный администратор" exec /opt/schooldiary/bin/bootstrap-admin'
unset BOOTSTRAP_ADMIN_PASSWORD
```

Дальше — как в [pilot.md](pilot.md), раздел 6: вход `http://192.168.10.20`, страница «Система», настройка
школы по шагам, выдача учётных записей.

## 9. Firewall

Без Docker обычный `ufw` закрывает и SSH, и дневник — отдельные правила `DOCKER-USER` не нужны:

```bash
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow from 10.10.10.0/24 to any port 22 proto tcp     # SSH — только сеть ИТ
for net in 10.10.10.0/24 10.10.20.0/24 10.10.30.0/24; do       # дневник — сети участников пилота
  sudo ufw allow from "$net" to any port 80 proto tcp
done
sudo ufw enable
sudo ufw status numbered
```

PostgreSQL слушает только `127.0.0.1`, открывать для него порт не нужно. Проверка: из сети учителей
дневник открывается, из гостевого Wi-Fi — нет.

## 10. Резервные копии

`scripts/native-backup.sh` делает копию базы (`pg_dump`) и вложений, считает контрольные суммы, хранит
последние `BACKUP_KEEP` копий, при необходимости отправляет копию на другой носитель и записывает итог
на страницу «Система» → «Эксплуатация».

```bash
sudo tee /etc/default/schooldiary >/dev/null <<'EOF'
BACKUP_DIR=/var/backups/schooldiary
BACKUP_KEEP=14
# Вторая копия на другом носителе: смонтированный каталог или user@host:/path (rsync по SSH-ключу root).
BACKUP_REMOTE=/mnt/backup/schooldiary
EOF
sudo chmod 600 /etc/default/schooldiary

sudo tee /etc/cron.d/schooldiary >/dev/null <<'EOF'
SHELL=/bin/bash
# Ежедневно в 02:30; --quiesce останавливает службы на время копии (10–60 секунд).
30 2 * * * root set -a; . /etc/default/schooldiary; /opt/schooldiary/scripts/native-backup.sh --quiesce >/dev/null 2>>/var/log/schooldiary-cron.log
EOF

# Первая копия — сразу, вручную:
sudo bash -c 'set -a; . /etc/default/schooldiary; /opt/schooldiary/scripts/native-backup.sh'
ls -l /var/backups/schooldiary
```

В каталоге копии: `database.dump`, `files.tar.gz`, `manifest.json` (время, версия схемы, выпуск) и `SHA256SUMS`.

### Восстановление

Делайте вне уроков. Всё, что внесено после выбранной копии, будет потеряно.

```bash
COPY=/var/backups/schooldiary/diary-ГГГГММДД-ЧЧММСС
(cd "$COPY" && sha256sum -c SHA256SUMS)                         # обе строки «OK», иначе — другая копия

# Страховочная копия текущего состояния, затем остановка дневника.
sudo bash -c 'set -a; . /etc/default/schooldiary; BACKUP_KEEP=1000 /opt/schooldiary/scripts/native-backup.sh --quiesce'
sudo systemctl stop schooldiary-worker schooldiary-server

# База: пересоздать пустую и загрузить копию от имени владельца diary (расширение pgcrypto создаст сам дамп).
sudo -u postgres dropdb diary
sudo -u postgres createdb -O diary -E UTF8 -T template0 diary
sudo -u postgres pg_restore --no-owner --role=diary -d diary "$COPY/database.dump"

# Вложения.
sudo rm -rf /var/lib/schooldiary/files
sudo tar -C /var/lib/schooldiary -xzf "$COPY/files.tar.gz"
sudo chown -R diary:diary /var/lib/schooldiary/files && sudo chmod 700 /var/lib/schooldiary/files

sudo systemctl start schooldiary-server schooldiary-worker
curl -fsS http://192.168.10.20/health/ready ; echo
```

Если копия сделана более старой версией дневника, сервер при запуске сам применит недостающие миграции.
Копию из более новой версии на старый код не восстанавливайте — сначала обновите код (раздел 11).
**Хотя бы раз за пилот проверьте восстановление** — на тестовой виртуальной машине или вне учебного времени.

## 11. Обновление

```bash
cd /opt/schooldiary
sudo bash -c 'set -a; . /etc/default/schooldiary; /opt/schooldiary/scripts/native-backup.sh --quiesce'
sudo git fetch --tags
sudo git checkout pilot-3                                     # новый выпуск
(cd web && sudo npm ci && sudo npm run build)
VERSION="$(sudo git describe --tags --always)"   # от root: репозиторий принадлежит root
for cmd in server worker bootstrap-admin; do
  sudo env CGO_ENABLED=0 /usr/local/go/bin/go build -trimpath \
    -ldflags="-s -w -X schooldiarysvu/internal/buildinfo.Version=$VERSION" -o bin/$cmd ./cmd/$cmd || break
done
sudo systemctl restart schooldiary-server schooldiary-worker
curl -fsS http://192.168.10.20/health/ready ; echo
```

Если новый выпуск требует другой версии Go или Node.js, это будет сказано в его описании; поставьте её по разделу 3.
**Откат** — прежний тег, пересборка и восстановление копии, сделанной перед обновлением (раздел 10):
миграции применяются только вперёд, и прежний код новую схему базы не примет.

## 12. Обслуживание и диагностика

```bash
systemctl status schooldiary-server schooldiary-worker postgresql --no-pager
journalctl -u schooldiary-server --since "1 hour ago" --no-pager       # запросы и ошибки (JSON, без паролей)
journalctl -u schooldiary-worker --since today --no-pager
journalctl -u schooldiary-server --since "1 day ago" --no-pager | grep КОД_ЗАПРОСА
sudo systemctl restart schooldiary-server schooldiary-worker
df -h / /var/lib/schooldiary /var/backups
```

| Симптом | Что проверить |
| --- | --- |
| `server` не стартует, в журнале `bind: permission denied` | в службе есть `AmbientCapabilities=CAP_NET_BIND_SERVICE`; после правки — `daemon-reload` |
| `bind: cannot assign requested address` | `HTTP_ADDR` не совпадает с IP сервера: `ip -br addr` |
| `password authentication failed for user "diary"` | пароль в `DATABASE_URL` и в базе разные: задайте новый (раздел 2) и поправьте `diary.env` |
| `permission denied to create extension "pgcrypto"` | выполните `CREATE EXTENSION` от `postgres` (раздел 2) |
| Ошибка про `STATIC_DIR` / `index.html` | интерфейс не собран: `cd web && sudo npm run build` |
| Открывается с сервера, но не у участников | `sudo ufw status numbered`, ACL на маршрутизаторе, сети участников в правилах |
| «Слишком много попыток» при входе | защита от перебора: подождать указанное время или сбросить пароль |
| Нет записи о копии на «Системе» | `/var/backups/schooldiary/backup.log`, `/var/log/schooldiary-cron.log` |

## 13. Переход на HTTPS

Для постоянной работы перед дневником ставится Nginx с сертификатом, а сервер переводится на локальный адрес:

1. Сертификат — внутренний центр сертификации или Let's Encrypt, см. [https.md](https.md).
2. `sudo apt-get install -y nginx`; за основу конфигурации — `deploy/nginx/https.conf`, `locations.conf`,
   `security-headers.conf` и `profile-headers-https.conf`. В них поменяйте `proxy_pass http://backend:8080` на
   `http://127.0.0.1:8080`, а корень статики — на `/opt/schooldiary/web/dist`.
3. В `diary.env`: `HTTP_ADDR=127.0.0.1:8080`, `NETWORK_PROFILE=https`, `SESSION_COOKIE_SECURE=true`,
   `TRUST_PROXY_HEADERS=true` (адрес клиента теперь приходит от Nginx), `TLS_CERT_FILE` — путь к сертификату;
   строку `STATIC_DIR` удалите — статику отдаёт Nginx. В службе сервера `AmbientCapabilities` больше не нужен.
4. `ufw`: открыть 443 (и 80 для перенаправления) вместо прежнего правила для 80.
5. Сбросить пароли участникам пилота и завершить их сеансы — [pilot.md](pilot.md), раздел 12.
