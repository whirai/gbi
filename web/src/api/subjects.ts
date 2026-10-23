import { api, getToken } from './client'
import type { Subject } from './types'

export interface ColumnDef {
  name: string
  type: string
  comment: string
}

export interface TableDTO {
  id: number
  subject_id: number
  name: string
  kind: string
  columns: ColumnDef[]
}

export interface RowsPage {
  columns: string[]
  total: number
  rows: Record<string, unknown>[] // 每行含 _rid
}

// 服务端列筛选:列名 → 候选值数组(IN 语义,多列 AND)。
export type RowsFilters = Record<string, string[]>

export interface QueryResult {
  columns: string[]
  rows: unknown[][]
}

export interface ImportResult {
  table_id: number
  inserted: number
}

export const listSubjects = () => api<Subject[]>('/subjects')

export const createSubject = (body: { name: string; code: string; desc: string }) =>
  api<Subject>('/subjects', { method: 'POST', body })

export const updateSubject = (id: number, body: { name: string; desc: string }) =>
  api<Subject>(`/subjects/${id}`, { method: 'PUT', body })

export const removeSubject = (id: number) => api<null>(`/subjects/${id}`, { method: 'DELETE' })

export const listTables = (sid: number) => api<TableDTO[]>(`/subjects/${sid}/tables`)

export const createTable = (sid: number, body: { name: string; kind: string; columns: ColumnDef[] }) =>
  api<TableDTO>(`/subjects/${sid}/tables`, { method: 'POST', body })

export const removeTable = (sid: number, tid: number) =>
  api<null>(`/subjects/${sid}/tables/${tid}`, { method: 'DELETE' })

export const listRows = (sid: number, tid: number, page: number, size: number, filters: RowsFilters = {}) => {
  const q = new URLSearchParams({ page: String(page), size: String(size) })
  if (Object.keys(filters).length > 0) q.set('filters', JSON.stringify(filters))
  return api<RowsPage>(`/subjects/${sid}/tables/${tid}/rows?${q.toString()}`)
}

export const listColumnValues = (sid: number, tid: number, column: string) =>
  api<string[]>(`/subjects/${sid}/tables/${tid}/columns/${encodeURIComponent(column)}/values`)

export const updateCell = (sid: number, tid: number, rowid: number, column: string, value: unknown) =>
  api<null>(`/subjects/${sid}/tables/${tid}/rows/${rowid}`, { method: 'PUT', body: { column, value } })

export const deleteRow = (sid: number, tid: number, rowid: number) =>
  api<null>(`/subjects/${sid}/tables/${tid}/rows/${rowid}`, { method: 'DELETE' })

export async function importExcel(
  sid: number,
  file: File,
  params: Record<string, string>,
): Promise<ImportResult> {
  const fd = new FormData()
  fd.append('file', file)
  for (const [k, v] of Object.entries(params)) fd.append(k, v)
  const headers: Record<string, string> = {}
  const token = getToken()
  if (token) headers.Authorization = `Bearer ${token}`
  const res = await fetch(`/api/v1/subjects/${sid}/import`, { method: 'POST', headers, body: fd })
  const json = await res.json().catch(() => ({ code: res.status, message: res.statusText }))
  if (!res.ok || json.code !== 0) {
    throw new Error(json.message ?? '导入失败')
  }
  return json.data as ImportResult
}

export const runQuery = (body: { sql: string; subject_ids: number[] }) =>
  api<QueryResult>('/query', { method: 'POST', body })
