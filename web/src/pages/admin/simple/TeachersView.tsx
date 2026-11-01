import { useEffect, useId, useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { BookOpen, KeyRound, Plus, Trash2, UserPlus } from 'lucide-react'
import type { User } from '../../../api/types'
import { useToast } from '../../../components/toastContext'
import { Field, Modal, QueryState } from '../../../components/UI'
import { options } from '../helpers'
import { createPerson, endAssignment, ensureAssignment, resetPassword, setClassTeacher, type Credential } from './actions'
import { BackToTasks, CredentialsDialog } from './common'
import { useSimpleData } from './data'

type Pair = { classId: string; subjectId: string }

/** Строки «класс — предмет»: учитель математики обычно ведёт её в нескольких классах. */
function PairsEditor({ pairs, onChange, idPrefix }: { pairs: Pair[]; onChange: (pairs: Pair[]) => void; idPrefix: string }): React.JSX.Element {
  const { classes, subjects } = useSimpleData()
  const set = (index: number, patch: Partial<Pair>) => onChange(pairs.map((pair, at) => (at === index ? { ...pair, ...patch } : pair)))
  return (
    <fieldset className="pair-editor span-2">
      <legend>Что ведёт</legend>
      {pairs.map((pair, index) => (
        <div className="pair-row" key={index}>
          <div className="field">
            <label htmlFor={`${idPrefix}-subject-${index}`}>Предмет</label>
            <select id={`${idPrefix}-subject-${index}`} value={pair.subjectId} onChange={(event) => set(index, { subjectId: event.target.value })}>
              <option value="">Выберите…</option>
              {options(subjects.data).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </div>
          <div className="field">
            <label htmlFor={`${idPrefix}-class-${index}`}>Класс</label>
            <select id={`${idPrefix}-class-${index}`} value={pair.classId} onChange={(event) => set(index, { classId: event.target.value })}>
              <option value="">Выберите…</option>
              {classes.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
            </select>
          </div>
          {pairs.length > 1 && <button type="button" className="icon-button" aria-label={`Убрать строку ${index + 1}`} onClick={() => onChange(pairs.filter((_, at) => at !== index))}><Trash2 aria-hidden="true" /></button>}
        </div>
      ))}
      <button type="button" className="button text-button" onClick={() => onChange([...pairs, { classId: pairs.at(-1)?.classId ?? '', subjectId: pairs.at(-1)?.subjectId ?? '' }])}>
        <Plus aria-hidden="true" />Ещё класс или предмет
      </button>
    </fieldset>
  )
}

function AddTeacherDialog({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: (credentials: Credential[]) => void }): React.JSX.Element {
  const id = useId()
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const { classes, period, periodAssignments, activeClassTeacher } = useSimpleData()
  const [name, setName] = useState('')
  const [pairs, setPairs] = useState<Pair[]>([{ classId: '', subjectId: '' }])
  const [homeroom, setHomeroom] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  useEffect(() => { if (open) { setName(''); setPairs([{ classId: '', subjectId: '' }]); setHomeroom(''); setError('') } }, [open])
  const freeClasses = classes.filter((item) => !activeClassTeacher(item.id))
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    const load = pairs.filter((pair) => pair.classId && pair.subjectId)
    if (pairs.some((pair) => Boolean(pair.classId) !== Boolean(pair.subjectId))) { setError('В каждой строке выберите и предмет, и класс — или удалите строку'); return }
    setPending(true)
    setError('')
    let created: Awaited<ReturnType<typeof createPerson>> | null = null
    try {
      created = await createPerson(name, homeroom ? ['teacher', 'class_teacher'] : ['teacher'])
      const teacherId = created.user.teacher_id!
      if (period) for (const pair of load) await ensureAssignment(periodAssignments, period, teacherId, pair.classId, pair.subjectId)
      if (homeroom) await setClassTeacher(created.user, homeroom)
      onDone([created.credential])
    } catch (caught) {
      const message = caught instanceof Error ? caught.message : 'Не удалось сохранить'
      if (created) {
        showToast(`Учитель сохранён, но не всё удалось: ${message}. Проверьте нагрузку в списке учителей.`, 'error')
        onDone([created.credential])
      } else {
        setError(message)
      }
    } finally {
      await queryClient.invalidateQueries({ queryKey: ['users'] })
      await queryClient.invalidateQueries({ queryKey: ['teaching-assignments'] })
      await queryClient.invalidateQueries({ queryKey: ['class-teachers'] })
      setPending(false)
    }
  }
  return (
    <Modal open={open} onOpenChange={(next) => { if (!next) onClose() }} title="Новый учитель" description="Укажите, что и в каких классах он ведёт — тогда предметы сразу можно ставить в расписание." wide>
      <form className="admin-form" onSubmit={submit}>
        <Field label="Фамилия, имя, отчество *" htmlFor={`${id}-name`}>
          <input id={`${id}-name`} required autoFocus value={name} onChange={(event) => setName(event.target.value)} placeholder="Петрова Анна Ивановна" />
        </Field>
        <Field label="Классный руководитель" htmlFor={`${id}-homeroom`} hint="Можно оставить пустым">
          <select id={`${id}-homeroom`} value={homeroom} onChange={(event) => setHomeroom(event.target.value)}>
            <option value="">Нет</option>
            {freeClasses.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
        </Field>
        {period ? <PairsEditor pairs={pairs} onChange={setPairs} idPrefix={id} /> : <p className="muted span-2">Нагрузку можно будет указать, когда настроят текущую четверть.</p>}
        {error && <div className="form-error span-2" role="alert">{error}</div>}
        <div className="card-actions span-2">
          <button className="button primary" disabled={pending}>{pending ? 'Сохраняем…' : 'Сохранить'}</button>
          <button type="button" className="button secondary" onClick={onClose}>Отмена</button>
        </div>
      </form>
    </Modal>
  )
}

function LoadDialog({ teacher, onClose }: { teacher: User | null; onClose: () => void }): React.JSX.Element {
  const id = useId()
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const { period, periodAssignments } = useSimpleData()
  const [pairs, setPairs] = useState<Pair[]>([{ classId: '', subjectId: '' }])
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => { setPairs([{ classId: '', subjectId: '' }]); setError('') }, [teacher])
  const current = periodAssignments.filter((item) => item.teacher_id === teacher?.teacher_id)
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['teaching-assignments'] })
  const add = async (event: FormEvent) => {
    event.preventDefault()
    if (!teacher?.teacher_id || !period) return
    setPending(true)
    setError('')
    try {
      for (const pair of pairs.filter((item) => item.classId && item.subjectId)) await ensureAssignment(periodAssignments, period, teacher.teacher_id, pair.classId, pair.subjectId)
      await refresh()
      setPairs([{ classId: '', subjectId: '' }])
      showToast('Нагрузка сохранена')
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Не удалось сохранить')
    } finally {
      setPending(false)
    }
  }
  return (
    <Modal open={Boolean(teacher)} onOpenChange={(open) => { if (!open) onClose() }} title={`Что ведёт: ${teacher?.display_name ?? ''}`}
      description={period ? `Четверть «${period.name}».` : undefined} wide>
      <div className="stack-form">
        {current.length ? (
          <ul className="simple-list compact">
            {current.map((item) => (
              <li key={item.id}>
                <div><strong>{item.subject_name}</strong><small>{item.class_name} класс</small></div>
                <button type="button" className="button secondary small" onClick={async () => {
                  if (!window.confirm(`Снять с предмета «${item.subject_name}» в ${item.class_name}? Если в расписании класса стоят его уроки по этому предмету, назначьте на них другого учителя.`)) return
                  try { await endAssignment(item); await refresh(); showToast('Предмет снят') } catch (caught) { showToast(caught instanceof Error ? caught.message : 'Не удалось', 'error') }
                }}>Снять</button>
              </li>
            ))}
          </ul>
        ) : <p className="muted">Пока ничего не назначено.</p>}
        {period && (
          <form className="admin-form" onSubmit={add}>
            <PairsEditor pairs={pairs} onChange={setPairs} idPrefix={id} />
            {error && <div className="form-error span-2" role="alert">{error}</div>}
            <div className="card-actions span-2"><button className="button primary" disabled={pending}>{pending ? 'Сохраняем…' : 'Добавить'}</button></div>
          </form>
        )}
      </div>
    </Modal>
  )
}

