import { BookOpen, CalendarDays, Check, FileSpreadsheet, GraduationCap, KeyRound, Link2, Repeat, School, Users } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { useAuth } from '../../../authContext'
import { setupSteps, type SimpleView } from './setup'
import { usePeriodEntries, useSchoolAnalytics, useSimpleData } from './data'
import { ClassDetail, ClassesView } from './ClassesView'
import { ScheduleView } from './ScheduleView'
import { ParentsView } from './ParentsView'
import { StudentsView } from './StudentsView'
import { TeachersView } from './TeachersView'
import { PasswordsView, SubjectsView } from './SubjectsView'

type Go = (view: SimpleView, classId?: string) => void

const tiles: { view: SimpleView; icon: LucideIcon; title: string; text: string }[] = [
  { view: 'students', icon: GraduationCap, title: 'Ученики и родители', text: 'Добавить ученика вместе с родителем, перевести в другой класс' },
  { view: 'parents', icon: Link2, title: 'Родители и дети', text: 'Связать родителя с ребёнком — без этого он не видит оценки' },
  { view: 'teachers', icon: Users, title: 'Учителя', text: 'Добавить учителя и указать, что он ведёт' },
  { view: 'classes', icon: School, title: 'Классы', text: 'Создать класс, назначить классного руководителя' },
  { view: 'schedule', icon: CalendarDays, title: 'Расписание', text: 'Поставить уроки в сетку недели' },
  { view: 'subjects', icon: BookOpen, title: 'Предметы и кабинеты', text: 'Список предметов, в том числе типовой' },
  { view: 'passwords', icon: KeyRound, title: 'Пароли и вход', text: 'Выдать новый пароль, если его забыли' },
]

/**
 * Сигналы сервера, которые в упрощённом режиме решаются на своих экранах (остальные — на главной панели). Тексты сервера
 * ссылаются на вкладки расширенного режима, поэтому здесь — свои подсказки.
 */
const alertViews: Record<string, { view: SimpleView; text: string }> = {
  unenrolled_students: { view: 'students', text: 'Без класса ученик не видит расписание и оценки. Нажмите «Другой класс» у ученика.' },
  teachers_without_assignments: { view: 'teachers', text: 'Учитель не увидит журнал, пока не указано, что он ведёт. Нажмите «Что ведёт».' },
  classes_without_class_teacher: { view: 'classes', text: 'Откройте класс и назначьте классного руководителя.' },
  parents_without_children: { view: 'parents', text: 'Пока родитель не связан с ребёнком, он не видит оценки. Дневник подскажет подходящих учеников по фамилии и отчеству.' },
  pending_password_change: { view: 'passwords', text: 'Эти люди ещё ни разу не вошли со своим паролем. Если временный пароль потерялся — выдайте новый.' },
}

