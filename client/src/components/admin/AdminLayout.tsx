import { useState, type ReactNode } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { Button, Tooltip } from '@heroui/react'
import {
  LayoutDashboard, ListOrdered, Store, Bell, ScrollText,
  Key, LogOut, ChevronLeft, ChevronRight, RefreshCw, Menu, FileSpreadsheet, Settings, UserRound, BookOpenText,
} from 'lucide-react'
import ThemeToggle from '../ThemeToggle'
import BrandMark from '../BrandMark'
import { useAuth } from '../../context/AuthContext'

interface NavItem {
  key: string
  label: string
  icon: typeof LayoutDashboard
  path: string
}

const NAV_ITEMS: NavItem[] = [
  { key: 'overview', label: '总览', icon: LayoutDashboard, path: '/admin' },
  { key: 'orders', label: '订单管理', icon: ListOrdered, path: '/admin/orders' },
  { key: 'merchants', label: '商户管理', icon: Store, path: '/admin/merchants' },
  { key: 'notifications', label: '通知队列', icon: Bell, path: '/admin/notifications' },
  { key: 'reconciliation', label: '对账管理', icon: FileSpreadsheet, path: '/admin/reconciliation' },
  { key: 'docs', label: '接入文档', icon: BookOpenText, path: '/admin/docs' },
  { key: 'audit', label: '审计日志', icon: ScrollText, path: '/admin/audit' },
  { key: 'settings', label: '站点设置', icon: Settings, path: '/admin/settings' },
  { key: 'sandbox', label: '开发沙箱', icon: Key, path: '/admin/sandbox' },
]

interface Props {
  children: ReactNode
  refreshing?: boolean
  onRefresh?: () => void
  title?: string
  subtitle?: string
  actions?: ReactNode
}

