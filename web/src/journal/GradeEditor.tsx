import { useEffect, useRef, type KeyboardEvent } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { Save } from 'lucide-react'
import type { JournalEntry } from '../api/types'
import { workNames } from '../api/types'
import { formatDate } from '../lib/dates'
import { Field, Modal } from '../components/UI'
import { GradeHistory } from './GradeHistory'
import type { GradeConflict, GradeInput } from './useGradeMutation'

const gradeSchema = z.object({ value: z.string().min(1, 'Выберите оценку'), comment: z.string().max(1000, 'Не более 1000 символов') })
type GradeForm = z.infer<typeof gradeSchema>

export function GradeEditor({ entry, initialValue, open, onOpenChange, allowedValues, onSave, isSaving, conflict, onCloseConflict, nextName, onNext }: {
  entry: JournalEntry | null
  /** Значение, введённое с клавиатуры до открытия редактора; не заменяет серверное значение в `entry`. */
  initialValue?: number
  open: boolean
  onOpenChange: (open: boolean) => void
  allowedValues: number[]
  onSave: (input: GradeInput, onSaved?: () => void) => void
  isSaving: boolean
  conflict: GradeConflict | null
  onCloseConflict: () => void
  /** Мобильный журнал: имя следующего ученика и переход к нему после сохранения («Сохранить и к следующему»). */
  nextName?: string
  onNext?: () => void
}): React.JSX.Element {
  const { register, handleSubmit, setValue, reset, watch, formState: { errors } } = useForm<GradeForm>({ resolver: zodResolver(gradeSchema), defaultValues: { value: '', comment: '' } })
  const value = watch('value')
  const picker = useRef<HTMLFieldSetElement>(null)
  // Форма сбрасывается только при смене ячейки: обновление серверных данных не должно стирать локальный ввод.
  const entryKey = entry ? `${entry.student_id}:${entry.lesson_work_id}` : ''
  useEffect(() => {
    if (!entry) return
    const stored = entry.grade_value === null || entry.grade_value === undefined ? '' : String(entry.grade_value)
    reset({ value: initialValue === undefined ? stored : String(initialValue), comment: entry.grade_comment ?? '' })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [entryKey, initialValue, reset])
  // После сохранения окно закрывается или, если выбран «к следующему», переходит к следующему ученику (только после
  // успешного ответа сервера: при конфликте версий окно остаётся на текущем ученике с его вводом).
  const submit = (next: boolean) => handleSubmit((values) => {
    if (entry) onSave({ entry, value: Number(values.value), comment: values.comment }, next && onNext ? onNext : () => onOpenChange(false))
  })
  const save = submit(false)
  const keyboard = (event: KeyboardEvent<HTMLFormElement>) => {
    const target = event.target as HTMLElement
    if (target.tagName === 'TEXTAREA') return
    if (/^\d$/.test(event.key) && allowedValues.includes(Number(event.key))) { event.preventDefault(); setValue('value', event.key, { shouldValidate: true }); return }
    if (event.key === 'Enter' && value && !isSaving) { event.preventDefault(); void save() }
  }
  // Фокус на выбор оценки: иначе Radix фокусирует «Закрыть», и Enter закрыл бы окно вместо сохранения.
  const focusPicker = (event: Event) => {
    const target = picker.current?.querySelector<HTMLElement>('[aria-pressed="true"]') ?? picker.current?.querySelector<HTMLElement>('button')
    if (target) { event.preventDefault(); target.focus() }
  }
  return <Modal open={open} onOpenChange={onOpenChange} onOpenAutoFocus={focusPicker} title="Оценка ученика" description={entry ? `${entry.student_name} · ${formatDate(entry.lesson_date)}` : undefined}>
    {entry && <form className="stack-form" onSubmit={save} onKeyDown={keyboard}>
      <dl className="detail-list compact"><div><dt>Работа</dt><dd>{workNames[entry.work_type] ?? entry.work_type}: {entry.work_title}</dd></div><div><dt>Серверная версия</dt><dd>{entry.grade_version ?? 'Оценка ещё не создана'}</dd></div></dl>
      <fieldset className="grade-picker" ref={picker}><legend>Допустимые значения</legend>{allowedValues.map((item) => <button type="button" key={item} aria-pressed={value === String(item)} className={value === String(item) ? 'grade-choice selected' : 'grade-choice'} onClick={() => setValue('value', String(item), { shouldValidate: true })}>{item}</button>)}</fieldset>
      <input type="hidden" {...register('value')} />{errors.value && <p className="field-error" role="alert">{errors.value.message}</p>}
      <GradeHistory gradeId={entry.grade_id} version={entry.grade_version} />
      <Field label="Комментарий" htmlFor="grade-comment" error={errors.comment?.message}><textarea id="grade-comment" rows={3} placeholder="Необязательный комментарий" {...register('comment')} /></Field>
      {conflict && <div className="conflict-state" role="alert"><strong>Серверная версия изменилась</strong><p>{conflict.message}</p><dl><div><dt>Ваше несохранённое значение</dt><dd>{conflict.value}; {conflict.comment || 'без комментария'}</dd></div><div><dt>Текущее значение на сервере</dt><dd>{entry.grade_value ?? 'нет'}; версия {entry.grade_version ?? '—'}</dd></div></dl><p>Локальный ввод сохранён в этом окне. Обновите данные, проверьте их и сохраните повторно вручную.</p><button type="button" className="button secondary" onClick={onCloseConflict}>Загрузить серверную версию</button></div>}
      <div className="dialog-actions"><button className="button primary" disabled={isSaving || !value}><Save />{isSaving ? 'Сохраняем…' : 'Сохранить'}</button>{onNext && nextName && <button type="button" className="button secondary" disabled={isSaving || !value} onClick={() => void submit(true)()}>Сохранить и к следующему: {nextName}</button>}<span className="save-state" aria-live="polite">{isSaving ? 'Сохраняем' : conflict ? 'Не сохранено' : entry.grade_id && entry.grade_id !== 'pending' ? 'Сохранено на сервере' : 'Новая оценка'}</span></div>
      <p className="muted">Enter — сохранить, Escape — закрыть. Исправление оценки не удаляет историю изменений.</p>
    </form>}
  </Modal>
}
