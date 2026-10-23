import { Browser, Page, expect, test } from '@playwright/test'
import { login } from '../helpers'

test.describe('组织管理全流程', () => {
  test.describe.configure({ mode: 'serial' })

  const stamp = `${Date.now()}`
  const deptName = `销售部${stamp}`
  const subDeptName = `华东区${stamp}`
  const devUsername = `dev${stamp}`
  const devName = `开发甲${stamp}`
  const plainUsername = `plain${stamp}`
  const plainName = `普通甲${stamp}`
  const groupName = `销售分析组${stamp}`

  let page: Page

  test.beforeAll(async ({ browser }: { browser: Browser }) => {
    page = await browser.newPage()
    await login(page, 'admin', 'admin123')
    await expect(page).toHaveURL(/\/admin\/users/)
  })

  test.afterAll(async () => {
    await page.close()
  })

  test('部门:新建顶层与子部门', async () => {
    await page.goto('/admin/departments')
    await page.getByRole('button', { name: '新建顶层部门' }).click()
    await page.getByLabel('部门名称').fill(deptName)
    await page.getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.locator('.ant-tree-treenode', { hasText: deptName })).toBeVisible()

    await page
      .locator('.ant-tree-treenode', { hasText: deptName })
      .getByRole('button', { name: '添加子级' })
      .click()
    await page.getByLabel('部门名称').fill(subDeptName)
    await page.getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.locator('.ant-tree-treenode', { hasText: subDeptName })).toBeVisible()
  })

  test('用户:新建开发者并挂部门', async () => {
    await page.goto('/admin/users')
    await page.getByRole('button', { name: '新建用户' }).click()
    await page.getByLabel('用户名').fill(devUsername)
    await page.getByLabel('密码').fill('pass1234')
    await page.getByLabel('姓名').fill(devName)
    await page.getByLabel('部门').click()
    await page.locator('.ant-select-dropdown').getByTitle(deptName).click()
    await page.getByLabel('角色').click()
    await page.locator('.ant-select-dropdown').getByTitle('开发者').click()
    await page.getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.getByRole('row', { name: devUsername })).toBeVisible()
  })

  test('用户组:建组并添加成员', async () => {
    await page.goto('/admin/groups')
    await page.getByRole('button', { name: '新建用户组' }).click()
    await page.getByLabel('组名称').fill(groupName)
    await page.getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.getByRole('row', { name: groupName })).toBeVisible()

    await page.getByRole('row', { name: groupName }).getByRole('button', { name: /成\s*员/ }).click()
    // 不能点 placeholder 文本——Select 的透明搜索输入框覆盖其上拦截指针;点 combobox 本身
    await page.locator('.ant-drawer').getByRole('combobox').click()
    await page.locator('.ant-select-dropdown').getByText(devName).click()
    await expect(page.getByRole('row', { name: devUsername })).toBeVisible()
    await page.locator('.ant-drawer').getByRole('button', { name: /关\s*闭/ }).click()
  })

  test('角色:重命名开发者', async () => {
    await page.goto('/admin/roles')
    await page.getByRole('row', { name: 'developer' }).getByRole('button', { name: '重命名' }).click()
    await page.getByLabel('角色名称').fill('研发工程师')
    await page.getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.getByRole('row', { name: '研发工程师' })).toBeVisible()

    // 还原,避免影响后续用例的角色下拉选项文本(重命名后按新名称定位行)
    await page.getByRole('row', { name: '研发工程师' }).getByRole('button', { name: '重命名' }).click()
    await page.getByLabel('角色名称').fill('开发者')
    await page.getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.getByRole('row', { name: '开发者' })).toBeVisible()
  })

  test('普通用户只能进入门户', async () => {
    await page.goto('/admin/users')
    await page.getByRole('button', { name: '新建用户' }).click()
    await page.getByLabel('用户名').fill(plainUsername)
    await page.getByLabel('密码').fill('pass1234')
    await page.getByLabel('姓名').fill(plainName)
    await page.getByLabel('角色').click()
    await page.locator('.ant-select-dropdown').getByTitle('普通用户').click()
    await page.getByRole('button', { name: /确\s*定/ }).click()

    // 退出 admin,用普通用户登录(用户表格也有"系统管理员"行,须用顶栏下拉触发器精确定位)
    await page.locator('.ant-dropdown-trigger').click()
    await page.getByText('退出登录').click()
    await login(page, plainUsername, 'pass1234')
    await expect(page).toHaveURL(/\/portal/)
    await expect(page.getByText('暂无有权查看的 BI 页面')).toBeVisible()
  })

  test('清理:删组/删用户/删部门', async () => {
    await login(page, 'admin', 'admin123')
    await expect(page).toHaveURL(/\/admin\/users/) // 等登录导航完成,避免后续 goto 中断登录

    await page.goto('/admin/groups')
    await page.getByRole('row', { name: groupName }).getByRole('button', { name: /删\s*除/ }).click()
    await page.locator('.ant-popover').getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.getByRole('row', { name: groupName })).toHaveCount(0)

    await page.goto('/admin/users')
    await page.getByRole('row', { name: devUsername }).getByRole('button', { name: /删\s*除/ }).click()
    await page.locator('.ant-popover').getByRole('button', { name: /确\s*定/ }).click()
    await page.getByRole('row', { name: plainUsername }).getByRole('button', { name: /删\s*除/ }).click()
    await page.locator('.ant-popover').getByRole('button', { name: /确\s*定/ }).click()

    await page.goto('/admin/departments')
    await page
      .locator('.ant-tree-treenode', { hasText: subDeptName })
      .getByRole('button', { name: /删\s*除/ })
      .click()
    await page.locator('.ant-popover').getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.locator('.ant-tree-treenode', { hasText: subDeptName })).toHaveCount(0)
    await page
      .locator('.ant-tree-treenode', { hasText: deptName })
      .getByRole('button', { name: /删\s*除/ })
      .click()
    await page.locator('.ant-popover').getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.locator('.ant-tree-treenode', { hasText: deptName })).toHaveCount(0)
  })
})
