import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, Download, FileSpreadsheet, Info, Upload } from 'lucide-react'
import { mutation, saveBlob } from '../../api/client'
import { dateISO } from '../../lib/dates'
import { useDirectory } from './helpers'
import type { UserImportResult, UserImportRow } from '../../api/types'
import { parseImport } from './importParser'
import { useToast } from '../../components/toastContext'
import { StatusBadge } from '../../components/UI'

const roleTitles: Record<UserImportRow['role'], string> = { student: 'Ученик', teacher: 'Учитель', parent: 'Родитель', principal: 'Руководитель' }

function credentialsCSV(result: UserImportResult): Blob {
  // Ячейка, начинающаяся с = + - @ (или табуляции), в Excel становится формулой — апостроф делает её текстом.
  const escape = (value: string) => `"${(/^\s*[=+\-@]/.test(value) || /^[\t\r]/.test(value) ? `'${value}` : value).replace(/"/g, '""')}"`
  const lines = [['ФИО', 'Роль', 'Класс', 'Логин', 'Временный пароль'].map(escape).join(';')]
  result.created.forEach((item) => lines.push([item.display_name, roleTitles[item.role as UserImportRow['role']] ?? item.role, item.class_name ?? '', item.login, item.password].map(escape).join(';')))
  // BOM — чтобы Excel открыл кириллицу в UTF-8 без мастера импорта.
  return new Blob(['﻿' + lines.join('\r\n')], { type: 'text/csv;charset=utf-8' })
}

