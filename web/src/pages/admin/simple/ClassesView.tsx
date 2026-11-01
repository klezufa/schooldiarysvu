import { useEffect, useId, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { CalendarDays, ChevronRight, Plus, School, UserPlus } from 'lucide-react'
import { api, mutation } from '../../../api/client'
import type { NamedEntity, Student } from '../../../api/types'
import { dateISO } from '../../../lib/dates'
import { useToast } from '../../../components/toastContext'
import { Field, Modal, QueryState } from '../../../components/UI'
import { options } from '../helpers'
import { endAssignment, ensureAssignment, setClassTeacher, type Credential } from './actions'
import { BackToTasks, CredentialsDialog } from './common'
import { usePeriodEntries, useSchoolAnalytics, useSimpleData } from './data'
import { AddStudentDialog } from './StudentsView'

function CreateClassDialog({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: (created: NamedEntity) => void }): React.JSX.Element {
  const id = useId()
  const queryClient = useQueryClient()
  const { year, activeTeachers } = useSimpleData()
  const [name, setName] = useState('')
  const [teacherId, setTeacherId] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  useEffect(() => { if (open) { setName(''); setTeacherId(''); setError('') } }, [open])
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!year) return
    setPending(true)
    setError('')
    try {
      const created = await mutation<NamedEntity>('/classes', 'POST', { academic_year_id: year.id, name: name.trim() })
      const teacher = activeTeachers.find((item) => item.id === teacherId)
      if (teacher) await setClassTeacher(teacher, created.id)
      await queryClient.invalidateQueries({ queryKey: ['classes'] })
      await queryClient.invalidateQueries({ queryKey: ['class-teachers'] })
      await queryClient.invalidateQueries({ queryKey: ['users'] })
      onCreated(created)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Не удалось создать класс')
    } finally {
      setPending(false)
    }
  }
  return (
    <Modal open={open} onOpenChange={(next) => { if (!next) onClose() }} title="Новый класс" description={year ? `Учебный год ${year.name}.` : 'Учебный год ещё не настроен.'}>
      <form className="stack-form" onSubmit={submit}>
        <Field label="Название класса *" htmlFor={`${id}-name`} hint="Например, 5А">
          <input id={`${id}-name`} required autoFocus maxLength={20} value={name} onChange={(event) => setName(event.target.value)} />
        </Field>
        <Field label="Классный руководитель" htmlFor={`${id}-teacher`} hint="Можно назначить позже">
          <select id={`${id}-teacher`} value={teacherId} onChange={(event) => setTeacherId(event.target.value)}>
            <option value="">Пока не назначен</option>
            {activeTeachers.map((item) => <option key={item.id} value={item.id}>{item.display_name}</option>)}
          </select>
        </Field>
        {error && <div className="form-error" role="alert">{error}</div>}
        <div className="dialog-actions">
          <button className="button primary" disabled={pending || !year}>{pending ? 'Создаём…' : 'Создать класс'}</button>
          <button type="button" className="button secondary" onClick={onClose}>Отмена</button>
        </div>
      </form>
    </Modal>
  )
}

export function ClassesView({ onBack, onOpen }: { onBack: () => void; onOpen: (classId: string) => void }): React.JSX.Element {
  const { classes, period, activeClassTeacher, activeTeachers } = useSimpleData()
  const analytics = useSchoolAnalytics()
  const entries = usePeriodEntries(period?.id)
  const [creating, setCreating] = useState(false)
  const students = (classId: string) => analytics.data?.attendance_today.find((item) => item.class_id === classId)?.students
  const teacherName = (classId: string) => {
    const link = activeClassTeacher(classId)
    return link ? activeTeachers.find((item) => item.teacher_id === link.teacher_id)?.display_name : undefined
  }
  const lessons = (classId: string) => (entries.data ?? []).filter((entry) => entry.class_id === classId).length
  return (
    <section className="panel">
      <BackToTasks onBack={onBack} />
      <div className="section-heading">
        <div><h2>Классы</h2><p className="muted">Нажмите на класс, чтобы увидеть учеников, учителей и расписание.</p></div>
        <button type="button" className="button primary" onClick={() => setCreating(true)}><Plus aria-hidden="true" />Создать класс</button>
      </div>
      {classes.length === 0 ? <div className="empty-state"><School aria-hidden="true" /><h3>Классов пока нет</h3><p>Нажмите «Создать класс».</p></div> : (
        <ul className="class-cards">
          {classes.map((item) => (
            <li key={item.id}>
              <button type="button" className="class-card" onClick={() => onOpen(item.id)} aria-label={`Класс ${item.name}`}>
                <strong>{item.name}</strong>
                <span>Учеников: {students(item.id) ?? '—'}</span>
                <span>{teacherName(item.id) ? `Кл. руководитель: ${teacherName(item.id)}` : 'Классный руководитель не назначен'}</span>
                <span className={lessons(item.id) ? '' : 'warning-text'}>{lessons(item.id) ? `Уроков в неделю: ${lessons(item.id)}` : 'Расписания нет'}</span>
                <ChevronRight aria-hidden="true" className="class-card-arrow" />
              </button>
            </li>
          ))}
        </ul>
      )}
      <CreateClassDialog open={creating} onClose={() => setCreating(false)} onCreated={(created) => { setCreating(false); onOpen(created.id) }} />
    </section>
  )
}

