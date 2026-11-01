import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { Link, Navigate, useLocation, useNavigate } from 'react-router-dom'
import { Eye, EyeOff, LockKeyhole, UserRound } from 'lucide-react'
import { useAuth } from '../authContext'
import { ApiError } from '../api/client'
import { Field } from '../components/UI'
import { AuthLayout } from './AuthLayout'

const loginSchema = z.object({ login: z.string().trim().min(3, 'Введите логин').max(64), password: z.string().min(1, 'Введите пароль').max(128) })
type LoginInput = z.infer<typeof loginSchema>

export function LoginPage(): React.JSX.Element {
  const { user, login, isLoading } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [showPassword, setShowPassword] = useState(false)
  const notice = (location.state as { registered?: string } | null)?.registered
  const { register, handleSubmit, setError, formState: { errors, isSubmitting } } = useForm<LoginInput>({
    resolver: zodResolver(loginSchema),
    defaultValues: { login: notice ?? '', password: '' },
  })
  if (!isLoading && user) return <Navigate to={user.must_change_password ? '/password' : '/today'} replace />
  const submit = handleSubmit(async (values) => {
    try {
      const loggedIn = await login(values)
      const from = (location.state as { from?: string } | null)?.from
      navigate(loggedIn.must_change_password ? '/password' : from ?? '/today', { replace: true })
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Не удалось войти'
      // Сервер называет срок блокировки («Повторите через N мин.»); подсказка — только если срок не пришёл.
      const waitHint = error instanceof ApiError && error.status === 429 && !error.retryAfter ? ' Подождите перед следующей попыткой.' : ''
      setError('root', { message: `${message}${waitHint}` })
    }
  })
  return (
    <AuthLayout>
      <div className="auth-card" aria-labelledby="login-title">
        <div>
          <h2 id="login-title">Вход в систему</h2>
          <p>Пожалуйста, авторизуйтесь для продолжения</p>
        </div>
        {notice && <div className="auth-note" role="status">Учётная запись создана. Войдите с выбранным паролем.</div>}
        <form onSubmit={submit} noValidate>
          <Field label="Логин" htmlFor="login" error={errors.login?.message}>
            <div className="input-with-icon"><UserRound aria-hidden="true" /><input id="login" autoComplete="username" autoFocus={!notice} {...register('login')} /></div>
          </Field>
          <Field label="Пароль" htmlFor="password" error={errors.password?.message}>
            <div className="input-with-icon password-input">
              <LockKeyhole aria-hidden="true" />
              <input id="password" type={showPassword ? 'text' : 'password'} autoComplete="current-password" autoFocus={Boolean(notice)} {...register('password')} />
              <button type="button" className="icon-button reveal" onClick={() => setShowPassword(!showPassword)} aria-label={showPassword ? 'Скрыть пароль' : 'Показать пароль'} aria-pressed={showPassword}>
                {showPassword ? <EyeOff aria-hidden="true" /> : <Eye aria-hidden="true" />}
              </button>
            </div>
          </Field>
          {errors.root && <div className="form-error" role="alert">{errors.root.message}</div>}
          <button className="button primary login-submit" disabled={isSubmitting}>{isSubmitting ? 'Проверяем…' : 'Войти'}</button>
        </form>
        <div className="auth-links">
          <Link to="/register">Есть код приглашения?</Link>
          <span className="muted">Забыли пароль — обратитесь к администратору</span>
        </div>
      </div>
    </AuthLayout>
  )
}
