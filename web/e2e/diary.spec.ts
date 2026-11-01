import { test, expect, type Page } from '@playwright/test'
import { readFile } from 'node:fs/promises'
import AxeBuilder from '@axe-core/playwright'

const initialPassword = process.env.E2E_DEMO_PASSWORD
// Пароль после обязательной смены выводится из временного: в репозитории нет готовых паролей.
const passwordFor = (role: string) => process.env.E2E_NEW_PASSWORD ?? `${initialPassword}-${role}-changed`

async function login(page: Page, loginName: string, role: string): Promise<void> {
  test.skip(!initialPassword, 'Set E2E_DEMO_PASSWORD for the development-only demo database')
  await page.goto('/login')
  await page.getByLabel('Логин', { exact: true }).fill(loginName)
  await page.getByLabel('Пароль', { exact: true }).fill(initialPassword!)
  const firstResponse = page.waitForResponse((response) => response.url().endsWith('/api/v1/auth/login'))
  await page.getByRole('button', { name: 'Войти', exact: true }).click()
  if ((await firstResponse).status() === 401) {
    await page.getByLabel('Пароль', { exact: true }).fill(passwordFor(role))
    await page.getByRole('button', { name: 'Войти', exact: true }).click()
  }
  await page.waitForURL(/\/(password|today|admin)$/)
  if (page.url().includes('/password')) {
    await page.getByLabel('Текущий пароль').fill(initialPassword!)
    await page.getByLabel('Новый пароль', { exact: true }).fill(passwordFor(role))
    await page.getByLabel('Повторите новый пароль').fill(passwordFor(role))
    const passwordResponse = page.waitForResponse((response) => response.url().endsWith('/api/v1/auth/password'))
    await page.getByRole('button', { name: 'Сохранить пароль' }).click()
    expect((await passwordResponse).status()).toBe(204)
  }
  await expect(page).not.toHaveURL(/\/login|\/password/)
}

async function selectSeptemberJournal(page: Page): Promise<void> {
  await page.goto('/journal')
  await page.getByLabel('Месяц', { exact: true }).fill('2026-09')
  await page.getByRole('combobox', { name: 'Период', exact: true }).selectOption({ label: 'I четверть' })
  await expect(page.locator('.journal-table, .mobile-lesson').first()).toBeVisible()
}

test('teacher edits a real grade and uses desktop journal without critical accessibility violations', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  await login(page, 'demo-teacher1', 'teacher')
  await selectSeptemberJournal(page)
  await expect(page.getByRole('heading', { name: 'Журнал', exact: true })).toBeVisible()
  const cell = page.locator('[data-grade-cell]').first()
  await expect(cell).toBeVisible()
  await cell.focus()
  // Быстрый ввод: цифра открывает редактор, Enter сохраняет и переводит фокус на следующего ученика.
  const saved = page.waitForResponse((response) => /\/api\/v1\/grades/.test(response.url()) && response.request().method() !== 'GET')
  await page.keyboard.press('4')
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.keyboard.press('Enter')
  expect([200, 201]).toContain((await saved).status())
  await expect(page.getByRole('dialog')).not.toBeVisible()
  await expect(cell).toHaveText(/^4/)
  const nextRowCell = page.locator('[data-grade-cell][data-row="1"][data-col="0"]')
  if (await nextRowCell.count()) await expect(nextRowCell).toBeFocused()
  await cell.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).not.toBeVisible()
  const accessibility = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze()
  expect(accessibility.violations.filter((item) => item.impact === 'critical')).toEqual([])
  expect(errors).toEqual([])
})

test('student loads real grades and toggles personal homework completion', async ({ page }) => {
  await login(page, 'demo-student1', 'student')
  await page.goto('/grades')
  await expect(page.getByRole('heading', { name: 'Оценки', exact: true })).toBeVisible()
  // Демо-оценки стоят в I четверти: период выбирается явно, чтобы тест не зависел от сегодняшней даты.
  await page.getByRole('combobox', { name: 'Период' }).selectOption({ label: 'I четверть' })
  await expect(page.getByText('Математика', { exact: true }).first()).toBeVisible()
  await page.goto('/homework')
  const checkbox = page.getByRole('checkbox', { name: /Личная отметка о выполнении/ }).first()
  await expect(checkbox).toBeVisible()
  // Оптимистичное обновление применяется асинхронно, поэтому ждём состояние, а не проверяем его сразу после клика.
  const toggle = async (expected: boolean) => {
    const saved = page.waitForResponse((response) => /\/api\/v1\/homework\/[^/]+\/completion/.test(response.url()) && response.request().method() === 'PUT')
    await checkbox.click()
    expect((await saved).status()).toBe(200)
    if (expected) await expect(checkbox).toBeChecked(); else await expect(checkbox).not.toBeChecked()
  }
  const initiallyChecked = await checkbox.isChecked()
  await toggle(!initiallyChecked)
  await toggle(initiallyChecked)
  // Состояние переживает перезагрузку: оно сохранено на сервере, а не только в кэше.
  await page.reload()
  const reloaded = page.getByRole('checkbox', { name: /Личная отметка о выполнении/ }).first()
  await expect(reloaded).toBeVisible()
  if (initiallyChecked) await expect(reloaded).toBeChecked(); else await expect(reloaded).not.toBeChecked()
})

