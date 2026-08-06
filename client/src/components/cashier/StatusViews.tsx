import { Button } from '@heroui/react'
import {
  AlertTriangle,
  ArrowLeft,
  Check,
  CheckCircle2,
  Clock3,
  Copy,
  Printer,
  RefreshCw,
  ShieldCheck,
  WalletCards,
  XCircle,
} from 'lucide-react'
import { FaAlipay, FaPaypal } from 'react-icons/fa6'
import type { CashierInfo } from '../dashboard/types'
import BrandMark from '../BrandMark'

function CheckoutHeader() {
  return (
    <header className="border-b border-border bg-surface print:hidden">
      <div className="mx-auto flex h-[72px] max-w-[960px] items-center justify-between px-5 sm:px-8">
        <div className="flex items-center gap-3">
          <BrandMark size="sm" />
          <div>
            <p className="text-[15px] font-bold tracking-tight">WePay</p>
            <p className="text-[11px] font-medium text-slate-500 dark:text-slate-400">安全收银台</p>
          </div>
        </div>
        <div className="flex items-center gap-1.5 text-[11px] font-semibold text-slate-500 dark:text-slate-400">
          <ShieldCheck className="h-3.5 w-3.5 text-emerald-500" />
          加密连接
        </div>
      </div>
    </header>
  )
}

function leaveCheckout() {
  if (window.opener) {
    window.close()
    return
  }
  if (window.history.length > 1) {
    window.history.back()
    return
  }
  window.location.href = '/'
}

function channelName(payMethod?: string) {
  if (payMethod === 'alipay') return '支付宝'
  if (payMethod === 'paypal') return 'PayPal'
  return 'Sandbox 钱包'
}

function ChannelIcon({ payMethod }: { payMethod?: string }) {
  if (payMethod === 'alipay') return <FaAlipay className="h-4 w-4 text-[#1677ff]" />
  if (payMethod === 'paypal') return <FaPaypal className="h-4 w-4 text-[#0070ba]" />
  return <WalletCards className="h-3.5 w-3.5 text-emerald-500" />
}

export function PaidView({ info }: { info: CashierInfo }) {
  const copyTradeNo = async () => {
    if (!info.thirdPartyTradeNo) return
    await navigator.clipboard.writeText(info.thirdPartyTradeNo)
  }

  return (
    <div className="min-h-screen bg-background text-foreground">
      <CheckoutHeader />
      <main className="mx-auto max-w-[560px] px-4 py-10 sm:px-6 sm:py-14">
        <div className="overflow-hidden rounded-lg border border-border bg-surface">
          <div className="px-6 pb-7 pt-8 text-center sm:px-9">
            <div className="relative mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-emerald-500 text-white shadow-[0_12px_30px_rgba(16,185,129,0.3)]">
              <Check className="h-8 w-8 stroke-[3]" />
              <span className="absolute -inset-2 -z-10 rounded-full bg-emerald-500/10" />
            </div>
            <h1 className="mt-5 text-2xl font-bold tracking-tight">支付成功</h1>
            <p className="mt-1.5 flex items-center justify-center gap-1.5 text-xs text-slate-500 dark:text-slate-400">
              <CheckCircle2 className="h-3.5 w-3.5 text-emerald-500" />
              支付结果已同步给商户
            </p>
            <div className="mt-6 text-4xl font-bold tracking-tight tabular-nums">
              <span className="mr-1 text-xl font-semibold">¥</span>{info.amount.toFixed(2)}
            </div>
          </div>

          <div className="border-y border-slate-100 bg-slate-50/80 px-6 py-5 dark:border-white/[0.07] dark:bg-white/[0.025] sm:px-9">
            <dl className="space-y-4 text-xs">
              <div className="flex items-start justify-between gap-6">
                <dt className="shrink-0 text-slate-500 dark:text-slate-400">商品</dt>
                <dd className="text-right font-semibold">{info.productName}</dd>
              </div>
              <div className="flex items-center justify-between gap-6">
                <dt className="text-slate-500 dark:text-slate-400">支付方式</dt>
                <dd className="flex items-center gap-1.5 font-semibold">
                  <ChannelIcon payMethod={info.payMethod} />
                  {channelName(info.payMethod)}
                </dd>
              </div>
              <div className="flex items-start justify-between gap-6">
                <dt className="shrink-0 text-slate-500 dark:text-slate-400">订单号</dt>
                <dd className="break-all text-right font-mono text-[11px] font-medium">{info.orderNo}</dd>
              </div>
              {info.thirdPartyTradeNo && (
                <div className="flex items-start justify-between gap-6">
                  <dt className="shrink-0 text-slate-500 dark:text-slate-400">支付流水号</dt>
                  <dd>
                    <button type="button" onClick={copyTradeNo} className="flex items-center gap-1.5 break-all text-right font-mono text-[11px] font-medium hover:text-emerald-600 dark:hover:text-emerald-400">
                      {info.thirdPartyTradeNo}
                      <Copy className="h-3 w-3 shrink-0" />
                    </button>
                  </dd>
                </div>
              )}
              {info.payAt && (
                <div className="flex items-center justify-between gap-6">
                  <dt className="text-slate-500 dark:text-slate-400">支付时间</dt>
                  <dd className="font-mono text-[11px] font-medium">{new Date(info.payAt).toLocaleString('zh-CN', { hour12: false })}</dd>
                </div>
              )}
            </dl>
          </div>

          <div className="grid gap-3 p-6 print:hidden sm:grid-cols-2 sm:px-9">
            <Button
              variant="ghost"
              onPress={() => window.print()}
              className="h-11 w-full rounded-xl border border-slate-200 bg-white text-xs font-bold text-slate-700 hover:bg-slate-50 dark:border-white/10 dark:bg-white/[0.03] dark:text-slate-200 dark:hover:bg-white/[0.06]"
            >
              <Printer className="h-4 w-4" /> 打印支付凭证
            </Button>
            <Button
              onPress={leaveCheckout}
              className="h-11 w-full rounded-md bg-emerald-500 text-xs font-bold text-white hover:bg-emerald-600"
            >
              完成 <ArrowLeft className="h-4 w-4 rotate-180" />
            </Button>
          </div>
        </div>
        <p className="mt-5 text-center text-[11px] text-slate-400 print:hidden">沙箱环境 · 本次交易不会产生真实扣款</p>
      </main>
    </div>
  )
}

