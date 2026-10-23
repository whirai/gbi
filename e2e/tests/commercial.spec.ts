import { Browser, Page, expect, test } from '@playwright/test'
import fs from 'node:fs'
import path from 'node:path'
import { login } from '../helpers'

test.describe('商用车复杂分析全流程', () => {
  test.describe.configure({ mode: 'serial' })

  const stamp = `${Date.now()}`
  const subjectName = `商用车主题${stamp}`
  const subjectCode = 'cvms_e2e'
  const pageName = `商用车分析页${stamp}`

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

  test('准备:主题+导入夹具+页面', async () => {
    const sub = await apiFetch('/subjects', {
      method: 'POST',
      body: { name: subjectName, code: subjectCode, desc: '' },
    })
    expect(sub.status).toBe(200)
    sid = sub.json.data.id
    expect(sid).toBeGreaterThan(0)

    // 导入商用车裁剪夹具(5000 行 40 列)
    const fixture = fs.readFileSync(path.resolve(__dirname, '../fixtures/comm_vehicles.xlsx'))
    const token = await page.evaluate(() => localStorage.getItem('gbi_token'))
    const resp = await page.evaluate(
      async ({ sid, token, buf }) => {
        const fd = new FormData()
        fd.append('file', new Blob([new Uint8Array(buf)], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }), 'comm_vehicles.xlsx')
        fd.append('mode', 'create')
        fd.append('table_name', 'commercial')
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
    expect(resp.json.data.inserted).toBe(5000)

    const pg = await apiFetch('/pages', { method: 'POST', body: { name: pageName, desc: '' } })
    expect(pg.status).toBe(200)
    pageId = pg.json.data.id
  })

  test('工作台:对话生成 3 个复杂分析图表', async () => {
    await page.goto(`/studio?page=${pageId}`)
    await page.locator('.ant-select').first().click()
    await page.locator('.ant-select-dropdown').getByText(subjectName).click()
    await page.keyboard.press('Escape')
    await page.getByPlaceholder(/描述您的可视化需求/).fill('分析商用车销量:品牌市占率、吨位段结构、城市排名')
    await page.getByRole('button', { name: /发\s*送/ }).click()

    await expect(page.getByText('已为您生成商用车复杂分析图表:Top10品牌市占率、吨位段结构、Top10城市。')).toBeVisible({ timeout: 30000 })
    await expect(page.locator('canvas').first()).toBeVisible({ timeout: 15000 })
    expect(await page.locator('canvas').count()).toBeGreaterThanOrEqual(3)
  })

  test('持久化+数据抽查', async () => {
    // 配置含 3 图表
    const pg = await apiFetch(`/pages/${pageId}`)
    expect(pg.status).toBe(200)
    const charts = pg.json.data.config.charts
    expect(charts.length).toBe(3)
    expect(charts.map((c: { id: string }) => c.id).sort()).toEqual(['mass-category', 'top-brands', 'top-cities'])

    // 闭环断言:第一轮图表带坏标签(labelTransform.overall 不存在)→ 前端上报渲染错误
    // → agent 自动核验注入 → fakellm 修正分支重发 update_page → 终态配置不含坏标签。
    const fixed = charts.find((c: { id: string }) => c.id === 'top-brands')
    expect(fixed?.title).toContain('已修正')
    expect(JSON.stringify(fixed?.spec)).not.toContain('labelTransform')

    // 图表 1(Top10 品牌):查询 200、10 行、按 qty 降序
    const c1 = charts.find((c: { id: string }) => c.id === 'top-brands')
    const q1 = await apiFetch('/query', { method: 'POST', body: { sql: c1.sql, subject_ids: c1.subjects } })
    expect(q1.status).toBe(200)
    expect(q1.json.data.rows.length).toBe(10)
    expect(q1.json.data.rows[0][1]).toBeGreaterThanOrEqual(q1.json.data.rows[1][1])

    // 图表 2(吨位段×类别):查询 200 且有数据
    const c2 = charts.find((c: { id: string }) => c.id === 'mass-category')
    const q2 = await apiFetch('/query', { method: 'POST', body: { sql: c2.sql, subject_ids: c2.subjects } })
    expect(q2.status).toBe(200)
    expect(q2.json.data.rows.length).toBeGreaterThan(0)

    // 图表 3(Top10 城市):查询 200、10 行
    const c3 = charts.find((c: { id: string }) => c.id === 'top-cities')
    const q3 = await apiFetch('/query', { method: 'POST', body: { sql: c3.sql, subject_ids: c3.subjects } })
    expect(q3.status).toBe(200)
    expect(q3.json.data.rows.length).toBe(10)
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
