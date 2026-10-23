import { describe, expect, it, vi } from 'vitest'
import { act, render, screen, waitFor } from '@testing-library/react'
import Subjects from './Subjects'

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

const { listRows } = vi.hoisted(() => ({
  listRows: vi.fn(async () => ({ rows: [], total: 0 })),
}))

vi.mock('../../api/subjects', () => ({
  listSubjects: vi.fn(async () => [
    { id: 1, name: '乘用车月度销量', code: 'pvms', desc: '' },
    { id: 2, name: '商用车月度销量', code: 'cvms', desc: '' },
  ]),
  listTables: vi.fn(async (sid: number) =>
    sid === 1
      ? [{ id: 1, name: 't_pvms', kind: 'DWD', columns: [{ name: 'Quantity', type: 'INTEGER' }] }]
      : [{ id: 2, name: 't_cvms', kind: 'DWD', columns: [{ name: 'Quantity', type: 'INTEGER' }] }],
  ),
  listRows,
  listColumnValues: vi.fn(async () => []),
  createSubject: vi.fn(),
  updateSubject: vi.fn(),
  removeSubject: vi.fn(),
  removeTable: vi.fn(),
  updateCell: vi.fn(),
  deleteRow: vi.fn(),
}))

describe('Subjects 主题切换', () => {
  it('切换主题时不携带上一个主题的表ID发起数据预览请求(线上404回归)', async () => {
    render(<Subjects />)

    // 选择主题1:表列表与数据预览都指向主题1
    await screen.findByText('乘用车月度销量')
    await act(async () => {
      screen.getByText('乘用车月度销量').closest('li')!.click()
    })
    await waitFor(() => expect(listRows).toHaveBeenCalledWith(1, 1, 1, 20, {}))

    // 切到主题2:先等主题2的表加载完成,再断言期间从未发出(主题2, 表1)的陈旧请求
    await act(async () => {
      screen.getByText('商用车月度销量').closest('li')!.click()
    })
    await waitFor(() => expect(listRows).toHaveBeenCalledWith(2, 2, 1, 20, {}))

    const staleCall = listRows.mock.calls.find((c: unknown[]) => c[0] === 2 && c[1] === 1)
    expect(staleCall).toBeUndefined()
  })
})
