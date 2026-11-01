import { useEffect, useState, type FormEvent } from 'react'
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, LogOut, Pencil, RotateCcw, Search, UserMinus, UserPlus, Wand2 } from 'lucide-react'
import { ApiError, apiPage, mutation } from '../../api/client'
import type { PasswordReset, Role, User } from '../../api/types'
import { privilegedRoles, regularRoles, roleNames } from '../../api/types'
import { useAuth } from '../../authContext'
import { formatDate } from '../../lib/dates'
import { useToast } from '../../components/toastContext'
import { Field, Modal, QueryState, StatusBadge } from '../../components/UI'
import { RolePills, SecretDialog } from './shared'
import { randomPassword, suggestLogin, useDirectory, useIsSystemAdmin } from './helpers'

const pageSize = 25

type Secret = Parameters<typeof SecretDialog>[0]['secret']

function useDebounced<T>(value: T, delay = 300): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay)
    return () => window.clearTimeout(timer)
  }, [value, delay])
  return debounced
}

function RoleChoices({ value, onChange, allowPrivileged }: { value: Role[]; onChange: (roles: Role[]) => void; allowPrivileged: boolean }): React.JSX.Element {
  const roles = allowPrivileged ? [...privilegedRoles, ...regularRoles] : regularRoles
  return (
    <fieldset className="checkbox-group">
      <legend>Роли *</legend>
      {roles.map((role) => (
        <label key={role}>
          <input type="checkbox" checked={value.includes(role)} onChange={(event) => onChange(event.target.checked ? [...value, role] : value.filter((item) => item !== role))} />
          {roleNames[role]}
        </label>
      ))}
      <small>{allowPrivileged ? 'Административные роли может выдавать только системный администратор.' : 'Учитель может одновременно быть классным руководителем. Административные роли выдаёт системный администратор.'}</small>
    </fieldset>
  )
}

function CreateUserDialog({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: (user: User, password: string) => void }): React.JSX.Element {
  const system = useIsSystemAdmin()
  const { classOptions, openClasses } = useDirectory()
  const [name, setName] = useState('')
  const [login, setLogin] = useState('')
  const [loginTouched, setLoginTouched] = useState(false)
  const [password, setPassword] = useState(randomPassword)
  const [roles, setRoles] = useState<Role[]>(['student'])
  const [classId, setClassId] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  useEffect(() => {
    if (open) { setName(''); setLogin(''); setLoginTouched(false); setPassword(randomPassword()); setRoles(['student']); setClassId(''); setError('') }
  }, [open])
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!roles.length) { setError('Выберите хотя бы одну роль'); return }
    setPending(true)
    setError('')
    try {
      const created = await mutation<User>('/users', 'POST', {
        login: login.trim().toLowerCase(), password, display_name: name.trim(), roles,
        class_id: roles.includes('student') && classId ? classId : undefined,
      })
      onCreated(created, password)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Не удалось создать пользователя')
    } finally {
      setPending(false)
    }
  }
  return (
    <Modal open={open} onOpenChange={(next) => { if (!next) onClose() }} title="Новый пользователь" description="При первом входе пользователь сменит временный пароль на свой." wide>
      <form className="admin-form" onSubmit={submit}>
        <Field label="Фамилия, имя, отчество *" htmlFor="new-name">
          <input id="new-name" required value={name} autoFocus onChange={(event) => { setName(event.target.value); if (!loginTouched) setLogin(suggestLogin(event.target.value)) }} />
        </Field>
        <Field label="Логин *" htmlFor="new-login" hint="Подставляется из ФИО; можно изменить">
          <input id="new-login" required value={login} onChange={(event) => { setLogin(event.target.value); setLoginTouched(true) }} />
        </Field>
        <Field label="Временный пароль *" htmlFor="new-password" hint="Не менее 12 символов. Будет показан после создания">
          <div className="input-action">
            <input id="new-password" required minLength={12} value={password} onChange={(event) => setPassword(event.target.value)} />
            <button type="button" className="icon-button" aria-label="Сгенерировать пароль" title="Сгенерировать пароль" onClick={() => setPassword(randomPassword())}><Wand2 aria-hidden="true" /></button>
          </div>
        </Field>
        {roles.includes('student') ? (
          <Field label="Сразу зачислить в класс" htmlFor="new-class" hint="С сегодняшнего дня; в класс следующего учебного года — с первого дня того года">
            <select id="new-class" value={classId} onChange={(event) => setClassId(event.target.value)}>
              <option value="">Без класса</option>
              {classOptions(openClasses).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </Field>
        ) : <div />}
        <div style={{ gridColumn: '1 / -1' }}><RoleChoices value={roles} onChange={setRoles} allowPrivileged={system} /></div>
        {error && <div className="form-error" role="alert">{error}</div>}
        <div className="card-actions">
          <button className="button primary" disabled={pending}><UserPlus aria-hidden="true" />{pending ? 'Создаём…' : 'Создать'}</button>
          <button type="button" className="button secondary" onClick={onClose}>Отмена</button>
        </div>
      </form>
    </Modal>
  )
}

