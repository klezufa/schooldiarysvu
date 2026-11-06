import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { AuthProvider, ProtectedRoute } from './auth'
import { useAuth } from './authContext'
import { userFixture } from './test/fixtures'

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

function Probe(): React.JSX.Element {
  const { changePassword } = useAuth()
  const children = useQuery({ queryKey: ['children'], queryFn: async () => {
    const response = await fetch('/api/v1/children')
    if (!response.ok) throw new Error('forbidden')
    return response.json() as Promise<{ id: string }[]>
  }, retry: false })
  return <div>
    <p>{children.isError ? 'ошибка' : children.data ? `детей: ${children.data.length}` : 'загрузка'}</p>
    <button onClick={() => void changePassword({ old_password: 'old-password-123', new_password: 'new-password-456' })}>change</button>
  </div>
}

describe('first login with a temporary password', () => {
  it('refetches data that failed with 403 before the password was changed', async () => {
    let passwordChanged = false
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/auth/password')) { passwordChanged = true; return Promise.resolve(new Response(null, { status: 204 })) }
      if (url.endsWith('/me')) return Promise.resolve(json({ ...userFixture, roles: ['parent'], must_change_password: !passwordChanged }))
      if (url.endsWith('/children')) return Promise.resolve(passwordChanged ? json([{ id: 'c1' }, { id: 'c2' }]) : json({ code: 'password_change_required', message: 'Сначала смените временный пароль' }, 403))
      return Promise.resolve(json({}, 404))
    }))
    document.cookie = 'diary_csrf=' + 'x'.repeat(40)
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(<QueryClientProvider client={client}><MemoryRouter><AuthProvider><Probe /></AuthProvider></MemoryRouter></QueryClientProvider>)
    expect(await screen.findByText('ошибка')).toBeInTheDocument()
    await act(async () => { screen.getByRole('button', { name: 'change' }).click() })
    await waitFor(() => expect(screen.getByText('детей: 2')).toBeInTheDocument())
  })
})

describe('opening a protected page without a session', () => {
  it('redirects to the login page instead of checking the session forever', async () => {
    // 401 от /me вызывает событие diary:unauthorized, пока сам запрос /me ещё не завершился. Раньше обработчик
    // очищал весь кеш вместе с этим запросом, и страница навсегда оставалась на «Проверяем сессию…».
    vi.stubGlobal('fetch', vi.fn().mockImplementation(() => Promise.resolve(json({ code: 'unauthorized', message: 'Требуется вход' }, 401))))
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(<QueryClientProvider client={client}><MemoryRouter initialEntries={['/']}><AuthProvider>
      <Routes>
        <Route path="/login" element={<p>страница входа</p>} />
        <Route path="/" element={<ProtectedRoute><p>главная</p></ProtectedRoute>} />
      </Routes>
    </AuthProvider></MemoryRouter></QueryClientProvider>)
    expect(await screen.findByText('страница входа')).toBeInTheDocument()
    expect(screen.queryByText('главная')).not.toBeInTheDocument()
  })
})
