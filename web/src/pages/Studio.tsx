import { useCallback, useEffect, useRef, useState } from 'react'
import { Button, Input, Popconfirm, Select, message } from 'antd'
import { SendOutlined } from '@ant-design/icons'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { listSubjects } from '../api/subjects'
import type { Subject } from '../api/types'
import { chatStream, deleteConversation, getConversation } from '../api/studio'
import PageView from '../components/PageView'
import { appendAssistantDelta, type ChatItem } from './chatItems'

export default function Studio() {
  const nav = useNavigate()
  const [params] = useSearchParams()
  const pageId = Number(params.get('page'))

  const [subjects, setSubjects] = useState<Subject[]>([])
  const [selectedSubjects, setSelectedSubjects] = useState<number[]>([])
  const [items, setItems] = useState<ChatItem[]>([])
  const [input, setInput] = useState('')
  const [busy, setBusy] = useState(false)
  const [refreshKey, setRefreshKey] = useState(0)
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    // 切页先清空,避免旧页会话消息残留;取消守卫阻止迟到响应写入新页
    setItems([])
    if (!pageId) return
    let active = true
    listSubjects()
      .then((s) => {
        if (active) setSubjects(s)
      })
      .catch((e) => message.error((e as Error).message))
    getConversation(pageId)
      .then((resp) => {
        if (active) setItems(resp.messages.map((m) => ({ role: m.role, text: m.content })))
      })
      .catch(() => undefined)
    return () => {
      active = false
    }
  }, [pageId])

  useEffect(() => {
    // jsdom 无 scrollIntoView(测试环境),可选调用兜底
    bottomRef.current?.scrollIntoView?.({ behavior: 'smooth' })
  }, [items])

  const send = useCallback(async () => {
    const msg = input.trim()
    if (!msg || busy) return
    if (!pageId) {
      message.error('缺少页面参数(?page=<id>)')
      return
    }
    if (selectedSubjects.length === 0) {
      message.error('请先选择数据集(至少一个数据主题)')
      return
    }
    setInput('')
    setBusy(true)
    setItems((prev) => [...prev, { role: 'user', text: msg }, { role: 'assistant', text: '' }])
    try {
      await chatStream(pageId, selectedSubjects, msg, (type, data) => {
        if (type === 'message_delta') {
          try {
            const d = JSON.parse(data) as { text?: string }
            if (d.text) {
              // 末项可能已被 tool_call 条目占据 → 推入新 assistant 条目而非丢弃增量
              const delta = d.text
              setItems((prev) => appendAssistantDelta(prev, delta))
            }
          } catch {
            /* ignore malformed delta */
          }
        } else if (type === 'tool_call') {
          try {
            const d = JSON.parse(data) as { name?: string }
            setItems((prev) => [...prev, { role: 'tool', text: `调用工具:${d.name ?? ''}` }])
          } catch {
            /* ignore */
          }
        } else if (type === 'config_updated') {
          setRefreshKey((k) => k + 1)
        } else if (type === 'error') {
          try {
            const d = JSON.parse(data) as { message?: string }
            message.error(d.message ?? '生成失败')
          } catch {
            message.error('生成失败')
          }
        }
      })
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setBusy(false)
    }
  }, [input, busy, pageId, selectedSubjects])

  if (!pageId) {
    return (
      <div style={{ padding: 60, textAlign: 'center', color: '#64748b' }}>
        请从 BI 页面管理页选择页面进入工作台
      </div>
    )
  }

  return (
    <div style={{ display: 'flex', height: '100vh' }}>
      {/* 左:聊天 */}
      <div
        style={{
          width: '40%',
          minWidth: 360,
          display: 'flex',
          flexDirection: 'column',
          background: 'rgba(255,255,255,0.55)',
          backdropFilter: 'blur(14px)',
          borderRight: '1px solid rgba(255,255,255,0.9)',
        }}
      >
        <div style={{ padding: 12, borderBottom: '1px solid rgba(14,165,233,0.15)' }}>
          <Button size="small" onClick={() => nav('/admin/pages')} style={{ marginBottom: 8 }}>
            ← 返回页面管理
          </Button>
          <Popconfirm title="清空当前对话?页面配置不受影响" onConfirm={async () => {
            try {
              await deleteConversation(pageId)
              setItems([])
              setRefreshKey((k) => k + 1)
              message.success('对话已清空')
            } catch (e) {
              message.error((e as Error).message)
            }
          }}>
            <Button size="small" danger style={{ marginLeft: 8 }}>新对话</Button>
          </Popconfirm>
          <div style={{ fontFamily: 'SimHei', color: '#64748b', marginBottom: 4 }}>
            数据集(数据主题)
          </div>
          <Select
            mode="multiple"
            style={{ width: '100%' }}
            placeholder="选择数据主题"
            value={selectedSubjects}
            onChange={setSelectedSubjects}
            options={subjects.map((s) => ({ value: s.id, label: `${s.name} (${s.code})` }))}
          />
        </div>
        <div style={{ flex: 1, overflowY: 'auto', padding: 12 }}>
          {items.map((it, i) => (
            <div
              key={i}
              style={{
                display: 'flex',
                justifyContent: it.role === 'user' ? 'flex-end' : 'flex-start',
                marginBottom: 8,
              }}
            >
              <div
                style={{
                  maxWidth: '85%',
                  padding: '8px 12px',
                  borderRadius: 10,
                  background:
                    it.role === 'user'
                      ? '#0ea5e9'
                      : it.role === 'tool'
                        ? 'rgba(14,165,233,0.10)'
                        : 'rgba(255,255,255,0.9)',
                  color: it.role === 'user' ? '#fff' : '#0c4a6e',
                  fontFamily: it.role === 'tool' ? 'SimHei' : undefined,
                  fontSize: it.role === 'tool' ? 12 : undefined,
                  whiteSpace: 'pre-wrap',
                }}
              >
                {it.text || (busy && it.role === 'assistant' ? '…' : '')}
              </div>
            </div>
          ))}
          <div ref={bottomRef} />
        </div>
        <div style={{ padding: 12, borderTop: '1px solid rgba(14,165,233,0.15)' }}>
          <Input.TextArea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="描述您的可视化需求,如:按月统计销售额,画折线图"
            autoSize={{ minRows: 2, maxRows: 4 }}
            onPressEnter={(e) => {
              if (!e.shiftKey) {
                e.preventDefault()
                send()
              }
            }}
          />
          <Button
            type="primary"
            icon={<SendOutlined />}
            loading={busy}
            onClick={send}
            style={{ marginTop: 8, width: '100%' }}
          >
            发送
          </Button>
        </div>
      </div>
      {/* 右:实时预览 */}
      <div style={{ flex: 1, overflowY: 'auto', padding: 16 }}>
        <PageView key={refreshKey} pageId={pageId} />
      </div>
    </div>
  )
}
