import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError, setToken } from './client'

beforeEach(() => {
  localStorage.clear()
  vi.restoreAllMocks()
})

describe('api client', () => {
  it('解包 {code:0,data} 并附带 JWT', async () => {
    setToken('t1')
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ code: 0, message: 'ok', data: { hello: 1 } }),
    }) as unknown as typeof fetch

    const data = await api<{ hello: number }>('/x')
    expect(data.hello).toBe(1)
    const [, init] = (global.fetch as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(init.headers.Authorization).toBe('Bearer t1')
    expect((global.fetch as ReturnType<typeof vi.fn>).mock.calls[0][0]).toBe('/api/v1/x')
  })

  it('code≠0 抛 ApiError 并带服务端 message', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ code: 1001, message: '用户名或密码错误' }),
    }) as unknown as typeof fetch

    await expect(api('/x')).rejects.toThrow('用户名或密码错误')
    await expect(api('/x')).rejects.toBeInstanceOf(ApiError)
  })
})