export function ExpiredFailedView({ info }: { info: CashierInfo }) {
  const expired = info.status === 'expired'
  return (
    <div className="min-h-screen bg-background text-foreground">
      <CheckoutHeader />
      <main className="mx-auto flex max-w-[520px] items-center px-4 py-16 sm:px-6">
        <div className="w-full rounded-lg border border-border bg-surface p-7 text-center sm:p-9">
          <div className={`mx-auto flex h-14 w-14 items-center justify-center rounded-full ${expired ? 'bg-amber-500/10 text-amber-500' : 'bg-rose-500/10 text-rose-500'}`}>
            {expired ? <Clock3 className="h-7 w-7" /> : <AlertTriangle className="h-7 w-7" />}
          </div>
          <h1 className="mt-5 text-xl font-bold">{expired ? '订单已超时关闭' : '本次支付未完成'}</h1>
          <p className="mx-auto mt-2 max-w-sm text-xs leading-5 text-slate-500 dark:text-slate-400">
            {expired ? '为保障交易安全，订单超过有效支付时间后会自动关闭。请返回商户页面重新下单。' : '支付通道未能完成本次交易，请返回商户页面后重试或更换支付方式。'}
          </p>
          <div className="mt-6 rounded-xl bg-slate-50 px-4 py-3 text-left dark:bg-white/[0.035]">
            <div className="flex items-center justify-between gap-4 text-[11px]">
              <span className="text-slate-500 dark:text-slate-400">订单号</span>
              <span className="truncate font-mono font-medium">{info.orderNo}</span>
            </div>
          </div>
          <Button onPress={leaveCheckout} className="mt-6 h-11 w-full rounded-xl bg-slate-950 text-xs font-bold text-white hover:bg-slate-800 dark:bg-white dark:text-slate-950 dark:hover:bg-slate-200">
            <ArrowLeft className="h-4 w-4" /> 返回商户页面
          </Button>
        </div>
      </main>
    </div>
  )
}

export function ErrorView({ error, onRetry }: { error: string; onRetry?: () => void }) {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <CheckoutHeader />
      <main className="mx-auto flex max-w-[520px] items-center px-4 py-16 sm:px-6">
        <div className="w-full rounded-lg border border-border bg-surface p-7 text-center sm:p-9">
          <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-rose-500/10 text-rose-500">
            <XCircle className="h-7 w-7" />
          </div>
          <h1 className="mt-5 text-xl font-bold">无法打开收银台</h1>
          <p className="mt-2 text-xs leading-5 text-slate-500 dark:text-slate-400">{error}</p>
          <div className="mt-6 grid gap-3 sm:grid-cols-2">
            <Button variant="ghost" onPress={leaveCheckout} className="h-11 rounded-xl border border-slate-200 bg-white text-xs font-bold text-slate-700 dark:border-white/10 dark:bg-white/[0.03] dark:text-slate-200">
              <ArrowLeft className="h-4 w-4" /> 返回
            </Button>
            {onRetry && (
              <Button onPress={onRetry} className="h-11 rounded-xl bg-slate-950 text-xs font-bold text-white dark:bg-white dark:text-slate-950">
                <RefreshCw className="h-4 w-4" /> 重新加载
              </Button>
            )}
          </div>
        </div>
      </main>
    </div>
  )
}

export function LoadingView() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <CheckoutHeader />
      <main className="mx-auto max-w-[560px] px-4 py-16 sm:px-6">
        <div className="rounded-lg border border-border bg-surface p-8 text-center">
          <div className="mx-auto h-9 w-9 animate-spin rounded-full border-[3px] border-slate-200 border-t-emerald-500 dark:border-white/10 dark:border-t-emerald-400" />
          <p className="mt-5 text-sm font-bold">正在加载订单</p>
          <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">正在安全获取支付信息，请稍候…</p>
        </div>
      </main>
    </div>
  )
}
