import { describe, expect, it, vi } from 'vitest'
import { act, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, useLocation } from 'react-router-dom'
import Portal from './Portal'

// 探针:暴露当前路由 search,观察点击后 URL 是否变化
function LocationProbe() {
  const loc = useLocation()
  return <div data-testid="loc">{loc.pathname + loc.search}</div>
}

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

vi.mock('../api/pages', () => ({
  portalMenus: vi.fn(async () => [
    {
      id: 1,
      name: '销量数据',
      items: [
        { id: 11, name: '乘用车月度销量', page_id: 1 },
        { id: 12, name: '款型映射', page_id: 2 },
      ],
    },
  ]),
}))

vi.mock('../components/PageView', () => ({
  default: ({ pageId }: { pageId: number }) => <div data-testid="page-view">page-{pageId}</div>,
}))

vi.mock('../store/auth', () => ({
  useAuth: () => ({ user: { name: '管理员', role_code: 'admin' }, logout: vi.fn() }),
}))

describe('Portal 菜单项导航', () => {
  it('点击菜单项后菜单项栏仍可见,可切回其他页面(线上事故回归)', async () => {
    render(
      <MemoryRouter initialEntries={['/portal']}>
        <Portal />
        <LocationProbe />
      </MemoryRouter>,
    )

    await screen.findByRole('menuitem', { name: '乘用车月度销量' })
    await screen.findByRole('menuitem', { name: '款型映射' })

    // 点击「款型映射」:页面切换,菜单项栏不得消失
    // (async act 包裹:react-router v7 的导航是异步过渡,须刷新微任务队列)
    await act(async () => {
      screen.getByRole('menuitem', { name: '款型映射' }).click()
    })
    await waitFor(() => expect(screen.getByTestId('page-view').textContent).toBe('page-2'))
    expect(screen.getByRole('menuitem', { name: '乘用车月度销量' })).toBeTruthy()
    expect(screen.getByRole('menuitem', { name: '款型映射' })).toBeTruthy()

    // 可直接切回「乘用车月度销量」
    await act(async () => {
      screen.getByRole('menuitem', { name: '乘用车月度销量' }).click()
    })
    await waitFor(() => expect(screen.getByTestId('page-view').textContent).toBe('page-1'))
  })
})
