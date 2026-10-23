import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, waitFor } from '@testing-library/react'
import G2Chart from './G2Chart'

// jsdom 缺 matchMedia,antd 响应式组件需要(与 Portal.test.tsx 同款 shim)
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
})

// Chart 构造发生在 runQuery 的微任务 .then 里,测试无法在 render 前拿到实例,
// 故将 render 行为提升为共享可控 mock(按 vitest 惯例在 beforeEach 重建)。
const { Chart, chartRender, reportRenderError } = vi.hoisted(() => {
  const chartRender = vi.fn(async () => undefined)
  // 须为可构造的普通函数:箭头函数不能作为构造函数(new Chart)
  const Chart = vi.fn(function () {
    return { options: vi.fn(), render: chartRender, destroy: vi.fn() }
  })
  return { Chart, chartRender, reportRenderError: vi.fn(async () => undefined) }
})

vi.mock('../api/pages', async (orig) => ({ ...(await orig()), reportRenderError }))
vi.mock('../api/subjects', () => ({
  runQuery: vi.fn(async () => ({ columns: ['x', 'y'], rows: [[1, 2]] })),
}))
vi.mock('@antv/g2', () => ({ Chart }))

// 文件级 beforeEach:对文件内两个 describe(含并发/卸载用例)统一生效
beforeEach(() => {
  chartRender.mockReset()
  chartRender.mockImplementation(async () => undefined)
  // 并发/卸载用例依赖调用计数与「未被调用」断言,需清空跨用例累积
  reportRenderError.mockClear()
})

describe('G2Chart 渲染错误捕获', () => {
  it('render reject 时上报图表 id 与错误消息', async () => {
    chartRender.mockRejectedValueOnce(new Error('boom'))
    render(
      <G2Chart
        pageId={4}
        chart={{ id: 'c1', title: 't', width: 'full', type: 'interval', sql: 'SELECT 1', subjects: [2], spec: { type: 'interval' } }}
      />,
    )
    await waitFor(() => expect(reportRenderError).toHaveBeenCalledWith(4, 'c1', 'boom'))
  })

  it('渲染窗口内 console.error 归入该图且 console 原样还原', async () => {
    const orig = console.error
    chartRender.mockImplementationOnce(async () => {
      console.error('Unknown Component: labelTransform.overall')
    })
    render(
      <G2Chart
        pageId={4}
        chart={{ id: 'c2', title: 't', width: 'full', type: 'interval', sql: 'SELECT 1', subjects: [2], spec: { type: 'interval' } }}
      />,
    )
    await waitFor(() =>
      expect(reportRenderError).toHaveBeenCalledWith(4, 'c2', expect.stringContaining('labelTransform')),
    )
    expect(console.error).toBe(orig)
  })

  it('同一 chartId+message 不同 pageId 5 秒内不互相抑制(去重键含 pageId)', async () => {
    chartRender.mockImplementation(async () => {
      console.error('dup-msg-p2')
    })
    const chart = {
      id: 'c1', title: 't', width: 'full', type: 'interval', sql: 'SELECT 1', subjects: [2], spec: { type: 'interval' },
    }
    render(<G2Chart pageId={4} chart={chart} />)
    render(<G2Chart pageId={5} chart={chart} />)
    await waitFor(() => expect(reportRenderError).toHaveBeenCalledWith(4, 'c1', 'dup-msg-p2'))
    await waitFor(() => expect(reportRenderError).toHaveBeenCalledWith(5, 'c1', 'dup-msg-p2'))
  })
})

