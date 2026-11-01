import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import {
  AlertOctagon, AlertTriangle, ArrowRight, CalendarCog, GraduationCap, Info, KeyRound, LayoutGrid, School, ScrollText,
  Upload, UserPlus, Users, Workflow,
} from 'lucide-react'
import { api, queryString } from '../../api/client'
import type { SchoolAnalytics, SchoolSettings } from '../../api/types'
import { isAdministrator, roleNames } from '../../api/types'
import { useAuth } from '../../authContext'
import { formatDate } from '../../lib/dates'
import { BarList, ChartCard, ColumnChart, LineChart, SegmentedBar, StatTile } from '../../components/charts'
import { formatNumber, formatPercent, gradeTone, seriesColors, toneColor } from '../../lib/chartMath'
import { QueryState } from '../../components/UI'
import { InsecureConnectionNotice } from '../../components/InsecureConnectionNotice'
import { HeroBanner } from './HeroBanner'
import { EmptyPeriodHint, PeriodSelect } from './PeriodControls'
import { useDashboardPeriod } from './usePeriods'
import { distributionParts, firstName, greeting } from './helpers'

const actions = [
  { to: '/admin?tab=people', icon: UserPlus, title: 'Пользователи и роли', text: 'Добавить, изменить роли, сбросить пароль, деактивировать' },
  { to: '/admin?tab=import', icon: Upload, title: 'Импорт списком', text: 'Создать класс учеников или учителей из таблицы за один шаг' },
  { to: '/admin?tab=invitations', icon: KeyRound, title: 'Приглашения', text: 'Одноразовые коды для самостоятельной регистрации' },
  { to: '/admin?tab=enrollment', icon: GraduationCap, title: 'Зачисление и перевод', text: 'Массовое зачисление, перевод класса на новый год' },
  { to: '/admin?tab=assignments', icon: Workflow, title: 'Назначения', text: 'Учителя на классы и предметы, классные руководители, родители' },
  { to: '/admin?tab=schedule', icon: CalendarCog, title: 'Расписание', text: 'Недельный шаблон и создание уроков на период' },
  { to: '/admin?tab=structure', icon: LayoutGrid, title: 'Учебная структура', text: 'Классы, предметы, кабинеты, учебные годы' },
  { to: '/admin?tab=audit', icon: ScrollText, title: 'Журнал действий', text: 'Кто, что и когда изменил' },
]

const actionNames: Record<string, string> = {
  'user.created': 'Создан пользователь', 'user.updated': 'Изменён пользователь', 'user.deactivated': 'Деактивирован пользователь',
  'user.reactivated': 'Пользователь возвращён', 'user.password_reset': 'Сброшен пароль', 'user.sessions_revoked': 'Завершены сеансы',
  'user.registered': 'Регистрация по приглашению', 'password.changed': 'Сменён пароль',
  'invitation.created': 'Создано приглашение', 'invitation.revoked': 'Отозвано приглашение',
  'enrollment.created': 'Зачисление', 'enrollment.updated': 'Изменено зачисление', 'enrollment.ended': 'Окончание зачисления',
  'teaching_assignment.created': 'Назначен учитель', 'teaching_assignment.updated': 'Изменено назначение учителя',
  'class_teacher.assigned': 'Назначен классный руководитель', 'class_teacher.updated': 'Изменён классный руководитель',
  'parent_student.linked': 'Родитель связан с ребёнком', 'parent_student.updated': 'Изменена связь родителя',
  'lessons.generated': 'Созданы уроки по расписанию', 'lesson.created': 'Создан урок', 'lesson.updated': 'Изменён урок',
  'lesson_work.created': 'Добавлена работа', 'grade.created': 'Выставлена оценка', 'grade.updated': 'Исправлена оценка',
  'grade.deleted': 'Удалена оценка', 'term_grade.upserted': 'Итоговая отметка', 'attendance.upserted': 'Отмечена посещаемость',
  'attendance.marked_all_present': 'Все присутствуют', 'homework.created': 'Создано задание', 'homework.updated': 'Изменено задание',
  'homework.published': 'Опубликовано задание', 'homework.copied': 'Скопировано задание', 'homework.completion_changed': 'Отметка о выполнении',
  'file.uploaded': 'Загружен файл', 'file.archived': 'Удалён файл', 'announcement.created': 'Новое объявление', 'announcement.archived': 'Снято объявление',
  'schedule_entry.created': 'Строка расписания', 'schedule_entry.updated': 'Изменено расписание', 'schedule_entry.deleted': 'Удалена строка расписания',
  'academic_year.created': 'Создан учебный год', 'academic_year.updated': 'Изменён учебный год', 'academic_year.archived': 'Учебный год в архиве',
  'academic_period.created': 'Создан учебный период', 'academic_period.updated': 'Изменён учебный период', 'academic_period.closed': 'Период закрыт',
  'academic_period.late_edits': 'Поздние правки периода', 'class.created': 'Создан класс', 'class.updated': 'Изменён класс',
  'subject.created': 'Создан предмет', 'subject.updated': 'Изменён предмет', 'room.created': 'Создан кабинет', 'room.updated': 'Изменён кабинет',
  'grade_scale.created': 'Создана шкала оценок', 'grade_scale.updated': 'Изменена шкала оценок', 'school_settings.updated': 'Изменены параметры школы',
}

