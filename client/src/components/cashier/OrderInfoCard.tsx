import { Check, Clock3, Copy, PackageCheck, ReceiptText, ShieldCheck } from 'lucide-react'
import type { CashierInfo } from '../dashboard/types'

interface Props {
  info: CashierInfo
  countdown: string
  copied: string | null
  onCopy: (text: string, key: string) => void
}

export default function OrderInfoCard({ info, countdown, copied, onCopy }: Props) {
  return (
    <div className="overflow-hidden rounded-lg border border-border bg-surface">
      <div className="flex items-center justify-between border-b border-border px-5 py-4">
        <div className="flex items-center gap-2 text-sm font-bold">
          <ReceiptText className="h-4 w-4 text-slate-400" />
          订单摘要
        </div>
        <div className="flex items-center gap-1.5 rounded-full bg-amber-50 px-2.5 py-1 font-mono text-[11px] font-bold tabular-nums text-amber-700 dark:bg-amber-400/10 dark:text-amber-300">
          <Clock3 className="h-3.5 w-3.5" />
          {countdown || '--:--'}
        </div>
      </div>

      <div className="p-5">
        <div className="flex items-start gap-3.5">
          <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-slate-100 text-slate-500 dark:bg-white/[0.06] dark:text-slate-300">
            <PackageCheck className="h-5 w-5" />
          </div>
          <div className="min-w-0 flex-1">
            <p className="text-[11px] font-medium text-slate-500 dark:text-slate-400">Sandbox Pay 测试商户</p>
            <h2 className="mt-0.5 break-words text-sm font-bold leading-5">{info.productName}</h2>
          </div>
        </div>

        <div className="my-5 h-px bg-border" />

        <div className="space-y-3 text-xs">
          <div className="flex items-center justify-between gap-4">
            <span className="text-slate-500 dark:text-slate-400">商品金额</span>
            <span className="font-medium tabular-nums">¥{info.amount.toFixed(2)}</span>
          </div>
          <div className="flex items-center justify-between gap-4">
            <span className="text-slate-500 dark:text-slate-400">支付手续费</span>
            <span className="font-medium text-emerald-600 dark:text-emerald-400">¥0.00</span>
          </div>
        </div>

        <div className="my-5 border-t border-dashed border-slate-200 dark:border-white/10" />

        <div className="flex items-end justify-between gap-4">
          <div>
            <p className="text-xs font-semibold text-slate-600 dark:text-slate-300">应付金额</p>
            <p className="mt-1 text-[10px] text-slate-400">人民币 CNY</p>
          </div>
          <p className="text-[28px] font-bold tracking-tight tabular-nums text-slate-950 dark:text-white">
            <span className="mr-1 text-base font-semibold">¥</span>{info.amount.toFixed(2)}
          </p>
        </div>

        <div className="mt-5 rounded-lg bg-surface-secondary p-3">
          <div className="flex items-center justify-between gap-3">
            <span className="shrink-0 text-[10px] text-slate-500 dark:text-slate-400">订单号</span>
            <button
              type="button"
              onClick={() => onCopy(info.orderNo, 'orderNo')}
              title={info.orderNo}
              className="flex min-w-0 items-center gap-1.5 font-mono text-[10px] font-medium text-slate-600 hover:text-slate-950 dark:text-slate-300 dark:hover:text-white"
            >
              <span className="truncate">{info.orderNo}</span>
              {copied === 'orderNo' ? <Check className="h-3 w-3 shrink-0 text-emerald-500" /> : <Copy className="h-3 w-3 shrink-0" />}
            </button>
          </div>
        </div>

        <div className="mt-4 flex items-start gap-2 text-[10px] leading-4 text-slate-400 dark:text-slate-500">
          <ShieldCheck className="mt-0.5 h-3.5 w-3.5 shrink-0 text-emerald-500" />
          请确认商品与金额无误后再付款。支付结果将实时同步给商户。
        </div>
      </div>
    </div>
  )
}
