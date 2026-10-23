import { afterEach, describe, expect, it, vi } from 'vitest'
import { listColumnValues, listRows } from './subjects'

// 断言 fetch 收到的 URL;client.api 统一在 /api/v1 前缀下。
function mockFetch() {
  const fetchMock = vi.fn(
    async (..._args: unknown[]) =>
      new Response(JSON.stringify({ code: 0, data: { columns: [], total: 0, rows: [] } }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
  )
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function lastUrl(fetchMock: ReturnType<typeof mockFetch>): URL {
  const raw = fetchMock.mock.calls.at(-1)?.[0] as string
  return new URL(raw, 'http://localhost')
}

describe('subjects api', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    localStorage.clear()
  })

  it('listRows 无筛选时不携带 filters 参数', async () => {
    const f = mockFetch()
    await listRows(1, 2, 3, 20)
    const u = lastUrl(f)
    expect(u.pathname).toBe('/api/v1/subjects/1/tables/2/rows')
    expect(u.searchParams.get('page')).toBe('3')
    expect(u.searchParams.get('size')).toBe('20')
    expect(u.searchParams.get('filters')).toBeNull()
  })

  it('listRows 携带 filters JSON(列名 → 候选值数组)', async () => {
    const f = mockFetch()
    await listRows(1, 2, 1, 50, { region: ['华东'], month: ['2026-01', '2026-02'] })
    const u = lastUrl(f)
    expect(u.searchParams.get('size')).toBe('50')
    expect(JSON.parse(u.searchParams.get('filters') ?? '')).toEqual({
      region: ['华东'],
      month: ['2026-01', '2026-02'],
    })
  })

  it('listColumnValues 编码列名并指向 values 端点', async () => {
    const f = mockFetch()
    await listColumnValues(1, 2, 'a b')
    const u = lastUrl(f)
    expect(u.pathname).toBe('/api/v1/subjects/1/tables/2/columns/a%20b/values')
  })
})
