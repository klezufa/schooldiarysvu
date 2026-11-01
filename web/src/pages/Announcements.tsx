import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { Megaphone, Plus, Trash2 } from 'lucide-react'
import { api, mutation, queryString } from '../api/client'
import type { Announcement } from '../api/types'
import { useAuth } from '../authContext'
import { useStudent } from '../studentContext'
import { useAcademic } from '../hooks/useAcademic'
import { formatDate } from '../lib/dates'
import { scopedKey } from '../lib/student'
import { Field, Modal, PageHeader, QueryState } from '../components/UI'
import { useToast } from '../components/toastContext'

const schema = z.object({ title: z.string().trim().min(1, 'Введите заголовок').max(300), body: z.string().trim().min(1, 'Введите текст').max(20000), audience_type: z.enum(['school', 'class', 'subject']), class_id: z.string().optional(), subject_id: z.string().optional() }).superRefine((value, context) => {
  if (value.audience_type === 'class' && !value.class_id) context.addIssue({ code: 'custom', path: ['class_id'], message: 'Выберите класс' })
  if (value.audience_type === 'subject' && !value.subject_id) context.addIssue({ code: 'custom', path: ['subject_id'], message: 'Выберите предмет' })
})
type AnnouncementInput = z.infer<typeof schema>

export function AnnouncementsPage(): React.JSX.Element {
  const { hasRole } = useAuth()
  const { studentId } = useStudent()
  const { classes, subjects } = useAcademic()
  const [open, setOpen] = useState(false)
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const editable = hasRole('teacher', 'class_teacher', 'administrator', 'system_admin')
  const announcements = useQuery({ queryKey: scopedKey('announcements', studentId), queryFn: () => api<Announcement[]>(`/announcements${queryString({ student_id: studentId })}`) })
  const { register, handleSubmit, watch, reset, formState: { errors } } = useForm<AnnouncementInput>({ resolver: zodResolver(schema), defaultValues: { audience_type: 'school', title: '', body: '' } })
  const audience = watch('audience_type')
  const create = useMutation({ mutationFn: (values: AnnouncementInput) => mutation<Announcement>('/announcements', 'POST', { ...values, class_id: values.audience_type === 'class' ? values.class_id : undefined, subject_id: values.audience_type === 'subject' ? values.subject_id : undefined }), onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['announcements'] }); setOpen(false); reset(); showToast('Объявление опубликовано') }, onError: (error) => showToast(error.message, 'error') })
  const archive = useMutation({ mutationFn: (id: string) => mutation<void>(`/announcements/${id}`, 'DELETE'), onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['announcements'] }); showToast('Объявление архивировано') }, onError: (error) => showToast(error.message, 'error') })
  return <div><PageHeader eyebrow="Школьная информация" title="Объявления" description="Официальные сообщения школы, класса и преподавателей." actions={editable ? <button className="button primary" onClick={() => setOpen(true)}><Plus />Создать объявление</button> : undefined} />
    <QueryState isLoading={announcements.isLoading} error={announcements.error} retry={announcements.refetch} isEmpty={!announcements.data?.length} emptyTitle="Объявлений пока нет" emptyText="Новые сообщения появятся здесь и в уведомлениях."><div className="announcement-list">{announcements.data?.map((item) => <article className="panel announcement" key={item.id}><div className="announcement-icon"><Megaphone aria-hidden="true" /></div><div><time>{formatDate(item.published_at, { day: 'numeric', month: 'long', year: 'numeric', hour: '2-digit', minute: '2-digit' })}</time><h2>{item.title}</h2><p>{item.body}</p></div>{editable && <button className="icon-button danger-text" aria-label={`Архивировать объявление ${item.title}`} onClick={() => { if (window.confirm(`Архивировать объявление «${item.title}»? Оно исчезнет из общей ленты.`)) archive.mutate(item.id) }}><Trash2 /></button>}</article>)}</div></QueryState>
    <Modal open={open} onOpenChange={setOpen} title="Новое объявление"><form className="stack-form" onSubmit={handleSubmit((values) => create.mutate(values))}><Field label="Заголовок" htmlFor="announcement-title" error={errors.title?.message}><input id="announcement-title" {...register('title')} /></Field><Field label="Текст" htmlFor="announcement-body" error={errors.body?.message}><textarea id="announcement-body" rows={6} {...register('body')} /></Field><Field label="Аудитория" htmlFor="announcement-audience"><select id="announcement-audience" {...register('audience_type')}><option value="school">Вся школа</option><option value="class">Класс</option><option value="subject">Предмет</option></select></Field>{audience === 'class' && <Field label="Класс" error={errors.class_id?.message}><select {...register('class_id')}><option value="">Выберите класс</option>{classes.data?.map((item) => <option value={item.id} key={item.id}>{item.name}</option>)}</select></Field>}{audience === 'subject' && <Field label="Предмет" error={errors.subject_id?.message}><select {...register('subject_id')}><option value="">Выберите предмет</option>{subjects.data?.map((item) => <option value={item.id} key={item.id}>{item.name}</option>)}</select></Field>}<button className="button primary" disabled={create.isPending}>Опубликовать</button></form></Modal>
  </div>
}
