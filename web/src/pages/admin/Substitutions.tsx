import { useId, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Repeat, Undo2, UserCheck, Wand2 } from 'lucide-react'
import { api, mutation, queryString } from '../../api/client'
import type { SubstitutionCandidate, SubstitutionLesson } from '../../api/types'
import { addDays, dateISO, formatDate, formatTime } from '../../lib/dates'
import { QueryState, StatusBadge } from '../../components/UI'
import { useToast } from '../../components/toastContext'
import { usePeople } from './helpers'
import { candidateReasonText, monthRange, substituteTotals } from './substitutionsModel'

const reasons = ['Болезнь', 'Курсы повышения квалификации', 'Командировка', 'Отпуск', 'Семейные обстоятельства']
const shownCandidates = 4

/**
 * Замены. Администратор выбирает отсутствующего учителя и дни — видит его уроки и для каждого свободных учителей,
 * лучшие первыми: кто ведёт этот предмет, кто знает класс, кто в этот день и так в школе. Класс и заменяющий
 * получают уведомление; ниже — учёт замен за месяц для табеля.
 */
export function SubstitutionsPanel(): React.JSX.Element {
  const id = useId()
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const teachers = usePeople('teacher')
  const [teacherId, setTeacherId] = useState('')
  const [from, setFrom] = useState(dateISO())
  const [to, setTo] = useState(dateISO())
  const [reason, setReason] = useState(reasons[0]!)
  const [busy, setBusy] = useState(false)
  const teacherOptions = (teachers.data ?? []).filter((user) => user.teacher_id && user.is_active)
  const validRange = Boolean(from && to && from <= to)
  const lessons = useQuery({
    queryKey: ['substitutions', 'teacher', teacherId, from, to],
    queryFn: () => api<SubstitutionLesson[]>(`/substitutions${queryString({ from, to, teacher_id: teacherId })}`),
    enabled: Boolean(teacherId) && validRange,
  })
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['substitutions'] })
  // Уроки, которые ещё ведёт сам отсутствующий: им нужна замена.
  const open = (lessons.data ?? []).filter((lesson) => lesson.teacher_id === teacherId && lesson.status !== 'cancelled' && !lesson.original_teacher_id)
  const assignAll = async () => {
    if (!window.confirm(`Назначить на ${open.length} ${open.length === 1 ? 'урок' : 'уроков'} первого предложенного учителя? Любую замену потом можно отменить или поменять.`)) return
    setBusy(true)
    let assigned = 0
    try {
      // По одному: после каждой замены учитель занят, и следующий подбор это учитывает.
      for (const lesson of open) {
        const candidates = await api<SubstitutionCandidate[]>(`/substitutions/candidates?lesson_id=${lesson.id}`)
        const best = candidates[0]
        if (!best) continue
        await mutation(`/lessons/${lesson.id}/substitution`, 'PUT', { teacher_id: best.teacher_id, reason })
        assigned++
      }
      showToast(assigned === open.length ? `Замены назначены: ${assigned}` : `Назначено ${assigned} из ${open.length}. Для остальных свободных учителей нет`, assigned === open.length ? 'success' : 'error')
    } catch (caught) {
      showToast(caught instanceof Error ? caught.message : 'Не удалось назначить замены', 'error')
    } finally {
      setBusy(false)
      await refresh()
    }
  }
  return (
    <div className="substitutions">
      <section className="panel">
        <div className="section-heading">
          <div><h2>Кто отсутствует</h2><p className="muted">Выберите учителя и дни — дневник покажет его уроки и предложит, кто может заменить. Класс, родители и заменяющий получат уведомление.</p></div>
        </div>
        <div className="filter-bar">
          <label className="grow">Учитель
            <select value={teacherId} onChange={(event) => setTeacherId(event.target.value)}>
              <option value="">Выберите учителя…</option>
              {teacherOptions.map((user) => <option key={user.teacher_id} value={user.teacher_id!}>{user.display_name}</option>)}
            </select>
          </label>
          <label>С<input type="date" value={from} onChange={(event) => { setFrom(event.target.value); if (event.target.value > to) setTo(event.target.value) }} /></label>
          <label>По<input type="date" value={to} min={from} onChange={(event) => setTo(event.target.value)} /></label>
          <label className="grow">Причина
            <input list={`${id}-reasons`} value={reason} maxLength={300} onChange={(event) => setReason(event.target.value)} />
            <datalist id={`${id}-reasons`}>{reasons.map((item) => <option key={item} value={item} />)}</datalist>
          </label>
        </div>
        <div className="card-actions substitution-days">
          <button type="button" className="button secondary small" onClick={() => { setFrom(dateISO()); setTo(dateISO()) }}>Сегодня</button>
          <button type="button" className="button secondary small" onClick={() => { const next = addDays(dateISO(), 1); setFrom(next); setTo(next) }}>Завтра</button>
          <button type="button" className="button secondary small" onClick={() => { setFrom(dateISO()); setTo(addDays(dateISO(), 6)) }}>Неделя</button>
        </div>
        {teacherId ? (
          <QueryState isLoading={lessons.isLoading} error={lessons.error} retry={lessons.refetch} isEmpty={!lessons.data?.length}
            emptyTitle="Уроков нет" emptyText="В эти дни у учителя нет уроков в расписании.">
            {open.length > 1 && (
              <div className="notice info substitution-bulk">
                <Wand2 aria-hidden="true" />
                <span>Уроков без замены: {open.length}. Можно подобрать для каждого или назначить сразу всех предложенных.</span>
                <button type="button" className="button primary small" disabled={busy} onClick={() => void assignAll()}>Назначить предложенных</button>
              </div>
            )}
            <ul className="simple-list substitution-list">
              {lessons.data?.map((lesson) => <SubstitutionRow key={lesson.id} lesson={lesson} absentTeacherId={teacherId} reason={reason} disabled={busy} onChanged={refresh} />)}
            </ul>
          </QueryState>
        ) : <p className="muted">Сначала выберите учителя.</p>}
      </section>
      <SubstitutionLog />
    </div>
  )
}

