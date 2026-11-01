import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { SchedulePage } from './Schedule'
import { ToastProvider } from '../components/Toast'

const auth = vi.hoisted(() => ({ roles: ['teacher'] as string[] }))
const student = vi.hoisted(() => ({ studentId: undefined as string | undefined }))

vi.mock('../authContext', () => ({ useAuth: () => ({ user: null, hasRole: (...roles: string[]) => roles.some((role) => auth.roles.includes(role)) }) }))
vi.mock('../studentContext', () => ({ useStudent: () => ({ studentId: student.studentId, children: [], setStudentId: vi.fn(), isLoading: false }) }))

function renderSchedule() {
  const fetchMock = vi.fn().mockResolvedValue(new Response('[]', { status: 200, headers: { 'Content-Type': 'application/json' } }))
  vi.stubGlobal('fetch', fetchMock)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><ToastProvider><MemoryRouter><SchedulePage /></MemoryRouter></ToastProvider></QueryClientProvider>)
  return fetchMock
}

describe('schedule page query gating', () => {
  it('loads the schedule for a teacher, who has no student id', async () => {
    auth.roles = ['teacher']; student.studentId = undefined
    const fetchMock = renderSchedule()
    await screen.findByRole('heading', { name: 'Уроков нет' })
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes('/schedule'))).toBe(true)
  })

  it('does not request a parent schedule until a child is selected', async () => {
    auth.roles = ['parent']; student.studentId = undefined
    const fetchMock = renderSchedule()
    await screen.findByRole('heading', { name: 'Уроков нет' })
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes('/schedule'))).toBe(false)
  })

  it('scopes a parent request to the selected child', async () => {
    auth.roles = ['parent']; student.studentId = 'child-2'
    const fetchMock = renderSchedule()
    await screen.findByRole('heading', { name: 'Уроков нет' })
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes('student_id=child-2'))).toBe(true)
  })
})
