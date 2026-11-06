# Установка на сервер Linux

Руководство для ИТ-специалиста школы: от чистого сервера до работающего дневника с HTTPS, резервными копиями
и контролем сертификата. Проверено на Ubuntu Server 24.04 LTS; на Debian 12 отличается только адрес репозитория
Docker. Запуск на Windows 11 для пилота и разработки — [windows-local.md](windows-local.md).

## 1. Что понадобится

| | Минимум | Рекомендуется |
| --- | --- | --- |
| Процессор | 2 ядра | 4 ядра |
| Память | 4 ГБ | 8 ГБ |
| Диск | 20 ГБ SSD | 50 ГБ SSD + отдельный носитель для копий |
| Сеть | статический IP в школьной сети | отдельный сегмент для серверов ([network.md](network.md)) |

Заранее решите:

- **имя дневника** — например `dnevnik.school.example` (публичный домен школы) или `dnevnik.svu`
  (только внутреннее имя, тогда нужен внутренний центр сертификации — см. [https.md](https.md));
- **куда уходят резервные копии** — сетевое хранилище, второй диск или другой сервер, не тот же диск;
- **кто системный администратор** дневника — у него полный доступ к структуре школы и данным.

Серверу нужен исходящий доступ к `download.docker.com`, Docker Hub и репозиторию с кодом (для установки
и обновлений), а при публичном сертификате — к Let's Encrypt и API DNS-провайдера. Входящий доступ из
интернета не нужен.

## 2. Подготовка системы

```bash
sudo apt-get update && sudo apt-get -y upgrade
sudo apt-get install -y ca-certificates curl git openssl rsync
sudo timedatectl set-timezone Europe/Moscow   # часы сервера влияют на сроки сессий и сертификатов
timedatectl status                            # «System clock synchronized: yes»
```

Docker Engine и Compose — из официального репозитория Docker (в репозитории Ubuntu версия Compose может быть
старее 2.24, а профиль https требует 2.24+):

```bash
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}") stable" |
  sudo tee /etc/apt/sources.list.d/docker.list >/dev/null
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
docker compose version   # 2.24 или новее
```

Актуальная инструкция — <https://docs.docker.com/engine/install/ubuntu/>. Не добавляйте обычных пользователей
в группу `docker`: это равносильно правам root. Команды ниже выполняются через `sudo`.

## 3. Код и настройки

```bash
sudo git clone https://github.com/klezufa/schooldiarysvu.git /opt/schooldiary
cd /opt/schooldiary
sudo git checkout <тег выпуска>     # не разворачивайте случайный коммит из main
```

Если вы разворачиваете приватную копию репозитория, создайте для сервера в GitHub **deploy key только на чтение**, а не личный токен.

Файл `/opt/schooldiary/.env` (права 600, в git не попадает):

```bash
sudo install -m 600 /dev/null .env
sudo tee .env >/dev/null <<EOF
APP_ENV=production
# Только шестнадцатеричные символы: пароль подставляется в DATABASE_URL, а символы / + = @ ломают адрес.
POSTGRES_PASSWORD=$(openssl rand -hex 24)
SCHOOL_TIMEZONE=Europe/Moscow
# IP сервера в школьной сети: порты публикуются только на этом интерфейсе (docs/network.md).
BIND_ADDR=192.168.10.20
# Имя дневника: install-cert.sh проверяет, что сертификат его содержит.
DIARY_HOSTNAME=dnevnik.school.example
TLS_DIR=/etc/schooldiary/tls
# Профиль https: docker compose и скрипты scripts/*.sh берут этот набор файлов автоматически.
COMPOSE_FILE=docker-compose.yml:docker-compose.https.yml
EOF
```

Пароль PostgreSQL нигде не нужно вводить вручную: скрипты обращаются к базе изнутри контейнера. Не меняйте
его после первого запуска — база уже инициализирована с этим паролем.

## 4. Сертификат

Профиль `https` — режим для реальных данных. Получите сертификат одним из способов из [https.md](https.md):

- внутренний центр сертификации (школьные управляемые устройства):
  `sudo bash scripts/install-cert.sh /path/fullchain.pem /path/privkey.pem`;
- публичный сертификат Let's Encrypt через DNS-01: настройте `/etc/schooldiary/lego.env` и выполните
  `sudo bash scripts/renew-cert.sh`.

