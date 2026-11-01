import { useEffect, useId, useState, type FormEvent } from 'react'
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowRightLeft, KeyRound, Search, UserPlus } from 'lucide-react'
import { apiPage, mutation, queryString } from '../../../api/client'
import type { User } from '../../../api/types'
import { dateISO } from '../../../lib/dates'
import { useToast } from '../../../components/toastContext'
import { Field, Modal, QueryState } from '../../../components/UI'
import { usePeople } from '../helpers'
import { createPerson, linkParent, resetPassword, type Credential } from './actions'
import { BackToTasks, CredentialsDialog } from './common'
import { useSimpleData } from './data'

type ParentMode = 'new' | 'existing' | 'none'

/** Ученик сразу с классом и родителем: три действия расширенного режима (пользователь, зачисление, связь) за один шаг. */
export function AddStudentDialog({ open, classId, onClose, onDone }: {
  open: boolean; classId?: string; onClose: () => void; onDone: (credentials: Credential[]) => void
}): React.JSX.Element {
  const id = useId()
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const { classes } = useSimpleData()
  const parents = usePeople('parent')
  const [name, setName] = useState('')
  const [chosenClass, setChosenClass] = useState('')
  const [parentMode, setParentMode] = useState<ParentMode>('new')
  const [parentName, setParentName] = useState('')
  const [parentId, setParentId] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  useEffect(() => {
    if (open) { setName(''); setChosenClass(classId ?? ''); setParentMode('new'); setParentName(''); setParentId(''); setError('') }
  }, [open, classId])
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setPending(true)
    setError('')
    const created: Credential[] = []
    try {
      const student = await createPerson(name, ['student'], chosenClass)
      created.push(student.credential)
      if (parentMode === 'new') {
        const parent = await createPerson(parentName, ['parent'])
        created.push(parent.credential)
        await linkParent(parent.user, student.user)
      } else if (parentMode === 'existing') {
        const parent = parents.data?.find((item) => item.id === parentId)
        if (parent) await linkParent(parent, student.user)
      }
      await queryClient.invalidateQueries({ queryKey: ['users'] })
      onDone(created)
    } catch (caught) {
      const message = caught instanceof Error ? caught.message : 'Не удалось сохранить'
      if (created.length) {
        // Ученик уже создан: показываем его данные, чтобы не потерять пароль, и говорим, что не получилось с родителем.
        await queryClient.invalidateQueries({ queryKey: ['users'] })
        showToast(`Ученик сохранён, родителя добавить не удалось: ${message}`, 'error')
        onDone(created)
      } else {
        setError(message)
      }
    } finally {
      setPending(false)
    }
  }
  return (
    <Modal open={open} onOpenChange={(next) => { if (!next) onClose() }} title="Новый ученик" description="Логин и временный пароль появятся после сохранения." wide>
      <form className="admin-form" onSubmit={submit}>
        <Field label="Фамилия, имя, отчество ученика *" htmlFor={`${id}-name`}>
          <input id={`${id}-name`} required autoFocus value={name} onChange={(event) => setName(event.target.value)} placeholder="Иванов Иван Петрович" />
        </Field>
        <Field label="Класс *" htmlFor={`${id}-class`} hint={classes.length ? undefined : 'Сначала создайте класс в разделе «Классы»'}>
          <select id={`${id}-class`} required value={chosenClass} onChange={(event) => setChosenClass(event.target.value)}>
            <option value="">Выберите класс…</option>
            {classes.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
        </Field>
        <fieldset className="choice-group span-2">
          <legend>Родитель</legend>
          <label><input type="radio" name={`${id}-parent`} checked={parentMode === 'new'} onChange={() => setParentMode('new')} />Добавить родителя</label>
          <label><input type="radio" name={`${id}-parent`} checked={parentMode === 'existing'} onChange={() => setParentMode('existing')} />Родитель уже есть в дневнике (брат или сестра учатся здесь)</label>
          <label><input type="radio" name={`${id}-parent`} checked={parentMode === 'none'} onChange={() => setParentMode('none')} />Без родителя</label>
        </fieldset>
        {parentMode === 'new' && (
          <Field label="Фамилия, имя, отчество родителя *" htmlFor={`${id}-parent-name`}>
            <input id={`${id}-parent-name`} required value={parentName} onChange={(event) => setParentName(event.target.value)} placeholder="Иванова Мария Сергеевна" />
          </Field>
        )}
        {parentMode === 'existing' && (
          <Field label="Родитель *" htmlFor={`${id}-parent-id`}>
            <select id={`${id}-parent-id`} required value={parentId} onChange={(event) => setParentId(event.target.value)}>
              <option value="">Выберите…</option>
              {parents.data?.filter((item) => item.is_active).map((item) => <option key={item.id} value={item.id}>{item.display_name}</option>)}
            </select>
          </Field>
        )}
        {error && <div className="form-error span-2" role="alert">{error}</div>}
        <div className="card-actions span-2">
          <button className="button primary" disabled={pending}>{pending ? 'Сохраняем…' : 'Сохранить'}</button>
          <button type="button" className="button secondary" onClick={onClose}>Отмена</button>
        </div>
      </form>
    </Modal>
  )
}

