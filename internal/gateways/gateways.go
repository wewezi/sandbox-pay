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
)

// PayResult 下单接口的回执：告诉前端往哪儿跳。
//
// type 只有 'url' | 'form' 两种字面量，前端 cashier.js / sandbox.js 按它分支。
// data 一律是链接 —— 支付宝给的是签名好的 GET URL（原版是一段自提交表单，
// 前端得先剥掉里面的 <script> 再手动 submit，因为 CSP 不让跑内联脚本），
// cashier.js 见到 data 以 http 开头就直接 location.href。
type PayResult struct {
	Type string `json:"type"`
	Data string `json:"data"`
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
	QRCodeURL   string `json:"qrCodeUrl"`
	AccountName string `json:"accountName"`
	AccountNo   string `json:"accountNo"`
	BankName    string `json:"bankName"`
	Remark      string `json:"remark"`
}

// CashierInfo 收银台要展示的订单快照，按 status 分三种形态：
//
//	paid    —— 多出 payMethod / thirdPartyTradeNo / payAt
//	expired —— 只有四个基础字段
//	pending —— 多出 expireAt / paymentInfo
//
// 靠 omitempty 丢掉当前形态用不到的字段，对齐原版三个分支返回的三个不同
// 对象字面量 —— 收银台页按 status 取字段，多出来的空字段会渲染成 "undefined"。
type CashierInfo struct {
	OrderNo           string              `json:"orderNo"`
	Amount            float64             `json:"amount"`
	ProductName       string              `json:"productName"`
	Status            string              `json:"status"` // pending | paid | expired | failed
	PayMethod         string              `json:"payMethod,omitempty"`
	ThirdPartyTradeNo string              `json:"thirdPartyTradeNo,omitempty"`
	PayAt             *string             `json:"payAt,omitempty"`
	ExpireAt          *string             `json:"expireAt,omitempty"`
	PaymentInfo       *CashierPaymentInfo `json:"paymentInfo,omitempty"`
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
