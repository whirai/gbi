import { Browser, Page, expect, test } from '@playwright/test'
import { login } from '../helpers'

test.describe('展示层全流程', () => {
  test.describe.configure({ mode: 'serial' })

  const stamp = `${Date.now()}`
  const subjectName = `图表主题${stamp}`
  const subjectCode = `c${stamp}`
  const pageName = `销售看板${stamp}`
  const pageName2 = `私有看板${stamp}`
  const menuName = `销售菜单${stamp}`
  const itemName = `总览${stamp}`

  let page: Page
  let sid = 0
  let pageId = 0
  let pageId2 = 0

  // 页面上下文内的带 token fetch(复用登录态)
  const apiFetch = (path: string, init?: { method?: string; body?: unknown }) =>
    page.evaluate(
      async ({ path, method, body }) => {
        const res = await fetch(`/api/v1${path}`, {
          method: method ?? 'GET',
          headers: {
            Authorization: `Bearer ${localStorage.getItem('gbi_token')}`,
            ...(body ? { 'Content-Type': 'application/json' } : {}),
          },
          body: body ? JSON.stringify(body) : undefined,
        })
        return { status: res.status, json: await res.json() }
      },
      { path, method: init?.method, body: init?.body },
    )

  test.beforeAll(async ({ browser }: { browser: Browser }) => {
    page = await browser.newPage()
    await login(page, 'admin', 'admin123')
    await expect(page).toHaveURL(/\/admin\/users/)
  })

  test.afterAll(async () => {
    await page.close()
  })

  test('准备:创建主题(API)', async () => {
    const res = await apiFetch('/subjects', {
      method: 'POST',
      body: { name: subjectName, code: subjectCode, desc: '' },
    })
    expect(res.status).toBe(200)
    sid = res.json.data.id
    expect(sid).toBeGreaterThan(0)
  })

  test('页面:新建为草稿并发布', async () => {
    await page.goto('/admin/pages')
    await page.getByRole('button', { name: '新建页面' }).click()
    await page.getByLabel('页面名称').fill(pageName)
    await page.getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.getByRole('row', { name: pageName })).toBeVisible()
    await expect(page.getByRole('row', { name: pageName })).toContainText('草稿')

    // 发布(两字按钮 → 正则)
    await page.getByRole('row', { name: pageName }).getByRole('button', { name: /发\s*布/ }).click()
    await expect(page.getByRole('row', { name: pageName })).toContainText('已发布')

    // 第二个页面(私有,不授权)
    await page.getByRole('button', { name: '新建页面' }).click()
    await page.getByLabel('页面名称').fill(pageName2)
    await page.getByRole('button', { name: /确\s*定/ }).click()
    await page.getByRole('row', { name: pageName2 }).getByRole('button', { name: /发\s*布/ }).click()
  })

  test('页面:经API写入图表配置并生成版本', async () => {
    // 取 pageId
    const list = await apiFetch('/pages')
    pageId = list.json.data.find((p: { name: string }) => p.name === pageName).id
    pageId2 = list.json.data.find((p: { name: string }) => p.name === pageName2).id

    const chart = {
      id: 'c1',
      title: '区域金额',
      width: 'full',
      type: 'interval',
      sql: `SELECT 'a' AS region, 100 AS amount UNION ALL SELECT 'b', 200`,
      subjects: [sid],
      spec: { type: 'interval', encode: { x: 'region', y: 'amount' } },
    }
    const upd = await apiFetch(`/pages/${pageId}`, {
      method: 'PUT',
      body: { config: { version: 1, layout: 'fluid-grid', charts: [chart] } },
    })
    expect(upd.status).toBe(200)

    const versions = await apiFetch(`/pages/${pageId}/versions`)
    expect(versions.status).toBe(200)
    expect(versions.json.data.length).toBe(1)
  })

  test('菜单:建菜单+菜单项指向页面', async () => {
    await page.goto('/admin/menus')
    await page.getByRole('button', { name: '新建菜单' }).click()
    await page.getByLabel('菜单名称').fill(menuName)
    await page.getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.locator('.ant-list-item', { hasText: menuName })).toBeVisible()

    await page.locator('.ant-list-item', { hasText: menuName }).click()
    await page.getByRole('button', { name: '添加菜单项' }).click()
    await page.getByLabel('指向页面').click()
    await page.locator('.ant-select-dropdown').getByText(pageName).first().click()
    await page.getByLabel('菜单项名称').fill(itemName)
    await page.getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.getByRole('row', { name: itemName })).toBeVisible()
  })

  test('门户:admin 可见菜单并渲染图表画布', async () => {
    await page.goto('/portal')
    await expect(page.getByText(menuName).first()).toBeVisible()
    await expect(page.getByText(itemName)).toBeVisible()
    // 菜单项标签点击后渲染画布
    await page.getByText(itemName).click()
    await expect(page.locator('canvas').first()).toBeVisible({ timeout: 15000 })
  })

  test('授权:普通用户仅见被授权页面', async () => {
    // 建两个普通用户
    for (const name of [`p3a${stamp}`, `p3b${stamp}`]) {
      await page.goto('/admin/users')
      await page.getByRole('button', { name: '新建用户' }).click()
      await page.getByLabel('用户名').fill(name)
      await page.getByLabel('密码').fill('pass1234')
      await page.getByLabel('姓名').fill(name)
      await page.getByLabel('角色').click()
      await page.locator('.ant-select-dropdown').getByTitle('普通用户').click()
      await page.getByRole('button', { name: /确\s*定/ }).click()
      await expect(page.getByRole('row', { name })).toBeVisible()
    }

    // 仅把 page1 授权给 p3a(经权限弹窗)
    await page.goto('/admin/pages')
    await page.getByRole('row', { name: pageName }).getByRole('button', { name: /权\s*限/ }).click()
    // 不用 check():授权弹窗数据加载(Spin)结束时 React 会重渲染复选框节点,
    // check() 对旧句柄的状态断言误报"状态未变"。click + 自动重试的 toBeChecked 更稳。
    const p3aBox = page.getByLabel(`p3a${stamp} (p3a${stamp})`)
    await p3aBox.click()
    await expect(p3aBox).toBeChecked()
    await page.getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.getByText('权限已保存')).toBeVisible()

    // p3a 登录:门户有菜单,只见 page1;管理页API 403
    await page.locator('.ant-dropdown-trigger').last().click()
    await page.getByText('退出登录').click()
    await login(page, `p3a${stamp}`, 'pass1234')
    await expect(page).toHaveURL(/\/portal/)
    await expect(page.getByText(menuName).first()).toBeVisible()
    await page.getByText(itemName).click()
    await expect(page.getByText(pageName)).toBeVisible()
    // 图表取数授权回归:已授权用户必须能渲染图表画布(曾被 role:user /query 策略缺失阻断)
    await expect(page.locator('canvas').first()).toBeVisible({ timeout: 15000 })

    const status = await page.evaluate(async () => {
      const res = await fetch('/api/v1/pages', {
        headers: { Authorization: `Bearer ${localStorage.getItem('gbi_token')}` },
      })
      return res.status
    })
    expect(status).toBe(403)

    // 未授权页面详情 → 404
    const detail = await apiFetch(`/pages/${pageId2}`)
    expect(detail.status).toBe(404)

    // p3b 登录:门户无可见菜单
    await page.locator('.ant-dropdown-trigger').last().click()
    await page.getByText('退出登录').click()
    await login(page, `p3b${stamp}`, 'pass1234')
    await expect(page).toHaveURL(/\/portal/)
    await expect(page.getByText('暂无有权查看的 BI 页面')).toBeVisible()
  })

  test('清理:删页面/菜单/主题', async () => {
    await login(page, 'admin', 'admin123')
    await expect(page).toHaveURL(/\/admin\/users/)

    await page.goto('/admin/pages')
    for (const name of [pageName, pageName2]) {
      await page.getByRole('row', { name }).getByRole('button', { name: /删\s*除/ }).click()
      await page.locator('.ant-popover').getByRole('button', { name: /确\s*定/ }).click()
      await expect(page.getByRole('row', { name })).toHaveCount(0)
    }

    await page.goto('/admin/menus')
    await page.locator('.ant-list-item', { hasText: menuName }).getByRole('button', { name: /删\s*除/ }).click()
    await page.locator('.ant-popover').getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.locator('.ant-list-item', { hasText: menuName })).toHaveCount(0)

    await page.goto('/admin/subjects')
    await page.locator('.ant-list-item', { hasText: subjectName }).getByRole('button', { name: /删\s*除/ }).click()
    await page.locator('.ant-popover').getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.locator('.ant-list-item', { hasText: subjectName })).toHaveCount(0)
  })
})
