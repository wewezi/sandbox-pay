package gateways

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/plutov/paypal"

	"sandbox-pay/internal/apiurl"
	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/logger"
	"sandbox-pay/internal/money"
	"sandbox-pay/internal/services"
)

// PayPal is a sandbox-only adapter. OAuth refreshes are serialized, while
// requests use a copied token and never read the SDK's mutable token state.
type PayPal struct {
	cfg      config.Config
	log      *logger.Logger
	store    *db.Store
	rates    *services.ExchangeRateService
	payments *services.PaymentService

	mu        sync.Mutex
	cli       *paypal.Client
	token     string
	expiresAt time.Time
	orderMu   sync.Mutex
}

func NewPayPal(cfg config.Config, store *db.Store, rates *services.ExchangeRateService,
	payments *services.PaymentService) *PayPal {
	return &PayPal{
		cfg: cfg, log: logger.New("PayPalService"), store: store,
		rates: rates, payments: payments,
	}
}

func (p *PayPal) client() (*paypal.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if config.String(p.cfg, "PAYPAL_ENVIRONMENT") != "sandbox" {
		return nil, httperr.BadRequest("PayPal 仅支持 sandbox 环境")
	}
	if p.cli != nil {
		return p.cli, nil
	}
	clientID := strings.TrimSpace(p.cfg.Get("PAYPAL_CLIENT_ID"))
	secret := strings.TrimSpace(p.cfg.Get("PAYPAL_CLIENT_SECRET"))
	if clientID == "" || secret == "" {
		return nil, httperr.BadRequest(
			"PayPal 渠道未配置，请在 .env 设置 PAYPAL_CLIENT_ID / PAYPAL_CLIENT_SECRET")
	}
	cli, err := paypal.NewClient(clientID, secret, paypal.APIBaseSandBox)
	if err != nil {
		return nil, err
	}
	cli.SetHTTPClient(&http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	})
	p.cli = cli
	return cli, nil
}

func (p *PayPal) accessToken(cli *paypal.Client) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" && time.Now().Add(30*time.Second).Before(p.expiresAt) {
		return p.token, nil
	}
	token, err := cli.GetAccessToken()
	if err != nil {
		return "", err
	}
	if token == nil || token.Token == "" || token.ExpiresIn <= 0 ||
		!strings.EqualFold(token.Type, "Bearer") {
		return "", invalidGatewayResponse("oauth")
	}
	p.token = token.Token
	p.expiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	return p.token, nil
}

// PrepareCreateOrder checks the client and OAuth before the cashier changes
// its local channel. OAuth failures cannot have created a provider payment.
func (p *PayPal) PrepareCreateOrder(orderNos ...string) error {
	if len(orderNos) > 0 {
		quote, err := p.payments.EnsurePayPalQuote(orderNos[0], p.rates)
		if err != nil {
			return err
		}
		if !quote.Available {
			return httperr.BadRequest(quote.Reason)
		}
	}
	cli, err := p.client()
	if err != nil {
		return err
	}
	_, err = p.accessToken(cli)
	return err
}

