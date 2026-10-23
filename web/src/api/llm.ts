import { api } from './client'

export interface LLMProvider {
  id: number
  name: string
  base_url: string
  api_key_masked: string
  has_key: boolean
  model_count: number
}

export interface LLMModel {
  id: number
  provider_id: number
  name: string
  context_window: number
  max_tokens: number
  is_current: boolean
}

export interface LLMCurrent {
  provider: { id: number; name: string; base_url: string; has_key: boolean } | null
  model: LLMModel | null
}

export const listProviders = () => api<LLMProvider[]>('/llm/providers')
export const createProvider = (body: { name: string; base_url: string; api_key?: string }) =>
  api<LLMProvider>('/llm/providers', { method: 'POST', body })
export const updateProvider = (id: number, body: { name?: string; base_url?: string; api_key?: string }) =>
  api<LLMProvider>(`/llm/providers/${id}`, { method: 'PUT', body })
export const deleteProvider = (id: number) => api<null>(`/llm/providers/${id}`, { method: 'DELETE' })
export const listModels = (pid: number) => api<LLMModel[]>(`/llm/providers/${pid}/models`)
export const createModel = (pid: number, body: { name: string; context_window: number; max_tokens: number }) =>
  api<LLMModel>(`/llm/providers/${pid}/models`, { method: 'POST', body })
export const updateModel = (id: number, body: { name: string; context_window: number; max_tokens: number }) =>
  api<LLMModel>(`/llm/models/${id}`, { method: 'PUT', body })
export const deleteModel = (id: number) => api<null>(`/llm/models/${id}`, { method: 'DELETE' })
export const setCurrentModel = (id: number) => api<null>(`/llm/models/${id}/current`, { method: 'POST' })
export const currentModel = () => api<LLMCurrent>('/llm/current')
