import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { Copy, Download, FileUp, Paperclip, Pencil, Plus, Send, Trash2 } from 'lucide-react'
import { api, download, mutation, queryString, saveBlob, ApiError } from '../api/client'
import type { Homework, Student } from '../api/types'
import { useAuth } from '../authContext'
import { useStudent } from '../studentContext'
import { useAcademic } from '../hooks/useAcademic'
import { addDays, dateISO, formatDate } from '../lib/dates'
import { scopedKey } from '../lib/student'
import { loadDraft, removeDraft, saveDraft } from '../lib/drafts'
import { Field, Modal, PageHeader, QueryState, StatusBadge } from '../components/UI'
import { useToast } from '../components/toastContext'

const homeworkSchema = z.object({ class_id: z.string().uuid('Выберите класс'), subject_id: z.string().uuid('Выберите предмет'), lesson_id: z.string().optional(), text: z.string().trim().min(1, 'Введите задание').max(10000), due_date: z.string().min(1, 'Укажите срок'), status: z.enum(['draft', 'published']), student_ids: z.array(z.string()).optional() })
type HomeworkInput = z.infer<typeof homeworkSchema>

type EditorState = { homework?: Homework; copy?: boolean }
const NEW_HOMEWORK_DRAFT = 'homework_new'

function HomeworkEditor({ state, onClose }: { state: EditorState | null; onClose: () => void }): React.JSX.Element {
  const { classes, subjects } = useAcademic()
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const item = state?.homework
  const copy = state?.copy ?? false
  const { register, handleSubmit, reset, watch, setError, formState: { errors } } = useForm<HomeworkInput>({ resolver: zodResolver(homeworkSchema), defaultValues: { class_id: '', subject_id: '', lesson_id: '', text: '', due_date: dateISO(), status: 'draft', student_ids: [] } })
  const classId = watch('class_id')
  const text = watch('text')
  // Несохранённый текст нового задания переживает случайное закрытие окна (в рамках вкладки, с TTL).
  useEffect(() => { if (state && !item) saveDraft(NEW_HOMEWORK_DRAFT, text) }, [state, item, text])
  const students = useQuery({ queryKey: ['class-students', classId, dateISO()], queryFn: () => api<Student[]>(`/classes/${classId}/students?date=${dateISO()}`), enabled: Boolean(state && classId && !item) })
  useEffect(() => {
    if (!state) return
    reset({
      class_id: item?.class_id ?? classes.data?.[0]?.id ?? '', subject_id: item?.subject_id ?? subjects.data?.[0]?.id ?? '',
      lesson_id: item?.lesson_id ?? '', text: item?.text ?? (item ? '' : loadDraft(NEW_HOMEWORK_DRAFT)),
      due_date: copy ? addDays(item?.due_date ?? dateISO(), 7) : item?.due_date ?? dateISO(), status: copy ? 'draft' : item?.status ?? 'draft', student_ids: [],
    })
  }, [state, item, copy, classes.data, subjects.data, reset])
  const save = useMutation({
    mutationFn: (values: HomeworkInput) => {
      const payload = { ...values, lesson_id: values.lesson_id || undefined, student_ids: values.student_ids?.length ? values.student_ids : undefined }
      if (item && copy) return mutation<Homework>(`/homework/${item.id}/copy`, 'POST', { due_date: values.due_date, status: values.status, student_ids: values.student_ids?.length ? values.student_ids : undefined })
      if (item) return mutation<Homework>(`/homework/${item.id}?version=${item.version}`, 'PATCH', { text: values.text, due_date: values.due_date })
      return mutation<Homework>('/homework', 'POST', payload)
    },
    onSuccess: () => { if (!item) removeDraft(NEW_HOMEWORK_DRAFT); queryClient.invalidateQueries({ queryKey: ['homework'] }); showToast(copy ? 'Задание и независимые копии файлов созданы' : item ? 'Задание обновлено' : 'Задание создано'); onClose() },
    onError: (error) => {
      if (error instanceof ApiError) Object.entries(error.fieldErrors).forEach(([field, message]) => setError(field as keyof HomeworkInput, { message }))
      setError('root', { message: error.message })
    },
  })
  return <Modal open={Boolean(state)} onOpenChange={(open) => { if (!open) onClose() }} title={copy ? 'Копировать задание' : item ? 'Редактировать задание' : 'Новое домашнее задание'} description={copy ? 'Вложения будут скопированы в независимые приватные файлы.' : 'Можно сохранить черновик и опубликовать позже.'} wide>
    <form className="stack-form" onSubmit={handleSubmit((values) => save.mutate(values))}>
      {!item && <div className="form-row"><Field label="Класс" htmlFor="homework-class" error={errors.class_id?.message}><select id="homework-class" {...register('class_id')}>{classes.data?.map((entity) => <option key={entity.id} value={entity.id}>{entity.name}</option>)}</select></Field><Field label="Предмет" htmlFor="homework-subject" error={errors.subject_id?.message}><select id="homework-subject" {...register('subject_id')}>{subjects.data?.map((entity) => <option key={entity.id} value={entity.id}>{entity.name}</option>)}</select></Field></div>}
      {item && <p className="muted">{item.class_name} · {item.subject_name} · версия {item.version}</p>}
      <Field label="Текст задания" htmlFor="homework-text" error={errors.text?.message}><textarea id="homework-text" rows={7} {...register('text')} /></Field>
      <div className="form-row"><Field label="Срок выполнения" htmlFor="homework-due" error={errors.due_date?.message}><input id="homework-due" type="date" {...register('due_date')} /></Field>{(!item || copy) && <Field label="Публикация" htmlFor="homework-status"><select id="homework-status" {...register('status')}><option value="draft">Черновик</option><option value="published">Опубликовано</option></select></Field>}</div>
      {!item && <Field label="Получатели" hint="Если никого не выбрано, задание получат все ученики класса"><div className="recipient-grid">{students.data?.map((student) => <label key={student.id}><input type="checkbox" value={student.id} {...register('student_ids')} />{student.display_name}</label>)}</div></Field>}
      {errors.root && <div className="form-error" role="alert">{errors.root.message}</div>}
      <button className="button primary" disabled={save.isPending}>{save.isPending ? 'Сохраняем…' : copy ? 'Создать копию' : 'Сохранить задание'}</button>
    </form>
  </Modal>
}

