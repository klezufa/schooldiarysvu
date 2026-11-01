// Нагрузочный сценарий «учебное утро» для k6: 120 учителей, до 360 учеников и до 360 родителей одновременно.
// Каждый виртуальный пользователь входит один раз, затем повторяет то, что делает его страница в браузере, с паузами
// «на чтение». Учителя пишут: оценки и отметку «все присутствуют».
//
//   BASE_URL=http://127.0.0.1:8097 PASSWORD=<DEMO_PASSWORD> FIXTURES=../.tmp/load/fixtures.json PROFILE=morning k6 run load/school-day.js
//
// PROFILE: smoke — по нескольку пользователей (проверка сценария), morning — утренний наплыв 600 пользователей,
// logins — только входы с нарастающей частотой (поиск предела хеширования паролей).
import http from 'k6/http'
import { check, fail, sleep } from 'k6'
import { Counter, Trend } from 'k6/metrics'
import { SharedArray } from 'k6/data'

const BASE = (__ENV.BASE_URL || 'http://127.0.0.1:8097') + '/api/v1'
const PASSWORD = __ENV.PASSWORD || fail('PASSWORD is required')
const PROFILE = __ENV.PROFILE || 'smoke'
const fixtures = new SharedArray('fixtures', () => [JSON.parse(open(__ENV.FIXTURES || '../.tmp/load/fixtures.json'))])

const loginDuration = new Trend('login_duration', true)
const writeDuration = new Trend('write_duration', true)
const gradeConflicts = new Counter('grade_conflicts')

const pick = (items) => items[Math.floor(Math.random() * items.length)]
const think = (min, max) => sleep(min + Math.random() * (max - min))
const today = new Date().toISOString().slice(0, 10)

const profiles = {
  smoke: {
    teachers: { executor: 'per-vu-iterations', vus: 3, iterations: 2, exec: 'teacher' },
    students: { executor: 'per-vu-iterations', vus: 3, iterations: 2, exec: 'student' },
    parents: { executor: 'per-vu-iterations', vus: 3, iterations: 2, exec: 'parent' },
  },
  // Утро: за 3 минуты собираются все, 6 минут работают. Паузы: учитель 10–30 с, ученик и родитель 5–20 с.
  morning: {
    teachers: { executor: 'ramping-vus', exec: 'teacher', startVUs: 0, stages: [{ duration: '2m', target: 120 }, { duration: '6m', target: 120 }, { duration: '30s', target: 0 }] },
    students: { executor: 'ramping-vus', exec: 'student', startVUs: 0, stages: [{ duration: '3m', target: 300 }, { duration: '5m', target: 300 }, { duration: '30s', target: 0 }] },
    parents: { executor: 'ramping-vus', exec: 'parent', startVUs: 0, stages: [{ duration: '3m', target: 180 }, { duration: '5m', target: 180 }, { duration: '30s', target: 0 }] },
  },
  // Только входы: частота растёт до 20 в секунду — где начинаются ошибки или очередь.
  logins: {
    logins: { executor: 'ramping-arrival-rate', exec: 'loginOnly', startRate: 1, timeUnit: '1s', preAllocatedVUs: 50, maxVUs: 300,
      stages: [{ duration: '30s', target: 5 }, { duration: '30s', target: 10 }, { duration: '30s', target: 20 }, { duration: '30s', target: 20 }] },
  },
}

