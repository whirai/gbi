import { expect, test } from '@playwright/test'
import { login } from '../helpers'

test('管理员登录进入控制台', async ({ page }) => {
  await login(page, 'admin', 'admin123')
  await expect(page).toHaveURL(/\/admin\/users/)
  await expect(page.getByRole('menuitem', { name: '用户管理' })).toBeVisible()
})

test('错误密码提示错误信息', async ({ page }) => {
  await login(page, 'admin', 'wrong-pass')
  await expect(page.getByText('用户名或密码错误')).toBeVisible()
  await expect(page).toHaveURL(/\/login/)
})

test('退出登录回到登录页', async ({ page }) => {
  await login(page, 'admin', 'admin123')
  await expect(page).toHaveURL(/\/admin\/users/)
  // 用户表格也有"系统管理员"行,须用顶栏下拉触发器精确定位(同 org.spec)
  await page.locator('.ant-dropdown-trigger').click()
  await page.getByText('退出登录').click()
  await expect(page).toHaveURL(/\/login/)
})
