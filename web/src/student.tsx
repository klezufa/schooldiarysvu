import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from './api/client'
import type { Student } from './api/types'
import { useAuth } from './authContext'
import { saveSelectedChild, selectedChild, studentScope } from './lib/student'
import { StudentContext } from './studentContext'

export function StudentProvider({ children }: { children: ReactNode }): React.JSX.Element {
  const { user } = useAuth()
  const isParent = Boolean(user?.roles.includes('parent'))
  // Пока временный пароль не сменён, backend отвечает 403 на весь API, кроме /me: не запрашиваем заранее.
  const ready = isParent && !user?.must_change_password
  const query = useQuery({ queryKey: ['children'], queryFn: () => api<Student[]>('/children'), enabled: ready })
  const [selected, setSelected] = useState(selectedChild)
  useEffect(() => {
    if (!isParent || !query.data?.length) return
    if (!query.data.some((child) => child.id === selected)) {
      setSelected(query.data[0]!.id)
      saveSelectedChild(query.data[0]!.id)
    }
  }, [isParent, query.data, selected])
  const value = useMemo(() => ({
    studentId: studentScope(user, selected), children: query.data ?? [],
    setStudentId: (id: string) => { setSelected(id); saveSelectedChild(id) },
    isLoading: isParent && query.isLoading,
  }), [user, selected, query.data, isParent, query.isLoading])
  return <StudentContext.Provider value={value}>{children}</StudentContext.Provider>
}

