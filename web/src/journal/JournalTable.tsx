import { useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { createColumnHelper, flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { useVirtualizer } from '@tanstack/react-virtual'
import type { JournalEntry, Student } from '../api/types'
import { workNames } from '../api/types'
import { formatDate } from '../lib/dates'
import { gradeTone } from '../lib/chartMath'
import { GradeEditor } from './GradeEditor'
import type { GradeConflict, GradeInput } from './useGradeMutation'

export type JournalMode = 'lesson' | 'student' | 'all'
type JournalRow = { student: Student; cells: Record<string, JournalEntry>; average: number | null }

export function JournalTable({ entries, students, allowedValues, onSave, isSaving, conflict, onCloseConflict, month, mode, selectedLessonId, selectedStudentId, compact }: {
  entries: JournalEntry[]
  students: Student[]
  allowedValues: number[]
  onSave: (input: GradeInput, onSaved?: () => void) => void
  isSaving: boolean
  conflict: GradeConflict | null
  onCloseConflict: () => void
  month: string
  mode: JournalMode
  selectedLessonId: string
  selectedStudentId: string
  compact: boolean
}): React.JSX.Element {
  const [editing, setEditing] = useState<{ entry: JournalEntry; row: number; col: number; initialValue?: number } | null>(null)
  const scrollRef = useRef<HTMLDivElement>(null)
  const works = useMemo(() => {
    const map = new Map<string, JournalEntry>()
    entries.filter((entry) => (!month || entry.lesson_date.startsWith(month)) && (mode !== 'lesson' || entry.lesson_id === selectedLessonId)).forEach((entry) => map.set(entry.lesson_work_id, entry))
    return [...map.values()].sort((a, b) => a.lesson_date.localeCompare(b.lesson_date))
  }, [entries, month, mode, selectedLessonId])
  const visibleStudents = useMemo(() => students.filter((student) => mode !== 'student' || student.id === selectedStudentId), [students, mode, selectedStudentId])
  const rows = useMemo<JournalRow[]>(() => visibleStudents.map((student) => {
    const own = entries.filter((entry) => entry.student_id === student.id)
    const cells = Object.fromEntries(own.map((entry) => [entry.lesson_work_id, entry]))
    const weighted = own.reduce((result, entry) => entry.grade_value === null || entry.grade_value === undefined ? result : { sum: result.sum + entry.grade_value * entry.weight, weight: result.weight + entry.weight }, { sum: 0, weight: 0 })
    return { student, cells, average: weighted.weight ? Math.round(weighted.sum / weighted.weight * 100) / 100 : null }
  }), [visibleStudents, entries])
  const helper = createColumnHelper<JournalRow>()
  const columns = useMemo(() => [
    helper.accessor('student.display_name', { id: 'student', header: 'Ученик', size: 240, cell: (info) => <span className="student-name" title={info.getValue()}>{info.getValue()}</span> }),
    ...works.map((work, columnIndex) => helper.display({ id: work.lesson_work_id, size: compact ? 82 : 108, header: () => <span className="work-heading"><strong>{formatDate(work.lesson_date, { day: '2-digit', month: '2-digit' })}</strong><small title={work.work_title}>{workNames[work.work_type] ?? work.work_type}</small></span>, cell: (info) => {
      const entry = info.row.original.cells[work.lesson_work_id]
      if (!entry) return <span className="grade-empty">—</span>
      return <button className={`grade-cell ${entry.grade_value != null ? `tone-${gradeTone(entry.grade_value)}` : 'grade-empty'} ${entry.grade_id === 'pending' ? 'saving' : ''}`} data-grade-cell data-row={info.row.index} data-col={columnIndex} aria-label={`${info.row.original.student.display_name}, ${formatDate(work.lesson_date)}, оценка ${entry.grade_value ?? 'не выставлена'}`} onClick={() => setEditing({ entry, row: info.row.index, col: columnIndex })}>{entry.grade_value ?? '·'}{entry.grade_comment && <span title={entry.grade_comment}> *</span>}</button>
    } })),
    helper.accessor('average', { id: 'average', header: 'Средний балл', size: 110, cell: (info) => <strong>{info.getValue()?.toFixed(2) ?? '—'}</strong> }),
  ], [helper, works, compact])
  const table = useReactTable({ data: rows, columns, getCoreRowModel: getCoreRowModel() })
  const virtualizer = useVirtualizer({ count: table.getRowModel().rows.length, getScrollElement: () => scrollRef.current, estimateSize: () => compact ? 44 : 56, overscan: 10 })
  const focusCell = (row: number, col: number) => {
    if (row < 0 || col < 0 || row >= rows.length || col >= works.length) return
    virtualizer.scrollToIndex(row)
    // Строка вне окна виртуализации появляется в DOM только после рендера.
    window.setTimeout(() => scrollRef.current?.querySelector<HTMLElement>(`[data-row="${row}"][data-col="${col}"]`)?.focus(), 40)
  }
  const keyboard = (event: KeyboardEvent<HTMLDivElement>) => {
    const target = event.target as HTMLElement
    if (!target.matches('[data-grade-cell]')) return
    const row = Number(target.dataset.row)
    const col = Number(target.dataset.col)
    const moves: Record<string, [number, number]> = { ArrowRight: [0, 1], ArrowLeft: [0, -1], ArrowDown: [1, 0], ArrowUp: [-1, 0] }
    const move = moves[event.key]
    if (move) { event.preventDefault(); focusCell(row + move[0], col + move[1]); return }
    if (/^[0-9]$/.test(event.key) && allowedValues.includes(Number(event.key))) {
      const workId = works[col]?.lesson_work_id
      const entry = workId ? rows[row]?.cells[workId] : undefined
      if (entry) { event.preventDefault(); setEditing({ entry, row, col, initialValue: Number(event.key) }) }
    }
  }
  // Редактор показывает актуальную серверную запись, а не снимок на момент открытия.
  const liveEntry = editing ? entries.find((item) => item.student_id === editing.entry.student_id && item.lesson_work_id === editing.entry.lesson_work_id) ?? editing.entry : null
  if (!works.length) return <div className="state-view"><h3>В выбранном периоде нет работ</h3><p>Выберите урок и добавьте классную или контрольную работу, чтобы выставлять оценки.</p></div>
  return <><div className={`journal-scroll ${compact ? 'compact' : ''}`} ref={scrollRef} onKeyDown={keyboard}><table className="journal-table"><thead>{table.getHeaderGroups().map((group) => <tr key={group.id}>{group.headers.map((header) => <th key={header.id} style={{ width: header.getSize() }} className={header.id === 'student' ? 'sticky-student' : header.id === 'average' ? 'sticky-average' : ''}>{flexRender(header.column.columnDef.header, header.getContext())}</th>)}</tr>)}</thead><tbody style={{ height: virtualizer.getTotalSize() }}>{virtualizer.getVirtualItems().map((virtualRow) => {
    const row = table.getRowModel().rows[virtualRow.index]!
    return <tr key={row.id} style={{ height: virtualRow.size, transform: `translateY(${virtualRow.start}px)` }}>{row.getVisibleCells().map((cell) => <td key={cell.id} style={{ width: cell.column.getSize() }} className={cell.column.id === 'student' ? 'sticky-student' : cell.column.id === 'average' ? 'sticky-average' : ''}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</td>)}</tr>
  })}</tbody></table></div><GradeEditor entry={liveEntry} initialValue={editing?.initialValue} open={Boolean(editing)} onOpenChange={(open) => { if (!open) setEditing(null) }} allowedValues={allowedValues} onSave={(input, done) => { const position = editing; onSave(input, () => { done?.(); if (position) focusCell(position.row + 1, position.col) }) }} isSaving={isSaving} conflict={conflict} onCloseConflict={onCloseConflict} /></>
}
