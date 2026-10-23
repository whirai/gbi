// api/groups.ts
import { api } from './client'
import type { Group, User } from './types'

export const list = () => api<Group[]>('/groups')

export const create = (body: { name: string; desc: string }) =>
  api<Group>('/groups', { method: 'POST', body })

export const update = (id: number, body: { name: string; desc: string }) =>
  api<Group>(`/groups/${id}`, { method: 'PUT', body })

export const remove = (id: number) => api<null>(`/groups/${id}`, { method: 'DELETE' })

export const members = (gid: number) => api<User[]>(`/groups/${gid}/members`)

export const addMembers = (gid: number, user_ids: number[]) =>
  api<null>(`/groups/${gid}/members`, { method: 'POST', body: { user_ids } })

export const removeMember = (gid: number, uid: number) =>
  api<null>(`/groups/${gid}/members/${uid}`, { method: 'DELETE' })
