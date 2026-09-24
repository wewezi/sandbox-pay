package services

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/logger"
	"sandbox-pay/internal/money"
)

// PageResult 通用分页信封。items 的元素类型由调用点决定。
type PageResult[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int64 `json:"page"`
	PageSize int64 `json:"pageSize"`
}

// StatsResult 后台统计。
//
// 只回数，不回展示字符串（`￥1.23`、`33.3%`、`1 笔` 那些）——
// 金额符号、千分位、百分号是 locale 的事，交给前端 Intl。
// SuccessRate 是 0..1 的比例，没有订单时为 null 而不是 0，
// 空账本上「0% 成功率」和「暂无数据」不是一个意思。
type StatsResult struct {
	Amount       float64  `json:"amount"`
	SuccessCount int64    `json:"successCount"`
	TotalCount   int64    `json:"totalCount"`
	SuccessRate  *float64 `json:"successRate"`
}

// TransactionOpts 订单列表的过滤 + 分页。分页参数保持 query 原始字符串，
// 由 service 自己解析 —— 和原版 Number(page) 的口径对齐。
type TransactionOpts struct {
	GroupID   *int64
	Page      string
	PageSize  string
	Status    string
	PayMethod string
	DateFrom  string
	DateTo    string
	Keyword   string
}

// TransactionItem 订单列表的一项。金额已经是元。
type TransactionItem struct {
	ID                string  `json:"id"`
	OrderNo           string  `json:"orderNo"`
	Channel           string  `json:"channel"`
	Amount            float64 `json:"amount"`
	RefundedAmount    float64 `json:"refundedAmount"`
	Fee               float64 `json:"fee"`
	SettleAmount      float64 `json:"settleAmount"`
	PayMethod         string  `json:"payMethod"`
	ProductName       string  `json:"productName"`
	ExternalOrderNo   *string `json:"externalOrderNo,omitempty"`
	ThirdPartyTradeNo *string `json:"thirdPartyTradeNo,omitempty"`
	Status            string  `json:"status"`
	// 只回时间戳。相对时间（「17分钟前」）由前端的 <sp-time> 算 ——
	// 在这儿拼字符串等于把响应那一刻钉死，页面挂着不动它永远不变，
	// 而且用的是服务器时钟不是浏览器时区。
	CreatedAt string  `json:"createdAt"`
	PayAt     *string `json:"payAt,omitempty"`
}

// NotificationItem 通知列表的一项。status 仍是 NotifyStatus 数字。
type NotificationItem struct {
	ID            int64   `json:"id"`
	OrderNo       *string `json:"orderNo"`
	URL           string  `json:"url"`
	Status        int     `json:"status"` // 0=Pending, 1=Success, 2=Failed, 3=Exhausted
	RetryCount    int64   `json:"retryCount"`
	LastError     *string `json:"lastError"`
	LastAttemptAt *string `json:"lastAttemptAt"`
	CreatedAt     string  `json:"createdAt"`
}

// SuccessResult 删单 / 重投的回执。
type SuccessResult struct {
	Success bool `json:"success"`
}

// ReplayResult 重投通知的回执。
type ReplayResult struct {
	Success bool    `json:"success"`
	ID      int64   `json:"id"`
	OrderNo *string `json:"orderNo,omitempty"`
}

// ResetResult 清库回执，告诉调用方各删了多少。
type ResetResult struct {
	Deleted struct {
		Orders        int64 `json:"orders"`
		Notifications int64 `json:"notifications"`
	} `json:"deleted"`
}

// AdminService 后台的统计、列表、删单、通知重投、清库。
type AdminService struct {
	log   *logger.Logger
	store *db.Store
}

// NewAdminService 构造。
func NewAdminService(store *db.Store) *AdminService {
	return &AdminService{log: logger.New("AdminService"), store: store}
}