function FileControls({ item }: { item: Homework }): React.JSX.Element {
  const { hasRole } = useAuth()
  const { studentId } = useStudent()
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const upload = useMutation({
    mutationFn: (file: File) => { const body = new FormData(); body.set('homework_id', item.id); body.set('file', file); return api('/files', { method: 'POST', body }) },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['homework'] }); showToast('Файл загружен в приватное хранилище') },
    onError: (error) => showToast(error.message, 'error'),
  })
  const remove = useMutation({ mutationFn: (id: string) => mutation<void>(`/files/${id}`, 'DELETE'), onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['homework'] }); showToast('Файл архивирован') }, onError: (error) => showToast(error.message, 'error') })
  const saveFile = async (id: string) => {
    try { const result = await download(`/files/${id}${queryString({ student_id: studentId })}`); saveBlob(result.blob, result.filename) } catch (error) { showToast(error instanceof Error ? error.message : 'Не удалось скачать файл', 'error') }
  }
  return <div className="file-controls">{item.files?.map((file) => <div className="file-row" key={file.id}><Paperclip aria-hidden="true" /><button className="file-download" onClick={() => saveFile(file.id)}>{file.original_name}<small>{Math.ceil(file.size_bytes / 1024)} КБ · {file.mime_type}</small></button><button className="icon-button" aria-label={`Скачать ${file.original_name}`} onClick={() => saveFile(file.id)}><Download /></button>{hasRole('teacher', 'class_teacher') && <button className="icon-button danger-text" aria-label={`Архивировать ${file.original_name}`} onClick={() => { if (window.confirm(`Архивировать файл «${file.original_name}»? Скачивание станет недоступно.`)) remove.mutate(file.id) }}><Trash2 /></button>}</div>)}
    {hasRole('teacher', 'class_teacher') && <label className={`button secondary file-upload ${upload.isPending ? 'disabled' : ''}`}><FileUp />{upload.isPending ? 'Загружаем…' : 'Прикрепить файл'}<input type="file" disabled={upload.isPending} onChange={(event) => { const file = event.target.files?.[0]; if (file) upload.mutate(file); event.target.value = '' }} /></label>}
  </div>
}

type Deadline = 'today' | 'tomorrow' | 'week' | 'later' | 'overdue' | 'all'

