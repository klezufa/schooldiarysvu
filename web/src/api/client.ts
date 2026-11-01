export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly requestId?: string,
    public readonly fieldErrors: Record<string, string> = {},
    public readonly retryAfter?: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export type QueryParams = Record<string, string | number | boolean | null | undefined>

export function queryString(params: QueryParams): string {
  const search = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') search.set(key, String(value))
  })
  const result = search.toString()
  return result ? `?${result}` : ''
}

export function csrfToken(): string {
  if (typeof document === 'undefined') return ''
  const item = document.cookie.split('; ').find((cookie) => cookie.startsWith('diary_csrf='))
  return item ? decodeURIComponent(item.slice('diary_csrf='.length)) : ''
}

function jsonHeaders(init?: RequestInit): Headers {
  const headers = new Headers(init?.headers)
  if (init?.body && !(init.body instanceof FormData) && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  if (init?.method && !['GET', 'HEAD', 'OPTIONS'].includes(init.method.toUpperCase())) {
    headers.set('X-CSRF-Token', csrfToken())
  }
  return headers
}

async function parseError(response: Response): Promise<ApiError> {
  const body = await response.json().catch(() => ({})) as {
    code?: string; message?: string; request_id?: string; field_errors?: Record<string, string>
  }
  const fallback: Record<number, string> = {
    401: 'Сессия истекла. Войдите снова.',
    403: 'У вас нет доступа к этому действию или данным.',
    409: 'Данные изменились. Проверьте серверную версию перед сохранением.',
    422: 'Проверьте правильность заполнения полей.',
    429: 'Слишком много запросов. Повторите попытку позже.',
  }
  return new ApiError(
    response.status,
    body.code ?? 'http_error',
    body.message ?? fallback[response.status] ?? 'Ошибка сервера. Попробуйте ещё раз.',
    body.request_id,
    body.field_errors,
    response.headers.get('Retry-After') ?? undefined,
  )
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  let response: Response
  try {
    response = await fetch(`/api/v1${path}`, {
      ...init,
      credentials: 'same-origin',
      headers: jsonHeaders(init),
    })
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') throw error
    throw new ApiError(0, 'offline', 'Нет соединения с сервером. Введённые данные не отправлены.')
  }
  if (!response.ok) {
    const error = await parseError(response)
    if (response.status === 401 && path !== '/auth/login' && typeof window !== 'undefined') {
      window.dispatchEvent(new CustomEvent('diary:unauthorized'))
    }
    throw error
  }
  if (response.status === 204) return undefined as T
  return await response.json() as T
}

export function mutation<T>(path: string, method: 'POST' | 'PUT' | 'PATCH' | 'DELETE', body?: unknown, headers?: HeadersInit): Promise<T> {
  return api<T>(path, {
    method,
    body: body === undefined ? undefined : JSON.stringify(body),
    headers,
  })
}

export async function download(path: string): Promise<{ blob: Blob; filename: string }> {
  let response: Response
  try {
    response = await fetch(`/api/v1${path}`, { credentials: 'same-origin' })
  } catch {
    throw new ApiError(0, 'offline', 'Нет соединения с сервером.')
  }
  if (!response.ok) throw await parseError(response)
  const disposition = response.headers.get('Content-Disposition') ?? ''
  const encoded = disposition.match(/filename\*=UTF-8''([^;]+)/i)?.[1]
  const plain = disposition.match(/filename="?([^";]+)"?/i)?.[1]
  const filename = encoded ? decodeURIComponent(encoded) : plain ?? 'export.csv'
  return { blob: await response.blob(), filename }
}

export function saveBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(url)
}

/** Страница списка и общее число записей из заголовка X-Total-Count. */
export async function apiPage<T>(path: string): Promise<{ items: T[]; total: number }> {
  let response: Response
  try {
    response = await fetch(`/api/v1${path}`, { credentials: 'same-origin' })
  } catch {
    throw new ApiError(0, 'offline', 'Нет соединения с сервером.')
  }
  if (!response.ok) throw await parseError(response)
  const items = await response.json() as T[]
  const total = Number(response.headers.get('X-Total-Count') ?? items.length)
  return { items, total: Number.isFinite(total) ? total : items.length }
}
