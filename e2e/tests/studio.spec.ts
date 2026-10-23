import { Browser, Page, expect, test } from '@playwright/test'
import fs from 'node:fs'
import path from 'node:path'
import { login } from '../helpers'

test.describe('生成式工作台全流程', () => {
  test.describe.configure({ mode: 'serial' })

  const stamp = `${Date.now()}`
  const subjectName = `生成主题${stamp}`
  const subjectCode = `g${stamp}`
  const pageName = `生成页${stamp}`

  let page: Page
  let sid = 0
  let pageId = 0

  const apiFetch = (p: string, init?: { method?: string; body?: unknown }) =>
    page.evaluate(
      async ({ p, method, body }) => {
        const res = await fetch(`/api/v1${p}`, {
          method: method ?? 'GET',
          headers: {
            Authorization: `Bearer ${localStorage.getItem('gbi_token')}`,
            ...(body ? { 'Content-Type': 'application/json' } : {}),
          },
          body: body ? JSON.stringify(body) : undefined,
        })
        return { status: res.status, json: await res.json() }
      },
      { p, method: init?.method, body: init?.body },
    )

  test.beforeAll(async ({ browser }: { browser: Browser }) => {
    page = await browser.newPage()
    await login(page, 'admin', 'admin123')
    await expect(page).toHaveURL(/\/admin\/users/)
  })

  test.afterAll(async () => {
    await page.close()
  })

  test('准备:主题+导入数据+页面', async () => {
    // 主题
    const sub = await apiFetch('/subjects', { method: 'POST', body: { name: subjectName, code: subjectCode, desc: '' } })
    expect(sub.status).toBe(200)
    sid = sub.json.data.id

    // 导入夹具(建表+灌数)
    const fixture = fs.readFileSync(path.resolve(__dirname, '../fixtures/import_sample.xlsx'))
    const token = await page.evaluate(() => localStorage.getItem('gbi_token'))
    const resp = await page.evaluate(
      async ({ sid, token, buf }) => {
        const fd = new FormData()
        fd.append('file', new Blob([new Uint8Array(buf)], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }), 'import_sample.xlsx')
        fd.append('mode', 'create')
        fd.append('table_name', 'sales')
        fd.append('kind', 'DWD')
        const res = await fetch(`/api/v1/subjects/${sid}/import`, {
          method: 'POST',
          headers: { Authorization: `Bearer ${token}` },
          body: fd,
        })
        return { status: res.status, json: await res.json() }
      },
      { sid, token, buf: Array.from(fixture) },
    )
    expect(resp.status).toBe(200)

    // 页面
    const pg = await apiFetch('/pages', { method: 'POST', body: { name: pageName, desc: '' } })
    expect(pg.status).toBe(200)
    pageId = pg.json.data.id
  })

  test('工作台:对话生成图表并渲染', async () => {
    await page.goto(`/studio?page=${pageId}`)
    // 选择数据集
    await page.locator('.ant-select').first().click()
    await page.locator('.ant-select-dropdown').getByText(subjectName).click()
    await page.keyboard.press('Escape')
    // 发送需求
    await page.getByPlaceholder(/描述您的可视化需求/).fill('按区域统计销售额,画柱状图')
    await page.getByRole('button', { name: /发\s*送/ }).click()
    // 等待最终文本与画布
    await expect(page.getByText('已为您生成区域销售汇总图表。')).toBeVisible({ timeout: 30000 })
    await expect(page.locator('canvas').first()).toBeVisible({ timeout: 15000 })
  })

  test('持久化:配置与会话可复查', async () => {
    // 配置有 1 个图表
    const pg = await apiFetch(`/pages/${pageId}`)
    expect(pg.status).toBe(200)
    expect(pg.json.data.config.charts.length).toBe(1)
    // 会话含 user+assistant 消息
    const conv = await apiFetch(`/studio/conversations/${pageId}`)
    expect(conv.status).toBe(200)
    const roles = conv.json.data.messages.map((m: { role: string }) => m.role)
    expect(roles).toContain('user')
    expect(roles).toContain('assistant')
  })

  test('清理:删页面+主题', async () => {
    await page.goto('/admin/pages')
    await page.getByRole('row', { name: pageName }).getByRole('button', { name: /删\s*除/ }).click()
    await page.locator('.ant-popover').getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.getByRole('row', { name: pageName })).toHaveCount(0)

    await page.goto('/admin/subjects')
    await page.locator('.ant-list-item', { hasText: subjectName }).getByRole('button', { name: /删\s*除/ }).click()
    await page.locator('.ant-popover').getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.locator('.ant-list-item', { hasText: subjectName })).toHaveCount(0)
  })
})
