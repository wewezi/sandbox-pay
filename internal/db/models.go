package db

import "time"

// 四张表的模型 + 状态常量。
//
// 状态用非类型化 const，用法和原版对象常量一致：db.StatusPaid、status int。
//
// 所有金额列都是整数分。元/分的换算只在 API 与网关边界上做，
// 走 money 包的 ToYuan / YuanStringToCents。

// 订单状态（和库存储的整数一一对应）。
const (
	StatusExpired   = -1
	StatusPending   = 0
	StatusPaid      = 1
	StatusFailed    = 2
	StatusRefunding = 3
	StatusRefunded  = 4
)

// 通知队列状态。
const (
	NotifyPending   = 0
	NotifySuccess   = 1
	NotifyFailed    = 2
	NotifyExhausted = 3
)

// 用户角色。
const (
	RoleAdmin      = "Admin"
	RoleSuperAdmin = "SuperAdmin"
)

// User 后台用户。Password 是 bcrypt hash。
type User struct {
	ID        int64
	Username  string
	Password  string
	Role      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Group 分组。停用后它名下所有 Key 都调不通（守卫连表查 isActive）。
type Group struct {
	ID        int64
	Name      string
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// APIKey 一把 API Key。一个分组可以有多把，用来轮换或区分环境。
//
// 两个方向都用它：
//   - 入站 —— 接入方带 Authorization: Bearer <key> 调网关
//   - 出站 —— 我们回调接入方时拿它当 HMAC 密钥签名，接入方照此验签
//
// 明文存储：自托管沙箱、全假数据，加密只增加排错成本。
// 沙箱页还要把它显示出来给人抄，存哈希就没法显示了。
type APIKey struct {
	ID      int64
	GroupID int64
	// Name 备注名，纯给人看的（"本地开发"、"CI"），不参与鉴权。
	Name     string
	Key      string
	IsActive bool
	// LastUsedAt 守卫每次校验通过后刷新，用来判断哪把 Key 还在用。
	LastUsedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ActiveKeyHit 入站鉴权命中：Key + 它所属的分组。
type ActiveKeyHit struct {
	Key   *APIKey
	Group *Group
}

// PaymentOrder 支付订单。金额列全部是整数分。
type PaymentOrder struct {
	ID          string
	OrderNo     string
	Amount      int64
	ProductName string
	PayMethod   string
	Status      int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ExpireAt    time.Time
	PayAt       *time.Time
	// ExchangeRate 下单时记下的汇率（CNY → 外币），PayPal 单有值。
	ExchangeRate *float64
	// ForeignAmount foreignCurrency 的最小单位金额。
	ForeignAmount     *int64
	ForeignCurrency   *string
	ExternalOrderNo   *string
	ThirdPartyTradeNo *string
	// RefundedAmount 整数分。
	RefundedAmount int64
	RefundTradeNo  *string
	RefundAt       *time.Time
	// ChannelCost 上游渠道（支付宝/PayPal）收的成本，整数分。
	ChannelCost int64
	// Fee Sandbox Pay 向接入方收的手续费，整数分。
	Fee int64
	// SettleAmount 扣掉 fee 后应结给接入方的金额，markPaid 时算出来，整数分。
	SettleAmount int64
	ReturnURL    *string
	NotifyURL    *string
	GroupID      *int64
	// APIKeyID 下单时用的那把 Key。回调签名认它 —— 谁下的单谁验签。
	// 后台测试下单没有 Key 上下文，这里是 nil；那些单也不带 notifyUrl。
	APIKeyID *int64
	UserID   int64
	// APIKey JOIN 出来的 Key，只有需要它签回调时才带上（withAPIKey）。
	APIKey *APIKey
}

// NotifyQueue 异步通知队列的一行。
type NotifyQueue struct {
	ID            int64
	OrderNo       *string
	URL           string
	Body          string
	Signature     string // enqueueNotification 写的 JSON：{timestamp, nonce, sign}
	RetryCount    int64
	Status        int
	CreatedAt     time.Time
	LastAttemptAt *time.Time
	LastError     *string
}
