import { describe, expect, it } from 'vitest'
import { appendAssistantDelta, type ChatItem } from './chatItems'

describe('appendAssistantDelta', () => {
  it('末项是 assistant 时追加文本(常规流式)', () => {
    const items: ChatItem[] = [
      { role: 'user', text: '画图' },
      { role: 'assistant', text: '好的' },
    ]
    const next = appendAssistantDelta(items, ',开始处理')
    expect(next).toHaveLength(2)
    expect(next[1]).toEqual({ role: 'assistant', text: '好的,开始处理' })
  })

  it('tool_call 条目之后到来的文本仍被渲染(推入新 assistant 项)', () => {
    // 复现真实时序:send 时推入 assistant 占位 → tool_call 事件追加 tool 项 →
    // 最终轮的 message_delta 到来时末项是 tool,增量不得丢弃
    const items: ChatItem[] = [
      { role: 'user', text: '按区域统计销售额' },
      { role: 'assistant', text: '' },
      { role: 'tool', text: '调用工具:get_schema' },
      { role: 'tool', text: '调用工具:update_page' },
    ]
    const next = appendAssistantDelta(items, '已为您生成图表。')
    expect(next).toHaveLength(5)
    expect(next[4]).toEqual({ role: 'assistant', text: '已为您生成图表。' })
    // 后续增量继续追加到该新 assistant 项
    const next2 = appendAssistantDelta(next, '完成')
    expect(next2[4]).toEqual({ role: 'assistant', text: '已为您生成图表。完成' })
  })

  it('空列表推入 assistant 项', () => {
    const next = appendAssistantDelta([], '你好')
    expect(next).toEqual([{ role: 'assistant', text: '你好' }])
  })

  it('纯函数:不 mutate 入参(StrictMode 双调用安全)', () => {
    const items: ChatItem[] = [
      { role: 'user', text: 'q' },
      { role: 'assistant', text: 'a' },
    ]
    const snapshot = JSON.stringify(items)
    const first = appendAssistantDelta(items, 'b')
    expect(JSON.stringify(items)).toBe(snapshot) // 入参未变
    // StrictMode 会以同一 prev 双调用 updater;纯函数两次结果一致
    const second = appendAssistantDelta(items, 'b')
    expect(second).toEqual(first)
    expect(second[1]).not.toBe(items[1]) // 返回新对象而非复用旧引用
  })
})
