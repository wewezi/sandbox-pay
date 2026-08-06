import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Card, Chip } from '@heroui/react'
import {
  ScrollText, Filter, X,
  Shield, User, Globe,
} from 'lucide-react'
import { useAuth } from '../../context/AuthContext'
import { showToast as toast } from '../../utils/toast'
import { api, ApiError } from '../../utils/api'
import AdminLayout from '../../components/admin/AdminLayout'
import Pagination from '../../components/admin/Pagination'
import type { AuditLogsResponse, AuditLogRecord } from '../../components/dashboard/types'

const ACTION_LABELS: Record<string, { label: string; color: string }> = {
  reset_secret: { label: '重置密钥', color: 'bg-amber-500/10 text-amber-500' },
  delete_order: { label: '删除订单', color: 'bg-rose-500/10 text-rose-500' },
  create_merchant: { label: '创建商户', color: 'bg-emerald-500/10 text-emerald-500' },
  update_merchant: { label: '更新商户', color: 'bg-blue-500/10 text-blue-500' },
  activate_merchant: { label: '启用商户', color: 'bg-emerald-500/10 text-emerald-500' },
  deactivate_merchant: { label: '停用商户', color: 'bg-zinc-500/10 text-zinc-500' },
  replay_notification: { label: '重发通知', color: 'bg-violet-500/10 text-violet-500' },
  admin_refund: { label: '管理退款', color: 'bg-rose-500/10 text-rose-500' },
}

const PAGE_SIZE = 20

export default function AuditLogsPage() {
  const navigate = useNavigate()
  const { logout } = useAuth()

  const [items, setItems] = useState<AuditLogRecord[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [refreshing, setRefreshing] = useState(false)

  // Filters
  const [actionFilter, setActionFilter] = useState('')
  const [actorFilter, setActorFilter] = useState('')

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true)
    try {
      const params = new URLSearchParams()
      params.set('page', String(page))
      params.set('pageSize', String(PAGE_SIZE))
      if (actionFilter) params.set('action', actionFilter)
      if (actorFilter) params.set('actor', actorFilter)

      const data = await api.get<AuditLogsResponse>(`/admin/audit-logs?${params.toString()}`)
      if (data) {
        setItems(data.items ?? [])
        setTotal(data.total ?? 0)
      }
      if (isRefresh) toast.success('审计日志已刷新')
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
  }, [page, actionFilter, actorFilter, logout, navigate])

  useEffect(() => {
    const initialLoad = setTimeout(load, 0)
    return () => clearTimeout(initialLoad)
  }, [load])

  const handleRefresh = () => load(true)

  const clearFilters = () => {
    setActionFilter('')
    setActorFilter('')
    setPage(1)
  }

  const hasFilters = actionFilter || actorFilter

  const formatDate = (dateStr?: string) => {
    if (!dateStr) return '—'
    const d = new Date(dateStr)
    return d.toLocaleString('zh-CN', {
      year: 'numeric', month: '2-digit', day: '2-digit',
      hour: '2-digit', minute: '2-digit', second: '2-digit',
    })
  }

  const getActionInfo = (action: string) => {
    return ACTION_LABELS[action] || { label: action, color: 'bg-surface text-muted' }
  }

  return (
    <AdminLayout
      title="审计日志"
      subtitle={`敏感操作追溯记录 · 共 ${total} 条`}
      refreshing={refreshing}
      onRefresh={handleRefresh}
    >
      {/* Filters */}
      <Card className="p-4 mb-6">
        <Card.Content className="p-0">
          <div className="flex items-center gap-3 flex-wrap">
            <Filter className="w-4 h-4 text-muted" />
            <span className="text-xs font-semibold text-foreground">筛选</span>

            <select
              value={actionFilter}
              onChange={e => { setActionFilter(e.target.value); setPage(1) }}
              className="h-8 rounded-lg border border-border bg-surface text-xs text-foreground px-2.5 focus:outline-none focus:border-emerald-500 transition-colors cursor-pointer"
            >
              <option value="">全部操作</option>
              {Object.entries(ACTION_LABELS).map(([key, { label }]) => (
                <option key={key} value={key}>{label}</option>
              ))}
            </select>

            <input
              type="text"
              value={actorFilter}
              onChange={e => { setActorFilter(e.target.value); setPage(1) }}
              placeholder="操作者..."
              className="h-8 rounded-lg border border-border bg-surface text-xs text-foreground px-2.5 w-40 focus:outline-none focus:border-emerald-500 transition-colors"
            />

            {hasFilters && (
              <Button
                size="sm"
                variant="ghost"
                onPress={clearFilters}
                className="text-[10px] text-muted hover:text-foreground flex items-center gap-1 px-2 py-0.5 h-auto min-h-0 cursor-pointer"
              >
                <X className="w-3 h-3" /> 清除
              </Button>
            )}
          </div>
        </Card.Content>
      </Card>

      {/* Audit logs */}
      <Card className="p-6">
        <Card.Content className="p-0">
          <div className="overflow-x-auto">
            {items.length === 0 ? (
              <div className="py-12 text-center">
                <ScrollText className="w-12 h-12 text-muted mx-auto mb-3" />
                <div className="text-sm text-muted">{hasFilters ? '没有匹配的日志记录' : '暂无审计日志'}</div>
              </div>
            ) : (
              <table className="w-full text-left">
                <thead>
                  <tr className="border-b border-border">
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">ID</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">操作</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">操作者</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">目标类型</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">目标 ID</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">IP</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">详情</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">时间</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map(log => {
                    const actionInfo = getActionInfo(log.action)
                    return (
                      <tr key={log.id} className="border-b border-border/60 last:border-none hover:bg-surface/50 transition-colors">
                        <td className="py-3.5 font-mono text-xs text-muted">#{log.id}</td>
                        <td className="py-3.5">
                          <Chip variant="soft" size="sm" className={`${actionInfo.color} font-semibold border-none`}>
                            <span className="flex items-center gap-1 px-1 text-[10px] tracking-wide">
                              <Shield className="w-3 h-3" /> {actionInfo.label}
                            </span>
                          </Chip>
                        </td>
                        <td className="py-3.5">
                          <span className="flex items-center gap-1.5 text-xs text-foreground">
                            <User className="w-3 h-3 text-muted" />
                            {log.actor}
                          </span>
                        </td>
                        <td className="py-3.5 font-mono text-xs text-muted">{log.targetType || '—'}</td>
                        <td className="py-3.5 font-mono text-xs text-foreground">{log.targetId || '—'}</td>
                        <td className="py-3.5">
                          <span className="flex items-center gap-1 text-xs text-muted font-mono">
                            <Globe className="w-3 h-3" />
                            {log.ip || '—'}
                          </span>
                        </td>
                        <td className="py-3.5">
                          {log.detail ? (
                            <span className="font-mono text-[10px] text-muted max-w-[200px] truncate block">
                              {JSON.stringify(log.detail)}
                            </span>
                          ) : (
                            <span className="text-[10px] text-muted">—</span>
                          )}
                        </td>
                        <td className="py-3.5 text-xs text-muted font-mono whitespace-nowrap">
                          {formatDate(log.createdAt)}
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
