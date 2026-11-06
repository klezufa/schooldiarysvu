import { useEffect, useId, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { BadgeCheck, FileArchive, SearchCheck, ShieldX } from 'lucide-react'
import { api, ApiError } from '../../api/client'
import type { AcademicYear, JournalArchiveVerification, NamedEntity } from '../../api/types'
import { isAdministrator } from '../../api/types'
import { useAuth } from '../../authContext'
import { classLabeler } from '../../lib/academicYears'
import { formatDate } from '../../lib/dates'

/**
 * Архив журнала за учебный год: выбрать класс и открыть документ для печати или PDF. Администратор и руководитель
 * здесь же проверяют распечатку по коду подлинности.
 */
export function JournalArchivePanel(): React.JSX.Element {
  const id = useId()
  const navigate = useNavigate()
  const { user, hasRole } = useAuth()
  const classes = useQuery({ queryKey: ['classes'], queryFn: () => api<NamedEntity[]>('/classes') })
  const years = useQuery({ queryKey: ['academic-years'], queryFn: () => api<AcademicYear[]>('/academic-years') })
  const label = classLabeler(classes.data ?? [], years.data)
  // Сначала классы последнего года: архив чаще всего нужен в конце года.
  const sorted = useMemo(() => {
    const start = (yearId: unknown) => years.data?.find((year) => year.id === yearId)?.starts_on ?? ''
    return [...(classes.data ?? [])].sort((a, b) => start(b.academic_year_id).localeCompare(start(a.academic_year_id)) || a.name.localeCompare(b.name, 'ru'))
  }, [classes.data, years.data])
  const [classId, setClassId] = useState('')
  useEffect(() => { if (!classId && sorted[0]) setClassId(sorted[0].id) }, [classId, sorted])
  const canVerify = Boolean(user && (isAdministrator(user.roles) || hasRole('principal')))
  return (
    <section className="panel archive-panel">
      <div className="section-heading">
        <div><h2>Архив журнала за год</h2><p className="muted">Оценки и пропуски по месяцам, темы уроков, задания, замены и итоговые оценки — для печати, подписи директора и хранения. У каждой выдачи свой код подлинности.</p></div>
      </div>
      <div className="filter-bar">
        <label className="grow" htmlFor={`${id}-class`}>Класс
          <select id={`${id}-class`} value={classId} onChange={(event) => setClassId(event.target.value)}>
            {sorted.map((item) => <option key={item.id} value={item.id}>{label(item)}</option>)}
          </select>
        </label>
        <button type="button" className="button primary" disabled={!classId} onClick={() => navigate(`/archive/${classId}`)}><FileArchive aria-hidden="true" />Сформировать архив</button>
      </div>
      {canVerify && <VerifyArchive />}
    </section>
  )
}

function VerifyArchive(): React.JSX.Element {
  const id = useId()
  const [code, setCode] = useState('')
  const [submitted, setSubmitted] = useState('')
  const check = useQuery({
    queryKey: ['journal-archive-verify', submitted],
    queryFn: () => api<JournalArchiveVerification>(`/journal-archives/verify?code=${encodeURIComponent(submitted)}`),
    enabled: Boolean(submitted),
    retry: false,
  })
  const notFound = check.error instanceof ApiError && check.error.status === 404
  return (
    <form className="archive-verify" onSubmit={(event) => { event.preventDefault(); setSubmitted(code.trim()) }}>
      <h3><label htmlFor={`${id}-code`}>Проверить распечатку</label></h3>
      <div className="filter-bar">
        <input id={`${id}-code`} className="grow" value={code} onChange={(event) => setCode(event.target.value)} placeholder="Код с распечатки, например 1A2B-3C4D-5E6F-7A8B" autoComplete="off" spellCheck={false} />
        <button type="submit" className="button secondary" disabled={code.trim().length < 4}><SearchCheck aria-hidden="true" />Проверить</button>
      </div>
      {check.data && (
        <div className="notice success" role="status"><BadgeCheck aria-hidden="true" />
          <span><strong>Распечатка подлинная.</strong> Журнал {check.data.class_name} класса за {check.data.year_name} год выдан {formatDate(check.data.generated_at, { day: 'numeric', month: 'long', year: 'numeric', hour: '2-digit', minute: '2-digit' })}, {check.data.generated_by}. Сверьте SHA-256 внизу страниц: <code className="archive-digest">{check.data.digest}</code></span>
        </div>
      )}
      {notFound && <div className="notice danger" role="status"><ShieldX aria-hidden="true" /><span><strong>Архив с таким кодом не выдавался.</strong> Проверьте, не перепутаны ли буквы и цифры.</span></div>}
      {check.error && !notFound && <div className="notice danger" role="status"><ShieldX aria-hidden="true" /><span>{check.error instanceof Error ? check.error.message : 'Не удалось проверить код'}</span></div>}
    </form>
  )
}
