import { useRef, useState } from 'react'
import { useMutation, useQueryClient, type QueryClient, type QueryKey } from '@tanstack/react-query'
import { mutation } from '../api/client'
import type { Grade, JournalEntry } from '../api/types'

export type GradeInput = { entry: JournalEntry; value: number; comment: string }
export type GradeConflict = { entry: JournalEntry; value: number; comment: string; message: string }

export function journalKey(classId: string, subjectId: string, periodId: string): QueryKey {
  return ['journal', classId, subjectId, periodId]
}

export function updateEntryGrade(entries: JournalEntry[] | undefined, input: GradeInput, grade?: Grade): JournalEntry[] | undefined {
  return entries?.map((entry) => entry.student_id === input.entry.student_id && entry.lesson_work_id === input.entry.lesson_work_id ? {
    ...entry,
    grade_id: grade?.id ?? entry.grade_id ?? 'pending',
    grade_value: input.value,
    grade_comment: input.comment,
    grade_version: grade?.version ?? entry.grade_version ?? 0,
  } : entry)
}

export function rollbackEntries(client: QueryClient, key: QueryKey, previous: JournalEntry[] | undefined): void {
  client.setQueryData(key, previous)
}

export function useGradeMutation(classId: string, subjectId: string, periodId: string) {
  const queryClient = useQueryClient()
  const key = journalKey(classId, subjectId, periodId)
  const [conflict, setConflict] = useState<GradeConflict | null>(null)
  const lastChange = useRef<{ before?: JournalEntry[]; after: Grade } | null>(null)
  const [canUndo, setCanUndo] = useState(false)
  const save = useMutation({
    mutationFn: ({ entry, value, comment }: GradeInput) => entry.grade_id && entry.grade_id !== 'pending'
      ? mutation<Grade>(`/grades/${entry.grade_id}?version=${entry.grade_version ?? 1}`, 'PATCH', { value, comment })
      : mutation<Grade>('/grades', 'POST', { student_id: entry.student_id, lesson_work_id: entry.lesson_work_id, value, comment }),
    onMutate: async (input) => {
      setConflict(null)
      await queryClient.cancelQueries({ queryKey: key })
      const before = queryClient.getQueryData<JournalEntry[]>(key)
      queryClient.setQueryData<JournalEntry[]>(key, (current) => updateEntryGrade(current, input))
      return { before, input }
    },
    onError: (error, input, context) => {
      rollbackEntries(queryClient, key, context?.before)
      if (error instanceof Error && 'status' in error && Number(error.status) === 409) {
        setConflict({ entry: input.entry, value: input.value, comment: input.comment, message: error.message })
      }
    },
    onSuccess: (grade, input, context) => {
      queryClient.setQueryData<JournalEntry[]>(key, (current) => updateEntryGrade(current, input, grade))
      lastChange.current = { before: context?.before, after: grade }
      setCanUndo(true)
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: key }),
  })
  const undo = useMutation({
    mutationFn: async () => {
      const change = lastChange.current
      if (!change) throw new Error('Нет изменения для отмены')
      const before = change.before?.find((entry) => entry.student_id === change.after.student_id && entry.lesson_work_id === change.after.lesson_work_id)
      if (before?.grade_id && before.grade_value !== null && before.grade_value !== undefined) {
        return mutation<Grade>(`/grades/${change.after.id}?version=${change.after.version}`, 'PATCH', { value: before.grade_value, comment: before.grade_comment ?? '' })
      }
      await mutation<void>(`/grades/${change.after.id}?version=${change.after.version}`, 'DELETE')
      return null
    },
    onSuccess: () => { lastChange.current = null; setCanUndo(false); queryClient.invalidateQueries({ queryKey: key }) },
  })
  const reloadServer = async () => {
    await queryClient.invalidateQueries({ queryKey: key })
    await queryClient.refetchQueries({ queryKey: key })
  }
  return { save, undo, conflict, setConflict, reloadServer, canUndo }
}
