export class ApiError extends Error {
  status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(init.headers ?? {}),
    },
    ...init,
  })

  if (response.status === 401 && path !== '/api/auth/login') {
    window.dispatchEvent(new CustomEvent('sevrtree:unauthorized'))
  }

  const text = await response.text()
  let payload: unknown = null
  if (text) {
    try {
      payload = JSON.parse(text)
    } catch {
      payload = null
    }
  }

  if (!response.ok) {
    const message =
      payload && typeof payload === 'object' && 'error' in payload
        ? String((payload as { error: unknown }).error)
        : `请求失败（HTTP ${response.status}）`
    throw new ApiError(message, response.status)
  }

  return payload as T
}

export const api = {
  get<T>(path: string): Promise<T> {
    return request<T>(path)
  },
  post<T>(path: string, body?: unknown): Promise<T> {
    return request<T>(path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) })
  },
  put<T>(path: string, body?: unknown): Promise<T> {
    return request<T>(path, { method: 'PUT', body: body === undefined ? undefined : JSON.stringify(body) })
  },
  del<T>(path: string): Promise<T> {
    return request<T>(path, { method: 'DELETE' })
  },
}