function EditUserDialog({ user, onClose, onSaved }: { user: User | null; onClose: () => void; onSaved: () => void }): React.JSX.Element {
  const system = useIsSystemAdmin()
  const [name, setName] = useState('')
  const [roles, setRoles] = useState<Role[]>([])
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  useEffect(() => { if (user) { setName(user.display_name); setRoles(user.roles); setError('') } }, [user])
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!user) return
    if (!roles.length) { setError('Выберите хотя бы одну роль'); return }
    setPending(true)
    try {
      await mutation(`/users/${user.id}`, 'PATCH', { display_name: name.trim(), roles })
      onSaved()
    } catch (caught) {
      setError(caught instanceof ApiError && caught.status === 403 ? 'Недостаточно прав: административными учётными записями управляет системный администратор.' : caught instanceof Error ? caught.message : 'Не удалось сохранить')
    } finally {
      setPending(false)
    }
  }
  return (
    <Modal open={Boolean(user)} onOpenChange={(next) => { if (!next) onClose() }} title="Изменить пользователя" description={user ? `Логин: ${user.login}` : undefined} wide>
      <form className="admin-form" onSubmit={submit}>
        <Field label="Фамилия, имя, отчество" htmlFor="edit-name"><input id="edit-name" required value={name} autoFocus onChange={(event) => setName(event.target.value)} /></Field>
        <div />
        <div style={{ gridColumn: '1 / -1' }}><RoleChoices value={roles} onChange={setRoles} allowPrivileged={system} /></div>
        {error && <div className="form-error" role="alert">{error}</div>}
        <div className="card-actions">
          <button className="button primary" disabled={pending}>{pending ? 'Сохраняем…' : 'Сохранить'}</button>
          <button type="button" className="button secondary" onClick={onClose}>Отмена</button>
        </div>
      </form>
    </Modal>
  )
}

