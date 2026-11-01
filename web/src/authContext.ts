import { createContext, useContext } from 'react'
import type { User } from './api/types'

export type LoginInput = { login: string; password: string }
export type PasswordInput = { old_password: string; new_password: string }
export type AuthValue = {
  user: User | null
  isLoading: boolean
  login: (input: LoginInput) => Promise<User>
  logout: () => Promise<void>
  changePassword: (input: PasswordInput) => Promise<void>
  hasRole: (...roles: User['roles'][number][]) => boolean
}

export const AuthContext = createContext<AuthValue | null>(null)

export function useAuth(): AuthValue {
  const value = useContext(AuthContext)
  if (!value) throw new Error('useAuth must be used within AuthProvider')
  return value
}

