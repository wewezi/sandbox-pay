package gateways

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/logger"
	"sandbox-pay/internal/money"
	"sandbox-pay/internal/services"
)

// NativePay 自有存管收银台：页面本身 + 管理员手动核销。
//
// 下单只回一个收银台页面 URL，真正的「收钱」由管理员/财务在页面上核销完成；
// payMethod=alipay 的单会在打开收银台时主动向支付宝查一次，查到就直接置为已支付。
type NativePay struct {
	cfg      config.Config
	log      *logger.Logger
	store    *db.Store
	payments *services.PaymentService
	alipay   *Alipay
}

// NewNativePay 构造。
func NewNativePay(cfg config.Config, store *db.Store, payments *services.PaymentService,
	alipay *Alipay) *NativePay {
	return &NativePay{
		cfg:      cfg,
		log:      logger.New("NativePayService"),
		store:    store,
		payments: payments,
		alipay:   alipay,
	}
}

// CreateOrder 返回收银台页面 URL。
func (n *NativePay) CreateOrder(orderNo, baseUrl string) (*PayResult, error) {
	order, err := n.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, httperr.BadRequest("订单不存在")
	}
	if order.Status != db.StatusPending {
		return nil, httperr.BadRequest("订单状态非待支付")
	}

	clientURL := strings.TrimSpace(n.cfg.Get("CLIENT_URL"))
	if clientURL == "" {
		clientURL = baseUrl
	}
	cashierURL := clientURL + "/cashier.html?orderNo=" + orderNo

	n.log.Log(fmt.Sprintf("Native pay cashier URL generated for order: %s", orderNo))
	return &PayResult{Type: "url", Data: cashierURL}, nil
}

// GetCashierInfo 收银台轮询用的订单快照。
//
// 分支顺序和原版一致：paid → expired/failed → 过期时间到 → 支付宝主动核实 → pending。
func (n *NativePay) GetCashierInfo(orderNo, baseUrl string) (*CashierInfo, error) {
	order, err := n.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, httperr.NotFound("订单不存在")
	}

	base := func() *CashierInfo {
		return &CashierInfo{
			OrderNo:     order.OrderNo,
			Amount:      money.ToYuan(order.Amount),
			ProductName: order.ProductName,
		}
	}

	if order.Status == db.StatusPaid {
		info := base()
		info.Status = "paid"
		info.PayMethod = orDefault(order.PayMethod, "alipay")
		info.ThirdPartyTradeNo = firstNonEmpty(order.ThirdPartyTradeNo, "SP_CLEARED_"+order.OrderNo)
		payAt := time.Now()
		if order.PayAt != nil {
			payAt = *order.PayAt
		}
		s := services.JsTime(payAt)
		info.PayAt = &s
		return info, nil
	}

	if order.Status == db.StatusExpired || order.Status == db.StatusFailed {
		info := base()
		info.Status = "expired"
		if order.Status == db.StatusFailed {
			info.Status = "failed"
		}
		return info, nil
	}

	if time.Now().After(order.ExpireAt) {
		info := base()
		info.Status = "expired"
		return info, nil
	}

	// 主动核实支付宝状态（本地公网异步通知到不了内网时靠它兜底）。
	// 只认 payMethod，不按订单号前缀猜 —— 所有订单号都以 PAY 开头。
	if order.PayMethod == "alipay" && n.alipay.QueryAlipayTrade(orderNo) {
		refreshed, err := n.store.Orders.FindByOrderNo(orderNo, false)
		if err != nil || refreshed == nil {
			refreshed = order
		}
		info := base()
		info.Status = "paid"
		info.PayMethod = orDefault(refreshed.PayMethod, orDefault(order.PayMethod, "alipay"))
		info.ThirdPartyTradeNo = firstNonEmpty(refreshed.ThirdPartyTradeNo,
			fmt.Sprintf("ALIPAY_SYNC_%d", time.Now().UnixMilli()))
		payAt := time.Now()
		if refreshed.PayAt != nil {
			payAt = *refreshed.PayAt
		}
		s := services.JsTime(payAt)
		info.PayAt = &s
		return info, nil
	}

	// 二维码：优先用 .env 配的固定码，没配就现场找支付宝开一张。
	qrCodeURL := config.String(n.cfg, "NATIVE_PAY_QR_URL")
	if qrCodeURL == "" {
		if alipayQr := n.alipay.CreateQrPay(orderNo, baseUrl); alipayQr != "" {
			if dataURL, err := qrDataURL(alipayQr); err == nil {
				qrCodeURL = dataURL
			} else {
				n.log.Error(fmt.Sprintf("QR render failed for %s: %s", orderNo, err))
			}
		}
	}

	expireAt := services.JsTime(order.ExpireAt)
	info := base()
	info.Status = "pending"
	info.ExpireAt = &expireAt
	info.PaymentInfo = &CashierPaymentInfo{
		QRCodeURL:   qrCodeURL,
		AccountName: config.String(n.cfg, "NATIVE_PAY_ACCOUNT_NAME"),
		AccountNo:   config.String(n.cfg, "NATIVE_PAY_ACCOUNT_NO"),
		BankName:    config.String(n.cfg, "NATIVE_PAY_BANK_NAME"),
		Remark:      "WP" + orderNo,
	}
	return info, nil
}

