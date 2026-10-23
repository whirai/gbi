import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import Branding from './Branding'
import { useBranding } from '../../store/branding'

const { updateBranding } = vi.hoisted(() => ({ updateBranding: vi.fn(async () => ({})) }))
vi.mock('../../api/branding', () => ({
  updateBranding,
  uploadBrandingImage: vi.fn(),
  deleteBrandingImage: vi.fn(),
  resetBranding: vi.fn(),
  getBranding: vi.fn(),
}))

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

describe('Branding 页', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useBranding.setState({
      systemName: '我的BI',
      browserTitle: '我的标题',
      logoUrl: null,
      faviconUrl: null,
      load: async () => {},
    })
  })

  it('回填名称/标题,未上传图片显示占位,保存调用接口', async () => {
    render(<Branding />)
    await waitFor(() => expect((screen.getByLabelText('系统名称') as HTMLInputElement).value).toBe('我的BI'))
    expect((screen.getByLabelText('浏览器标题') as HTMLInputElement).value).toBe('我的标题')
    expect(screen.getAllByText('未上传').length).toBeGreaterThan(0)

    fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    await waitFor(() =>
      expect(updateBranding).toHaveBeenCalledWith({ system_name: '我的BI', browser_title: '我的标题' }),
    )
  })
})
