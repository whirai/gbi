import { api } from './client'
import type { MeUser } from './types'

export interface LoginResult {
  token: string
  user: { id: number; username: string; name: string }
  role_code: string
}

export const login = (username: string, password: string) =>
  api<LoginResult>('/auth/login', { method: 'POST', body: { username, password } })

export const me = () => api<MeUser>('/auth/me')

export const changePassword = (old_password: string, new_password: string) =>
  api<null>('/auth/password', { method: 'POST', body: { old_password, new_password } })
