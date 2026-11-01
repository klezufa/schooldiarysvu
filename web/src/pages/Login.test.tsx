import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AuthProvider } from '../auth'
import { LoginPage } from './Login'

describe('login form', () => {
  it('shows localized validation instead of sending invalid credentials', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ code: 'unauthorized', message: 'Вход не выполнен', request_id: 'r' }), { status: 401, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const user = userEvent.setup()
    render(<QueryClientProvider client={client}><MemoryRouter><AuthProvider><LoginPage /></AuthProvider></MemoryRouter></QueryClientProvider>)
    const submit = await screen.findByRole('button', { name: 'Войти' })
    await user.click(submit)
    expect(await screen.findByText('Введите логин')).toBeInTheDocument()
    expect(screen.getByText('Введите пароль')).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})
