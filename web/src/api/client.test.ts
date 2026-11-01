import { ApiError, api, mutation, queryString } from './client'

describe('API client', () => {
  it('adds CSRF to mutations and preserves multipart bodies', async () => {
    document.cookie = 'diary_csrf=test-csrf-token; path=/'
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)
    const body = new FormData()
    body.set('file', new Blob(['test']), 'test.txt')
    await api('/files', { method: 'POST', body })
    const headers = fetchMock.mock.calls[0]?.[1].headers as Headers
    expect(headers.get('X-CSRF-Token')).toBe('test-csrf-token')
    expect(headers.has('Content-Type')).toBe(false)
  })

  it.each([401, 403, 409, 422, 429])('maps HTTP %i to a typed error', async (status) => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ code: 'test_error', message: 'Проверка', request_id: 'req-1', field_errors: { value: 'Неверно' } }), { status, headers: { 'Content-Type': 'application/json' } })))
    const promise = mutation('/test', 'POST', {})
    await expect(promise).rejects.toMatchObject({ status, code: 'test_error', requestId: 'req-1', fieldErrors: { value: 'Неверно' } })
  })

  it('does not hide network failures', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('network down')))
    await expect(api('/me')).rejects.toMatchObject({ status: 0, code: 'offline' })
  })

  it('builds scoped query strings and exposes ApiError', () => {
    expect(queryString({ student_id: 'child-1', empty: '', skipped: undefined, limit: 50 })).toBe('?student_id=child-1&limit=50')
    expect(new ApiError(409, 'version', 'Конфликт')).toBeInstanceOf(Error)
  })
})
