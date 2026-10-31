import { currentRoute, navigate } from './router';

const base = '/v1/api';
const tokenKey = 'token';
export const getToken = () => localStorage.getItem(tokenKey);
export const setToken = (value: string) => localStorage.setItem(tokenKey, value);
export const clearToken = () => localStorage.removeItem(tokenKey);
type RequestOptions = { signal?: AbortSignal };
type Envelope = { code?: unknown; data?: unknown; msg?: unknown; message?: unknown };

function pickMessage(json: Envelope | null) {
  const message = json?.message ?? json?.msg;
  if (Array.isArray(message)) return message.join('; ');
  return typeof message === 'string' && message !== 'success' ? message : '';
}

// T describes the Go DTO at the call site; it does not validate data at runtime.
async function request<T>(method: string, path: string, body?: unknown, options: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  let response: Response;
  try {
    response = await fetch(base + path, {
      method, headers, credentials: 'include', signal: options.signal,
      redirect: 'manual',
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch (error) {
    if (isAbort(error)) throw error;
    throw new Error('网络请求失败，检查服务是否在运行');
  }
  options.signal?.throwIfAborted();
  if (response.type === 'opaqueredirect' || (response.status >= 300 && response.status < 400)) {
    throw new Error(import.meta.env.DEV
      ? 'API 请求被重定向，请检查 VITE_API_TARGET 是否指向 Mintweft 的 Go 后端'
      : '接口返回了意外跳转，请检查服务配置');
  }
  if (response.status === 401 && currentRoute()?.admin && token === getToken()) {
    clearToken();
    navigate('/login?redirect=' + encodeURIComponent(location.pathname + location.search + location.hash), { replace: true });
    throw new Error('登录已过期');
  }
  let json: Envelope | null = null;
  try {
    const parsed: unknown = await response.json();
    if (typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed)) json = parsed;
  } catch (error) {
    if (isAbort(error)) throw error;
  }
  options.signal?.throwIfAborted();
  if (!response.ok) throw new Error(pickMessage(json) || `请求失败 (${response.status})`);
  if (!json || !('code' in json)) throw new Error('服务响应格式错误');
  if (json.code !== 200) throw new Error(pickMessage(json) || '请求失败');
  return json.data as T;
}

export const api = {
  get: <T = unknown>(path: string, options?: RequestOptions) => request<T>('GET', path, undefined, options),
  post: <T = unknown>(path: string, body?: unknown, options?: RequestOptions) => request<T>('POST', path, body ?? {}, options),
  patch: <T = unknown>(path: string, body?: unknown, options?: RequestOptions) => request<T>('PATCH', path, body ?? {}, options),
  put: <T = unknown>(path: string, body?: unknown, options?: RequestOptions) => request<T>('PUT', path, body ?? {}, options),
  del: <T = unknown>(path: string, options?: RequestOptions) => request<T>('DELETE', path, undefined, options),
};
export const isAbort = (error: unknown) => error instanceof DOMException && error.name === 'AbortError';

export function requireAuth() {
  if (getToken()) return true;
  navigate('/login?redirect=' + encodeURIComponent(location.pathname + location.search + location.hash), { replace: true });
  return false;
}
export async function logout() {
  const session = getToken();
  try { await api.post('/auth/logout'); } catch { /* local logout still works */ }
  if (getToken() === session) {
    clearToken();
    navigate('/login', { replace: true });
  }
}