function Home({ go, openAdvanced }: { go: Go; openAdvanced: (tab: string) => void }): React.JSX.Element {
  const { hasRole } = useAuth()
  const { classes, period, subjects, activeTeachers } = useSimpleData()
  const analytics = useSchoolAnalytics()
  const entries = usePeriodEntries(period?.id)
  const scheduled = new Set((entries.data ?? []).map((entry) => entry.class_id))
  const steps = setupSteps({
    hasCurrentPeriod: Boolean(period),
    classes: classes.length,
    subjects: subjects.data?.length ?? 0,
    teachers: activeTeachers.length,
    students: analytics.data?.counts.students ?? 0,
    classesWithoutSchedule: entries.isSuccess ? classes.filter((item) => !scheduled.has(item.id)) : [],
  })
  const done = steps.filter((step) => step.done).length
  const next = steps.find((step) => !step.done)
  const alerts = (analytics.data?.alerts ?? []).filter((alert) => alert.kind in alertViews)
  return (
    <>
      <section className="panel setup-panel">
        <div className="section-heading">
          <div><h2>{done === steps.length ? 'Школа готова к работе' : 'Подготовка школы'}</h2><p className="muted">Выполнено {done} из {steps.length}. Шаги идут по порядку — каждый следующий опирается на предыдущий.</p></div>
        </div>
        <div className="setup-progress" role="progressbar" aria-label="Подготовка школы" aria-valuemin={0} aria-valuemax={steps.length} aria-valuenow={done}><span style={{ width: `${(done / steps.length) * 100}%` }} /></div>
        <ol className="setup-steps">
          {steps.map((step, index) => (
            <li key={step.id} className={step.done ? 'done' : step === next ? 'next' : ''}>
              <span className="setup-mark" aria-hidden="true">{step.done ? <Check /> : index + 1}</span>
              <div><strong>{step.title}</strong><small>{step.text}</small></div>
              {!step.done && step.id !== 'period' && (
                <button type="button" className={`button small ${step === next ? 'primary' : 'secondary'}`} onClick={() => go(step.view, step.classId)}>{step.action}</button>
              )}
              {!step.done && step.id === 'period' && hasRole('system_admin') && (
                <button type="button" className="button small primary" onClick={() => openAdvanced('structure')}>{step.action}</button>
              )}
            </li>
          ))}
        </ol>
      </section>
      {alerts.length > 0 && (
        <section className="panel">
          <h2>Требует внимания</h2>
          <ul className="simple-list">
            {alerts.map((alert) => (
              <li key={alert.kind}>
                <div><strong>{alert.title}</strong><small>{alertViews[alert.kind]!.text}</small></div>
                <button type="button" className="button secondary small" onClick={() => go(alertViews[alert.kind]!.view)}>Исправить</button>
              </li>
            ))}
          </ul>
        </section>
      )}
      <section className="panel">
        <h2>Что вы хотите сделать?</h2>
        <nav className="simple-tiles" aria-label="Задачи администратора">
          {tiles.map(({ view, icon: Icon, title, text }) => (
            <button type="button" key={view} className="simple-tile" onClick={() => go(view)}>
              <span className="event-icon tone-blue"><Icon aria-hidden="true" /></span><strong>{title}</strong><span>{text}</span>
            </button>
          ))}
          <button type="button" className="simple-tile" onClick={() => openAdvanced('import')}>
            <span className="event-icon tone-green"><FileSpreadsheet aria-hidden="true" /></span><strong>Загрузить список из Excel</strong><span>Много учеников или учителей сразу — открывает расширенный режим</span>
          </button>
          <button type="button" className="simple-tile" onClick={() => go('schedule')}>
            <span className="event-icon tone-amber"><Repeat aria-hidden="true" /></span><strong>Новая четверть</strong><span>Откройте расписание новой четверти — дневник предложит перенести его из прошлой</span>
          </button>
        </nav>
      </section>
    </>
  )
}

export function SimpleAdmin(props: { view: SimpleView; classId?: string; go: Go; openAdvanced: (tab: string) => void }): React.JSX.Element {
  return <div className="simple-admin"><SimpleScreen {...props} /></div>
}

function SimpleScreen({ view, classId, go, openAdvanced }: { view: SimpleView; classId?: string; go: Go; openAdvanced: (tab: string) => void }): React.JSX.Element {
  const home = () => go('')
  switch (view) {
    case 'students': return <StudentsView onBack={home} />
    case 'parents': return <ParentsView onBack={home} />
    case 'teachers': return <TeachersView onBack={home} />
    case 'classes': return <ClassesView onBack={home} onOpen={(id) => go('class', id)} />
    case 'class': return classId ? <ClassDetail classId={classId} onBack={() => go('classes')} onSchedule={(id) => go('schedule', id)} /> : <ClassesView onBack={home} onOpen={(id) => go('class', id)} />
    case 'schedule': return <ScheduleView key={classId ?? ''} initialClassId={classId} onBack={home} />
    case 'subjects': return <SubjectsView onBack={home} />
    case 'passwords': return <PasswordsView onBack={home} />
    default: return <Home go={go} openAdvanced={openAdvanced} />
  }
}