// send retries an unauthorized request once with a refreshed token. Mutating
// requests retain the same PayPal-Request-Id across both attempts.
func (p *PayPal) send(method, path string, body any, requestID string, result any) error {
	cli, err := p.client()
	if err != nil {
		return rejectedChannelCreation(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		token, err := p.accessToken(cli)
		if err != nil {
			return rejectedChannelCreation(err)
		}
		req, err := cli.NewRequest(method, cli.APIBase+path, body)
		if err != nil {
			return rejectedChannelCreation(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Prefer", "return=representation")
		if requestID != "" {
			req.Header.Set("PayPal-Request-Id", requestID)
		}
		err = cli.Send(req, result)
		var responseError *paypal.ErrorResponse
		if attempt == 0 && errors.As(err, &responseError) && responseError.Response != nil &&
			responseError.Response.StatusCode == http.StatusUnauthorized {
			p.mu.Lock()
			if p.token == token {
				p.token = ""
				p.expiresAt = time.Time{}
			}
			p.mu.Unlock()
			continue
		}
		return err
	}
	return fmt.Errorf("PayPal authentication failed")
}

// The legacy SDK omits reference_id and capture status from its response
// structs. Keep those security-relevant fields in our decoding types.
type paypalOrderResponse struct {
	ID            string               `json:"id"`
	Status        string               `json:"status"`
	Intent        string               `json:"intent"`
	PurchaseUnits []paypalPurchaseUnit `json:"purchase_units"`
	Links         []paypal.Link        `json:"links"`
}

type paypalPurchaseUnit struct {
	ReferenceID string                     `json:"reference_id"`
	Amount      *paypal.PurchaseUnitAmount `json:"amount"`
	Payments    *struct {
		Captures []paypalCapture `json:"captures"`
	} `json:"payments"`
}

type paypalCapture struct {
	ID     string                     `json:"id"`
	Status string                     `json:"status"`
	Amount *paypal.PurchaseUnitAmount `json:"amount"`
}

func (p *PayPal) CreateOrder(orderNo, baseURL string) (*PayResult, error) {
	// Repeated calls reuse the persisted provider order and exchange rate.
	p.orderMu.Lock()
	defer p.orderMu.Unlock()
	order, err := p.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, httperr.NotFound("订单不存在")
	}
	if order.PayMethod != "paypal" || order.Status != db.StatusPending || !order.ExpireAt.After(time.Now()) {
		return nil, httperr.Conflict("订单状态不允许 PayPal 下单")
	}
	quote, err := p.payments.EnsurePayPalQuote(orderNo, p.rates)
	if err != nil {
		return nil, rejectedChannelCreation(err)
	}
	if !quote.Available {
		return nil, rejectedChannelCreation(httperr.BadRequest(quote.Reason))
	}
	order, err = p.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil {
		return nil, rejectedChannelCreation(err)
	}
	if order == nil || order.PayMethod != "paypal" || order.Status != db.StatusPending || !order.ExpireAt.After(time.Now()) {
		return nil, httperr.Conflict("订单状态不允许 PayPal 下单")
	}
	if _, err := p.client(); err != nil {
		return nil, rejectedChannelCreation(err)
	}
	var response paypalOrderResponse
	if order.ProviderOrderID != nil && *order.ProviderOrderID != "" {
		if order.ForeignAmount == nil || order.ForeignCurrency == nil || *order.ForeignCurrency != "USD" {
			return nil, httperr.Conflict("PayPal 订单缺少已绑定的金额信息")
		}
		if err := p.send(http.MethodGet, "/v2/checkout/orders/"+url.PathEscape(*order.ProviderOrderID), nil, "", &response); err != nil {
			return nil, err
		}
		if response.ID != *order.ProviderOrderID || !validPayPalOrder(&response, orderNo, *order.ForeignAmount) {
			return nil, invalidGatewayResponse("order")
		}
	} else {
		// The displayed quote and retries use the exact same frozen cents/rate.
		rate, usdCents := *order.ExchangeRate, *order.ForeignAmount
		callback := apiurl.URL(baseURL, "paypal/callback") + "?orderNo=" + url.QueryEscape(orderNo)
		body := struct {
			Intent             string                       `json:"intent"`
			PurchaseUnits      []paypal.PurchaseUnitRequest `json:"purchase_units"`
			ApplicationContext *paypal.ApplicationContext   `json:"application_context"`
		}{
			Intent: "CAPTURE",
			PurchaseUnits: []paypal.PurchaseUnitRequest{{
				Amount:      &paypal.PurchaseUnitAmount{Currency: "USD", Value: money.ToYuanString(usdCents)},
				Description: order.ProductName, ReferenceID: orderNo,
			}},
			ApplicationContext: &paypal.ApplicationContext{ReturnURL: callback, CancelURL: callback + "&cancel=1"},
		}
		if err := p.send(http.MethodPost, "/v2/checkout/orders", body, orderNo, &response); err != nil {
			var responseError *paypal.ErrorResponse
			if errors.As(err, &responseError) && responseError.Response != nil {
				switch responseError.Response.StatusCode {
				case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden,
					http.StatusNotFound, http.StatusUnprocessableEntity:
					return nil, rejectedChannelCreation(err)
				}
			}
			return nil, err
		}
		if !validPayPalOrder(&response, orderNo, usdCents) {
			return nil, invalidGatewayResponse("order")
		}
		if err := p.store.Orders.BindProviderOrder(orderNo, response.ID, rate, usdCents, "USD"); err != nil {
			return nil, err
		}
	}
	for _, link := range response.Links {
		if link.Rel == "approve" && validPayPalApprovalURL(link.Href) {
			return &PayResult{Type: "url", Data: link.Href}, nil
		}
	}
	return nil, invalidGatewayResponse("approval")
}

func validPayPalOrder(order *paypalOrderResponse, orderNo string, cents int64) bool {
	if order.ID == "" || order.Intent != "CAPTURE" || len(order.PurchaseUnits) != 1 {
		return false
	}
	if order.Status != "CREATED" && order.Status != "APPROVED" && order.Status != "PAYER_ACTION_REQUIRED" {
		return false
	}
	unit := order.PurchaseUnits[0]
	return unit.ReferenceID == orderNo && validPayPalAmount(unit.Amount, cents)
}

func validPayPalApprovalURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.User == nil && u.Port() == "" &&
		(u.Hostname() == "www.sandbox.paypal.com" || u.Hostname() == "sandbox.paypal.com")
}

// CaptureOrder validates the browser token before contacting PayPal. Invalid
// callbacks never change the local payment state.
func (p *PayPal) CaptureOrder(orderNo, token string) (*CaptureResult, error) {
	unlock := p.payments.LockCashierChannel(orderNo)
	defer unlock()
	p.orderMu.Lock()
	defer p.orderMu.Unlock()
	order, err := p.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, httperr.NotFound("订单不存在")
	}
	if order.PayMethod != "paypal" || token == "" || order.ProviderOrderID == nil || token != *order.ProviderOrderID {
		return p.captureFailed(orderNo, "provider_order_mismatch")
	}
	if order.Status == db.StatusPaid && order.ThirdPartyTradeNo != nil {
		return &CaptureResult{Success: true, CaptureID: order.ThirdPartyTradeNo}, nil
	}
	if order.Status != db.StatusPending || !order.ExpireAt.After(time.Now()) {
		return p.captureFailed(orderNo, "order_not_pending")
	}
	if order.ForeignAmount == nil || order.ForeignCurrency == nil || *order.ForeignCurrency != "USD" {
		return p.captureFailed(orderNo, "missing_foreign_amount")
	}
	if quote := services.PayPalQuoteForOrder(order); !quote.Available {
		return p.captureFailed(orderNo, "invalid_paypal_quote")
	}
	var response paypalOrderResponse
	path := "/v2/checkout/orders/" + url.PathEscape(token)
	err = p.send(http.MethodPost, path+"/capture", paypal.CaptureOrderRequest{}, "capture-"+orderNo, &response)
	if err != nil && alreadyCaptured(err) {
		err = p.send(http.MethodGet, path, nil, "", &response)
	}
	if err != nil {
		return p.captureFailed(orderNo, "capture_request_failed")
	}
	if response.ID != token || response.Status != "COMPLETED" || len(response.PurchaseUnits) != 1 ||
		response.PurchaseUnits[0].ReferenceID != orderNo {
		return p.captureFailed(orderNo, "capture_order_mismatch")
	}
	payments := response.PurchaseUnits[0].Payments
	if payments == nil || len(payments.Captures) != 1 {
		return p.captureFailed(orderNo, "capture_missing_fields")
	}
	capture := payments.Captures[0]
	if capture.ID == "" || capture.Status != "COMPLETED" {
		return p.captureFailed(orderNo, "capture_not_completed")
	}
	if !validPayPalAmount(capture.Amount, *order.ForeignAmount) {
		return p.captureFailed(orderNo, "amount_mismatch")
	}
	paid, err := p.payments.MarkPaidForChannel("paypal", orderNo, &capture.ID, *order.ForeignAmount, "USD")
	if err != nil {
		return nil, err
	}
	if paid.Status != db.StatusPaid || paid.ThirdPartyTradeNo == nil || *paid.ThirdPartyTradeNo != capture.ID {
		return p.captureFailed(orderNo, "order_state_changed")
	}
	return &CaptureResult{Success: true, CaptureID: &capture.ID}, nil
}

