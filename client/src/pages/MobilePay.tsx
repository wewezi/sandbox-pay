import { useCallback, useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import {
  AlertCircle,
  ArrowLeft,
  Check,
  ChevronRight,
  Clock3,
  LockKeyhole,
  RefreshCw,
  ShieldCheck,
  WalletCards,
} from 'lucide-react'
import { api, ApiError } from '../utils/api'
import { useOrderStatus } from '../utils/socket'
import { showToast as toast } from '../utils/toast'
import type { CashierInfo } from '../components/dashboard/types'
import BrandMark from '../components/BrandMark'

function closePage() {
  if (window.opener) {
    window.close()
    return
  }
  if (window.history.length > 1) window.history.back()
  else window.location.href = '/'
}

export default function MobilePay() {
  const [searchParams] = useSearchParams()
  const orderNo = searchParams.get('orderNo') || ''
  const [info, setInfo] = useState<CashierInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [paying, setPaying] = useState(false)
  const [paidSuccess, setPaidSuccess] = useState(false)
  const [countdown, setCountdown] = useState('')
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const fetchInfo = useCallback(async () => {
    if (!orderNo) return
    try {
      const data = await api.get<CashierInfo>(`/native-pay/cashier?orderNo=${encodeURIComponent(orderNo)}`)
      setInfo(data)
      setError('')
      if (data.status === 'paid') {
        setPaidSuccess(true)
        if (pollRef.current) clearInterval(pollRef.current)
      } else if (data.status === 'expired' || data.status === 'failed') {
        if (pollRef.current) clearInterval(pollRef.current)
      }
    } catch (err: unknown) {
      if (err instanceof ApiError) setError(err.message || '获取订单失败')
      else setError('网络连接异常，请稍后重试')
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

  useEffect(() => {
    if (!info?.expireAt || info.status !== 'pending') return
    const update = () => {
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
    update()
    const timer = setInterval(() => {
      if (!update()) clearInterval(timer)
    }, 1000)
    return () => clearInterval(timer)
  }, [info?.expireAt, info?.status, fetchInfo])

  const handleWsStatus = useCallback((status: string) => {
    if (status === 'paid' || status === 'refunded' || status === 'failed') {
      if (pollRef.current) clearInterval(pollRef.current)
      fetchInfo()
    }
  }, [fetchInfo])
  useOrderStatus(orderNo, handleWsStatus)

  const handleConfirmPay = async () => {
    if (!info || paying || info.status !== 'pending') return
    setPaying(true)
    try {
      await api.post('/native-pay/sandbox-confirm', {
        orderNo: info.orderNo,
        walletUser: 'mobile_scan@sandbox-pay.local',
        walletPass: '123456',
      })
      setPaidSuccess(true)
      if (pollRef.current) clearInterval(pollRef.current)
    } catch (err: unknown) {
      toast.error(`支付失败：${err instanceof Error ? err.message : '订单暂时无法支付'}`)
    } finally {
      setPaying(false)
    }
  }

  if (!orderNo) {
    return <MobileError message="付款链接缺少订单号，请回到电脑端重新扫码" />
  }

  if (loading) {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center bg-background text-foreground">
        <BrandMark size="lg" />
        <RefreshCw className="mt-6 h-5 w-5 animate-spin text-emerald-600 dark:text-emerald-400" />
        <p className="mt-3 text-xs text-[#888]">正在安全加载订单…</p>
      </div>
    )
  }

  if (error || !info) return <MobileError message={error || '未找到对应订单'} />

  const unavailable = info.status === 'expired' || info.status === 'failed'

  return (
    <div className="flex min-h-screen flex-col bg-background text-foreground">
      <header className="sticky top-0 z-30 border-b border-black/[0.04] bg-white/95 backdrop-blur-lg dark:border-white/10 dark:bg-[#181c19]/95">
        <div className="mx-auto flex h-14 max-w-md items-center justify-between px-4">
          <button type="button" onClick={closePage} aria-label="返回" className="flex h-8 w-8 items-center justify-center rounded-lg text-[#555] hover:bg-black/5 dark:text-slate-300 dark:hover:bg-white/5">
            <ArrowLeft className="h-5 w-5" />
          </button>
          <div className="flex items-center gap-2 text-sm font-semibold">
            <BrandMark size="sm" className="!h-7 !w-7 !rounded-lg" />
            WePay
          </div>
          <span className="flex h-8 w-8 items-center justify-center text-emerald-600 dark:text-emerald-400"><ShieldCheck className="h-[18px] w-[18px]" /></span>
        </div>
      </header>

      <main className="mx-auto flex w-full max-w-md flex-1 flex-col px-4 py-5">
        <div className="mb-4 rounded-lg bg-[#fff7e6] px-3 py-2.5 text-[11px] leading-5 text-[#b26b00] dark:bg-amber-400/10 dark:text-amber-200">
          沙箱测试付款，不会产生真实资金扣款
        </div>

        {paidSuccess ? (
          <section className="rounded-xl bg-white px-6 py-9 text-center shadow-[0_1px_2px_rgba(0,0,0,0.03)] dark:bg-[#181c19]">
            <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-emerald-600 text-white">
              <Check className="h-8 w-8 stroke-[3]" />
            </div>
            <h1 className="mt-5 text-xl font-semibold">支付成功</h1>
            <p className="mt-2 text-xs text-[#888] dark:text-slate-400">支付结果已同步至电脑端收银台</p>
            <div className="mt-6 text-[36px] font-semibold tracking-tight tabular-nums">
              <span className="mr-1 text-xl">¥</span>{info.amount.toFixed(2)}
            </div>
            <div className="mt-7 border-t border-[#ededed] pt-5 text-xs dark:border-white/10">
              <div className="flex justify-between gap-6 py-1.5"><span className="text-[#888]">商品</span><span className="text-right font-medium">{info.productName}</span></div>
              <div className="flex justify-between gap-6 py-1.5"><span className="text-[#888]">订单号</span><span className="truncate font-mono text-[10px]">{info.orderNo}</span></div>
            </div>
            <button type="button" onClick={closePage} className="mt-7 h-11 w-full rounded-lg bg-emerald-600 text-sm font-semibold text-white active:bg-emerald-700">完成</button>
          </section>
        ) : unavailable ? (
          <section className="rounded-xl bg-white px-6 py-10 text-center dark:bg-[#181c19]">
            <AlertCircle className="mx-auto h-12 w-12 text-[#fa9d3b]" />
            <h1 className="mt-4 text-lg font-semibold">{info.status === 'expired' ? '订单已过期' : '订单暂时无法支付'}</h1>
            <p className="mt-2 text-xs leading-5 text-[#888]">请关闭当前页面，并在电脑端重新发起付款。</p>
            <button type="button" onClick={closePage} className="mt-6 h-11 w-full rounded-lg border border-[#dcdcdc] text-sm font-medium dark:border-white/15">关闭页面</button>
          </section>
        ) : (
          <section className="overflow-hidden rounded-xl bg-white shadow-[0_1px_2px_rgba(0,0,0,0.03)] dark:bg-[#181c19]">
            <div className="px-6 pb-7 pt-8 text-center">
              <div className="mx-auto flex h-11 w-11 items-center justify-center rounded-xl bg-emerald-50 text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-400">
                <WalletCards className="h-5 w-5" />
              </div>
              <p className="mt-3 text-xs text-[#888]">向 WePay 测试商户付款</p>
              <div className="mt-3 text-[38px] font-semibold tracking-tight tabular-nums">
                <span className="mr-1 text-xl">¥</span>{info.amount.toFixed(2)}
              </div>
              <p className="mt-2 truncate text-[13px] font-medium">{info.productName}</p>
              {countdown && (
                <p className="mt-3 flex items-center justify-center gap-1.5 text-[11px] text-[#fa9d3b]">
                  <Clock3 className="h-3.5 w-3.5" /> 请在 {countdown} 内完成付款
                </p>
              )}
            </div>

            <div className="border-t border-[#ededed] px-5 py-2 dark:border-white/10">
              <div className="flex items-center justify-between py-3.5 text-[13px]">
                <span className="text-[#888]">支付方式</span>
                <span className="flex items-center gap-2 font-medium"><BrandMark size="sm" className="!h-6 !w-6 !rounded-md" /> Sandbox 钱包 <ChevronRight className="h-4 w-4 text-[#bbb]" /></span>
              </div>
              <div className="flex items-center justify-between border-t border-[#ededed] py-3.5 text-[13px] dark:border-white/10">
                <span className="text-[#888]">订单号</span>
                <span className="max-w-[220px] truncate font-mono text-[10px]">{info.orderNo}</span>
              </div>
            </div>

            <div className="px-5 pb-5 pt-4">
              <button
                type="button"
                disabled={paying}
                onClick={handleConfirmPay}
                className="flex h-12 w-full items-center justify-center gap-2 rounded-lg bg-emerald-600 text-[15px] font-semibold text-white transition-colors active:bg-emerald-700 disabled:opacity-60"
              >
                {paying ? <RefreshCw className="h-4 w-4 animate-spin" /> : <LockKeyhole className="h-4 w-4" />}
                {paying ? '正在支付…' : '立即支付'}
              </button>
              <p className="mt-3 flex items-center justify-center gap-1.5 text-[10px] text-[#aaa]"><ShieldCheck className="h-3 w-3 text-emerald-600 dark:text-emerald-400" /> WePay 安全支付</p>
            </div>
          </section>
        )}
      </main>

      <footer className="pb-5 text-center text-[10px] text-[#aaa]">本页面仅用于沙箱支付流程测试</footer>
    </div>
  )
}

function MobileError({ message }: { message: string }) {
  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-5 text-foreground">
      <div className="w-full max-w-sm rounded-xl bg-white px-6 py-9 text-center dark:bg-[#181c19]">
        <AlertCircle className="mx-auto h-12 w-12 text-[#fa5151]" />
        <h1 className="mt-4 text-lg font-semibold">无法打开付款页面</h1>
        <p className="mt-2 text-xs leading-5 text-[#888]">{message}</p>
        <button type="button" onClick={closePage} className="mt-6 h-11 w-full rounded-lg border border-[#dcdcdc] text-sm font-medium dark:border-white/15">关闭页面</button>
      </div>
    </div>
  )
}