export function SchoolDashboard(): React.JSX.Element {
  const { user } = useAuth()
  const admin = isAdministrator(user?.roles ?? [])
  const { periodId, setPeriodId, periods, previous } = useDashboardPeriod()
  const analytics = useQuery({ queryKey: ['analytics-school', periodId], queryFn: () => api<SchoolAnalytics>(`/analytics/school${queryString({ academic_period_id: periodId })}`), refetchInterval: 120_000 })
  const settings = useQuery({ queryKey: ['school-settings'], queryFn: () => api<SchoolSettings>('/school/settings') })
  const data = analytics.data
  const counts = data?.counts
  const attendanceColumns = (data?.attendance_today ?? []).filter((item) => item.rate != null).map((item) => ({
    key: item.class_id, label: item.class_name, value: item.rate!,
    color: item.rate! >= 0.9 ? 'var(--series-1)' : item.rate! >= 0.8 ? 'var(--tone-warning)' : 'var(--tone-critical)',
    hint: `отмечено присутствий: ${item.present}, отсутствий: ${item.absent}`,
  }))
  return (
    <div className="dashboard">
      <HeroBanner
        title={admin ? 'Панель администратора' : `${greeting()}, ${firstName(user?.display_name)}!`}
        text={`${settings.data?.school_name ?? 'Школа'} · ${data?.period_name ?? 'учебный период не выбран'}`}
        meta={<PeriodSelect value={periodId} onChange={setPeriodId} periods={periods} />}
        art={false}
      />
      {!periodId && data && !data.distribution.length && <EmptyPeriodHint previous={previous} onShow={setPeriodId} />}
      {admin && <InsecureConnectionNotice />}
      <div className="stat-grid">
        <StatTile icon={<GraduationCap />} tone="blue" label="Учеников" value={counts?.students ?? '…'} detail={counts ? `родителей: ${counts.parents}` : ''} />
        <StatTile icon={<Users />} tone="green" label="Учителей" value={counts?.teachers ?? '…'} detail={counts ? `предметов: ${counts.subjects}` : ''} />
        <StatTile icon={<School />} tone="violet" label="Классов" value={counts?.classes ?? '…'} detail={counts ? `активных сеансов: ${counts.active_sessions}` : ''} />
        <StatTile icon={<KeyRound />} tone="amber" label="Пользователей" value={counts?.users_active ?? '…'} detail={counts ? `деактивировано: ${counts.users_inactive}, с временным паролем: ${counts.pending_password_change}` : ''} />
      </div>
      <div className="dash-grid">
        {admin && (
          <section className="panel col-12">
            <div className="section-heading"><div><h2>Панель управления</h2><p className="muted">Частые задачи администратора</p></div></div>
            <nav className="quick-actions" aria-label="Быстрые действия">
              {actions.map(({ to, icon: Icon, title, text }) => (
                <Link key={to} to={to} className="quick-action"><span className="event-icon tone-blue"><Icon aria-hidden="true" /></span><strong>{title}</strong><span>{text}</span></Link>
              ))}
            </nav>
          </section>
        )}
        <div className="col-8">
          <ChartCard
            title="Посещаемость по классам сегодня"
            subtitle="Доля отметок «присутствовал» и «опоздал» среди отмеченных"
            table={{ columns: ['Класс', 'Учеников', 'Присутствия', 'Отсутствия', 'Доля'], rows: (data?.attendance_today ?? []).map((item) => [item.class_name, item.students, item.present, item.absent, formatPercent(item.rate)]) }}
          >
            <QueryState isLoading={analytics.isLoading} error={analytics.error} retry={analytics.refetch}>
              <ColumnChart data={attendanceColumns} max={1} format={(value) => formatPercent(value)} ariaLabel="Посещаемость по классам" emptyText="Сегодня посещаемость ещё не отмечали" />
            </QueryState>
          </ChartCard>
        </div>
        <section className="panel col-4">
          <div className="section-heading"><div><h2>Требует внимания</h2><p className="muted">Проверки данных школы</p></div></div>
          <QueryState isLoading={analytics.isLoading} error={analytics.error} retry={analytics.refetch} isEmpty={!data?.alerts.length} emptyTitle="Всё в порядке" emptyText="Проблем в данных не найдено.">
            <ul className="alert-list">
              {data?.alerts.map((alert) => {
                const Icon = alert.severity === 'danger' ? AlertOctagon : alert.severity === 'warning' ? AlertTriangle : Info
                return (
                  <li key={alert.kind} className={`alert-item alert-${alert.severity}`}>
                    <Icon aria-hidden="true" />
                    <div><strong>{alert.title}</strong><span>{alert.text}</span></div>
                    {alert.link && admin && <Link to={alert.link}>Открыть</Link>}
                  </li>
                )
              })}
            </ul>
          </QueryState>
        </section>
        <div className="col-4">
          <ChartCard title="Средний балл по классам" subtitle={data?.period_name ?? ''} table={{ columns: ['Класс', 'Средний балл', 'Оценок'], rows: (data?.grade_by_class ?? []).map((item) => [item.name, formatNumber(item.average), item.grades_count]) }}>
            <BarList data={(data?.grade_by_class ?? []).map((item) => ({ key: item.id, label: item.name, value: item.average, color: toneColor(gradeTone(item.average)), hint: `${item.grades_count} оценок` }))} max={5} ariaLabel="Средний балл по классам" />
          </ChartCard>
        </div>
        <div className="col-4">
          <ChartCard title="Средний балл по предметам" subtitle={data?.period_name ?? ''} table={{ columns: ['Предмет', 'Средний балл', 'Оценок'], rows: (data?.grade_by_subject ?? []).map((item) => [item.name, formatNumber(item.average), item.grades_count]) }}>
            <BarList data={(data?.grade_by_subject ?? []).map((item) => ({ key: item.id, label: item.name, value: item.average, color: toneColor(gradeTone(item.average)) }))} max={5} ariaLabel="Средний балл по предметам" />
          </ChartCard>
        </div>
        <div className="col-4">
          <ChartCard title="Пользователи по ролям" subtitle="Активные учётные записи" table={{ columns: ['Роль', 'Пользователей'], rows: (data?.users_by_role ?? []).map((item) => [roleNames[item.role], item.count]) }}>
            <BarList data={(data?.users_by_role ?? []).map((item, index) => ({ key: item.role, label: roleNames[item.role], value: item.count, color: seriesColors[index % seriesColors.length] }))} format={(value) => formatNumber(value, 0)} ariaLabel="Пользователи по ролям" />
          </ChartCard>
        </div>
        <div className="col-7">
          <ChartCard title="Посещаемость за две недели" subtitle="Доля присутствий по дням" table={{ columns: ['День', 'Посещаемость'], rows: (data?.attendance_trend ?? []).map((item) => [formatDate(item.day, { day: 'numeric', month: 'short' }), formatPercent(item.rate, 1)]) }}>
            <LineChart points={(data?.attendance_trend ?? []).map((item) => ({ key: item.day, label: formatDate(item.day, { day: 'numeric', month: 'short' }), value: item.rate }))} domain={[0, 1]} format={(value) => formatPercent(value, 1)} ariaLabel="Посещаемость по дням" color="var(--series-3)" />
          </ChartCard>
        </div>
        <div className="col-5">
          <ChartCard title="Распределение оценок" subtitle="Все оценки школы за период" table={{ columns: ['Оценка', 'Количество'], rows: (data?.distribution ?? []).map((item) => [item.value, item.count]) }}>
            <SegmentedBar parts={distributionParts(data?.distribution ?? [])} ariaLabel="Распределение оценок по школе" />
          </ChartCard>
        </div>
        <section className="panel col-12">
          <div className="section-heading"><div><h2>Последние действия</h2><p className="muted">Из журнала аудита</p></div>{admin && <Link to="/admin?tab=audit">Весь журнал<ArrowRight /></Link>}</div>
          <QueryState isLoading={analytics.isLoading} error={analytics.error} retry={analytics.refetch} isEmpty={!data?.recent_activity.length} emptyTitle="Действий пока нет">
            <ul className="activity-list">
              {data?.recent_activity.map((item) => (
                <li key={item.id}>
                  <span className={`kind-pill kind-${item.entity_type}`}>{item.entity_type.replace('_', ' ')}</span>
                  <span><strong>{actionNames[item.action] ?? item.action}</strong>{item.actor_name ? ` · ${item.actor_name}` : ''}</span>
                  <time>{formatDate(item.created_at, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })}</time>
                </li>
              ))}
            </ul>
          </QueryState>
        </section>
      </div>
    </div>
  )
}
