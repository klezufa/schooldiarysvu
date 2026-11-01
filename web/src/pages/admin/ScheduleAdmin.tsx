import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { CalendarPlus, CopyPlus, Plus, Trash2, Wand2 } from 'lucide-react'
import { api, mutation, queryString } from '../../api/client'
import type { AcademicPeriod, Assignment, CarryOverResult, GenerateLessonsResult, NamedEntity, ScheduleEntry } from '../../api/types'
import { dateISO } from '../../lib/dates'
import { defaultDateIn, previousPeriod } from '../../lib/academicYears'
import { subjectStyle } from '../../lib/subjects'
import { useToast } from '../../components/toastContext'
import { Field, Modal, QueryState } from '../../components/UI'
import { options, useDirectory } from './helpers'
import { PeriodSelect } from './shared'

const weekdays = ['Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб']
/** Типовое расписание звонков: подставляется по номеру урока, можно изменить. */
const bells: [string, string][] = [
  ['08:30', '09:15'], ['09:25', '10:10'], ['10:30', '11:15'], ['11:35', '12:20'],
  ['12:30', '13:15'], ['13:25', '14:10'], ['14:20', '15:05'], ['15:15', '16:00'],
]

type Draft = {
  id?: string; subject_id: string; teacher_id: string; room_id: string; weekday: number; lesson_number: number
  starts_at: string; ends_at: string; valid_from: string; valid_to: string; week_parity: number
}

const assignmentsQuery = { queryKey: ['teaching-assignments'], queryFn: () => api<Assignment[]>('/teaching-assignments') }

/**
 * Назначения учителей и шаблон расписания действуют в пределах учебного периода. В новой четверти их нет, и выбрать
 * предмет в шаблоне нельзя — предлагаем перенести всё из предыдущего периода одной кнопкой (повторы сервер пропускает).
 * Если предыдущий период — из прошлого учебного года, классы там другие (5А прошлого года → 6А нового): администратор
 * выбирает, из какого класса переносить.
 */
