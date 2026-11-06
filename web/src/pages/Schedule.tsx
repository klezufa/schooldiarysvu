import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { CalendarDays, ChevronLeft, ChevronRight, MapPin } from 'lucide-react'
import { api, apiPage, queryString } from '../api/client'
import type { AcademicYear, Lesson, NamedEntity, User } from '../api/types'
import { isAdministrator } from '../api/types'
import { useAuth } from '../authContext'
import { useStudent } from '../studentContext'
import { scopedKey } from '../lib/student'
import { addDays, dateISO, formatDate, formatTime, startOfWeek } from '../lib/dates'
import { subjectStyle } from '../lib/subjects'
import { lessonTone } from '../lib/lessons'
import { classLabeler, openClasses } from '../lib/academicYears'
import { PageHeader, QueryState, StatusBadge } from '../components/UI'
import { LessonPanel } from '../components/LessonPanel'

const weekdays = ['Понедельник', 'Вторник', 'Среда', 'Четверг', 'Пятница', 'Суббота', 'Воскресенье']

export function LessonCard({ lesson, onClick }: { lesson: Lesson; onClick?: () => void }): React.JSX.Element {
  const now = Date.now()
  const live = lesson.status !== 'cancelled' && new Date(lesson.starts_at).getTime() <= now && now < new Date(lesson.ends_at).getTime()
  const content = (
    <>
      <div className="lesson-time"><strong>{formatTime(lesson.starts_at)}</strong><span>{formatTime(lesson.ends_at)}</span></div>
      <div className="lesson-main">
        <div>
          <h3>{lesson.subject_name || 'Предмет'}</h3>
          {live ? <span className="now-badge">Идёт сейчас</span> : <StatusBadge tone={lessonTone(lesson.status)}>{lesson.status === 'cancelled' ? 'Отменён' : lesson.status === 'completed' ? 'Завершён' : 'По расписанию'}</StatusBadge>}
        </div>
        <p>{lesson.topic || 'Тема ещё не указана'}</p>
        {lesson.original_teacher_name && <small className="lesson-substitution">Замена · вместо: {lesson.original_teacher_name}</small>}
        <small>{lesson.class_name && <span>{lesson.class_name} · </span>}{lesson.room_name && <span><MapPin aria-hidden="true" />{lesson.room_name}</span>}</small>
      </div>
    </>
  )
  const className = `lesson-card ${lesson.status === 'cancelled' ? 'lesson-cancelled' : ''} ${live ? 'lesson-now' : ''}`
  return onClick
    ? <button className={className} style={subjectStyle(lesson.subject_name)} onClick={onClick}>{content}</button>
    : <article className={className} style={subjectStyle(lesson.subject_name)}>{content}</article>
}

type Scope = { kind: 'class' | 'teacher'; id: string }

/** Выбор класса или учителя для тех, у кого нет «своего» расписания: администратор, руководитель школы. */
function ScopePicker({ scope, onChange }: { scope: Scope; onChange: (scope: Scope) => void }): React.JSX.Element {
  const classes = useQuery({ queryKey: ['classes'], queryFn: () => api<NamedEntity[]>('/classes') })
  const years = useQuery({ queryKey: ['academic-years'], queryFn: () => api<AcademicYear[]>('/academic-years') })
  // Классы незавершённых лет с подписью года: после перехода на новый год «5А» есть в двух годах.
  const visible = useMemo(() => (years.data ? openClasses(classes.data, years.data) : (classes.data ?? [])), [classes.data, years.data])
  const current = years.data?.find((year) => year.is_current)
  const label = classLabeler(visible, years.data)
  const teachers = useQuery({ queryKey: ['users', 'by-role', 'teacher'], queryFn: async () => (await apiPage<User>('/users?role=teacher&limit=100')).items, retry: false })
  useEffect(() => {
    const first = visible.find((item) => item.academic_year_id === current?.id) ?? visible[0]
    if (!scope.id && first) onChange({ kind: 'class', id: first.id })
  }, [scope.id, visible, current, onChange])
  return (
    <div className="schedule-scope">
      <label>Показать
        <select value={scope.kind} onChange={(event) => onChange({ kind: event.target.value as Scope['kind'], id: '' })}>
          <option value="class">Расписание класса</option>
          {teachers.data && <option value="teacher">Расписание учителя</option>}
        </select>
      </label>
      {scope.kind === 'class' ? (
        <label>Класс
          <select value={scope.id} onChange={(event) => onChange({ kind: 'class', id: event.target.value })}>
            {visible.map((item) => <option key={item.id} value={item.id}>{label(item)}</option>)}
          </select>
        </label>
      ) : (
        <label>Учитель
          <select value={scope.id} onChange={(event) => onChange({ kind: 'teacher', id: event.target.value })}>
            <option value="">Выберите…</option>
            {teachers.data?.filter((user) => user.teacher_id).map((user) => <option key={user.id} value={user.teacher_id}>{user.display_name}</option>)}
          </select>
        </label>
      )}
    </div>
  )
}