export function PeopleAdmin({ startWithCreate = false }: { startWithCreate?: boolean }): React.JSX.Element {
  const { user: me } = useAuth()
  const system = useIsSystemAdmin()
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const { classOptions } = useDirectory()
  const [search, setSearch] = useState('')
  const [role, setRole] = useState('')
  const [status, setStatus] = useState('active')
  const [classId, setClassId] = useState('')
  const [page, setPage] = useState(0)
  const [creating, setCreating] = useState(startWithCreate)
  const [editing, setEditing] = useState<User | null>(null)
  const [secret, setSecret] = useState<Secret>(null)
  const [busy, setBusy] = useState('')
  const term = useDebounced(search.trim())
  useEffect(() => setPage(0), [term, role, status, classId])
  const params = new URLSearchParams({ limit: String(pageSize), offset: String(page * pageSize), status })
  if (term) params.set('search', term)
  if (role) params.set('role', role)
  if (classId) params.set('class_id', classId)
  const users = useQuery({ queryKey: ['users', params.toString()], queryFn: () => apiPage<User>(`/users?${params}`), placeholderData: keepPreviousData })
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['users'] })
  const pages = Math.max(1, Math.ceil((users.data?.total ?? 0) / pageSize))
  const canManage = (target: User) => system || !target.roles.some((item) => privilegedRoles.includes(item))

  const run = async (target: User, key: string, action: () => Promise<void>) => {
    setBusy(`${target.id}:${key}`)
    try { await action() } catch (caught) { showToast(caught instanceof Error ? caught.message : 'Операция не выполнена', 'error') } finally { setBusy('') }
  }
  const resetPassword = (target: User) => run(target, 'reset', async () => {
    if (!window.confirm(`Выдать новый временный пароль пользователю ${target.display_name}? Все его сеансы будут завершены.`)) return
    const result = await mutation<PasswordReset>(`/users/${target.id}/password-reset`, 'POST')
    setSecret({ title: 'Новый временный пароль', value: result.password, text: `${target.display_name} (логин ${target.login}) сменит его при первом входе.` })
    await refresh()
  })
  const revoke = (target: User) => run(target, 'revoke', async () => {
    await mutation(`/users/${target.id}/revoke-sessions`, 'POST')
    showToast('Все сеансы пользователя завершены')
  })
  const deactivate = (target: User) => run(target, 'deactivate', async () => {
    if (!window.confirm(`Деактивировать ${target.display_name}? Вход будет запрещён, история оценок и действий сохранится.`)) return
    await mutation(`/users/${target.id}`, 'DELETE')
    showToast('Пользователь деактивирован')
    await refresh()
  })
  const reactivate = (target: User) => run(target, 'reactivate', async () => {
    await mutation(`/users/${target.id}/reactivate`, 'POST')
    showToast('Пользователь снова может входить')
    await refresh()
  })

  return (
    <section className="panel">
      <div className="section-heading">
        <div><h2>Пользователи</h2><p className="muted">Всего найдено: {users.data?.total ?? '…'}</p></div>
        <button className="button primary" onClick={() => setCreating(true)}><UserPlus aria-hidden="true" />Добавить пользователя</button>
      </div>
      <div className="filter-bar" role="search">
        <label className="grow">Поиск по ФИО или логину
          <span className="input-with-icon"><Search aria-hidden="true" /><input type="search" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Например, Иванов" /></span>
        </label>
        <label>Роль
          <select value={role} onChange={(event) => setRole(event.target.value)}>
            <option value="">Все роли</option>
            {[...privilegedRoles, ...regularRoles].map((item) => <option key={item} value={item}>{roleNames[item]}</option>)}
          </select>
        </label>
        <label>Состояние
          <select value={status} onChange={(event) => setStatus(event.target.value)}>
            <option value="active">Активные</option><option value="inactive">Деактивированные</option><option value="all">Все</option>
          </select>
        </label>
        <label>Класс
          <select value={classId} onChange={(event) => setClassId(event.target.value)}>
            <option value="">Любой</option>
            {classOptions().map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
          </select>
        </label>
      </div>
      <QueryState isLoading={users.isLoading} error={users.error} retry={users.refetch} isEmpty={!users.data?.items.length} emptyTitle="Никого не найдено" emptyText="Измените условия поиска или добавьте пользователя.">
        <div className={`responsive-table ${users.isFetching ? 'refreshing' : ''}`}>
          <table className="users-table">
            <thead><tr><th>Пользователь</th><th>Роли</th><th>Класс</th><th>Последний вход</th><th>Состояние</th><th><span className="sr-only">Действия</span></th></tr></thead>
            <tbody>
              {users.data?.items.map((target) => {
                const manageable = canManage(target) && target.id !== me?.id
                return (
                  <tr key={target.id}>
                    <td><div className="person"><span className="avatar" aria-hidden="true">{target.display_name.slice(0, 1)}</span><div><strong>{target.display_name}</strong><small>{target.login}</small></div></div></td>
                    <td><RolePills roles={target.roles} /></td>
                    <td>{target.class_name ?? '—'}</td>
                    <td>{target.last_login_at ? formatDate(target.last_login_at, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }) : <span className="muted">не входил</span>}</td>
                    <td>
                      {!target.is_active ? <StatusBadge>Деактивирован</StatusBadge> : target.must_change_password ? <StatusBadge tone="warning">Временный пароль</StatusBadge> : <StatusBadge tone="success">Активен</StatusBadge>}
                    </td>
                    <td className="actions">
                      {manageable ? (
                        <div className="table-actions">
                          {target.is_active && <button className="icon-button" title="Изменить" aria-label={`Изменить ${target.display_name}`} onClick={() => setEditing(target)}><Pencil aria-hidden="true" /></button>}
                          {target.is_active && <button className="icon-button" title="Сбросить пароль" aria-label={`Сбросить пароль ${target.display_name}`} disabled={busy === `${target.id}:reset`} onClick={() => resetPassword(target)}><KeyRound aria-hidden="true" /></button>}
                          {target.is_active && <button className="icon-button" title="Завершить все сеансы" aria-label={`Завершить сеансы ${target.display_name}`} disabled={busy === `${target.id}:revoke`} onClick={() => revoke(target)}><LogOut aria-hidden="true" /></button>}
                          {target.is_active
                            ? <button className="icon-button danger-text" title="Деактивировать" aria-label={`Деактивировать ${target.display_name}`} disabled={busy === `${target.id}:deactivate`} onClick={() => deactivate(target)}><UserMinus aria-hidden="true" /></button>
                            : <button className="button small secondary" disabled={busy === `${target.id}:reactivate`} onClick={() => reactivate(target)}><RotateCcw aria-hidden="true" />Вернуть</button>}
                        </div>
                      ) : <span className="muted small-note">{target.id === me?.id ? 'это вы' : 'системный администратор'}</span>}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
        <div className="pager">
          <span>Страница {page + 1} из {pages}</span>
          <div className="card-actions">
            <button className="button secondary small" disabled={page === 0} onClick={() => setPage(page - 1)}>Назад</button>
            <button className="button secondary small" disabled={page + 1 >= pages} onClick={() => setPage(page + 1)}>Далее</button>
          </div>
        </div>
      </QueryState>
      <CreateUserDialog
        open={creating}
        onClose={() => setCreating(false)}
        onCreated={async (created, password) => {
          setCreating(false)
          setSecret({ title: 'Пользователь создан', value: password, text: `${created.display_name}, логин ${created.login}. Временный пароль нужно сменить при первом входе.`, extra: { label: 'Логин', value: created.login } })
          await refresh()
        }}
      />
      <EditUserDialog user={editing} onClose={() => setEditing(null)} onSaved={async () => { setEditing(null); showToast('Изменения сохранены'); await refresh() }} />
      <SecretDialog secret={secret} onClose={() => setSecret(null)} />
    </section>
  )
}