test('parent query context follows selected child', async ({ page }) => {
  await login(page, 'demo-parent1', 'parent')
  const requests: string[] = []
  page.on('request', (request) => { if (request.url().includes('/api/v1/')) requests.push(request.url()) })
  const selector = page.getByLabel('Ребёнок', { exact: true })
  await expect(selector.locator('option')).toHaveCount(2)
  await selector.selectOption({ index: 1 })
  await page.goto('/grades')
  const selected = await selector.inputValue()
  await expect(page.getByRole('heading', { name: 'Оценки', exact: true })).toBeVisible()
  expect(requests.some((url) => url.includes(`student_id=${selected}`))).toBe(true)
})

test('mobile uses one-lesson journal and navigation stays within the viewport', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await login(page, 'demo-teacher1', 'teacher')
  await selectSeptemberJournal(page)
  await expect(page.locator('.mobile-journal')).toBeVisible()
  await expect(page.locator('.desktop-journal')).not.toBeVisible()
  await expect(page.getByRole('navigation', { name: 'Мобильная навигация' })).toBeVisible()
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)
  expect(overflow).toBe(false)
  const smallTarget = await page.locator('.mobile-nav a, .mobile-nav button').first().evaluate((element) => ({ width: element.getBoundingClientRect().width, height: element.getBoundingClientRect().height }))
  expect(smallTarget.width).toBeGreaterThanOrEqual(44)
  expect(smallTarget.height).toBeGreaterThanOrEqual(44)
})

test('bulk attendance is applied again after a manual change (fresh idempotency key)', async ({ page }) => {
  await login(page, 'demo-teacher1', 'teacher')
  await selectSeptemberJournal(page)
  const firstStudent = page.getByRole('combobox', { name: /^Посещаемость: / }).first()
  await expect(firstStudent).toBeVisible()
  const bulk = () => page.waitForResponse((response) => response.url().endsWith('/api/v1/attendance/bulk'))
  let response = bulk()
  await page.getByRole('button', { name: 'Все присутствуют' }).click()
  expect((await response).status()).toBe(200)
  await firstStudent.selectOption('absent')
  await expect(firstStudent).toHaveValue('absent')
  response = bulk()
  await page.getByRole('button', { name: 'Все присутствуют' }).click()
  expect((await response).status()).toBe(200)
  await expect(firstStudent).toHaveValue('present')
  // Серверное состояние, а не кэш интерфейса.
  await page.reload()
  await selectSeptemberJournal(page)
  await expect(page.getByRole('combobox', { name: /^Посещаемость: / }).first()).toHaveValue('present')
})

test('a stale grade edit from a second session shows a version conflict and does not overwrite', async ({ browser }) => {
  const open = async () => {
    const context = await browser.newContext()
    const page = await context.newPage()
    await login(page, 'demo-teacher1', 'teacher')
    await selectSeptemberJournal(page)
    return { context, page, cell: page.locator('[data-grade-cell][data-row="0"][data-col="0"]') }
  }
  const first = await open()
  const second = await open()
  try {
    const before = (await first.cell.innerText()).trim()
    const winner = before.startsWith('3') ? '2' : '3'
    const loser = before.startsWith('5') ? '4' : '5'
    await second.cell.focus()
    const saved = second.page.waitForResponse((response) => /\/api\/v1\/grades/.test(response.url()) && response.request().method() !== 'GET')
    await second.page.keyboard.press(winner)
    await second.page.keyboard.press('Enter')
    expect([200, 201]).toContain((await saved).status())
    // Первая сессия по-прежнему держит устаревшую версию ячейки.
    await first.cell.focus()
    const stale = first.page.waitForResponse((response) => /\/api\/v1\/grades/.test(response.url()) && response.request().method() !== 'GET')
    await first.page.keyboard.press(loser)
    await first.page.keyboard.press('Enter')
    expect((await stale).status()).toBe(409)
    await expect(first.page.getByText('Серверная версия изменилась')).toBeVisible()
    await expect(first.page.getByRole('button', { name: loser, pressed: true })).toBeVisible()
    await first.page.keyboard.press('Escape')
    await first.page.reload()
    await selectSeptemberJournal(first.page)
    await expect(first.page.locator('[data-grade-cell][data-row="0"][data-col="0"]')).toHaveText(new RegExp(`^${winner}`))
  } finally {
    await first.context.close()
    await second.context.close()
  }
})

