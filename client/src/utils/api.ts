const BASE = '/v1/api'

interface ApiResponse<T = unknown> {
  code: number
  msg: string
  data: T
}

export class ApiError extends Error {
  status: number
  code: number
  constructor(message: string, status: number, code: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

function getToken(): string | null {
  return localStorage.getItem('token')
}

export function setToken(token: string) {
  localStorage.setItem('token', token)
}

export function clearToken() {
  localStorage.removeItem('token')
}

async function request<T = unknown>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const token = getToken()
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string>),
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  let res: Response
  try {
    res = await fetch(`${BASE}${path}`, { ...options, headers })
  } catch (err) {
    // Network failure (DNS, offline, CORS preflight, etc.) — surface as status 0.
    throw new ApiError(
      err instanceof Error ? err.message : 'Network error',
      0,
      0,
    )
  }

  let json: ApiResponse<T> | null = null
  try {
    json = (await res.json()) as ApiResponse<T>
  } catch {
    // Non-JSON response (e.g. HTML 401 page from a proxy).
  }

  if (!res.ok) {
    throw new ApiError(
      json?.msg || `HTTP ${res.status}`,
      res.status,
      json?.code ?? res.status,
    )
  }

  if (!json) {
    throw new ApiError('Invalid response', res.status, -1)
  }

  if (json.code !== 200) {
    throw new ApiError(json.msg || 'Request failed', res.status, json.code)
  }
  return json.data
}

export const api = {
  get<T = unknown>(path: string) {
    return request<T>(path)
  },
  post<T = unknown>(path: string, body?: unknown) {
    return request<T>(path, {
      method: 'POST',
      body: body ? JSON.stringify(body) : undefined,
    })
  },
  delete<T = unknown>(path: string, body?: unknown) {
    return request<T>(path, {
      method: 'DELETE',
      body: body ? JSON.stringify(body) : undefined,
    })
  },
  put<T = unknown>(path: string, body?: unknown) {
    return request<T>(path, {
      method: 'PUT',
      body: body ? JSON.stringify(body) : undefined,
    })
  },
  patch<T = unknown>(path: string, body?: unknown) {
    return request<T>(path, {
      method: 'PATCH',
      body: body ? JSON.stringify(body) : undefined,
    })
  },
}
