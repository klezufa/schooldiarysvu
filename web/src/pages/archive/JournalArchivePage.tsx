import { useNavigate, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, Printer, TriangleAlert } from 'lucide-react'
import { mutation } from '../../api/client'
import type { JournalArchive, JournalArchiveSubject } from '../../api/types'
import { formatDate, monthLabel } from '../../lib/dates'
import { QueryState } from '../../components/UI'
import { attendanceLegend, cellText, lessonsByMonth, termGrade } from './model'

/**
 * Архив журнала класса за учебный год для печати и хранения. Каждое открытие — новая выдача с кодом подлинности:
 * по нему руководитель проверит распечатку в дневнике. PDF сохраняется через окно печати браузера.
 */
export function JournalArchivePage(): React.JSX.Element {
  const { classId = '' } = useParams()
  const navigate = useNavigate()
  const archive = useQuery({
    queryKey: ['journal-archive', classId],
    queryFn: () => mutation<JournalArchive>('/journal-archives', 'POST', { class_id: classId }),
    enabled: Boolean(classId),
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
  })
  return (
    <div className="archive-page">
      <div className="archive-toolbar no-print">
        <button type="button" className="button secondary" onClick={() => navigate(-1)}><ArrowLeft aria-hidden="true" />Назад</button>
        <p>В окне печати выберите принтер или «Сохранить как PDF». Страницы уже настроены: А4, альбомная ориентация.</p>
        <button type="button" className="button primary" disabled={!archive.data} onClick={() => window.print()}><Printer aria-hidden="true" />Печать или PDF</button>
      </div>
      <QueryState isLoading={archive.isLoading} error={archive.error} retry={archive.refetch}>
        {archive.data && <ArchiveDocument archive={archive.data} />}
      </QueryState>
    </div>
  )
}

function ArchiveDocument({ archive }: { archive: JournalArchive }): React.JSX.Element {
  const issued = formatDate(archive.generated_at, { day: 'numeric', month: 'long', year: 'numeric', hour: '2-digit', minute: '2-digit' })
  return (
    <article className="archive-document">
      <section className="archive-sheet archive-cover">
        <p className="archive-school">{archive.school_name}</p>
        <h1>Журнал {archive.class_name} класса</h1>
        <p className="archive-year">{archive.year_name} учебный год</p>
        {!archive.final && (
          <p className="archive-warning"><TriangleAlert aria-hidden="true" />Не все учебные периоды закрыты — оценки ещё могут измениться. Окончательный архив формируют после закрытия года.</p>
        )}
        <table className="archive-table archive-summary">
          <thead><tr><th>Предмет</th><th>Учитель</th><th>Уроков</th></tr></thead>
          <tbody>
            {archive.subjects.map((subject) => (
              <tr key={subject.subject_id}><td>{subject.subject_name}</td><td>{subject.teachers.join(', ')}</td><td>{subject.lessons.filter((lesson) => lesson.status !== 'cancelled').length}</td></tr>
            ))}
          </tbody>
        </table>
        <p className="archive-meta">Учебные периоды: {archive.periods.map((period) => `${period.name} (${formatDate(period.starts_on, { day: 'numeric', month: 'short' })} — ${formatDate(period.ends_on, { day: 'numeric', month: 'short' })}${period.closed ? ', закрыт' : ''})`).join('; ') || 'не заданы'}</p>
        <p className="archive-meta">Учеников за год: {archive.students.length}{archive.students.some((student) => student.left_on) ? `, из них выбыли: ${archive.students.filter((student) => student.left_on).length}` : ''}</p>
        <div className="archive-signatures">
          <p>Директор _______________________ / _______________________ /</p>
          <p>Дата «____» ______________ 20____ г.</p>
          <p>М. П.</p>
        </div>
        <p className="archive-meta">Сформирован {issued}, {archive.generated_by}. Код подлинности <b>{archive.code}</b>.</p>
        <p className="archive-meta archive-digest">SHA-256: {archive.digest}</p>
      </section>
      {archive.subjects.map((subject) => <SubjectPages key={subject.subject_id} archive={archive} subject={subject} />)}
    </article>
  )
}

function studentLabel(student: JournalArchive['students'][number]): string {
  return student.left_on ? `${student.display_name} (выбыл ${formatDate(student.left_on, { day: '2-digit', month: '2-digit', year: 'numeric' })})` : student.display_name
}

function SubjectPages({ archive, subject }: { archive: JournalArchive; subject: JournalArchiveSubject }): React.JSX.Element {
  const cell = cellText(subject)
  const term = termGrade(subject)
  const heading = <header className="archive-heading"><h2>{subject.subject_name}</h2><span>{archive.class_name} · {archive.year_name} · {subject.teachers.join(', ')} · код {archive.code}</span></header>
  return (
    <>
      {lessonsByMonth(subject.lessons).map(({ month, lessons }) => (
        <section className="archive-sheet" key={month}>
          {heading}
          <h3>{monthLabel(month)}</h3>
          <table className="archive-table archive-grid">
            <thead>
              <tr><th className="archive-number">№</th><th className="archive-name">Ученик</th>{lessons.map((lesson) => <th key={lesson.id} className={lesson.status === 'cancelled' ? 'archive-cancelled' : ''}>{Number(lesson.lesson_date.slice(8))}</th>)}</tr>
            </thead>
            <tbody>
              {archive.students.map((student, index) => (
                <tr key={student.id}>
                  <td className="archive-number">{index + 1}</td>
                  <td className="archive-name">{studentLabel(student)}</td>
                  {lessons.map((lesson) => <td key={lesson.id}>{cell(lesson.id, student.id)}</td>)}
                </tr>
              ))}
            </tbody>
          </table>
          <table className="archive-table archive-topics">
            <thead><tr><th>Дата</th><th>Тема урока</th><th>Домашнее задание</th><th>Учитель</th></tr></thead>
            <tbody>
              {lessons.map((lesson) => (
                <tr key={lesson.id}>
                  <td>{formatDate(lesson.lesson_date, { day: 'numeric', month: 'numeric' })}</td>
                  <td>{lesson.status === 'cancelled' ? `Урок отменён${lesson.cancellation_reason ? `: ${lesson.cancellation_reason}` : ''}` : lesson.topic || '—'}</td>
                  <td>{lesson.homework || '—'}</td>
                  <td>{lesson.substitute_for ? `${lesson.teacher_name} (замена: ${lesson.substitute_for})` : lesson.teacher_name}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="archive-legend">{attendanceLegend}</p>
        </section>
      ))}
      {archive.periods.length > 0 && (
        <section className="archive-sheet">
          {heading}
          <h3>Итоговые оценки</h3>
          <table className="archive-table archive-terms">
            <thead><tr><th className="archive-number">№</th><th className="archive-name">Ученик</th>{archive.periods.map((period) => <th key={period.id}>{period.name}</th>)}</tr></thead>
            <tbody>
              {archive.students.map((student, index) => (
                <tr key={student.id}>
                  <td className="archive-number">{index + 1}</td>
                  <td className="archive-name">{studentLabel(student)}</td>
                  {archive.periods.map((period) => <td key={period.id}>{term(student.id, period.id)}</td>)}
                </tr>
              ))}
            </tbody>
          </table>
          <p className="archive-legend">Подпись учителя _______________________</p>
        </section>
      )}
    </>
  )
}
