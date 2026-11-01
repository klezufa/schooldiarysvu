import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { Archive, CalendarCog, GraduationCap, KeyRound, LayoutGrid, Pencil, Plus, ScrollText, ShieldAlert, Upload, Users, Workflow } from 'lucide-react'
import { api, mutation, queryString } from '../api/client'
import type { AcademicPeriod, AcademicYear, AuditLog, GradeScale, NamedEntity, SchoolSettings } from '../api/types'
import { formatDate } from '../lib/dates'
import { AdminForm, SaveButton, type FormField, type FormValues } from '../components/AdminForms'
import { send } from '../lib/adminApi'
import { PageHeader, QueryState, StatusBadge } from '../components/UI'
import { valueText } from '../lib/format'
import { useToast } from '../components/toastContext'
import { InsecureConnectionNotice } from '../components/InsecureConnectionNotice'
import { PeopleAdmin } from './admin/People'
import { ImportAdmin } from './admin/Import'
import { InvitationsAdmin } from './admin/Invitations'
import { EnrollmentAdmin } from './admin/Enrollment'
import { ScheduleAdmin } from './admin/ScheduleAdmin'
import { TeachingAssignmentsPanel } from './admin/TeachingAssignments'
import { options, useDirectory, useIsSystemAdmin, usePeople } from './admin/helpers'
import { useAdminMode, type AdminMode } from './admin/mode'
import { SimpleAdmin } from './admin/simple/SimpleAdmin'
import type { SimpleView } from './admin/simple/setup'

type EntityRow = { id: string; [key: string]: unknown }

const tabs = [
  { id: 'people', label: 'Пользователи', icon: Users },
  { id: 'import', label: 'Импорт', icon: Upload },
  { id: 'invitations', label: 'Приглашения', icon: KeyRound },
  { id: 'enrollment', label: 'Зачисление', icon: GraduationCap },
  { id: 'assignments', label: 'Назначения', icon: Workflow },
  { id: 'schedule', label: 'Расписание', icon: CalendarCog },
  { id: 'structure', label: 'Структура', icon: LayoutGrid },
  { id: 'audit', label: 'Аудит', icon: ScrollText },
] as const
type Tab = (typeof tabs)[number]['id']

const fieldNames: Record<string, string> = { year: 'учебный год', short_name: 'кратко', starts_on: 'с', ends_on: 'по', status: 'статус', allow_late_edits: 'поздние правки' }

function EntitySection({ title, path, data, fields, initial, archive = false, children }: {
  title: string; path: string; data: EntityRow[]; fields: FormField[]; initial?: FormValues; archive?: boolean; children?: (row: EntityRow) => React.ReactNode
}): React.JSX.Element {
  const [editing, setEditing] = useState<EntityRow | null>(null)
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  // path — ключ кэша запроса без косой черты; адрес API начинается с неё.
  const url = `/${path}`
  const invalidate = () => queryClient.invalidateQueries({ queryKey: [path] })
  const submit = async (values: FormValues) => {
    const payload = { ...values }
    if (path === 'grade-scales') {
      payload.allowed_values = String(values.allowed_values).split(',').map((value) => Number(value.trim())).filter((value) => Number.isFinite(value))
      if (!editing) delete payload.is_active
    }
    if (editing && (path === 'classes' || path === 'academic-periods')) delete payload.academic_year_id
    await send(editing ? `${url}/${editing.id}` : url, editing ? 'PATCH' : 'POST', payload)
    await invalidate()
    setEditing(null)
    showToast('Данные сохранены')
  }
  return (
    <section className="panel admin-entity">
      <div className="section-heading"><h2>{title}</h2><StatusBadge>{data.length}</StatusBadge></div>
      <div className="entity-list">
        {data.map((row) => (
          <div className="entity-row" key={row.id}>
            <div>
              <strong>{valueText(row.name ?? row.display_name ?? row.id)}</strong>
              <small>{Object.entries(row).filter(([key]) => key in fieldNames).map(([key, value]) => `${fieldNames[key]}: ${valueText(value)}`).join(' · ')}</small>
            </div>
            <div className="table-actions">
              <button className="icon-button" aria-label={`Изменить ${valueText(row.name ?? row.id)}`} onClick={() => setEditing(row)}><Pencil /></button>
              {children?.(row)}
              {archive && (
                <SaveButton danger onClick={async () => {
                  if (!window.confirm('Архивировать запись? Исторические учебные данные сохранятся.')) return
                  await send(`${url}/${row.id}`, 'DELETE')
                  await invalidate()
                  showToast('Запись архивирована')
                }}><Archive />Архивировать</SaveButton>
              )}
            </div>
          </div>
        ))}
      </div>
      <details className="admin-create" open={Boolean(editing)}>
        <summary><Plus />{editing ? 'Редактирование' : 'Добавить запись'}</summary>
        <AdminForm
          key={editing?.id ?? 'new'}
          fields={fields}
          initial={editing ? Object.fromEntries(fields.map((field) => [field.name, rowValue(editing, field.name)])) : initial}
          submitLabel={editing ? 'Сохранить изменения' : 'Создать'}
          onCancel={editing ? () => setEditing(null) : undefined}
          onSubmit={submit}
        />
      </details>
    </section>
  )
}

