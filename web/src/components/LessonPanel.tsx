import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { BookOpen, Save } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { api, mutation, queryString } from '../api/client'
import type { AuditLog, Homework, Lesson } from '../api/types'
import { useAuth } from '../authContext'
import { useStudent } from '../studentContext'
import { formatDate, formatTime } from '../lib/dates'
import { scopedKey } from '../lib/student'
import { Field, Modal, QueryState, StateView, StatusBadge } from './UI'
import { useToast } from './toastContext'
import { LessonCompletionCheck } from './LessonCompletionCheck'

const homeworkSchema = z.object({ text: z.string().trim().min(1, 'Введите текст задания').max(10000), due_date: z.string().min(1, 'Укажите срок'), status: z.enum(['draft', 'published']) })
type HomeworkInput = z.infer<typeof homeworkSchema>

function LessonHomework({ lesson }: { lesson: Lesson }): React.JSX.Element {
  const { hasRole } = useAuth()
  const { studentId } = useStudent()
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const editable = hasRole('teacher', 'class_teacher')
  const homework = useQuery({ queryKey: scopedKey('homework', studentId), queryFn: () => api<Homework[]>(`/homework${queryString({ student_id: studentId })}`) })
  const linked = homework.data?.filter((item) => item.lesson_id === lesson.id) ?? []
  const { register, handleSubmit, reset, formState: { errors } } = useForm<HomeworkInput>({ resolver: zodResolver(homeworkSchema), defaultValues: { text: '', due_date: lesson.lesson_date, status: 'draft' } })
  const save = useMutation({
    mutationFn: (values: HomeworkInput) => mutation<Homework>('/homework', 'POST', { ...values, class_id: lesson.class_id, subject_id: lesson.subject_id, lesson_id: lesson.id }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['homework'] }); reset(); showToast('Задание сохранено') },
    onError: (error) => showToast(error.message, 'error'),
  })
  const publish = useMutation({
    mutationFn: (item: Homework) => mutation<Homework>(`/homework/${item.id}/publish?version=${item.version}`, 'POST'),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['homework'] }); showToast('Задание опубликовано') },
    onError: (error) => showToast(error.message, 'error'),
  })
  return <QueryState isLoading={homework.isLoading} error={homework.error} retry={homework.refetch}>
    {linked.length > 0 && <div className="linked-homework">{linked.map((item) => <article className="homework-card" key={item.id}><div className="section-heading"><StatusBadge tone={item.status === 'published' ? 'success' : 'warning'}>{item.status === 'published' ? 'Опубликовано' : 'Черновик'}</StatusBadge><small>Срок: {formatDate(item.due_date)}</small></div><p>{item.text}</p>{item.files?.map((file) => <a href={`/api/v1/files/${file.id}${queryString({ student_id: studentId })}`} key={file.id}>{file.original_name}</a>)}{editable && item.status === 'draft' && <button className="button secondary" onClick={() => publish.mutate(item)} disabled={publish.isPending}>Опубликовать</button>}</article>)}</div>}
    {linked.length === 0 && !editable && <p className="muted">К этому уроку задание не добавлено.</p>}
    {editable && <form className="stack-form" onSubmit={handleSubmit((values) => save.mutate(values))}><h3>{linked.length ? 'Добавить ещё одно задание' : 'Задание к уроку'}</h3><Field label="Текст задания" htmlFor="lesson-homework-text" error={errors.text?.message}><textarea id="lesson-homework-text" rows={5} {...register('text')} /></Field><div className="form-row"><Field label="Срок выполнения" htmlFor="lesson-homework-date" error={errors.due_date?.message}><input id="lesson-homework-date" type="date" {...register('due_date')} /></Field><Field label="Состояние" htmlFor="lesson-homework-status"><select id="lesson-homework-status" {...register('status')}><option value="draft">Черновик</option><option value="published">Опубликовать сразу</option></select></Field></div><button className="button primary" disabled={save.isPending}><Save />{save.isPending ? 'Сохраняем…' : 'Сохранить задание'}</button></form>}
  </QueryState>
}

