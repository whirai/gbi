import { Chart } from '@antv/g2'
import { useEffect, useRef, useState } from 'react'
import type { ChartConfig } from '../api/pages'
import { reportRenderError } from '../api/pages'
import { runQuery } from '../api/subjects'

const lastReported = new Map<string, number>()
const DEDUPE_MS = 5000

// reportOnce 同一 (pageId+chartId+message) 5 秒内不重复上报;
// 键含 pageId,两页同 chartId 同 message 不互相抑制。
function reportOnce(pageId: number, chartId: string, message: string) {
  const key = pageId + '|' + chartId + '|' + message
  const now = Date.now()
  // 顺带淘汰过期键:键含完整错误消息,长会话下 Map 不能无界增长
  for (const [k, ts] of lastReported) {
    if (now - ts >= DEDUPE_MS) lastReported.delete(k)
  }
  const prev = lastReported.get(key)
  if (prev !== undefined && now - prev < DEDUPE_MS) return
  lastReported.set(key, now)
  void reportRenderError(pageId, chartId, message)
}

// 模块级保存原生 console 引用:包装只直呼原生(绝不调用其它包装),
// 避免双重捕获/双重归属。
const nativeError = console.error
const nativeWarn = console.warn

type ConsoleSink = (message: string) => void

// 模块级 sink 集合 + 引用计数:首个 sink 安装包装,最后一个 sink 移除时恢复原生。
// 与旧「逐个保存/还原 console 引用」不同,释放只按集合是否为空判定,
// 重叠窗口以任意顺序完成都不会互相覆盖包装、也不会残留包装(最终必为原生引用)。
// 分发只给最后注册(最新开始、最内层)的 sink:重叠窗口的消息归最新渲染,
// 外层窗口不重复归属。
const sinks = new Set<ConsoleSink>()

function dispatch(args: unknown[], native: (...a: unknown[]) => void) {
  const list = [...sinks]
  list[list.length - 1]?.(args.map(String).join(' '))
  native(...args)
}

// addSink 注册捕获 sink 并返回释放函数;释放幂等。
function addSink(sink: ConsoleSink): () => void {
  sinks.add(sink)
  if (sinks.size === 1) {
    console.error = (...a: unknown[]) => dispatch(a, nativeError)
    console.warn = (...a: unknown[]) => dispatch(a, nativeWarn)
  }
  let released = false
  return () => {
    if (released) return
    released = true
    sinks.delete(sink)
    if (sinks.size === 0) {
      console.error = nativeError
      console.warn = nativeWarn
    }
  }
}

// captureRender 双信号捕获:render() 失败 + 渲染窗口内 console.error/warn,
// 全部归属到该图表。窗口消息经模块级 sink 集合收集,重叠窗口只归最新开始的渲染。
async function captureRender(
  inst: Chart,
  pageId: number,
  chartId: string,
  isCancelled: () => boolean,
): Promise<void> {
  const captured: string[] = []
  const release = addSink((message) => captured.push(message))
  try {
    await inst.render()
  } catch (e) {
    // 卸载竞态:destroy 引发的 rejection 非真实渲染错误,不上报
    if (!isCancelled()) captured.push(e instanceof Error ? e.message : String(e))
  } finally {
    release()
  }
  // 卸载后图表用户从未看到,其窗口内捕获的消息一并丢弃
  if (isCancelled()) return
  for (const m of captured) {
    reportOnce(pageId, chartId, m)
  }
}

// G2Chart 取数(只读查询 API)并把行数据注入 G2 spec 渲染;
// 渲染错误(render reject + console 窗口)双信号捕获并按图归属上报。
export default function G2Chart({ chart, pageId }: { chart: ChartConfig; pageId: number }) {
  const ref = useRef<HTMLDivElement>(null)
  const [error, setError] = useState<string | null>(null)
  const [empty, setEmpty] = useState(false)

  useEffect(() => {
    // 取消标志为每次 effect 的局部变量:旧 run 迟到的 runQuery 结果直接丢弃,
    // 不会把陈旧数据渲染进同一容器,旧实例也不会漏 destroy(cleanup 执行时它尚未创建)。
    let cancelled = false
    let inst: Chart | null = null
    setError(null)
    setEmpty(false)
    runQuery({ sql: chart.sql, subject_ids: chart.subjects })
      .then((res) => {
        if (cancelled || !ref.current) return
        const data = res.rows.map((row) => {
          const obj: Record<string, unknown> = {}
          res.columns.forEach((c, i) => {
            obj[c] = row[i]
          })
          return obj
        })
        if (data.length === 0) {
          setEmpty(true)
          return
        }
        inst = new Chart({ container: ref.current, autoFit: true })
        inst.options({ ...chart.spec, data })
        void captureRender(inst, pageId, chart.id, () => cancelled)
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e))
      })
    return () => {
      cancelled = true
      inst?.destroy()
    }
  }, [chart, pageId])

  return (
    <div style={{ height: '100%', minHeight: 220, position: 'relative' }}>
      {error && <div style={{ color: '#ef4444', padding: 12 }}>图表加载失败:{error}</div>}
      {empty && <div style={{ color: '#64748b', padding: 12 }}>暂无数据</div>}
      <div ref={ref} style={{ width: '100%', height: '100%' }} />
    </div>
  )
}