export function TeachersView({ onBack }: { onBack: () => void }): React.JSX.Element {
  const { showToast } = useToast()
  const { teachers, periodAssignments, classes, activeClassTeacher } = useSimpleData()
  const [adding, setAdding] = useState(false)
  const [editing, setEditing] = useState<User | null>(null)
  const [credentials, setCredentials] = useState<Credential[] | null>(null)
  const list = (teachers.data ?? []).filter((user) => user.is_active && user.teacher_id).sort((a, b) => a.display_name.localeCompare(b.display_name, 'ru'))
  const loadText = (teacherId: string) => {
    const items = periodAssignments.filter((item) => item.teacher_id === teacherId)
    const bySubject = new Map<string, string[]>()
    items.forEach((item) => bySubject.set(item.subject_name ?? '', [...(bySubject.get(item.subject_name ?? '') ?? []), item.class_name ?? '']))
    return [...bySubject].map(([subject, list]) => `${subject} (${list.sort((a, b) => a.localeCompare(b, 'ru', { numeric: true })).join(', ')})`).join('; ')
  }
  const homeroomOf = (teacherId: string) => classes.find((item) => activeClassTeacher(item.id)?.teacher_id === teacherId)?.name
  return (
    <section className="panel">
      <BackToTasks onBack={onBack} />
      <div className="section-heading">
        <div><h2>Учителя</h2><p className="muted">У каждого учителя видно, что и в каких классах он ведёт в текущей четверти.</p></div>
        <button type="button" className="button primary" onClick={() => setAdding(true)}><UserPlus aria-hidden="true" />Добавить учителя</button>
      </div>
      <QueryState isLoading={teachers.isLoading} error={teachers.error} retry={teachers.refetch} isEmpty={!list.length} emptyTitle="Учителей пока нет" emptyText="Нажмите «Добавить учителя».">
        <ul className="simple-list">
          {list.map((user) => {
            const load = loadText(user.teacher_id!)
            const homeroom = homeroomOf(user.teacher_id!)
            return (
              <li key={user.id}>
                <div>
                  <strong>{user.display_name}</strong>
                  <small>{load || 'нагрузка не указана'}{homeroom ? ` · классный руководитель ${homeroom}` : ''} · логин {user.login}</small>
                </div>
                <div className="table-actions">
                  <button type="button" className="button secondary small" onClick={() => setEditing(user)}><BookOpen aria-hidden="true" />Что ведёт</button>
                  <button type="button" className="button secondary small" onClick={async () => {
                    try { setCredentials([await resetPassword(user)]) } catch (caught) { showToast(caught instanceof Error ? caught.message : 'Не удалось выдать пароль', 'error') }
                  }}><KeyRound aria-hidden="true" />Новый пароль</button>
                </div>
              </li>
            )
          })}
        </ul>
      </QueryState>
      <AddTeacherDialog open={adding} onClose={() => setAdding(false)} onDone={(created) => { setAdding(false); setCredentials(created) }} />
      <LoadDialog teacher={editing} onClose={() => setEditing(null)} />
      <CredentialsDialog credentials={credentials} onClose={() => setCredentials(null)} />
    </section>
  )
}