test('completion check appears before a lesson is marked completed and does not block saving', async ({ page }) => {
  await login(page, 'demo-teacher1', 'teacher')
  await page.goto('/schedule')
  const anyCard = page.locator('button.lesson-card')
  const settled = () => Promise.race([anyCard.first().waitFor({ state: 'visible' }), page.getByRole('heading', { name: 'Уроков нет' }).waitFor({ state: 'visible' })])
  await settled()
  for (let step = 0; step < 12 && !(await anyCard.first().isVisible()); step += 1) {
    await page.getByRole('button', { name: 'Предыдущий период' }).click()
    await settled()
  }
  const card = anyCard.first()
  await expect(card).toBeVisible()
  // Проверка нужна только при переходе в «Завершён»: если урок уже завершён, сначала возвращаем его в расписание.
  if (await card.getByText('Завершён').count()) {
    await card.click()
    await page.getByRole('combobox', { name: 'Статус' }).selectOption('planned')
    const reverted = page.waitForResponse((response) => /\/api\/v1\/lessons\//.test(response.url()) && response.request().method() === 'PATCH')
    await page.getByRole('button', { name: 'Сохранить урок' }).click()
    expect((await reverted).status()).toBe(200)
    await page.keyboard.press('Escape')
    await expect(page.getByRole('dialog')).not.toBeVisible()
    await expect(anyCard.first().getByText('По расписанию')).toBeVisible()
  }
  await anyCard.first().click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.getByRole('combobox', { name: 'Статус' }).selectOption('completed')
  const check = page.getByRole('region', { name: 'Проверка перед завершением урока' })
  await expect(check).toBeVisible()
  await expect(check).toContainText('Посещаемость')
  await expect(check).toContainText('Домашнее задание')
  await expect(page.getByRole('button', { name: 'Сохранить урок' })).toBeEnabled()
})

test('an unsent homework draft survives closing the dialog and is dropped in shared-computer mode', async ({ page }) => {
  await login(page, 'demo-teacher1', 'teacher')
  await page.goto('/homework')
  await page.getByRole('button', { name: 'Создать задание' }).click()
  await page.getByLabel('Текст задания').fill('Черновик e2e: параграф 12')
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).not.toBeVisible()
  await page.getByRole('button', { name: 'Создать задание' }).click()
  await expect(page.getByLabel('Текст задания')).toHaveValue('Черновик e2e: параграф 12')
  await page.keyboard.press('Escape')
  await page.goto('/settings')
  await page.getByRole('checkbox', { name: /Общий компьютер/ }).check()
  await page.goto('/homework')
  await page.getByRole('button', { name: 'Создать задание' }).click()
  await expect(page.getByLabel('Текст задания')).toHaveValue('')
  // Возвращаем настройку по умолчанию, чтобы не влиять на другие сценарии.
  await page.keyboard.press('Escape')
  await page.goto('/settings')
  await page.getByRole('checkbox', { name: /Общий компьютер/ }).uncheck()
})

const viewports = [
  { name: 'desktop', width: 1440, height: 900 },
  { name: 'tablet', width: 820, height: 1180 },
  { name: 'mobile', width: 390, height: 844 },
]
const pagesByRole: Record<string, { login: string; paths: string[] }> = {
  teacher: { login: 'demo-teacher1', paths: ['/today', '/schedule', '/journal', '/homework', '/announcements', '/reports', '/my-class', '/settings'] },
  student: { login: 'demo-student1', paths: ['/today', '/schedule', '/homework', '/grades', '/announcements', '/settings'] },
  parent: { login: 'demo-parent1', paths: ['/today', '/schedule', '/homework', '/grades', '/announcements'] },
  admin: { login: 'demo-admin', paths: ['/today', '/schedule', '/admin', '/admin?view=students', '/admin?view=teachers', '/admin?view=classes',
    '/admin?view=schedule', '/admin?view=subjects', '/admin?view=passwords', '/admin?tab=people', '/admin?tab=import', '/admin?tab=invitations', '/admin?tab=enrollment', '/admin?tab=assignments', '/admin?tab=schedule', '/admin?tab=structure', '/admin?tab=audit'] },
  sysadmin: { login: 'demo-sysadmin', paths: ['/today', '/system', '/admin?tab=structure'] },
}

for (const viewport of viewports) {
  for (const [role, config] of Object.entries(pagesByRole)) {
    test(`${role} pages fit the ${viewport.name} viewport without console errors`, async ({ page }) => {
      const problems: string[] = []
      let where = ''
      await page.setViewportSize({ width: viewport.width, height: viewport.height })
      await login(page, config.login, role)
      // Слушатели после входа: 401 от /me до авторизации и первая попытка входа — ожидаемое поведение, а не ошибка страницы.
      page.on('response', (response) => { if (response.status() >= 400) problems.push(`${where} HTTP ${response.status()} ${response.url().replace(/^https?:\/\/[^/]+/, '')}`) })
      page.on('pageerror', (error) => problems.push(`${where} pageerror: ${error.message}`))
      page.on('console', (message) => { if (message.type() === 'error') problems.push(`${where} console: ${message.text().slice(0, 140)}`) })
      for (const path of config.paths) {
        where = path
        await page.goto(path)
        await expect(page.locator('main h1').first()).toBeVisible()
        await page.waitForLoadState('networkidle')
        const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
        expect(overflow, `horizontal overflow on ${path}`).toBeLessThanOrEqual(0)
      }
      expect(problems).toEqual([])
    })
  }
}

