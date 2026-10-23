import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import RowsGrid from './RowsGrid'

// jsdom 缺 matchMedia,antd 响应式组件需要
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

const { listRows, listColumnValues } = vi.hoisted(() => ({
  listRows: vi.fn(async () => ({
    columns: ['name', 'qty'],
    total: 1,
    rows: [{ _rid: 1, name: 'a', qty: 1 }],
  })),
  listColumnValues: vi.fn(async () => ['a', 'b']),
}))

vi.mock('../../../api/subjects', () => ({
  listRows,
  listColumnValues,
  updateCell: vi.fn(),
  deleteRow: vi.fn(),
}))

const table = {
  id: 7,
  subject_id: 1,
  name: 't',
  kind: 'DWD',
  columns: [
    { name: 'name', type: 'TEXT', comment: '名称' },
    { name: 'qty', type: 'INTEGER', comment: '' },
  ],
}

function headerTexts(container: HTMLElement): string[] {
  return Array.from(container.querySelectorAll('thead th')).map((th) => th.textContent ?? '')
}

describe('RowsGrid', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
  })
  afterEach(() => {
    localStorage.clear()
  })

  it('首屏按默认分页加载并渲染列', async () => {
    const { container } = render(<RowsGrid subjectId={1} table={table} />)
    await waitFor(() => expect(listRows).toHaveBeenCalledWith(1, 7, 1, 20, {}))
    await waitFor(() => expect(headerTexts(container).some((t) => t.includes('name'))).toBe(true))
  })

  it('列设置可隐藏列并持久化到 localStorage', async () => {
    const { container } = render(<RowsGrid subjectId={1} table={table} />)
    await waitFor(() => expect(headerTexts(container).some((t) => t.includes('qty'))).toBe(true))

    fireEvent.click(screen.getByRole('button', { name: /列设置/ }))
    const qtyCheckbox = await screen.findByRole('checkbox', { name: 'qty' })
    fireEvent.click(qtyCheckbox)

    await waitFor(() => expect(headerTexts(container).some((t) => t.includes('qty'))).toBe(false))
    expect(localStorage.getItem('gbi.rows_hidden.7')).toBe('["qty"]')
  })

  it('全选复选框:全不选隐藏全部数据列并持久化,再全选恢复;部分显示时半选', async () => {
    const { container } = render(<RowsGrid subjectId={1} table={table} />)
    await waitFor(() => expect(headerTexts(container).some((t) => t.includes('name'))).toBe(true))

    fireEvent.click(screen.getByRole('button', { name: /列设置/ }))
    const selectAll = (await screen.findByRole('checkbox', { name: '全选' })) as HTMLInputElement
    expect(selectAll.checked).toBe(true)
    expect(selectAll.indeterminate).toBe(false)

    // 全不选:数据列表头全部消失,锁定列(#、操作)仍在
    fireEvent.click(selectAll)
    await waitFor(() =>
      expect(headerTexts(container).some((t) => t.includes('name') || t.includes('qty'))).toBe(false),
    )
    expect(headerTexts(container).some((t) => t.trim() === '#')).toBe(true)
    expect(headerTexts(container).some((t) => t.includes('操作'))).toBe(true)
    expect(JSON.parse(localStorage.getItem('gbi.rows_hidden.7') ?? '[]')).toEqual(['name', 'qty'])

    // 再点一次 → 全选恢复
    fireEvent.click(selectAll)
    await waitFor(() => expect(headerTexts(container).some((t) => t.includes('qty'))).toBe(true))
    expect(JSON.parse(localStorage.getItem('gbi.rows_hidden.7') ?? 'null')).toEqual([])

    // 逐列取消一列 → 半选态
    fireEvent.click(screen.getByRole('checkbox', { name: 'qty' }))
    await waitFor(() => expect(selectAll.indeterminate).toBe(true))
  })
})
