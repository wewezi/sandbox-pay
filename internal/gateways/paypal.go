package gateways

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/plutov/paypal"

	"sandbox-pay/internal/apiurl"
	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/logger"
	"sandbox-pay/internal/money"
	"sandbox-pay/internal/services"
)

// PayPal PayPal 渠道。客户端同样是惰性创建 + 缓存。
type PayPal struct {
	cfg      config.Config
	log      *logger.Logger
	store    *db.Store
	rates    *services.ExchangeRateService
	payments *services.PaymentService

	mu  sync.Mutex
	cli *paypal.Client
}

// NewPayPal 构造。
func NewPayPal(cfg config.Config, store *db.Store, rates *services.ExchangeRateService,
	payments *services.PaymentService) *PayPal {
	return &PayPal{
		cfg:      cfg,
		log:      logger.New("PayPalService"),
		store:    store,
		rates:    rates,
		payments: payments,
	}
}

// client 取（必要时创建）SDK 客户端。没配凭证报 400，理由同 Alipay.client。
func (p *PayPal) client() (*paypal.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cli != nil {
		return p.cli, nil
	}

	clientID := strings.TrimSpace(p.cfg.Get("PAYPAL_CLIENT_ID"))
	secret := strings.TrimSpace(p.cfg.Get("PAYPAL_CLIENT_SECRET"))
	if clientID == "" || secret == "" {
		return nil, httperr.BadRequest(
			"PayPal 渠道未配置，请在 .env 设置 PAYPAL_CLIENT_ID / PAYPAL_CLIENT_SECRET")
	}

	environment := config.String(p.cfg, "PAYPAL_ENVIRONMENT")
	base := paypal.APIBaseSandBox
	if environment == "live" {
		base = paypal.APIBaseLive
	}
	cli, err := paypal.NewClient(clientID, secret, base)
	if err != nil {
		return nil, err
	}

	p.log.Log(fmt.Sprintf("PayPal initialized in %s mode", environment))
	p.cli = cli
	return cli, nil
}

// CreateOrder 下单，返回 PayPal 的 approve 链接。
//
// 美元金额由人民币分按实时汇率折算，并把 (汇率, 美分, USD) 回写订单 ——
// 退款时要按同一个汇率把部分退款折回美分。
func (p *PayPal) CreateOrder(orderNo, baseUrl string) (*PayResult, error) {
	// 先取客户端：没配凭证要以 400 原样抛出，别被下面的失败分支盖成
	// 「PayPal 下单失败」，而且没配就根本不用碰数据库和汇率接口。
	cli, err := p.client()
	if err != nil {
		return nil, err
	}

	order, err := p.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, httperr.BadRequest("订单不存在")
	}

	rate := p.rates.GetCnyToUsdRate()
	// order.Amount 是人民币分，按汇率换成美分。兜到 1 分：￥0.01 换算后是
	// 0.0014 美元，round 归零，PayPal 会以 CANNOT_BE_ZERO_OR_NEGATIVE 拒单。
	usdCents := int64(math.Max(1, math.Round(money.ToYuan(order.Amount)*rate*100)))

	if err := p.store.Orders.Update(orderNo, map[string]any{
		"exchangeRate":    rate,
		"foreignAmount":   usdCents,
		"foreignCurrency": "USD",
	}); err != nil {
		return nil, err
	}

	callback := apiurl.URL(baseUrl, "paypal/callback")
	orderResp, err := cli.CreateOrder("CAPTURE",
		[]paypal.PurchaseUnitRequest{{
			Amount: &paypal.PurchaseUnitAmount{
				Currency: "USD",
				Value:    money.ToYuanString(usdCents),
			},
			Description: order.ProductName,
			ReferenceID: order.OrderNo,
		}},
		nil,
		&paypal.ApplicationContext{
			ReturnURL: callback + "?orderNo=" + order.OrderNo,
			CancelURL: callback + "?orderNo=" + order.OrderNo + "&cancel=1",
		},
	)
	if err == nil {
		for _, link := range orderResp.Links {
			if link.Rel == "approve" && link.Href != "" {
				return &PayResult{Type: "url", Data: link.Href}, nil
			}
		}
		err = fmt.Errorf("PayPal 响应缺少确认链接")
	}

	p.log.Error(fmt.Sprintf("PayPal order creation failed: %s", err))
	return nil, httperr.BadRequest("PayPal 下单失败")
}

