// api/users.ts
import { api } from './client'
import type { PageData, User } from './types'

export const list = (page: number, size: number, keyword: string) =>
  api<PageData<User>>(`/users?page=${page}&size=${size}&keyword=${encodeURIComponent(keyword)}`)

// 后端单页上限 100,选择器场景需要全量用户:循环分页拉取直到覆盖 total
export const listAllUsers = async (): Promise<User[]> => {
  const all: User[] = []
  for (let page = 1; ; page++) {
    const res = await list(page, 100, '')
    all.push(...res.items)
    if (res.items.length === 0 || all.length >= res.total) break
  }
  return all
}

export const create = (body: {
  username: string
  password: string
  name: string
  dept_id: number | null
  role_id: number
}) => api<User>('/users', { method: 'POST', body })

export const update = (
  id: number,
  body: { name: string; dept_id: number | null; role_id: number; status: string },
) => api<User>(`/users/${id}`, { method: 'PUT', body })

export const remove = (id: number) => api<null>(`/users/${id}`, { method: 'DELETE' })

export const resetPassword = (id: number, password: string) =>
  api<null>(`/users/${id}/password`, { method: 'POST', body: { password } })