function lessonTitle(lesson: SubstitutionLesson): string {
  return `${formatDate(lesson.lesson_date, { weekday: 'short', day: 'numeric', month: 'long' })}, ${formatTime(lesson.starts_at)} · ${lesson.subject_name}, ${lesson.class_name}`
}

function SubstitutionRow({ lesson, absentTeacherId, reason, disabled, onChanged }: {
  lesson: SubstitutionLesson; absentTeacherId: string; reason: string; disabled: boolean; onChanged: () => Promise<unknown>
}): React.JSX.Element {
  const { showToast } = useToast()
  const [picking, setPicking] = useState(false)
  const [showAll, setShowAll] = useState(false)
  const [pending, setPending] = useState(false)
  const candidates = useQuery({
    queryKey: ['substitutions', 'candidates', lesson.id],
    queryFn: () => api<SubstitutionCandidate[]>(`/substitutions/candidates?lesson_id=${lesson.id}`),
    enabled: picking,
  })
  const run = async (action: () => Promise<unknown>, message: string) => {
    setPending(true)
    try {
      await action()
      await onChanged()
      setPicking(false)
      showToast(message)
    } catch (caught) {
      showToast(caught instanceof Error ? caught.message : 'Не удалось сохранить', 'error')
    } finally {
      setPending(false)
    }
  }
  const assign = (candidate: SubstitutionCandidate) => run(() => mutation(`/lessons/${lesson.id}/substitution`, 'PUT', { teacher_id: candidate.teacher_id, reason }),
    `${candidate.display_name} заменит: ${lesson.subject_name}, ${lesson.class_name}`)
  const undo = () => {
    if (!window.confirm('Отменить замену? Урок вернётся учителю по расписанию, класс получит уведомление.')) return
    void run(() => mutation(`/lessons/${lesson.id}/substitution`, 'DELETE'), 'Замена отменена')
  }
  const substitutedAway = lesson.original_teacher_id === absentTeacherId
  const list = candidates.data ?? []
  const visible = showAll ? list : list.slice(0, shownCandidates)
  return (
    <li className="substitution-row">
      <div>
        <strong>{lessonTitle(lesson)}</strong>
        <small>
          {lesson.room_name ? `Кабинет ${lesson.room_name}` : 'Кабинет не указан'}
          {substitutedAway && <> · заменяет <b>{lesson.teacher_name}</b>{lesson.reason ? ` (${lesson.reason})` : ''}</>}
          {!substitutedAway && lesson.original_teacher_name && <> · сам заменяет: {lesson.original_teacher_name}</>}
        </small>
      </div>
      <div className="card-actions">
        {lesson.status === 'cancelled' && <StatusBadge>Урок отменён</StatusBadge>}
        {lesson.status !== 'cancelled' && substitutedAway && (
          <>
            <StatusBadge tone="success"><UserCheck aria-hidden="true" />Замена назначена</StatusBadge>
            <button type="button" className="button secondary small" disabled={pending || disabled} onClick={() => setPicking((value) => !value)}><Repeat aria-hidden="true" />Другой учитель</button>
            <button type="button" className="button secondary small" disabled={pending || disabled} onClick={undo}><Undo2 aria-hidden="true" />Отменить</button>
          </>
        )}
        {lesson.status !== 'cancelled' && !lesson.original_teacher_id && (
          <button type="button" className={`button small ${picking ? 'secondary' : 'primary'}`} disabled={pending || disabled} onClick={() => setPicking((value) => !value)}>
            {picking ? 'Скрыть' : 'Найти замену'}
          </button>
        )}
      </div>
      {picking && (
        <div className="substitution-candidates">
          <QueryState isLoading={candidates.isLoading} error={candidates.error} retry={candidates.refetch} isEmpty={!list.length}
            emptyTitle="Свободных учителей нет" emptyText="В это время заняты все. Урок можно отменить в расписании.">
            <ul className="simple-list compact" aria-label="Кто может заменить">
              {visible.map((candidate, index) => (
                <li key={candidate.teacher_id}>
                  <div>
                    <strong>{candidate.display_name}{index === 0 && <StatusBadge tone="info">Лучший вариант</StatusBadge>}</strong>
                    <small>{candidate.lessons_that_day ? `Уроков в этот день: ${candidate.lessons_that_day}` : 'В этот день уроков нет'}</small>
                    {candidate.reasons.length > 0 && <span className="reason-chips">{candidate.reasons.map((item) => <span key={item} className="badge badge-info">{candidateReasonText[item]}</span>)}</span>}
                  </div>
                  <button type="button" className="button primary small" disabled={pending} onClick={() => void assign(candidate)}>Назначить</button>
                </li>
              ))}
            </ul>
            {list.length > shownCandidates && (
              <button type="button" className="button text-button" onClick={() => setShowAll((value) => !value)}>{showAll ? 'Показать лучших' : `Показать всех свободных (${list.length})`}</button>
            )}
          </QueryState>
        </div>
      )}
    </li>
  )
}