function MoveStudentDialog({ student, onClose }: { student: User | null; onClose: () => void }): React.JSX.Element {
  const id = useId()
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const { classes } = useSimpleData()
  const [classId, setClassId] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  useEffect(() => { setClassId(''); setError('') }, [student])
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!student?.student_id) return
    setPending(true)
    setError('')
    try {
      await mutation('/enrollments', 'POST', { student_id: student.student_id, class_id: classId, starts_on: dateISO() })
      await queryClient.invalidateQueries({ queryKey: ['users'] })
      showToast(`${student.display_name}: класс изменён`)
      onClose()
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Не удалось перевести')
    } finally {
      setPending(false)
    }
  }
  return (
    <Modal open={Boolean(student)} onOpenChange={(open) => { if (!open) onClose() }} title={`Другой класс: ${student?.display_name ?? ''}`}
      description="С сегодняшнего дня ученик будет в новом классе. Оценки в прежнем классе сохранятся.">
      <form className="stack-form" onSubmit={submit}>
        <Field label="Новый класс *" htmlFor={`${id}-class`}>
          <select id={`${id}-class`} required value={classId} onChange={(event) => setClassId(event.target.value)}>
            <option value="">Выберите класс…</option>
            {classes.filter((item) => item.name !== student?.class_name).map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
        </Field>
        {error && <div className="form-error" role="alert">{error}</div>}
        <div className="dialog-actions">
          <button className="button primary" disabled={pending}>{pending ? 'Переводим…' : 'Перевести'}</button>
          <button type="button" className="button secondary" onClick={onClose}>Отмена</button>
        </div>
      </form>
    </Modal>
  )
}

export function StudentsView({ onBack }: { onBack: () => void }): React.JSX.Element {
  const { showToast } = useToast()
  const { classes } = useSimpleData()
  const [search, setSearch] = useState('')
  const [classId, setClassId] = useState('')
  const [adding, setAdding] = useState(false)
  const [moving, setMoving] = useState<User | null>(null)
  const [credentials, setCredentials] = useState<Credential[] | null>(null)
  const students = useQuery({
    queryKey: ['users', 'simple-students', search, classId],
    queryFn: () => apiPage<User>(`/users${queryString({ role: 'student', search: search.trim() || undefined, class_id: classId || undefined, limit: 100 })}`),
    placeholderData: keepPreviousData,
  })
  const newPassword = async (user: User) => {
    try {
      setCredentials([await resetPassword(user)])
    } catch (caught) {
      showToast(caught instanceof Error ? caught.message : 'Не удалось выдать пароль', 'error')
    }
  }
  return (
    <section className="panel">
      <BackToTasks onBack={onBack} />
      <div className="section-heading">
        <div><h2>Ученики и родители</h2><p className="muted">Ученик добавляется сразу в класс и вместе с родителем.</p></div>
        <button type="button" className="button primary" onClick={() => setAdding(true)}><UserPlus aria-hidden="true" />Добавить ученика</button>
      </div>
      <div className="filter-bar">
        <label className="grow">Поиск<span className="input-action"><Search aria-hidden="true" /><input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Фамилия или логин" /></span></label>
        <label>Класс
          <select value={classId} onChange={(event) => setClassId(event.target.value)}>
            <option value="">Все классы</option>
            {classes.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
        </label>
      </div>
      <QueryState isLoading={students.isLoading} error={students.error} retry={students.refetch} isEmpty={!students.data?.items.length}
        emptyTitle="Учеников пока нет" emptyText="Нажмите «Добавить ученика».">
        <ul className="simple-list">
          {students.data?.items.map((user) => (
            <li key={user.id}>
              <div><strong>{user.display_name}</strong><small>{user.class_name ? `${user.class_name} класс` : 'без класса'} · логин {user.login}</small></div>
              <div className="table-actions">
                <button type="button" className="button secondary small" onClick={() => setMoving(user)}><ArrowRightLeft aria-hidden="true" />Другой класс</button>
                <button type="button" className="button secondary small" onClick={() => void newPassword(user)}><KeyRound aria-hidden="true" />Новый пароль</button>
              </div>
            </li>
          ))}
        </ul>
        {(students.data?.total ?? 0) > 100 && <p className="muted">Показаны первые 100 — уточните поиск.</p>}
      </QueryState>
      <AddStudentDialog open={adding} onClose={() => setAdding(false)} onDone={(created) => { setAdding(false); setCredentials(created) }} />
      <MoveStudentDialog student={moving} onClose={() => setMoving(null)} />
      <CredentialsDialog credentials={credentials} onClose={() => setCredentials(null)} />
    </section>
  )
}