export function ClassDetail({ classId, onBack, onSchedule }: { classId: string; onBack: () => void; onSchedule: (classId: string) => void }): React.JSX.Element {
  const id = useId()
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const { classes, period, periodAssignments, activeTeachers, activeClassTeacher, subjects } = useSimpleData()
  const current = classes.find((item) => item.id === classId)
  const roster = useQuery({ queryKey: ['class-students', classId], queryFn: () => api<Student[]>(`/classes/${classId}/students?date=${dateISO()}`) })
  const [adding, setAdding] = useState(false)
  const [credentials, setCredentials] = useState<Credential[] | null>(null)
  const [homeroom, setHomeroom] = useState('')
  const [subjectId, setSubjectId] = useState('')
  const [teacherId, setTeacherId] = useState('')
  const link = activeClassTeacher(classId)
  const homeroomTeacher = link ? activeTeachers.find((item) => item.teacher_id === link.teacher_id) : undefined
  const load = periodAssignments.filter((item) => item.class_id === classId).sort((a, b) => (a.subject_name ?? '').localeCompare(b.subject_name ?? '', 'ru'))
  const refresh = async (...keys: string[]) => { for (const key of keys) await queryClient.invalidateQueries({ queryKey: [key] }) }
  const run = async (action: () => Promise<void>, message: string) => {
    try { await action(); showToast(message) } catch (caught) { showToast(caught instanceof Error ? caught.message : 'Не удалось сохранить', 'error') }
  }
  if (!current) return <section className="panel"><BackToTasks onBack={onBack} label="К списку классов" /><p>Класс не найден.</p></section>
  return (
    <section className="panel">
      <BackToTasks onBack={onBack} label="К списку классов" />
      <div className="section-heading">
        <div><h2>{current.name} класс</h2><p className="muted">{period ? `Четверть «${period.name}»` : 'Четверть не настроена'}</p></div>
        <button type="button" className="button primary" onClick={() => onSchedule(classId)}><CalendarDays aria-hidden="true" />Расписание класса</button>
      </div>
      <div className="class-detail">
        <div className="class-block">
          <h3>Классный руководитель</h3>
          <p>{homeroomTeacher?.display_name ?? 'Не назначен'}</p>
          <form className="inline-form" onSubmit={(event) => {
            event.preventDefault()
            const teacher = activeTeachers.find((item) => item.id === homeroom)
            if (teacher) void run(async () => { await setClassTeacher(teacher, classId, link); await refresh('class-teachers', 'users'); setHomeroom('') }, 'Классный руководитель назначен')
          }}>
            <label htmlFor={`${id}-homeroom`} className="sr-only">Учитель</label>
            <select id={`${id}-homeroom`} value={homeroom} onChange={(event) => setHomeroom(event.target.value)}>
              <option value="">Выберите учителя…</option>
              {activeTeachers.filter((item) => item.teacher_id !== link?.teacher_id).map((item) => <option key={item.id} value={item.id}>{item.display_name}</option>)}
            </select>
            <button className="button secondary small" disabled={!homeroom}>{homeroomTeacher ? 'Сменить' : 'Назначить'}</button>
          </form>
        </div>
        <div className="class-block">
          <h3>Предметы и учителя</h3>
          {load.length ? (
            <ul className="simple-list compact">
              {load.map((item) => (
                <li key={item.id}>
                  <div><strong>{item.subject_name}</strong><small>{item.teacher_name}</small></div>
                  <button type="button" className="button secondary small" onClick={() => {
                    if (window.confirm(`Снять ${item.teacher_name} с предмета «${item.subject_name}» в ${current.name}?`)) void run(async () => { await endAssignment(item); await refresh('teaching-assignments') }, 'Учитель снят с предмета')
                  }}>Снять</button>
                </li>
              ))}
            </ul>
          ) : <p className="muted">Учителя не назначены. Добавьте их здесь или прямо в расписании.</p>}
          {period && (
            <form className="inline-form" onSubmit={(event) => {
              event.preventDefault()
              const teacher = activeTeachers.find((item) => item.id === teacherId)
              if (teacher?.teacher_id && subjectId) void run(async () => { await ensureAssignment(periodAssignments, period, teacher.teacher_id!, classId, subjectId); await refresh('teaching-assignments'); setSubjectId(''); setTeacherId('') }, 'Учитель назначен')
            }}>
              <label htmlFor={`${id}-subject`} className="sr-only">Предмет</label>
              <select id={`${id}-subject`} value={subjectId} onChange={(event) => setSubjectId(event.target.value)}>
                <option value="">Предмет…</option>
                {options(subjects.data).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
              <label htmlFor={`${id}-teacher`} className="sr-only">Учитель</label>
              <select id={`${id}-teacher`} value={teacherId} onChange={(event) => setTeacherId(event.target.value)}>
                <option value="">Учитель…</option>
                {activeTeachers.map((item) => <option key={item.id} value={item.id}>{item.display_name}</option>)}
              </select>
              <button className="button secondary small" disabled={!subjectId || !teacherId}>Назначить</button>
            </form>
          )}
        </div>
        <div className="class-block span-2">
          <div className="section-heading">
            <h3>Ученики ({roster.data?.length ?? '…'})</h3>
            <button type="button" className="button secondary small" onClick={() => setAdding(true)}><UserPlus aria-hidden="true" />Добавить ученика в {current.name}</button>
          </div>
          <QueryState isLoading={roster.isLoading} error={roster.error} retry={roster.refetch} isEmpty={!roster.data?.length} emptyTitle="В классе пока нет учеников">
            <ol className="roster">{roster.data?.map((student) => <li key={student.id}>{student.display_name}</li>)}</ol>
          </QueryState>
        </div>
      </div>
      <AddStudentDialog open={adding} classId={classId} onClose={() => setAdding(false)}
        onDone={async (created) => { setAdding(false); setCredentials(created); await refresh('class-students', 'analytics-school') }} />
      <CredentialsDialog credentials={credentials} onClose={() => setCredentials(null)} />
    </section>
  )
}
