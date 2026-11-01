import { clearPrivateDrafts } from './student'

const PREFIX = 'diary_draft_'
const SHARED_COMPUTER_KEY = 'diary_shared_computer'
/** Несохранённый ввод живёт недолго: черновик старше этого срока считается устаревшим и удаляется. */
export const DRAFT_TTL_MS = 2 * 60 * 60 * 1000

type StoredDraft = { value: string; expires: number }

/** Режим «общий компьютер» выключает любое временное хранение; при сбое хранилища черновики тоже отключены. */
export function draftsEnabled(): boolean {
  try { return localStorage.getItem(SHARED_COMPUTER_KEY) !== 'true' } catch { return false }
}

export function setSharedComputer(enabled: boolean): void {
  try { localStorage.setItem(SHARED_COMPUTER_KEY, String(enabled)) } catch { /* хранилище недоступно: черновики и так не сохраняются */ }
  if (enabled) clearPrivateDrafts()
}

export function saveDraft(name: string, value: string, now = Date.now()): void {
  if (!draftsEnabled()) return
  try {
    if (!value.trim()) sessionStorage.removeItem(PREFIX + name)
    else sessionStorage.setItem(PREFIX + name, JSON.stringify({ value, expires: now + DRAFT_TTL_MS } satisfies StoredDraft))
  } catch { /* переполнение или запрет хранилища: ввод остаётся только в памяти формы */ }
}

export function loadDraft(name: string, now = Date.now()): string {
  if (!draftsEnabled()) return ''
  try {
    const raw = sessionStorage.getItem(PREFIX + name)
    if (!raw) return ''
    const stored = JSON.parse(raw) as Partial<StoredDraft>
    if (typeof stored.value !== 'string' || typeof stored.expires !== 'number' || stored.expires <= now) { sessionStorage.removeItem(PREFIX + name); return '' }
    return stored.value
  } catch { return '' }
}

export function removeDraft(name: string): void {
  try { sessionStorage.removeItem(PREFIX + name) } catch { /* нечего удалять */ }
}
