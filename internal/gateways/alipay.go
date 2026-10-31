package gateways

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smartwalle/alipay/v3"

	"sandbox-pay/internal/apiurl"
	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/logger"
	"sandbox-pay/internal/money"
	"sandbox-pay/internal/services"
)

// Alipay 支付宝渠道。
//
// SDK 客户端惰性创建并缓存：没配凭证时服务照常启动，第一次真正调支付宝才失败。
type Alipay struct {
	cfg      config.Config
	log      *logger.Logger
	store    *db.Store
	payments *services.PaymentService

	mu  sync.Mutex
	cli *alipay.Client
	// Only the refund operation needs this narrow seam for transport tests.
	refundSDK alipayRefundClient
}

// NewAlipay 构造。
func NewAlipay(cfg config.Config, store *db.Store, payments *services.PaymentService) *Alipay {
	return &Alipay{
		cfg:      cfg,
		log:      logger.New("AlipayService"),
		store:    store,
		payments: payments,
	}
}

// client 取（必要时创建）SDK 客户端。
//
// 三项凭证任一为空就报 400 —— 这是配置问题不是服务端故障，回 500 会误导人。
// 私钥/公钥解析失败是普通 error 原样往上抛，由全局错误处理变成 500。
func (a *Alipay) client() (*alipay.Client, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cli != nil {
		return a.cli, nil
	}

	appID := strings.TrimSpace(a.cfg.Get("ALIPAY_APP_ID"))
	privateKey := strings.TrimSpace(a.cfg.Get("ALIPAY_PRIVATE_KEY"))
	publicKey := strings.TrimSpace(a.cfg.Get("ALIPAY_PUBLIC_KEY"))
	if appID == "" || privateKey == "" || publicKey == "" {
		return nil, httperr.BadRequest(
			"支付宝渠道未配置，请在 .env 设置 ALIPAY_APP_ID / ALIPAY_PRIVATE_KEY / ALIPAY_PUBLIC_KEY")
	}

	// production=false 才吃 WithSandboxGateway；网关地址照 .env 走，
	// 默认值见 config.Defaults（支付宝沙箱网关）。
	cli, err := alipay.New(appID, privateKey, false,
		alipay.WithSandboxGateway(config.String(a.cfg, "ALIPAY_SERVER_URL")))
	if err != nil {
		return nil, gatewayConfigurationFailure("ALIPAY_PRIVATE_KEY", err)
	}
	// 不加载支付宝公钥就全挂：响应验签（NeedVerify=true）和 notify 的
	// VerifySign 都靠它提供验签器，缺了直接 ErrAliPublicKeyNotFound。
	if err := cli.LoadAliPayPublicKey(publicKey); err != nil {
		return nil, gatewayConfigurationFailure("ALIPAY_PUBLIC_KEY", err)
	}

	a.log.Log("Alipay SDK initialized")
	a.cli = cli
	return cli, nil
}

// CreatePagePay 电脑网站支付：返回签名好的跳转 URL。
//
// 原版回的是一段自提交 HTML 表单，前端要剥掉里面的 <script> 再手动 submit
// （CSP 禁 inline script）。这里直接给签名后的 GET URL，前端 location.href
// 一跳就完事；type 仍然叫 'form'，前端只认 'url' | 'form' 两个字面量。
func (a *Alipay) CreatePagePay(orderNo, baseUrl string) (*PayResult, error) {
	order, err := a.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, httperr.BadRequest("订单不存在")
	}
	if order.Status != db.StatusPending {
		return nil, httperr.BadRequest("订单状态非待支付")
	}
	if !order.ExpireAt.After(time.Now()) {
		return nil, httperr.Conflict("订单已过期，请重新下单")
	}

	clientURL := strings.TrimSpace(a.cfg.Get("CLIENT_URL"))
	if clientURL == "" {
		clientURL = baseUrl
	}
	returnURL := clientURL + "/cashier?orderNo=" + orderNo
	if order.ReturnURL != nil && *order.ReturnURL != "" {
		if target, err := url.Parse(*order.ReturnURL); err == nil {
			query := target.Query()
			query.Set("orderNo", orderNo)
			target.RawQuery = query.Encode()
			returnURL = target.String()
		}
	}

	// 取 SDK 放在「业务失败分支」之外，让「渠道未配置」以 400 原样抛出，
	// 不被下面的 500 盖掉。
	sdk, err := a.client()
	if err != nil {
		return nil, err
	}

	u, err := sdk.TradePagePay(alipay.TradePagePay{
		Trade: alipay.Trade{
			NotifyURL:   apiurl.URL(baseUrl, "alipay/notify"),
			ReturnURL:   returnURL,
			Subject:     order.ProductName,
			OutTradeNo:  order.OrderNo,
			TotalAmount: money.ToYuanString(order.Amount),
			ProductCode: "FAST_INSTANT_TRADE_PAY",
			TimeExpire:  alipayPaymentDeadline(order.ExpireAt),
		},
	})
	if err != nil {
		a.log.Error("Alipay page pay error: " + GatewayErrorSummary(err, "alipay"))
		return nil, err
	}

	a.log.Log(fmt.Sprintf("Alipay page pay form generated for order: %s", orderNo))
	return &PayResult{Type: "form", Data: u.String()}, nil
}

