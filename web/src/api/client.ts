export class ApiError extends Error {
  constructor(public code: number, message: string) {
    super(message)
  }
}

const TOKEN_KEY = 'gbi_token'

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(t: string | null): void {
  if (t) localStorage.setItem(TOKEN_KEY, t)
  else localStorage.removeItem(TOKEN_KEY)
}

export async function api<T>(
  path: string,
  opts: { method?: string; body?: unknown } = {},
): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  const token = getToken()
  if (token) headers.Authorization = `Bearer ${token}`

  let res: Response
  try {
    res = await fetch(`/api/v1${path}`, {
      method: opts.method ?? 'GET',
      headers,
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
    })
  } catch {
    // 网络层失败(离线/DNS/连接重置)统一为 ApiError 契约,不裸抛 TypeError
    throw new ApiError(0, '网络错误,请检查连接')
  }
  const json = await res
    .json()
    .catch(() => ({ code: res.status, message: res.statusText }))

  if (res.status === 401) {
    setToken(null)
    if (!location.pathname.startsWith('/login')) location.href = '/login'
  }
  if (!res.ok || json.code !== 0) {
    throw new ApiError(json.code ?? res.status, json.message ?? '请求失败')
  }
  return json.data as T
}
