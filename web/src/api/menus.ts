import { api } from './client'

export interface Menu {
  id: number
  name: string
  desc: string
  sort: number
}

export interface MenuItem {
  id: number
  menu_id: number
  page_id: number
  name: string
  sort: number
}

export const listMenus = () => api<Menu[]>('/menus')

export const createMenu = (body: { name: string; desc: string; sort: number }) =>
  api<Menu>('/menus', { method: 'POST', body })

export const updateMenu = (id: number, body: { name?: string; desc?: string; sort?: number }) =>
  api<Menu>(`/menus/${id}`, { method: 'PUT', body })

export const removeMenu = (id: number) => api<null>(`/menus/${id}`, { method: 'DELETE' })

export const listMenuItems = (mid: number) => api<MenuItem[]>(`/menus/${mid}/items`)

export const createMenuItem = (mid: number, body: { page_id: number; name: string; sort: number }) =>
  api<MenuItem>(`/menus/${mid}/items`, { method: 'POST', body })

export const updateMenuItem = (mid: number, iid: number, body: { name?: string; sort?: number; page_id?: number }) =>
  api<MenuItem>(`/menus/${mid}/items/${iid}`, { method: 'PUT', body })

export const removeMenuItem = (mid: number, iid: number) =>
  api<null>(`/menus/${mid}/items/${iid}`, { method: 'DELETE' })