export function SchedulePage(): React.JSX.Element {
  const { studentId } = useStudent()
  const { hasRole, user } = useAuth()
  const [week, setWeek] = useState(startOfWeek())
  const [day, setDay] = useState(dateISO())
  const [mode, setMode] = useState<'week' | 'day'>('week')
  const [selected, setSelected] = useState<Lesson | null>(null)
  // У учителя и ученика есть «своё» расписание; администратору и руководителю нужно выбрать класс или учителя.
  const ownSchedule = hasRole('teacher', 'class_teacher', 'student', 'parent')
  const school = !ownSchedule && Boolean(user && (isAdministrator(user.roles) || hasRole('principal')))
  const [scope, setScope] = useState<Scope>({ kind: 'class', id: '' })
  const from = mode === 'week' ? week : day
  const to = mode === 'week' ? addDays(week, 6) : day
  const params = school
    ? { from, to, class_id: scope.kind === 'class' ? scope.id : undefined, teacher_id: scope.kind === 'teacher' ? scope.id : undefined }
    : { from, to, student_id: studentId }
  const schedule = useQuery({
    queryKey: scopedKey('schedule', school ? `${scope.kind}:${scope.id}` : studentId, from, to),
    queryFn: () => api<Lesson[]>(`/schedule${queryString(params)}`),
    // У сотрудников нет student_id: их расписание строится по учётной записи. Родителю нужен выбранный ребёнок.
    enabled: school ? Boolean(scope.id) : !hasRole('parent') || Boolean(studentId),
  })
  const days = Array.from({ length: 7 }, (_, index) => addDays(week, index))
  const visibleDays = days.filter((date, index) => index < 6 || schedule.data?.some((lesson) => lesson.lesson_date === date))
  return (
    <div>
      <PageHeader eyebrow="Учебная неделя" title="Расписание" description="Время, кабинеты, темы, отмены и связанные задания." actions={<button className="button secondary" onClick={() => { setWeek(startOfWeek()); setDay(dateISO()) }}>Текущая неделя</button>} />
      {school && <ScopePicker scope={scope} onChange={setScope} />}
      <div className="toolbar">
        <div className="segmented" aria-label="Период расписания">
          <button className={mode === 'week' ? 'active' : ''} onClick={() => setMode('week')}>Неделя</button>
          <button className={mode === 'day' ? 'active' : ''} onClick={() => setMode('day')}>День</button>
        </div>
        <div className="date-navigation">
          <button className="icon-button" aria-label="Предыдущий период" onClick={() => (mode === 'week' ? setWeek(addDays(week, -7)) : setDay(addDays(day, -1)))}><ChevronLeft /></button>
          <strong>{mode === 'week' ? `${formatDate(week, { day: 'numeric', month: 'short' })} — ${formatDate(to, { day: 'numeric', month: 'short', year: 'numeric' })}` : formatDate(day, { weekday: 'long', day: 'numeric', month: 'long' })}</strong>
          <button className="icon-button" aria-label="Следующий период" onClick={() => (mode === 'week' ? setWeek(addDays(week, 7)) : setDay(addDays(day, 1)))}><ChevronRight /></button>
        </div>
        {mode === 'day' && <input aria-label="Дата расписания" type="date" value={day} onChange={(event) => setDay(event.target.value)} />}
      </div>
      <QueryState isLoading={schedule.isLoading} error={schedule.error} retry={schedule.refetch} isEmpty={!schedule.data?.length} emptyTitle="Уроков нет"
        emptyText={school ? 'На выбранный период уроки не созданы. Администратор создаёт их по шаблону: «Администрирование → Расписание».' : 'На выбранный период занятия не запланированы.'}>
        {mode === 'day' ? (
          <div className="lesson-list">{schedule.data?.map((lesson) => <LessonCard key={lesson.id} lesson={lesson} onClick={() => setSelected(lesson)} />)}</div>
        ) : (
          <div className="week-grid">
            {visibleDays.map((date) => (
              <section className={`day-column ${date === dateISO() ? 'today-column' : ''}`} key={date}>
                <header><span>{weekdays[days.indexOf(date)]}</span><strong>{formatDate(date, { day: 'numeric', month: 'short' })}</strong></header>
                <div>{schedule.data?.filter((lesson) => lesson.lesson_date === date).map((lesson) => <LessonCard key={lesson.id} lesson={lesson} onClick={() => setSelected(lesson)} />)}</div>
              </section>
            ))}
          </div>
        )}
      </QueryState>
      <LessonPanel lesson={selected} onClose={() => setSelected(null)} />
      {!schedule.isLoading && !schedule.error && <p className="calendar-note"><CalendarDays aria-hidden="true" />Нажмите на урок, чтобы открыть подробности и домашнее задание.</p>}
    </div>
  )
}
