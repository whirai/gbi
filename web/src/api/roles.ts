// api/roles.ts
import { api } from './client'
import type { Role } from './types'

export const list = () => api<Role[]>('/roles')

export const rename = (id: number, name: string) =>
  api<Role>(`/roles/${id}`, { method: 'PUT', body: { name } })
