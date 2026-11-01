import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { journalKey, rollbackEntries, updateEntryGrade, useGradeMutation } from './useGradeMutation'
import { entryFixture } from '../test/fixtures'
import type { Grade, JournalEntry } from '../api/types'

const responseGrade: Grade = {
  id: entryFixture.grade_id!, student_id: entryFixture.student_id, lesson_work_id: entryFixture.lesson_work_id,
  academic_period_id: '90000000-0000-4000-8000-000000000001', value: 5, version: 2, comment: '',
}

function setup() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const key = journalKey('class', 'subject', 'period')
  client.setQueryData(key, [entryFixture])
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
  const hook = renderHook(() => useGradeMutation('class', 'subject', 'period'), { wrapper })
  return { client, key, hook }
}

describe('grade optimistic mutation', () => {
  it('updates a cell and applies the returned server version', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify(responseGrade), { status: 200, headers: { 'Content-Type': 'application/json' } })))
    const { client, key, hook } = setup()
    hook.result.current.save.mutate({ entry: entryFixture, value: 5, comment: '' })
    await waitFor(() => expect(client.getQueryData<JournalEntry[]>(key)?.[0]?.grade_version).toBe(2))
    expect(client.getQueryData<JournalEntry[]>(key)?.[0]?.grade_value).toBe(5)
  })

  it('rolls back on error and retains conflict details', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ code: 'conflict', message: 'Чужая версия', request_id: 'r1' }), { status: 409, headers: { 'Content-Type': 'application/json' } })))
    const { client, key, hook } = setup()
    hook.result.current.save.mutate({ entry: entryFixture, value: 2, comment: 'Локальное' })
    await waitFor(() => expect(hook.result.current.conflict?.value).toBe(2))
    expect(client.getQueryData<JournalEntry[]>(key)?.[0]?.grade_value).toBe(4)
    expect(hook.result.current.conflict?.comment).toBe('Локальное')
  })

  it('supports pure update and rollback helpers', () => {
    const client = new QueryClient()
    const key = journalKey('c', 's', 'p')
    const updated = updateEntryGrade([entryFixture], { entry: entryFixture, value: 5, comment: '' }, responseGrade)
    client.setQueryData(key, updated)
    expect(client.getQueryData<JournalEntry[]>(key)?.[0]?.grade_value).toBe(5)
    rollbackEntries(client, key, [entryFixture])
    expect(client.getQueryData<JournalEntry[]>(key)?.[0]?.grade_value).toBe(4)
  })
})