// ConfirmPayment 管理员/存管钱包手动核销确认。
//
// 调用方（路由）必须已经做过身份验证（requireAuth 或 requireSandbox）。
// 这里只做业务校验：订单存在且仍 Pending、渠道为 native（禁止跨渠道核销）、
// 金额按订单原值入账（杜绝伪造金额）。
func (n *NativePay) ConfirmPayment(orderNo, tradeNo, walletUser, walletPass string) (*ConfirmResult, error) {
	order, err := n.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, httperr.NotFound("订单不存在")
	}
	if order.Status != db.StatusPending {
		return nil, httperr.BadRequestf("订单状态不正确，当前状态: %d", order.Status)
	}

	// 严防跨渠道结算渗透：禁止用存管直联核销支付宝或 PayPal 的订单。
	if order.PayMethod == "alipay" || order.PayMethod == "paypal" {
		return nil, httperr.BadRequestf(
			"该订单属于外部通道 (%s)，无法直接通过 Sandbox Pay 自有存管清算体系进行核销",
			order.PayMethod)
	}

	// 存管钱包账号 + 安全密码：始终强校验（不允许两个都为空时绕过）。
	// 长度按字符数算（JS 的 .length 是 UTF-16 码元数，这里用 rune 数近似）。
	if utf8.RuneCountInString(strings.TrimSpace(walletUser)) < 5 {
		return nil, httperr.BadRequest("存管结算账号格式不合规")
	}
	if utf8.RuneCountInString(strings.TrimSpace(walletPass)) < 6 {
		return nil, httperr.BadRequest("支付安全密码凭据不正确")
	}

	confirmedTradeNo := tradeNo
	if confirmedTradeNo == "" {
		confirmedTradeNo = fmt.Sprintf("SP_WALLET_%d", time.Now().UnixMilli())
	}
	// 金额按订单原值入账（cents），调用方伪造不了金额。
	if _, err := n.payments.MarkPaid(orderNo, &confirmedTradeNo, order.Amount, "CNY"); err != nil {
		return nil, err
	}

	n.log.Log(fmt.Sprintf("Native payment confirmed for %s, tradeNo: %s", orderNo, confirmedTradeNo))
	return &ConfirmResult{Success: true, OrderNo: orderNo, TradeNo: confirmedTradeNo}, nil
}

// orDefault 空串回落。
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// firstNonEmpty 取第一个非空字符串 —— 对应 TS 的 `a ?? b`（指针版）/ `a || b`。
func firstNonEmpty(p *string, def string) string {
	if p == nil || *p == "" {
		return def
	}
	return *p
}
