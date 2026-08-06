import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Card, Chip } from '@heroui/react'
import {
  FileSpreadsheet, Upload, CheckCircle2, AlertTriangle, XCircle,
  Filter, X, Loader2, EyeOff,
} from 'lucide-react'
import { useAuth } from '../../context/AuthContext'
import { showToast as toast } from '../../utils/toast'
import { api, ApiError } from '../../utils/api'
import AdminLayout from '../../components/admin/AdminLayout'
import Pagination from '../../components/admin/Pagination'
import { RECON_STATUS_MAP } from '../../components/admin/status-helpers'

interface ReconRecord {
  id: number
  provider: string
  billDate: string
  orderNo: string
  upstreamTradeNo: string
  upstreamAmount: string
  localAmount: string
  upstreamFee: string
  status: string
  note?: string
  createdAt?: string
}

interface ReconListResponse {
  items: ReconRecord[]
  total: number
  page: number
  pageSize: number
}

interface UploadResult {
  provider: string
  billDate: string
  matched: number
  mismatched: number
  missingLocal: number
  missingUpstream: number
}

const RECON_ICON_MAP: Record<string, typeof CheckCircle2> = {
  matched: CheckCircle2,
  amount_mismatch: XCircle,
  missing_local: AlertTriangle,
  missing_upstream: AlertTriangle,
}

const PAGE_SIZE = 20