function CarryOverNotice({ period, previous, classId, className, hasAssignments, sourceClasses, onDone }: {
  period: AcademicPeriod; previous: AcademicPeriod; classId: string; className: string; hasAssignments: boolean
  sourceClasses: NamedEntity[]; onDone: () => Promise<void>
}): React.JSX.Element {
  const { showToast } = useToast()
  const crossYear = previous.academic_year_id !== period.academic_year_id
  const [allClasses, setAllClasses] = useState(false)
  const [sourceClass, setSourceClass] = useState('')
  const [pending, setPending] = useState(false)
  useEffect(() => {
    if (crossYear && !sourceClasses.some((item) => item.id === sourceClass)) {
      setSourceClass((sourceClasses.find((item) => item.name === className) ?? sourceClasses[0])?.id ?? '')
    }
  }, [crossYear, sourceClasses, sourceClass, className])
  const missing = !hasAssignments ? 'нет назначений учителей — поэтому в шаблоне нельзя выбрать предмет' : 'пустой шаблон расписания'
  const carryOver = async () => {
    setPending(true)
    try {
      const body = crossYear
        ? { from_period_id: previous.id, class_id: sourceClass, target_class_id: classId }
        : { from_period_id: previous.id, class_id: allClasses ? undefined : classId }
      const result = await mutation<CarryOverResult>(`/academic-periods/${period.id}/carry-over`, 'POST', body)
      const skipped = result.assignments_skipped + result.entries_skipped
      showToast(`Перенесено назначений: ${result.assignments_created}, уроков шаблона: ${result.entries_created}`
        + (skipped ? `. Пропущено ${skipped}: уже есть, слот занят или учитель неактивен` : ''))
      await onDone()
    } catch (caught) {
      showToast(caught instanceof Error ? caught.message : 'Не удалось перенести', 'error')
    } finally {
      setPending(false)
    }
  }
  return (
    <div className="notice info period-hint carry-over" role="status">
      <CopyPlus aria-hidden="true" />
      <span>
        <strong>В периоде «{period.name}» у класса {className} {missing}.</strong> Назначения и шаблон действуют в пределах периода —
        {crossYear
          ? <> это новый учебный год: выберите класс прошлого года, чьи учителя и шаблон достанутся {className}.</>
          : <> перенесите их из «{previous.name}».</>}
        {' '}Уже существующие назначения и занятые слоты не изменятся.
      </span>
      {crossYear ? (
        <label className="checkbox-label">Из класса
          <select aria-label="Класс прошлого года" value={sourceClass} onChange={(event) => setSourceClass(event.target.value)}>
            {sourceClasses.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
        </label>
      ) : (
        <label className="checkbox-label"><input type="checkbox" checked={allClasses} onChange={(event) => setAllClasses(event.target.checked)} />Для всех классов</label>
      )}
      <button type="button" className="button small primary" disabled={pending || (crossYear && !sourceClass)} onClick={carryOver}>{pending ? 'Переносим…' : `Перенести из «${previous.name}»`}</button>
    </div>
  )
}

function EntryDialog({ draft, classId, periodId, periodStart, periodEnd, onClose, onSaved }: {
  draft: Draft | null; classId: string; periodId: string; periodStart: string; periodEnd: string; onClose: () => void; onSaved: (message: string) => void
}): React.JSX.Element {
  const { subjects, rooms } = useDirectory()
  const assignments = useQuery(assignmentsQuery)
  const [value, setValue] = useState<Draft | null>(draft)
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  useEffect(() => { setValue(draft); setError('') }, [draft])
  const classAssignments = (assignments.data ?? []).filter((item) => item.class_id === classId && (!periodId || item.academic_period_id === periodId))
  const subjectIds = new Set(classAssignments.map((item) => item.subject_id))
  const teachers = classAssignments.filter((item) => item.subject_id === value?.subject_id)
  const set = (patch: Partial<Draft>) => setValue((current) => (current ? { ...current, ...patch } : current))
  const save = async (event: FormEvent) => {
    event.preventDefault()
    if (!value) return
    setPending(true)
    setError('')
    const common = {
      teacher_id: value.teacher_id, room_id: value.room_id || undefined, weekday: value.weekday, lesson_number: value.lesson_number,
      starts_at: value.starts_at, ends_at: value.ends_at, valid_from: value.valid_from, valid_to: value.valid_to || undefined, week_parity: value.week_parity,
    }
    try {
      if (value.id) {
        await mutation(`/schedule-entries/${value.id}`, 'PATCH', { ...common, room_id: value.room_id || null, valid_to: value.valid_to || null })
        onSaved('Строка расписания изменена')
      } else {
        await mutation('/schedule-entries', 'POST', { ...common, class_id: classId, subject_id: value.subject_id, academic_period_id: periodId })
        onSaved('Урок добавлен в шаблон')
      }
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Не удалось сохранить')
    } finally {
      setPending(false)
    }
  }
  const remove = async () => {
    if (!value?.id || !window.confirm('Удалить урок из шаблона? Уже созданные уроки с оценками останутся.')) return
    try {
      await mutation(`/schedule-entries/${value.id}`, 'DELETE')
      onSaved('Строка удалена из шаблона')
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Не удалось удалить')
    }
  }
  return (
    <Modal open={Boolean(draft)} onOpenChange={(open) => { if (!open) onClose() }} title={value?.id ? 'Урок в шаблоне' : 'Добавить урок в шаблон'} description="Шаблон повторяется каждую неделю (или через неделю) в пределах срока действия." wide>
      {value && (
        <form className="admin-form" onSubmit={save}>
          <Field label="Предмет *" htmlFor="entry-subject" hint={value.id ? 'Предмет строки не меняется — удалите её и создайте новую' : subjectIds.size ? 'Только предметы, на которые классу назначен учитель в этом периоде'
            : 'В этом периоде у класса нет назначений учителей: перенесите их из прошлого периода (кнопка над сеткой) или добавьте во вкладке «Назначения»'}>
            <select id="entry-subject" required disabled={Boolean(value.id)} value={value.subject_id} onChange={(event) => set({ subject_id: event.target.value, teacher_id: '' })}>
              <option value="">Выберите…</option>
              {options(subjects.data).filter((option) => value.id || subjectIds.has(option.value)).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </Field>
          <Field label="Учитель *" htmlFor="entry-teacher" hint="Только учителя, назначенные на этот класс и предмет">
            <select id="entry-teacher" required value={value.teacher_id} onChange={(event) => set({ teacher_id: event.target.value })}>
              <option value="">Выберите…</option>
              {[...new Map(teachers.map((item) => [item.teacher_id, item])).values()].map((item) => <option key={item.teacher_id} value={item.teacher_id}>{item.teacher_name ?? item.teacher_id}</option>)}
              {value.id && !teachers.some((item) => item.teacher_id === value.teacher_id) && <option value={value.teacher_id}>Текущий учитель</option>}
            </select>
          </Field>
          <Field label="День недели *" htmlFor="entry-day">
            <select id="entry-day" value={value.weekday} onChange={(event) => set({ weekday: Number(event.target.value) })}>
              {weekdays.map((day, index) => <option key={day} value={index + 1}>{day}</option>)}
            </select>
          </Field>
          <Field label="Номер урока *" htmlFor="entry-number">
            <select id="entry-number" value={value.lesson_number} onChange={(event) => { const number = Number(event.target.value); const bell = bells[number - 1]; set({ lesson_number: number, ...(bell ? { starts_at: bell[0], ends_at: bell[1] } : {}) }) }}>
              {Array.from({ length: 10 }, (_, index) => index + 1).map((number) => <option key={number} value={number}>{number}</option>)}
            </select>
          </Field>
          <Field label="Начало *" htmlFor="entry-start"><input id="entry-start" type="time" required value={value.starts_at} onChange={(event) => set({ starts_at: event.target.value })} /></Field>
          <Field label="Окончание *" htmlFor="entry-end"><input id="entry-end" type="time" required value={value.ends_at} onChange={(event) => set({ ends_at: event.target.value })} /></Field>
          <Field label="Кабинет" htmlFor="entry-room">
            <select id="entry-room" value={value.room_id} onChange={(event) => set({ room_id: event.target.value })}>
              <option value="">Не указан</option>{options(rooms.data).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </Field>
          <Field label="Неделя" htmlFor="entry-parity" hint="Чётность — по номеру недели в году">
            <select id="entry-parity" value={value.week_parity} onChange={(event) => set({ week_parity: Number(event.target.value) })}>
              <option value={0}>Каждая неделя</option><option value={1}>Нечётные недели</option><option value={2}>Чётные недели</option>
            </select>
          </Field>
          <Field label="Действует с *" htmlFor="entry-from"><input id="entry-from" type="date" required min={periodStart} max={periodEnd || undefined} value={value.valid_from} onChange={(event) => set({ valid_from: event.target.value })} /></Field>
          <Field label="Действует по" htmlFor="entry-to" hint="Пусто — до конца периода"><input id="entry-to" type="date" min={value.valid_from || periodStart} max={periodEnd || undefined} value={value.valid_to} onChange={(event) => set({ valid_to: event.target.value })} /></Field>
          {error && <div className="form-error" role="alert">{error}</div>}
          <div className="card-actions">
            <button className="button primary" disabled={pending}>{pending ? 'Сохраняем…' : 'Сохранить'}</button>
            {value.id && <button type="button" className="button danger" onClick={remove}><Trash2 aria-hidden="true" />Удалить из шаблона</button>}
            <button type="button" className="button secondary" onClick={onClose}>Отмена</button>
          </div>
        </form>
      )}
    </Modal>
  )
}

export function ScheduleAdmin(): React.JSX.Element {
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const { years, periods, currentPeriod, classOptions, classesOfYear, yearName } = useDirectory()
  const [classId, setClassId] = useState('')
  const [periodId, setPeriodId] = useState('')
  const [draft, setDraft] = useState<Draft | null>(null)
  const [from, setFrom] = useState(dateISO())
  const [to, setTo] = useState('')
  const [allClasses, setAllClasses] = useState(false)
  const [skip, setSkip] = useState('')
  const [generating, setGenerating] = useState(false)
  const [generated, setGenerated] = useState<GenerateLessonsResult | null>(null)
  const choosePeriod = (next: AcademicPeriod) => { setPeriodId(next.id); setFrom(defaultDateIn(next)); setTo(next.ends_on) }
  useEffect(() => {
    if (!periodId && currentPeriod) { setPeriodId(currentPeriod.id); setFrom(defaultDateIn(currentPeriod)); setTo(currentPeriod.ends_on) }
  }, [periodId, currentPeriod])
  const period = periods.data?.find((item) => item.id === periodId)
  // Классы относятся к году: в списке только классы года выбранного периода.
  const yearClasses = useMemo(() => classesOfYear(period?.academic_year_id), [classesOfYear, period?.academic_year_id])
  useEffect(() => {
    if (period && !yearClasses.some((item) => item.id === classId)) setClassId(yearClasses[0]?.id ?? '')
  }, [period, yearClasses, classId])
  const previous = previousPeriod(periods.data, period)
  const assignments = useQuery(assignmentsQuery)
  const hasAssignments = (assignments.data ?? []).some((item) => item.class_id === classId && item.academic_period_id === periodId)
  const className = yearClasses.find((item) => item.id === classId)?.name ?? ''
  const entries = useQuery({
    queryKey: ['admin-schedule-entries', classId, periodId],
    queryFn: () => api<ScheduleEntry[]>(`/admin/schedule-entries${queryString({ class_id: classId, academic_period_id: periodId })}`),
    enabled: Boolean(classId),
  })
  const maxNumber = Math.max(6, ...(entries.data ?? []).map((entry) => entry.lesson_number))
  const grid = useMemo(() => {
    const map = new Map<string, ScheduleEntry[]>()
    entries.data?.forEach((entry) => {
      const key = `${entry.weekday}-${entry.lesson_number}`
      map.set(key, [...(map.get(key) ?? []), entry])
    })
    return map
  }, [entries.data])
  const startDraft = (weekday: number, number: number): Draft => {
    const bell = bells[number - 1] ?? ['08:30', '09:15']
    return { subject_id: '', teacher_id: '', room_id: '', weekday, lesson_number: number, starts_at: bell[0], ends_at: bell[1], valid_from: period?.starts_on ?? dateISO(), valid_to: '', week_parity: 0 }
  }
  const editDraft = (entry: ScheduleEntry): Draft => ({
    id: entry.id, subject_id: entry.subject_id, teacher_id: entry.teacher_id, room_id: entry.room_id ?? '', weekday: entry.weekday, lesson_number: entry.lesson_number,
    starts_at: entry.starts_at, ends_at: entry.ends_at, valid_from: entry.valid_from, valid_to: entry.valid_to ?? '', week_parity: entry.week_parity ?? 0,
  })
  const generate = async (event: FormEvent) => {
    event.preventDefault()
    setGenerating(true)
    try {
      const skipDates = skip.split(/[\s,;]+/).map((item) => item.trim()).filter((item) => /^\d{4}-\d{2}-\d{2}$/.test(item))
      const result = await mutation<GenerateLessonsResult>('/schedule/generate', 'POST', { from, to, class_id: allClasses ? undefined : classId, skip_dates: skipDates })
      setGenerated(result)
      showToast(`Создано уроков: ${result.created}`)
      await queryClient.invalidateQueries({ queryKey: ['schedule'] })
    } catch (caught) {
      showToast(caught instanceof Error ? caught.message : 'Не удалось создать уроки', 'error')
    } finally {
      setGenerating(false)
    }
  }

  return (
    <div className="dash-grid">
      <section className="panel col-12">
        <div className="section-heading">
          <div><h2>Недельный шаблон расписания</h2><p className="muted">Нажмите на пустую ячейку, чтобы добавить урок, или на урок — чтобы изменить его.</p></div>
        </div>
        <div className="filter-bar">
          <label>Учебный период
            <PeriodSelect value={periodId} onChange={choosePeriod} periods={periods.data} years={years.data} />
          </label>
          <label>Класс
            <select value={classId} onChange={(event) => setClassId(event.target.value)} disabled={!yearClasses.length}>
              {classOptions(yearClasses).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </label>
        </div>
        {period && !yearClasses.length && (
          <div className="notice info period-hint" role="status"><CalendarPlus aria-hidden="true" />
            <span>В учебном году «{yearName(period.academic_year_id)}» ещё нет классов. Создайте их во вкладке «Структура», затем переведите туда учеников во вкладке «Зачисление».</span>
          </div>
        )}
        {period && previous && classId && period.status !== 'closed' && assignments.isSuccess && entries.isSuccess && (!hasAssignments || entries.data.length === 0) && (
          <CarryOverNotice period={period} previous={previous} classId={classId} className={className} hasAssignments={hasAssignments}
            sourceClasses={classesOfYear(previous.academic_year_id)}
            onDone={async () => { await queryClient.invalidateQueries({ queryKey: ['teaching-assignments'] }); await queryClient.invalidateQueries({ queryKey: ['admin-schedule-entries'] }) }} />
        )}
        <QueryState isLoading={entries.isLoading} error={entries.error} retry={entries.refetch}>
          <div className="schedule-editor" role="grid" aria-label="Недельный шаблон расписания">
            <div className="head" />
            {weekdays.map((day) => <div className="head" key={day} role="columnheader">{day}</div>)}
            {Array.from({ length: maxNumber }, (_, row) => row + 1).map((number) => (
              <div key={number} style={{ display: 'contents' }} role="row">
                <div className="slot-number" role="rowheader">{number}<small className="muted">{bells[number - 1]?.[0]}</small></div>
                {weekdays.map((_, dayIndex) => {
                  const items = grid.get(`${dayIndex + 1}-${number}`) ?? []
                  return (
                    <div className="slot" key={dayIndex} role="gridcell">
                      {items.map((entry) => (
                        <button type="button" key={entry.id} className="entry-chip" style={subjectStyle(entry.subject_name)} onClick={() => setDraft(editDraft(entry))}>
                          <strong>{entry.subject_name}</strong>
                          <span>{entry.starts_at}–{entry.ends_at}{entry.room_name ? ` · ${entry.room_name}` : ''}</span>
                          <span>{entry.teacher_name}{entry.week_parity ? ` · ${entry.week_parity === 1 ? 'нечёт.' : 'чёт.'}` : ''}</span>
                        </button>
                      ))}
                      {items.length === 0 && (
                        <button type="button" className="icon-button add-slot" aria-label={`Добавить урок: ${weekdays[dayIndex]}, ${number}-й урок`} onClick={() => setDraft(startDraft(dayIndex + 1, number))}><Plus aria-hidden="true" /></button>
                      )}
                    </div>
                  )
                })}
              </div>
            ))}
          </div>
          <button className="button text-button" onClick={() => setDraft(startDraft(1, maxNumber + 1 > 10 ? 10 : maxNumber + 1))}><CalendarPlus aria-hidden="true" />Добавить урок после последнего</button>
        </QueryState>
      </section>
      <section className="panel col-12">
        <div className="section-heading"><div><h2><Wand2 aria-hidden="true" className="inline-icon" /> Создать уроки по шаблону</h2><p className="muted">Уроки появятся в расписании и журнале. Повторный запуск безопасен: существующие уроки не дублируются, ручные изменения сохраняются.</p></div></div>
        <form className="filter-bar" onSubmit={generate}>
          <label>С<input type="date" required value={from} onChange={(event) => setFrom(event.target.value)} /></label>
          <label>По<input type="date" required value={to} onChange={(event) => setTo(event.target.value)} /></label>
          <label className="grow">Пропустить даты (праздники, каникулы)<input value={skip} onChange={(event) => setSkip(event.target.value)} placeholder="2026-11-04, 2026-12-31" /></label>
          <label className="checkbox-label" style={{ flexDirection: 'row', minWidth: 0 }}><input type="checkbox" checked={allClasses} onChange={(event) => setAllClasses(event.target.checked)} />Для всех классов</label>
          <button className="button primary" disabled={generating || !from || !to}>{generating ? 'Создаём…' : 'Создать уроки'}</button>
        </form>
        {generated && (
          <p className="muted" role="status">
            Создано: <strong>{generated.created}</strong> · уже были: <strong>{generated.existing}</strong> · пропущено из-за занятости или снятого назначения учителя: <strong>{generated.conflicts}</strong>
          </p>
        )}
      </section>
      <EntryDialog
        draft={draft}
        classId={classId}
        periodId={periodId}
        periodStart={period?.starts_on ?? ''}
        periodEnd={period?.ends_on ?? ''}
        onClose={() => setDraft(null)}
        onSaved={async (message) => { setDraft(null); showToast(message); await queryClient.invalidateQueries({ queryKey: ['admin-schedule-entries'] }) }}
      />
    </div>
  )
}