function LessonDetails({ lesson, onUpdated }: { lesson: Lesson; onUpdated: (lesson: Lesson) => void }): React.JSX.Element {
  const { hasRole } = useAuth()
  const { showToast } = useToast()
  const queryClient = useQueryClient()
  const [topic, setTopic] = useState(lesson.topic)
  const [status, setStatus] = useState<Lesson['status']>(lesson.status)
  const [reason, setReason] = useState(lesson.cancellation_reason ?? '')
  useEffect(() => { setTopic(lesson.topic); setStatus(lesson.status); setReason(lesson.cancellation_reason ?? '') }, [lesson])
  const save = useMutation({
    mutationFn: () => mutation<Lesson>(`/lessons/${lesson.id}?version=${lesson.version}`, 'PATCH', { topic, status, cancellation_reason: status === 'cancelled' ? reason : undefined }),
    onSuccess: (updated) => { onUpdated(updated); queryClient.invalidateQueries({ queryKey: ['schedule'] }); queryClient.setQueryData(['lesson', lesson.id], updated); showToast('Урок сохранён') },
    onError: (error) => showToast(error.message, 'error'),
  })
  return <div className="lesson-details"><dl className="detail-list"><div><dt>Дата</dt><dd>{formatDate(lesson.lesson_date, { weekday: 'long', day: 'numeric', month: 'long' })}</dd></div><div><dt>Время</dt><dd>{formatTime(lesson.starts_at)}–{formatTime(lesson.ends_at)}</dd></div><div><dt>Класс и предмет</dt><dd>{lesson.class_name} · {lesson.subject_name}</dd></div><div><dt>Кабинет</dt><dd>{lesson.room_name || 'Не указан'}</dd></div></dl>
    {hasRole('teacher', 'class_teacher') ? <div className="stack-form"><Field label="Тема урока" htmlFor="lesson-topic"><textarea id="lesson-topic" rows={4} value={topic} onChange={(event) => setTopic(event.target.value)} /></Field><Field label="Статус" htmlFor="lesson-status"><select id="lesson-status" value={status} onChange={(event) => setStatus(event.target.value as Lesson['status'])}><option value="planned">По расписанию</option><option value="completed">Завершён</option><option value="cancelled">Отменён</option></select></Field>{status === 'completed' && lesson.status !== 'completed' && <LessonCompletionCheck lesson={lesson} />}{status === 'cancelled' && <Field label="Причина отмены" htmlFor="lesson-reason"><textarea id="lesson-reason" value={reason} onChange={(event) => setReason(event.target.value)} /></Field>}{save.error && <StateView error={save.error} />}<button className="button primary" onClick={() => save.mutate()} disabled={save.isPending}><Save />{save.isPending ? 'Сохраняем…' : 'Сохранить урок'}</button><p className="muted">Версия записи: {lesson.version}. При конфликте изменения не перезаписываются автоматически.</p></div> : <div><h3>Тема</h3><p>{lesson.topic || 'Тема не указана'}</p>{lesson.status === 'cancelled' && <StateView error={new Error(`Урок отменён. ${lesson.cancellation_reason ?? ''}`)} />}</div>}
  </div>
}

export function LessonPanel({ lesson, onClose }: { lesson: Lesson | null; onClose: () => void }): React.JSX.Element {
  const { hasRole } = useAuth()
  const navigate = useNavigate()
  const [tab, setTab] = useState<'lesson' | 'journal' | 'homework' | 'history'>('lesson')
  const [current, setCurrent] = useState<Lesson | null>(lesson)
  useEffect(() => { setCurrent(lesson); setTab('lesson') }, [lesson])
  const history = useQuery({ queryKey: ['audit', 'lesson', lesson?.id], queryFn: () => api<AuditLog[]>(`/audit?entity_type=lesson&entity_id=${lesson?.id}&limit=50`), enabled: Boolean(lesson && tab === 'history' && hasRole('administrator', 'system_admin')) })
  return <Modal open={Boolean(current)} onOpenChange={(open) => { if (!open) onClose() }} title={current ? `${current.subject_name} · ${current.class_name}` : 'Урок'} description={current ? `${formatDate(current.lesson_date)} · ${formatTime(current.starts_at)}` : undefined} wide>
    {current && <><div className="tabs" role="tablist" aria-label="Сведения об уроке"><button role="tab" aria-selected={tab === 'lesson'} className={tab === 'lesson' ? 'active' : ''} onClick={() => setTab('lesson')}>Урок</button>{hasRole('teacher', 'class_teacher') && <button role="tab" aria-selected={tab === 'journal'} className={tab === 'journal' ? 'active' : ''} onClick={() => setTab('journal')}>Журнал</button>}<button role="tab" aria-selected={tab === 'homework'} className={tab === 'homework' ? 'active' : ''} onClick={() => setTab('homework')}>Домашнее задание</button><button role="tab" aria-selected={tab === 'history'} className={tab === 'history' ? 'active' : ''} onClick={() => setTab('history')}>История</button></div>
      <div role="tabpanel" className="tab-panel">
        {tab === 'lesson' && <LessonDetails lesson={current} onUpdated={setCurrent} />}
        {tab === 'journal' && <div className="state-view"><BookOpen aria-hidden="true" /><h3>Журнал с выбранным уроком</h3><p>Класс, предмет, период и урок будут переданы в рабочую таблицу.</p><button className="button primary" onClick={() => { onClose(); navigate('/journal', { state: { lesson: current } }) }}>Открыть журнал</button></div>}
        {tab === 'homework' && <LessonHomework lesson={current} />}
        {tab === 'history' && (hasRole('administrator') ? <QueryState isLoading={history.isLoading} error={history.error} retry={history.refetch} isEmpty={!history.data?.length} emptyTitle="Изменений ещё не было"><div className="audit-list">{history.data?.map((item) => <article key={item.id}><strong>{item.action}</strong><time>{formatDate(item.created_at, { day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit' })}</time><pre>{JSON.stringify({ before: item.old_values, after: item.new_values }, null, 2)}</pre></article>)}</div></QueryState> : <div className="state-view"><h3>История хранится на сервере</h3><p>Полный аудит урока доступен администратору. История изменения конкретной оценки открывается в редакторе оценки журнала, в блоке «История изменений».</p></div>)}
      </div></>}
  </Modal>
}
