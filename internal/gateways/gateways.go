// Package gateways 第三方支付渠道适配层：支付宝、PayPal、自有存管收银台。
//
// 三个网关都是「第一次真正调上游时才建客户端」—— 没配凭证不影响启动，
// 只在真正请求的那一刻报 400，并把缺的 .env 项列出来。
//
// 依赖方向：gateways → services（services 不 import gateways，无环）。
// 退款是反过来的：RefundService 通过 services.Refunder /
// services.PayPalRefunder 两个接口拿到网关，不直接引用这里。
package gateways

import (
	"encoding/base64"

	"github.com/skip2/go-qrcode"

	"sandbox-pay/internal/services"
)

// PayResult 下单回执，data 为支付链接。
// type 保留 "url" / "form" 两种值以兼容 API，二者的 data 均是 URL。
// web/sandbox.ts 打开收银台；web/cashier.ts 校验 HTTP(S) 协议后跳转上游。
// 支付宝返回签名好的 GET URL，不返回内联脚本或自提交表单。
type PayResult struct {
	Type       string  `json:"type"`
	Data       string  `json:"data"`
	OrderNo    string  `json:"orderNo,omitempty"`
	PaymentURL string  `json:"paymentUrl,omitempty"`
	Status     int     `json:"status"`
	Amount     float64 `json:"amount,omitempty"`
	ExpireAt   string  `json:"expireAt,omitempty"`
}

// CaptureResult PayPal capture 的回执。业务失败一律 {success:false, ...}，
// 只有落库本身出错才以 error 返回。
type CaptureResult struct {
	Success   bool    `json:"success"`
	CaptureID *string `json:"captureId,omitempty"`
	Error     *string `json:"error,omitempty"`
}

// CashierPaymentInfo 自有渠道收银台展示的打款信息。
type CashierPaymentInfo struct {
	QRCodeURL     string `json:"qrCodeUrl"`
	QRCodeChannel string `json:"qrCodeChannel,omitempty"`
	AccountName   string `json:"accountName"`
	AccountNo     string `json:"accountNo"`
	BankName      string `json:"bankName"`
	Remark        string `json:"remark"`
}

// CashierInfo 收银台要展示的订单快照，每个状态都给出明确的返回地址：
//
//	paid    —— 多出 payMethod / thirdPartyTradeNo / payAt
//	expired / failed —— 订单基础字段和返回地址
//	pending —— 多出 expireAt / paymentInfo / availableChannels
//
// availableChannels lists selectable methods for a pending merchant order.
// The selected method does not prevent choosing another one before payment.
type CashierInfo struct {
	OrderNo           string                `json:"orderNo"`
	Amount            float64               `json:"amount"`
	ProductName       string                `json:"productName"`
	ProjectName       string                `json:"projectName,omitempty"`
	ExternalOrderNo   *string               `json:"externalOrderNo,omitempty"`
	CreatedAt         string                `json:"createdAt,omitempty"`
	ReturnURL         string                `json:"returnUrl"`
	Status            string                `json:"status"` // pending | paid | expired | failed
	PayMethod         string                `json:"payMethod,omitempty"`
	AvailableChannels []string              `json:"availableChannels,omitempty"`
	ChannelLockReason string                `json:"channelLockReason,omitempty"`
	PayPalQuote       *services.PayPalQuote `json:"paypalQuote,omitempty"`
	ForeignAmount     *float64              `json:"foreignAmount,omitempty"`
	ForeignCurrency   string                `json:"foreignCurrency,omitempty"`
	ExchangeRate      *float64              `json:"exchangeRate,omitempty"`
	ThirdPartyTradeNo string                `json:"thirdPartyTradeNo,omitempty"`
	PayAt             *string               `json:"payAt,omitempty"`
	ExpireAt          *string               `json:"expireAt,omitempty"`
	PaymentInfo       *CashierPaymentInfo   `json:"paymentInfo,omitempty"`
}

// ConfirmResult 手动核销的回执。
type ConfirmResult struct {
	Success bool   `json:"success"`
	OrderNo string `json:"orderNo"`
	TradeNo string `json:"tradeNo"`
}

// strp 取局部变量地址。
// Go 没有 TS 那种「函数参数要 *string、手上只有一个字面量」的顺手写法，
// markPaid / markFailed 的 thirdPartyTradeNo、reason 都是 *string，靠它装箱。
func strp(s string) *string { return &s }

// qrDataURL 把二维码内容编成 300×300 的 PNG data URL。
//
// 前端 <img src> 直接吃，CSP 的 img-src 已经放行了 data:。
func qrDataURL(content string) (string, error) {
	qr, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return "", err
	}
	png, err := qr.PNG(300)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}
