import { beforeEach, describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import Login from './Login'
import { useBranding } from '../store/branding'

Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string) => ({
    matches: false, media: query, onchange: null,
    addListener: () => {}, removeListener: () => {},
    addEventListener: () => {}, removeEventListener: () => {}, dispatchEvent: () => false,
  }),
})

describe('Login 品牌', () => {
  beforeEach(() => {
    useBranding.setState({ systemName: '生成式 BI', logoUrl: null })
  })

  it('渲染自定义系统名与 logo', () => {
    useBranding.setState({ systemName: '我的BI', logoUrl: '/api/v1/branding/image/logo?v=9' })
    render(
      <MemoryRouter>
        <Login />
      </MemoryRouter>,
    )
    expect(screen.getByRole('heading', { name: '我的BI' })).toBeTruthy()
    const img = screen.getByRole('img', { name: 'logo' }) as HTMLImageElement
    expect(img.getAttribute('src')).toBe('/api/v1/branding/image/logo?v=9')
  })

  it('无 logo 时仅显示默认系统名', () => {
    render(
      <MemoryRouter>
        <Login />
      </MemoryRouter>,
    )
    expect(screen.getByRole('heading', { name: '生成式 BI' })).toBeTruthy()
    expect(screen.queryByRole('img', { name: 'logo' })).toBeNull()
  })
})