func alreadyCaptured(err error) bool {
	var responseError *paypal.ErrorResponse
	if !errors.As(err, &responseError) {
		return false
	}
	for _, detail := range responseError.Details {
		if detail.Issue == "ORDER_ALREADY_CAPTURED" {
			return true
		}
	}
	return false
}

func (p *PayPal) captureFailed(orderNo, message string) (*CaptureResult, error) {
	p.log.Warn(fmt.Sprintf("PayPal capture rejected for %s: %s", orderNo, message))
	return &CaptureResult{Error: &message}, nil
}

func validPayPalAmount(amount *paypal.PurchaseUnitAmount, expected int64) bool {
	if amount == nil || amount.Currency != "USD" || expected <= 0 {
		return false
	}
	cents, ok := paypalAmountCents(amount.Value)
	return ok && cents == expected
}

// Parse decimal amounts without rounding malformed or fractional-cent values.
func paypalAmountCents(value string) (int64, bool) {
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, false
	}
	for _, part := range parts {
		if part == "" {
			return 0, false
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return 0, false
			}
		}
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole > (math.MaxInt64-99)/100 {
		return 0, false
	}
	var fraction int64
	if len(parts) == 2 {
		if len(parts[1]) > 2 {
			return 0, false
		}
		fraction, _ = strconv.ParseInt(parts[1], 10, 64)
		if len(parts[1]) == 1 {
			fraction *= 10
		}
	}
	return whole*100 + fraction, true
}

