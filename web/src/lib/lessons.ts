import type { Lesson } from '../api/types'

export function lessonTone(status: Lesson['status']): 'neutral' | 'success' | 'warning' | 'danger' | 'info' {
  return status === 'cancelled' ? 'danger' : status === 'completed' ? 'success' : 'info'
}

