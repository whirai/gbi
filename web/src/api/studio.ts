import { api, getToken } from './client'

export interface StudioMessage {
  id: number
  conversation_id: number
  role: 'user' | 'assistant'
  content: string
  created_at: string
}

export interface ConversationResp {
  conversation: { id: number; page_id: number; title: string }
  messages: StudioMessage[]
}

export const getConversation = (pageId: number) =>
  api<ConversationResp>(`/studio/conversations/${pageId}`)

export const deleteConversation = (pageId: number) =>
  api<void>(`/studio/conversations/${pageId}`, { method: 'DELETE' })

export type StudioEventType = 'message_delta' | 'tool_call' | 'config_updated' | 'done' | 'error'

// chatStream 解析 SSE,逐事件回调;返回 Promise(resolve 于 done/error)。
export async function chatStream(
  pageId: number,
  subjectIds: number[],
  message: string,
  onEvent: (type: StudioEventType, data: string) => void,
): Promise<void> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  const token = getToken()
  if (token) headers.Authorization = `Bearer ${token}`
  const res = await fetch('/api/v1/studio/chat', {
    method: 'POST',
    headers,
    body: JSON.stringify({ page_id: pageId, subject_ids: subjectIds, message }),
  })
  if (!res.ok || !res.body) {
    const j = await res.json().catch(() => ({ message: res.statusText }))
    throw new Error(j.message ?? '请求失败')
  }
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buf = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buf += decoder.decode(value, { stream: true })
    // SSE 以空行分隔事件
    let idx: number
    while ((idx = buf.indexOf('\n\n')) >= 0) {
      const raw = buf.slice(0, idx)
      buf = buf.slice(idx + 2)
      let event = 'message'
      let data = ''
      for (const line of raw.split('\n')) {
        if (line.startsWith('event: ')) event = line.slice(7).trim()
        else if (line.startsWith('data: ')) data += line.slice(6)
      }
      onEvent(event as StudioEventType, data)
    }
  }
}
