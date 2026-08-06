import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Card, Chip } from '@heroui/react'
import {
  Bell, CheckCircle2, Clock, AlertCircle, RotateCcw,
  Filter, X,
} from 'lucide-react'
import { useAuth } from '../../context/AuthContext'
import { showToast as toast } from '../../utils/toast'
import { api, ApiError } from '../../utils/api'
import AdminLayout from '../../components/admin/AdminLayout'
import Pagination from '../../components/admin/Pagination'
import { NOTIFY_STATUS_MAP } from '../../components/admin/status-helpers'
import type { NotificationsResponse, NotificationRecord } from '../../components/dashboard/types'

const STATUS_OPTIONS = [
  { value: '', label: '全部状态' },
  { value: '0', label: '待发送' },
  { value: '1', label: '已成功' },
  { value: '2', label: '失败' },
  { value: '3', label: '已耗尽' },
]

const NOTIFY_ICON_MAP: Record<number, typeof CheckCircle2> = {
  0: Clock,
  1: CheckCircle2,
  2: AlertCircle,
  3: AlertCircle,
}

const PAGE_SIZE = 20

export default function NotificationsPage() {
  const navigate = useNavigate()
  const { logout } = useAuth()

  const [items, setItems] = useState<NotificationRecord[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [statusFilter, setStatusFilter] = useState('')
  const [refreshing, setRefreshing] = useState(false)
  const [replayingId, setReplayingId] = useState<number | null>(null)

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true)
    try {
      const params = new URLSearchParams()
      params.set('page', String(page))
      params.set('pageSize', String(PAGE_SIZE))
      if (statusFilter) params.set('status', statusFilter)

      const data = await api.get<NotificationsResponse>(`/admin/notifications?${params.toString()}`)
      if (data) {
        setItems(data.items ?? [])
        setTotal(data.total ?? 0)
      }
      if (isRefresh) toast.success('通知数据已刷新')
    } catch (err: unknown) {
      if (err instanceof ApiError && err.status === 401) {
        logout()
        navigate('/login')
      } else {
        toast.error('加载失败: ' + (err instanceof Error ? err.message : '网络异常'))
      }
    } finally {
      if (isRefresh) setRefreshing(false)
    }
  }, [page, statusFilter, logout, navigate])

  useEffect(() => {
    const initialLoad = setTimeout(load, 0)
    return () => clearTimeout(initialLoad)
  }, [load])

  const handleRefresh = () => load(true)

  const handleReplay = async (item: NotificationRecord) => {
    setReplayingId(item.id)
    try {
      await api.post(`/admin/notifications/${item.id}/replay`)
      toast.success(`通知 #${item.id} 已重新入队`)
      load()
    } catch (err: unknown) {
      toast.error('重发失败: ' + (err instanceof Error ? err.message : '网络异常'))
    } finally {
      setReplayingId(null)
    }
  }

  const formatDate = (dateStr?: string) => {
    if (!dateStr) return '—'
    const d = new Date(dateStr)
    return d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
  }

  return (
    <AdminLayout
      title="通知队列"
      subtitle={`异步回调通知状态监控 · 共 ${total} 条`}
      refreshing={refreshing}
      onRefresh={handleRefresh}
    >
      {/* Filter bar */}
      <Card className="p-4 mb-6">
        <Card.Content className="p-0">
          <div className="flex items-center gap-3 flex-wrap">
            <Filter className="w-4 h-4 text-muted" />
            <span className="text-xs font-semibold text-foreground">状态筛选</span>
            <div className="flex items-center gap-1 bg-surface p-1 rounded-xl">
              {STATUS_OPTIONS.map(o => (
                <Button
                  key={o.value}
                  size="sm"
                  variant={statusFilter === o.value ? 'primary' : 'ghost'}
                  onPress={() => { setStatusFilter(o.value); setPage(1) }}
                  className={`px-3 py-1 rounded-lg text-xs font-medium transition-colors h-7 cursor-pointer ${
                    statusFilter === o.value
                      ? 'bg-emerald-500 text-white font-semibold shadow-sm'
                      : 'text-muted hover:text-foreground'
                  }`}
                >
                  {o.label}
                </Button>
              ))}
            </div>
            {statusFilter && (
              <Button
                size="sm"
                variant="ghost"
                onPress={() => { setStatusFilter(''); setPage(1) }}
                className="text-[10px] text-muted hover:text-foreground flex items-center gap-1 px-2 py-0.5 h-auto min-h-0 cursor-pointer"
              >
                <X className="w-3 h-3" /> 清除
              </Button>
            )}
          </div>
        </Card.Content>
      </Card>

      {/* Notifications table */}
      <Card className="p-6">
        <Card.Content className="p-0">
          <div className="overflow-x-auto">
            {items.length === 0 ? (
              <div className="py-12 text-center">
                <Bell className="w-12 h-12 text-muted mx-auto mb-3" />
                <div className="text-sm text-muted">暂无通知记录</div>
              </div>
            ) : (
              <table className="w-full text-left">
                <thead>
                  <tr className="border-b border-border">
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">ID</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">订单号</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">回调地址</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">状态</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">重试次数</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">最后错误</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">时间</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide text-right">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map(n => {
                    const st = NOTIFY_STATUS_MAP[n.status] || NOTIFY_STATUS_MAP[0]
                    const Icon = NOTIFY_ICON_MAP[n.status] || Clock
                    return (
                      <tr key={n.id} className="border-b border-border/60 last:border-none hover:bg-surface/50 transition-colors">
                        <td className="py-3.5 font-mono text-xs text-muted">#{n.id}</td>
                        <td className="py-3.5 font-mono text-xs text-foreground font-medium">{n.orderNo}</td>
                        <td className="py-3.5 font-mono text-[10px] text-muted max-w-[200px] truncate">{n.url}</td>
                        <td className="py-3.5">
                          <Chip variant="soft" size="sm" className={`${st.color} font-semibold border-none`}>
                            <span className="flex items-center gap-1 px-1 text-[10px] tracking-wide">
                              <Icon className="w-3 h-3" /> {st.label}
                            </span>
                          </Chip>
                        </td>
                        <td className="py-3.5 font-mono text-xs text-foreground">{n.retryCount}</td>
                        <td className="py-3.5 text-[10px] text-rose-400 max-w-[180px] truncate">{n.lastError || '—'}</td>
                        <td className="py-3.5 text-xs text-muted font-mono">{formatDate(n.lastAttemptAt || n.createdAt)}</td>
                        <td className="py-3.5 text-right">
                          {(n.status === 2 || n.status === 3) && (
                            <Button
                              size="sm"
                              variant="ghost"
                              onPress={() => handleReplay(n)}
                              isDisabled={replayingId === n.id}
                              className="px-2.5 py-1 text-[10px] font-medium text-amber-500 hover:bg-amber-500/10 rounded-lg transition-colors h-auto min-h-0 min-w-0 flex items-center gap-1 cursor-pointer ml-auto"
                            >
                              <RotateCcw className={`w-3 h-3 ${replayingId === n.id ? 'animate-spin' : ''}`} />
                              重发
                            </Button>
                          )}
                          {n.status !== 2 && n.status !== 3 && (
                            <span className="text-[10px] text-muted">—</span>
                          )}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            )}
          </div>

          {/* Pagination */}
          <Pagination page={page} total={total} pageSize={PAGE_SIZE} onChange={setPage} />
        </Card.Content>
      </Card>
    </AdminLayout>
  )
}
