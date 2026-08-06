// ── Transaction / Order ──────────────────────────────────────

export interface Transaction {
  id: string
  orderNo: string
  channel: 'ALIPAY' | 'WECHAT' | 'PAYPAL' | 'USDT' | 'NATIVE'
  payMethod?: string
  amount: number
  refundedAmount?: number
  fee?: number
  settleAmount?: number
  productName?: string
  externalOrderNo?: string
  thirdPartyTradeNo?: string
  status: 'SUCCESS' | 'PENDING' | 'FAILED' | 'REFUNDING' | 'REFUNDED'
  time: string
  rawDate?: string
  payAt?: string
}

export interface TransactionsResponse {
  items: Transaction[]
  total: number
  page: number
  pageSize: number
}

export interface Stats {
  totalAmount: string
  successCount: string
  successRate: string
  rawAmount?: number
  rawSuccessCount?: number
  rawTotalCount?: number
}

// ── Merchant ─────────────────────────────────────────────────

export interface MerchantInfo {
  name: string
  appKey: string
  appSecret: string
}

export interface MerchantRecord {
  id: number
  name: string
  appKey: string
  appSecret: string
  isActive: boolean
  createdAt?: string
}

// ── Notification Queue ───────────────────────────────────────

export interface NotificationRecord {
  id: number
  orderNo: string
  url: string
  status: number // 0=Pending, 1=Success, 2=Failed, 3=Exhausted
  retryCount: number
  lastError?: string
  lastAttemptAt?: string
  createdAt?: string
}

export interface NotificationsResponse {
  items: NotificationRecord[]
  total: number
  page: number
  pageSize: number
}

// ── Audit Log ────────────────────────────────────────────────

export interface AuditLogRecord {
  id: number
  action: string
  actor: string
  targetType?: string
  targetId?: string
  ip?: string
  detail?: Record<string, unknown>
  createdAt?: string
}

export interface AuditLogsResponse {
  items: AuditLogRecord[]
  total: number
  page: number
  pageSize: number
}

// ── Cashier ──────────────────────────────────────────────────

export interface CashierInfo {
  orderNo: string
  amount: number
  productName: string
  status: 'pending' | 'paid' | 'expired' | 'failed'
  payMethod?: string
  thirdPartyTradeNo?: string
  expireAt?: string
  payAt?: string
  paymentInfo?: {
    qrCodeUrl: string
    remark: string
  }
}