function rowValue(row: EntityRow, key: string): string | number | boolean {
  const value = row[key]
  if (Array.isArray(value)) return value.join(',')
  return typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean' ? value : ''
}

function StructureAdmin(): React.JSX.Element {
  const system = useIsSystemAdmin()
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const years = useQuery({ queryKey: ['academic-years'], queryFn: () => api<AcademicYear[]>('/academic-years') })
  const currentYear = years.data?.find((year) => year.is_current) ?? years.data?.[0]
  // Периоды и классы всех неархивных лет с подписью года: следующий год готовят заранее, до смены текущего.
  const periods = useQuery({ queryKey: ['academic-periods', 'all'], queryFn: () => api<AcademicPeriod[]>('/academic-periods') })
  const classes = useQuery({ queryKey: ['classes'], queryFn: () => api<NamedEntity[]>('/classes') })
  const yearName = (id: unknown) => years.data?.find((year) => year.id === id)?.name ?? ''
  const yearStart = (id: unknown) => years.data?.find((year) => year.id === id)?.starts_on ?? ''
  const classRows = [...(classes.data ?? [])]
    .sort((a, b) => yearStart(b.academic_year_id).localeCompare(yearStart(a.academic_year_id)) || a.name.localeCompare(b.name, 'ru'))
    .map((item) => ({ ...item, year: yearName(item.academic_year_id) }))
  const periodRows = (periods.data ?? []).map((item) => ({ ...item, year: yearName(item.academic_year_id) }))
  const subjects = useQuery({ queryKey: ['subjects'], queryFn: () => api<NamedEntity[]>('/subjects') })
  const rooms = useQuery({ queryKey: ['rooms'], queryFn: () => api<NamedEntity[]>('/rooms') })
  const scales = useQuery({ queryKey: ['grade-scales'], queryFn: () => api<GradeScale[]>('/grade-scales'), enabled: system })
  const settings = useQuery({ queryKey: ['school-settings'], queryFn: () => api<SchoolSettings>('/school/settings'), enabled: system })
  const namedFields: FormField[] = [{ name: 'name', label: 'Название', required: true }, { name: 'short_name', label: 'Короткое название' }]
  const refreshPeriods = () => queryClient.invalidateQueries({ queryKey: ['academic-periods'] })
  return (
    <QueryState isLoading={years.isLoading} error={years.error} retry={years.refetch}>
      <div className="admin-structure">
        <EntitySection title="Классы" path="classes" data={classRows as EntityRow[]} archive initial={{ academic_year_id: currentYear?.id }}
          fields={[{ name: 'academic_year_id', label: 'Учебный год', type: 'select', required: true, options: options(years.data) }, { name: 'name', label: 'Название класса', required: true, hint: 'Например, 5А' }]} />
        <EntitySection title="Предметы" path="subjects" data={(subjects.data ?? []) as EntityRow[]} fields={namedFields} archive />
        <EntitySection title="Кабинеты" path="rooms" data={(rooms.data ?? []) as EntityRow[]} fields={namedFields} archive />
        {system ? (
          <>
            <EntitySection title="Учебные годы" path="academic-years" data={(years.data ?? []) as EntityRow[]} archive initial={{ is_current: false }}
              fields={[{ name: 'name', label: 'Название', required: true, hint: 'Например, 2026/2027' }, { name: 'starts_on', label: 'Начало', type: 'date', required: true }, { name: 'ends_on', label: 'Окончание', type: 'date', required: true }, { name: 'is_current', label: 'Текущий учебный год', type: 'checkbox' }]} />
            <EntitySection title="Учебные периоды" path="academic-periods" data={periodRows as EntityRow[]} initial={{ academic_year_id: currentYear?.id }}
              fields={[{ name: 'academic_year_id', label: 'Учебный год', type: 'select', required: true, options: options(years.data) }, { name: 'name', label: 'Название', required: true, hint: 'Например, II четверть' }, { name: 'starts_on', label: 'Начало', type: 'date', required: true }, { name: 'ends_on', label: 'Окончание', type: 'date', required: true }]}>
              {(period) => (
                <>
                  {period.status !== 'closed' && (
                    <SaveButton onClick={async () => {
                      if (!window.confirm('Закрыть период? Обычное редактирование журнала будет запрещено.')) return
                      await send(`/academic-periods/${period.id}/close`, 'POST'); await refreshPeriods(); showToast('Период закрыт')
                    }}>Закрыть период</SaveButton>
                  )}
                  <SaveButton onClick={async () => {
                    await send(`/academic-periods/${period.id}/late-edits`, 'PUT', { allowed: !period.allow_late_edits }); await refreshPeriods(); showToast('Разрешение поздних правок изменено')
                  }}>{period.allow_late_edits ? 'Запретить поздние правки' : 'Разрешить поздние правки'}</SaveButton>
                </>
              )}
            </EntitySection>
            <EntitySection title="Шкалы оценок" path="grade-scales" data={(scales.data ?? []) as EntityRow[]} initial={{ is_active: true, allowed_values: '2,3,4,5' }}
              fields={[{ name: 'name', label: 'Название', required: true }, { name: 'allowed_values', label: 'Значения через запятую', required: true }, { name: 'is_active', label: 'Активна', type: 'checkbox' }]} />
            <section className="panel">
              <h2>Параметры школы</h2>
              <QueryState isLoading={settings.isLoading} error={settings.error} retry={settings.refetch}>
                {settings.data && (
                  <AdminForm fields={[{ name: 'school_name', label: 'Название школы', required: true }, { name: 'timezone', label: 'Часовой пояс IANA', required: true, hint: 'Например, Europe/Moscow' }]}
                    initial={settings.data}
                    onSubmit={async (values) => { await mutation('/school/settings', 'PUT', values); await queryClient.invalidateQueries({ queryKey: ['school-settings'] }); showToast('Параметры школы сохранены') }} />
                )}
              </QueryState>
            </section>
          </>
        ) : (
          <section className="panel">
            <h2>Учебные годы, периоды и шкалы</h2>
            <div className="notice info"><ShieldAlert aria-hidden="true" /><span>Эти настройки влияют на весь журнал школы (закрытие периодов, шкала оценок), поэтому их меняет системный администратор.</span></div>
            <ul className="compact-list">{years.data?.map((year) => <li key={year.id}><strong>{year.name}{year.is_current ? ' · текущий' : ''}</strong><span>{year.starts_on} — {year.ends_on}</span></li>)}</ul>
          </section>
        )}
      </div>
    </QueryState>
  )
}

