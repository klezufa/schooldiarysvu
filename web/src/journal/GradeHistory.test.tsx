import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { GradeHistory } from './GradeHistory'

function renderHistory(props: { gradeId?: string | null; version?: number | null }, body: unknown = []) {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  vi.stubGlobal('fetch', fetchMock)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><GradeHistory {...props} /></QueryClientProvider>)
  return fetchMock
}

describe('grade history', () => {
  it('does not request anything until the block is opened', async () => {
    const fetchMock = renderHistory({ gradeId: 'g1', version: 2 })
    expect(screen.getByText('История изменений')).toBeInTheDocument()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('shows value changes, comment and author after opening', async () => {
    const user = userEvent.setup()
    renderHistory({ gradeId: 'g1', version: 2 }, [
      { id: 'a2', action: 'grade.updated', actor_name: 'Демо Учитель 1', old_value: 5, new_value: 4, old_comment: '', new_comment: 'Исправлено', created_at: '2026-09-02T10:00:00Z' },
      { id: 'a1', action: 'grade.created', actor_name: 'Демо Учитель 1', old_value: null, new_value: 5, old_comment: null, new_comment: null, created_at: '2026-09-01T10:00:00Z' },
    ])
    await user.click(screen.getByText('История изменений'))
    expect(await screen.findByText('5 → 4; комментарий: «Исправлено»')).toBeInTheDocument()
    expect(screen.getByText('Изменена')).toBeInTheDocument()
    expect(screen.getByText('Выставлена')).toBeInTheDocument()
    expect(screen.getAllByText(/Демо Учитель 1/)).toHaveLength(2)
  })

  it('renders nothing for a grade that has not been saved yet', () => {
    const fetchMock = renderHistory({ gradeId: 'pending', version: 0 })
    expect(screen.queryByText('История изменений')).not.toBeInTheDocument()
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
