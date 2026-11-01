import type { User } from '../api/types'

export function studentScope(user: User | null | undefined, selectedChild?: string): string | undefined {
  if (user?.roles.includes('parent')) return selectedChild
  return user?.student_id
}

export function scopedKey(base: string, studentId?: string, ...context: unknown[]): unknown[] {
  return [base, studentId ?? 'self', ...context]
}

export function saveSelectedChild(studentId: string): void {
  sessionStorage.setItem('diary_selected_child', studentId)
}

export function selectedChild(): string {
  return sessionStorage.getItem('diary_selected_child') ?? ''
}

export function clearPrivateDrafts(): void {
  sessionStorage.removeItem('diary_selected_child')
  Object.keys(sessionStorage)
    .filter((key) => key.startsWith('diary_draft_'))
    .forEach((key) => sessionStorage.removeItem(key))
}