export const options = {
  scenarios: profiles[PROFILE],
  // Сессия живёт всю смену, как вкладка браузера: без этого k6 сбрасывает cookie на каждой итерации.
  noCookiesReset: true,
  thresholds: {
    http_req_failed: ['rate<0.01'],
    'http_req_duration{kind:read}': ['p(95)<500'],
    'http_req_duration{kind:write}': ['p(95)<800'],
    login_duration: ['p(95)<2000'],
  },
  summaryTrendStats: ['avg', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
}

// Состояние виртуального пользователя (у каждого VU свой экземпляр модуля).
const session = { csrf: '', ready: false }

function login(loginName) {
  const response = http.post(`${BASE}/auth/login`, JSON.stringify({ login: loginName, password: PASSWORD }),
    { headers: { 'Content-Type': 'application/json' }, tags: { kind: 'login', name: 'POST /auth/login' } })
  loginDuration.add(response.timings.duration)
  if (!check(response, { 'login 200': (r) => r.status === 200 })) return false
  session.csrf = response.json('csrf_token')
  session.ready = true
  return true
}

const read = (path, name) => {
  const response = http.get(`${BASE}${path}`, { tags: { kind: 'read', name } })
  check(response, { [`${name} 200`]: (r) => r.status === 200 })
  return response
}

const write = (method, path, body, name, extraHeaders = {}) => {
  const response = http.request(method, `${BASE}${path}`, JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': session.csrf, ...extraHeaders },
    tags: { kind: 'write', name },
    // 409 у оценки — нормальный ответ: ученику по этой работе уже поставили оценку (в том числе другой VU).
    responseCallback: http.expectedStatuses(200, 201, 409),
  })
  writeDuration.add(response.timings.duration)
  return response
}

// Сколько пользователей каждой роли: VU берёт свою учётную запись по номеру, чтобы входы не повторялись.
const teacherOf = () => fixtures[0].teachers[(__VU - 1) % fixtures[0].teachers.length]
const studentOf = () => fixtures[0].students[(__VU - 1) % fixtures[0].students.length]
const parentOf = () => fixtures[0].parents[(__VU - 1) % fixtures[0].parents.length]

export function teacher() {
  const me = teacherOf()
  if (!session.ready && !login(me.login)) return sleep(5)
  const period = fixtures[0].period_id
  // «Сегодня» и дашборд учителя.
  read('/me', 'GET /me')
  read('/schedule/current', 'GET /schedule/current')
  read(`/schedule?from=${today}&to=${today}`, 'GET /schedule')
  think(3, 8)
  // Журнал класса и выставление оценки.
  read(`/journal?class_id=${me.class_id}&subject_id=${me.subject_id}&academic_period_id=${period}`, 'GET /journal')
  read(`/classes/${me.class_id}/students?date=${today}`, 'GET /classes/{id}/students')
  const grade = write('POST', '/grades', { student_id: pick(me.student_ids), lesson_work_id: pick(me.work_ids), value: pick([3, 4, 5]), comment: '' }, 'POST /grades')
  if (grade.status === 409) gradeConflicts.add(1)
  else check(grade, { 'POST /grades 201': (r) => r.status === 201 })
  think(5, 15)
  // Урок: посещаемость «все присутствуют» (новый ключ идемпотентности — новое действие учителя).
  const lesson = pick(me.lesson_ids)
  read(`/attendance?lesson_id=${lesson}`, 'GET /attendance')
  const bulk = write('POST', '/attendance/bulk', { lesson_id: lesson }, 'POST /attendance/bulk', { 'Idempotency-Key': `k6-${__VU}-${__ITER}-${Date.now()}` })
  check(bulk, { 'POST /attendance/bulk 200': (r) => r.status === 200 })
  think(5, 15)
  read(`/analytics/teacher?academic_period_id=${period}`, 'GET /analytics/teacher')
  think(5, 15)
}

function familyPages(studentId, asParent) {
  const scope = asParent ? `student_id=${studentId}&` : ''
  const period = fixtures[0].period_id
  read('/me', 'GET /me')
  if (asParent) read('/children', 'GET /children')
  read(`/schedule?${scope}from=${today}&to=${today}`, 'GET /schedule')
  read(`/announcements?${scope}`.replace(/[?&]$/, ''), 'GET /announcements')
  think(5, 15)
  read(`/analytics/student?${scope}academic_period_id=${period}`, 'GET /analytics/student')
  think(3, 10)
  read(`/grades?${scope}academic_period_id=${period}`, 'GET /grades')
  think(3, 10)
  read(`/homework${asParent ? `?student_id=${studentId}` : ''}`, 'GET /homework')
  think(5, 20)
}

export function student() {
  const me = studentOf()
  if (!session.ready && !login(me.login)) return sleep(5)
  familyPages(me.student_id, false)
}

export function parent() {
  const me = parentOf()
  if (!session.ready && !login(me.login)) return sleep(5)
  familyPages(me.student_id, true)
}

export function loginOnly() {
  const all = fixtures[0].students
  login(all[Math.floor(Math.random() * all.length)].login)
}
