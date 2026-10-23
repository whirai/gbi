import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import Studio from './Studio'

const { deleteConversation } = vi.hoisted(() => ({ deleteConversation: vi.fn(async () => undefined) }))

vi.mock('../api/studio', () => ({
  chatStream: vi.fn(),
  getConversation: vi.fn(async () => ({ conversation: { id: 1, page_id: 4, title: '' }, messages: [] })),
  deleteConversation,
}))
vi.mock('../api/subjects', () => ({ listSubjects: vi.fn(async () => []) }))
vi.mock('../components/PageView', () => ({ default: () => <div data-testid="page-view" /> }))

Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation((query: string) => ({
    matches: false, media: query, onchange: null, addListener: vi.fn(), removeListener: vi.fn(),
    addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn(),
  })),
})

describe('Studio 新对话', () => {
  it('点击新对话按钮发出 DELETE 请求', async () => {
    render(
      <MemoryRouter initialEntries={['/studio?page=4']}>
        <Studio />
      </MemoryRouter>,
    )
    const btn = await screen.findByText('新对话')
    btn.click()
    // Popconfirm:点击子按钮仅展开弹层,需再点「确定」(主按钮)才触发 onConfirm
    const ok = await waitFor(() => {
      const el = document.querySelector('.ant-popconfirm-buttons .ant-btn-primary') as HTMLButtonElement | null
      expect(el).not.toBeNull()
      return el as HTMLButtonElement
    })
    ok.click()
    await waitFor(() => expect(deleteConversation).toHaveBeenCalledWith(4))
  })
})
