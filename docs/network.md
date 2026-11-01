# Сеть и firewall

Дневник — внутренний сервис школы: из интернета он недоступен, внутри сети его видят только те сегменты,
которым он нужен. Наружу открыты только порты Nginx; PostgreSQL и backend не опубликованы на хост и доступны
лишь во внутренней сети Compose.

| Порт | Кто слушает | Назначение |
| --- | --- | --- |
| 443/tcp | Nginx | дневник (профиль `https`) |
| 80/tcp | Nginx | только перенаправление на HTTPS и `/health/*` для проверок |
| 22/tcp | sshd | администрирование, только из сегмента ИТ |

Профиль `local-http` (без `docker-compose.https.yml`) по умолчанию слушает только `127.0.0.1:8080`; открывать его
в школьную сеть (`BIND_ADDR=0.0.0.0`) допустимо лишь для пилота в изолированном сегменте — трафик и пароли
идут открытым текстом ([security.md](security.md)).

## Сегменты

Пример для школы с VLAN (адреса условные):

| Сегмент | Сеть | Доступ к дневнику |
| --- | --- | --- |
| Серверы | 192.168.10.0/24 | сервер дневника `192.168.10.20`, AdGuard Home |
| Администрация и учителя | 10.10.20.0/24 | да |
| Ученики (компьютерные классы, Wi-Fi) | 10.10.30.0/24 | по решению школы: ученикам и родителям дневник нужен для просмотра оценок |
| Гостевой Wi-Fi | 10.10.90.0/24 | нет |
| ИТ-администраторы | 10.10.10.0/24 | да, плюс SSH |

Главный контроль — ACL между VLAN на маршрутизаторе или L3-коммутаторе: разрешить нужным сегментам TCP 80/443
до `192.168.10.20`, остальное — запретить. Правила на самом сервере — второй рубеж.

## Firewall на сервере

**Docker публикует порты в обход ufw.** Опубликованные порты контейнеров обрабатываются в цепочке `FORWARD`
(после DNAT), а ufw фильтрует `INPUT` — правило `ufw deny 443` не закроет дневник. Поэтому:

1. `BIND_ADDR` в `.env` — IP сервера в сегменте серверов: Nginx не будет слушать другие интерфейсы
   (например, сеть управления или Docker-мосты).
2. SSH ограничивается ufw (это обычный хостовый порт):

   ```bash
   sudo ufw default deny incoming
   sudo ufw default allow outgoing
   sudo ufw allow from 10.10.10.0/24 to any port 22 proto tcp
   sudo ufw enable
   ```

3. Доступ к портам дневника — правилами в цепочке `DOCKER-USER`, которую Docker проверяет первой и не
   перезаписывает. Скрипт `/usr/local/sbin/schooldiary-firewall.sh` (замените интерфейс и сети на свои):

   ```bash
   #!/bin/sh
   set -eu
   IF=ens18   # внешний интерфейс сервера: ip -br addr
   iptables -F DOCKER-USER
   iptables -A DOCKER-USER -i "$IF" -m conntrack --ctstate RELATED,ESTABLISHED -j RETURN
   for net in 10.10.10.0/24 10.10.20.0/24 10.10.30.0/24; do
     iptables -A DOCKER-USER -i "$IF" -s "$net" -j RETURN
   done
   iptables -A DOCKER-USER -i "$IF" -j DROP
   iptables -A DOCKER-USER -j RETURN
   ```

   Правила `DOCKER-USER` не сохраняются после перезагрузки — применяйте их юнитом systemd после Docker,
   `/etc/systemd/system/schooldiary-firewall.service`:

   ```ini
   [Unit]
   Description=Firewall rules for School Diary published ports
   After=docker.service
   Requires=docker.service

   [Service]
   Type=oneshot
   RemainAfterExit=yes
   ExecStart=/usr/local/sbin/schooldiary-firewall.sh

   [Install]
   WantedBy=multi-user.target
   ```

   ```bash
   sudo chmod 700 /usr/local/sbin/schooldiary-firewall.sh
   sudo systemctl daemon-reload && sudo systemctl enable --now schooldiary-firewall
   ```

   Проверка: с компьютера гостевой сети `curl -m 5 https://dnevnik.school.example` завершается таймаутом,
   из сети учителей — открывается.

## Исходящий доступ

Серверу нужны: DNS (AdGuard Home), NTP, `download.docker.com` и реестр образов (установка, обновления), репозиторий
с кодом (обновления), а для публичного сертификата — `acme-v02.api.letsencrypt.org` и API DNS-провайдера.
Остальной исходящий трафик можно запретить на маршрутизаторе: приложение само никуда не обращается.

## Имя и сертификат

Имя дневника направляется на `192.168.10.20` через AdGuard Home — [adguard-home.md](adguard-home.md).
Сертификат и переход на HTTPS — [https.md](https.md). DNS не создаёт доверия к сертификату, а доступ по IP-адресу
без подходящего имени в сертификате вызывает предупреждение браузера — пользователей нельзя учить игнорировать его.
