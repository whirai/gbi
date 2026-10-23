import { Browser, Page, expect, test } from '@playwright/test'
import { login } from '../helpers'

// 前置:playwright.config 以 GBI_LLM_* 播种 provider「deepseek」/ 模型「fake-model」并设为当前。
// 本文件 serial 且结束时必须把种子模型恢复为当前,否则其后 studio 等用例会打错模型。
test.describe('大模型设置', () => {
  test.describe.configure({ mode: 'serial' })
  let page: Page

  test.beforeAll(async ({ browser }: { browser: Browser }) => {
    page = await browser.newPage()
    await login(page, 'admin', 'admin123')
    await expect(page).toHaveURL(/\/admin\/users/)
  })
  test.afterAll(async () => { await page.close() })

  // 页面仅有一个 Card(当前模型摘要);两张 Table 依次为 Provider、模型。
  // 用表格作用域定位 row:antd Descriptions 也是 <table>,按 role=row 全局匹配会与摘要行冲突。
  const currentCard = () => page.locator('.ant-card')
  const providerTable = () => page.locator('.ant-table').nth(0)
  const modelTable = () => page.locator('.ant-table').nth(1)
  const providerRow = (name: RegExp) => providerTable().getByRole('row', { name })
  const modelRow = (name: RegExp) => modelTable().getByRole('row', { name })

  test('当前模型:env 播种可见', async () => {
    await page.goto('/admin/llm')
    const card = currentCard()
    await expect(card.locator('.ant-card-head-title')).toHaveText('当前模型')
    await expect(card.getByText('fake-model')).toBeVisible()
    await expect(card.getByText('http://localhost:9999')).toBeVisible()
  })

  test('Provider/Model:新建、设为当前、删除保护、恢复种子', async () => {
    await page.goto('/admin/llm')
    const card = currentCard()

    // 新建 provider
    await page.getByRole('button', { name: /新建 Provider/ }).click()
    await page.getByLabel('名称').fill('e2e_provider')
    await page.getByLabel(/Base URL/).fill('http://localhost:9999')
    await page.getByLabel('API Key').fill('sk-e2e-12345678')
    await page.getByRole('button', { name: /确\s*定/ }).click()
    const e2eProvider = providerRow(/e2e_provider/)
    await expect(e2eProvider).toBeVisible()

    // 选中该 provider 并新建 model
    await e2eProvider.getByRole('button', { name: '管理模型' }).click()
    await page.getByRole('button', { name: /新建模型/ }).click()
    await page.getByLabel('模型名').fill('e2e-model')
    await page.getByLabel('上下文窗口').fill('200000')
    await page.getByLabel('输出上限').fill('16384')
    await page.getByRole('button', { name: /确\s*定/ }).click()
    const e2eModel = modelRow(/e2e-model/)
    await expect(e2eModel).toBeVisible()

    // 设为当前:摘要卡切到 e2e provider/model
    await e2eModel.getByRole('button', { name: '设为当前' }).click()
    await expect(card.getByText('e2e_provider')).toBeVisible()
    await expect(card.getByText('e2e-model')).toBeVisible()

    // 删除保护:当前模型拒删(模型仍在,当前未变)
    await e2eModel.getByRole('button', { name: /删\s*除/ }).click()
    await page.locator('.ant-popover:visible').getByRole('button', { name: /确\s*定/ }).click()
    await expect(page.getByText(/不能删除当前模型/)).toBeVisible()
    await expect(e2eModel).toBeVisible()
    await expect(card.getByText('e2e-model')).toBeVisible()

    // 恢复种子模型为当前:先切回种子 provider 的模型列表(fake-model 当前时按钮不可点,
    // 上方刚切换过,此刻它非当前)
    await providerRow(/deepseek/).getByRole('button', { name: '管理模型' }).click()
    const fakeModel = modelRow(/fake-model/)
    await expect(fakeModel).toBeVisible()
    await fakeModel.getByRole('button', { name: '设为当前' }).click()
    await expect(card.getByText('fake-model')).toBeVisible()

    // 再删 e2e 模型与 provider
    await e2eProvider.getByRole('button', { name: '管理模型' }).click()
    await expect(e2eModel).toBeVisible()
    await e2eModel.getByRole('button', { name: /删\s*除/ }).click()
    await page.locator('.ant-popover:visible').getByRole('button', { name: /确\s*定/ }).click()
    await expect(e2eModel).toHaveCount(0)

    await e2eProvider.getByRole('button', { name: /删\s*除/ }).click()
    await page.locator('.ant-popover:visible').getByRole('button', { name: /确\s*定/ }).click()
    await expect(e2eProvider).toHaveCount(0)

    // 恢复确认:重新加载后当前仍为种子 fake-model,不影响后续用例
    await page.reload()
    await expect(currentCard().getByText('fake-model')).toBeVisible()
    await expect(currentCard().getByText('deepseek')).toBeVisible()
  })
})
