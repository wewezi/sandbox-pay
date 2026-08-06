import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Chip, InputGroup, TextField, Card } from '@heroui/react'
import {
  Search, CheckCircle2, Clock, HandCoins, Undo2, Trash2,
  AlertCircle, Filter, X,
} from 'lucide-react'
import { useAuth } from '../../context/AuthContext'
import { showToast as toast } from '../../utils/toast'
import { api, ApiError } from '../../utils/api'
import AdminLayout from '../../components/admin/AdminLayout'
import Pagination from '../../components/admin/Pagination'
import { ORDER_STATUS_LABEL, ORDER_STATUS_COLOR } from '../../components/admin/status-helpers'
import RefundDialog from '../../components/dashboard/RefundDialog'
import DeleteConfirmDialog from '../../components/dashboard/DeleteConfirmDialog'
import type { Transaction, TransactionsResponse, Stats } from '../../components/dashboard/types'

const STATUS_OPTIONS = [
  { value: '', label: '全部状态' },
  { value: '0', label: '待支付' },
  { value: '1', label: '已支付' },
  { value: '2', label: '失败' },
  { value: '3', label: '退款中' },
  { value: '4', label: '已退款' },
  { value: '-1', label: '已过期' },
]

const PAY_METHOD_OPTIONS = [
  { value: '', label: '全部渠道' },
  { value: 'alipay', label: '支付宝' },
  { value: 'paypal', label: 'PayPal' },
  { value: 'native', label: '官方存管' },
]

const PAGE_SIZE = 20

