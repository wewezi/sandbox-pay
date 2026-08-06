import { useCallback, useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import {
  BadgeCheck,
  Check,
  ChevronRight,
  Clock3,
  Copy,
  ExternalLink,
  Headphones,
  Info,
  KeyRound,
  LockKeyhole,
  QrCode,
  RefreshCw,
  ScanLine,
  ShieldCheck,
  Smartphone,
  Sparkles,
  UserRound,
  WalletCards,
} from 'lucide-react'
import { FaAlipay, FaPaypal } from 'react-icons/fa6'
import { Button } from '@heroui/react'
import { api, ApiError } from '../utils/api'
import { useOrderStatus } from '../utils/socket'
import { showToast as toast } from '../utils/toast'
import type { CashierInfo } from '../components/dashboard/types'
import OrderInfoCard from '../components/cashier/OrderInfoCard'
import { PaidView, ExpiredFailedView, ErrorView, LoadingView } from '../components/cashier/StatusViews'
import ScannerModal from '../components/cashier/ScannerModal'
import BrandMark from '../components/BrandMark'

type PaymentMethod = 'alipay' | 'paypal' | 'native'

const paymentMethods: Array<{
  key: PaymentMethod
  title: string
  description: string
  badge?: string
}> = [
  { key: 'alipay', title: '支付宝', description: '跳转或扫码支付' },
  { key: 'paypal', title: 'PayPal', description: '国际账户与银行卡' },
  { key: 'native', title: 'Sandbox 钱包', description: '沙箱账户快捷支付' },
]

function MethodIcon({ method }: { method: PaymentMethod }) {
  if (method === 'alipay') {
    return (
      <span className="flex h-11 w-11 items-center justify-center rounded-xl bg-[#1677ff]/10 text-[#1677ff]">
        <FaAlipay className="h-6 w-6" />
      </span>
    )
  }
  if (method === 'paypal') {
    return (
      <span className="flex h-11 w-11 items-center justify-center rounded-xl bg-[#003087]/10 text-[#0070ba] dark:text-[#4ca7ef]">
        <FaPaypal className="h-6 w-6" />
      </span>
    )
  }
  return (
    <span className="flex h-11 w-11 items-center justify-center rounded-xl bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
      <WalletCards className="h-5 w-5" />
    </span>
  )
}

export default function Cashier() {
  const [searchParams] = useSearchParams()
  const orderNo = searchParams.get('orderNo') || ''
  const [info, setInfo] = useState<CashierInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [copied, setCopied] = useState<string | null>(null)
  const [countdown, setCountdown] = useState('')
  const [activeMethod, setActiveMethod] = useState<PaymentMethod>('alipay')
  const [nativeMode, setNativeMode] = useState<'qrcode' | 'login'>('qrcode')
  const [walletUser, setWalletUser] = useState('')
  const [walletPass, setWalletPass] = useState('')
  const [payingNative, setPayingNative] = useState(false)
  const [showScannerModal, setShowScannerModal] = useState(false)
  const [switching, setSwitching] = useState(false)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const initialMethodSetRef = useRef(false)

  const fetchInfo = useCallback(async () => {
    try {
      const data = await api.get<CashierInfo>(`/native-pay/cashier?orderNo=${encodeURIComponent(orderNo)}`)
      setInfo(data)
      setError('')

      if (!initialMethodSetRef.current) {
        const method = data.payMethod?.toLowerCase()
        if (method === 'paypal' || method === 'alipay') setActiveMethod(method)
        if (method === 'native' || method === 'bank') setActiveMethod('native')
        initialMethodSetRef.current = true
      }

      if (data.status === 'paid' || data.status === 'expired' || data.status === 'failed') {
        if (pollRef.current) clearInterval(pollRef.current)
      }
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setError(err.message || '订单查询失败')
      } else {
        setError('网络异常，请稍后重试')
      }
    } finally {
      setLoading(false)
    }
  }, [orderNo])

  useEffect(() => {
    if (!orderNo) return

    const initialFetch = setTimeout(fetchInfo, 0)
    pollRef.current = setInterval(fetchInfo, 5000)
    return () => {
      clearTimeout(initialFetch)
      if (pollRef.current) clearInterval(pollRef.current)
    }
  }, [orderNo, fetchInfo])

  const handleWsStatus = useCallback((status: string) => {
    if (status === 'paid' || status === 'refunded' || status === 'failed') {
      if (pollRef.current) clearInterval(pollRef.current)
      fetchInfo()
    }
  }, [fetchInfo])
  useOrderStatus(orderNo, handleWsStatus)

  useEffect(() => {
    if (!info?.expireAt || info.status !== 'pending') return

    const updateCountdown = () => {
      const diff = new Date(info.expireAt!).getTime() - Date.now()
      if (diff <= 0) {
        setCountdown('已过期')
        fetchInfo()
        return false
      }
      const mins = Math.floor(diff / 60000)
      const secs = Math.floor((diff % 60000) / 1000)
      setCountdown(`${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`)
      return true
    }

    updateCountdown()
    const timer = setInterval(() => {
      if (!updateCountdown()) clearInterval(timer)
    }, 1000)
    return () => clearInterval(timer)
  }, [info?.expireAt, info?.status, fetchInfo])

  const handleCopy = async (value: string, key: string) => {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(key)
      toast.success('已复制到剪贴板')
      setTimeout(() => setCopied(null), 2000)
    } catch {
      toast.error('复制失败，请手动复制')
    }
  }

  const handleInvokeRealGateway = async (channel: 'alipay' | 'paypal') => {
    if (!info || switching) return
    setSwitching(true)
    try {
      const payload = await api.post<{ type: string; data: string }>(
        '/native-pay/switch-channel',
        { orderNo: info.orderNo, channel },
      )
      const gatewayPayload = payload?.data
      if (!gatewayPayload) {
        toast.error('支付网关未返回有效跳转信息')
        return
      }

      if (channel === 'paypal') {
        window.location.href = gatewayPayload
        return
      }

      if (gatewayPayload.startsWith('http')) {
        window.location.href = gatewayPayload
        return
      }

      const parser = new DOMParser()
      const doc = parser.parseFromString(gatewayPayload, 'text/html')
      const form = doc.querySelector('form')
      if (!form) {
        toast.error('支付宝网关返回的表单无效')
        return
      }
      doc.querySelectorAll('script').forEach(script => script.remove())
      document.body.appendChild(form)
      form.submit()
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : '沙箱通道暂时不可用'
      toast.error(`无法连接支付网关：${message}`)
    } finally {
      setSwitching(false)
    }
  }

  const handleNativePay = async () => {
    if (!info || payingNative) return
    setPayingNative(true)
    try {
      await api.post('/native-pay/sandbox-confirm', {
        orderNo: info.orderNo,
        walletUser: walletUser || 'demo_buyer@sandbox-pay.local',
        walletPass: walletPass || '123456',
      })
      toast.success('支付成功，正在确认订单状态')
      fetchInfo()
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : '账户信息校验失败'
      toast.error(`支付失败：${message}`)
    } finally {
      setPayingNative(false)
    }
  }

  if (!orderNo) return <ErrorView error="支付链接缺少订单号，请向商户重新获取" />
  if (loading) return <LoadingView />
  if (error) return <ErrorView error={error} onRetry={() => window.location.reload()} />
  if (!info) return null
  if (info.status === 'paid') return <PaidView info={info} />
  if (info.status === 'expired' || info.status === 'failed') return <ExpiredFailedView info={info} />

  const paymentInfo = info.paymentInfo
  const qrUrl = paymentInfo?.qrCodeUrl?.trim()
    ? paymentInfo.qrCodeUrl
    : `https://api.qrserver.com/v1/create-qr-code/?size=300x300&margin=8&data=${encodeURIComponent(`SANDBOX_PAY_ORDER_${info.orderNo}`)}`
  const walletQrUrl = `https://api.qrserver.com/v1/create-qr-code/?size=300x300&margin=8&data=${encodeURIComponent(`${window.location.origin}/mobile-pay?orderNo=${info.orderNo}`)}`
  const remark = paymentInfo?.remark || `WP${info.orderNo}`

  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-b border-border bg-surface/95 backdrop-blur-xl">
        <div className="mx-auto flex h-[72px] max-w-[1160px] items-center justify-between px-5 sm:px-8">
          <div className="flex items-center gap-3">
            <BrandMark size="sm" />
            <div>
              <div className="flex items-center gap-2 text-[15px] font-bold tracking-tight">
                WePay
                <BadgeCheck className="h-4 w-4 fill-emerald-500 text-white dark:text-[#0d1014]" />
              </div>
              <p className="text-[11px] font-medium text-slate-500 dark:text-slate-400">安全收银台</p>
            </div>
          </div>
          <div className="flex items-center gap-2 rounded-full border border-emerald-200 bg-emerald-50 px-3 py-1.5 text-[11px] font-semibold text-emerald-700 dark:border-emerald-500/20 dark:bg-emerald-500/10 dark:text-emerald-300">
            <ShieldCheck className="h-3.5 w-3.5" />
            <span className="hidden sm:inline">连接安全</span>
          </div>
        </div>
      </header>

      <main className="mx-auto w-full min-w-0 max-w-[1160px] px-4 py-6 sm:px-8 sm:py-10">
        <div className="mb-6 flex items-start gap-3 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-amber-950 dark:border-amber-400/20 dark:bg-amber-400/10 dark:text-amber-100">
          <Info className="mt-0.5 h-4 w-4 shrink-0 text-amber-600 dark:text-amber-300" />
          <div className="text-xs leading-5">
            <span className="font-bold">当前为沙箱测试环境。</span>
            <span className="ml-1 text-amber-800 dark:text-amber-200/80">所有支付均为模拟交易，不会产生真实扣款。</span>
          </div>
        </div>

        <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] items-start gap-6 lg:grid-cols-[minmax(0,1fr)_360px] lg:gap-8">
          <section className="min-w-0 overflow-hidden rounded-lg border border-border bg-surface">
            <div className="border-b border-slate-100 px-5 py-5 dark:border-white/[0.07] sm:px-7 sm:py-6">
              <div className="flex items-start justify-between gap-4">
                <div className="min-w-0">
                  <div className="mb-1 flex items-center gap-2 text-xs font-semibold text-emerald-600 dark:text-emerald-400">
                    <span className="flex h-5 w-5 items-center justify-center rounded-full bg-emerald-500 text-[10px] text-white">1</span>
                    选择支付方式
                  </div>
                  <h1 className="text-xl font-bold tracking-tight sm:text-2xl">完成付款</h1>
                  <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">请选择一种可用方式支付当前订单</p>
                </div>
                <div className="shrink-0 text-right lg:hidden">
                  <p className="text-[11px] text-slate-500">应付金额</p>
                  <p className="mt-0.5 text-xl font-bold tabular-nums">¥{info.amount.toFixed(2)}</p>
                </div>
              </div>
            </div>

            <div className="p-5 sm:p-7">
              <div className="grid gap-3 sm:grid-cols-3" role="radiogroup" aria-label="支付方式">
                {paymentMethods.map(method => {
                  const selected = activeMethod === method.key
                  return (
                    <button
                      key={method.key}
                      type="button"
                      role="radio"
                      aria-checked={selected}
                      onClick={() => setActiveMethod(method.key)}
                      className={`group relative flex items-center gap-3 rounded-xl border p-3.5 text-left outline-none transition duration-200 focus-visible:ring-2 focus-visible:ring-emerald-500/50 sm:block ${
                        selected
                          ? 'border-emerald-500 bg-emerald-50/70 shadow-[0_0_0_1px_rgba(16,185,129,0.08)] dark:bg-emerald-500/[0.08]'
                          : 'border-slate-200 bg-white hover:border-slate-400 dark:border-white/10 dark:bg-white/[0.02] dark:hover:border-white/20'
                      }`}
                    >
                      <span className={`absolute right-3 top-3 flex h-4 w-4 items-center justify-center rounded-full border ${selected ? 'border-emerald-500 bg-emerald-500' : 'border-slate-300 dark:border-slate-600'}`}>
                        {selected && <Check className="h-2.5 w-2.5 stroke-[3] text-white" />}
                      </span>
                      <MethodIcon method={method.key} />
                      <span className="min-w-0 sm:mt-3 sm:block">
                        <span className="flex items-center gap-1.5 pr-5 text-sm font-bold">
                          {method.title}
                          {method.badge && (
                            <span className={`rounded px-1.5 py-0.5 text-[9px] font-bold ${method.key === 'native' ? 'bg-emerald-500 text-white' : 'bg-slate-100 text-slate-500 dark:bg-white/10 dark:text-slate-300'}`}>
                              {method.badge}
                            </span>
                          )}
                        </span>
                        <span className="mt-0.5 block text-[11px] text-slate-500 dark:text-slate-400">{method.description}</span>
                      </span>
                    </button>
                  )
                })}
              </div>

              <div className="my-6 h-px bg-slate-100 dark:bg-white/[0.07]" />

              {activeMethod === 'alipay' && (
                <div className="grid items-center gap-6 sm:grid-cols-[1fr_180px] sm:gap-8">
                  <div>
                    <div className="mb-5">
                      <h2 className="text-base font-bold">使用支付宝付款</h2>
                      <p className="mt-1.5 text-xs leading-5 text-slate-500 dark:text-slate-400">点击按钮前往支付宝沙箱，或使用支付宝沙箱客户端扫描右侧二维码。</p>
                    </div>
                    <Button
                      isDisabled={switching}
                      onPress={() => handleInvokeRealGateway('alipay')}
                      className="h-12 min-w-0 w-full rounded-md bg-[#1677ff] px-5 text-sm font-bold text-white hover:bg-[#0f68e8] disabled:opacity-60"
                    >
                      {switching ? <RefreshCw className="h-4 w-4 animate-spin" /> : <FaAlipay className="h-5 w-5" />}
                      {switching ? '正在连接支付宝…' : `支付宝支付 ¥${info.amount.toFixed(2)}`}
                      {!switching && <ChevronRight className="ml-auto h-4 w-4" />}
                    </Button>
                    <div className="mt-4 flex items-start gap-2 text-[11px] leading-4 text-slate-500 dark:text-slate-400">
                      <ExternalLink className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                      付款将在支付宝沙箱页面完成，结束后请返回本页查看结果。
                    </div>
                  </div>
                  <div className="flex flex-col items-center">
                    <div className="rounded-xl border border-slate-200 bg-white p-2.5 shadow-sm">
                      <img src={qrUrl} alt="支付宝付款二维码" className="h-36 w-36 object-contain" />
                    </div>
                    <span className="mt-2 flex items-center gap-1 text-[11px] text-slate-500 dark:text-slate-400">
                      <ScanLine className="h-3.5 w-3.5" /> 扫码完成付款
                    </span>
                    <button
                      type="button"
                      onClick={() => handleCopy(remark, 'remark')}
                      className="mt-2 flex items-center gap-1.5 rounded-md bg-slate-100 px-2 py-1 font-mono text-[10px] text-slate-600 hover:text-slate-950 dark:bg-white/[0.06] dark:text-slate-300 dark:hover:text-white"
                    >
                      附言 {remark}
                      {copied === 'remark' ? <Check className="h-3 w-3 text-emerald-500" /> : <Copy className="h-3 w-3" />}
                    </button>
                  </div>
                </div>
              )}

              {activeMethod === 'paypal' && (
                <div className="mx-auto max-w-lg py-2 text-center">
                  <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-2xl bg-[#003087]/10 text-[#0070ba] dark:text-[#4ca7ef]">
                    <FaPaypal className="h-8 w-8" />
                  </div>
                  <h2 className="mt-4 text-lg font-bold">使用 PayPal 安全结账</h2>
                  <p className="mx-auto mt-1.5 max-w-sm text-xs leading-5 text-slate-500 dark:text-slate-400">支持 PayPal 沙箱账户及测试银行卡。系统会自动处理测试币种换算。</p>
                  <Button
                    isDisabled={switching}
                    onPress={() => handleInvokeRealGateway('paypal')}
                    className="mt-6 h-12 w-full rounded-md bg-[#0070ba] px-5 text-sm font-bold text-white hover:bg-[#005ea6] disabled:opacity-60"
                  >
                    {switching ? <RefreshCw className="h-4 w-4 animate-spin" /> : <LockKeyhole className="h-4 w-4" />}
                    {switching ? '正在连接 PayPal…' : `PayPal 支付 ¥${info.amount.toFixed(2)}`}
                    {!switching && <ExternalLink className="ml-auto h-4 w-4" />}
                  </Button>
                  <p className="mt-3 text-[11px] text-slate-400">你将离开此页面并前往 PayPal 沙箱完成付款</p>
                </div>
              )}

              {activeMethod === 'native' && (
                <div>
                  <div className="mb-6 flex rounded-xl bg-slate-100 p-1 dark:bg-black/20" role="tablist" aria-label="Sandbox 钱包支付方式">
                    <button
                      type="button"
                      role="tab"
                      aria-selected={nativeMode === 'qrcode'}
                      onClick={() => setNativeMode('qrcode')}
                      className={`flex h-10 flex-1 items-center justify-center gap-2 rounded-lg text-xs font-bold transition ${nativeMode === 'qrcode' ? 'bg-white text-slate-950 shadow-sm dark:bg-white/10 dark:text-white' : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'}`}
                    >
                      <QrCode className="h-4 w-4" /> 钱包扫码
                    </button>
                    <button
                      type="button"
                      role="tab"
                      aria-selected={nativeMode === 'login'}
                      onClick={() => setNativeMode('login')}
                      className={`flex h-10 flex-1 items-center justify-center gap-2 rounded-lg text-xs font-bold transition ${nativeMode === 'login' ? 'bg-white text-slate-950 shadow-sm dark:bg-white/10 dark:text-white' : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'}`}
                    >
                      <KeyRound className="h-4 w-4" /> 账户支付
                    </button>
                  </div>

                  {nativeMode === 'qrcode' ? (
                    <div className="grid items-center gap-7 sm:grid-cols-[190px_1fr]">
                      <div className="mx-auto text-center">
                        <div className="rounded-2xl border border-emerald-200 bg-white p-3 shadow-[0_12px_35px_rgba(16,185,129,0.12)]">
                          <img src={walletQrUrl} alt="Sandbox 钱包付款二维码" className="h-40 w-40 object-contain" />
                        </div>
                        <p className="mt-2 text-[11px] text-slate-500 dark:text-slate-400">二维码将在付款后自动失效</p>
                      </div>
                      <div>
                        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
                          <Smartphone className="h-5 w-5" />
                        </div>
                        <h2 className="mt-3 text-base font-bold">打开 WePay 钱包</h2>
                        <p className="mt-1 text-xs leading-5 text-slate-500 dark:text-slate-400">在沙箱钱包中选择“扫一扫”，扫描二维码后确认本次付款。</p>
                        <Button
                          onPress={() => setShowScannerModal(true)}
                          className="mt-5 h-12 w-full rounded-md bg-emerald-500 px-5 text-sm font-bold text-white hover:bg-emerald-600"
                        >
                          <ScanLine className="h-4 w-4" />
                          模拟钱包扫码支付
                          <ChevronRight className="ml-auto h-4 w-4" />
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <div className="mx-auto max-w-md">
                      <div className="mb-5 text-center">
                        <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-xl bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
                          <ShieldCheck className="h-6 w-6" />
                        </div>
                        <h2 className="mt-3 text-base font-bold">验证沙箱钱包账户</h2>
                        <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">留空可使用预置体验账户完成支付</p>
                      </div>

                      <div className="space-y-4">
                        <label className="block">
                          <span className="mb-1.5 block text-xs font-semibold text-slate-700 dark:text-slate-300">钱包账户</span>
                          <span className="flex h-12 items-center rounded-xl border border-slate-200 bg-white px-3.5 transition focus-within:border-emerald-500 focus-within:ring-2 focus-within:ring-emerald-500/10 dark:border-white/10 dark:bg-black/20">
                            <UserRound className="h-4 w-4 shrink-0 text-slate-400" />
                            <input
                              type="text"
                              value={walletUser}
                              onChange={event => setWalletUser(event.target.value)}
                              autoComplete="username"
                              placeholder="demo_buyer@sandbox-pay.local"
                              className="h-full min-w-0 flex-1 bg-transparent px-3 text-sm outline-none placeholder:text-slate-400"
                            />
                          </span>
                        </label>
                        <label className="block">
                          <span className="mb-1.5 block text-xs font-semibold text-slate-700 dark:text-slate-300">支付密码</span>
                          <span className="flex h-12 items-center rounded-xl border border-slate-200 bg-white px-3.5 transition focus-within:border-emerald-500 focus-within:ring-2 focus-within:ring-emerald-500/10 dark:border-white/10 dark:bg-black/20">
                            <KeyRound className="h-4 w-4 shrink-0 text-slate-400" />
                            <input
                              type="password"
                              value={walletPass}
                              onChange={event => setWalletPass(event.target.value)}
                              autoComplete="current-password"
                              placeholder="默认密码 123456"
                              className="h-full min-w-0 flex-1 bg-transparent px-3 text-sm tracking-wide outline-none placeholder:tracking-normal placeholder:text-slate-400"
                            />
                          </span>
                        </label>
                      </div>

                      <Button
                        isDisabled={payingNative}
                        onPress={handleNativePay}
                        className="mt-5 h-12 w-full rounded-md bg-emerald-500 px-5 text-sm font-bold text-white hover:bg-emerald-600 disabled:opacity-60"
                      >
                        {payingNative ? <RefreshCw className="h-4 w-4 animate-spin" /> : <LockKeyhole className="h-4 w-4" />}
                        {payingNative ? '正在处理支付…' : `确认支付 ¥${info.amount.toFixed(2)}`}
                      </Button>
                    </div>
                  )}
                </div>
              )}
            </div>

            <div className="flex flex-wrap items-center justify-center gap-x-6 gap-y-2 border-t border-slate-100 bg-slate-50/70 px-5 py-4 text-[11px] font-medium text-slate-500 dark:border-white/[0.07] dark:bg-white/[0.02] dark:text-slate-400">
              <span className="flex items-center gap-1.5"><ShieldCheck className="h-3.5 w-3.5 text-emerald-500" /> TLS 加密传输</span>
              <span className="flex items-center gap-1.5"><Clock3 className="h-3.5 w-3.5 text-emerald-500" /> 实时确认结果</span>
              <span className="flex items-center gap-1.5"><Headphones className="h-3.5 w-3.5 text-emerald-500" /> 支付问题可联系商户</span>
            </div>
          </section>

          <aside className="min-w-0 lg:sticky lg:top-6">
            <OrderInfoCard info={info} countdown={countdown} copied={copied} onCopy={handleCopy} />
            <div className="mt-4 flex items-center justify-center gap-2 text-[11px] text-slate-400 dark:text-slate-500">
              <Sparkles className="h-3.5 w-3.5" />
              由 WePay 提供支付技术服务
            </div>
          </aside>
        </div>

        {showScannerModal && (
          <ScannerModal
            isOpen={showScannerModal}
            onOpenChange={setShowScannerModal}
            orderNo={info.orderNo}
            amount={info.amount}
            onPaid={fetchInfo}
          />
        )}
      </main>
    </div>
  )
}
