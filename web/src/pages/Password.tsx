import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { useNavigate } from 'react-router-dom'
import { ApiError } from '../api/client'
import { useAuth } from '../authContext'
import { Field } from '../components/UI'
import { useToast } from '../components/toastContext'

const passwordSchema = z.object({
  old_password: z.string().min(1, 'Введите текущий пароль'),
  new_password: z.string().min(12, 'Не менее 12 символов').max(128, 'Не более 128 символов'),
  confirmation: z.string(),
}).refine((value) => value.new_password === value.confirmation, { path: ['confirmation'], message: 'Пароли не совпадают' })
type PasswordInput = z.infer<typeof passwordSchema>

export function PasswordPage(): React.JSX.Element {
  const { user, changePassword, logout } = useAuth()
  const navigate = useNavigate()
  const { showToast } = useToast()
  const { register, handleSubmit, setError, formState: { errors, isSubmitting } } = useForm<PasswordInput>({ resolver: zodResolver(passwordSchema) })
  const submit = handleSubmit(async ({ old_password, new_password }) => {
    try {
      await changePassword({ old_password, new_password })
      showToast('Пароль изменён. Другие сессии отозваны.')
      navigate('/today', { replace: true })
    } catch (error) {
      if (error instanceof ApiError) Object.entries(error.fieldErrors).forEach(([field, message]) => setError(field as 'old_password' | 'new_password', { message }))
      setError('root', { message: error instanceof Error ? error.message : 'Не удалось изменить пароль' })
    }
  })
  return <main className="password-page"><section className="password-card"><p className="eyebrow">Безопасность аккаунта</p><h1>{user?.must_change_password ? 'Создайте постоянный пароль' : 'Смена пароля'}</h1><p>{user?.must_change_password ? 'Временный пароль нужно заменить перед началом работы. Это обязательное требование системы.' : 'После смены пароля другие активные сессии будут завершены.'}</p>
    <form onSubmit={submit} noValidate>
      <Field label="Текущий пароль" htmlFor="old-password" error={errors.old_password?.message}><input id="old-password" type="password" autoComplete="current-password" autoFocus {...register('old_password')} /></Field>
      <Field label="Новый пароль" htmlFor="new-password" error={errors.new_password?.message} hint="Минимум 12 символов; используйте уникальную фразу"><input id="new-password" type="password" autoComplete="new-password" {...register('new_password')} /></Field>
      <Field label="Повторите новый пароль" htmlFor="confirmation" error={errors.confirmation?.message}><input id="confirmation" type="password" autoComplete="new-password" {...register('confirmation')} /></Field>
      {errors.root && <div className="form-error" role="alert">{errors.root.message}</div>}
      <button className="button primary" disabled={isSubmitting}>{isSubmitting ? 'Сохраняем…' : 'Сохранить пароль'}</button>
    </form><button className="button text-button" onClick={async () => { await logout(); navigate('/login') }}>Выйти из аккаунта</button>
  </section></main>
}
