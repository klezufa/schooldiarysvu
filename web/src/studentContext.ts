import { createContext, useContext } from 'react'
import type { Student } from './api/types'

export const StudentContext = createContext<{ studentId?: string; children: Student[]; setStudentId: (id: string) => void; isLoading: boolean } | null>(null)

export function useStudent(): NonNullable<React.ContextType<typeof StudentContext>> {
  const value = useContext(StudentContext)
  if (!value) throw new Error('useStudent must be used within StudentProvider')
  return value
}