export function HomeworkPage(): React.JSX.Element {
  const { hasRole } = useAuth()
  const { studentId } = useStudent()
  const { subjects } = useAcademic()
  const { showToast } = useToast()
  const queryClient = useQueryClient()
  const [deadline, setDeadline] = useState<Deadline>('all')
  const [subject, setSubject] = useState('')
  const [hideCompleted, setHideCompleted] = useState(false)
  const [editor, setEditor] = useState<EditorState | null>(null)
  const editable = hasRole('teacher', 'class_teacher')
  const student = hasRole('student')
  const homework = useQuery({ queryKey: scopedKey('homework', studentId), queryFn: () => api<Homework[]>(`/homework${queryString({ student_id: studentId })}`), enabled: editable || Boolean(studentId) })
  const filtered = useMemo(() => {
    const today = dateISO()
    const tomorrow = addDays(today, 1)
    const week = addDays(today, 7)
    return (homework.data ?? []).filter((item) => {
      if (subject && item.subject_id !== subject) return false
      if (hideCompleted && item.completed) return false
      if (deadline === 'today') return item.due_date === today
      if (deadline === 'tomorrow') return item.due_date === tomorrow
      if (deadline === 'week') return item.due_date >= today && item.due_date <= week
      if (deadline === 'later') return item.due_date > week
      if (deadline === 'overdue') return item.due_date < today && !item.completed
      return true
    }).sort((a, b) => a.due_date.localeCompare(b.due_date))
  }, [homework.data, subject, hideCompleted, deadline])
  const publish = useMutation({ mutationFn: (item: Homework) => mutation<Homework>(`/homework/${item.id}/publish?version=${item.version}`, 'POST'), onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['homework'] }); showToast('Задание опубликовано') }, onError: (error) => showToast(error.message, 'error') })
  const completion = useMutation({
    mutationFn: ({ item, completed }: { item: Homework; completed: boolean }) => mutation<{ completed: boolean }>(`/homework/${item.id}/completion`, 'PUT', { student_id: studentId, completed }),
    onMutate: async ({ item, completed }) => {
      const key = scopedKey('homework', studentId)
      await queryClient.cancelQueries({ queryKey: key })
      const before = queryClient.getQueryData<Homework[]>(key)
      queryClient.setQueryData<Homework[]>(key, (current) => current?.map((value) => value.id === item.id ? { ...value, completed } : value))
      return { before, key }
    },
    onError: (error, _input, context) => { if (context) queryClient.setQueryData(context.key, context.before); showToast(error.message, 'error') },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['homework'] }),
  })
  return <div><PageHeader eyebrow="Учебные материалы" title={editable ? 'Домашние задания' : 'Мои задания'} description={student ? 'Личная отметка помогает отслеживать выполнение и не является официальной сдачей работы.' : 'Черновики, публикации, получатели и приватные вложения.'} actions={editable ? <button className="button primary" onClick={() => setEditor({})}><Plus />Создать задание</button> : undefined} />
    <div className="toolbar homework-toolbar"><div className="segmented deadline-tabs" aria-label="Срок задания">{([['today', 'Сегодня'], ['tomorrow', 'Завтра'], ['week', 'На неделю'], ['later', 'Позже'], ['overdue', 'Просрочено'], ['all', 'Все']] as [Deadline, string][]).map(([value, label]) => <button key={value} className={deadline === value ? 'active' : ''} onClick={() => setDeadline(value)}>{label}</button>)}</div><label>Предмет<select value={subject} onChange={(event) => setSubject(event.target.value)}><option value="">Все предметы</option>{subjects.data?.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>{student && <label className="checkbox-label"><input type="checkbox" checked={hideCompleted} onChange={(event) => setHideCompleted(event.target.checked)} />Скрыть выполненные</label>}</div>
    <QueryState isLoading={homework.isLoading} error={homework.error} retry={homework.refetch} isEmpty={!filtered.length} emptyTitle="Заданий не найдено" emptyText="Измените фильтры или выберите другой период."><div className="homework-grid">{filtered.map((item) => <article className="panel homework-item" key={item.id}><div className="section-heading"><div><p className="eyebrow">{item.subject_name} · {item.class_name}</p><h2>{item.text.split('\n')[0]?.slice(0, 100) ?? 'Домашнее задание'}</h2></div><StatusBadge tone={item.status === 'published' ? 'success' : 'warning'}>{item.status === 'published' ? 'Опубликовано' : 'Черновик'}</StatusBadge></div><p className="homework-text">{item.text}</p><div className="homework-meta"><span>Срок: <strong>{formatDate(item.due_date)}</strong></span>{item.due_date < dateISO() && !item.completed && <StatusBadge tone="danger">Просрочено</StatusBadge>}</div><FileControls item={item} />
      {student && item.status === 'published' && <label className="completion-check"><input type="checkbox" checked={item.completed ?? false} disabled={completion.isPending} onChange={(event) => completion.mutate({ item, completed: event.target.checked })} /><span><strong>Личная отметка о выполнении</strong><small>Не является официальной сдачей работы учителю</small></span></label>}
      {editable && <div className="card-actions"><button className="button secondary" onClick={() => setEditor({ homework: item })}><Pencil />Изменить</button><button className="button secondary" onClick={() => setEditor({ homework: item, copy: true })}><Copy />Копировать</button>{item.status === 'draft' && <button className="button primary" disabled={publish.isPending} onClick={() => publish.mutate(item)}><Send />Опубликовать</button>}</div>}
    </article>)}</div></QueryState>
    <HomeworkEditor state={editor} onClose={() => setEditor(null)} />
  </div>
}