// GetStats 统计走 SQL 聚合而不是把行读进内存，
// 所以不管订单表多大，应用侧都是 O(1)。
func (s *AdminService) GetStats(groupID *int64) (*StatsResult, error) {
	st, err := s.store.Orders.Stats(groupID)
	if err != nil {
		return nil, err
	}
	out := &StatsResult{
		Amount:       money.ToYuan(st.TotalAmountCents),
		SuccessCount: st.SuccessCount,
		TotalCount:   st.TotalCount,
	}
	if st.TotalCount > 0 {
		rate := float64(st.SuccessCount) / float64(st.TotalCount)
		out.SuccessRate = &rate
	}
	return out, nil
}

// GetTransactions 订单列表：分页 + 过滤，状态转展示名。
func (s *AdminService) GetTransactions(opts TransactionOpts) (*PageResult[TransactionItem], error) {
	page, pageSize, offset, limit := paginate(opts.Page, opts.PageSize)

	orders, total, err := s.store.Orders.Search(db.SearchOpts{
		GroupID:   opts.GroupID,
		Status:    parseStatus(opts.Status),
		PayMethod: opts.PayMethod,
		DateFrom:  opts.DateFrom,
		DateTo:    opts.DateTo,
		Keyword:   opts.Keyword,
		Offset:    offset,
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}

	items := make([]TransactionItem, 0, len(orders))
	for _, order := range orders {
		items = append(items, TransactionItem{
			ID:                order.ID,
			OrderNo:           order.OrderNo,
			Channel:           channelOf(order.PayMethod),
			Amount:            money.ToYuan(order.Amount),
			RefundedAmount:    money.ToYuan(order.RefundedAmount),
			Fee:               money.ToYuan(order.Fee),
			SettleAmount:      money.ToYuan(order.SettleAmount),
			PayMethod:         order.PayMethod,
			ProductName:       order.ProductName,
			ExternalOrderNo:   order.ExternalOrderNo,
			ThirdPartyTradeNo: order.ThirdPartyTradeNo,
			Status:            statusOf(order.Status),
			CreatedAt:         JsTime(order.CreatedAt),
			PayAt:             jsTimePtr(order.PayAt),
		})
	}
	return &PageResult[TransactionItem]{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

// DeleteTransaction 删单。
func (s *AdminService) DeleteTransaction(orderNo, actor string, ip *string) (*SuccessResult, error) {
	order, err := s.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, httperr.NotFound("订单不存在")
	}
	if err := s.store.Orders.DeleteByOrderNo(orderNo); err != nil {
		return nil, err
	}
	s.log.Log(fmt.Sprintf("Order %s deleted by %s%s (amount=%d channel=%s status=%d)",
		orderNo, actorName(actor), fromIP(ip), order.Amount, order.PayMethod, order.Status))
	return &SuccessResult{Success: true}, nil
}

// GetNotificationStatus 通知列表：分页 + 状态过滤，status 保持数字。
func (s *AdminService) GetNotificationStatus(page, pageSize, status string) (*PageResult[NotificationItem], error) {
	p, size, offset, limit := paginate(page, pageSize)

	items, total, err := s.store.Notifies.Search(db.NotifySearchOpts{
		Status: parseStatus(status),
		Offset: offset,
		Limit:  limit,
	})
	if err != nil {
		return nil, err
	}

	out := make([]NotificationItem, 0, len(items))
	for _, n := range items {
		out = append(out, NotificationItem{
			ID:            n.ID,
			OrderNo:       n.OrderNo,
			URL:           n.URL,
			Status:        n.Status,
			RetryCount:    n.RetryCount,
			LastError:     n.LastError,
			LastAttemptAt: jsTimePtr(n.LastAttemptAt),
			CreatedAt:     JsTime(n.CreatedAt),
		})
	}
	return &PageResult[NotificationItem]{Items: out, Total: total, Page: p, PageSize: size}, nil
}

// ReplayNotification 把一条通知重新入队：状态回 Pending、计数清零、lastError 置 NULL。
//
// 显式写 nil = 把 lastError 置成 NULL（patch 里出现的 key 就会写），
// 否则旧错误会一直挂在刚重投的记录上误导人。
func (s *AdminService) ReplayNotification(id int64, actor string, ip *string) (*ReplayResult, error) {
	notify, err := s.store.Notifies.FindByID(id)
	if err != nil {
		return nil, err
	}
	if notify == nil {
		return nil, httperr.NotFound("通知记录不存在")
	}
	if err := s.store.Notifies.Update(id, map[string]any{
		"status":        db.NotifyPending,
		"retryCount":    int64(0),
		"lastAttemptAt": time.Now(),
		"lastError":     nil,
	}); err != nil {
		return nil, err
	}
	s.log.Log(fmt.Sprintf("Notification %d (order %s) requeued by %s%s",
		id, tsOpt(notify.OrderNo), actorName(actor), fromIP(ip)))
	return &ReplayResult{Success: true, ID: id, OrderNo: notify.OrderNo}, nil
}

// ResetData 清空订单和通知（沙箱用）。
func (s *AdminService) ResetData(actor string, ip *string) (*ResetResult, error) {
	orderCount, err := s.store.Orders.Count()
	if err != nil {
		return nil, err
	}
	notifyCount, err := s.store.Notifies.Count()
	if err != nil {
		return nil, err
	}

	// 先删通知（有 orderNo 引用），再删订单。
	if err := s.store.Notifies.Clear(); err != nil {
		return nil, err
	}
	if err := s.store.Orders.Clear(); err != nil {
		return nil, err
	}

	s.log.Warn(fmt.Sprintf("Data reset by %s%s: %d orders, %d notifications deleted",
		actorName(actor), fromIP(ip), orderCount, notifyCount))

	out := &ResetResult{}
	out.Deleted.Orders = orderCount
	out.Deleted.Notifications = notifyCount
	return out, nil
}

// paginate 分页参数收敛：页码至少 1，每页 1..200。
//
// 刻意复刻 JS 的 `Math.max(1, Number(page) || 1)`：
// 空串/非法值不算错误，回落默认值就行。
func paginate(page, pageSize string) (p, size, offset, limit int64) {
	pv := int64(math.Max(1, jsNumberOr(page, 1)))
	sv := int64(math.Min(200, math.Max(1, jsNumberOr(pageSize, 50))))
	return pv, sv, (pv - 1) * sv, sv
}

// jsNumberOr `Number(x) || def`：空串、0、非法值统统回落。
func jsNumberOr(raw string, def float64) float64 {
	trimmed := strings.TrimSpace(raw)
	var n float64
	if trimmed == "" {
		n = 0
	} else {
		parsed, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return def // NaN 分支
		}
		n = parsed
	}
	if n == 0 {
		return def
	}
	return n
}

// parseStatus query 里的状态数字 → 过滤条件。空值不筛，非法值也不筛
// （前端的 select 只会发 ""/"0"/"1"...，非法值说明是手工拼的 URL，别报错）。
func parseStatus(raw string) *int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	n, err := strconv.Atoi(trimmed)
	if err != nil {
		return nil
	}
	return &n
}

// channelOf payMethod → 展示用渠道名。
func channelOf(payMethod string) string {
	if payMethod == "" {
		return "ALIPAY"
	}
	upper := strings.ToUpper(payMethod)
	for _, known := range []string{"PAYPAL", "WECHAT", "USDT", "NATIVE"} {
		if strings.Contains(upper, known) {
			return known
		}
	}
	return upper
}

// statusOf 订单状态 → 展示用状态名。
func statusOf(status int) string {
	switch status {
	case db.StatusPaid:
		return "SUCCESS"
	case db.StatusFailed, db.StatusExpired:
		return "FAILED"
	case db.StatusRefunding:
		return "REFUNDING"
	case db.StatusRefunded:
		return "REFUNDED"
	default:
		return "PENDING"
	}
}

// actorName 操作者没传时回落 "admin"（和原版默认参数一致）。
func actorName(actor string) string {
	if actor == "" {
		return "admin"
	}
	return actor
}

// fromIP 有 IP 才拼 " from <ip>"，没有就是空串。
func fromIP(ip *string) string {
	if ip == nil || *ip == "" {
		return ""
	}
	return " from " + *ip
}