// CaptureOrder 用户在 PayPal 付完回来后 capture 并入账。
//
// 业务失败一律落成 {success:false, ...}（同时把订单标失败），
// 只有落库本身出错才返回 error。
func (p *PayPal) CaptureOrder(orderNo, token string) (*CaptureResult, error) {
	cli, err := p.client()
	if err != nil {
		return p.captureFailed(orderNo, err.Error())
	}

	resp, err := cli.CaptureOrder(token, paypal.CaptureOrderRequest{})
	if err != nil {
		return p.captureFailed(orderNo, err.Error())
	}
	if resp == nil || resp.Status != "COMPLETED" {
		if _, err := p.payments.MarkFailed(orderNo, strp("PayPal capture not completed")); err != nil {
			return nil, err
		}
		return &CaptureResult{}, nil
	}

	// capture 的金额从 purchase_units[0].payments.captures[0] 取 ——
	// 这是 PayPal 实际扣到的钱，拿它和订单金额比对，挡住金额不符的回调。
	var captureID, rawValue, rawCurrency string
	if len(resp.PurchaseUnits) > 0 && resp.PurchaseUnits[0].Payments != nil &&
		len(resp.PurchaseUnits[0].Payments.Captures) > 0 {
		capture := resp.PurchaseUnits[0].Payments.Captures[0]
		captureID = capture.ID
		if capture.Amount != nil {
			rawValue = capture.Amount.Value
			rawCurrency = capture.Amount.Currency
		}
	}
	// PayPal 的 value 是 "10.00" 这种十进制字符串，边界上转成最小单位整数。
	capturedCents := money.YuanStringToCents(rawValue)
	capturedCurrency := strings.ToUpper(rawCurrency)
	if capturedCurrency == "" {
		capturedCurrency = "USD"
	}

	if captureID == "" || capturedCents <= 0 {
		p.log.Error(fmt.Sprintf("PayPal capture missing id/amount for %s", orderNo))
		if _, err := p.payments.MarkFailed(orderNo, strp("PayPal capture missing fields")); err != nil {
			return nil, err
		}
		return &CaptureResult{}, nil
	}

	if _, err := p.payments.MarkPaid(orderNo, &captureID, capturedCents, capturedCurrency); err != nil {
		// 金额不匹配 —— 不标失败（钱是真收到了），标记为需要人工对账。
		p.log.Error(fmt.Sprintf("PayPal capture amount validation rejected for %s: %s. Capture %s requires manual reconciliation.",
			orderNo, err, captureID))
		return &CaptureResult{CaptureID: &captureID, Error: strp("amount_mismatch")}, nil
	}

	p.log.Log(fmt.Sprintf("PayPal order %s captured: %s (%d %s cents)",
		orderNo, captureID, capturedCents, capturedCurrency))
	return &CaptureResult{Success: true, CaptureID: &captureID}, nil
}

// captureFailed 统一的失败分支：记日志 + 订单标失败 + 回 {success:false, error}。
func (p *PayPal) captureFailed(orderNo, message string) (*CaptureResult, error) {
	p.log.Error(fmt.Sprintf("PayPal capture failed for %s: %s", orderNo, message))
	if _, err := p.payments.MarkFailed(orderNo, &message); err != nil {
		return nil, err
	}
	return &CaptureResult{Error: &message}, nil
}

// Refund 退一笔 PayPal capture。captureID 就是订单上的 thirdPartyTradeNo，
// refundCents 是 currency 的最小单位整数值。返回上游退款单号，空串 = 失败。
// 实现 services.PayPalRefunder。
func (p *PayPal) Refund(captureID string, refundCents int64, currency string, reason string) string {
	if currency == "" {
		currency = "USD"
	}
	if reason == "" {
		reason = "Group initiated refund"
	}

	cli, err := p.client()
	if err != nil {
		p.log.Error(fmt.Sprintf("PayPal refund failed for capture %s: %s", captureID, err))
		return ""
	}

	req, err := cli.NewRequest("POST", cli.APIBase+"/v2/payments/captures/"+captureID+"/refund",
		map[string]any{
			"amount": map[string]any{
				"value":         money.ToYuanString(refundCents),
				"currency_code": currency,
			},
			"note_to_payer": reason,
		})
	if err != nil {
		p.log.Error(fmt.Sprintf("PayPal refund failed for capture %s: %s", captureID, err))
		return ""
	}

	var result map[string]any
	if err := cli.SendWithAuth(req, &result); err != nil {
		p.log.Error(fmt.Sprintf("PayPal refund failed for capture %s: %s", captureID, err))
		return ""
	}

	id, _ := result["id"].(string)
	status, _ := result["status"].(string)
	if id != "" && (status == "COMPLETED" || status == "PENDING") {
		p.log.Log(fmt.Sprintf("PayPal refund initiated for capture %s: %s (%s)", captureID, id, status))
		return id
	}
	if raw, err := json.Marshal(result); err == nil {
		p.log.Warn(fmt.Sprintf("PayPal refund unexpected response: %s", raw))
	}
	return ""
}
