/**
 * 后端接口返回的形状，供 TypeScript 页面使用。
 *
 * 这是一份**独立**的声明文件：不 import 后端的任何东西。
 *
 *   1. 后端是 Go，前端只检查 web/ 中的 TypeScript。
 *      引 Go 源码既引不来类型（tsc 不认 Go 的 struct tag），
 *      也会把外部文件拖进这个程序。契约只能在这儿手写一份 ——
 *      改了 Go 那边的 DTO 字段，这里要一起改，否则 tsc 不会报，
 *      只会在运行时渲染出 "undefined"。
 *
 *   2. 日期是 string，不是 Date。
 *      Go 那边存的是 time.Time，但序列化之后到前端手里是
 *      `2006-01-02T15:04:05.000Z` 这样的 ISO 字符串。直接标成 Date
 *      会让 tsc 以为能调 .getTime()，运行时才炸。
 *
 * 后台列表接口（订单 / 通知 / 统计）在 service 里已经把分转成元、
 * 状态转成展示名，形状和库里的行对不上，那些在下面单独写。
 *
 * 但**展示格式**不在后端做：金额符号、千分位、百分号、相对时间
 * 都是 locale 的事，接口只回数和 ISO 时间戳，前端用 Intl 渲染。
 */

/** `GET /admin/groups` 的一项。对应后端 services.GroupWithKeys。 */
export type RefundResult = {
  orderNo: string;
  refundedAmount: number;
  refundTradeNo: string;
  currency: string;
  status: 'completed' | 'pending';
  requestId: string;
};

export type GroupWithKeys = {
  id: number;
  name: string;
  isActive: boolean;
  createdAt: string;
  webhookConfigured: boolean;
  keys: GroupKey[];
};

/**
 * 列表只包含令牌元数据。完整 key 仅在创建回执中返回。
 */
export type GroupKey = {
  id: number;
  name: string;
  keyPrefix: string;
  keySuffix: string;
  key?: string;
  isActive: boolean;
  /** 没用过的 Key 为 undefined（Go 那边是 omitempty） */
  lastUsedAt?: string;
  createdAt: string;
};

export type APIKeyDTO = GroupKey & {
  groupId: number;
  groupName: string;
  groupIsActive: boolean;
  updatedAt: string;
};

export type IssuedAPIKey = APIKeyDTO & { key: string };

/** `POST /auth/login` */
export type LoginResult = {
  token: string;
  tokenType: string;
  username: string;
  role: string;
};

/**
 * 下单 / 测试下单 / 切渠道的跳转信息。
 *
 * data **一律是链接**：native 和 PayPal 是 approve 页，支付宝是签名好的
 * GET URL。cashier.ts 收到链接后跳转 ——
 * 原来那段自提交 form HTML 已经没有了（CSP 不让跑内联 script），
 * type 保留 'form' 只是为了不改已有的两值字面量。
 */
export type GatewayPayResult = {
  type: 'url' | 'form';
  data: string;
};

export type AdminTestPayResult = GatewayPayResult & {
  orderNo: string;
  status: number;
  payMethod?: 'native' | 'alipay' | 'paypal';
  createdAt?: string;
  amount: number;
  expireAt: string;
  productName?: string;
  externalOrderNo?: string;
  notifyUrl?: string;
  returnUrl?: string;
  groupName?: string;
  apiKeyId?: number;
  apiKeyName?: string;
};

/**
 * `GET /admin/stats`。全是数，展示格式由前端的 Intl 决定。
 *
 * successRate 是 0..1 的比例；一笔订单都没有时为 null ——
 * 空账本上「0%」和「暂无数据」不是一个意思。
 */
export type AdminStats = {
  /** 元 */
  amount: number;
  successCount: number;
  totalCount: number;
  successRate: number | null;
};

/**
 * 后台订单列表的状态名。和库里的状态数字不是一套 ——
 * service 的 statusOf 把 Paid 叫 SUCCESS、Expired 并进 FAILED。
 */
export type AdminTxStatus =
  | 'PENDING'
  | 'SUCCESS'
  | 'FAILED'
  | 'REFUNDING'
  | 'REFUNDED';

/** `GET /admin/transactions` 的一项。金额已经是元。 */
export type GeoIPLocation = {
  country?: string;
  region?: string;
  city?: string;
  isp?: string;
  network?: 'public' | 'private' | 'loopback' | 'link_local' | 'reserved' | 'unknown';
};

export type AdminTransaction = {
  id: string;
  orderNo: string;
  /** ALIPAY / PAYPAL / NATIVE，前端有自己的文案表 */
  channel: string;
  amount: number;
  refundedAmount: number;
  fee: number;
  settleAmount: number;
  payMethod: string;
  productName: string;
  externalOrderNo?: string;
  thirdPartyTradeNo?: string;
  refundRequestId?: string;
  pendingRefundAmount?: number;
  status: AdminTxStatus;
  /** 数据库原始状态：-1 表示已过期，和支付失败分开显示。 */
  rawStatus?: number;
  groupId?: number;
  groupName?: string;
  apiKeyId?: number;
  apiKeyName?: string;
  notifyUrl?: string;
  notifyStatus?: number;
  notifyRetryCount?: number;
  notifyLastError?: string;
  channelCost?: number;
  clientIp?: string;
  clientLocation?: GeoIPLocation;
  /** ISO 字符串。相对时间由前端的 <sp-time> 算，后端不拼「17分钟前」。 */
  createdAt: string;
  payAt?: string;
  updatedAt?: string;
  expireAt?: string;
};