describe('G2Chart 并发与卸载竞态', () => {
  it('两个 G2Chart 并发渲染可组合:重叠窗口消息归最新渲染,结束后 console 还原原生引用', async () => {
    const nativeError = console.error
    const nativeWarn = console.warn
    let resolveA!: () => void
    let resolveB!: () => void
    // 手动控制完成顺序:B(后开始)先完成、A 后完成,以验证链式还原
    chartRender.mockImplementationOnce(() => new Promise<undefined>((res) => { resolveA = () => res(undefined) }))
    chartRender.mockImplementationOnce(() => new Promise<undefined>((res) => { resolveB = () => res(undefined) }))
    const chart = (id: string) => ({
      id, title: 't', width: 'full', type: 'interval', sql: 'SELECT 1', subjects: [2], spec: { type: 'interval' },
    })
    render(<G2Chart pageId={4} chart={chart('cA')} />)
    render(<G2Chart pageId={4} chart={chart('cB')} />)
    // 两个 captureRender 的拦截器均已就位(chartRender 在 await 之前同步被调用)
    await waitFor(() => expect(chartRender).toHaveBeenCalledTimes(2))
    // 重叠窗口内 console.error 只归属最新开始的渲染 B(最内层),不双重捕获
    console.error('overlap-msg')
    resolveB()
    await waitFor(() => expect(reportRenderError).toHaveBeenCalledWith(4, 'cB', 'overlap-msg'))
    // B 完成后还原的是 A 的拦截器,A 的捕获窗口继续生效
    console.error('a-window-msg')
    resolveA()
    await waitFor(() => expect(reportRenderError).toHaveBeenCalledWith(4, 'cA', 'a-window-msg'))
    expect(reportRenderError).not.toHaveBeenCalledWith(4, 'cA', 'overlap-msg')
    expect(reportRenderError.mock.calls.filter((c) => (c as unknown[])[2] === 'overlap-msg')).toHaveLength(1)
    // 两个渲染全部结束后 console 还原为原生引用
    expect(console.error).toBe(nativeError)
    expect(console.warn).toBe(nativeWarn)
  })

  it('重叠窗口非 LIFO 完成(先开始的先完成):不破坏后开始者的捕获,全部结束后 console 还原原生', async () => {
    const nativeError = console.error
    const nativeWarn = console.warn
    let resolveA!: () => void
    let resolveB!: () => void
    // 手动控制完成顺序:A(先开始)先完成、B 后完成 —— 与旧实现在非 LIFO 下的
    // bug 场景一致(A 还原会覆盖 B 的包装并把包装永久残留)
    chartRender.mockImplementationOnce(() => new Promise<undefined>((res) => { resolveA = () => res(undefined) }))
    chartRender.mockImplementationOnce(() => new Promise<undefined>((res) => { resolveB = () => res(undefined) }))
    const chart = (id: string) => ({
      id, title: 't', width: 'full', type: 'interval', sql: 'SELECT 1', subjects: [2], spec: { type: 'interval' },
    })
    render(<G2Chart pageId={4} chart={chart('cA')} />)
    render(<G2Chart pageId={4} chart={chart('cB')} />)
    await waitFor(() => expect(chartRender).toHaveBeenCalledTimes(2))
    resolveA()
    // 等 A 的 finally 完成(B 仍活跃):包装必须仍在,否则 B 的剩余消息会丢失
    await new Promise((r) => setTimeout(r, 0))
    expect(console.error).not.toBe(nativeError)
    console.error('b-after-a-done')
    resolveB()
    await waitFor(() => expect(reportRenderError).toHaveBeenCalledWith(4, 'cB', 'b-after-a-done'))
    // 全部渲染结束后必须还原原生引用,且不残留包装(A 无消息,不产生额外上报)
    expect(console.error).toBe(nativeError)
    expect(console.warn).toBe(nativeWarn)
    expect(reportRenderError).toHaveBeenCalledTimes(1)
  })

  it('卸载后 render 的 rejection 不上报(卸载竞态不误报用户未看到的图表)', async () => {
    let rejectRender!: (e: Error) => void
    chartRender.mockImplementationOnce(() => new Promise<undefined>((_, reject) => { rejectRender = reject }))
    const { unmount } = render(
      <G2Chart
        pageId={4}
        chart={{ id: 'c3', title: 't', width: 'full', type: 'interval', sql: 'SELECT 1', subjects: [2], spec: { type: 'interval' } }}
      />,
    )
    await waitFor(() => expect(chartRender).toHaveBeenCalled())
    unmount() // cleanup:disposed=true + inst.destroy()
    rejectRender(new Error('destroy during render'))
    await new Promise((r) => setTimeout(r, 30))
    expect(reportRenderError).not.toHaveBeenCalled()
  })
})