export default function OrdersPage() {
  const navigate = useNavigate()
  const { logout } = useAuth()

  const [orders, setOrders] = useState<Transaction[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [stats, setStats] = useState<Stats>({ totalAmount: '-', successCount: '-', successRate: '-' })
  const [refreshing, setRefreshing] = useState(false)

  // Filters
  const [keyword, setKeyword] = useState('')
  const [status, setStatus] = useState('')
  const [payMethod, setPayMethod] = useState('')
  const [dateFrom, setDateFrom] = useState('')
  const [dateTo, setDateTo] = useState('')

  // Refund dialog
  const [refundTarget, setRefundTarget] = useState<Transaction | null>(null)
  const [refundAmount, setRefundAmount] = useState('')
  const [refundReason, setRefundReason] = useState('')
  const [refundSubmitting, setRefundSubmitting] = useState(false)

  // Delete dialog
  const [deleteTarget, setDeleteTarget] = useState<Transaction | null>(null)
  const [deleteSubmitting, setDeleteSubmitting] = useState(false)

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true)
    try {
      const params = new URLSearchParams()
      params.set('page', String(page))
      params.set('pageSize', String(PAGE_SIZE))
      if (keyword) params.set('keyword', keyword)
      if (status) params.set('status', status)
      if (payMethod) params.set('payMethod', payMethod)
      if (dateFrom) params.set('dateFrom', dateFrom)
      if (dateTo) params.set('dateTo', dateTo)

      const [txsData, statsData] = await Promise.all([
        api.get<TransactionsResponse>(`/admin/transactions?${params.toString()}`),
        api.get<Stats>('/admin/stats'),
      ])

      if (txsData) {
        const items = Array.isArray(txsData) ? txsData : (txsData.items ?? [])
        setOrders(items)
        setTotal(Array.isArray(txsData) ? items.length : (txsData.total ?? 0))
      }
      if (statsData) setStats(statsData)
      if (isRefresh) toast.success('数据已刷新')
    } catch (err: unknown) {
      if (err instanceof ApiError && err.status === 401) {
        toast.error('认证过期')
        logout()
        navigate('/login')
      } else {
        toast.error('加载失败: ' + (err instanceof Error ? err.message : '网络异常'))
      }
    } finally {
      if (isRefresh) setRefreshing(false)
    }
  }, [page, keyword, status, payMethod, dateFrom, dateTo, logout, navigate])

  useEffect(() => {
    const initialLoad = setTimeout(load, 0)
    return () => clearTimeout(initialLoad)
  }, [load])

  const handleRefresh = () => load(true)

  const handleSearch = () => {
    setPage(1)
    load()
  }

  const clearFilters = () => {
    setKeyword('')
    setStatus('')
    setPayMethod('')
    setDateFrom('')
    setDateTo('')
    setPage(1)
  }

  const hasFilters = keyword || status || payMethod || dateFrom || dateTo

  const handleConfirmNative = async (t: Transaction) => {
    try {
      await api.post('/native-pay/confirm', { orderNo: t.orderNo })
      toast.success(`订单 ${t.orderNo} 已确认收款`)
      load(true)
    } catch (err: unknown) {
      toast.error('确认失败: ' + (err instanceof Error ? err.message : '网络异常'))
    }
  }

  const openRefundDialog = (t: Transaction) => {
    const refundable = Math.max(0, Number(t.amount || 0) - Number(t.refundedAmount || 0))
    setRefundTarget(t)
    setRefundAmount(refundable.toFixed(2))
    setRefundReason('')
  }

  const closeRefundDialog = () => {
    if (refundSubmitting) return
    setRefundTarget(null)
    setRefundAmount('')
    setRefundReason('')
  }

  const submitRefund = async () => {
    if (!refundTarget) return
    const amountNum = Number(refundAmount)
    if (!Number.isFinite(amountNum) || amountNum <= 0) {
      toast.error('请输入正确的退款金额')
      return
    }
    const refundable = Number(refundTarget.amount || 0) - Number(refundTarget.refundedAmount || 0)
    if (amountNum - refundable > 0.01) {
      toast.error(`退款金额超出可退余额 ￥${refundable.toFixed(2)}`)
      return
    }
    setRefundSubmitting(true)
    try {
      await api.post('/admin/refund', {
        orderNo: refundTarget.orderNo,
        amount: amountNum,
        reason: refundReason || undefined,
      })
      toast.success(`订单 ${refundTarget.orderNo} 已发起退款`)
      setRefundTarget(null)
      setRefundAmount('')
      setRefundReason('')
      load(true)
    } catch (err: unknown) {
      toast.error('退款失败: ' + (err instanceof Error ? err.message : '网络异常'))
    } finally {
      setRefundSubmitting(false)
    }
  }

  const openDeleteDialog = (t: Transaction) => setDeleteTarget(t)
  const closeDeleteDialog = () => { if (!deleteSubmitting) setDeleteTarget(null) }

  const submitDelete = async () => {
    if (!deleteTarget) return
    setDeleteSubmitting(true)
    try {
      await api.delete(`/admin/transactions/${deleteTarget.orderNo}`)
      toast.success(`订单 ${deleteTarget.orderNo} 已删除`)
      setDeleteTarget(null)
      load(true)
    } catch (err: unknown) {
      toast.error('删除失败: ' + (err instanceof Error ? err.message : '网络异常'))
    } finally {
      setDeleteSubmitting(false)
    }
  }

  return (
    <AdminLayout
      title="订单管理"
      subtitle={`共 ${total} 笔交易记录`}
      refreshing={refreshing}
      onRefresh={handleRefresh}
    >
      {/* Stats strip */}
      <section className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-6">
        {[
          { title: '总交易额', val: stats.totalAmount },
          { title: '成功单数', val: stats.successCount },
          { title: '支付成功率', val: stats.successRate },
        ].map((s, i) => (
          <Card key={i} className="p-4">
            <Card.Content className="p-0">
              <div className="text-xs text-muted mb-1">{s.title}</div>
              <div className="text-lg font-bold text-foreground">{s.val}</div>
            </Card.Content>
          </Card>
        ))}
      </section>

      {/* Filters */}
      <Card className="p-5 mb-6">
        <Card.Content className="p-0">
          <div className="flex items-center gap-2 mb-4">
            <Filter className="w-4 h-4 text-muted" />
            <span className="text-xs font-semibold text-foreground">筛选条件</span>
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
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-3">
            <div className="lg:col-span-2">
              <TextField aria-label="搜索单号" fullWidth>
                <InputGroup className="w-full rounded-xl border border-border focus-within:border-emerald-500 transition-colors h-9">
                  <InputGroup.Prefix>
                    <Search className="w-3.5 h-3.5 text-muted" />
                  </InputGroup.Prefix>
                  <InputGroup.Input
                    type="text"
                    placeholder="搜索单号 / 外部单号..."
                    value={keyword}
                    onChange={e => setKeyword(e.target.value)}
                    onKeyDown={e => e.key === 'Enter' && handleSearch()}
                    className="py-1.5 text-xs text-foreground bg-transparent w-full focus:outline-none"
                  />
                </InputGroup>
              </TextField>
            </div>

            <select
              value={status}
              onChange={e => { setStatus(e.target.value); setPage(1) }}
              className="h-9 rounded-xl border border-border bg-surface text-xs text-foreground px-3 focus:outline-none focus:border-emerald-500 transition-colors cursor-pointer"
            >
              {STATUS_OPTIONS.map(o => (
                <option key={o.value} value={o.value}>{o.label}</option>
              ))}
            </select>

            <select
              value={payMethod}
              onChange={e => { setPayMethod(e.target.value); setPage(1) }}
              className="h-9 rounded-xl border border-border bg-surface text-xs text-foreground px-3 focus:outline-none focus:border-emerald-500 transition-colors cursor-pointer"
            >
              {PAY_METHOD_OPTIONS.map(o => (
                <option key={o.value} value={o.value}>{o.label}</option>
              ))}
            </select>

            <div className="flex gap-2">
              <input
                type="date"
                value={dateFrom}
                onChange={e => { setDateFrom(e.target.value); setPage(1) }}
                className="flex-1 h-9 rounded-xl border border-border bg-surface text-xs text-foreground px-2 focus:outline-none focus:border-emerald-500 transition-colors"
                placeholder="开始日期"
              />
              <input
                type="date"
                value={dateTo}
                onChange={e => { setDateTo(e.target.value); setPage(1) }}
                className="flex-1 h-9 rounded-xl border border-border bg-surface text-xs text-foreground px-2 focus:outline-none focus:border-emerald-500 transition-colors"
                placeholder="结束日期"
              />
            </div>
          </div>
        </Card.Content>
      </Card>

      {/* Orders table */}
      <Card className="p-6">
        <Card.Content className="p-0">
          <div className="overflow-x-auto">
            {orders.length === 0 ? (
              <div className="py-12 text-center text-muted text-sm border border-dashed border-border rounded-xl">
                {hasFilters ? '没有匹配当前筛选条件的订单' : '暂无交易记录'}
              </div>
            ) : (
              <table className="w-full text-left">
                <thead>
                  <tr className="border-b border-border">
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">订单号</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">商品</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">渠道</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">金额</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">已退</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">状态</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">时间</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide text-right">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {orders.map(t => (
                    <tr key={t.id} className="group border-b border-border/60 last:border-none hover:bg-surface/50 transition-colors">
                      <td className="py-3.5 font-mono text-xs text-foreground font-medium">{t.orderNo}</td>
                      <td className="py-3.5 text-xs text-foreground max-w-[160px] truncate" title={t.productName}>
                        {t.productName || '—'}
                      </td>
                      <td className="py-3.5">
                        <span className="px-2 py-0.5 rounded text-[10px] bg-surface text-muted font-mono font-medium">
                          {t.channel === 'NATIVE' ? '官方存管' : t.channel}
                        </span>
                      </td>
                      <td className="py-3.5 font-mono font-semibold text-sm text-foreground">
                        ￥{Number(t.amount).toFixed(2)}
                      </td>
                      <td className="py-3.5 font-mono text-xs text-muted">
                        {Number(t.refundedAmount || 0) > 0
                          ? `￥${Number(t.refundedAmount).toFixed(2)}`
                          : '—'}
                      </td>
                      <td className="py-3.5">
                        <Chip variant="soft" size="sm" className={`${ORDER_STATUS_COLOR[t.status] || 'bg-surface text-muted'} font-semibold border-none`}>
                          <span className="flex items-center gap-1 px-1 text-[10px] tracking-wide">
                            {t.status === 'SUCCESS' && <CheckCircle2 className="w-3 h-3" />}
                            {t.status === 'PENDING' && <Clock className="w-3 h-3" />}
                            {t.status === 'FAILED' && <AlertCircle className="w-3 h-3" />}
                            {ORDER_STATUS_LABEL[t.status] || t.status}
                          </span>
                        </Chip>
                      </td>
                      <td className="py-3.5 text-xs text-muted font-mono">{t.time}</td>
                      <td className="py-3.5">
                        <div className="flex items-center gap-1.5 justify-end">
                          {t.channel === 'NATIVE' && t.status === 'PENDING' && (
                            <Button
                              size="sm"
                              variant="ghost"
                              onPress={() => handleConfirmNative(t)}
                              className="px-2.5 py-1 text-[10px] font-medium text-emerald-500 hover:bg-emerald-500/10 rounded-lg transition-colors h-auto min-h-0 min-w-0 flex items-center gap-1 cursor-pointer"
                            >
                              <HandCoins className="w-3 h-3" /> 确认收款
                            </Button>
                          )}
                          {t.status === 'SUCCESS' && Number(t.amount || 0) - Number(t.refundedAmount || 0) > 0.01 && (
                            <Button
                              size="sm"
                              variant="ghost"
                              onPress={() => openRefundDialog(t)}
                              className="px-2.5 py-1 text-[10px] font-medium text-rose-500 hover:bg-rose-500/10 rounded-lg transition-colors h-auto min-h-0 min-w-0 flex items-center gap-1 cursor-pointer"
                            >
                              <Undo2 className="w-3 h-3" /> 退款
                            </Button>
                          )}
                          <Button
                            size="sm"
                            variant="ghost"
                            onPress={() => openDeleteDialog(t)}
                            aria-label="删除订单"
                            className="px-2 py-1 text-muted hover:text-rose-500 hover:bg-rose-500/10 rounded-lg transition-colors h-auto min-h-0 min-w-0 flex items-center justify-center cursor-pointer"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>

          <Pagination page={page} total={total} pageSize={PAGE_SIZE} onChange={setPage} />
        </Card.Content>
      </Card>

      <RefundDialog
        target={refundTarget}
        amount={refundAmount}
        setAmount={setRefundAmount}
        reason={refundReason}
        setReason={setRefundReason}
        submitting={refundSubmitting}
        onClose={closeRefundDialog}
        onSubmit={submitRefund}
      />

      <DeleteConfirmDialog
        target={deleteTarget}
        submitting={deleteSubmitting}
        onClose={closeDeleteDialog}
        onSubmit={submitDelete}
      />
    </AdminLayout>
  )
}
