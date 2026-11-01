import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { GradeHistoryEntry } from '../api/types'
import { formatDate } from '../lib/dates'
import { QueryState } from '../components/UI'

const actionNames: Record<string, string> = { 'grade.created': 'Выставлена', 'grade.updated': 'Изменена', 'grade.deleted': 'Отменена' }

function describe(entry: GradeHistoryEntry): string {
  if (entry.action === 'grade.updated') {
    const parts = [`${entry.old_value ?? '—'} → ${entry.new_value ?? '—'}`]
    if ((entry.old_comment ?? '') !== (entry.new_comment ?? '')) parts.push(`комментарий: «${entry.new_comment || 'удалён'}»`)
    return parts.join('; ')
  }
  if (entry.action === 'grade.deleted') return `было ${entry.old_value ?? '—'}`
  return `${entry.new_value ?? '—'}${entry.new_comment ? `; комментарий: «${entry.new_comment}»` : ''}`
}

function HistoryList({ gradeId, version }: { gradeId: string; version: number }): React.JSX.Element {
  // Версия входит в ключ: после сохранения оценки история запрашивается заново.
  const history = useQuery({ queryKey: ['grade-history', gradeId, version], queryFn: () => api<GradeHistoryEntry[]>(`/grades/${gradeId}/history`) })
  return <QueryState isLoading={history.isLoading} error={history.error} retry={history.refetch} isEmpty={!history.data?.length} emptyTitle="Изменений нет">
    <ol className="grade-history">{history.data?.map((entry) => <li key={entry.id}>
      <strong>{actionNames[entry.action] ?? entry.action}</strong> <span>{describe(entry)}</span>
      <small>{entry.actor_name ?? 'Система'} · <time dateTime={entry.created_at}>{formatDate(entry.created_at, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })}</time></small>
    </li>)}</ol>
  </QueryState>
}

/** История изменения оценки (только чтение). Запрос выполняется лишь после раскрытия блока. */
export function GradeHistory({ gradeId, version }: { gradeId?: string | null; version?: number | null }): React.JSX.Element | null {
  const [open, setOpen] = useState(false)
  if (!gradeId || gradeId === 'pending') return null
  return <details className="grade-history-block" onToggle={(event) => setOpen(event.currentTarget.open)}>
    <summary>История изменений</summary>
    {open && <HistoryList gradeId={gradeId} version={version ?? 0} />}
  </details>
}
