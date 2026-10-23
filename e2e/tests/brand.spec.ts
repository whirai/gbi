import { Browser, Page, expect, test } from '@playwright/test'
import path from 'node:path'
import { login } from '../helpers'

test.describe('品牌设置', () => {
  test.describe.configure({ mode: 'serial' })
  let page: Page

  test.beforeAll(async ({ browser }: { browser: Browser }) => {
    page = await browser.newPage()
    await login(page, 'admin', 'admin123')
    await expect(page).toHaveURL(/\/admin\/users/)
  })

  test.afterAll(async ({ playwright }) => {
    // 兜底:serial 模式下首个用例失败会跳过"恢复默认"用例,
    // 这里直连后端重置品牌,避免自定义标题/favicon 污染后续 spec。
    try {
      const api = await playwright.request.newContext({ baseURL: 'http://localhost:8080' })
      const res = await api.post('/api/v1/auth/login', {
        data: { username: 'admin', password: 'admin123' },
      })
      const body = (await res.json()) as { data?: { token?: string } }
      const token = body.data?.token
      if (token) {
        await api.delete('/api/v1/branding', {
          headers: { Authorization: `Bearer ${token}` },
        })
      }
      await api.dispose()
    } catch {
      // 后端/页面不可用时忽略,不掩盖用例本身的失败
    }
    await page.close()
  })

  test('上传 logo/favicon 并修改名称与标题', async () => {
    await page.goto('/admin/branding')
    const fixture = path.resolve(__dirname, '../fixtures/logo.png')
    const inputs = page.locator('input[type="file"]')
    await inputs.nth(0).setInputFiles(fixture) // 卡片顺序:Logo 在前、Favicon 在后
    await expect(page.getByText('已上传').first()).toBeVisible()
    await expect(page.locator('img[alt="Logo"]')).toBeVisible()

    await inputs.nth(1).setInputFiles(fixture)
    // 等 Favicon 卡片回显上传后的图片,确保上传完成后再保存(避免并发写配置互相覆盖)
    await expect(page.locator('img[alt="Favicon"]')).toBeVisible()

    await page.getByLabel('系统名称').fill('E2E品牌')
    await page.getByLabel('浏览器标题').fill('E2E标题')
    await page.getByRole('button', { name: /保\s*存/ }).click()
    await expect(page.getByText('已保存')).toBeVisible()

    await expect(page).toHaveTitle('E2E标题')
    await expect(page.locator('.ant-layout-sider').getByText(/E2E品牌 · 控制台/)).toBeVisible()
    await expect(page.locator('link[rel~="icon"]')).toHaveAttribute(
      'href',
      /\/api\/v1\/branding\/image\/favicon/,
    )
  })

  test('恢复全部默认', async () => {
    await page.getByRole('button', { name: /恢复全部默认/ }).click()
    await page.locator('.ant-popover').getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.getByText('已恢复默认')).toBeVisible()
    await expect(page).toHaveTitle('生成式 BI')
    await expect(page.locator('.ant-layout-sider').getByText(/生成式 BI · 控制台/)).toBeVisible()
    await expect(page.locator('link[rel~="icon"]')).toHaveAttribute('href', '/favicon.svg')
  })
})
