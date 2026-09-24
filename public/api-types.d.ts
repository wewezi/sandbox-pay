/**
 * 后端接口返回的形状，给前端的 checkJs 用。
 *
 * 这是一份**独立**的声明文件：不 import 后端的任何东西。
 *
 *   1. 后端是 Go，前端的 tsconfig 只看 public/。
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
export type GroupWithKeys = {
  id: number;
  name: string;
  isActive: boolean;
  createdAt: string;
  /** 明文，沙箱自用要给人抄 */
  keys: GroupKey[];
};

/**
 * 单把 Key。`POST /admin/groups/:id/keys` 的返回也是这个形状
 * （那边还多 groupId 和 updatedAt，是这里的超集）。
 */
export type GroupKey = {
  id: number;
  name: string;
  key: string;
  isActive: boolean;
  /** 没用过的 Key 为 undefined（Go 那边是 omitempty） */
  lastUsedAt?: string;
  createdAt: string;
};

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
 * GET URL。cashier.js 见到 data 以 http 开头就直接 location.href ——
 * 原来那段自提交 form HTML 已经没有了（CSP 不让跑内联 script），
 * type 保留 'form' 只是为了不改已有的两值字面量。
 */
export type GatewayPayResult = {
  type: 'url' | 'form';
  data: string;
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
  status: AdminTxStatus;
  /** ISO 字符串。相对时间由前端的 <sp-time> 算，后端不拼「17分钟前」。 */
  createdAt: string;
  payAt?: string;
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

type CashierBase = {
  orderNo: string;
  amount: number;
  productName: string;
};

/** `GET /native-pay/cashier`，按 status 分成三种。 */
export type CashierPending = CashierBase & {
  status: 'pending';
  expireAt: string;
  /** pending 路径现在没回这个字段；有的话用来决定要不要显示钱包核销 */
  payMethod?: string;
  paymentInfo: {
    qrCodeUrl?: string;
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
};

export type CashierClosed = CashierBase & {
  status: 'expired' | 'failed';
};

export type CashierInfo = CashierPending | CashierPaid | CashierClosed;
