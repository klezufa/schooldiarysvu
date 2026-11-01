# Диагностика

Первые три команды при любой проблеме на сервере (из `/opt/schooldiary`):

```bash
sudo docker compose ps                         # какие сервисы не running (healthy)
sudo docker compose logs --since 30m backend   # ошибки backend, миграций, конфигурации
sudo bash scripts/check-cert.sh --host "$(grep ^DIARY_HOSTNAME= .env | cut -d= -f2):443"
```

Страница «Система» (вход системного администратора) показывает состояние сервисов, задержки и ошибки
запросов, последнюю резервную копию и срок сертификата. Проблемы Windows-установки —
[windows-local.md](windows-local.md#8-диагностика).

## Сервисы не запускаются

| Сообщение в `docker compose logs backend` | Причина и решение |
| --- | --- |
| `https profile requires secure session cookies` | профиль `https` с `SESSION_COOKIE_SECURE=false`: уберите эту переменную из `.env` (`docker-compose.https.yml` задаёт `true`) |
| `invalid SCHOOL_TIMEZONE` | имя часового пояса в формате IANA: `Europe/Moscow`, `Asia/Yekaterinburg` |
| `password authentication failed` | `POSTGRES_PASSWORD` в `.env` изменён после первого запуска; база помнит исходный пароль — верните его |
| `Dirty database version` | миграция прервалась (сбой питания, нехватка места). Не правьте `schema_migrations` вручную: восстановите копию `pre-update` или последнюю ночную ([backup-restore.md](backup-restore.md)) |

**Nginx: `not a directory` / `Are you trying to mount a directory onto a file`.** Контейнер запускался, когда
`fullchain.pem` или `privkey.pem` ещё не было, и Docker создал на их месте каталоги. Остановите nginx
(`sudo docker compose stop nginx`), удалите пустые каталоги в `TLS_DIR`, установите сертификат
(`scripts/install-cert.sh`) и запустите `sudo docker compose up -d nginx`.

**Nginx: `cannot load certificate key … Permission denied`.** Ключ должен принадлежать uid 101 (пользователь
Nginx в образе): `sudo chown 101:101 /etc/schooldiary/tls/privkey.pem && sudo chmod 600 /etc/schooldiary/tls/privkey.pem`.
`install-cert.sh`, запущенный через `sudo`, делает это сам.

**Не скачиваются образы (`docker compose build` / `pull` — таймаут к Docker Hub).** Проверьте исходящий доступ
сервера ([network.md](network.md#исходящий-доступ)); если Docker Hub недоступен из сети школы, укажите зеркало
реестра в `/etc/docker/daemon.json` (`"registry-mirrors": ["https://…"]`) и перезапустите Docker.

## Браузер предупреждает о сертификате

| Ошибка в Chrome/Edge | Что не так |
| --- | --- |
| `NET::ERR_CERT_COMMON_NAME_INVALID` | открыто имя, которого нет в сертификате (например, IP-адрес или другое имя) |
| `NET::ERR_CERT_AUTHORITY_INVALID` | на устройстве нет корневого сертификата школьного ЦС, или в `fullchain.pem` нет промежуточного сертификата |
| `NET::ERR_CERT_DATE_INVALID` | сертификат истёк (`check-cert.sh`) или на устройстве неверные дата и время |

`sudo bash scripts/check-cert.sh --host dnevnik.school.example:443` сравнивает сертификат, который реально отдаёт
сервер, с файлом: «отдаёт другой сертификат» — Nginx не перезагружен после замены (`scripts/install-cert.sh`
делает это сам). Цепочку, которую видит клиент, покажет
`openssl s_client -connect dnevnik.school.example:443 -servername dnevnik.school.example </dev/null`.

## Дневник не открывается по имени

`nslookup dnevnik.school.example` на устройстве пользователя должен вернуть внутренний IP сервера от DNS-сервера
AdGuard Home. Другой ответ — устройство идёт мимо AdGuard (DHCP, VPN, «безопасный DNS» браузера, iCloud
Private Relay): [adguard-home.md](adguard-home.md#2-устройства-должны-спрашивать-adguard). Верный адрес, но
таймаут — firewall между сегментами или `DOCKER-USER` ([network.md](network.md)).

## Вход

**«Слишком много попыток» (HTTP 429).** После 5 неверных паролей подряд вход под этим логином с этого адреса
блокируется на 15 минут; для кодов приглашений действует отдельный такой же счётчик. Сброс пароля
администратором снимает блокировку сразу. Адрес клиента backend берёт из `X-Real-IP` только от своего Nginx
(`TRUST_PROXY_HEADERS` в `docker-compose.yml`): если перед Nginx поставить ещё один прокси, все пользователи
окажутся «с одного адреса» — не делайте этого без доработки настройки доверенных прокси.

**«Сервер занят: много одновременных входов» (HTTP 503).** Входов одновременно больше, чем сервер хеширует паролей
(`PASSWORD_HASH_CONCURRENCY`, по умолчанию 2), и очередь не продвинулась за 10 секунд. Повторите вход через несколько
секунд. Если это повторяется каждое утро, а у сервера есть свободная память, увеличьте `PASSWORD_HASH_CONCURRENCY` в `.env`
(каждый слот — 64 МиБ) и, соответственно, `mem_limit` и `GOMEMLIMIT` backend в `docker-compose.yml`.

**Все пользователи разлогинены.** Ожидаемо после восстановления из копии (сессии завершаются) и после перехода
на `https` (старые cookie без флага `Secure` не подходят).

## Резервные копии

Причина последней неудачи — на «Системе» и в `BACKUP_DIR/backup.log`; в записи указан шаг, на котором произошла
ошибка.

| Сообщение | Решение |
| --- | --- |
| `контейнер postgres не запущен` | `sudo docker compose up -d postgres`, затем повторите копию |
| `миграции БД в незавершённом состоянии` | см. `Dirty database version` выше |
| `для BACKUP_REMOTE=… нужен rsync` | `sudo apt-get install rsync`; копия на сервере при этом уже создана |
| шаг «передача копии», `Permission denied (publickey)` | проверьте SSH-ключ root для `BACKUP_REMOTE` ([backup-restore.md](backup-restore.md#вторая-копия-на-другом-носителе)) |
| `No space left on device` | `df -h`, `sudo docker system df`; уменьшите `BACKUP_KEEP` или перенесите `BACKUP_DIR` на другой диск |

`restore.sh` отказывается:

- «контрольные суммы не совпадают» — копия повреждена при переносе; возьмите копию из другого места (`BACKUP_REMOTE`);
- «схема копии новее, чем знает этот код» — сначала обновите код до версии, которой сделана копия (`git_commit` в
  `manifest.json`);
- «нет терминала для подтверждения» — запуск без терминала (cron, ssh без `-t`): добавьте `--yes`, только если
  уверены в выборе копии.

## Место на диске

```bash
df -h /                                   # свободное место
sudo docker system df                     # образы, тома, кеш сборки
sudo docker builder prune --filter until=168h   # кеш сборки старше недели
sudo du -sh /var/backups/schooldiary      # копии
```

Журналы контейнеров ограничены (10 МБ × 5 файлов на сервис). Не удаляйте тома `postgres-data` и `files-data`
командой `docker volume prune` или `docker compose down -v`: это и есть данные дневника.
