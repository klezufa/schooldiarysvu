import { useEffect, useId, useMemo, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Info, Plus, Trash2 } from 'lucide-react'
import { api, queryString } from '../../../api/client'
import type { AcademicPeriod, ScheduleEntry } from '../../../api/types'
import { previousPeriod } from '../../../lib/academicYears'
import { subjectStyle } from '../../../lib/subjects'
import { useToast } from '../../../components/toastContext'
import { Field, Modal, QueryState } from '../../../components/UI'
import { options } from '../helpers'
import { CarryOverNotice } from '../scheduleShared'
import { bells, weekdays } from '../scheduleSlots'
import { removeLesson, saveLesson, type LessonSlot } from './actions'
import { BackToTasks } from './common'
import { useSimpleData } from './data'

type Editing = { slot: LessonSlot; entry?: ScheduleEntry }

function LessonDialog({ editing, classId, className, period, onClose, onSaved }: {
  editing: Editing | null; classId: string; className: string; period: AcademicPeriod; onClose: () => void; onSaved: (message: string) => void
}): React.JSX.Element {
  const id = useId()
  const { subjects, rooms, activeTeachers, assignments } = useSimpleData()
  const [subjectId, setSubjectId] = useState('')
  const [teacherId, setTeacherId] = useState('')
  const [roomId, setRoomId] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  useEffect(() => {
    setSubjectId(editing?.entry?.subject_id ?? '')
    setTeacherId(editing?.entry?.teacher_id ?? '')
    setRoomId(editing?.entry?.room_id ?? '')
    setError('')
  }, [editing])
  // Сначала учителя, которые уже ведут этот предмет в классе, затем остальные: обычно выбор — из первой группы.
  const periodAssignments = (assignments.data ?? []).filter((item) => item.academic_period_id === period.id && item.class_id === classId && item.subject_id === subjectId)
  const assigned = new Set(periodAssignments.map((item) => item.teacher_id))
  const teacherOptions = activeTeachers.map((user) => ({ value: user.teacher_id!, label: user.display_name }))
  useEffect(() => {
    if (!editing?.entry && subjectId && !teacherId && periodAssignments.length === 1) setTeacherId(periodAssignments[0]!.teacher_id)
  }, [editing, subjectId, teacherId, periodAssignments])
  const save = async (event: FormEvent) => {
    event.preventDefault()
    if (!editing) return
    setPending(true)
    setError('')
    try {
      await saveLesson(assignments.data ?? [], period, classId, editing.slot, { subjectId, teacherId, roomId }, editing.entry)
      onSaved(editing.entry ? 'Урок изменён' : 'Урок добавлен в расписание')
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Не удалось сохранить')
    } finally {
      setPending(false)
    }
  }
  const remove = async () => {
    if (!editing?.entry || !window.confirm('Убрать урок из расписания? Будущие уроки исчезнут, проведённые и уроки с оценками останутся.')) return
    try {
      await removeLesson(editing.entry)
      onSaved('Урок убран из расписания')
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Не удалось убрать урок')
    }
  }
  const slot = editing?.slot
  const bell = slot ? bells[slot.number - 1] : undefined
  return (
    <Modal open={Boolean(editing)} onOpenChange={(open) => { if (!open) onClose() }}
      title={slot ? `${className}: ${weekdays[slot.weekday - 1]}, ${slot.number}-й урок` : ''}
      description={bell ? `${bell[0]}–${bell[1]}, каждую неделю до конца четверти «${period.name}».` : undefined}>
      {editing && (
        <form className="stack-form" onSubmit={save}>
          <Field label="Предмет *" htmlFor={`${id}-subject`}>
            <select id={`${id}-subject`} required value={subjectId} onChange={(event) => { setSubjectId(event.target.value); setTeacherId('') }}>
              <option value="">Выберите предмет…</option>
              {options(subjects.data).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </Field>
          <Field label="Учитель *" htmlFor={`${id}-teacher`} hint={subjectId && !assigned.size ? 'Выбранный учитель будет вести этот предмет в классе' : undefined}>
            <select id={`${id}-teacher`} required value={teacherId} onChange={(event) => setTeacherId(event.target.value)}>
              <option value="">Выберите учителя…</option>
              {assigned.size > 0 && <optgroup label="Ведут этот предмет в классе">{teacherOptions.filter((item) => assigned.has(item.value)).map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}</optgroup>}
              <optgroup label={assigned.size ? 'Другие учителя' : 'Учителя'}>{teacherOptions.filter((item) => !assigned.has(item.value)).map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}</optgroup>
            </select>
          </Field>
          <Field label="Кабинет" htmlFor={`${id}-room`}>
            <select id={`${id}-room`} value={roomId} onChange={(event) => setRoomId(event.target.value)}>
              <option value="">Не указан</option>
              {options(rooms.data).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </Field>
          {error && <div className="form-error" role="alert">{error}</div>}
          <div className="dialog-actions">
            <button className="button primary" disabled={pending}>{pending ? 'Сохраняем…' : 'Сохранить'}</button>
            {editing.entry && <button type="button" className="button danger" onClick={remove}><Trash2 aria-hidden="true" />Убрать урок</button>}
            <button type="button" className="button secondary" onClick={onClose}>Отмена</button>
          </div>
        </form>
      )}
    </Modal>
  )
}

export function ScheduleView({ initialClassId, onBack }: { initialClassId?: string; onBack: () => void }): React.JSX.Element {
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const { classes, period: currentPeriod, periods, year, assignments, classesOfYear } = useSimpleData()
  const [classId, setClassId] = useState(initialClassId ?? '')
  const [periodId, setPeriodId] = useState('')
  const [editing, setEditing] = useState<Editing | null>(null)
  useEffect(() => { if (!classId && classes[0]) setClassId(classes[0].id) }, [classId, classes])
  useEffect(() => { if (!periodId && currentPeriod) setPeriodId(currentPeriod.id) }, [periodId, currentPeriod])
  const yearPeriods = (periods.data ?? []).filter((item) => item.academic_year_id === year?.id).sort((a, b) => a.starts_on.localeCompare(b.starts_on))
  const period = yearPeriods.find((item) => item.id === periodId)
  const previous = previousPeriod(periods.data, period)
  const className = classes.find((item) => item.id === classId)?.name ?? ''
  const entries = useQuery({
    queryKey: ['admin-schedule-entries', classId, periodId],
    queryFn: () => api<ScheduleEntry[]>(`/admin/schedule-entries${queryString({ class_id: classId, academic_period_id: periodId })}`),
    enabled: Boolean(classId && periodId),
  })
  const rows = Math.min(8, Math.max(6, ...(entries.data ?? []).map((entry) => entry.lesson_number)))
  const grid = useMemo(() => {
    const map = new Map<string, ScheduleEntry>()
    entries.data?.forEach((entry) => map.set(`${entry.weekday}-${entry.lesson_number}`, entry))
    return map
  }, [entries.data])
  const hasAssignments = (assignments.data ?? []).some((item) => item.class_id === classId && item.academic_period_id === periodId)
  const closed = period?.status === 'closed'
  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: ['admin-schedule-entries'] })
    await queryClient.invalidateQueries({ queryKey: ['teaching-assignments'] })
    await queryClient.invalidateQueries({ queryKey: ['schedule'] })
  }
  if (!classes.length || !period) {
    return (
      <section className="panel">
        <BackToTasks onBack={onBack} />
        <h2>Расписание</h2>
        <p className="muted">{!period ? 'Текущая четверть не настроена — её создаёт системный администратор.' : 'Сначала создайте классы в разделе «Классы».'}</p>
      </section>
    )
  }
  return (
    <section className="panel">
      <BackToTasks onBack={onBack} />
      <div className="section-heading">
        <div><h2>Расписание класса</h2><p className="muted">Нажмите на клетку, чтобы поставить урок, или на урок, чтобы изменить его.</p></div>
      </div>
      <div className="filter-bar">
        <label>Класс
          <select value={classId} onChange={(event) => setClassId(event.target.value)}>
            {classes.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
        </label>
        <label>Четверть
          <select value={periodId} onChange={(event) => setPeriodId(event.target.value)}>
            {yearPeriods.map((item) => <option key={item.id} value={item.id}>{item.name}{item.id === currentPeriod?.id ? ' (сейчас)' : ''}{item.status === 'closed' ? ' (закрыта)' : ''}</option>)}
          </select>
        </label>
      </div>
      <div className="notice info" role="note"><Info aria-hidden="true" /><span>Расписание повторяется каждую неделю до конца четверти. Изменения сразу видны ученикам, родителям и в журнале учителя. Проведённые уроки и оценки не меняются.</span></div>
      {closed && <div className="notice" role="status"><Info aria-hidden="true" /><span>Четверть закрыта — расписание можно только посмотреть.</span></div>}
      {!closed && previous && entries.isSuccess && entries.data.length === 0 && (
        <CarryOverNotice period={period} previous={previous} classId={classId} className={className} hasAssignments={hasAssignments}
          sourceClasses={classesOfYear(previous.academic_year_id)} onDone={refresh} plain />
      )}
      <QueryState isLoading={entries.isLoading} error={entries.error} retry={entries.refetch}>
        <div className="schedule-editor simple-schedule" role="grid" aria-label={`Расписание ${className}`}>
          <div className="head" />
          {weekdays.map((day) => <div className="head" key={day} role="columnheader">{day}</div>)}
          {Array.from({ length: rows }, (_, row) => row + 1).map((number) => (
            <div key={number} style={{ display: 'contents' }} role="row">
              <div className="slot-number" role="rowheader">{number}<small className="muted">{bells[number - 1]?.[0]}</small></div>
              {weekdays.map((day, dayIndex) => {
                const entry = grid.get(`${dayIndex + 1}-${number}`)
                const slot = { weekday: dayIndex + 1, number }
                return (
                  <div className="slot" key={day} role="gridcell">
                    {entry ? (
                      <button type="button" className="entry-chip" style={subjectStyle(entry.subject_name)} disabled={closed} onClick={() => setEditing({ slot, entry })}>
                        <strong>{entry.subject_name}</strong>
                        <span>{entry.teacher_name}</span>
                        {entry.room_name && <span>каб. {entry.room_name}</span>}
                      </button>
                    ) : !closed && (
                      <button type="button" className="icon-button add-slot" aria-label={`Поставить урок: ${day}, ${number}-й урок`} onClick={() => setEditing({ slot })}><Plus aria-hidden="true" /></button>
                    )}
                  </div>
                )
              })}
            </div>
          ))}
        </div>
      </QueryState>
      <LessonDialog editing={editing} classId={classId} className={className} period={period} onClose={() => setEditing(null)}
        onSaved={async (message) => { setEditing(null); showToast(message); await refresh() }} />
    </section>
  )
}