export function ImportAdmin(): React.JSX.Element {
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const [text, setText] = useState('')
  const [defaultRole, setDefaultRole] = useState<UserImportRow['role']>('student')
  const [preview, setPreview] = useState<UserImportResult | null>(null)
  const [done, setDone] = useState<UserImportResult | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const { years, year } = useDirectory()
  // Классы из таблицы ищутся в выбранном учебном году: при подготовке нового года — в нём, а не в текущем.
  const [yearId, setYearId] = useState('')
  const importYear = years.data?.find((item) => item.id === yearId) ?? year
  const openYears = (years.data ?? []).filter((item) => item.ends_on >= dateISO())
  const parsed = parseImport(text, defaultRole)

  const run = async (dryRun: boolean) => {
    setPending(true)
    setError('')
    try {
      const result = await mutation<UserImportResult>('/users/import', 'POST', { dry_run: dryRun, academic_year_id: importYear?.id, rows: parsed.rows })
      if (dryRun || result.errors.length) {
        setPreview(result)
      } else {
        setDone(result)
        setPreview(null)
        setText('')
        showToast(`Создано учётных записей: ${result.created.length}`)
        await queryClient.invalidateQueries({ queryKey: ['users'] })
      }
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Не удалось выполнить импорт')
    } finally {
      setPending(false)
    }
  }

  if (done) {
    return (
      <section className="panel">
        <div className="section-heading"><div><h2><CheckCircle2 aria-hidden="true" className="inline-icon" /> Создано: {done.created.length}</h2><p className="muted">Скачайте листы доступа сейчас — временные пароли больше нигде не сохраняются.</p></div></div>
        <div className="card-actions" style={{ marginBottom: '1rem' }}>
          <button className="button primary" onClick={() => saveBlob(credentialsCSV(done), `логины-${new Date().toISOString().slice(0, 10)}.csv`)}><Download aria-hidden="true" />Скачать логины и пароли (CSV)</button>
          <button className="button secondary" onClick={() => window.print()}>Распечатать</button>
          <button className="button text-button" onClick={() => setDone(null)}>Новый импорт</button>
        </div>
        <div className="responsive-table">
          <table><thead><tr><th>ФИО</th><th>Роль</th><th>Класс</th><th>Логин</th><th>Временный пароль</th></tr></thead>
            <tbody>{done.created.map((item) => <tr key={item.row}><td>{item.display_name}</td><td>{roleTitles[item.role as UserImportRow['role']] ?? item.role}</td><td>{item.class_name ?? '—'}</td><td><code>{item.login}</code></td><td><code>{item.password}</code></td></tr>)}</tbody>
          </table>
        </div>
      </section>
    )
  }

  return (
    <div className="dash-grid">
      <section className="panel col-7">
        <div className="section-heading"><div><h2><FileSpreadsheet aria-hidden="true" className="inline-icon" /> Импорт списком</h2><p className="muted">Скопируйте строки из Excel или вставьте CSV. До 200 строк за раз.</p></div></div>
        <div className="stack-form">
          <label className="field">Роль, если колонка «роль» пустая
            <select value={defaultRole} onChange={(event) => setDefaultRole(event.target.value as UserImportRow['role'])}>
              {Object.entries(roleTitles).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
            </select>
          </label>
          {openYears.length > 1 && (
            <label className="field">Учебный год для колонки «класс»
              <select value={importYear?.id ?? ''} onChange={(event) => { setYearId(event.target.value); setPreview(null) }}>
                {openYears.map((item) => <option key={item.id} value={item.id}>{item.name}{item.is_current ? ' (текущий)' : ''}</option>)}
              </select>
            </label>
          )}
          <label className="field">Таблица
            <textarea rows={10} value={text} onChange={(event) => { setText(event.target.value); setPreview(null) }} placeholder={'ФИО;роль;класс;логин\nИванов Иван Петрович;ученик;5А\nПетрова Анна Сергеевна;ученик;5А\nСидоров Олег Ильич;учитель'} />
          </label>
          <label className="button secondary file-upload" style={{ width: 'fit-content' }}>
            <Upload aria-hidden="true" />Загрузить файл CSV
            <input type="file" accept=".csv,.txt,text/csv,text/plain" onChange={async (event) => { const file = event.target.files?.[0]; if (file) { setText(await file.text()); setPreview(null) } }} />
          </label>
          {parsed.problems.map((problem) => <div className="form-error" key={problem.line}>Строка {problem.line}: {problem.message}</div>)}
          {error && <div className="form-error" role="alert">{error}</div>}
          <div className="card-actions">
            <button className="button secondary" disabled={pending || !parsed.rows.length || parsed.problems.length > 0} onClick={() => run(true)}>Проверить ({parsed.rows.length})</button>
            <button className="button primary" disabled={pending || !preview || preview.errors.length > 0} onClick={() => run(false)}>Создать учётные записи</button>
          </div>
        </div>
      </section>
      <aside className="panel col-5">
        <h2>Как это работает</h2>
        <ul className="compact-list">
          <li><strong>Колонки</strong><span>ФИО · роль · класс · логин. Обязательно только ФИО.</span></li>
          <li><strong>Логины</strong><span>Строятся из ФИО: «Иванов Иван Петрович» → ivanov.ip; при совпадении добавляется номер.</span></li>
          <li><strong>Классы</strong><span>Ученики сразу зачисляются в указанный класс {importYear ? `учебного года ${importYear.name}` : 'текущего учебного года'}; в класс следующего года — с первого дня того года.</span></li>
          <li><strong>Пароли</strong><span>Генерируются автоматически и показываются один раз; при входе их нужно сменить.</span></li>
          <li><strong>Всё или ничего</strong><span>Если в любой строке ошибка, не создаётся никто — исправьте и повторите.</span></li>
        </ul>
        <div className="notice info"><Info aria-hidden="true" /><span>Сначала нажмите «Проверить»: сервер покажет будущие логины и ошибки, ничего не создавая.</span></div>
      </aside>
      {preview && (
        <section className="panel col-12">
          <div className="section-heading">
            <div><h2>Проверка</h2><p className="muted">{preview.errors.length ? 'Исправьте ошибки и проверьте снова' : 'Ошибок нет — можно создавать'}</p></div>
            {preview.errors.length ? <StatusBadge tone="danger">Ошибок: {preview.errors.length}</StatusBadge> : <StatusBadge tone="success">Готово к созданию: {preview.created.length}</StatusBadge>}
          </div>
          <div className="responsive-table import-preview">
            <table><thead><tr><th>№</th><th>ФИО</th><th>Роль</th><th>Класс</th><th>Логин / ошибка</th></tr></thead>
              <tbody>
                {parsed.rows.map((row, index) => {
                  const number = index + 1
                  const problem = preview.errors.find((item) => item.row === number)
                  const planned = preview.created.find((item) => item.row === number)
                  return <tr key={number}><td>{number}</td><td>{row.display_name}</td><td>{roleTitles[row.role]}</td><td>{row.class_name ?? '—'}</td>{problem ? <td className="error">{problem.message}</td> : <td><code>{planned?.login ?? '—'}</code></td>}</tr>
                })}
              </tbody>
            </table>
          </div>
        </section>
      )}
    </div>
  )
}