// HandleNotify 处理支付宝异步通知，返回回给支付宝的纯文本 "success"/"failure"。
//
// 支付宝要的是 text/plain 而不是统一信封 —— 路由层直接把字符串写进响应体。
// 验签器/凭证缺失以 error 上抛（400），和原版把 SDK 取值放在 try 外面一致。
func (a *Alipay) HandleNotify(params map[string]string) (string, error) {
	sdk, err := a.client()
	if err != nil {
		return "", err
	}

	values := make(url.Values, len(params))
	for k, v := range params {
		values.Set(k, v)
	}
	if err := sdk.VerifySign(context.Background(), values); err != nil {
		a.log.Warn("Alipay notify signature verification failed")
		return "failure", nil
	}

	tradeStatus := params["trade_status"]
	if tradeStatus == "TRADE_SUCCESS" || tradeStatus == "TRADE_FINISHED" {
		// 支付宝回的是 "10.00" 这种元字符串，边界上转成分。
		if _, err := a.payments.MarkPaidForChannel("alipay", params["out_trade_no"], strp(params["trade_no"]),
			money.YuanStringToCents(params["total_amount"]), "CNY"); err != nil {
			// 金额不匹配或其他校验失败 —— 记下来并让支付宝稍后重试。
			a.log.Error(fmt.Sprintf("Alipay notify rejected for %s: %s", params["out_trade_no"], err))
			return "failure", nil
		}
		a.log.Log(fmt.Sprintf("Alipay payment success: %s, trade_no: %s",
			params["out_trade_no"], params["trade_no"]))
	}
	return "success", nil
}

// CreateQrPay 当面付预下单，返回二维码码串（原版的 null 用空串表示）。
//
// 建单不存在 / 非待支付 / 任何上游失败都只是「拿不到码」，回空串，
// 收银台照常渲染线下打款信息。
func (a *Alipay) CreateQrPay(orderNo, baseUrl string) string {
	order, err := a.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil || order == nil || order.Status != db.StatusPending || order.PayMethod != "alipay" || !order.ExpireAt.After(time.Now()) {
		return ""
	}

	// CLIENT_URL 优先：本机开发时 baseUrl 是 localhost，支付宝回调进不来，
	// 填 ngrok 之类的公网地址才收得到 notify。
	publicURL := strings.TrimSpace(a.cfg.Get("CLIENT_URL"))
	if publicURL == "" {
		publicURL = baseUrl
	}

	sdk, err := a.client()
	if err != nil {
		a.log.Error("Alipay precreate error: " + GatewayErrorSummary(err, "alipay"))
		return ""
	}

	rsp, err := sdk.TradePreCreate(context.Background(), alipay.TradePreCreate{
		Trade: alipay.Trade{
			NotifyURL:   apiurl.URL(publicURL, "alipay/notify"),
			Subject:     order.ProductName,
			OutTradeNo:  order.OrderNo,
			TotalAmount: money.ToYuanString(order.Amount),
			TimeExpire:  alipayPaymentDeadline(order.ExpireAt),
			// 当面付销售产品码。Trade.ProductCode 没有 omitempty，
			// 不填会往 biz_content 里发一个空串。
			ProductCode: "FACE_TO_FACE_PAYMENT",
		},
	})
	if err != nil {
		a.log.Error("Alipay precreate error: " + GatewayErrorSummary(err, "alipay"))
		return ""
	}
	if rsp == nil {
		return ""
	}
	if raw, err := json.Marshal(rsp); err == nil {
		a.log.Log(fmt.Sprintf("Alipay precreate result for %s: %s", orderNo, raw))
	}

	if rsp.SubCode == "ACQ.TRADE_HAS_SUCCESS" {
		// 支付宝那边已经付过了 —— 主动查一次拿到真实 trade_no 和金额，
		// 走带金额校验的 markPaid。
		a.log.Log(fmt.Sprintf(
			"Alipay precreate detected order %s already paid, switching to active query.", orderNo))
		a.QueryAlipayTrade(orderNo)
		return ""
	}
	return rsp.QRCode
}

// Alipay interprets its absolute deadline in UTC+8. Reusing the stored
// deadline prevents a later URL or QR retry from extending the payment window.
func alipayPaymentDeadline(deadline time.Time) string {
	return deadline.In(time.FixedZone("CST", 8*60*60)).Format("2006-01-02 15:04:05")
}

type alipayRefundClient interface {
	TradeRefund(context.Context, alipay.TradeRefund) (*alipay.TradeRefundRsp, error)
}

// Refund preserves the legacy interface. New callers provide their durable
// operation ID through RefundWithStatus instead of generating an ID per retry.
func (a *Alipay) Refund(orderNo string, refundCents int64, reason string) string {
	var randomID [16]byte
	if _, err := rand.Read(randomID[:]); err != nil {
		return ""
	}
	id, status, err := a.RefundWithStatus(orderNo, refundCents, reason, "RF_"+hex.EncodeToString(randomID[:]))
	if err != nil || status != "COMPLETED" {
		return ""
	}
	return id
}

