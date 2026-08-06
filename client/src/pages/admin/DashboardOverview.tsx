import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { Card, Button, Chip } from '@heroui/react'
import {
  DollarSign, CheckCircle2, TrendingUp, ListOrdered, Store, Bell,
  ArrowRight, Clock, AlertCircle, Zap,
} from 'lucide-react'
import { useAuth } from '../../context/AuthContext'
import { showToast as toast } from '../../utils/toast'
import { api, ApiError } from '../../utils/api'
import AdminLayout from '../../components/admin/AdminLayout'
import type {
  Stats, Transaction, TransactionsResponse,
  NotificationsResponse, MerchantRecord,
} from '../../components/dashboard/types'

export default function DashboardOverview() {
  const navigate = useNavigate()
  const { logout } = useAuth()

  const [stats, setStats] = useState<Stats>({ totalAmount: '-', successCount: '-', successRate: '-' })
  const [recentOrders, setRecentOrders] = useState<Transaction[]>([])
  const [pendingNotifyCount, setPendingNotifyCount] = useState(0)
  const [merchantCount, setMerchantCount] = useState(0)
  const [refreshing, setRefreshing] = useState(false)

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true)
    try {
      const [statsData, txsData, notifyData, merchantsData] = await Promise.all([
        api.get<Stats>('/admin/stats'),
        api.get<TransactionsResponse>('/admin/transactions?pageSize=5'),
        api.get<NotificationsResponse>('/admin/notifications?status=0&pageSize=1'),
        api.get<MerchantRecord[]>('/admin/merchants'),
      ])
      if (statsData) setStats(statsData)
      if (txsData) {
        const items = Array.isArray(txsData) ? txsData : (txsData.items ?? [])
        setRecentOrders(items)
      }
      if (notifyData) setPendingNotifyCount(notifyData.total ?? 0)
      if (Array.isArray(merchantsData)) setMerchantCount(merchantsData.length)
      if (isRefresh) toast.success('数据已刷新')
    } catch (err: unknown) {
      if (err instanceof ApiError && err.status === 401) {
        toast.error('认证过期，请重新登录')
        logout()
        navigate('/login')
      } else {
        const message = err instanceof Error ? err.message : '网络异常'
        toast.error('加载数据失败: ' + message)
      }
    } finally {
      if (isRefresh) setRefreshing(false)
    }
  }, [logout, navigate])

  useEffect(() => {
    const initialLoad = setTimeout(load, 0)
    return () => clearTimeout(initialLoad)
  }, [load])

  const handleRefresh = () => load(true)

  const statCards = [
    { title: '总交易额', val: stats.totalAmount, icon: DollarSign, color: 'text-muted', bg: 'bg-surface-secondary' },
    { title: '成功单数', val: stats.successCount, icon: CheckCircle2, color: 'text-muted', bg: 'bg-surface-secondary' },
    { title: '支付成功率', val: stats.successRate, icon: TrendingUp, color: 'text-muted', bg: 'bg-surface-secondary' },
    { title: '待处理通知', val: `${pendingNotifyCount} 条`, icon: Bell, color: 'text-amber-500', bg: 'bg-amber-500/10' },
  ]

  const statusLabel: Record<string, string> = {
    SUCCESS: '成功', PENDING: '待支付', FAILED: '失败', REFUNDING: '退款中', REFUNDED: '已退款',
  }
  const statusColor: Record<string, string> = {
    SUCCESS: 'bg-emerald-500/10 text-emerald-500',
    PENDING: 'bg-amber-500/10 text-amber-500',
    FAILED: 'bg-rose-500/10 text-rose-500',
    REFUNDING: 'bg-blue-500/10 text-blue-500',
    REFUNDED: 'bg-zinc-500/10 text-zinc-500',
  }

  return (
    <AdminLayout
      title="控制台总览"
      subtitle="WePay 沙箱运营数据一览"
      refreshing={refreshing}
      onRefresh={handleRefresh}
    >
      {/* Stat cards */}
      <section className="mb-8 grid grid-cols-2 gap-3 sm:gap-5 lg:grid-cols-4">
        {statCards.map((s, i) => (
          <Card key={i} className="p-5">
            <Card.Content className="p-0">
              <div className="flex items-center justify-between mb-3">
                <span className="text-xs font-medium text-muted tracking-wide">{s.title}</span>
                <div className={`w-8 h-8 rounded-lg ${s.bg} ${s.color} flex items-center justify-center`}>
                  <s.icon className="w-4 h-4" />
                </div>
              </div>
              <h3 className="text-2xl font-bold tracking-tight text-foreground">{s.val}</h3>
            </Card.Content>
          </Card>
        ))}
      </section>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Recent orders */}
        <Card className="lg:col-span-2 p-6">
          <Card.Content className="p-0">
            <div className="flex items-center justify-between mb-5">
              <h2 className="text-sm font-semibold text-foreground flex items-center gap-2">
                <ListOrdered className="w-4 h-4 text-muted" />
                最近交易
              </h2>
              <Button
                size="sm"
                variant="ghost"
                onPress={() => navigate('/admin/orders')}
                className="text-xs text-emerald-500 hover:text-emerald-600 flex items-center gap-1 px-2 py-1 h-auto min-h-0 cursor-pointer"
              >
                查看全部 <ArrowRight className="w-3 h-3" />
              </Button>
            </div>
            {recentOrders.length === 0 ? (
              <div className="py-10 text-center text-muted text-sm border border-dashed border-border rounded-xl">
                暂无交易记录
              </div>
            ) : (
              <div className="space-y-2">
                {recentOrders.map(t => (
                  <div key={t.id} className="flex items-center justify-between py-3 px-4 rounded-xl hover:bg-surface/50 transition-colors border border-transparent hover:border-border">
                    <div className="flex items-center gap-3 min-w-0">
                      <div className="min-w-0">
                        <div className="font-mono text-xs text-foreground font-medium truncate">{t.orderNo}</div>
                        {t.productName && <div className="text-[10px] text-muted truncate mt-0.5">{t.productName}</div>}
                      </div>
                      <span className="px-2 py-0.5 rounded text-[10px] bg-surface text-muted font-mono shrink-0">
                        {t.channel === 'NATIVE' ? '官方存管' : t.channel}
                      </span>
                    </div>
                    <div className="flex items-center gap-3 shrink-0">
                      <span className="font-mono font-semibold text-sm text-foreground">￥{Number(t.amount).toFixed(2)}</span>
                      <Chip
                        variant="soft"
                        size="sm"
                        className={`${statusColor[t.status] || 'bg-surface text-muted'} font-semibold border-none`}
                      >
                        <span className="flex items-center gap-1 px-1 text-[10px] tracking-wide">
                          {t.status === 'SUCCESS' && <CheckCircle2 className="w-3 h-3" />}
                          {t.status === 'PENDING' && <Clock className="w-3 h-3" />}
                          {t.status === 'FAILED' && <AlertCircle className="w-3 h-3" />}
                          {statusLabel[t.status] || t.status}
                        </span>
                      </Chip>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </Card.Content>
        </Card>

        {/* Quick actions */}
        <Card className="p-6">
          <Card.Content className="p-0">
            <h2 className="text-sm font-semibold text-foreground mb-5 flex items-center gap-2">
              <Zap className="w-4 h-4 text-emerald-500" />
              快速操作
            </h2>
            <div className="space-y-3">
              <button
                onClick={() => navigate('/admin/orders')}
                className="w-full flex items-center gap-3 p-4 rounded-xl border border-border hover:border-emerald-500/30 hover:bg-emerald-500/5 transition-all cursor-pointer text-left"
              >
                <div className="w-9 h-9 rounded-lg bg-surface-secondary text-muted flex items-center justify-center shrink-0">
                  <ListOrdered className="w-4 h-4" />
                </div>
                <div className="min-w-0">
                  <div className="text-xs font-semibold text-foreground">订单管理</div>
                  <div className="text-[10px] text-muted">查看与管理全部交易订单</div>
                </div>
              </button>

              <button
                onClick={() => navigate('/admin/merchants')}
                className="w-full flex items-center gap-3 p-4 rounded-xl border border-border hover:border-emerald-500/30 hover:bg-emerald-500/5 transition-all cursor-pointer text-left"
              >
                <div className="w-9 h-9 rounded-lg bg-surface-secondary text-muted flex items-center justify-center shrink-0">
                  <Store className="w-4 h-4" />
                </div>
                <div className="min-w-0">
                  <div className="text-xs font-semibold text-foreground">商户管理</div>
                  <div className="text-[10px] text-muted">创建与管理接入商户 ({merchantCount})</div>
                </div>
              </button>

              <button
                onClick={() => navigate('/admin/notifications')}
                className="w-full flex items-center gap-3 p-4 rounded-xl border border-border hover:border-amber-500/30 hover:bg-amber-500/5 transition-all cursor-pointer text-left"
              >
                <div className="w-9 h-9 rounded-lg bg-amber-500/10 text-amber-500 flex items-center justify-center shrink-0">
                  <Bell className="w-4 h-4" />
                </div>
                <div className="min-w-0">
                  <div className="text-xs font-semibold text-foreground">通知队列</div>
                  <div className="text-[10px] text-muted">
                    {pendingNotifyCount > 0
                      ? <span className="text-amber-500 font-semibold">{pendingNotifyCount} 条待处理</span>
                      : '异步通知状态监控'}
                  </div>
                </div>
              </button>

              <button
                onClick={() => navigate('/admin/sandbox')}
                className="w-full flex items-center gap-3 p-4 rounded-xl border border-border hover:border-emerald-500/30 hover:bg-emerald-500/5 transition-all cursor-pointer text-left"
              >
                <div className="w-9 h-9 rounded-lg bg-surface-secondary text-muted flex items-center justify-center shrink-0">
                  <Zap className="w-4 h-4" />
                </div>
                <div className="min-w-0">
                  <div className="text-xs font-semibold text-foreground">开发沙箱</div>
                  <div className="text-[10px] text-muted">密钥管理与测试下单</div>
                </div>
              </button>
            </div>
          </Card.Content>
        </Card>
      </div>
    </AdminLayout>
  )
}
