import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AttendancePanel } from './Attendance'
import { ToastProvider } from '../components/Toast'

const student = { id: '20000000-0000-4000-8000-000000000001', display_name: 'Ученик Тестовый' }

describe('attendance bulk operation', () => {
  it('sends a stable idempotency key and shows the server result', async () => {
    const fetchMock = vi.fn().mockImplementation((_url: string, init?: RequestInit) => {
      if (init?.method === 'POST') return Promise.resolve(new Response(JSON.stringify({ created_or_updated: 1, items: [{ lesson_id: 'lesson', student_id: student.id, status: 'present', version: 1 }] }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
      return Promise.resolve(new Response('[]', { status: 200, headers: { 'Content-Type': 'application/json' } }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const user = userEvent.setup()
    render(<QueryClientProvider client={client}><ToastProvider><AttendancePanel lessonId="lesson" students={[student]} /></ToastProvider></QueryClientProvider>)
    const button = await screen.findByRole('button', { name: 'Все присутствуют' })
    await waitFor(() => expect(button).toBeEnabled())
    await user.click(button)
    await screen.findByText('Посещаемость сохранена: 1 записей')
    const headers = fetchMock.mock.calls.find(([, init]) => (init as RequestInit | undefined)?.method === 'POST')?.[1].headers as Headers
    expect(headers.get('Idempotency-Key')).toBeTruthy()
    expect(headers.get('X-CSRF-Token')).not.toBeNull()
  })

  it('uses a fresh idempotency key after a successful bulk operation but keeps it for a retry after an error', async () => {
    let failNext = false
    const fetchMock = vi.fn().mockImplementation((_url: string, init?: RequestInit) => {
      if (init?.method === 'POST') {
        if (failNext) { failNext = false; return Promise.resolve(new Response(JSON.stringify({ code: 'internal', message: 'Сбой', request_id: 'r' }), { status: 500, headers: { 'Content-Type': 'application/json' } })) }
        return Promise.resolve(new Response(JSON.stringify({ created_or_updated: 1, items: [] }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
      }
      return Promise.resolve(new Response('[]', { status: 200, headers: { 'Content-Type': 'application/json' } }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const user = userEvent.setup()
    render(<QueryClientProvider client={client}><ToastProvider><AttendancePanel lessonId="lesson" students={[student]} /></ToastProvider></QueryClientProvider>)
    const button = await screen.findByRole('button', { name: 'Все присутствуют' })
    await waitFor(() => expect(button).toBeEnabled())
    const keys = () => fetchMock.mock.calls.filter(([, init]) => (init as RequestInit | undefined)?.method === 'POST').map(([, init]) => ((init as RequestInit).headers as Headers).get('Idempotency-Key'))
    failNext = true
    await user.click(button)
    await waitFor(() => expect(keys()).toHaveLength(1))
    await waitFor(() => expect(button).toBeEnabled())
    await user.click(button)
    await waitFor(() => expect(keys()).toHaveLength(2))
    await waitFor(() => expect(button).toBeEnabled())
    await user.click(button)
    await waitFor(() => expect(keys()).toHaveLength(3))
    const [first, second, third] = keys()
    expect(second).toBe(first)
    expect(third).not.toBe(second)
  })
})