test('administrator manages a user: create with roles, edit roles, reset password, deactivate and reactivate', async ({ page }) => {
  page.on('dialog', (dialog) => { void dialog.accept() })
  await login(page, 'demo-admin', 'admin')
  await page.goto('/admin?tab=people')
  const newLogin = `e2e-${Date.now().toString(36)}`
  await page.getByRole('button', { name: 'Добавить пользователя' }).click()
  const dialog = page.getByRole('dialog', { name: 'Новый пользователь' })
  await dialog.getByLabel('Фамилия, имя, отчество *').fill('Тестовый Учитель Иванович')
  await expect(dialog.getByLabel('Логин *')).toHaveValue('testovyy.ui')
  await dialog.getByLabel('Логин *').fill(newLogin)
  // Без роли форма не отправляется; административные роли обычному администратору не предлагаются.
  await dialog.getByRole('checkbox', { name: 'Ученик' }).uncheck()
  await expect(dialog.getByRole('checkbox', { name: 'Администратор', exact: true })).toHaveCount(0)
  await dialog.getByRole('button', { name: 'Создать' }).click()
  await expect(dialog.getByRole('alert')).toContainText('Выберите хотя бы одну роль')
  await dialog.getByRole('checkbox', { name: 'Учитель', exact: true }).check()
  await dialog.getByRole('button', { name: 'Создать' }).click()
  // Временный пароль показывается один раз.
  const secret = page.getByRole('dialog', { name: 'Пользователь создан' })
  await expect(secret.getByTestId('secret-value')).not.toBeEmpty()
  await secret.getByRole('button', { name: 'Готово' }).click()

  await page.getByRole('searchbox').fill(newLogin)
  const row = page.getByRole('row', { name: new RegExp(newLogin) })
  await expect(row).toContainText('Учитель')
  await row.getByRole('button', { name: /^Изменить/ }).click()
  const edit = page.getByRole('dialog', { name: 'Изменить пользователя' })
  await edit.getByRole('checkbox', { name: 'Классный руководитель' }).check()
  await edit.getByRole('button', { name: 'Сохранить' }).click()
  await expect(row).toContainText('Классный руководитель')

  await row.getByRole('button', { name: /^Сбросить пароль/ }).click()
  await expect(page.getByRole('dialog', { name: 'Новый временный пароль' }).getByTestId('secret-value')).toHaveText(/^[A-Z2-9]{4}-[A-Z2-9]{4}-[A-Z2-9]{4}$/)
  await page.getByRole('button', { name: 'Готово' }).click()

  await row.getByRole('button', { name: /^Деактивировать/ }).click()
  await expect(page.getByRole('row', { name: new RegExp(newLogin) })).toHaveCount(0)
  await page.getByRole('combobox', { name: 'Состояние' }).selectOption('inactive')
  const inactive = page.getByRole('row', { name: new RegExp(newLogin) })
  await expect(inactive).toContainText('Деактивирован')
  await inactive.getByRole('button', { name: 'Вернуть' }).click()
  await expect(page.getByRole('row', { name: new RegExp(newLogin) })).toHaveCount(0)

  // Зачисления показываются по именам, журнал аудита фиксирует действия.
  await page.getByRole('tab', { name: 'Зачисление' }).click()
  await expect(page.getByRole('row').filter({ hasText: 'Демо Ученик 1' }).filter({ hasText: '5А' }).first()).toBeVisible()
  await page.getByRole('tab', { name: 'Аудит' }).click()
  await expect(page.locator('.audit-list strong').filter({ hasText: 'user.password_reset' }).first()).toBeVisible()
})

test('parent registers with a one-time invitation and immediately sees the linked child', async ({ page, browser }) => {
  await login(page, 'demo-admin', 'admin')
  await page.goto('/admin?tab=invitations')
  await page.getByRole('combobox', { name: 'Связать с ребёнком' }).selectOption({ label: 'Демо Ученик 3, 5А' })
  await page.getByRole('button', { name: 'Создать приглашение' }).click()
  const code = (await page.getByTestId('secret-value').textContent())!.trim()
  expect(code).toMatch(/^[A-Z2-9]{5}(-[A-Z2-9]{5}){3}$/)

  const visitor = await browser.newPage({ baseURL: process.env.E2E_BASE_URL ?? 'http://127.0.0.1:5173' })
  const parentLogin = `e2e-parent-${Date.now().toString(36)}`
  const parentPassword = `${initialPassword}-invited`
  await visitor.goto(`/register?code=${code}`)
  await visitor.getByRole('button', { name: 'Проверить код' }).click()
  await expect(visitor.getByText('Ребёнок:')).toContainText('Демо Ученик 3')
  await visitor.getByLabel('Фамилия, имя и отчество').fill('Приглашённая Родительница')
  await visitor.getByLabel('Логин').fill(parentLogin)
  await visitor.getByLabel('Пароль', { exact: true }).fill(parentPassword)
  await visitor.getByLabel('Повторите пароль').fill(parentPassword)
  await visitor.getByRole('button', { name: 'Создать учётную запись' }).click()
  await expect(visitor.getByRole('status').filter({ hasText: 'Учётная запись создана' })).toBeVisible()
  await visitor.getByLabel('Пароль', { exact: true }).fill(parentPassword)
  await visitor.getByRole('button', { name: 'Войти', exact: true }).click()
  await visitor.waitForURL(/\/today$/)
  await expect(visitor.getByRole('combobox', { name: 'Ребёнок' })).toContainText('Демо Ученик 3')
  // Код одноразовый.
  await visitor.goto(`/register?code=${code}`)
  await visitor.getByRole('button', { name: 'Проверить код' }).click()
  await expect(visitor.getByText(/уже использован или истёк/)).toBeVisible()
  await visitor.close()
})

