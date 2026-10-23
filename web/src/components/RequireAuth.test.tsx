import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import RequireAuth from './RequireAuth'
import { useAuth } from '../store/auth'

// jsdom 缺 matchMedia,antd Spin 需要
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

vi.mock('../api/auth', () => ({
  me: vi.fn(async () => ({ id: 1, username: 'admin', name: '系统管理员', role_code: 'admin' })),
}))

afterEach(() => {
  localStorage.clear()
  useAuth.setState({ user: null })
})

describe('RequireAuth 挂载加载', () => {
  // 回归:卸载守卫若用 effect 内局部 active 标志(deps 含 user),setUser 触发的
  // 同步重渲染会先执行 cleanup(active=false),pending 的 finally 被吞掉,
  // setChecking(false) 永不执行 → 整页永久转圈(线上事故,4 个 e2e 用例首测超时)。
  it('token 存在时 me() 解析后必须渲染 children,不得卡在加载态', async () => {
    localStorage.setItem('gbi_token', 'test-token')
    render(
      <MemoryRouter initialEntries={['/admin']}>
        <Routes>
          <Route
            path="/admin"
            element={
              <RequireAuth roles={['admin']}>
                <div data-testid="child">受保护内容</div>
              </RequireAuth>
            }
          />
          <Route path="/login" element={<div data-testid="login">登录页</div>} />
        </Routes>
      </MemoryRouter>,
    )
    await waitFor(() => expect(screen.getByTestId('child')).toBeTruthy())
  })
})