function AssignmentsAdmin(): React.JSX.Element {
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const teachers = usePeople('teacher')
  const students = usePeople('student')
  const parents = usePeople('parent')
  const { classes, classLabel, classOptions, openClasses } = useDirectory()
  const classTeachers = useQuery({ queryKey: ['class-teachers'], queryFn: () => api<EntityRow[]>('/class-teachers') })
  const parentLinks = useQuery({ queryKey: ['parent-student'], queryFn: () => api<EntityRow[]>('/parent-student') })
  const teacherOptions = teachers.data?.filter((user) => user.teacher_id).map((user) => ({ value: user.teacher_id!, label: user.display_name })) ?? []
  const studentOptions = students.data?.filter((user) => user.student_id).map((user) => ({ value: user.student_id!, label: `${user.display_name}${user.class_name ? `, ${user.class_name}` : ''}` })) ?? []
  const parentOptions = parents.data?.filter((user) => user.parent_id).map((user) => ({ value: user.parent_id!, label: user.display_name })) ?? []
  const label = (list: { value: string; label: string }[], id: unknown) => list.find((item) => item.value === id)?.label ?? valueText(id)
  const className = (id: unknown) => { const found = classes.data?.find((item) => item.id === id); return found ? classLabel(found) : valueText(id) }
  const isoDate = (value: unknown) => valueText(value).slice(0, 10)
  const saved = async (key: string) => { await queryClient.invalidateQueries({ queryKey: [key] }); showToast('Назначение сохранено') }
  return (
    <div className="admin-structure">
      <TeachingAssignmentsPanel />
      <section className="panel">
        <h2>Классный руководитель</h2>
        <AdminForm fields={[{ name: 'teacher_id', label: 'Учитель', type: 'select', required: true, options: teacherOptions }, { name: 'class_id', label: 'Класс', type: 'select', required: true, options: classOptions(openClasses) }, { name: 'starts_on', label: 'Начало', type: 'date', required: true }]}
          onSubmit={async (values) => { await send('/class-teachers', 'POST', values); await saved('class-teachers') }} />
        <div className="entity-list">
          {classTeachers.data?.map((item) => (
            <div className="entity-row" key={item.id}>
              <div><strong>{label(teacherOptions, item.teacher_id)}</strong><small>{className(item.class_id)} · с {isoDate(item.starts_on)}{item.ends_on ? ` по ${isoDate(item.ends_on)}` : ' · действует'}</small></div>
              {!item.ends_on && <AdminForm fields={[{ name: 'ends_on', label: 'Окончание', type: 'date', required: true }]} submitLabel="Завершить" onSubmit={async (values) => { await send(`/class-teachers/${item.id}`, 'PATCH', values); await saved('class-teachers') }} />}
            </div>
          ))}
        </div>
      </section>
      <section className="panel span-2">
        <h2>Родитель и ребёнок</h2>
        <AdminForm fields={[{ name: 'parent_id', label: 'Родитель', type: 'select', required: true, options: parentOptions }, { name: 'student_id', label: 'Ребёнок', type: 'select', required: true, options: studentOptions }, { name: 'valid_from', label: 'Действует с', type: 'date', required: true }, { name: 'valid_to', label: 'Действует до', type: 'date' }]}
          onSubmit={async (values) => { await send('/parent-student', 'POST', values); await saved('parent-student') }} />
        <div className="entity-list">
          {parentLinks.data?.map((item, index) => (
            <div className="entity-row" key={`${item.parent_id}-${item.student_id}-${index}`}>
              <div><strong>{label(parentOptions, item.parent_id)}</strong><small>ребёнок: {label(studentOptions, item.student_id)} · с {isoDate(item.valid_from)}{item.valid_to ? ` по ${isoDate(item.valid_to)}` : ' · действует'}</small></div>
              {!item.valid_to && <AdminForm fields={[{ name: 'valid_to', label: 'Действует до', type: 'date', required: true }]} submitLabel="Завершить связь"
                onSubmit={async (values) => { await send('/parent-student', 'PUT', { parent_id: String(item.parent_id), student_id: String(item.student_id), valid_from: isoDate(item.valid_from), valid_to: values.valid_to }); await saved('parent-student') }} />}
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}

const auditActions = ['', 'user.created', 'user.updated', 'user.deactivated', 'user.password_reset', 'user.registered', 'invitation.created', 'enrollment.created', 'grade.created', 'grade.updated', 'grade.deleted', 'lessons.generated', 'schedule_entry.updated', 'password.changed']

function AuditAdmin(): React.JSX.Element {
  const [filters, setFilters] = useState({ entity_type: '', action: '', from: '', to: '' })
  const [offset, setOffset] = useState(0)
  const params = useMemo(() => ({ ...filters, from: filters.from ? new Date(filters.from).toISOString() : undefined, to: filters.to ? new Date(filters.to).toISOString() : undefined, limit: 50, offset }), [filters, offset])
  const audit = useQuery({ queryKey: ['audit', params], queryFn: () => api<AuditLog[]>(`/audit${queryString(params)}`) })
  const update = (patch: Partial<typeof filters>) => { setOffset(0); setFilters({ ...filters, ...patch }) }
  return (
    <section className="panel">
      <div className="filter-bar">
        <label>Действие<select value={filters.action} onChange={(event) => update({ action: event.target.value })}>{auditActions.map((action) => <option key={action} value={action}>{action || 'Все действия'}</option>)}</select></label>
        <label>Тип объекта<input value={filters.entity_type} placeholder="user, grade, lesson…" onChange={(event) => update({ entity_type: event.target.value })} /></label>
        <label>С<input type="datetime-local" value={filters.from} onChange={(event) => update({ from: event.target.value })} /></label>
        <label>По<input type="datetime-local" value={filters.to} onChange={(event) => update({ to: event.target.value })} /></label>
      </div>
      <QueryState isLoading={audit.isLoading} error={audit.error} retry={audit.refetch} isEmpty={!audit.data?.length} emptyTitle="Записи не найдены">
        <div className="audit-list">
          {audit.data?.map((item) => (
            <article key={item.id}>
              <div><strong>{item.action}</strong><StatusBadge>{item.entity_type}</StatusBadge><time>{formatDate(item.created_at, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit', second: '2-digit' })}</time></div>
              <small>Объект: {item.entity_id} · запрос: {item.request_id}</small>
              <details><summary>Старые и новые значения</summary><pre>{JSON.stringify({ old: item.old_values, new: item.new_values }, null, 2)}</pre></details>
            </article>
          ))}
        </div>
        <div className="pager">
          <span>Записи {offset + 1}–{offset + (audit.data?.length ?? 0)}</span>
          <div className="card-actions">
            <button className="button secondary small" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 50))}>Назад</button>
            <button className="button secondary small" disabled={(audit.data?.length ?? 0) < 50} onClick={() => setOffset(offset + 50)}>Далее</button>
          </div>
        </div>
      </QueryState>
    </section>
  )
}

function AdvancedAdmin({ tab, select, startWithCreate }: { tab: Tab; select: (tab: Tab) => void; startWithCreate: boolean }): React.JSX.Element {
  return (
    <>
      <div className="tabs admin-tabs" role="tablist" aria-label="Разделы администрирования">
        {tabs.map(({ id, label, icon: Icon }) => (
          <button role="tab" id={`tab-${id}`} aria-selected={tab === id} aria-controls="admin-panel" className={tab === id ? 'active' : ''} key={id} onClick={() => select(id)}>
            <Icon aria-hidden="true" />{label}
          </button>
        ))}
      </div>
      <div role="tabpanel" id="admin-panel" aria-labelledby={`tab-${tab}`}>
        {tab === 'people' && <PeopleAdmin startWithCreate={startWithCreate} />}
        {tab === 'import' && <ImportAdmin />}
        {tab === 'invitations' && <InvitationsAdmin />}
        {tab === 'enrollment' && <EnrollmentAdmin />}
        {tab === 'assignments' && <AssignmentsAdmin />}
        {tab === 'schedule' && <ScheduleAdmin />}
        {tab === 'structure' && <StructureAdmin />}
        {tab === 'audit' && <AuditAdmin />}
      </div>
    </>
  )
}

const simpleViews: SimpleView[] = ['students', 'teachers', 'classes', 'class', 'schedule', 'passwords', 'subjects']

export function AdminPage(): React.JSX.Element {
  const [params, setParams] = useSearchParams()
  const [mode, setMode] = useAdminMode()
  const requested = params.get('tab')
  // Ссылка на конкретную вкладку (с главной панели, из подсказок) открывает расширенный режим, не меняя выбор.
  const advanced = mode === 'advanced' || Boolean(requested)
  const tab: Tab = tabs.some((item) => item.id === requested) ? (requested as Tab) : 'people'
  const select = (next: Tab) => setParams({ tab: next }, { replace: true })
  const requestedView = params.get('view') as SimpleView | null
  const view: SimpleView = requestedView && simpleViews.includes(requestedView) ? requestedView : ''
  const go = (next: SimpleView, classId?: string) => {
    setParams(next ? { view: next, ...(classId ? { class: classId } : {}) } : {})
    window.scrollTo({ top: 0 })
  }
  const switchMode = (next: AdminMode) => { setMode(next); setParams(next === 'advanced' ? { tab } : {}, { replace: true }) }
  return (
    <div>
      <PageHeader eyebrow="Управление школой" title="Администрирование"
        description={advanced ? 'Пользователи и роли, зачисление, назначения, расписание и учебная структура.' : 'Частые задачи — по шагам, без лишних настроек.'}
        actions={
          <div className="segmented mode-switch" role="group" aria-label="Режим администрирования">
            <button type="button" aria-pressed={!advanced} className={advanced ? '' : 'active'} onClick={() => switchMode('simple')}>Простой режим</button>
            <button type="button" aria-pressed={advanced} className={advanced ? 'active' : ''} onClick={() => switchMode('advanced')}>Расширенный режим</button>
          </div>
        } />
      <InsecureConnectionNotice />
      {advanced
        ? <AdvancedAdmin tab={tab} select={select} startWithCreate={params.get('new') === '1'} />
        : <SimpleAdmin view={view} classId={params.get('class') ?? undefined} go={go} openAdvanced={(next) => setParams({ tab: next })} />}
    </div>
  )
}
