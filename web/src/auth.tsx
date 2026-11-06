import { useEffect, useMemo, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Navigate, useLocation } from 'react-router-dom'
import { api, mutation } from './api/client'
import type { LoginResponse, User } from './api/types'
import { AuthContext, useAuth, type AuthValue, type LoginInput, type PasswordInput } from './authContext'
import { clearPrivateDrafts } from './lib/student'
import { Loading } from './components/UI'

export function AuthProvider({ children }: { children: ReactNode }): React.JSX.Element {
  const queryClient = useQueryClient()
  const me = useQuery({
    queryKey: ['me'],
    queryFn: () => api<User>('/me'),
    retry: (count, error) => !(error instanceof Error && 'status' in error && [401, 403].includes(Number(error.status))) && count < 1,
    staleTime: 60_000,
  })
  const loginMutation = useMutation({
    mutationFn: (input: LoginInput) => mutation<LoginResponse>('/auth/login', 'POST', input),
    onSuccess: (response) => {
      const user = Object.fromEntries(Object.entries(response).filter(([key]) => key !== 'csrf_token')) as User
      queryClient.clear()
      queryClient.setQueryData(['me'], user)
    },
  })
  const logoutMutation = useMutation({
    mutationFn: () => mutation<void>('/auth/logout', 'POST'),
    onSettled: () => {
      clearPrivateDrafts()
      queryClient.clear()
      queryClient.setQueryData(['me'], null)
    },
  })
  const passwordMutation = useMutation({
    mutationFn: (input: PasswordInput) => mutation<void>('/auth/password', 'POST', input),
    // После смены пароля доступ к API открывается: сбои 403, закэшированные до этого, нужно перезапросить.
    onSuccess: async () => { await queryClient.invalidateQueries() },
  })
  useEffect(() => {
    const expired = () => {
      clearPrivateDrafts()
      // 401 часто приходит от самого /me, пока он ещё выполняется. Удалить его из кеша нельзя: TanStack Query
      // молча отменит запрос, и ProtectedRoute навсегда останется на «Проверяем сессию…». Поэтому удаляем
      // остальные данные, а /me помечаем как «пользователя нет» — страница сразу уходит на вход.
      queryClient.removeQueries({ predicate: (query) => query.queryKey[0] !== 'me' })
      queryClient.setQueryData(['me'], null)
    }
    window.addEventListener('diary:unauthorized', expired)
    return () => window.removeEventListener('diary:unauthorized', expired)
  }, [queryClient])
  const value = useMemo<AuthValue>(() => ({
    user: me.data ?? null,
    isLoading: me.isLoading,
    login: async (input) => {
      const response = await loginMutation.mutateAsync(input)
      return Object.fromEntries(Object.entries(response).filter(([key]) => key !== 'csrf_token')) as User
    },
    logout: () => logoutMutation.mutateAsync(),
    changePassword: (input) => passwordMutation.mutateAsync(input),
    hasRole: (...roles) => Boolean(me.data?.roles.some((role) => roles.includes(role))),
  }), [me.data, me.isLoading, loginMutation, logoutMutation, passwordMutation])
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function ProtectedRoute({ children }: { children: ReactNode }): React.JSX.Element {
  const { user, isLoading } = useAuth()
  const location = useLocation()
  if (isLoading) return <Loading label="Проверяем сессию…" />
  if (!user) return <Navigate to="/login" replace state={{ from: location.pathname }} />
  if (user.must_change_password && location.pathname !== '/password') return <Navigate to="/password" replace />
  return <>{children}</>
}