Сертификат устанавливается **до** первого запуска: если файла нет, Docker создаст на его месте пустой каталог,
и Nginx не запустится (как исправить — [troubleshooting.md](troubleshooting.md)).

## 5. Первый запуск

```bash
cd /opt/schooldiary
sudo docker compose config --quiet && echo "конфигурация в порядке"
sudo docker compose up -d --build
sudo docker compose ps     # postgres, backend, worker, nginx — running (healthy)
curl -fsS http://192.168.10.20/health/ready    # {"status":"ready"}; адрес — BIND_ADDR из .env
```

Backend при каждом запуске применяет недостающие миграции базы; отдельный шаг миграции не нужен.

## 6. Системный администратор

Первый вход создаётся одноразовой утилитой. Пароль вводится без отображения и не попадает в историю команд:

```bash
read -rsp 'Пароль системного администратора (от 12 символов): ' BOOTSTRAP_ADMIN_PASSWORD; echo
export BOOTSTRAP_ADMIN_PASSWORD
sudo -E docker compose run --rm --no-deps \
  -e BOOTSTRAP_ADMIN_LOGIN=sysadmin -e BOOTSTRAP_ADMIN_DISPLAY_NAME='Системный администратор' -e BOOTSTRAP_ADMIN_PASSWORD \
  --entrypoint /app/bootstrap-admin backend
unset BOOTSTRAP_ADMIN_PASSWORD
```

Утилита работает только пока в системе нет ни одного системного администратора; повторный запуск откажется
(«a system administrator already exists»). Дальше пользователи создаются в интерфейсе: системный администратор
назначает администраторов, администраторы — учителей, учеников и родителей (импорт списком, приглашения).

Демонстрационные данные (`seed`) в production не запускаются: утилита отказывается работать вне `APP_ENV=development`.

## 7. Расписание обслуживания

Настройки для cron — в `/etc/default/schooldiary`:

```bash
sudo tee /etc/default/schooldiary >/dev/null <<'EOF'
BACKUP_DIR=/var/backups/schooldiary
BACKUP_KEEP=14
# Вторая копия на другом носителе: каталог NAS (смонтированный) или user@host:/path (rsync по SSH-ключу root).
BACKUP_REMOTE=backup@nas.school.lan:/volume1/schooldiary
EOF
sudo chmod 600 /etc/default/schooldiary
```

Задания `/etc/cron.d/schooldiary`:

```bash
sudo tee /etc/cron.d/schooldiary >/dev/null <<'EOF'
SHELL=/bin/bash
# Ежедневная копия в 02:30. С --quiesce дневник недоступен 10–60 секунд, зато база и файлы строго согласованы.
30 2 * * * root set -a; . /etc/default/schooldiary; /opt/schooldiary/scripts/backup.sh --quiesce >/dev/null 2>>/var/log/schooldiary-cron.log
# Продление публичного сертификата (только для Let's Encrypt; lego продлевает за 30 дней до окончания).
45 3 * * * root set -a; . /etc/default/schooldiary; /opt/schooldiary/scripts/renew-cert.sh >>/var/log/schooldiary-cert.log 2>&1
EOF
sudo chmod +x /opt/schooldiary/scripts/*.sh
```

Итог каждой копии и восстановления виден системному администратору на странице **«Система» → «Эксплуатация»**:
дата последней копии, её размер, ошибка последней неудачной попытки и срок действия сертификата. Подробности —
[backup-restore.md](backup-restore.md).

## 8. Проверка после установки

1. `https://dnevnik.school.example` открывается без предупреждений на школьном компьютере и на телефоне.
2. `http://dnevnik.school.example` перенаправляет на HTTPS (код 301).
3. Вход системного администратора; страница «Система»: все сервисы в норме, профиль `https`, срок сертификата.
4. В инструментах разработчика браузера у cookie сессии есть флаги `Secure`, `HttpOnly`, `SameSite=Lax`.
5. Первая резервная копия вручную: `sudo bash -c 'set -a; . /etc/default/schooldiary; /opt/schooldiary/scripts/backup.sh'`,
   затем на «Системе» появилась запись о копии.
6. Пробное восстановление на отдельном тестовом сервере ([backup-restore.md](backup-restore.md#проверка-восстановления)).
7. Firewall: с гостевого Wi-Fi и из сегмента учеников (если доступа быть не должно) дневник не открывается
   ([network.md](network.md)).

Обновление версии — [update.md](update.md). Если что-то не работает — [troubleshooting.md](troubleshooting.md).
