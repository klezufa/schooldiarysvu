# Тестирование

Что и чем проверяется, как это запустить и где искать тест конкретной функции.

## Уровни

| Уровень | Где | Что проверяет | Запуск |
| --- | --- | --- | --- |
| Модульные Go | `internal/**/*_test.go` | правила без базы: ограничители входа, имена файлов, пагинация, метрики, конфигурация | `go test ./...` |
| Интеграционные Go | `internal/httpserver/*_integration_test.go`, `internal/database` | весь путь HTTP → сервис → PostgreSQL: права каждой роли, транзакции и аудит, версии, даты | `TEST_DATABASE_URL=… go test -race ./...` |
| Покрытие API | `api/coverage.py` | каждая операция из `api/openapi.yaml` вызывается интеграционными тестами (в CI — обязательно) | `python3 api/coverage.py --strict` |
| Компоненты интерфейса | `web/src/**/*.test.ts(x)`, Vitest | логика экранов и вспомогательные функции: журнал, формы, графики, подготовка школы | `cd web && npm run test -- --run` |
| Сквозные | `web/e2e/diary.spec.ts`, Playwright | собранный интерфейс против настоящего сервера и базы: сценарии всех ролей, три ширины экрана, доступность (axe) | см. ниже |
| Безопасность | `security/pentest.py` | 144 атаки извне через Nginx — [security-audit.md](security-audit.md) | только тестовый стенд |
| Нагрузка | `load/school-day.js` (k6) | учебное утро на 600 пользователей — [load/README.md](../load/README.md) | только тестовый стенд |

Интеграционные тесты создают себе временную базу на сервере `TEST_DATABASE_URL` и удаляют её; от времени запуска
они не зависят. Сквозным тестам нужна база с базовыми демо-данными:

```bash
# сервер с собранным интерфейсом и демо-данные (APP_ENV=development)
HTTP_ADDR=127.0.0.1:8080 STATIC_DIR=web/dist APP_ENV=development DATABASE_URL=… go run ./cmd/server &
ALLOW_DEMO_SEED=true APP_ENV=development DEMO_PASSWORD='временный-пароль' DATABASE_URL=… go run ./cmd/seed
cd web && E2E_BASE_URL=http://127.0.0.1:8080 E2E_DEMO_PASSWORD='временный-пароль' npx playwright test
```

`E2E_TIMEZONE=UTC` запускает браузер в другом часовом поясе, чем школа, — так в CI. После полуночи по школьному времени
сутки браузера и сервера расходятся; всё, что зависит от «сегодня» (действует ли связь, что удалять, а что завершать),
должен решать сервер. Повторите прогон с этой переменной, если меняете такую логику.

Сквозные тесты меняют данные (пароли демо-пользователей, расписание II четверти), поэтому повторный прогон — на
свежей базе. Демо-данные привязаны к 2026/2027 учебному году.

## Матрица: функция → тесты

Go — интеграционный тест (`internal/httpserver`), E2E — сквозной сценарий Playwright, Vitest — тест компонента.

