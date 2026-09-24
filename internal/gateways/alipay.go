package gateways

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
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
		return nil, err
	}
	// 不加载支付宝公钥就全挂：响应验签（NeedVerify=true）和 notify 的
	// VerifySign 都靠它提供验签器，缺了直接 ErrAliPublicKeyNotFound。
	if err := cli.LoadAliPayPublicKey(publicKey); err != nil {
		return nil, err
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

	clientURL := strings.TrimSpace(a.cfg.Get("CLIENT_URL"))
	if clientURL == "" {
		clientURL = baseUrl
	}
	returnURL := clientURL + "/cashier.html?orderNo=" + orderNo
	if order.ReturnURL != nil && *order.ReturnURL != "" {
		sep := "?"
		if strings.Contains(*order.ReturnURL, "?") {
			sep = "&"
		}
		returnURL = *order.ReturnURL + sep + "orderNo=" + orderNo
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
		},
	})
	if err != nil {
		a.log.Error(fmt.Sprintf("Alipay page pay error: %s", err))
		return nil, httperr.Internal("生成支付宝支付表单失败")
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
		if _, err := a.payments.MarkPaid(params["out_trade_no"], strp(params["trade_no"]),
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
	if err != nil || order == nil || order.Status != db.StatusPending {
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
		a.log.Error(fmt.Sprintf("Alipay precreate error: %s", err))
		return ""
	}

	rsp, err := sdk.TradePreCreate(context.Background(), alipay.TradePreCreate{
		Trade: alipay.Trade{
			NotifyURL:   apiurl.URL(publicURL, "alipay/notify"),
			Subject:     order.ProductName,
			OutTradeNo:  order.OrderNo,
			TotalAmount: money.ToYuanString(order.Amount),
			// 当面付销售产品码。Trade.ProductCode 没有 omitempty，
			// 不填会往 biz_content 里发一个空串。
			ProductCode: "FACE_TO_FACE_PAYMENT",
		},
	})
	if err != nil {
		a.log.Error(fmt.Sprintf("Alipay precreate error: %s", err))
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

// Refund 发起支付宝退款，用 out_request_no 做幂等。
//
// 网关自己吞掉所有错误：返回空串 = 失败，由 RefundService 统一转成
// 「上游退款失败」。实现 services.Refunder。
func (a *Alipay) Refund(orderNo string, refundCents int64, reason string) string {
	if reason == "" {
		reason = "Group initiated refund"
	}
	outRequestNo := fmt.Sprintf("RF_%s_%d", orderNo, time.Now().UnixMilli())

	sdk, err := a.client()
	if err != nil {
		a.log.Error(fmt.Sprintf("Alipay refund error for %s: %s", orderNo, err))
		return ""
	}

	rsp, err := sdk.TradeRefund(context.Background(), alipay.TradeRefund{
		OutTradeNo:   orderNo,
		RefundAmount: money.ToYuanString(refundCents),
		RefundReason: reason,
		OutRequestNo: outRequestNo,
	})
	if err != nil {
		a.log.Error(fmt.Sprintf("Alipay refund error for %s: %s", orderNo, err))
		return ""
	}
	if rsp == nil {
		return ""
	}

	// fund_change=Y 才算钱真的动了 —— 重复退款时支付宝回成功但不改资金。
	ok := (rsp.Code == alipay.CodeSuccess || rsp.Msg == "Success") && rsp.FundChange == "Y"
	if ok {
		a.log.Log(fmt.Sprintf("Alipay refund success for %s: %s", orderNo, outRequestNo))
		return outRequestNo
	}
	if raw, err := json.Marshal(rsp); err == nil {
		a.log.Warn(fmt.Sprintf("Alipay refund non-success for %s: %s", orderNo, raw))
	}
	return ""
}

// QueryAlipayTrade 主动查单：确认支付宝那边真收到钱了才入账。
//
// 查询失败、状态未到、金额缺失、markPaid 拒绝 —— 全都只是「这次没确认下来」，
// 返回 false 且不向上抛（收银台还要继续渲染）。
func (a *Alipay) QueryAlipayTrade(orderNo string) bool {
	sdk, err := a.client()
	if err != nil {
		a.log.Error(fmt.Sprintf("Alipay queryAlipayTrade error for %s: %s", orderNo, err))
		return false
	}

	rsp, err := sdk.TradeQuery(context.Background(), alipay.TradeQuery{OutTradeNo: orderNo})
	if err != nil {
		a.log.Error(fmt.Sprintf("Alipay queryAlipayTrade error for %s: %s", orderNo, err))
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

	if _, err := a.payments.MarkPaid(orderNo, &tradeNo, totalAmountCents, "CNY"); err != nil {
		a.log.Error(fmt.Sprintf("Alipay active query markPaid rejected for %s: %s", orderNo, err))
		return false
	}
	a.log.Log(fmt.Sprintf("Alipay active query confirmed paid for %s, tradeNo: %s", orderNo, tradeNo))
	return true
}
