// api/depts.ts
import { api } from './client'
import type { Department } from './types'

export const tree = () => api<Department[]>('/departments')

export const create = (body: { name: string; parent_id: number | null; sort: number }) =>
  api<Department>('/departments', { method: 'POST', body })

export const update = (id: number, body: { name: string; sort: number }) =>
  api<Department>(`/departments/${id}`, { method: 'PUT', body })

export const remove = (id: number) => api<null>(`/departments/${id}`, { method: 'DELETE' })
