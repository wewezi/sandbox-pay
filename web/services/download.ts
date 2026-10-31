import { clearToken, getToken } from './api';
import { currentRoute, navigate } from './router';

export async function downloadAPI(path: string, filename: string, options: { signal?: AbortSignal } = {}) {
  const token = getToken();
  const response = await fetch('/v1/api' + path, { credentials: 'include', headers: token ? { Authorization: `Bearer ${token}` } : {}, redirect: 'error', signal: options.signal });
  options.signal?.throwIfAborted();
  if (response.status === 401 && currentRoute()?.admin && token === getToken()) { clearToken(); navigate('/login?redirect=' + encodeURIComponent(location.pathname + location.search + location.hash), { replace: true }); throw new Error('登录已过期'); }
  if (!response.ok) { const error = await response.json().catch(() => null) as { msg?: string } | null; throw new Error(error?.msg || '下载失败'); }
  const blob = await response.blob();
  options.signal?.throwIfAborted();
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a'); link.href = url; link.download = filename; link.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}
