import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { KeyRound, LockKeyhole, UserRound } from 'lucide-react'
import { ApiError, api, mutation, queryString } from '../api/client'
import type { InvitationLookup, User } from '../api/types'
import { formatDate } from '../lib/dates'
import { Field, StatusBadge } from '../components/UI'
import { AuthLayout } from './AuthLayout'

const registerSchema = z.object({
  display_name: z.string().trim().min(3, 'Укажите фамилию и имя').max(160),
  login: z.string().trim().toLowerCase().regex(/^[a-z0-9][a-z0-9._-]{2,63}$/, 'Латиница, цифры, точка, дефис или подчёркивание; от 3 символов'),
  password: z.string().min(12, 'Не менее 12 символов').max(128, 'Не более 128 символов'),
  confirmation: z.string(),
}).refine((value) => value.password === value.confirmation, { path: ['confirmation'], message: 'Пароли не совпадают' })
type RegisterInput = z.infer<typeof registerSchema>

export function RegisterPage(): React.JSX.Element {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const [code, setCode] = useState(params.get('code') ?? '')
  const [invitation, setInvitation] = useState<InvitationLookup | null>(null)
  const [lookupError, setLookupError] = useState('')
  const [checking, setChecking] = useState(false)
  const { register, handleSubmit, setError, formState: { errors, isSubmitting } } = useForm<RegisterInput>({ resolver: zodResolver(registerSchema) })

  const check = async () => {
    setChecking(true)
    setLookupError('')
    try {
      setInvitation(await api<InvitationLookup>(`/invitations/lookup${queryString({ code: code.trim() })}`))
    } catch (error) {
      setInvitation(null)
      setLookupError(error instanceof ApiError && error.status === 404
        ? 'Код не найден, уже использован или истёк. Попросите новый код у администратора школы.'
        : error instanceof Error ? error.message : 'Не удалось проверить код')
    } finally {
      setChecking(false)
    }
  }

  const submit = handleSubmit(async (values) => {
    try {
      const user = await mutation<User>('/auth/register', 'POST', { code: code.trim(), login: values.login, display_name: values.display_name, password: values.password })
      navigate('/login', { replace: true, state: { registered: user.login } })
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) setError('login', { message: error.message })
      else setError('root', { message: error instanceof Error ? error.message : 'Не удалось создать учётную запись' })
    }
  })

  return (
    <AuthLayout>
      <div className="auth-card">
        <div>
          <h2>Регистрация</h2>
          <p>По одноразовому коду приглашения от администратора школы</p>
        </div>
        {!invitation ? (
          <form onSubmit={(event) => { event.preventDefault(); void check() }} noValidate>
            <Field label="Код приглашения" htmlFor="invite-code" error={lookupError || undefined} hint="Формат XXXXX-XXXXX-XXXXX-XXXXX, регистр не важен">
              <div className="input-with-icon"><KeyRound aria-hidden="true" /><input id="invite-code" value={code} autoFocus autoComplete="one-time-code" onChange={(event) => setCode(event.target.value)} /></div>
            </Field>
            <button className="button primary login-submit" disabled={checking || code.trim().length < 8}>{checking ? 'Проверяем…' : 'Проверить код'}</button>
          </form>
        ) : (
          <>
            <div className="auth-note">
              <strong>Приглашение действительно</strong>
              <div className="role-pills" style={{ margin: '.4rem 0' }}>{invitation.role_names.map((name) => <StatusBadge key={name} tone="info">{name}</StatusBadge>)}</div>
              {invitation.class_name && <div>Класс: <strong>{invitation.class_name}</strong></div>}
              {invitation.student_name && <div>Ребёнок: <strong>{invitation.student_name}</strong></div>}
              {invitation.note && <div>{invitation.note}</div>}
              <small className="muted">Действует до {formatDate(invitation.expires_at, { day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit' })}</small>
            </div>
            <form onSubmit={submit} noValidate>
              <Field label="Фамилия, имя и отчество" htmlFor="reg-name" error={errors.display_name?.message}>
                <div className="input-with-icon"><UserRound aria-hidden="true" /><input id="reg-name" autoComplete="name" autoFocus {...register('display_name')} /></div>
              </Field>
              <Field label="Логин" htmlFor="reg-login" error={errors.login?.message} hint="Будет использоваться для входа">
                <input id="reg-login" autoComplete="username" {...register('login')} />
              </Field>
              <Field label="Пароль" htmlFor="reg-password" error={errors.password?.message} hint="Минимум 12 символов; удобно использовать фразу из нескольких слов">
                <div className="input-with-icon"><LockKeyhole aria-hidden="true" /><input id="reg-password" type="password" autoComplete="new-password" {...register('password')} /></div>
              </Field>
              <Field label="Повторите пароль" htmlFor="reg-confirmation" error={errors.confirmation?.message}>
                <input id="reg-confirmation" type="password" autoComplete="new-password" {...register('confirmation')} />
              </Field>
              {errors.root && <div className="form-error" role="alert">{errors.root.message}</div>}
              <button className="button primary login-submit" disabled={isSubmitting}>{isSubmitting ? 'Создаём…' : 'Создать учётную запись'}</button>
              <button type="button" className="button text-button" onClick={() => setInvitation(null)}>Ввести другой код</button>
            </form>
          </>
        )}
        <div className="auth-links"><Link to="/login">Уже есть учётная запись? Войти</Link></div>
      </div>
    </AuthLayout>
  )
}
