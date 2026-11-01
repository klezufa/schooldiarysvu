import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { ArrowRight, BookOpenCheck, CalendarDays, CheckCircle2, ClipboardList, Megaphone, Star, UserCheck } from 'lucide-react'
import { api, queryString } from '../../api/client'
import type { Announcement, Lesson, StudentAnalytics } from '../../api/types'
import { useAuth } from '../../authContext'
import { useStudent } from '../../studentContext'
import { scopedKey } from '../../lib/student'
import { dateISO, formatDate, formatTime } from '../../lib/dates'
import { subjectStyle } from '../../lib/subjects'
import { ChartCard, ColumnChart, LineChart, Meter, SegmentedBar, StatTile } from '../../components/charts'
import { formatNumber, formatPercent, gradeTone } from '../../lib/chartMath'
import { Empty, QueryState, StatusBadge } from '../../components/UI'
import { HeroBanner } from './HeroBanner'
import { EmptyPeriodHint, PeriodSelect } from './PeriodControls'
import { useDashboardPeriod } from './usePeriods'
import { averageTone, distributionParts, dueLabel, firstName, gradeColumns, weekLabel } from './helpers'

export function StudentDashboard(): React.JSX.Element {
  const { user } = useAuth()
  const { studentId, children, isLoading: childrenLoading } = useStudent()
  const isParent = Boolean(user?.roles.includes('parent'))
  const child = children.find((item) => item.id === studentId)
  const today = dateISO()
  const { periodId, setPeriodId, periods, previous } = useDashboardPeriod()
  const analytics = useQuery({
    queryKey: scopedKey('analytics-student', studentId, periodId),
    queryFn: () => api<StudentAnalytics>(`/analytics/student${queryString({ student_id: isParent ? studentId : undefined, academic_period_id: periodId })}`),
    enabled: Boolean(studentId),
  })
  const lessons = useQuery({
    queryKey: scopedKey('schedule', studentId, today, today),
    queryFn: () => api<Lesson[]>(`/schedule${queryString({ from: today, to: today, student_id: studentId })}`),
    enabled: Boolean(studentId),
  })
  const announcements = useQuery({
    queryKey: scopedKey('announcements', studentId),
    queryFn: () => api<Announcement[]>(`/announcements${queryString({ student_id: studentId })}`),
    enabled: Boolean(studentId),
  })
  if (isParent && !childrenLoading && !children.length) {
    return <Empty title="К учётной записи не привязаны дети" text="Обратитесь к администратору школы: он свяжет вашу учётную запись с учётной записью ребёнка." />
  }
  const data = analytics.data
  const active = lessons.data?.filter((lesson) => lesson.status !== 'cancelled') ?? []
  const next = active.find((lesson) => new Date(lesson.ends_at).getTime() > Date.now())
  const title = isParent ? `${firstName(user?.display_name) ? `Здравствуйте, ${firstName(user?.display_name)}!` : 'Здравствуйте!'}` : `Добро пожаловать, ${firstName(user?.display_name)}!`
  const subtitle = isParent && child ? `Успехи и расписание: ${child.display_name}` : 'Пусть этот учебный год будет успешным!'
  const attendance = data?.attendance_rate
  return (
    <div className="dashboard">
      <HeroBanner
        title={title}
        text={subtitle}
        meta={<PeriodSelect value={periodId} onChange={setPeriodId} periods={periods} />}
      />
      {!periodId && data && data.grades_count === 0 && <EmptyPeriodHint previous={previous} onShow={setPeriodId} />}
      <div className="stat-grid">
        <StatTile icon={<CalendarDays />} tone="blue" label="Сегодня" value={formatDate(today, { day: 'numeric', month: 'long' })} detail={formatDate(today, { weekday: 'long' })} />
        <StatTile
          icon={<BookOpenCheck />}
          tone="teal"
          label="Уроков"
          value={lessons.isLoading ? '…' : active.length}
          detail={next ? `Дальше: ${next.subject_name}, ${formatTime(next.starts_at)}` : active.length ? 'На сегодня всё' : 'Уроков нет'}
        />
        <StatTile
          icon={<Star />}
          tone={averageTone(data?.average)}
          label="Средний балл"
          value={data?.average != null ? formatNumber(data.average, 2) : '—'}
          detail={data ? `${data.grades_count} оценок за период` : 'Загружаем…'}
        />
        <StatTile
          icon={<UserCheck />}
          tone="violet"
          label="Посещаемость"
          value={formatPercent(attendance)}
          detail={data ? `Пропусков: ${data.absences}, опозданий: ${data.late_count}` : ''}
        />
      </div>
      <div className="dash-grid">
        <section className="panel col-7">
          <div className="section-heading">
            <div><h2>Ближайшие задания</h2><p className="muted">Что сдать в ближайшие дни</p></div>
            <Link to="/homework">Все задания<ArrowRight /></Link>
          </div>
          <QueryState isLoading={analytics.isLoading} error={analytics.error} retry={analytics.refetch} isEmpty={!data?.upcoming_homework.length} emptyTitle="Заданий на ближайшие дни нет">
            <ul className="event-list">
              {data?.upcoming_homework.map((item) => {
                const due = dueLabel(item.due_date)
                return (
                  <li key={item.id}>
                    <span className={`event-icon ${item.completed ? 'tone-green' : 'tone-blue'}`}>{item.completed ? <CheckCircle2 aria-hidden="true" /> : <ClipboardList aria-hidden="true" />}</span>
                    <div className="event-body"><strong>{item.subject_name}</strong><span>{item.text}</span></div>
                    {item.completed ? <StatusBadge tone="success">Выполнено</StatusBadge> : <StatusBadge tone={due.tone}>{due.text}</StatusBadge>}
                  </li>
                )
              })}
            </ul>
          </QueryState>
        </section>
        <section className="panel col-5">
          <div className="section-heading">
            <div><h2>Сегодня по расписанию</h2><p className="muted">{active.length ? `${active.length} уроков` : 'Свободный день'}</p></div>
            <Link to="/schedule">Неделя<ArrowRight /></Link>
          </div>
          <QueryState isLoading={lessons.isLoading} error={lessons.error} retry={lessons.refetch} isEmpty={!lessons.data?.length} emptyTitle="Сегодня уроков нет">
            <ul className="event-list">
              {lessons.data?.map((lesson) => (
                <li key={lesson.id} style={subjectStyle(lesson.subject_name)}>
                  <span className="event-icon" style={{ background: 'var(--subject-wash)', color: 'var(--subject)' }}><BookOpenCheck aria-hidden="true" /></span>
                  <div className="event-body">
                    <strong>{lesson.subject_name}</strong>
                    <span>{formatTime(lesson.starts_at)}–{formatTime(lesson.ends_at)}{lesson.room_name ? ` · каб. ${lesson.room_name}` : ''}{lesson.topic ? ` · ${lesson.topic}` : ''}</span>
                  </div>
                  {lesson.status === 'cancelled' && <StatusBadge tone="danger">Отменён</StatusBadge>}
                </li>
              ))}
            </ul>
          </QueryState>
        </section>

        <div className="col-7">
          <ChartCard
            title="Средний балл по предметам"
            subtitle={data?.period_name ? `Средневзвешенный, ${data.period_name}` : 'Средневзвешенный за период'}
            table={{ columns: ['Предмет', 'Средний балл', 'Оценок', 'Итоговая'], rows: (data?.subjects ?? []).map((item) => [item.subject_name, item.average != null ? formatNumber(item.average) : '—', item.grades_count, item.term_value ?? '—']) }}
          >
            <QueryState isLoading={analytics.isLoading} error={analytics.error} retry={analytics.refetch}>
              <ColumnChart data={gradeColumns(data?.subjects ?? [])} max={5} ariaLabel="Средний балл по предметам" emptyText="Оценок за период пока нет" />
            </QueryState>
          </ChartCard>
        </div>
        <div className="col-5">
          <ChartCard
            title="Динамика успеваемости"
            subtitle="Средний балл по неделям"
            table={{ columns: ['Неделя', 'Средний балл', 'Оценок'], rows: (data?.dynamics ?? []).map((item) => [weekLabel(item.week_start), formatNumber(item.average), item.grades_count]) }}
          >
            <QueryState isLoading={analytics.isLoading} error={analytics.error} retry={analytics.refetch}>
              <LineChart
                points={(data?.dynamics ?? []).map((item) => ({ key: item.week_start, label: weekLabel(item.week_start), value: item.average, hint: `${item.grades_count} оценок` }))}
                domain={[1, 5]}
                ariaLabel="Средний балл по неделям"
                color="var(--series-1)"
              />
            </QueryState>
          </ChartCard>
        </div>

        <div className="col-5">
          <ChartCard title="Из чего складываются оценки" subtitle="Количество оценок каждого вида за период"
            table={{ columns: ['Оценка', 'Количество'], rows: (data?.distribution ?? []).map((item) => [item.value, item.count]) }}>
            <SegmentedBar parts={distributionParts(data?.distribution ?? [])} ariaLabel="Распределение оценок" />
          </ChartCard>
        </div>
        <section className="panel col-4">
          <div className="section-heading"><h2>Последние оценки</h2><Link to="/grades">Все<ArrowRight /></Link></div>
          <QueryState isLoading={analytics.isLoading} error={analytics.error} retry={analytics.refetch} isEmpty={!data?.recent.length} emptyTitle="Оценок пока нет">
            <ul className="event-list">
              {data?.recent.map((grade, index) => (
                <li key={`${grade.lesson_date}-${index}`}>
                  <span className={`grade-chip tone-${gradeTone(grade.value)}`}>{formatNumber(grade.value)}</span>
                  <div className="event-body"><strong>{grade.subject_name}</strong><span>{grade.title} · {formatDate(grade.lesson_date, { day: 'numeric', month: 'short' })}</span></div>
                </li>
              ))}
            </ul>
          </QueryState>
        </section>
        <section className="panel col-3">
          <h2>Задания</h2>
          <div className="meter-stack">
            <Meter label="Выполнено" tone="good" value={data ? ratioOf(data.homework.done, data.homework.done + data.homework.open + data.homework.overdue) : null} detail={data ? `${data.homework.done} из ${data.homework.done + data.homework.open + data.homework.overdue}` : undefined} />
            <Meter label="Просрочено" tone="critical" value={data ? ratioOf(data.homework.overdue, data.homework.done + data.homework.open + data.homework.overdue) : null} detail={data ? `${data.homework.overdue} заданий` : undefined} />
            <Meter label="Посещаемость" tone={attendance != null && attendance < 0.85 ? 'warning' : 'info'} value={attendance} detail={data ? `Отмечено уроков: ${data.lessons_marked}` : undefined} />
          </div>
        </section>

        <section className="panel col-12">
          <div className="section-heading"><div><h2>Новости школы</h2><p className="muted">Объявления для вас и вашего класса</p></div><Link to="/announcements">Все объявления<ArrowRight /></Link></div>
          <QueryState isLoading={announcements.isLoading} error={announcements.error} retry={announcements.refetch} isEmpty={!announcements.data?.length} emptyTitle="Объявлений нет">
            <ul className="event-list">
              {announcements.data?.slice(0, 3).map((item) => (
                <li key={item.id}>
                  <span className="event-icon tone-amber"><Megaphone aria-hidden="true" /></span>
                  <div className="event-body"><strong>{item.title}</strong><span>{item.body}</span></div>
                  <StatusBadge>{formatDate(item.published_at, { day: 'numeric', month: 'short' })}</StatusBadge>
                </li>
              ))}
            </ul>
          </QueryState>
        </section>
      </div>
    </div>
  )
}

function ratioOf(part: number, whole: number): number | null {
  return whole > 0 ? part / whole : null
}