export default function AdminLayout({
  children,
  refreshing = false,
  onRefresh,
  title,
  subtitle,
  actions,
}: Props) {
  const navigate = useNavigate()
  const location = useLocation()
  const { username, role, logout } = useAuth()
  const [collapsed, setCollapsed] = useState(false)
  const [mobileOpen, setMobileOpen] = useState(false)

  const currentPath = location.pathname

  const isActive = (item: NavItem) => {
    if (item.path === '/admin') return currentPath === '/admin'
    return currentPath.startsWith(item.path)
  }

  const handleNav = (item: NavItem) => {
    navigate(item.path)
    setMobileOpen(false)
  }

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  return (
    <div className="admin-shell flex min-h-screen bg-background text-foreground">
      {/* Mobile overlay */}
      {mobileOpen && (
        <div
          className="fixed inset-0 z-40 bg-black/50 lg:hidden"
          onClick={() => setMobileOpen(false)}
        />
      )}

      {/* Sidebar */}
      <aside
        className={`
          fixed top-0 left-0 z-50 h-screen flex flex-col
          bg-surface border-r border-border transition-all duration-300 shadow-[1px_0_0_rgba(0,0,0,0.01)]
          ${collapsed ? 'w-[72px]' : 'w-[248px]'}
          ${mobileOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'}
        `}
      >
        {/* Logo */}
        <div className="flex h-[72px] shrink-0 items-center gap-3 border-b border-border px-5">
          <BrandMark size="sm" />
          {!collapsed && (
            <div className="overflow-hidden">
              <div className="whitespace-nowrap text-sm font-bold tracking-tight text-foreground">WePay</div>
              <div className="mt-0.5 text-[10px] font-medium tracking-wide text-muted">商户平台</div>
            </div>
          )}
        </div>

        {/* Navigation */}
        <nav className="flex-1 space-y-1 overflow-y-auto px-3 py-4">
          {!collapsed && <div className="mb-2 px-3 text-[10px] font-medium text-muted">管理菜单</div>}
          {NAV_ITEMS.map(item => {
            const active = isActive(item)
            const Icon = item.icon
            const btn = (
              <button
                key={item.key}
                onClick={() => handleNav(item)}
                className={`
                  relative w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-[13px] font-medium transition-colors cursor-pointer
                  ${active
                    ? 'border-l-2 border-emerald-600 bg-surface-secondary text-foreground font-semibold'
                    : 'border-l-2 border-transparent text-muted hover:text-foreground hover:bg-surface-secondary'
                  }
                  ${collapsed ? 'justify-center px-0' : ''}
                `}
              >
                <Icon className={`h-[17px] w-[17px] shrink-0 ${active ? 'text-emerald-600 dark:text-emerald-400' : ''}`} />
                {!collapsed && <span className="whitespace-nowrap">{item.label}</span>}
              </button>
            )

            if (collapsed) {
              return (
                <Tooltip key={item.key}>
                  <Tooltip.Trigger>{btn}</Tooltip.Trigger>
                  <Tooltip.Content className="bg-foreground text-background px-2 py-1 rounded text-xs">
                    {item.label}
                  </Tooltip.Content>
                </Tooltip>
              )
            }
            return btn
          })}
        </nav>

        {/* Sidebar footer */}
        <div className="shrink-0 space-y-1 border-t border-border p-3">
          {collapsed ? (
            <Tooltip>
              <Tooltip.Trigger>
                <Button
                  isIconOnly
                  variant="ghost"
                  onPress={() => setCollapsed(false)}
                  className="h-9 w-full rounded-lg text-muted hover:text-foreground"
                >
                  <ChevronRight className="w-4 h-4" />
                </Button>
              </Tooltip.Trigger>
              <Tooltip.Content className="bg-foreground text-background px-2 py-1 rounded text-xs">展开侧栏</Tooltip.Content>
            </Tooltip>
          ) : (
            <button
              onClick={() => setCollapsed(true)}
              className="flex w-full cursor-pointer items-center gap-3 rounded-lg px-3 py-2 text-xs text-muted hover:bg-surface-secondary hover:text-foreground"
            >
              <ChevronLeft className="w-4 h-4 shrink-0" />
              <span>收起侧栏</span>
            </button>
          )}
        </div>
      </aside>

      {/* Main content area */}
      <div className={`flex min-h-screen flex-1 flex-col transition-all duration-300 ${collapsed ? 'lg:ml-[72px]' : 'lg:ml-[248px]'}`}>
        {/* Top header bar */}
        <header className="sticky top-0 z-30 flex h-[72px] shrink-0 items-center justify-between gap-4 border-b border-border bg-surface px-4 sm:px-6">
          <div className="flex items-center gap-3">
            <Button
              isIconOnly
              variant="ghost"
              onPress={() => setMobileOpen(true)}
              className="h-9 w-9 rounded-lg text-muted lg:hidden"
            >
              <Menu className="w-5 h-5" />
            </Button>
            {title && (
              <div>
                <h1 className="text-[17px] font-semibold tracking-tight text-foreground">{title}</h1>
                {subtitle && <p className="mt-0.5 hidden text-[11px] text-muted sm:block">{subtitle}</p>}
              </div>
            )}
          </div>

          <div className="flex items-center gap-2.5">
            {actions}
            {onRefresh && (
              <Tooltip>
                <Tooltip.Trigger>
                  <Button
                    isIconOnly
                    variant="ghost"
                    onPress={onRefresh}
                    isDisabled={refreshing}
                    aria-label="刷新数据"
                    className="h-9 w-9 min-w-9 rounded-lg border border-border bg-surface text-foreground hover:bg-surface-secondary"
                  >
                    <RefreshCw className={`w-4 h-4 ${refreshing ? 'animate-spin text-emerald-500' : 'text-muted'}`} />
                  </Button>
                </Tooltip.Trigger>
                <Tooltip.Content className="bg-foreground text-background px-2 py-1 rounded text-xs">刷新数据</Tooltip.Content>
              </Tooltip>
            )}

            <ThemeToggle />

            {username && (
              <div className="hidden items-center gap-2 rounded-lg bg-surface-secondary px-2.5 py-1.5 text-xs sm:flex">
                <span className="flex h-6 w-6 items-center justify-center rounded-full bg-surface-tertiary text-muted">
                  <UserRound className="h-3.5 w-3.5" />
                </span>
                <div className="leading-tight">
                  <div className="font-medium text-foreground">{username}</div>
                  {role && <div className="mt-0.5 text-[9px] uppercase tracking-wide text-muted">{role}</div>}
                </div>
              </div>
            )}

            <Button
              onPress={handleLogout}
              variant="ghost"
              className="flex h-9 items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-medium text-muted hover:bg-rose-500/5 hover:text-rose-500"
            >
              <LogOut className="w-3.5 h-3.5" />
              <span className="hidden sm:inline">退出</span>
            </Button>
          </div>
        </header>

        {/* Page content */}
        <main className="mx-auto w-full max-w-[1400px] flex-1 p-4 sm:p-6 xl:p-8">
          {children}
        </main>
      </div>
    </div>
  )
}
