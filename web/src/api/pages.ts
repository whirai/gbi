import { api } from './client'

export interface PageSummary {
  id: number
  name: string
  desc: string
  status: string
  created_by: number
  updated_at: string
}

export interface ChartConfig {
  id: string
  title: string
  width: string
  type: string
  sql: string
  subjects: number[]
  spec: Record<string, unknown>
}

export interface PageConfig {
  version: number
  layout: string
  charts: ChartConfig[]
}

export interface PageDetail extends PageSummary {
  config: PageConfig
}

export interface PageVersion {
  id: number
  page_id: number
  version: number
  saved_by: number
  created_at: string
}

export interface ACLGrant {
  subject_type: 'user' | 'group'
  subject_id: number
  subject_name: string
}

export interface VisibleMenuItem {
  id: number
  name: string
  page_id: number
}

export interface VisibleMenu {
  id: number
  name: string
  items: VisibleMenuItem[]
}

export const listPages = () => api<PageSummary[]>('/pages')

export const createPage = (body: { name: string; desc: string }) =>
  api<PageDetail>('/pages', { method: 'POST', body })

export const getPage = (id: number) => api<PageDetail>(`/pages/${id}`)

export const updatePage = (id: number, body: { name?: string; desc?: string; config?: PageConfig }) =>
  api<PageDetail>(`/pages/${id}`, { method: 'PUT', body })

export const removePage = (id: number) => api<null>(`/pages/${id}`, { method: 'DELETE' })

export const publishPage = (id: number) => api<null>(`/pages/${id}/publish`, { method: 'POST' })

export const unpublishPage = (id: number) => api<null>(`/pages/${id}/unpublish`, { method: 'POST' })

export const listVersions = (id: number) => api<PageVersion[]>(`/pages/${id}/versions`)

export const restoreVersion = (id: number, vid: number) =>
  api<null>(`/pages/${id}/versions/${vid}/restore`, { method: 'POST' })

export const getACL = (id: number) => api<ACLGrant[]>(`/pages/${id}/acl`)

export const setACL = (id: number, grants: { subject_type: string; subject_id: number }[]) =>
  api<null>(`/pages/${id}/acl`, { method: 'PUT', body: { grants } })

export const portalPages = () => api<PageSummary[]>('/portal/pages')

export const portalMenus = () => api<VisibleMenu[]>('/portal/menus')

// reportRenderError 上报单图渲染错误(静默:失败不影响渲染)。
export const reportRenderError = (pageId: number, chartId: string, message: string) =>
  api<void>(`/pages/${pageId}/render-errors`, { method: 'POST', body: { chart_id: chartId, message } }).catch(() => undefined)
