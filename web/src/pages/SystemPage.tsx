import { useQuery } from '@tanstack/react-query'
import { Activity, AlertOctagon, AlertTriangle, CheckCircle2, Cpu, Database, Gauge, HardDrive, RefreshCw, Server, ShieldCheck, Timer, TriangleAlert } from 'lucide-react'
import { api } from '../api/client'
import type { ServiceStatus, SystemOverview } from '../api/types'
import { operationChecks } from '../lib/operations'
import { formatDate } from '../lib/dates'
import { BarList, ChartCard, DataTable, LineChart, Meter, StatTile } from '../components/charts'
import { formatBytes, formatNumber, formatPercent, type Tone } from '../lib/chartMath'
import { PageHeader, QueryState } from '../components/UI'

const serviceNames: Record<string, string> = { server: 'Веб-сервер и API', worker: 'Фоновые задачи' }
const statusNames: Record<ServiceStatus['status'], string> = { up: 'Работает', stale: 'Отвечает с задержкой', down: 'Не отвечает' }

function duration(seconds: number): string {
  const days = Math.floor(seconds / 86_400)
  const hours = Math.floor((seconds % 86_400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days) return `${days} д ${hours} ч`
  if (hours) return `${hours} ч ${minutes} мин`
  return `${minutes} мин`
}

function loadTone(share: number | null | undefined): Tone {
  if (share == null) return 'neutral'
  if (share >= 0.9) return 'critical'
  if (share >= 0.75) return 'warning'
  return 'good'
}

function stat(stats: Record<string, unknown>, key: string): number | undefined {
  const value = stats[key]
  return typeof value === 'number' ? value : undefined
}

function time(value: string): string {
  return formatDate(value, { hour: '2-digit', minute: '2-digit' })
}

export function SystemPage(): React.JSX.Element {
  const overview = useQuery({ queryKey: ['system-overview'], queryFn: () => api<SystemOverview>('/system/overview'), refetchInterval: 15_000 })
  const data = overview.data
  const host = data?.host
  const memoryShare = host?.memory_total ? (host.memory_used ?? 0) / host.memory_total : null
  const diskShare = host?.disk_total ? (host.disk_used ?? 0) / host.disk_total : null
  const db = data?.database
  const up = data?.services.filter((service) => service.status === 'up').length ?? 0
  const series = data?.series ?? []
  return (
    <div>
      <PageHeader
        eyebrow="Системный администратор"
        title="Состояние системы"
        description="Нагрузка на сервер, сервисы и базу данных. Данные обновляются каждые 15 секунд."
        actions={<button className="button secondary" onClick={() => overview.refetch()} disabled={overview.isFetching}><RefreshCw aria-hidden="true" />Обновить</button>}
      />
      <QueryState isLoading={overview.isLoading} error={overview.error} retry={overview.refetch}>
        {data && (
          <div className={overview.isFetching ? 'refreshing' : ''}>
            <div className="stat-grid">
              <StatTile icon={<Server />} tone={up === data.services.length ? 'green' : 'rose'} label="Сервисы" value={`${up} из ${data.services.length}`} detail={`версия ${data.version} · ${data.environment}`} />
              <StatTile icon={<Activity />} tone="blue" label="Запросов в минуту" value={formatNumber(data.http.requests_last_minute, 0)} detail={`за час: ${formatNumber(data.http.requests_last_hour, 0)}, сейчас в работе: ${data.http.in_flight}`} />
              <StatTile icon={<Timer />} tone={data.http.p95_ms > 1000 ? 'amber' : 'teal'} label="Время ответа (95%)" value={`${formatNumber(data.http.p95_ms, 0)} мс`} detail={`медиана ${formatNumber(data.http.p50_ms, 0)} мс, за 5 минут`} />
              <StatTile icon={<TriangleAlert />} tone={data.http.error_rate > 0.01 ? 'rose' : 'green'} label="Ошибки сервера" value={formatPercent(data.http.error_rate, 1)} detail="доля ответов 5xx за 5 минут" />
            </div>
            <div className="dash-grid">
              <section className="panel col-12">
                <div className="section-heading"><div><h2>Эксплуатация</h2><p className="muted">Резервные копии, сертификат, сетевой профиль и место на диске</p></div><ShieldCheck aria-hidden="true" /></div>
                <ul className="alert-list ops-list">
                  {operationChecks(data.operations, new Date(), { total: host?.disk_total, used: host?.disk_used }).map((check) => {
                    const Icon = check.tone === 'danger' ? AlertOctagon : check.tone === 'warning' ? AlertTriangle : CheckCircle2
                    return <li key={check.key} className={`alert-item alert-${check.tone}`}><Icon aria-hidden="true" /><div><strong>{check.title}</strong><span>{check.text}</span></div></li>
                  })}
                </ul>
                {data.operations.last_restore_at && <p className="muted chart-note">Последнее восстановление из копии: {formatDate(data.operations.last_restore_at, { day: 'numeric', month: 'long', year: 'numeric', hour: '2-digit', minute: '2-digit' })}</p>}
              </section>
              <section className="panel col-8">
                <div className="section-heading"><div><h2>Сервисы</h2><p className="muted">По сигналам, которые процессы пишут в базу каждые 15 секунд</p></div></div>
                <div className="service-grid">
                  {data.services.map((service) => (
                    <article className="service-card" key={`${service.service}-${service.instance}`}>
                      <header><strong>{serviceNames[service.service] ?? service.service}</strong><span className={`status-dot status-${service.status}`}>{statusNames[service.status]}</span></header>
                      <small>{service.instance} · версия {service.version}</small>
                      <dl className="kv">
                        <dt>Работает</dt><dd>{duration(service.uptime_seconds)}</dd>
                        <dt>Последний сигнал</dt><dd>{time(service.last_seen_at)}</dd>
                        {stat(service.stats, 'goroutines') !== undefined && <><dt>Горутины</dt><dd>{stat(service.stats, 'goroutines')}</dd></>}
                        {stat(service.stats, 'heap_alloc_bytes') !== undefined && <><dt>Память процесса</dt><dd>{formatBytes(stat(service.stats, 'heap_alloc_bytes'))}</dd></>}
                        {stat(service.stats, 'cpu_percent') !== undefined && <><dt>Процессор (Go)</dt><dd>{formatNumber(stat(service.stats, 'cpu_percent')!, 1)}%</dd></>}
                        {stat(service.stats, 'runs') !== undefined && <><dt>Циклов / ошибок</dt><dd>{stat(service.stats, 'runs')} / {stat(service.stats, 'failures') ?? 0}</dd></>}
                      </dl>
                    </article>
                  ))}
                  {!data.services.length && <p className="muted">Сервисы ещё не прислали сигналы.</p>}
                </div>
              </section>
              <section className="panel col-4">
                <div className="section-heading"><div><h2>Сервер</h2><p className="muted">{host?.os} / {host?.arch} · {host?.num_cpu} ядер · {host?.go_version}</p></div><Cpu aria-hidden="true" /></div>
                <div className="meter-stack">
                  <Meter label="Процессор" value={host?.cpu_percent != null ? host.cpu_percent / 100 : null} tone={loadTone(host?.cpu_percent != null ? host.cpu_percent / 100 : null)} />
                  <Meter label="Оперативная память" value={memoryShare} tone={loadTone(memoryShare)} detail={host?.memory_total ? `${formatBytes(host.memory_used)} из ${formatBytes(host.memory_total)}` : 'недоступно'} />
                  <Meter label="Диск с файлами" value={diskShare} tone={loadTone(diskShare)} detail={host?.disk_total ? `${formatBytes(host.disk_used)} из ${formatBytes(host.disk_total)} · ${host.disk_path ?? ''}` : 'недоступно'} />
                  <p className="muted">Работает без перезапуска: {duration(data.uptime_seconds)}</p>
                </div>
              </section>
              <div className="col-4">
                <ChartCard title="Запросы в минуту" subtitle="Последний час" table={{ columns: ['Время', 'Запросов'], rows: series.map((point) => [time(point.t), point.requests_per_minute]) }}>
                  <LineChart points={series.map((point) => ({ key: point.t, label: time(point.t), value: point.requests_per_minute }))} ariaLabel="Запросы в минуту за последний час" format={(value) => formatNumber(value, 0)} />
                </ChartCard>
              </div>
              <div className="col-4">
                <ChartCard title="Время ответа, 95%" subtitle="Миллисекунды, последний час" table={{ columns: ['Время', 'мс'], rows: series.map((point) => [time(point.t), formatNumber(point.p95_ms, 0)]) }}>
                  <LineChart points={series.map((point) => ({ key: point.t, label: time(point.t), value: point.p95_ms }))} ariaLabel="Время ответа за последний час" format={(value) => `${formatNumber(value, 0)} мс`} color="var(--series-2)" />
                </ChartCard>
              </div>
              <div className="col-4">
                <ChartCard title="Загрузка процессора" subtitle="Хост, последний час" table={{ columns: ['Время', 'Процессор', 'Память'], rows: series.map((point) => [time(point.t), point.cpu_percent != null ? `${formatNumber(point.cpu_percent, 1)}%` : '—', point.memory_used_percent != null ? `${formatNumber(point.memory_used_percent, 1)}%` : '—']) }}>
                  <LineChart points={series.filter((point) => point.cpu_percent != null).map((point) => ({ key: point.t, label: time(point.t), value: point.cpu_percent! }))} domain={[0, 100]} ariaLabel="Загрузка процессора за последний час" format={(value) => `${formatNumber(value, 1)}%`} color="var(--series-7)" />
                </ChartCard>
              </div>
              <section className="panel col-5">
                <div className="section-heading"><div><h2>База данных</h2><p className="muted">{db?.version.split(' on ')[0]}</p></div><Database aria-hidden="true" /></div>
                {db && (
                  <>
                    <div className="meter-stack">
                      <Meter label="Подключения" value={db.max_connections ? db.connections / db.max_connections : null} tone={loadTone(db.max_connections ? db.connections / db.max_connections : null)} detail={`${db.connections} из ${db.max_connections}, активных ${db.active_connections}; пул приложения ${db.pool.acquired}/${db.pool.max}`} />
                      <Meter label="Попадание в кэш" value={db.cache_hit_ratio} tone={db.cache_hit_ratio != null && db.cache_hit_ratio < 0.95 ? 'warning' : 'good'} detail="ниже 95% — базе не хватает памяти" />
                    </div>
                    <dl className="kv db-kv">
                      <dt>Размер базы</dt><dd>{formatBytes(db.size_bytes)}</dd>
                      <dt>Файлы вложений</dt><dd>{formatNumber(data.storage.files_count, 0)} · {formatBytes(data.storage.files_bytes)}</dd>
                      <dt>Самый долгий запрос</dt><dd>{formatNumber(db.longest_query_seconds, 1)} с</dd>
                      <dt>Транзакций (откатов)</dt><dd>{formatNumber(db.xact_commit, 0)} ({formatNumber(db.xact_rollback, 0)})</dd>
                      <dt>Взаимные блокировки</dt><dd>{db.deadlocks}</dd>
                      <dt>Временных файлов</dt><dd>{db.temp_files}</dd>
                      <dt>PostgreSQL запущен</dt><dd>{formatDate(db.started_at, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })}</dd>
                    </dl>
                  </>
                )}
              </section>
              <div className="col-7">
                <ChartCard title="Крупнейшие таблицы" subtitle="Размер вместе с индексами" table={{ columns: ['Таблица', 'Размер', 'Строк (оценка)'], rows: (db?.tables ?? []).map((table) => [table.name, formatBytes(table.size_bytes), formatNumber(table.rows_estimate, 0)]) }}>
                  <BarList data={(db?.tables ?? []).map((table) => ({ key: table.name, label: table.name, value: table.size_bytes, hint: `${table.rows_estimate} строк` }))} format={(value) => formatBytes(value)} ariaLabel="Размер таблиц" />
                </ChartCard>
              </div>
              <section className="panel col-12">
                <div className="section-heading"><div><h2>Нагрузка по маршрутам API</h2><p className="muted">Самые частые запросы за последние 5 минут</p></div><Gauge aria-hidden="true" /></div>
                <DataTable
                  columns={['Маршрут', 'Запросов', 'Ошибок 5xx', '95% ответа, мс']}
                  rows={data.http.routes.map((route) => [route.route, route.count, route.errors, formatNumber(route.p95_ms, 0)])}
                  caption="Нагрузка по маршрутам"
                />
                {!data.http.routes.length && <p className="muted">За последние 5 минут запросов не было.</p>}
              </section>
              <section className="panel col-12 system-counts">
                <HardDrive aria-hidden="true" />
                <span>Активных сеансов: <strong>{data.counts.active_sessions}</strong></span>
                <span>Пользователей: <strong>{data.counts.users_active}</strong></span>
                <span>С временным паролем: <strong>{data.counts.pending_password_change}</strong></span>
                <span>Открытых приглашений: <strong>{data.counts.open_invitations}</strong></span>
                <span className="muted">Сформировано {formatDate(data.generated_at, { hour: '2-digit', minute: '2-digit', second: '2-digit' })}</span>
              </section>
            </div>
          </div>
        )}
      </QueryState>
    </div>
  )
}