// RefundWithStatus reuses out_request_no for retries. A timeout or an invalid
// response cannot prove that Alipay rejected the refund, so it remains UNKNOWN.
func (a *Alipay) RefundWithStatus(orderNo string, refundCents int64, reason, requestID string) (string, string, error) {
	if orderNo == "" || refundCents <= 0 || requestID == "" || len(requestID) > 64 ||
		strings.IndexFunc(requestID, func(c rune) bool {
			return !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-')
		}) >= 0 {
		return "", "FAILED", httperr.BadRequest("支付宝退款参数无效")
	}
	if reason == "" {
		reason = "Group initiated refund"
	}
	sdk := a.refundSDK
	if sdk == nil {
		client, err := a.client()
		if err != nil {
			return "", "FAILED", err
		}
		sdk = client
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rsp, err := sdk.TradeRefund(ctx, alipay.TradeRefund{
		OutTradeNo:   orderNo,
		RefundAmount: money.ToYuanString(refundCents),
		RefundReason: reason,
		OutRequestNo: requestID,
	})
	if err != nil {
		var businessError *alipay.Error
		if errors.As(err, &businessError) && definiteAlipayRefundFailure(*businessError) {
			return "", "FAILED", err
		}
		return "", "UNKNOWN", err
	}
	if rsp == nil {
		return "", "UNKNOWN", fmt.Errorf("支付宝退款响应为空")
	}
	if rsp.Code != alipay.CodeSuccess {
		if definiteAlipayRefundFailure(rsp.Error) {
			return "", "FAILED", rsp.Error
		}
		return "", "UNKNOWN", rsp.Error
	}
	if rsp.OutTradeNo != orderNo || !validAlipayRefundAmount(rsp.RefundFee, refundCents) ||
		(rsp.FundChange != "Y" && rsp.FundChange != "N") {
		return "", "UNKNOWN", fmt.Errorf("支付宝退款响应与请求不匹配")
	}
	// N with a successful, matching response confirms this operation was
	// processed before; replaying it must not become another refund.
	a.log.Log(fmt.Sprintf("Alipay refund confirmed for %s: %s", orderNo, requestID))
	return requestID, "COMPLETED", nil
}

func definiteAlipayRefundFailure(response alipay.Error) bool {
	switch response.Code {
	case alipay.CodeInvalidAuthToken, alipay.CodeMissingParam, alipay.CodeInvalidParam,
		alipay.CodeInsufficientConditions, alipay.CodeCallLimited, alipay.CodePermissionDenied:
		return true
	case alipay.CodeBusinessFailed:
		return !strings.Contains(response.SubCode, "SYSTEM_ERROR") &&
			!strings.Contains(response.SubCode, "SYSTEM_BUSY")
	default:
		return false
	}
}

func validAlipayRefundAmount(value string, expected int64) bool {
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" || expected <= 0 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole > (math.MaxInt64-99)/100 {
		return false
	}
	var fraction int64
	if len(parts) == 2 {
		if len(parts[1]) > 2 {
			return false
		}
		fraction, _ = strconv.ParseInt(parts[1], 10, 64)
		if len(parts[1]) == 1 {
			fraction *= 10
		}
	}
	return whole*100+fraction == expected
}

// QueryAlipayTrade 主动查单：确认支付宝那边真收到钱了才入账。
//
// 查询失败、状态未到、金额缺失、markPaid 拒绝 —— 全都只是「这次没确认下来」，
// 返回 false 且不向上抛（收银台还要继续渲染）。
func (a *Alipay) QueryAlipayTrade(orderNo string) bool {
	sdk, err := a.client()
	if err != nil {
		a.log.Error("Alipay query error for " + orderNo + ": " + GatewayErrorSummary(err, "alipay"))
		return false
	}

	// The cashier waits for this read before showing the order. A stalled
	// provider must release it promptly so other payment methods remain usable.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rsp, err := sdk.TradeQuery(ctx, alipay.TradeQuery{OutTradeNo: orderNo})
	if err != nil {
		a.log.Error("Alipay query error for " + orderNo + ": " + GatewayErrorSummary(err, "alipay"))
		return false
	}
	if rsp == nil {
		return false
	}
	if rsp.TradeStatus != alipay.TradeStatusSuccess && rsp.TradeStatus != alipay.TradeStatusFinished {
		return false
	}

	tradeNo := rsp.TradeNo
	totalAmountCents := money.YuanStringToCents(rsp.TotalAmount)
	if tradeNo == "" || totalAmountCents <= 0 {
		a.log.Warn(fmt.Sprintf("Alipay query missing tradeNo/totalAmount for %s, skip markPaid", orderNo))
		return false
	}

	if _, err := a.payments.MarkPaidForChannel("alipay", orderNo, &tradeNo, totalAmountCents, "CNY"); err != nil {
		a.log.Error(fmt.Sprintf("Alipay active query markPaid rejected for %s: %s", orderNo, err))
		return false
	}
	a.log.Log(fmt.Sprintf("Alipay active query confirmed paid for %s, tradeNo: %s", orderNo, tradeNo))
	return true
}
