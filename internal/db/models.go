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
	ID             int64
	Username       string
	Password       string
	Role           string
	IsActive       bool
	SessionVersion int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Group 分组。停用后它名下所有 Key 都调不通（守卫连表查 isActive）。
type Group struct {
	ID        int64
	Name      string
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
	// WebhookSecret signs callbacks independently of API authentication tokens.
	WebhookSecret string `json:"-"`
}

// APIKey 一把 API Key。一个分组可以有多把，用来轮换或区分环境。
//
// The database stores a one-way hash. Key is populated only by Insert's
// issuance receipt and is empty on subsequent reads.
type APIKey struct {
	ID      int64
	GroupID int64
	// Name 备注名，纯给人看的（"本地开发"、"CI"），不参与鉴权。
	Name      string
	Key       string `json:"-"`
	KeyHash   string `json:"-"`
	KeyPrefix string
	KeySuffix string
	IsActive  bool
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
	// ProviderOrderID 绑定 PayPal 上游订单，回调 token 必须与它一致。
	ProviderOrderID *string
	// RefundedAmount 整数分。
	RefundedAmount int64
	RefundTradeNo  *string
	RefundAt       *time.Time
	// ChannelCost 上游渠道（支付宝/PayPal）收的成本，整数分。
	ChannelCost int64
	// Fee Mintweft 向接入方收的手续费，整数分。
	Fee int64
	// SettleAmount 扣掉 fee 后应结给接入方的金额，markPaid 时算出来，整数分。
	SettleAmount int64
	ReturnURL    *string
	NotifyURL    *string
	GroupID      *int64
	// ClientIP and ClientLocation capture the request source at order creation.
	// ClientLocation is the JSON snapshot from the local IP database.
	ClientIP       *string
	ClientLocation *string
	// SigningSecret is a private snapshot, never exposed in order DTOs.
	// Legacy orders retain their API token for version 1 signatures; new orders
	// snapshot the project's independent webhook secret for version 2.
	SigningSecret    *string `json:"-"`
	SignatureVersion int
	// APIKeyID records the authentication token used to create the order.
	APIKeyID *int64
	UserID   int64
	// APIKey is optional joined authentication metadata, without plaintext.
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
	AvailableAt   *time.Time
}

const (
	RefundProcessing = "processing"
	RefundPending    = "pending"
	RefundUnknown    = "unknown"
	RefundCompleted  = "completed"
	RefundFailed     = "failed"
)

// RefundOperation 保留每次退款的请求标识、在途状态和上游单号。
type RefundOperation struct {
	ID        string
	RequestID string
	Reason    string
	OrderNo   string
	Amount    int64
	TradeNo   *string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// UsageLog records safe API metadata. Credential and project names are
// snapshots so later renames or deletions cannot rewrite historical evidence.
// No authentication credentials, headers or request/response bodies are stored.
type UsageLog struct {
	ID             int64
	RequestID      string
	Method         string
	Path           string
	StatusCode     int
	DurationMs     float64
	GroupID        *int64
	GroupName      string
	APIKeyID       *int64
	APIKeyName     string
	KeyPrefix      string
	KeySuffix      string
	ClientIP       string
	ClientLocation string
	OrderNo        string
	ErrorMessage   string
	CreatedAt      time.Time
}
