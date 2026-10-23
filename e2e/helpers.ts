import { Page } from '@playwright/test'

export async function login(page: Page, username: string, password: string) {
  await page.goto('/login')
  await page.getByLabel('用户名').fill(username)
  await page.getByLabel('密码').fill(password)
  // antd 对两字中文按钮自动插空格("登 录"),用正则容忍
  await page.getByRole('button', { name: /登\s*录/ }).click()
}
