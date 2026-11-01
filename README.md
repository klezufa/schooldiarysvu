<div align="center">

# Школьный дневник

**Электронный журнал и дневник для одной школы, который работает на собственном сервере в локальной сети**

[![CI](https://github.com/klezufa/schooldiarysvu/actions/workflows/ci.yml/badge.svg)](https://github.com/klezufa/schooldiarysvu/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-17-4169E1?logo=postgresql&logoColor=white)
![Docker](https://img.shields.io/badge/Docker_Compose-2.24+-2496ED?logo=docker&logoColor=white)

<img src="docs/screenshots/grades.png" alt="Оценки ученика: средний балл по предметам и динамика по неделям" width="900">

</div>

Учителя ведут журнал, оценки, посещаемость и домашние задания. Ученики и родители видят расписание, оценки
и задания. Администрация управляет пользователями, классами, расписанием и переходом в новый учебный год.
Данные хранятся у школы: никаких облачных сервисов, внешних аналитик и обязательного доступа в интернет.

## Возможности

| Роль | Что доступно |
| --- | --- |
| **Учитель** | журнал по уроку, по ученику и целиком; быстрый ввод оценок с клавиатуры; посещаемость одной кнопкой; работы с весами; домашние задания с вложениями; отчёты по своим классам; мобильный режим |
| **Классный руководитель** | сводка по классу: успеваемость, пропуски, незаполненные уроки |
| **Ученик** | главная с ближайшими заданиями и уроками, расписание, оценки с графиками, отметка «сделано» у заданий |
| **Родитель** | то же по каждому ребёнку с переключением между детьми |
| **Администратор** | пользователи и роли, импорт из CSV, приглашения, зачисление и перевод классов, назначения учителей, шаблон расписания с переносом в новую четверть и учебный год, аудит изменений |
| **Системный администратор** | учебные годы и периоды, закрытие периодов, шкалы оценок, страница «Система»: нагрузка, сервисы, резервные копии, сертификат, место на диске |

Изменения журнала сохраняются с проверкой версии: два учителя не перезапишут оценки друг друга незаметно.
Важные действия попадают в журнал аудита в той же транзакции, что и само изменение.

## Скриншоты

<table>
  <tr>
    <td width="50%"><img src="docs/screenshots/journal.png" alt="Журнал учителя"><p align="center"><b>Журнал</b> — оценки по работам и средний балл</p></td>
    <td width="50%"><img src="docs/screenshots/dashboard.png" alt="Главная ученика"><p align="center"><b>Главная ученика</b> — задания и уроки на сегодня</p></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/schedule.png" alt="Расписание учителя на неделю"><p align="center"><b>Расписание</b> — неделя с кабинетами и темами</p></td>
    <td><img src="docs/screenshots/admin.png" alt="Администрирование"><p align="center"><b>Администрирование</b> — пользователи, роли, структура</p></td>
  </tr>
</table>

<p align="center">
  <img src="docs/screenshots/mobile-journal.png" alt="Мобильный журнал: оценки по уроку" width="260">
  &nbsp;&nbsp;&nbsp;
  <img src="docs/screenshots/mobile-parent.png" alt="Главная родителя на телефоне" width="260">
  <br>
  <sub>Телефон: журнал урока у учителя и главная у родителя</sub>
</p>

## Как устроено

```
Браузер ──► Nginx (TLS, статика React) ──► Go: модульный монолит ──► PostgreSQL
                                                 │
                                                 └──► приватный том с вложениями
```

- **Backend** — Go 1.26, `chi`, `pgx` и `sqlc`; слои handler → service → repository. Контракт API —
  `api/openapi.yaml` (`/api/v1`), типы для frontend генерируются из него.
- **Frontend** — React 19, TypeScript, Vite, TanStack Query; интерфейс на русском, адаптивный.
- **Хранилище** — PostgreSQL 17 как единственный источник данных; файлы — на отдельном томе, метаданные в базе.
- **Фоновый worker** — сводки уведомлений, очистка сессий, сигналы для страницы «Система».
- **Развёртывание** — Docker Compose: профили `local-http` (пилот, изолированная сеть) и `https` (рабочий режим).

Подробнее — [docs/architecture.md](docs/architecture.md).

## Быстрый старт

Есть три способа запуска, выберите подходящий.

### 1. Docker Compose — попробовать на своём компьютере

Нужны **Docker** с Compose 2.24 или новее и **Git**.

```bash
git clone https://github.com/klezufa/schooldiarysvu.git
cd schooldiarysvu
cp .env.example .env          # задайте свой POSTGRES_PASSWORD (и такой же пароль в DATABASE_URL)
docker compose up -d --build
```

Откройте <http://127.0.0.1:8080>. Порт задаёт `HTTP_PORT`. По умолчанию дневник доступен только с этого компьютера,
для доступа из сети задайте `BIND_ADDR`.

Затем создайте системного администратора:

```bash
read -rsp 'Пароль (от 12 символов): ' BOOTSTRAP_ADMIN_PASSWORD; echo; export BOOTSTRAP_ADMIN_PASSWORD
docker compose run --rm -e BOOTSTRAP_ADMIN_LOGIN=sysadmin -e BOOTSTRAP_ADMIN_DISPLAY_NAME='Системный администратор' \
  -e BOOTSTRAP_ADMIN_PASSWORD --entrypoint /app/bootstrap-admin backend
unset BOOTSTRAP_ADMIN_PASSWORD
```

Или загрузите демонстрационную школу с учителями, учениками, родителями и оценками. Логины начинаются с `demo-`,
например `demo-teacher1`, `demo-student1`, `demo-parent1`, `demo-admin`. Пароль — тот, что вы задали:

```bash
DEMO_PASSWORD='временный-пароль' docker compose run --rm --no-deps -e APP_ENV=development -e ALLOW_DEMO_SEED=true \
  -e DEMO_PASSWORD -e DEMO_DATASET=rich --entrypoint /app/seed backend
```

> Демо-данные загружаются только при `APP_ENV=development`. Профиль `local-http` работает без шифрования —
> не используйте его с реальными данными вне изолированной сети.

### 2. Windows 11 без Docker — пилот или разработка

Нужны **Go 1.26+**, **Node.js 22+** (проверено на 24), **PostgreSQL 17** и **Git**. Нужны только программы PostgreSQL
(`winget install --id PostgreSQL.PostgreSQL.17 -e`): скрипт создаёт свой кластер в папке проекта и не трогает
службу Windows.

```powershell
deploy\windows\diary.cmd doctor      # проверить, всё ли установлено
deploy\windows\diary.cmd setup       # собрать и подготовить базу (один раз)
deploy\windows\diary.cmd start       # запустить
deploy\windows\diary.cmd create-admin   # системный администратор (или seed-demo для демо-данных)
```

Адрес — <http://127.0.0.1:8090>. Остановка, резервные копии, обновление и решение проблем —
в [docs/windows-local.md](docs/windows-local.md).

### 3. Сервер школы — рабочая установка

Linux-сервер (проверено на Ubuntu Server 24.04 LTS) с профилем `https`:

| | Минимум | Рекомендуется |
| --- | --- | --- |
| Процессор | 2 ядра | 4 ядра |
| Память | 4 ГБ | 8 ГБ |
| Диск | 20 ГБ SSD | 50 ГБ SSD + отдельный носитель для копий |

Пошаговое руководство от чистого сервера до HTTPS, резервных копий и мониторинга сертификата —
**[docs/installation.md](docs/installation.md)**. Скрипты эксплуатации:

| Скрипт | Назначение |
| --- | --- |
| `scripts/backup.sh [--quiesce]` | резервная копия: база, вложения, manifest, контрольные суммы |
| `scripts/restore.sh КАТАЛОГ` | восстановление со страховочной копией; рабочая база заменяется только после успешной проверки |
| `scripts/install-cert.sh FULLCHAIN KEY` | проверка и установка сертификата, перезагрузка Nginx без остановки |
| `scripts/renew-cert.sh` | выпуск и продление сертификата Let's Encrypt (DNS-01) |
| `scripts/check-cert.sh [--host]` | срок сертификата и что реально отдаёт сервер |

## Разработка

Что нужно:

| Инструмент | Версия | Зачем |
| --- | --- | --- |
| Go | 1.26.10 (закреплена в `go.mod`) | backend, worker, утилиты |
| Node.js и npm | 24 | frontend |
| PostgreSQL | 17 | база для разработки и интеграционных тестов |
| sqlc | 1.29.0 | генерация кода запросов из `db/queries` |
| oapi-codegen | 2.4.1 | генерация типов API из `api/openapi.yaml` |
| Docker Compose | 2.24+ | контейнерный запуск (необязательно) |

```bash
make generate                     # sqlc + oapi-codegen + типы frontend
DATABASE_URL='postgres://diary:пароль@localhost:5432/diary?sslmode=disable' make migrate-up

# backend и frontend в режиме разработки
APP_ENV=development DATABASE_URL='…' go run ./cmd/server
cd web && npm ci && npm run dev   # http://127.0.0.1:5173, запросы /api проксируются на backend
```

Проверки — те же, что запускает CI:

```bash
go vet ./... && go test -race ./...           # с TEST_DATABASE_URL выполняются и интеграционные тесты
cd web && npm run typecheck && npm run lint && npm run test -- --run && npm run build
E2E_DEMO_PASSWORD='…' npx playwright test     # сквозные сценарии всех ролей на трёх ширинах экрана
```

Интеграционным тестам нужен PostgreSQL. Каждый тест создаёт себе временную базу и удаляет её после завершения.

### Структура

```
api/            контракт OpenAPI
cmd/            server, worker, seed (демо-данные), bootstrap-admin, healthcheck
db/             миграции и SQL-запросы для sqlc
internal/       service (бизнес-логика и права), httpserver, database, config, credentials
web/            React-приложение, Vitest и Playwright
deploy/         Dockerfile, конфигурация Nginx, скрипты для Windows
scripts/        резервное копирование, восстановление, сертификаты
load/           нагрузочные сценарии k6
security/       воспроизводимый пентест
docs/           установка, эксплуатация, архитектура, безопасность
```

## Безопасность и нагрузка

- Пароли хранятся как Argon2id. Вход ограничен по паре «адрес — логин» и по адресу, чтобы нельзя было
  перебирать пароли и пробовать один пароль на многих логинах.
- Сессии — в cookie `HttpOnly`, отдельный CSRF-токен, строгий CSP.
- Права проверяет сервер у каждого объекта, скрытая кнопка в интерфейсе защитой не считается.
- Вложения проверяются по содержимому и отдаются только на скачивание.
- Нагрузочный тест на школу в 360 учеников, 360 родителей и 150 учителей прошёл без ошибок: 600 человек одновременно,
  95-й процентиль чтения — 60 мс. Подробности — в [load/README.md](load/README.md).
- Пентест из 144 проверок, статический анализ, проверка зависимостей и образов — в
  [docs/security-audit.md](docs/security-audit.md).

## Документация

| Тема | Документы |
| --- | --- |
| Установка и эксплуатация | [installation](docs/installation.md) · [https](docs/https.md) · [adguard-home](docs/adguard-home.md) · [network](docs/network.md) · [backup-restore](docs/backup-restore.md) · [update](docs/update.md) · [troubleshooting](docs/troubleshooting.md) |
| Windows 11 | [windows-local](docs/windows-local.md) |
| Устройство системы | [architecture](docs/architecture.md) · [security](docs/security.md) · [ux](docs/ux.md) · [решения (ADR)](docs/adr) |
| Проверки | [security-audit](docs/security-audit.md) · [load](load/README.md) |
