import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Ban, KeyRound } from 'lucide-react'
import { api, mutation } from '../../api/client'
import type { Invitation, InvitationCreated, Role } from '../../api/types'
import { privilegedRoles, regularRoles, roleNames } from '../../api/types'
import { formatDate } from '../../lib/dates'
import { useToast } from '../../components/toastContext'
import { Field, QueryState, StatusBadge } from '../../components/UI'
import { RolePills, SecretDialog } from './shared'
import { useDirectory, useIsSystemAdmin, usePeople } from './helpers'

const statusView: Record<Invitation['status'], { label: string; tone: 'success' | 'neutral' | 'warning' | 'danger' | 'info' }> = {
  open: { label: 'Действует', tone: 'info' }, used: { label: 'Использовано', tone: 'success' },
  expired: { label: 'Истекло', tone: 'neutral' }, revoked: { label: 'Отозвано', tone: 'danger' },
}

export function InvitationsAdmin(): React.JSX.Element {
  const system = useIsSystemAdmin()
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const { classOptions, openClasses } = useDirectory()
  const students = usePeople('student')
  const invitations = useQuery({ queryKey: ['invitations'], queryFn: () => api<Invitation[]>('/invitations?limit=100') })
  const [roles, setRoles] = useState<Role[]>(['parent'])
  const [classId, setClassId] = useState('')
  const [studentId, setStudentId] = useState('')
  const [ttl, setTtl] = useState(168)
  const [note, setNote] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  const [secret, setSecret] = useState<Parameters<typeof SecretDialog>[0]['secret']>(null)
  const available = system ? [...privilegedRoles, ...regularRoles] : regularRoles

  const create = async (event: FormEvent) => {
    event.preventDefault()
    if (!roles.length) { setError('Выберите роль'); return }
    setPending(true)
    setError('')
    try {
      const result = await mutation<InvitationCreated>('/invitations', 'POST', {
        role_codes: roles, ttl_hours: ttl, note: note.trim() || undefined,
        class_id: roles.includes('student') && classId ? classId : undefined,
        student_id: roles.includes('parent') && studentId ? studentId : undefined,
      })
      const link = `${window.location.origin}/register?code=${encodeURIComponent(result.code)}`
      setSecret({ title: 'Приглашение создано', value: result.code, text: 'Передайте код или ссылку человеку лично. Код одноразовый.', extra: { label: 'Ссылка для регистрации', value: link } })
      setNote('')
      await queryClient.invalidateQueries({ queryKey: ['invitations'] })
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Не удалось создать приглашение')
    } finally {
      setPending(false)
    }
  }
  const revoke = async (item: Invitation) => {
    if (!window.confirm('Отозвать приглашение? Код перестанет работать.')) return
    try {
      await mutation(`/invitations/${item.id}`, 'DELETE')
      showToast('Приглашение отозвано')
      await queryClient.invalidateQueries({ queryKey: ['invitations'] })
    } catch (caught) {
      showToast(caught instanceof Error ? caught.message : 'Не удалось отозвать', 'error')
    }
  }

  return (
    <div className="dash-grid">
      <section className="panel col-5">
        <div className="section-heading"><div><h2><KeyRound aria-hidden="true" className="inline-icon" /> Новое приглашение</h2><p className="muted">Человек сам выберет логин и пароль на странице регистрации.</p></div></div>
        <form className="stack-form" onSubmit={create}>
          <fieldset className="checkbox-group">
            <legend>Роль *</legend>
            {available.map((role) => (
              <label key={role}><input type="checkbox" checked={roles.includes(role)} onChange={(event) => setRoles(event.target.checked ? [...roles, role] : roles.filter((item) => item !== role))} />{roleNames[role]}</label>
            ))}
          </fieldset>
          {roles.includes('student') && (
            <Field label="Зачислить в класс" htmlFor="invite-class" hint="В класс следующего учебного года ученик будет зачислен с первого дня того года">
              <select id="invite-class" value={classId} onChange={(event) => setClassId(event.target.value)}>
                <option value="">Без класса</option>
                {classOptions(openClasses).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </Field>
          )}
          {roles.includes('parent') && (
            <Field label="Связать с ребёнком" htmlFor="invite-child" hint="Родитель сразу увидит оценки и расписание ребёнка">
              <select id="invite-child" value={studentId} onChange={(event) => setStudentId(event.target.value)}>
                <option value="">Не связывать</option>
                {students.data?.filter((item) => item.student_id).map((item) => <option key={item.id} value={item.student_id}>{item.display_name}{item.class_name ? `, ${item.class_name}` : ''}</option>)}
              </select>
            </Field>
          )}
          <Field label="Срок действия" htmlFor="invite-ttl">
            <select id="invite-ttl" value={ttl} onChange={(event) => setTtl(Number(event.target.value))}>
              <option value={24}>1 сутки</option><option value={72}>3 дня</option><option value={168}>7 дней</option><option value={720}>30 дней</option>
            </select>
          </Field>
          <Field label="Заметка для себя" htmlFor="invite-note" hint="Например: «мама Иванова И., 5А»"><input id="invite-note" maxLength={200} value={note} onChange={(event) => setNote(event.target.value)} /></Field>
          {error && <div className="form-error" role="alert">{error}</div>}
          <button className="button primary" disabled={pending}>{pending ? 'Создаём…' : 'Создать приглашение'}</button>
        </form>
      </section>
      <section className="panel col-7">
        <div className="section-heading"><div><h2>Приглашения</h2><p className="muted">Последние 100</p></div></div>
        <QueryState isLoading={invitations.isLoading} error={invitations.error} retry={invitations.refetch} isEmpty={!invitations.data?.length} emptyTitle="Приглашений пока нет">
          <div className="responsive-table">
            <table>
              <thead><tr><th>Роль</th><th>Кому</th><th>Состояние</th><th>Срок</th><th><span className="sr-only">Действия</span></th></tr></thead>
              <tbody>
                {invitations.data?.map((item) => (
                  <tr key={item.id}>
                    <td><RolePills roles={item.role_codes} /></td>
                    <td>
                      {item.note || '—'}
                      {(item.class_name || item.student_name) && <><br /><small className="muted">{item.class_name ? `класс ${item.class_name}` : ''}{item.student_name ? `ребёнок: ${item.student_name}` : ''}</small></>}
                      {item.used_by_login && <><br /><small className="muted">зарегистрирован: {item.used_by_login}</small></>}
                    </td>
                    <td><StatusBadge tone={statusView[item.status].tone}>{statusView[item.status].label}</StatusBadge></td>
                    <td>{formatDate(item.expires_at, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })}</td>
                    <td className="actions">{item.status === 'open' && (system || !item.role_codes.some((role) => privilegedRoles.includes(role))) && <button className="icon-button danger-text" title="Отозвать" aria-label="Отозвать приглашение" onClick={() => revoke(item)}><Ban aria-hidden="true" /></button>}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </QueryState>
      </section>
      <SecretDialog secret={secret} onClose={() => setSecret(null)} />
    </div>
  )
}
