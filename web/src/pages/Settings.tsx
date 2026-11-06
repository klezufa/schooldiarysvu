import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery } from '@tanstack/react-query'
import { api, mutation } from '../api/client'
import { useToast } from '../components/toastContext'
import { Field, PageHeader, QueryState, StatusBadge } from '../components/UI'
import { useAuth } from '../authContext'
import { Link } from 'react-router-dom'
import { draftsEnabled, setSharedComputer } from '../lib/drafts'
import { useTheme, type ThemeChoice } from '../themeContext'

const themeOptions: { value: ThemeChoice; label: string }[] = [
  { value: 'light', label: 'Светлая' }, { value: 'dark', label: 'Тёмная' }, { value: 'system', label: 'Как в системе' },
]

const preferencesSchema = z.object({ internal_enabled: z.boolean(), digest_frequency: z.enum(['never', 'daily', 'weekly']), quiet_from: z.string().optional(), quiet_to: z.string().optional() })
type Preferences = z.infer<typeof preferencesSchema>

export function SettingsPage(): React.JSX.Element {
  const { user } = useAuth()
  const { showToast } = useToast()
  const { choice, setChoice } = useTheme()
  const [largeText, setLargeText] = useState(() => localStorage.getItem('diary_large_text') === 'true')
  const [sharedComputer, setShared] = useState(() => !draftsEnabled())
  const [contrast, setContrast] = useState(() => localStorage.getItem('diary_contrast') === 'true')
  useEffect(() => {
    document.documentElement.classList.toggle('large-text', largeText)
    localStorage.setItem('diary_large_text', String(largeText))
  }, [largeText])
  useEffect(() => {
    document.documentElement.classList.toggle('high-contrast', contrast)
    localStorage.setItem('diary_contrast', String(contrast))
  }, [contrast])
  const { register, handleSubmit, watch, formState: { errors, isSubmitting } } = useForm<Preferences>({ resolver: zodResolver(preferencesSchema), defaultValues: { internal_enabled: true, digest_frequency: 'daily', quiet_from: '22:00', quiet_to: '07:00' } })
  const frequency = watch('digest_frequency')
  const digest = useQuery({ queryKey: ['digest', frequency], queryFn: () => api<Record<string, unknown>>(`/notifications/digest?frequency=${frequency}`), enabled: frequency === 'daily' || frequency === 'weekly' })
  const save = useMutation({ mutationFn: (values: Preferences) => mutation<Record<string, unknown>>('/notification-preferences', 'PUT', values), onSuccess: () => showToast('Настройки уведомлений сохранены'), onError: (error) => showToast(error.message, 'error') })
  const health = useQuery({ queryKey: ['health'], queryFn: async () => { const response = await fetch('/health/ready'); if (!response.ok) throw new Error('Сервер не отвечает'); return response.json() as Promise<Record<string, unknown>> }, refetchInterval: 60_000 })
  return <div><PageHeader eyebrow="Персональные параметры" title="Настройки" description="Уведомления, доступность и безопасность учётной записи." />
    <div className="settings-grid">
      <section className="panel"><h2>Уведомления</h2><p className="muted">Сводки приходят только внутри системы. Тихие часы учитываются фоновым обработчиком.</p>
        <form onSubmit={handleSubmit((values) => save.mutateAsync(values))}>
          <label className="toggle-row"><span><strong>Внутренние уведомления</strong><small>Новые оценки, задания и системные события</small></span><input type="checkbox" {...register('internal_enabled')} /></label>
          <Field label="Периодичность сводки" htmlFor="digest-frequency"><select id="digest-frequency" {...register('digest_frequency')}><option value="never">Не формировать</option><option value="daily">Ежедневно</option><option value="weekly">Еженедельно</option></select></Field>
          <div className="form-row"><Field label="Тихие часы с" htmlFor="quiet-from" error={errors.quiet_from?.message}><input id="quiet-from" type="time" {...register('quiet_from')} /></Field><Field label="Тихие часы до" htmlFor="quiet-to" error={errors.quiet_to?.message}><input id="quiet-to" type="time" {...register('quiet_to')} /></Field></div>
          <button className="button primary" disabled={isSubmitting || save.isPending}>Сохранить настройки</button>
        </form>
      </section>
      <section className="panel"><h2>Оформление</h2><div className="toggle-row"><span><strong>Тема</strong><small>Тёмная бережёт глаза вечером; «как в системе» следует настройке устройства</small></span><div className="segmented" role="group" aria-label="Тема оформления">{themeOptions.map((option) => <button key={option.value} type="button" className={choice === option.value ? 'active' : ''} aria-pressed={choice === option.value} onClick={(event) => { const box = event.currentTarget.getBoundingClientRect(); setChoice(option.value, { x: box.left + box.width / 2, y: box.top + box.height / 2 }) }}>{option.label}</button>)}</div></div><hr /><h2>Доступность</h2><label className="toggle-row"><span><strong>Увеличенный текст</strong><small>Размер сохраняется на этом устройстве</small></span><input type="checkbox" checked={largeText} onChange={(event) => setLargeText(event.target.checked)} /></label><label className="toggle-row"><span><strong>Высокий контраст</strong><small>Более заметные границы и текст</small></span><input type="checkbox" checked={contrast} onChange={(event) => setContrast(event.target.checked)} /></label><hr /><h2>Конфиденциальность</h2><label className="toggle-row"><span><strong>Общий компьютер</strong><small>Не сохранять черновики даже временно; при включении существующие черновики удаляются</small></span><input type="checkbox" checked={sharedComputer} onChange={(event) => { setShared(event.target.checked); setSharedComputer(event.target.checked) }} /></label><p className="muted">Черновики хранятся только в этой вкладке до двух часов и удаляются при выходе из системы.</p><hr /><h2>Аккаунт</h2><p className="muted">{user?.display_name}<br />{user?.login}</p><Link className="button secondary" to="/password">Сменить пароль</Link></section>
      <section className="panel"><div className="section-heading"><h2>Текущая сводка</h2>{digest.data && <StatusBadge tone="info">{frequency === 'daily' ? 'За день' : 'За неделю'}</StatusBadge>}</div><QueryState isLoading={digest.isLoading} error={digest.error} retry={digest.refetch} isEmpty={frequency === 'never'} emptyTitle="Сводки отключены"><pre className="data-preview" tabIndex={0} aria-label="Текущая сводка уведомлений">{JSON.stringify(digest.data, null, 2)}</pre></QueryState></section>
      <section className="panel"><h2>Состояние системы</h2><QueryState isLoading={health.isLoading} error={health.error} retry={health.refetch}><div className="health-ok"><StatusBadge tone="success">Доступно</StatusBadge><pre className="data-preview" tabIndex={0} aria-label="Ответ проверки состояния">{JSON.stringify(health.data, null, 2)}</pre></div></QueryState><p className="muted">Черновики форм хранятся только в памяти страницы и очищаются при выходе. Автономный режим не подменяет сохранение на сервере.</p></section>
    </div>
  </div>
}