test('administrator sees the school schedule by class and the weekly template editor', async ({ page }) => {
  await login(page, 'demo-admin', 'admin')
  await page.goto('/schedule')
  await expect(page.getByRole('combobox', { name: 'Класс' })).toBeVisible()
  await expect(page.getByText('Доступ ограничен')).toHaveCount(0)
  await page.goto('/admin?tab=schedule')
  await expect(page.getByRole('grid', { name: 'Недельный шаблон расписания' })).toBeVisible()
  // Базовые демо-строки шаблона заведены в I четверти.
  await page.getByRole('combobox', { name: 'Учебный период' }).selectOption({ label: 'I четверть' })
  await expect(page.locator('.entry-chip').filter({ hasText: 'Математика' }).first()).toBeVisible()
  // Годы, периоды и шкалы — зона системного администратора.
  await page.getByRole('tab', { name: 'Структура' }).click()
  await expect(page.getByText('их меняет системный администратор')).toBeVisible()
  await page.goto('/system')
  await expect(page.getByRole('heading', { name: 'Доступ ограничен' })).toBeVisible()
})

test('system administrator sees service health, load and database state', async ({ page }) => {
  await login(page, 'demo-sysadmin', 'sysadmin')
  await page.goto('/system')
  await expect(page.getByRole('heading', { name: 'Состояние системы' })).toBeVisible()
  await expect(page.getByText('Веб-сервер и API')).toBeVisible()
  await expect(page.getByRole('heading', { name: 'База данных' })).toBeVisible()
  await expect(page.getByRole('meter', { name: 'Подключения' })).toBeVisible()
  await page.goto('/admin?tab=structure')
  await expect(page.getByRole('heading', { name: 'Учебные годы' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Параметры школы' })).toBeVisible()
})

test('administrator creates, renames and archives a room in the structure section', async ({ page }) => {
  await login(page, 'demo-admin', 'admin')
  await page.goto('/admin?tab=structure')
  const rooms = page.locator('section.admin-entity').filter({ has: page.getByRole('heading', { name: 'Кабинеты', exact: true }) })
  const name = `Кабинет e2e ${Date.now().toString(36)}`
  await rooms.locator('summary').click()
  await rooms.getByLabel(/^Название/).fill(name)
  await rooms.getByRole('button', { name: 'Создать' }).click()
  const row = rooms.locator('.entity-row').filter({ hasText: name })
  await expect(row).toBeVisible()

  await row.getByRole('button', { name: `Изменить ${name}` }).click()
  await rooms.getByLabel(/^Название/).fill(`${name} (ремонт)`)
  await rooms.getByRole('button', { name: 'Сохранить изменения' }).click()
  const renamed = rooms.locator('.entity-row').filter({ hasText: `${name} (ремонт)` })
  await expect(renamed).toBeVisible()

  page.once('dialog', (dialog) => void dialog.accept())
  await renamed.getByRole('button', { name: 'Архивировать' }).click()
  await expect(renamed).toHaveCount(0)
})

/** Первый вход с временным паролем, выданным администратором: дневник требует сразу придумать свой. */
async function firstSignIn(page: Page, loginName: string, temporary: string, ownPassword: string): Promise<void> {
  await page.goto('/login')
  await page.getByLabel('Логин', { exact: true }).fill(loginName)
  await page.getByLabel('Пароль', { exact: true }).fill(temporary)
  await page.getByRole('button', { name: 'Войти', exact: true }).click()
  await page.waitForURL(/\/password$/)
  await page.getByLabel('Текущий пароль').fill(temporary)
  await page.getByLabel('Новый пароль', { exact: true }).fill(ownPassword)
  await page.getByLabel('Повторите новый пароль').fill(ownPassword)
  await page.getByRole('button', { name: 'Сохранить пароль' }).click()
  await page.waitForURL(/\/today$/)
}

function isoDay(offset: number): string {
  const date = new Date()
  date.setDate(date.getDate() + offset)
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

test('simple mode: an administrator prepares a class from scratch, the new teacher and parent see the result', async ({ page, browser }) => {
  test.setTimeout(120_000)
  await login(page, 'demo-admin', 'admin')
  await page.goto('/admin')
  await expect(page.getByRole('heading', { name: 'Подготовка школы' })).toBeVisible()
  const tiles = page.getByRole('navigation', { name: 'Задачи администратора' })

  // Предмет из типового списка.
  await tiles.getByRole('button', { name: /Предметы и кабинеты/ }).click()
  await page.getByText('Выбрать из типового списка').click()
  await page.getByRole('checkbox', { name: 'Физика' }).check()
  await page.getByRole('button', { name: 'Добавить отмеченные (1)' }).click()
  await expect(page.locator('.tag-list li').filter({ hasText: 'Физика' })).toBeVisible()
  await page.getByRole('button', { name: 'К списку задач' }).click()

  // Класс: после создания сразу открывается его карточка.
  const stamp = Date.now().toString(36).slice(-4)
  const className = `7Е-${stamp}`
  await tiles.getByRole('button', { name: /Классы/ }).click()
  await page.getByRole('button', { name: 'Создать класс' }).click()
  await page.getByLabel('Название класса *').fill(className)
  await page.getByRole('dialog').getByRole('button', { name: 'Создать класс' }).click()
  await expect(page.getByRole('heading', { name: `${className} класс` })).toBeVisible()
  await page.getByRole('button', { name: 'К списку классов' }).click()
  await page.getByRole('button', { name: 'К списку задач' }).click()

  // Учитель сразу с нагрузкой и классным руководством; логин подбирается из ФИО.
  await tiles.getByRole('button', { name: /Учителя/ }).click()
  await page.getByRole('button', { name: 'Добавить учителя' }).click()
  const teacherDialog = page.getByRole('dialog')
  await teacherDialog.getByLabel('Фамилия, имя, отчество *').fill(`Ершова${stamp} Елена Викторовна`)
  await teacherDialog.getByLabel('Классный руководитель').selectOption({ label: className })
  await teacherDialog.getByLabel('Предмет').selectOption({ label: 'Физика' })
  await teacherDialog.getByLabel('Класс', { exact: true }).selectOption({ label: className })
  await teacherDialog.getByRole('button', { name: 'Сохранить' }).click()
  const credentials = page.getByRole('dialog', { name: 'Данные для входа' })
  await expect(credentials).toBeVisible()
  const teacherLogin = (await credentials.getByTestId('credential-login').first().textContent())!.trim()
  const teacherPassword = (await credentials.getByTestId('credential-password').first().textContent())!.trim()
  expect(teacherLogin).toMatch(/^ershova.+\.ev\d*$/)
  await credentials.getByRole('button', { name: 'Готово' }).click()
  await expect(page.locator('.simple-list li').filter({ hasText: `Ершова${stamp}` })).toContainText(`Физика (${className})`)
  await page.getByRole('button', { name: 'К списку задач' }).click()

  // Ученик вместе с новым родителем: два логина в одном окне.
  await tiles.getByRole('button', { name: /Ученики и родители/ }).click()
  await page.getByRole('button', { name: 'Добавить ученика' }).click()
  const studentDialog = page.getByRole('dialog')
  await studentDialog.getByLabel('Фамилия, имя, отчество ученика *').fill(`Ершов${stamp} Егор Ильич`)
  await studentDialog.getByLabel('Класс *').selectOption({ label: className })
  await studentDialog.getByLabel('Фамилия, имя, отчество родителя *').fill(`Ершов${stamp} Илья Петрович`)
  await studentDialog.getByRole('button', { name: 'Сохранить' }).click()
  await expect(credentials.getByTestId('credential-login')).toHaveCount(2)
  const parentLogin = (await credentials.getByTestId('credential-login').nth(1).textContent())!.trim()
  const parentPassword = (await credentials.getByTestId('credential-password').nth(1).textContent())!.trim()
  await credentials.getByRole('button', { name: 'Готово' }).click()
  await expect(page.locator('.simple-list li').filter({ hasText: `Ершов${stamp} Егор` })).toContainText(`${className} класс`)
  await page.getByRole('button', { name: 'К списку задач' }).click()

  // Урок в сетку: учитель предлагается первым, уроки создаются сразу — без отдельного шага.
  await tiles.getByRole('button', { name: /Расписание/ }).click()
  await page.getByRole('combobox', { name: 'Класс' }).selectOption({ label: className })
  await page.getByRole('button', { name: 'Поставить урок: Ср, 6-й урок' }).click()
  const lessonDialog = page.getByRole('dialog')
  await lessonDialog.getByLabel('Предмет *').selectOption({ label: 'Физика' })
  await expect(lessonDialog.getByLabel('Учитель *')).toHaveValue(/.+/)
  await lessonDialog.getByRole('button', { name: 'Сохранить' }).click()
  await expect(page.locator('.entry-chip').filter({ hasText: 'Физика' })).toBeVisible()

  // Режим запоминается: расширенный показывает вкладки и после перезагрузки.
  await page.getByRole('button', { name: 'Расширенный режим' }).click()
  await expect(page.getByRole('tab', { name: 'Пользователи' })).toBeVisible()
  await page.goto('/admin')
  await expect(page.getByRole('tab', { name: 'Пользователи' })).toBeVisible()
  await page.getByRole('button', { name: 'Простой режим' }).click()
  await expect(page.getByRole('heading', { name: 'Подготовка школы' })).toBeVisible()

  const ownPassword = `${initialPassword}-own-simple`
  const teacherContext = await browser.newContext({ baseURL: process.env.E2E_BASE_URL ?? 'http://127.0.0.1:5173' })
  const teacher = await teacherContext.newPage()
  await firstSignIn(teacher, teacherLogin, teacherPassword, ownPassword)
  await expect(teacher.getByRole('link', { name: 'Мой класс' })).toBeVisible()
  const lessons = await (await teacher.request.get(`/api/v1/schedule?from=${isoDay(0)}&to=${isoDay(13)}`)).json() as { subject_name?: string; class_name?: string }[]
  expect(lessons.some((lesson) => lesson.subject_name === 'Физика' && lesson.class_name === className)).toBe(true)
  await teacherContext.close()

  const parentContext = await browser.newContext({ baseURL: process.env.E2E_BASE_URL ?? 'http://127.0.0.1:5173' })
  const parent = await parentContext.newPage()
  await firstSignIn(parent, parentLogin, parentPassword, ownPassword)
  await expect(parent.getByRole('combobox', { name: 'Ребёнок' })).toContainText(`Ершов${stamp} Егор`)
  await parentContext.close()
})

// Последним: перенос меняет данные II четверти. В новой четверти нет назначений, и предмет в шаблоне выбрать нельзя,
// пока назначения и шаблон не перенесены из предыдущего периода.
test('administrator carries assignments and the weekly template into a new period', async ({ page }) => {
  await login(page, 'demo-admin', 'admin')
  await page.goto('/admin?tab=schedule')
  await page.getByRole('combobox', { name: 'Класс' }).selectOption({ label: '5А' })
  await page.getByRole('combobox', { name: 'Учебный период' }).selectOption({ label: 'II четверть' })
  await page.getByRole('button', { name: 'Добавить урок: Пн, 1-й урок' }).click()
  await expect(page.getByRole('combobox', { name: 'Предмет *' }).locator('option')).toHaveCount(1)
  await expect(page.getByText('перенесите их из прошлого периода')).toBeVisible()
  await page.getByRole('button', { name: 'Отмена' }).click()

  const notice = page.locator('.carry-over')
  await expect(notice).toContainText('нет назначений учителей')
  await notice.getByRole('button', { name: 'Перенести из «I четверть»' }).click()
  await expect(page.getByText(/Перенесено назначений: [1-9]/)).toBeVisible()
  await expect(notice).toHaveCount(0)
  await expect(page.locator('.entry-chip').filter({ hasText: 'Математика' }).first()).toBeVisible()

  await page.locator('.add-slot').first().click()
  await expect(page.getByRole('combobox', { name: 'Предмет *' }).locator('option')).not.toHaveCount(1)
  await page.getByRole('button', { name: 'Отмена' }).click()
})

async function apiCall<T>(page: Page, method: 'GET' | 'POST', path: string, data?: unknown): Promise<T> {
  const csrf = (await page.context().cookies()).find((cookie) => cookie.name === 'diary_csrf')?.value ?? ''
  const response = await page.request.fetch(`/api/v1${path}`, { method, data, headers: { 'X-CSRF-Token': decodeURIComponent(csrf) } })
  expect(response.ok(), `${method} ${path}: ${response.status()} ${await response.text()}`).toBeTruthy()
  return response.json() as Promise<T>
}

// Последним: создаёт следующий учебный год. Классы относятся к году, поэтому 5А прошлого года и 6А нового — разные
// классы: перевод и перенос назначений идут явно «5А → 6А», а списки не смешивают одноимённые классы разных лет.
test('system administrator moves a class into the next academic year with its teachers and template', async ({ page }) => {
  await login(page, 'demo-sysadmin', 'sysadmin')
  type Period = { id: string; name: string; academic_year_id: string }
  const periods = await apiCall<Period[]>(page, 'GET', '/academic-periods')
  const first = periods.find((item) => item.name === 'I четверть')!
  const second = periods.find((item) => item.name === 'II четверть')!
  const classes = await apiCall<{ id: string; name: string }[]>(page, 'GET', '/classes')
  const fifth = classes.find((item) => item.name === '5А')!
  // Источник для переноса между годами — последний период прошлого года (II четверть); перенос идемпотентен.
  await apiCall(page, 'POST', `/academic-periods/${second.id}/carry-over`, { from_period_id: first.id, class_id: fifth.id })
  const year = await apiCall<{ id: string }>(page, 'POST', '/academic-years', { name: '2027/2028', starts_on: '2027-09-01', ends_on: '2028-05-31' })
  const next = await apiCall<{ id: string }>(page, 'POST', '/academic-periods', { academic_year_id: year.id, name: 'I четверть', starts_on: '2027-09-01', ends_on: '2027-10-31' })
  await apiCall(page, 'POST', '/classes', { academic_year_id: year.id, name: '6А' })

  await page.goto('/admin?tab=enrollment')
  await page.getByLabel('Из класса *').selectOption({ label: '5А · 2026/2027' })
  await page.getByLabel('В класс *').selectOption({ label: '6А · 2027/2028' })
  await expect(page.getByLabel('С даты *').last()).toHaveValue('2027-09-01')
  page.once('dialog', (dialog) => dialog.accept())
  await page.getByRole('button', { name: 'Перевести класс' }).click()
  await expect(page.getByText(/Переведено: [1-9]/)).toBeVisible()

  await page.goto('/admin?tab=schedule')
  await page.getByRole('combobox', { name: 'Учебный период' }).selectOption(next.id)
  // Классы только года выбранного периода: одноимённые классы прошлого года сюда не попадают.
  const classSelect = page.locator('.filter-bar').getByRole('combobox', { name: 'Класс' })
  await expect(classSelect.locator('option')).toHaveText(['6А'])
  const notice = page.locator('.carry-over')
  await expect(notice).toContainText('новый учебный год')
  await notice.getByRole('combobox', { name: 'Класс прошлого года' }).selectOption({ label: '5А' })
  await notice.getByRole('button', { name: 'Перенести из «II четверть»' }).click()
  await expect(page.getByText(/Перенесено назначений: [1-9]/)).toBeVisible()
  await expect(page.locator('.entry-chip').filter({ hasText: 'Математика' }).first()).toBeVisible()

  // Назначения нового года — с именами (раньше список показывал идентификаторы) и только выбранного периода.
  await page.goto('/admin?tab=assignments')
  await page.getByLabel('Учебный период *').selectOption(next.id)
  await expect(page.getByText(/Назначения на «I четверть» \([1-9]/)).toBeVisible()
  await expect(page.locator('.entity-row').filter({ hasText: 'Математика · 6А' }).first()).toBeVisible()
})

test('teacher attaches a file to homework and the student downloads exactly that file', async ({ page, browser }) => {
  const content = Buffer.from('%PDF-1.4\n1 0 obj <<>> endobj\ntrailer <<>>\n%%EOF\n')
  await login(page, 'demo-teacher1', 'teacher')
  await page.goto('/homework')
  const card = page.locator('.homework-item').filter({ hasText: 'Демонстрационное домашнее задание' }).first()
  const uploaded = page.waitForResponse((response) => response.url().endsWith('/api/v1/files') && response.request().method() === 'POST')
  await card.locator('input[type="file"]').setInputFiles({ name: 'план-урока.pdf', mimeType: 'application/pdf', buffer: content })
  expect((await uploaded).status()).toBe(201)
  await expect(card.getByRole('button', { name: 'Скачать план-урока.pdf' })).toBeVisible()

  const context = await browser.newContext()
  const student = await context.newPage()
  try {
    await login(student, 'demo-student1', 'student')
    await student.goto('/homework')
    const studentCard = student.locator('.homework-item').filter({ hasText: 'Демонстрационное домашнее задание' }).first()
    const download = student.waitForEvent('download')
    await studentCard.getByRole('button', { name: 'Скачать план-урока.pdf' }).click()
    const file = await download
    expect(file.suggestedFilename()).toBe('план-урока.pdf')
    expect(Buffer.compare(await readFile(await file.path()), content)).toBe(0)
    // Ученику нельзя загружать и архивировать файлы.
    await expect(studentCard.locator('input[type="file"]')).toHaveCount(0)
    await expect(studentCard.getByRole('button', { name: /Архивировать/ })).toHaveCount(0)
  } finally {
    await context.close()
  }
})

test('repeated wrong passwords block the login and say how long to wait', async ({ page }) => {
  // Несуществующий логин: блокировка действует на пару «адрес — логин» и не мешает остальным сценариям.
  await page.goto('/login')
  for (let attempt = 1; attempt <= 6; attempt++) {
    await page.getByLabel('Логин', { exact: true }).fill('nobody.e2e')
    await page.getByLabel('Пароль', { exact: true }).fill('wrong-password-123')
    const response = page.waitForResponse((item) => item.url().endsWith('/api/v1/auth/login'))
    await page.getByRole('button', { name: 'Войти', exact: true }).click()
    const result = await response
    if (attempt < 6) {
      expect(result.status()).toBe(401)
    } else {
      expect(result.status()).toBe(429)
      expect(Number(result.headers()['retry-after'])).toBeGreaterThan(14 * 60)
    }
  }
  await expect(page.getByText(/Слишком много попыток\. Повторите через 15 мин\./)).toBeVisible()
})
