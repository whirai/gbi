import { create } from 'zustand'
import { getBranding } from '../api/branding'

interface BrandingState {
  systemName: string
  browserTitle: string
  logoUrl: string | null
  faviconUrl: string | null
  load: () => Promise<void>
}

export const useBranding = create<BrandingState>((set) => ({
  systemName: '生成式 BI',
  browserTitle: '生成式 BI',
  logoUrl: null,
  faviconUrl: null,
  load: async () => {
    try {
      const b = await getBranding()
      set({
        systemName: b.system_name,
        browserTitle: b.browser_title,
        logoUrl: b.logo_url,
        faviconUrl: b.favicon_url,
      })
    } catch {
      // 品牌接口失败不阻塞使用:保持默认值
    }
  },
}))
