import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import BrandMark from './BrandMark'

/**
 * 路由级认证守卫。
 * 未登录时跳转 /login（携带 redirect 参数，登录成功后跳回原地址）。
 */
export default function RequireAuth({ children }: { children: React.ReactNode }) {
  const { isLoggedIn, loading } = useAuth()
  const navigate = useNavigate()

  useEffect(() => {
    if (!loading && !isLoggedIn) {
      const returnUrl = window.location.href
      navigate(`/login?redirect=${encodeURIComponent(returnUrl)}`, { replace: true })
    }
  }, [loading, isLoggedIn, navigate])

  if (loading || !isLoggedIn) {
    return (
      <div className="min-h-screen bg-background text-foreground flex items-center justify-center">
        <div className="text-center">
          <BrandMark size="lg" className="mb-4 animate-pulse" />
          <p className="text-sm text-muted">正在识别登录状态…</p>
        </div>
      </div>
    )
  }

  return <>{children}</>
}
