import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { CopyPlus } from 'lucide-react'
import { api, mutation } from '../../api/client'
import type { AcademicPeriod, Assignment, CarryOverResult } from '../../api/types'
import { defaultDateIn, previousPeriod } from '../../lib/academicYears'
import { AdminForm } from '../../components/AdminForms'
import { send } from '../../lib/adminApi'
import { useToast } from '../../components/toastContext'
import { Field, QueryState } from '../../components/UI'
import { options, useDirectory, usePeople } from './helpers'
import { PeriodSelect } from './shared'

type Draft = { teacher_id: string; class_id: string; subject_id: string; starts_on: string; ends_on: string }
const emptyDraft: Draft = { teacher_id: '', class_id: '', subject_id: '', starts_on: '', ends_on: '' }

/**
 * Назначения «учитель — класс — предмет» действуют в пределах учебного периода. Экран работает с одним периодом:
 * классы — только его учебного года, даты — внутри периода, список — назначения этого периода. Раньше форма
 * предлагала первый период года даже в середине второй четверти, а список смешивал все периоды без подписи.
 */
export function TeachingAssignmentsPanel(): React.JSX.Element {
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const { subjects, years, periods, currentPeriod, classOptions, classesOfYear } = useDirectory()
  const teachers = usePeople('teacher')
  const assignments = useQuery({ queryKey: ['teaching-assignments'], queryFn: () => api<Assignment[]>('/teaching-assignments') })
  const [periodId, setPeriodId] = useState('')
  const [draft, setDraft] = useState<Draft>(emptyDraft)
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  const choosePeriod = (next: AcademicPeriod) => {
    setPeriodId(next.id)
    setDraft((current) => ({ ...current, class_id: '', starts_on: defaultDateIn(next), ends_on: '' }))
  }
  useEffect(() => {
    if (!periodId && currentPeriod) { setPeriodId(currentPeriod.id); setDraft((current) => ({ ...current, starts_on: defaultDateIn(currentPeriod) })) }
  }, [periodId, currentPeriod])
  const period = periods.data?.find((item) => item.id === periodId)
  const yearClasses = classesOfYear(period?.academic_year_id)
  const previous = previousPeriod(periods.data, period)
  const inPeriod = useMemo(() => (assignments.data ?? [])
    .filter((item) => item.academic_period_id === periodId)
    .sort((a, b) => (a.class_name ?? '').localeCompare(b.class_name ?? '', 'ru') || (a.subject_name ?? '').localeCompare(b.subject_name ?? '', 'ru')), [assignments.data, periodId])
  const teacherOptions = teachers.data?.filter((user) => user.teacher_id && user.is_active).map((user) => ({ value: user.teacher_id!, label: user.display_name })) ?? []
  const set = (patch: Partial<Draft>) => setDraft((current) => ({ ...current, ...patch }))
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['teaching-assignments'] })

  const create = async (event: FormEvent) => {
    event.preventDefault()
    setPending(true)
    setError('')
    try {
      await mutation('/teaching-assignments', 'POST', { ...draft, academic_period_id: periodId, ends_on: draft.ends_on || undefined })
      showToast('Назначение сохранено')
      setDraft((current) => ({ ...current, teacher_id: '', subject_id: '' }))
      await refresh()
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Не удалось сохранить назначение')
    } finally {
      setPending(false)
    }
  }
  const carryOver = async () => {
    if (!period || !previous) return
    setPending(true)
    try {
      const result = await mutation<CarryOverResult>(`/academic-periods/${period.id}/carry-over`, 'POST', { from_period_id: previous.id, include_schedule: false })
      showToast(`Перенесено назначений: ${result.assignments_created}${result.assignments_skipped ? `, уже были: ${result.assignments_skipped}` : ''}`)
      await refresh()
    } catch (caught) {
      showToast(caught instanceof Error ? caught.message : 'Не удалось перенести', 'error')
    } finally {
      setPending(false)
    }
  }

  return (
    <section className="panel">
      <h2>Учитель на класс и предмет</h2>
      <form className="stack-form" onSubmit={create}>
        <div className="form-row">
          <Field label="Учебный период *" htmlFor="assignment-period">
            <PeriodSelect id="assignment-period" value={periodId} onChange={choosePeriod} periods={periods.data} years={years.data} />
          </Field>
          <Field label="Класс *" htmlFor="assignment-class" hint={period && !yearClasses.length ? 'В учебном году этого периода ещё нет классов — создайте их во вкладке «Структура»' : undefined}>
            <select id="assignment-class" required value={draft.class_id} onChange={(event) => set({ class_id: event.target.value })}>
              <option value="">Выберите…</option>{classOptions(yearClasses).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </Field>
        </div>
        <div className="form-row">
          <Field label="Предмет *" htmlFor="assignment-subject">
            <select id="assignment-subject" required value={draft.subject_id} onChange={(event) => set({ subject_id: event.target.value })}>
              <option value="">Выберите…</option>{options(subjects.data).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </Field>
          <Field label="Учитель *" htmlFor="assignment-teacher">
            <select id="assignment-teacher" required value={draft.teacher_id} onChange={(event) => set({ teacher_id: event.target.value })}>
              <option value="">Выберите…</option>{teacherOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </Field>
        </div>
        <div className="form-row">
          <Field label="С *" htmlFor="assignment-from" hint="Внутри периода; новый учитель в середине четверти — с даты замены">
            <input id="assignment-from" type="date" required min={period?.starts_on} max={period?.ends_on} value={draft.starts_on} onChange={(event) => set({ starts_on: event.target.value })} />
          </Field>
          <Field label="По" htmlFor="assignment-to" hint="Пусто — до конца периода">
            <input id="assignment-to" type="date" min={draft.starts_on || period?.starts_on} max={period?.ends_on} value={draft.ends_on} onChange={(event) => set({ ends_on: event.target.value })} />
          </Field>
        </div>
        {error && <div className="form-error" role="alert">{error}</div>}
        <button className="button primary" disabled={pending || !periodId}>{pending ? 'Сохраняем…' : 'Назначить'}</button>
      </form>

      <h3 className="list-heading">Назначения на «{period?.name ?? '…'}» ({inPeriod.length})</h3>
      {period && previous && period.status !== 'closed' && assignments.isSuccess && inPeriod.length === 0 && (
        previous.academic_year_id === period.academic_year_id ? (
          <div className="notice info period-hint carry-over" role="status">
            <CopyPlus aria-hidden="true" />
            <span><strong>В «{period.name}» назначений пока нет.</strong> Перенесите назначения всех классов из «{previous.name}» — потом поправьте, если учителя сменились.</span>
            <button type="button" className="button small primary" disabled={pending} onClick={carryOver}>Перенести из «{previous.name}»</button>
          </div>
        ) : (
          <div className="notice info period-hint" role="status">
            <CopyPlus aria-hidden="true" />
            <span>Это первый период нового учебного года: классы в нём новые. Назначения прошлого года переносятся по классам (5А → 6А) во вкладке «Расписание» вместе с шаблоном.</span>
          </div>
        )
      )}
      <QueryState isLoading={assignments.isLoading} error={assignments.error} retry={assignments.refetch}>
        <div className="entity-list">
          {inPeriod.map((item) => (
            <div className="entity-row" key={item.id}>
              <div>
                <strong>{item.subject_name ?? 'Предмет'} · {item.class_name ?? 'класс'}</strong>
                <small>{item.teacher_name ?? 'Учитель'} · с {item.starts_on}{item.ends_on ? ` по ${item.ends_on}` : ' до конца периода'}</small>
              </div>
              {!item.ends_on && (
                <AdminForm fields={[{ name: 'ends_on', label: 'Завершить', type: 'date', required: true }]} submitLabel="Завершить"
                  onSubmit={async (values) => { await send(`/teaching-assignments/${item.id}`, 'PATCH', values); await refresh(); showToast('Назначение завершено') }} />
              )}
            </div>
          ))}
        </div>
      </QueryState>
    </section>
  )
}
