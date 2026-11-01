import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate } from 'react-router-dom'
import { AlertCircle, ArrowRight, BookOpen, CalendarCheck, ClipboardList, Clock3, FilePenLine, HeartHandshake } from 'lucide-react'
import { api, queryString } from '../../api/client'
import type { CurrentLessons, Lesson, TeacherAnalytics } from '../../api/types'
import { useAuth } from '../../authContext'
import { dateISO, formatTime } from '../../lib/dates'
import { subjectStyle } from '../../lib/subjects'
import { ChartCard, LineChart, Meter, SegmentedBar, StatTile } from '../../components/charts'
import { formatNumber, gradeTone } from '../../lib/chartMath'
import { QueryState, StatusBadge } from '../../components/UI'
import { EmptyPeriodHint, PeriodSelect } from './PeriodControls'
import { useDashboardPeriod } from './usePeriods'
import { distributionParts, firstName, greeting, weekLabel } from './helpers'

export function TeacherDashboard(): React.JSX.Element {
  const { user } = useAuth()
  const navigate = useNavigate()
  const today = dateISO()
  const { periodId, setPeriodId, periods, previous } = useDashboardPeriod()
  const analytics = useQuery({ queryKey: ['analytics-teacher', periodId], queryFn: () => api<TeacherAnalytics>(`/analytics/teacher${queryString({ academic_period_id: periodId })}`) })
  const current = useQuery({ queryKey: ['schedule-current'], queryFn: () => api<CurrentLessons>('/schedule/current') })
  const schedule = useQuery({ queryKey: ['schedule', 'self', today, today], queryFn: () => api<Lesson[]>(`/schedule?from=${today}&to=${today}`) })
  const data = analytics.data
  const focus = current.data?.current ?? current.data?.next
  const attention = (data?.unmarked_past_lessons ?? 0) + (data?.unfilled_lessons ?? 0) + (data?.draft_homework ?? 0)
  return (
    <div className="dashboard">
      <section className="hero-banner teacher-hero">
        <div>
          <p className="eyebrow">{greeting()}, {firstName(user?.display_name)}</p>
          {current.isLoading ? <h1>Определяем текущий урок…</h1> : focus ? (
            <>
              <h1>{current.data?.current ? 'Сейчас идёт' : 'Следующий урок'}: {focus.subject_name} · {focus.class_name}</h1>
              <p><Clock3 aria-hidden="true" className="inline-icon" /> {formatTime(focus.starts_at)}–{formatTime(focus.ends_at)}{focus.room_name ? ` · кабинет ${focus.room_name}` : ''} · {focus.topic || 'тема не указана'}</p>
              <div className="hero-meta">
                <button className="button primary" onClick={() => navigate('/journal', { state: { lesson: focus } })}><BookOpen aria-hidden="true" />Открыть журнал урока</button>
                <PeriodSelect value={periodId} onChange={setPeriodId} periods={periods} />
              </div>
            </>
          ) : (
            <>
              <h1>Уроков сегодня больше нет</h1>
              <p>Хорошее время проверить задания и заполнить журнал.</p>
              <div className="hero-meta"><PeriodSelect value={periodId} onChange={setPeriodId} periods={periods} /></div>
            </>
          )}
        </div>
      </section>
      {!periodId && data && !data.distribution.length && <EmptyPeriodHint previous={previous} onShow={setPeriodId} />}
      <div className="stat-grid">
        <StatTile icon={<CalendarCheck />} tone="blue" label="Уроков сегодня" value={data?.today_lessons ?? '…'} detail={data?.period_name ?? ''} />
        <StatTile icon={<AlertCircle />} tone={data && data.unmarked_past_lessons > 0 ? 'amber' : 'green'} label="Не отмечены проведёнными" value={data?.unmarked_past_lessons ?? '…'} detail="прошедшие уроки со статусом «запланирован»" />
        <StatTile icon={<FilePenLine />} tone={data && data.unfilled_lessons > 0 ? 'rose' : 'green'} label="Без оценок или посещаемости" value={data?.unfilled_lessons ?? '…'} detail="проведённые уроки периода" />
        <StatTile icon={<ClipboardList />} tone="violet" label="Задания" value={data ? `${data.active_homework}` : '…'} detail={data ? `активных, черновиков: ${data.draft_homework}` : ''} />
      </div>
      <div className="dash-grid">
        <section className="panel col-7">
          <div className="section-heading">
            <div><h2>Мои классы</h2><p className="muted">Средний балл и посещаемость по вашим урокам</p></div>
            <Link to="/reports">Отчёты<ArrowRight /></Link>
          </div>
          <QueryState isLoading={analytics.isLoading} error={analytics.error} retry={analytics.refetch} isEmpty={!data?.classes.length} emptyTitle="Нет назначений на текущий период" emptyText="Администратор назначает классы и предметы в разделе «Администрирование → Назначения».">
            <div className="class-stats">
              {data?.classes.map((item) => (
                <div className="class-stat" key={`${item.class_id}-${item.subject_id}`} style={subjectStyle(item.subject_name)}>
                  <div><strong>{item.class_name} · {item.subject_name}</strong><small>{item.students} учеников</small></div>
                  <span className={`big-average grade-chip tone-${item.average != null ? gradeTone(item.average) : 'neutral'}`}>{item.average != null ? formatNumber(item.average) : '—'}</span>
                  <Meter label="Посещаемость" value={item.attendance_rate} tone={item.attendance_rate != null && item.attendance_rate < 0.85 ? 'warning' : 'good'} />
                </div>
              ))}
            </div>
          </QueryState>
        </section>
        <div className="col-5">
          <ChartCard title="Распределение оценок" subtitle="Все ваши оценки за период" table={{ columns: ['Оценка', 'Количество'], rows: (data?.distribution ?? []).map((item) => [item.value, item.count]) }}>
            <SegmentedBar parts={distributionParts(data?.distribution ?? [])} ariaLabel="Распределение выставленных оценок" />
            <p className="muted chart-note">{attention ? `Требуют внимания: ${attention}` : 'Журнал в порядке'}</p>
          </ChartCard>
        </div>
        <div className="col-7">
          <ChartCard title="Динамика среднего балла" subtitle="По неделям периода, все классы" table={{ columns: ['Неделя', 'Средний балл', 'Оценок'], rows: (data?.dynamics ?? []).map((item) => [weekLabel(item.week_start), formatNumber(item.average), item.grades_count]) }}>
            <LineChart points={(data?.dynamics ?? []).map((item) => ({ key: item.week_start, label: weekLabel(item.week_start), value: item.average, hint: `${item.grades_count} оценок` }))} domain={[1, 5]} ariaLabel="Средний балл по неделям" />
          </ChartCard>
        </div>
        <section className="panel col-5">
          <div className="section-heading"><div><h2>Нужна поддержка</h2><p className="muted">Самый низкий средний балл (от 3 оценок)</p></div><HeartHandshake aria-hidden="true" /></div>
          <QueryState isLoading={analytics.isLoading} error={analytics.error} retry={analytics.refetch} isEmpty={!data?.low_performers.length} emptyTitle="Пока некого выделить">
            <ul className="event-list">
              {data?.low_performers.map((student) => (
                <li key={student.student_id}>
                  <span className={`grade-chip tone-${gradeTone(student.average)}`}>{formatNumber(student.average, 1)}</span>
                  <div className="event-body"><strong>{student.display_name}</strong><span>{student.class_name} · {student.grades_count} оценок</span></div>
                </li>
              ))}
            </ul>
          </QueryState>
        </section>
        <section className="panel col-12">
          <div className="section-heading"><div><h2>Уроки сегодня</h2><p className="muted">Нажмите на урок, чтобы открыть его журнал</p></div><Link to="/schedule">Вся неделя<ArrowRight /></Link></div>
          <QueryState isLoading={schedule.isLoading} error={schedule.error} retry={schedule.refetch} isEmpty={!schedule.data?.length} emptyTitle="Сегодня нет уроков">
            <ul className="event-list">
              {schedule.data?.map((lesson) => (
                <li key={lesson.id} style={subjectStyle(lesson.subject_name)}>
                  <span className="event-icon" style={{ background: 'var(--subject-wash)', color: 'var(--subject)' }}><BookOpen aria-hidden="true" /></span>
                  <div className="event-body">
                    <strong>{formatTime(lesson.starts_at)} · {lesson.subject_name} · {lesson.class_name}</strong>
                    <span>{lesson.topic || 'Тема не указана'}{lesson.room_name ? ` · каб. ${lesson.room_name}` : ''}</span>
                  </div>
                  <StatusBadge tone={lesson.status === 'completed' ? 'success' : lesson.status === 'cancelled' ? 'danger' : 'warning'}>
                    {lesson.status === 'completed' ? 'Завершён' : lesson.status === 'cancelled' ? 'Отменён' : 'Запланирован'}
                  </StatusBadge>
                  <button className="button small secondary" disabled={lesson.status === 'cancelled'} onClick={() => navigate('/journal', { state: { lesson } })}>Журнал</button>
                </li>
              ))}
            </ul>
          </QueryState>
        </section>
      </div>
    </div>
  )
}