// Refund keeps the legacy completed-refund interface. The service uses the
// structured method below to distinguish pending/unknown outcomes.
func (p *PayPal) Refund(captureID string, refundCents int64, currency, reason string) string {
	id, status, err := p.RefundWithStatus(captureID, refundCents, currency, reason, "")
	if err != nil || status != "COMPLETED" {
		return ""
	}
	return id
}

// RefundWithStatus accepts a durable operation ID for retrying an uncertain
// result without issuing a second refund. PENDING is never reported completed.
func (p *PayPal) RefundWithStatus(captureID string, refundCents int64, currency, reason, requestID string) (string, string, error) {
	if captureID == "" || refundCents <= 0 || (currency != "" && currency != "USD") {
		return "", "FAILED", httperr.BadRequest("PayPal 退款参数无效")
	}
	if reason == "" {
		reason = "Group initiated refund"
	}
	body := map[string]any{
		"amount":        map[string]string{"value": money.ToYuanString(refundCents), "currency_code": "USD"},
		"note_to_payer": reason,
	}
	var result paypal.RefundResponse
	if err := p.send(http.MethodPost, "/v2/payments/captures/"+url.PathEscape(captureID)+"/refund", body, requestID, &result); err != nil {
		var responseError *paypal.ErrorResponse
		if errors.As(err, &responseError) && responseError.Response != nil &&
			responseError.Response.StatusCode >= 400 && responseError.Response.StatusCode < 500 &&
			responseError.Response.StatusCode != http.StatusConflict {
			return "", "FAILED", err
		}
		if httperr.As(err) != nil {
			return "", "FAILED", err
		}
		return "", "UNKNOWN", err
	}
	if result.Status == "FAILED" || result.Status == "CANCELLED" {
		return result.ID, "FAILED", nil
	}
	if result.ID == "" || !validPayPalAmount(result.Amount, refundCents) {
		return result.ID, "UNKNOWN", fmt.Errorf("PayPal 退款响应缺少有效单号或金额")
	}
	switch result.Status {
	case "COMPLETED", "PENDING":
		return result.ID, result.Status, nil
	default:
		return result.ID, "UNKNOWN", fmt.Errorf("PayPal 退款状态未知")
	}
}

// QueryRefund reconciles an already accepted refund without issuing another
// POST. Missing or inconsistent data keeps the existing operation uncertain.
func (p *PayPal) QueryRefund(refundID string, expectedCents int64, expectedCurrency string) (string, error) {
	if refundID == "" || expectedCents <= 0 || expectedCurrency != "USD" {
		return "UNKNOWN", httperr.BadRequest("PayPal 退款查询参数无效")
	}
	var result paypal.RefundResponse
	if err := p.send(http.MethodGet, "/v2/payments/refunds/"+url.PathEscape(refundID), nil, "", &result); err != nil {
		return "UNKNOWN", err
	}
	if result.ID != refundID || !validPayPalAmount(result.Amount, expectedCents) {
		return "UNKNOWN", fmt.Errorf("PayPal 退款查询响应与现有退款不匹配")
	}
	switch result.Status {
	case "COMPLETED", "PENDING":
		return result.Status, nil
	case "FAILED", "CANCELLED":
		return "FAILED", nil
	default:
		return "UNKNOWN", fmt.Errorf("PayPal 退款状态未知")
	}
}
