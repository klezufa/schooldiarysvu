import { createContext, useContext } from 'react'
import type { Student } from './api/types'

/** noChildren — родитель, у которого список детей загружен и пуст: администратор ещё не связал его с ребёнком. */
export const StudentContext = createContext<{ studentId?: string; children: Student[]; setStudentId: (id: string) => void; isLoading: boolean; noChildren?: boolean } | null>(null)

export function useStudent(): NonNullable<React.ContextType<typeof StudentContext>> {
  const value = useContext(StudentContext)
  if (!value) throw new Error('useStudent must be used within StudentProvider')
  return value
}