| Область | Что проверяется | Тесты |
| --- | --- | --- |
| Вход и сессии | токен хранится только хешем, CSRF, смена пароля отзывает старую сессию, выход | Go `TestAPIEndToEnd` |
| Перебор паролей | блокировка пары «адрес — логин» и адреса, срок в ответе, подмена заголовков | Go `TestLoginLimiter_*`, `TestAddressLimiter_*`; E2E «repeated wrong passwords»; пентест |
| Пользователи и роли | создание с ролями, смена ролей, сброс пароля, отзыв сессий, деактивация и возврат; администратор не трогает системного | Go `TestUserManagementPrivilegesAndLifecycle`; E2E «administrator manages a user» |
| Импорт, зачисление, перевод класса | весь импорт или ничего, логины, массовое зачисление, перевод в новый класс | Go `TestImportEnrollmentAndPromotion` |
| Связь родителя с ребёнком | подсказки по фамилии (мужская и женская форма) и отчеству от имени родителя, порядок подсказок, класс ребёнка; ошибочная сегодняшняя связь удаляется сразу, давняя только завершается; родитель без связи видит объяснение на всех страницах | Go `TestChildMatchReasons` (20 случаев ФИО), `TestParentChildSuggestionsAndLinkRemoval`; E2E «a parent added without a child is told why there are no grades» |
| Приглашения и регистрация | одноразовый код, связь с ребёнком, повторное использование | Go `TestInvitationsAndRegistration`; E2E «parent registers with a one-time invitation» |
| Справочники и связи | предметы и кабинеты (создание, повтор, переименование, архив), переименование класса и периода, состав класса на дату, связь родителя с ребёнком и её завершение, завершение назначения учителя | Go `TestDirectoryAndRelationsAdministration`; E2E «creates, renames and archives a room» |
| Учебные годы | новый год, классы года, перевод с учителями и шаблоном, даты внутри года | Go `TestAcademicYearTransition`; E2E «moves a class into the next academic year» |
| Расписание | шаблон, пересечения класса/учителя/кабинета, создание уроков, перенос в новую четверть, синхронизация будущих уроков при правке шаблона | Go `TestScheduleManagementGenerationAndAnalytics`, `TestCarryOverAcademicPeriod`, `TestScheduleEntryChangesResyncFutureLessons`; E2E «school schedule by class», «carries assignments and the weekly template» |
| Журнал и оценки | уроки, работы, оценки с версией, конфликт параллельной правки, история, удаление, закрытый период и поздние правки, откат при сбое аудита | Go `TestAPIEndToEnd`, `TestTermGradesAndLessonWorks`; E2E «teacher edits a real grade», «stale grade edit»; Vitest журнал |
| Посещаемость | массовая отметка с ключом идемпотентности, ручная правка, доступ только учителям | Go `TestAPIEndToEnd`, `TestTermGradesAndLessonWorks`; E2E «bulk attendance» |
| Итоговые оценки | только учитель назначения, шкала, повторное сохранение, уведомление ученику | Go `TestTermGradesAndLessonWorks` |
| Домашние задания и файлы | черновик скрыт от ученика, правка с версией, публикация, копирование, вложения только своим | Go `TestHomeworkDraftEditPublishAndNotifications`, `TestAPIEndToEnd`, `TestManagementAndReportsEndToEnd`; E2E «attaches a file», «unsent homework draft» |
| Уведомления | уведомление о задании ученику и родителю, прочтение только своего, настройки, сводки и тихие часы | Go `TestHomeworkDraftEditPublishAndNotifications`, `TestWorkerDigestAndQuietHours` |
| Объявления | видимость классу, удаление | Go `TestManagementAndReportsEndToEnd` |
| Отчёты и аналитика | отчёты класса, ученика, предмета, незаполненные уроки, CSV без формул, дашборды | Go `TestManagementAndReportsEndToEnd`, `TestScheduleManagementGenerationAndAnalytics` |
| Доступ к чужим данным | ученик, родитель и учитель не видят чужое (404 без раскрытия существования) | Go во всех тестах выше; E2E «parent query context follows selected child»; пентест |
| Упрощённый режим администрирования | класс с нуля: предмет из типового списка, класс, учитель с нагрузкой и классным руководством, ученик с родителем, урок в сетку; новые учитель и родитель входят и видят результат; режим запоминается | E2E «simple mode»; Vitest `setup.test.ts` |
| Система и эксплуатация | состояние сервисов, копии, сертификат, диск | Go `TestSystemOverviewOperations`; E2E «system administrator sees service health»; Vitest `operations.test.ts` |
| Интерфейс | все страницы каждой роли на компьютере, планшете и телефоне без ошибок в консоли и горизонтальной прокрутки; доступность (axe) | E2E «… pages fit the … viewport», «mobile uses one-lesson journal» |
| Схема и даты | ограничения таблиц, «сегодня» по часовому поясу школы, а не сервера базы | Go `TestSchemaConstraintsAndQueries`, `TestSchoolTodayFollowsSchoolTimezone` |

## Правила для новых тестов

- Новая операция API — интеграционный тест с проверкой прав (иначе CI остановится на `api/coverage.py`).
- Проверять поведение, а не только код ответа: что изменилось в данных, что видит другая роль.
- Даты «в будущем» считать от текущего дня, а не записывать константой: тест не должен устаревать.
- Отказ проверять вместе с положительным случаем: тот же пользователь получает доступ к своему.