/** Замены за месяц — для табеля и оплаты: сколько уроков провёл каждый заменяющий. */
function SubstitutionLog(): React.JSX.Element {
  const [month, setMonth] = useState(dateISO().slice(0, 7))
  const range = monthRange(month)
  const log = useQuery({
    queryKey: ['substitutions', 'month', month],
    queryFn: () => api<SubstitutionLesson[]>(`/substitutions${queryString(range)}`),
    enabled: Boolean(month),
  })
  const done = useMemo(() => (log.data ?? []).filter((lesson) => lesson.status !== 'cancelled'), [log.data])
  const totals = useMemo(() => substituteTotals(done), [done])
  return (
    <section className="panel">
      <div className="section-heading">
        <div><h2>Учёт замен</h2><p className="muted">Кто кого заменял за месяц — для табеля и оплаты.</p></div>
        <label>Месяц<input type="month" value={month} onChange={(event) => setMonth(event.target.value)} /></label>
      </div>
      <QueryState isLoading={log.isLoading} error={log.error} retry={log.refetch} isEmpty={!done.length} emptyTitle="Замен не было" emptyText="За этот месяц замены не назначались.">
        <div className="substitution-totals">
          {totals.map((item) => <div key={item.name}><strong>{item.lessons}</strong><span>{item.name}</span></div>)}
        </div>
        <div className="responsive-table">
          <table>
            <thead><tr><th>Дата и время</th><th>Класс и предмет</th><th>Заменял</th><th>Вместо</th><th>Причина</th></tr></thead>
            <tbody>
              {done.map((lesson) => (
                <tr key={lesson.id}>
                  <td>{formatDate(lesson.lesson_date, { day: 'numeric', month: 'short' })}, {formatTime(lesson.starts_at)}</td>
                  <td>{lesson.class_name}, {lesson.subject_name}</td>
                  <td>{lesson.teacher_name}</td>
                  <td>{lesson.original_teacher_name}</td>
                  <td>{lesson.reason || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </QueryState>
    </section>
  )
}
