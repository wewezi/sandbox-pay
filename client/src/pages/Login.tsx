import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { ArrowLeft, Loader2, Lock, LogIn, ShieldCheck, User } from 'lucide-react'
import BrandMark from '../components/BrandMark'
import { useAuth } from '../context/AuthContext'
import { api, ApiError } from '../utils/api'

interface LoginResult {
  token: string
  tokenType: string
  username: string
  role: string
}

/**
 * 管理员账号密码登录。
 * 登录成功后跳转回 RequireAuth 拦截前的地址（redirect 参数），否则进入后台首页。
 */
export default function Login() {
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const { login } = useAuth()

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  const resolveRedirect = () => {
    const raw = params.get('redirect')
    if (!raw) return '/admin'
    try {
      // redirect 是完整 URL（RequireAuth 用 window.location.href 编码），
      // 只取同源的 path，避免开放重定向到外部站点。
      const url = new URL(raw, window.location.origin)
      if (url.origin !== window.location.origin) return '/admin'
      return `${url.pathname}${url.search}${url.hash}` || '/admin'
    } catch {
      return raw.startsWith('/') ? raw : '/admin'
    }
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (submitting) return
    setError('')
    setSubmitting(true)
    try {
      const result = await api.post<LoginResult>('/auth/login', {
        username: username.trim(),
        password,
      })
      login({ token: result.token, username: result.username, role: result.role })
      navigate(resolveRedirect(), { replace: true })
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.status === 0 ? '网络异常，请稍后重试' : err.message)
      } else {
        setError('登录失败，请稍后重试')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex min-h-screen flex-col bg-background text-foreground">
      <header className="border-b border-border bg-surface">
        <div className="mx-auto flex h-[72px] max-w-6xl items-center px-5 sm:px-8">
          <Link to="/" className="flex items-center gap-3">
            <BrandMark size="sm" />
            <div>
              <div className="text-sm font-bold">WePay</div>
              <div className="mt-0.5 text-[10px] text-muted">商户平台</div>
            </div>
          </Link>
        </div>
      </header>

      <main className="flex flex-1 items-center justify-center px-5 py-12">
        <div className="w-full max-w-[420px] rounded-lg border border-border bg-surface p-7 sm:p-9">
          <div className="text-center">
            <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
              <Lock className="h-7 w-7" />
            </div>
            <h1 className="mt-5 text-xl font-semibold">登录商户后台</h1>
            <p className="mt-2 text-[13px] leading-6 text-muted">
              使用管理员账号密码登录 WePay 商户平台。
            </p>
          </div>

          <form className="mt-7 space-y-4" onSubmit={handleSubmit}>
            <label className="block">
              <span className="mb-1.5 block text-xs font-medium text-muted">用户名</span>
              <div className="flex items-center rounded-lg border border-border bg-surface transition-all focus-within:border-emerald-500 focus-within:ring-2 focus-within:ring-emerald-500/30">
                <User className="ml-3 h-4 w-4 shrink-0 text-muted" />
                <input
                  type="text"
                  autoComplete="username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  disabled={submitting}
                  required
                  className="w-full bg-transparent px-3 py-2.5 text-sm focus:outline-none disabled:opacity-50"
                  placeholder="admin"
                />
              </div>
            </label>

            <label className="block">
              <span className="mb-1.5 block text-xs font-medium text-muted">密码</span>
              <div className="flex items-center rounded-lg border border-border bg-surface transition-all focus-within:border-emerald-500 focus-within:ring-2 focus-within:ring-emerald-500/30">
                <Lock className="ml-3 h-4 w-4 shrink-0 text-muted" />
                <input
                  type="password"
                  autoComplete="current-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  disabled={submitting}
                  required
                  className="w-full bg-transparent px-3 py-2.5 text-sm focus:outline-none disabled:opacity-50"
                  placeholder="••••••••"
                />
              </div>
            </label>

            {error && (
              <div
                role="alert"
                className="rounded-lg bg-rose-500/10 px-3 py-2.5 text-xs leading-5 text-rose-600 dark:text-rose-400"
              >
                {error}
              </div>
            )}

            <button
              type="submit"
              disabled={submitting || !username.trim() || !password}
              className="flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-emerald-500 text-xs font-semibold text-white transition-colors hover:bg-emerald-600 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {submitting ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <LogIn className="h-4 w-4" />
              )}
              {submitting ? '登录中…' : '登录'}
            </button>
          </form>

          <div className="mt-6 rounded-lg bg-[#f5f6f7] px-4 py-3 dark:bg-white/[0.04]">
            <div className="flex items-start gap-2.5 text-[11px] leading-5 text-muted">
              <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
              管理后台受身份验证保护，未登录用户无法访问商户数据。
            </div>
          </div>

          <Link
            to="/"
            className="mt-4 flex h-11 w-full items-center justify-center gap-2 rounded-lg border border-border bg-surface text-xs font-semibold transition-colors hover:bg-surface-secondary"
          >
            <ArrowLeft className="h-4 w-4" /> 返回产品首页
          </Link>
        </div>
      </main>

      <footer className="pb-8 text-center text-[10px] text-muted">WePay · 安全支付接入服务</footer>
    </div>
  )
}
