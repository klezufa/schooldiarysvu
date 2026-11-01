import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AuthProvider } from '../auth'
import { StudentProvider } from '../student'
import { ToastProvider } from '../components/Toast'
import { HomeworkPage } from './Homework'
import { homeworkFixture, userFixture } from '../test/fixtures'

describe('homework personal completion', () => {
  it('labels the mark as personal and sends only the current student id', async () => {
    const calls: { url: string; init?: RequestInit }[] = []
    vi.stubGlobal('fetch', vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      calls.push({ url, init })
      let body: unknown = []
      let status = 200
      if (url === '/api/v1/me') body = userFixture
      else if (url.startsWith('/api/v1/homework') && init?.method === 'PUT') body = { completed: true }
      else if (url.startsWith('/api/v1/homework')) body = [homeworkFixture]
      else if (url.startsWith('/api/v1/academic-years') || url.startsWith('/api/v1/subjects')) body = []
      else status = 404
      return Promise.resolve(new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }))
    }))
    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    const user = userEvent.setup()
    render(<QueryClientProvider client={client}><MemoryRouter><ToastProvider><AuthProvider><StudentProvider><HomeworkPage /></StudentProvider></AuthProvider></ToastProvider></MemoryRouter></QueryClientProvider>)
    const checkbox = await screen.findByRole('checkbox', { name: /Личная отметка о выполнении/ })
    expect(screen.getByText('Не является официальной сдачей работы учителю')).toBeInTheDocument()
    await user.click(checkbox)
    await waitFor(() => expect(calls.some((call) => call.init?.method === 'PUT')).toBe(true))
    const mutation = calls.find((call) => call.init?.method === 'PUT')
    expect(mutation?.url).toContain(homeworkFixture.id)
    expect(JSON.parse(String(mutation?.init?.body))).toEqual({ student_id: userFixture.student_id, completed: true })
  })
})
