import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import LLM from './LLM'

// jsdom 缺 matchMedia,antd 响应式组件需要
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
})

const api = vi.hoisted(() => ({
  listProviders: vi.fn(),
  createProvider: vi.fn(),
  updateProvider: vi.fn(),
  deleteProvider: vi.fn(),
  listModels: vi.fn(),
  createModel: vi.fn(),
  updateModel: vi.fn(),
  deleteModel: vi.fn(),
  setCurrentModel: vi.fn(),
  currentModel: vi.fn(),
}))

vi.mock('../../api/llm', () => api)

const RAW_KEY = 'sk-1234567890abcd'

// 后端 DTO 不含 key 原文;故意在 mock 里带上,验证页面只消费掩码字段
const provider = {
  id: 1,
  name: 'deepseek',
  base_url: 'https://api.deepseek.com/anthropic',
  api_key_masked: 'sk-***abcd',
  has_key: true,
  model_count: 1,
  api_key: RAW_KEY,
}

const model = {
  id: 11,
  provider_id: 1,
  name: 'deepseek-flash',
  context_window: 384000,
  max_tokens: 16384,
  is_current: true,
}

describe('LLM 大模型设置页', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    api.listProviders.mockResolvedValue([provider])
    api.listModels.mockResolvedValue([model])
    api.currentModel.mockResolvedValue({
      provider: { id: 1, name: 'deepseek', base_url: 'https://api.deepseek.com/anthropic', has_key: true },
      model,
    })
  })

  it('当前模型卡片展示 provider 名称与模型名', async () => {
    render(<LLM />)

    const card = (await screen.findByText('当前模型')).closest('.ant-card') as HTMLElement
    expect(card).not.toBeNull()
    await waitFor(() => expect(within(card).getByText('deepseek')).toBeTruthy())
    expect(within(card).getByText('deepseek-flash')).toBeTruthy()
    expect(within(card).getByText('384000')).toBeTruthy()
    expect(within(card).getByText('16384')).toBeTruthy()
    expect(within(card).getByText('已配置')).toBeTruthy()
  })

  it('Provider 表格只展示掩码,不渲染 key 原文', async () => {
    render(<LLM />)

    expect(await screen.findByText('sk-***abcd')).toBeTruthy()
    expect(screen.queryByText(RAW_KEY)).toBeNull()
  })

  it('编辑 Provider 时 API Key 输入框留空,不回显原文', async () => {
    render(<LLM />)

    const providerTable = screen.getAllByRole('table')[0]
    await waitFor(() => expect(within(providerTable).getByText('deepseek')).toBeTruthy())
    // antd 在两个汉字的按钮文本间自动插入空格(编 辑),用正则匹配
    fireEvent.click(within(providerTable).getByRole('button', { name: /编\s*辑/ }))

    const keyInput = (await screen.findByLabelText('API Key')) as HTMLInputElement
    expect(keyInput.value).toBe('')
    expect(screen.queryByText(RAW_KEY)).toBeNull()
  })

  it('无当前模型时提示未配置', async () => {
    api.currentModel.mockResolvedValue({ provider: null, model: null })
    api.listProviders.mockResolvedValue([])
    api.listModels.mockResolvedValue([])

    render(<LLM />)

    expect(await screen.findByText(/未配置\(请新建 Provider 与模型并设为当前\)/)).toBeTruthy()
    await waitFor(() => expect(api.currentModel).toHaveBeenCalled())
  })
})
