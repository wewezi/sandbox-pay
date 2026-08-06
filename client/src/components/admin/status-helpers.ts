/**
 * 订单状态标签与颜色（统一定义，各页面共享）
 */
export const ORDER_STATUS_LABEL: Record<string, string> = {
  SUCCESS: '成功',
  PENDING: '待支付',
  FAILED: '失败',
  REFUNDING: '退款中',
  REFUNDED: '已退款',
}

export const ORDER_STATUS_COLOR: Record<string, string> = {
  SUCCESS: 'bg-emerald-500/10 text-emerald-500',
  PENDING: 'bg-amber-500/10 text-amber-500',
  FAILED: 'bg-rose-500/10 text-rose-500',
  REFUNDING: 'bg-blue-500/10 text-blue-500',
  REFUNDED: 'bg-zinc-500/10 text-zinc-500',
}

/**
 * 通知状态标签与颜色
 */
export const NOTIFY_STATUS_MAP: Record<number, { label: string; color: string }> = {
  0: { label: '待发送', color: 'bg-amber-500/10 text-amber-500' },
  1: { label: '已成功', color: 'bg-emerald-500/10 text-emerald-500' },
  2: { label: '失败', color: 'bg-rose-500/10 text-rose-500' },
  3: { label: '已耗尽', color: 'bg-zinc-500/10 text-zinc-500' },
}

/**
 * 对账状态标签与颜色
 */
export const RECON_STATUS_MAP: Record<string, { label: string; color: string }> = {
  matched: { label: '已匹配', color: 'bg-emerald-500/10 text-emerald-500' },
  amount_mismatch: { label: '金额不符', color: 'bg-rose-500/10 text-rose-500' },
  missing_local: { label: '本地缺失', color: 'bg-amber-500/10 text-amber-500' },
  missing_upstream: { label: '上游缺失', color: 'bg-blue-500/10 text-blue-500' },
}
