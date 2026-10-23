// 聊天条目与其流式更新逻辑(纯函数,便于单测)。

export interface ChatItem {
  role: 'user' | 'assistant' | 'tool'
  text: string
}

// appendAssistantDelta 把文本增量追加到最后的 assistant 条目。
// 若末项不是 assistant(例如中途插入了 tool_call 条目),推入新的 assistant 条目,
// 而不是丢弃增量。返回新数组/新对象,不 mutate 入参(StrictMode 双调用安全)。
export function appendAssistantDelta(items: ChatItem[], text: string): ChatItem[] {
  const next = [...items]
  const last = next[next.length - 1]
  if (last && last.role === 'assistant') {
    next[next.length - 1] = { ...last, text: last.text + text }
  } else {
    next.push({ role: 'assistant', text })
  }
  return next
}
