import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getBranding } = vi.hoisted(() => ({ getBranding: vi.fn() }))
vi.mock('../api/branding', () => ({ getBranding }))

import { useBranding } from './branding'

describe('useBranding', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useBranding.setState({
      systemName: '生成式 BI',
      browserTitle: '生成式 BI',
      logoUrl: null,
      faviconUrl: null,
    })
  })

  it('load 应用服务端品牌', async () => {
    getBranding.mockResolvedValue({
      system_name: '我的BI',
      browser_title: '我的标题',
      logo_url: '/api/v1/branding/image/logo?v=1',
      favicon_url: '/api/v1/branding/image/favicon?v=1',
    })
    await useBranding.getState().load()
    const s = useBranding.getState()
    expect(s.systemName).toBe('我的BI')
    expect(s.browserTitle).toBe('我的标题')
    expect(s.logoUrl).toBe('/api/v1/branding/image/logo?v=1')
    expect(s.faviconUrl).toBe('/api/v1/branding/image/favicon?v=1')
  })

  it('load 失败保持默认', async () => {
    getBranding.mockRejectedValue(new Error('network'))
    await useBranding.getState().load()
    const s = useBranding.getState()
    expect(s.systemName).toBe('生成式 BI')
    expect(s.logoUrl).toBeNull()
  })
})