export default function ReconciliationPage() {
  const navigate = useNavigate()
  const { logout } = useAuth()

  // Upload form
  const [provider, setProvider] = useState<'alipay' | 'paypal'>('alipay')
  const [billDate, setBillDate] = useState('')
  const [csvText, setCsvText] = useState('')
  const [uploading, setUploading] = useState(false)
  const [uploadResult, setUploadResult] = useState<UploadResult | null>(null)
  const [showUpload, setShowUpload] = useState(false)

  // Records list
  const [items, setItems] = useState<ReconRecord[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [refreshing, setRefreshing] = useState(false)
  const [filterProvider, setFilterProvider] = useState('')
  const [filterDate, setFilterDate] = useState('')
  const [filterStatus, setFilterStatus] = useState('')

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true)
    try {
      const params = new URLSearchParams()
      params.set('page', String(page))
      params.set('pageSize', String(PAGE_SIZE))
      if (filterProvider) params.set('provider', filterProvider)
      if (filterDate) params.set('billDate', filterDate)
      if (filterStatus) params.set('status', filterStatus)

      const data = await api.get<ReconListResponse>(`/admin/reconciliation?${params.toString()}`)
      if (data) {
        setItems(data.items ?? [])
        setTotal(data.total ?? 0)
      }
      if (isRefresh) toast.success('对账数据已刷新')
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
  }, [page, filterProvider, filterDate, filterStatus, logout, navigate])

  useEffect(() => {
    const initialLoad = setTimeout(load, 0)
    return () => clearTimeout(initialLoad)
  }, [load])

  const handleRefresh = () => load(true)

  const handleUpload = async () => {
    if (!billDate) {
      toast.error('请选择账单日期')
      return
    }
    if (!csvText.trim()) {
      toast.error('请粘贴 CSV 内容')
      return
    }
    setUploading(true)
    setUploadResult(null)
    try {
      const result = await api.post<UploadResult>('/admin/reconciliation/upload', {
        provider,
        billDate,
        csv: csvText.trim(),
      })
      if (result) {
        setUploadResult(result)
        toast.success(`对账完成：${result.matched} 匹配, ${result.mismatched} 不符, ${result.missingLocal} 本地缺失, ${result.missingUpstream} 上游缺失`)
        load()
      }
    } catch (err: unknown) {
      toast.error('对账失败: ' + (err instanceof Error ? err.message : '网络异常'))
    } finally {
      setUploading(false)
    }
  }

  const clearFilters = () => {
    setFilterProvider('')
    setFilterDate('')
    setFilterStatus('')
    setPage(1)
  }

  const hasFilters = filterProvider || filterDate || filterStatus

  return (
    <AdminLayout
      title="对账管理"
      subtitle={`上传上游账单 CSV 进行对账匹配 · 共 ${total} 条记录`}
      refreshing={refreshing}
      onRefresh={handleRefresh}
      actions={
        <Button
          onPress={() => setShowUpload(!showUpload)}
          className={`px-4 py-2 rounded-xl text-xs font-semibold flex items-center gap-1.5 cursor-pointer h-9 ${
            showUpload
              ? 'bg-surface text-foreground border border-border'
              : 'bg-emerald-500 hover:bg-emerald-600 text-white'
          }`}
        >
          {showUpload ? <EyeOff className="w-3.5 h-3.5" /> : <Upload className="w-3.5 h-3.5" />}
          {showUpload ? '收起上传' : '上传账单'}
        </Button>
      }
    >
      {/* Upload form */}
      {showUpload && (
        <Card className="p-6 mb-6 border-emerald-500/20">
          <Card.Content className="p-0">
            <div className="flex items-center gap-3 mb-5">
              <div className="w-10 h-10 rounded-xl bg-emerald-500/10 text-emerald-500 flex items-center justify-center">
                <FileSpreadsheet className="w-5 h-5" />
              </div>
              <div>
                <h3 className="font-semibold text-base text-foreground tracking-tight">上传对账账单</h3>
                <p className="text-xs text-muted mt-0.5">粘贴上游 PSP（支付宝 / PayPal）的日账单 CSV，系统自动匹配本地订单</p>
              </div>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-4">
              <div className="space-y-1.5">
                <label className="text-xs font-medium text-muted">支付渠道</label>
                <select
                  value={provider}
                  onChange={e => setProvider(e.target.value as 'alipay' | 'paypal')}
                  className="w-full h-10 rounded-xl border border-border bg-surface text-sm text-foreground px-3 focus:outline-none focus:border-emerald-500 transition-colors cursor-pointer"
                >
                  <option value="alipay">支付宝</option>
                  <option value="paypal">PayPal</option>
                </select>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-medium text-muted">账单日期</label>
                <input
                  type="date"
                  value={billDate}
                  onChange={e => setBillDate(e.target.value)}
                  className="w-full h-10 rounded-xl border border-border bg-surface text-sm text-foreground px-3 focus:outline-none focus:border-emerald-500 transition-colors"
                />
              </div>

              <div className="space-y-1.5 flex items-end">
                <Button
                  onPress={handleUpload}
                  isDisabled={uploading}
                  className="w-full h-10 rounded-xl bg-emerald-500 hover:bg-emerald-600 text-white text-xs font-semibold flex items-center justify-center gap-2 cursor-pointer disabled:opacity-50"
                >
                  {uploading ? (
                    <>
                      <Loader2 className="w-4 h-4 animate-spin" /> 对账中...
                    </>
                  ) : (
                    <>
                      <Upload className="w-4 h-4" /> 开始对账
                    </>
                  )}
                </Button>
              </div>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium text-muted">CSV 内容</label>
              <textarea
                value={csvText}
                onChange={e => setCsvText(e.target.value)}
                placeholder={`粘贴 CSV 内容，例如：\n商户订单号,支付宝交易号,商户实收,商户手续费\nPAY20250622001,2025062222001,88.88,0.44\nPAY20250622002,2025062222002,100.00,0.50`}
                rows={6}
                className="w-full rounded-xl border border-border bg-surface text-xs font-mono text-foreground p-3 focus:outline-none focus:border-emerald-500 transition-colors resize-y"
              />
            </div>

            {/* Upload result */}
            {uploadResult && (
              <div className="mt-5 p-5 rounded-xl bg-surface border border-border">
                <div className="text-xs font-semibold text-foreground mb-3">对账结果</div>
                <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
                  {[
                    { label: '匹配成功', val: uploadResult.matched, color: 'text-emerald-500', bg: 'bg-emerald-500/10' },
                    { label: '金额不符', val: uploadResult.mismatched, color: 'text-rose-500', bg: 'bg-rose-500/10' },
                    { label: '本地缺失', val: uploadResult.missingLocal, color: 'text-amber-500', bg: 'bg-amber-500/10' },
                    { label: '上游缺失', val: uploadResult.missingUpstream, color: 'text-blue-500', bg: 'bg-blue-500/10' },
                  ].map(s => (
                    <div key={s.label} className={`p-3 rounded-lg ${s.bg} text-center`}>
                      <div className={`text-2xl font-bold ${s.color}`}>{s.val}</div>
                      <div className="text-[10px] text-muted mt-1">{s.label}</div>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </Card.Content>
        </Card>
      )}

      {/* Filters */}
      <Card className="p-4 mb-6">
        <Card.Content className="p-0">
          <div className="flex items-center gap-3 flex-wrap">
            <Filter className="w-4 h-4 text-muted" />
            <span className="text-xs font-semibold text-foreground">筛选</span>

            <select
              value={filterProvider}
              onChange={e => { setFilterProvider(e.target.value); setPage(1) }}
              className="h-8 rounded-lg border border-border bg-surface text-xs text-foreground px-2.5 focus:outline-none focus:border-emerald-500 transition-colors cursor-pointer"
            >
              <option value="">全部渠道</option>
              <option value="alipay">支付宝</option>
              <option value="paypal">PayPal</option>
            </select>

            <input
              type="date"
              value={filterDate}
              onChange={e => { setFilterDate(e.target.value); setPage(1) }}
              className="h-8 rounded-lg border border-border bg-surface text-xs text-foreground px-2.5 focus:outline-none focus:border-emerald-500 transition-colors"
            />

            <select
              value={filterStatus}
              onChange={e => { setFilterStatus(e.target.value); setPage(1) }}
              className="h-8 rounded-lg border border-border bg-surface text-xs text-foreground px-2.5 focus:outline-none focus:border-emerald-500 transition-colors cursor-pointer"
            >
              <option value="">全部状态</option>
              <option value="matched">已匹配</option>
              <option value="amount_mismatch">金额不符</option>
              <option value="missing_local">本地缺失</option>
              <option value="missing_upstream">上游缺失</option>
            </select>

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

      {/* Records table */}
      <Card className="p-6">
        <Card.Content className="p-0">
          <div className="overflow-x-auto">
            {items.length === 0 ? (
              <div className="py-12 text-center">
                <FileSpreadsheet className="w-12 h-12 text-muted mx-auto mb-3" />
                <div className="text-sm text-muted">{hasFilters ? '没有匹配的对账记录' : '暂无对账记录，点击右上角「上传账单」开始'}</div>
              </div>
            ) : (
              <table className="w-full text-left">
                <thead>
                  <tr className="border-b border-border">
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">ID</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">渠道</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">账单日期</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">订单号</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">上游交易号</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">上游金额</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">本地金额</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">手续费</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">状态</th>
                    <th className="pb-3 text-xs font-semibold text-muted tracking-wide">备注</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map(r => {
                    const st = RECON_STATUS_MAP[r.status] || { label: r.status, color: 'bg-surface text-muted' }
                    const Icon = RECON_ICON_MAP[r.status] || AlertTriangle
                    return (
                      <tr key={r.id} className="border-b border-border/60 last:border-none hover:bg-surface/50 transition-colors">
                        <td className="py-3.5 font-mono text-xs text-muted">#{r.id}</td>
                        <td className="py-3.5">
                          <span className="px-2 py-0.5 rounded text-[10px] bg-surface text-muted font-mono font-medium">
                            {r.provider}
                          </span>
                        </td>
                        <td className="py-3.5 text-xs text-foreground font-mono">{r.billDate}</td>
                        <td className="py-3.5 font-mono text-xs text-foreground font-medium">{r.orderNo || '—'}</td>
                        <td className="py-3.5 font-mono text-[10px] text-muted">{r.upstreamTradeNo || '—'}</td>
                        <td className="py-3.5 font-mono text-xs text-foreground">￥{r.upstreamAmount}</td>
                        <td className="py-3.5 font-mono text-xs text-foreground">￥{r.localAmount}</td>
                        <td className="py-3.5 font-mono text-xs text-muted">￥{r.upstreamFee}</td>
                        <td className="py-3.5">
                          <Chip variant="soft" size="sm" className={`${st.color} font-semibold border-none`}>
                            <span className="flex items-center gap-1 px-1 text-[10px] tracking-wide">
                              <Icon className="w-3 h-3" /> {st.label}
                            </span>
                          </Chip>
                        </td>
                        <td className="py-3.5 text-[10px] text-muted max-w-[160px] truncate" title={r.note}>
                          {r.note || '—'}
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