export type AdminRefundOperation = {
  id: string;
  requestId: string;
  reason: string;
  amount: number;
  tradeNo?: string;
  status: 'processing' | 'pending' | 'unknown' | 'completed' | 'failed';
  createdAt: string;
  updatedAt: string;
};

/** 后台订单详情；所有金额均为主货币单位，凭证密钥不在此响应中。 */
export type AdminOrderDetail = AdminTransaction & {
  rawStatus: number;
  updatedAt: string;
  expireAt: string;
  channelCost: number;
  returnUrl?: string;
  providerOrderId?: string;
  exchangeRate?: number;
  foreignAmount?: number;
  foreignCurrency?: string;
  refundTradeNo?: string;
  refundAt?: string;
  refunds: AdminRefundOperation[];
  notifications: AdminOrderNotification[];
};

export type Page<T> = {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
};

export type AdminTxPage = Page<AdminTransaction>;

/**
 * `GET /admin/notifications` 的一项。status 仍是状态数字：
 * 0=Pending, 1=Success, 2=Failed, 3=Exhausted。
 *
 * orderNo / lastError / lastAttemptAt 在库里的行是可空的，Go 那边
 * 不带 omitempty，所以没值时是 null 而不是字段缺失。
 */
export type AdminNotification = {
  id: number;
  orderNo: string | null;
  url: string;
  status: number;
  retryCount: number;
  lastError: string | null;
  lastAttemptAt: string | null;
  createdAt: string;
};

export type AdminNotifyPage = Page<AdminNotification>;

/** API 调用记录；只含请求元数据，不包含令牌、请求头或请求正文。 */
export type AdminUsageLog = {
  id: number;
  requestId: string;
  method: string;
  path: string;
  statusCode: number;
  durationMs: number;
  groupId?: number;
  groupName?: string;
  apiKeyId?: number;
  apiKeyName?: string;
  keyPrefix?: string;
  keySuffix?: string;
  clientIp?: string;
  clientLocation?: GeoIPLocation;
  orderNo?: string;
  errorMessage?: string;
  createdAt: string;
};

export type AdminUsageSummary = {
  totalCount: number;
  successCount: number;
  failureCount: number;
  averageDurationMs: number;
};

export type AdminUsagePage = Page<AdminUsageLog> & { summary: AdminUsageSummary };

export type AdminOrderNotification = AdminNotification & {
  body: string;
};

type CashierBase = {
  orderNo: string;
  externalOrderNo?: string;
  projectName?: string;
  createdAt?: string;
  amount: number;
  productName: string;
  returnUrl?: string;
};

/** `GET /native-pay/cashier`，按 status 分成三种。 */
export type CashierPending = CashierBase & {
  status: 'pending';
  expireAt: string;
  /** 待付订单当前选择的渠道，可在付款前更换。 */
  payMethod: string;
  availableChannels?: ('native' | 'alipay' | 'paypal')[];
  channelLockReason?: string;
  paypalQuote?: {
    amount: number | null;
    currency: 'USD';
    exchangeRate: number | null;
    available: boolean;
    reason?: string;
  };
  paymentInfo: {
    qrCodeUrl?: string;
    qrCodeChannel?: 'native' | 'alipay';
    accountName?: string;
    accountNo?: string;
    bankName?: string;
    remark?: string;
  };
};

export type CashierPaid = CashierBase & {
  status: 'paid';
  payMethod: string;
  thirdPartyTradeNo: string;
  payAt: string;
  foreignAmount?: number;
  foreignCurrency?: string;
  exchangeRate?: number;
};

export type CashierClosed = CashierBase & {
  status: 'expired' | 'failed';
};

export type CashierInfo = CashierPending | CashierPaid | CashierClosed;

/** Dashboard aggregates use orders created in the selected calendar period. Amounts are yuan. */
export type DashboardSummary = {
  amount: number;
  successCount: number;
  totalCount: number;
  successRate: number | null;
  refundedAmount: number;
  fee: number;
};

export type DashboardTrend = {
  date: string;
  amount: number;
  successCount: number;
  totalCount: number;
};

export type DashboardChannel = {
  payMethod: string;
  amount: number;
  successCount: number;
  totalCount: number;
  successRate: number | null;
  refundedAmount: number;
};

/** 「最近订单」的一行。status 是后台订单页那套展示名，不是状态数字。 */
export type DashboardRecentOrder = {
  orderNo: string;
  productName: string;
  /** 小写：native / alipay / paypal */
  payMethod: string;
  status: AdminTxStatus;
  amount: number;
  createdAt: string;
};

/** `GET /admin/dashboard`; tasks always describe the current global backlog. */
export type AdminDashboard = {
  days: number;
  utcOffset: number;
  dateFrom: string;
  dateTo: string;
  generatedAt: string;
  summary: DashboardSummary;
  previous: DashboardSummary;
  trend: DashboardTrend[];
  channels: DashboardChannel[];
  /** 全库最新的 8 笔，不受 days 限制。 */
  recentOrders: DashboardRecentOrder[];
  tasks: {
    pendingOrders: number;
    refundingOrders: number;
    pendingNotifications: number;
    failedNotifications: number;
    exhaustedNotifications: number;
    activeGroups: number;
    activeKeys: number;
  };
};
