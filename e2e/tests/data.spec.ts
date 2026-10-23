import { Browser, Page, expect, test } from '@playwright/test'
import path from 'node:path'
import { login } from '../helpers'

test.describe('数据层全流程', () => {
  test.describe.configure({ mode: 'serial' })

  const stamp = `${Date.now()}`
  const subjectName = `销售主题${stamp}`
  const subjectCode = `s${stamp}`
  const tableName = `orders${stamp}` // 建表基础名 → dwd_orders<stamp>
  const fullTable = `dwd_orders${stamp}`
  const importTable = `imp${stamp}` // 导入建表 → dwd_imp<stamp>

  let page: Page

  test.beforeAll(async ({ browser }: { browser: Browser }) => {
    page = await browser.newPage()
    await login(page, 'admin', 'admin123')
    await expect(page).toHaveURL(/\/admin\/users/)
  })

  test.afterAll(async () => {
    await page.close()
  })

  test('主题:新建与展示', async () => {
    await page.goto('/admin/subjects')
    // 该按钮带 PlusOutlined 图标,可访问名为 "plus 新建"(图标 aria 前缀),
    // 无法用 ^ 锚定开头;仅锚定结尾,仍不会匹配到"新建表"(以"表"结尾)
    await page.getByRole('button', { name: /新\s*建$/ }).click()
    await page.getByLabel('主题名称').fill(subjectName)
    await page.getByLabel(/主题编码/).fill(subjectCode)
    await page.getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.locator('.ant-list-item').filter({ hasText: subjectName })).toBeVisible()
  })

  test('表:手动建表(DWD 两列)', async () => {
    // 限定在列表项内点击(选中后详情卡片标题也含同名文本,全局 getByText 会 strict 冲突)
    await page.locator('.ant-list-item').filter({ hasText: subjectName }).click()
    await page.getByRole('button', { name: '新建表' }).click()
    await page.getByLabel(/表名/).fill(tableName)
    // 列定义:第一行默认已有一列,填列名;再加一列
    const nameInputs = page.getByPlaceholder('列名')
    await nameInputs.first().fill('region')
    await page.getByRole('button', { name: '添加列' }).click()
    await nameInputs.nth(1).fill('amount')
    await page.locator('.ant-select').filter({ hasText: 'TEXT' }).nth(1).click()
    await page.locator('.ant-select-dropdown').getByTitle('INTEGER').click()
    await page.getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.getByText(fullTable)).toBeVisible()
  })

  test('导入:create 模式建表灌数', async () => {
    await page.locator('.ant-list-item').filter({ hasText: subjectName }).click()
    await page.getByRole('button', { name: '导入Excel' }).click()
    await page.locator('.ant-upload input[type="file"]').setInputFiles(
      path.resolve(__dirname, '../fixtures/import_sample.xlsx'),
    )
    await page.getByPlaceholder('表名(不含前缀)').fill(importTable)
    await page.getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.getByText('导入成功,共 4 行')).toBeVisible()
    // 新表出现在 Tabs
    await expect(page.getByText(`dwd_${importTable}`)).toBeVisible()
  })

  test('数据:单元格编辑与删行', async () => {
    // 切到导入表
    await page.getByText(`dwd_${importTable}`).click()
    // 非激活 Tab 面板仍保留在 DOM 中(aria-hidden),orders 表同样有 amount 列头与占位行,
    // 所以列头/数据行一律限定在当前激活面板内,避免 strict 冲突/选错表;
    // 行用 .ant-table-row 类选择器:启用 scroll.x 后 antd 会在 tbody 首部插入隐藏测量行
    // (ant-table-measure-row,无 input),直接 tbody tr.first() 会选中它
    const activePane = page.locator('.ant-tabs-tabpane-active')
    // 编辑第一行 amount:100 → 999(INTEGER 列)
    // 用 columnheader 语义定位:启用 scroll.x 后 antd 会渲染隐藏测量行(ant-table-measure-cell),
    // locator('th', {hasText}) 会命中 2 个元素触发 strict 冲突
    const amountCol = activePane.getByRole('columnheader', { name: /amount/ })
    await expect(amountCol).toBeVisible()
    const firstAmount = activePane.locator('tbody tr.ant-table-row').first().locator('input').nth(2) // month、region、amount → 第3个输入框
    await firstAmount.fill('999')
    await firstAmount.blur()
    await expect(page.getByText('已保存')).toBeVisible()
    // 单元格是 <input>,value 不属于行文本,断言须用 toHaveValue 而非 toContainText
    await expect(firstAmount).toHaveValue('999')

    // 删除最后一行
    const rowCountBefore = await activePane.locator('tbody tr.ant-table-row').count()
    await activePane.locator('tbody tr.ant-table-row').last().getByText('删除').click()
    await page.locator('.ant-popover').getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.getByText('已删除')).toBeVisible()
    await expect(activePane.locator('tbody tr.ant-table-row')).toHaveCount(rowCountBefore - 1)
  })

  test('数据:列筛选与列设置', async () => {
    // 上一用例已切到导入表;重新点击确保激活面板
    await page.getByText(`dwd_${importTable}`).click()
    const activePane = page.locator('.ant-tabs-tabpane-active')
    const totalBefore = await activePane.locator('.ant-pagination-total-text').innerText()

    // 列筛选:region 选第一个候选值 → 服务端筛选后所有行值一致且总数变化
    const regionTh = activePane.getByRole('columnheader', { name: /region/ })
    await regionTh.locator('.ant-table-filter-trigger').click()
    const dropdown = page.locator('.ant-table-filter-dropdown')
    await expect(dropdown).toBeVisible()
    await dropdown.locator('.ant-select').click()
    const option = page.locator('.ant-select-dropdown:visible .ant-select-item-option').first()
    await expect(option).toBeVisible()
    const picked = (await option.innerText()).trim()
    await option.click()
    // 多选 Select 的候选弹层会盖住面板底部的「确定」:先点面板空白处关闭弹层
    await dropdown.click({ position: { x: 6, y: 6 } })
    await dropdown.getByRole('button', { name: /确\s*定/ }).click()

    await expect(activePane.locator('.ant-pagination-total-text')).not.toHaveText(totalBefore)
    const regionValues = await activePane.locator('tbody tr.ant-table-row').evaluateAll((trs) =>
      trs.map((tr) => (tr.querySelectorAll('input')[1] as HTMLInputElement).value),
    )
    expect(regionValues.length).toBeGreaterThan(0)
    expect(regionValues.every((v) => v === picked)).toBe(true)

    // 列设置:隐藏 amount 列,表头消失
    await activePane.getByRole('button', { name: /列设置/ }).click()
    const popover = page.locator('.ant-popover:visible')
    await popover.getByRole('checkbox', { name: 'amount' }).uncheck()
    await expect(activePane.getByRole('columnheader', { name: /amount/ })).toHaveCount(0)
  })

  test('查询 API:只读 SQL 返回数据', async () => {
    const subjects = await page.evaluate(async () => {
      const res = await fetch('/api/v1/subjects', {
        headers: { Authorization: `Bearer ${localStorage.getItem('gbi_token')}` },
      })
      return (await res.json()).data as { id: number; code: string }[]
    })
    const sid = subjects.find((s) => s.code === subjectCode)?.id
    expect(sid).toBeTruthy()

    const result = await page.evaluate(
      async ({ sid, table }) => {
        const res = await fetch('/api/v1/query', {
          method: 'POST',
          headers: {
            Authorization: `Bearer ${localStorage.getItem('gbi_token')}`,
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ sql: `SELECT COUNT(*) AS n FROM ${table}`, subject_ids: [sid] }),
        })
        return res.json()
      },
      { sid, table: `dwd_${importTable}` },
    )
    expect(result.code).toBe(0)
    expect(result.data.columns).toEqual(['n'])
    expect(result.data.rows[0][0]).toBe(3) // 导入 4 行 - 删 1 行
  })

  test('RBAC:普通用户访问主题 API 返回 403', async () => {
    // 用 admin 创建普通用户
    await page.goto('/admin/users')
    await page.getByRole('button', { name: '新建用户' }).click()
    await page.getByLabel('用户名').fill(`plain_d${stamp}`)
    await page.getByLabel('密码').fill('pass1234')
    await page.getByLabel('姓名').fill(`普通丙${stamp}`)
    await page.getByLabel('角色').click()
    await page.locator('.ant-select-dropdown').getByTitle('普通用户').click()
    await page.getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.getByRole('row', { name: `plain_d${stamp}` })).toBeVisible()

    // 退出并以普通用户登录
    await page.locator('.ant-dropdown-trigger').click()
    await page.getByText('退出登录').click()
    await login(page, `plain_d${stamp}`, 'pass1234')
    await expect(page).toHaveURL(/\/portal/)

    const status = await page.evaluate(async () => {
      const res = await fetch('/api/v1/subjects', {
        headers: { Authorization: `Bearer ${localStorage.getItem('gbi_token')}` },
      })
      return res.status
    })
    expect(status).toBe(403)
  })

  test('清理:删除主题(连带表与文件)', async () => {
    await login(page, 'admin', 'admin123')
    await expect(page).toHaveURL(/\/admin\/users/)
    await page.goto('/admin/subjects')
    await page.locator('.ant-list-item', { hasText: subjectName }).getByRole('button', { name: /删\s*除/ }).click()
    await page.locator('.ant-popover').getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.getByText(subjectName)).toHaveCount(0)
  })
})
