import { useId, useState, type FormEvent } from 'react'
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, Plus, Search } from 'lucide-react'
import { apiPage, mutation, queryString } from '../../../api/client'
import type { User } from '../../../api/types'
import { roleNames } from '../../../api/types'
import { useToast } from '../../../components/toastContext'
import { QueryState } from '../../../components/UI'
import { resetPassword, type Credential } from './actions'
import { BackToTasks, CredentialsDialog } from './common'
import { useSimpleData } from './data'
import { standardSubjects } from './setup'

function AddByName({ label, placeholder, onAdd }: { label: string; placeholder: string; onAdd: (name: string) => Promise<void> }): React.JSX.Element {
  const id = useId()
  const [name, setName] = useState('')
  const [pending, setPending] = useState(false)
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!name.trim()) return
    setPending(true)
    try { await onAdd(name.trim()); setName('') } finally { setPending(false) }
  }
  return (
    <form className="inline-form" onSubmit={submit}>
      <label htmlFor={id} className="sr-only">{label}</label>
      <input id={id} value={name} placeholder={placeholder} onChange={(event) => setName(event.target.value)} />
      <button className="button secondary small" disabled={pending || !name.trim()}><Plus aria-hidden="true" />{label}</button>
    </form>
  )
}

export function SubjectsView({ onBack }: { onBack: () => void }): React.JSX.Element {
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const { subjects, rooms } = useSimpleData()
  const existing = new Set((subjects.data ?? []).map((item) => item.name.toLowerCase()))
  const missing = standardSubjects.filter((name) => !existing.has(name.toLowerCase()))
  const [picked, setPicked] = useState<string[]>([])
  const create = (kind: 'subjects' | 'rooms') => async (name: string) => {
    try {
      await mutation(`/${kind}`, 'POST', { name })
      await queryClient.invalidateQueries({ queryKey: [kind] })
      showToast(kind === 'subjects' ? `Предмет «${name}» добавлен` : `Кабинет «${name}» добавлен`)
    } catch (caught) {
      showToast(caught instanceof Error ? caught.message : 'Не удалось добавить', 'error')
    }
  }
  const addPicked = async () => {
    for (const name of picked) await create('subjects')(name)
    setPicked([])
  }
  return (
    <section className="panel">
      <BackToTasks onBack={onBack} />
      <div className="class-detail">
        <div className="class-block">
          <h2>Предметы</h2>
          <QueryState isLoading={subjects.isLoading} error={subjects.error} retry={subjects.refetch} isEmpty={!subjects.data?.length} emptyTitle="Предметов пока нет">
            <ul className="tag-list">{subjects.data?.map((item) => <li key={item.id}>{item.name}</li>)}</ul>
          </QueryState>
          <AddByName label="Добавить предмет" placeholder="Название предмета" onAdd={create('subjects')} />
          {missing.length > 0 && (
            <details className="standard-subjects" open={!subjects.data?.length}>
              <summary>Выбрать из типового списка</summary>
              <div className="checkbox-group">
                {missing.map((name) => (
                  <label key={name}><input type="checkbox" checked={picked.includes(name)} onChange={(event) => setPicked(event.target.checked ? [...picked, name] : picked.filter((item) => item !== name))} />{name}</label>
                ))}
              </div>
              <div className="card-actions">
                <button type="button" className="button secondary small" onClick={() => setPicked(missing)}>Отметить все</button>
                <button type="button" className="button primary small" disabled={!picked.length} onClick={() => void addPicked()}>Добавить отмеченные ({picked.length})</button>
              </div>
            </details>
          )}
        </div>
        <div className="class-block">
          <h2>Кабинеты</h2>
          <p className="muted">Необязательно. Если указать кабинет в расписании, дневник не даст поставить два урока в один кабинет одновременно.</p>
          <ul className="tag-list">{rooms.data?.map((item) => <li key={item.id}>{item.name}</li>)}</ul>
          <AddByName label="Добавить кабинет" placeholder="Например, 204" onAdd={create('rooms')} />
        </div>
      </div>
    </section>
  )
}

export function PasswordsView({ onBack }: { onBack: () => void }): React.JSX.Element {
  const { showToast } = useToast()
  const [search, setSearch] = useState('')
  const [credentials, setCredentials] = useState<Credential[] | null>(null)
  const users = useQuery({
    queryKey: ['users', 'simple-search', search],
    queryFn: () => apiPage<User>(`/users${queryString({ search: search.trim(), limit: 20 })}`),
    enabled: search.trim().length >= 2,
    placeholderData: keepPreviousData,
  })
  return (
    <section className="panel">
      <BackToTasks onBack={onBack} />
      <div className="section-heading"><div><h2>Пароли и вход</h2><p className="muted">Если человек забыл пароль или не может войти — найдите его и выдайте новый временный пароль.</p></div></div>
      <div className="filter-bar">
        <label className="grow">Кого ищем<span className="input-action"><Search aria-hidden="true" /><input value={search} autoFocus onChange={(event) => setSearch(event.target.value)} placeholder="Фамилия или логин, от двух букв" /></span></label>
      </div>
      {search.trim().length >= 2 && (
        <QueryState isLoading={users.isLoading} error={users.error} retry={users.refetch} isEmpty={!users.data?.items.length} emptyTitle="Никого не нашли" emptyText="Проверьте написание фамилии.">
          <ul className="simple-list">
            {users.data?.items.map((user) => (
              <li key={user.id}>
                <div><strong>{user.display_name}</strong><small>{user.roles.map((role) => roleNames[role]).join(', ')}{user.class_name ? ` · ${user.class_name}` : ''} · логин {user.login}{user.is_active ? '' : ' · отключён'}</small></div>
                <button type="button" className="button secondary small" disabled={!user.is_active} onClick={async () => {
                  try { setCredentials([await resetPassword(user)]) } catch (caught) { showToast(caught instanceof Error ? caught.message : 'Не удалось выдать пароль', 'error') }
                }}><KeyRound aria-hidden="true" />Выдать новый пароль</button>
              </li>
            ))}
          </ul>
        </QueryState>
      )}
      <CredentialsDialog credentials={credentials} onClose={() => setCredentials(null)} />
    </section>
  )
}
