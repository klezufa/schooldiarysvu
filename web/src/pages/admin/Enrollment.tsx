import { useMemo, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowRightLeft, Search, UsersRound } from 'lucide-react'
import { api, apiPage, mutation } from '../../api/client'
import type { BulkResult, Enrollment, User } from '../../api/types'
import { dateISO } from '../../lib/dates'
import { defaultDateIn } from '../../lib/academicYears'
import { AdminForm } from '../../components/AdminForms'
import { send } from '../../lib/adminApi'
import { useToast } from '../../components/toastContext'
import { Field, QueryState } from '../../components/UI'
import { useDirectory } from './helpers'

export function EnrollmentAdmin(): React.JSX.Element {
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const { classes, years, classLabel, classOptions, openClasses } = useDirectory()
  const [search, setSearch] = useState('')
  const [onlyFree, setOnlyFree] = useState(true)
  const [selected, setSelected] = useState<string[]>([])
  const [targetClass, setTargetClass] = useState('')
  const [startsOn, setStartsOn] = useState(dateISO())
  const [source, setSource] = useState('')
  const [promoteTarget, setPromoteTarget] = useState('')
  const [promoteDate, setPromoteDate] = useState(dateISO())
  const [pending, setPending] = useState('')
  const [error, setError] = useState('')
  const students = useQuery({
    queryKey: ['users', 'enrollment-candidates', search],
    queryFn: async () => (await apiPage<User>(`/users?role=student&limit=100${search ? `&search=${encodeURIComponent(search)}` : ''}`)).items,
  })
  const enrollments = useQuery({ queryKey: ['enrollments'], queryFn: () => api<Enrollment[]>('/enrollments') })
  const candidates = useMemo(() => (students.data ?? []).filter((item) => item.student_id && (!onlyFree || !item.class_name)), [students.data, onlyFree])
  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: ['enrollments'] })
    await queryClient.invalidateQueries({ queryKey: ['users'] })
  }
  const report = (result: BulkResult, verb: string) => showToast(`${verb}: ${result.processed}${result.skipped ? `, уже были в классе: ${result.skipped}` : ''}`)

  const enroll = async (event: FormEvent) => {
    event.preventDefault()
    setPending('bulk')
    setError('')
    try {
      report(await mutation<BulkResult>('/enrollments/bulk', 'POST', { student_ids: selected, class_id: targetClass, starts_on: startsOn }), 'Зачислено')
      setSelected([])
      await refresh()
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Не удалось зачислить')
    } finally {
      setPending('')
    }
  }
  const promote = async (event: FormEvent) => {
    event.preventDefault()
    const label = (id: string) => { const found = classes.data?.find((item) => item.id === id); return found ? classLabel(found) : '' }
    const from = label(source)
    const to = label(promoteTarget)
    if (!window.confirm(`Перевести всех учеников ${from} в ${to} с ${promoteDate}? Текущие зачисления закроются накануне.`)) return
    setPending('promote')
    setError('')
    try {
      report(await mutation<BulkResult>(`/classes/${source}/promote`, 'POST', { target_class_id: promoteTarget, starts_on: promoteDate }), 'Переведено')
      await refresh()
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Не удалось перевести класс')
    } finally {
      setPending('')
    }
  }
  // Классы относятся к учебному году: дата зачисления по умолчанию — внутри года выбранного класса
  // (для класса следующего года — его первый день). Сервер дату вне года отклоняет.
  const yearOfClass = (id: string) => years.data?.find((year) => year.id === classes.data?.find((item) => item.id === id)?.academic_year_id)
  const enrollOptions = classOptions(openClasses)
  const sourceOptions = classOptions(classes.data)

  return (
    <div className="dash-grid">
      <section className="panel col-7">
        <div className="section-heading"><div><h2><UsersRound aria-hidden="true" className="inline-icon" /> Зачислить учеников</h2><p className="muted">Отметьте учеников и выберите класс. Если ученик уже учится в другом классе, его прежнее зачисление закроется накануне.</p></div></div>
        <form className="stack-form" onSubmit={enroll}>
          <div className="filter-bar">
            <label className="grow">Поиск
              <span className="input-with-icon"><Search aria-hidden="true" /><input type="search" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="ФИО или логин" /></span>
            </label>
            <label className="checkbox-label" style={{ minWidth: 0, flexDirection: 'row' }}><input type="checkbox" checked={onlyFree} onChange={(event) => setOnlyFree(event.target.checked)} />Только без класса</label>
          </div>
          <QueryState isLoading={students.isLoading} error={students.error} retry={students.refetch} isEmpty={!candidates.length} emptyTitle={onlyFree ? 'Все ученики уже в классах' : 'Учеников не найдено'}>
            <div className="recipient-grid tall">
              <label><input type="checkbox" checked={selected.length === candidates.length && candidates.length > 0} onChange={(event) => setSelected(event.target.checked ? candidates.map((item) => item.student_id!) : [])} /><strong>Выбрать всех ({candidates.length})</strong></label>
              {candidates.map((item) => (
                <label key={item.id}>
                  <input type="checkbox" checked={selected.includes(item.student_id!)} onChange={(event) => setSelected(event.target.checked ? [...selected, item.student_id!] : selected.filter((id) => id !== item.student_id))} />
                  {item.display_name}{item.class_name ? <small className="muted"> · {item.class_name}</small> : null}
                </label>
              ))}
            </div>
          </QueryState>
          <div className="form-row">
            <Field label="Класс *" htmlFor="bulk-class">
              <select id="bulk-class" required value={targetClass} onChange={(event) => { setTargetClass(event.target.value); setStartsOn(defaultDateIn(yearOfClass(event.target.value))) }}>
                <option value="">Выберите…</option>{enrollOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </Field>
            <Field label="С даты *" htmlFor="bulk-date"><input id="bulk-date" type="date" required min={yearOfClass(targetClass)?.starts_on} max={yearOfClass(targetClass)?.ends_on} value={startsOn} onChange={(event) => setStartsOn(event.target.value)} /></Field>
          </div>
          {error && <div className="form-error" role="alert">{error}</div>}
          <button className="button primary" disabled={!selected.length || !targetClass || pending === 'bulk'}>{pending === 'bulk' ? 'Зачисляем…' : `Зачислить выбранных (${selected.length})`}</button>
        </form>
      </section>
      <section className="panel col-5">
        <div className="section-heading"><div><h2><ArrowRightLeft aria-hidden="true" className="inline-icon" /> Перевод класса</h2><p className="muted">Переход на новый учебный год: 1) системный администратор создаёт год и его периоды; 2) в «Структуре» создаются классы нового года; 3) здесь каждый класс переводится (5А → 6А) с первого дня нового года; 4) в «Расписании» назначения и шаблон переносятся из класса прошлого года.</p></div></div>
        <form className="stack-form" onSubmit={promote}>
          <Field label="Из класса *" htmlFor="promote-from">
            <select id="promote-from" required value={source} onChange={(event) => setSource(event.target.value)}>
              <option value="">Выберите…</option>{sourceOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </Field>
          <Field label="В класс *" htmlFor="promote-to" hint="Класс нового учебного года создаётся во вкладке «Структура»">
            <select id="promote-to" required value={promoteTarget} onChange={(event) => { setPromoteTarget(event.target.value); setPromoteDate(defaultDateIn(yearOfClass(event.target.value))) }}>
              <option value="">Выберите…</option>{enrollOptions.filter((option) => option.value !== source).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </Field>
          <Field label="С даты *" htmlFor="promote-date" hint="Внутри учебного года класса назначения"><input id="promote-date" type="date" required min={yearOfClass(promoteTarget)?.starts_on} max={yearOfClass(promoteTarget)?.ends_on} value={promoteDate} onChange={(event) => setPromoteDate(event.target.value)} /></Field>
          <button className="button secondary" disabled={!source || !promoteTarget || pending === 'promote'}>{pending === 'promote' ? 'Переводим…' : 'Перевести класс'}</button>
        </form>
      </section>
      <section className="panel col-12">
        <div className="section-heading"><div><h2>Зачисления</h2><p className="muted">Действующие и завершённые</p></div></div>
        <QueryState isLoading={enrollments.isLoading} error={enrollments.error} retry={enrollments.refetch} isEmpty={!enrollments.data?.length} emptyTitle="Зачислений пока нет">
          <div className="responsive-table">
            <table>
              <thead><tr><th>Ученик</th><th>Класс</th><th>С</th><th>По</th><th>Завершить</th></tr></thead>
              <tbody>
                {enrollments.data?.map((item) => (
                  <tr key={item.id}>
                    <td>{item.student_name ?? item.student_id}</td>
                    <td>{item.class_name ?? item.class_id}</td>
                    <td>{item.starts_on}</td>
                    <td>{item.ends_on ?? 'действует'}</td>
                    <td>{!item.ends_on && (
                      <AdminForm fields={[{ name: 'ends_on', label: 'Дата окончания', type: 'date', required: true }]} submitLabel="Завершить"
                        onSubmit={async (values) => { await send(`/enrollments/${item.id}`, 'PATCH', { ends_on: values.ends_on }); await refresh(); showToast('Зачисление завершено') }} />
                    )}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </QueryState>
      </section>
    </div>
  )
}
