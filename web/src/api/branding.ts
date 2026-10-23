import { api, getToken } from './client'

export interface Branding {
  system_name: string
  browser_title: string
  logo_url: string | null
  favicon_url: string | null
}

export const getBranding = () => api<Branding>('/branding')

export const updateBranding = (body: { system_name: string; browser_title: string }) =>
  api<Branding>('/branding', { method: 'PUT', body })

export const deleteBrandingImage = (kind: 'logo' | 'favicon') =>
  api<null>(`/branding/${kind}`, { method: 'DELETE' })

export const resetBranding = () => api<null>('/branding', { method: 'DELETE' })

export async function uploadBrandingImage(kind: 'logo' | 'favicon', file: File): Promise<Branding> {
  const fd = new FormData()
  fd.append('file', file)
  const headers: Record<string, string> = {}
  const token = getToken()
  if (token) headers.Authorization = `Bearer ${token}`
  const res = await fetch(`/api/v1/branding/${kind}`, { method: 'POST', headers, body: fd })
  const json = await res.json().catch(() => ({ code: res.status, message: res.statusText }))
  if (!res.ok || json.code !== 0) {
    throw new Error(json.message ?? '上传失败')
  }
  return json.data as Branding
}
