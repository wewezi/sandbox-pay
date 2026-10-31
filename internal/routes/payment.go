package routes

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/gateways"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/httpx"
	"sandbox-pay/internal/logger"
	"sandbox-pay/internal/services"
	"sandbox-pay/internal/validate"
)

// 下单 schema。
//
// 渠道专属端点直接指定支付方式，通用端点由请求体指定。
var (
	paySchema = validate.Schema{
		"amount":          validate.Num(0.01, 0),
		"productName":     validate.Str(0, 200),
		"payMethod":       validate.StrIn("alipay", "paypal", "native"),
		"externalOrderNo": validate.OptStr(0, 0),
		"returnUrl":       validate.OptURLRule(),
		"notifyUrl":       validate.OptURLRule(),
	}

	channelPaySchema = validate.Schema{
		"amount":          validate.Num(0.01, 0),
		"productName":     validate.Str(0, 200),
		"externalOrderNo": validate.OptStr(0, 0),
		"returnUrl":       validate.OptURLRule(),
		"notifyUrl":       validate.OptURLRule(),
	}
)

// registerGateway /v1/api/gateway。所有路由都要 API Key：
//
//	Authorization: Bearer <apiKey>
//
// 一个分组可以有多把 Key，用哪把下的单就记在订单和使用日志上。
// 出站回调由分组独立的 WebhookSecret 签名，见 services/notify.go。
func (a *App) registerGateway(r chi.Router) {
	r.Post("/pay", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		body, err := validate.Check(httpx.JSONBody(r), paySchema)
		if err != nil {
			return err
		}
		out, err := a.createAndDispatch(r, body,
			inputWithMethod(body, body.Str("payMethod")))
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}))

	// ── 渠道专属端点（无需传 payMethod） ────────────────────────

	r.Post("/alipay/pay", httpx.H(a.channelPay("alipay")))

	r.Post("/paypal/pay", httpx.H(a.channelPay("paypal")))

	r.Post("/native/pay", httpx.H(a.channelPay("native")))

	// ── 通用查询与退款 ──────────────────────────────────────────

	r.Get("/query", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		group, err := mustGroup(r)
		if err != nil {
			return err
		}
		orderNo := queryParam(r, "orderNo")
		externalOrderNo := queryParam(r, "externalOrderNo")
		if (orderNo == "") == (externalOrderNo == "") {
			return httperr.BadRequest("请提供 orderNo 或 externalOrderNo 中的一项")
		}
		var out *services.OrderGroupQueryResult
		if orderNo != "" {
			out, err = a.c.Payment.QueryByGroup(group.ID, orderNo)
		} else {
			out, err = a.c.Payment.QueryByExternalOrderNo(group.ID, externalOrderNo)
		}
		if err != nil {
			return err
		}
		httpx.BindUsageOrder(r.Context(), out.OrderNo)
		httpx.OK(w, out)
		return nil
	}))

	r.Post("/refund", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		body, err := validate.Check(httpx.JSONBody(r), refundSchema)
		if err != nil {
			return err
		}
		group, err := mustGroup(r)
		if err != nil {
			return err
		}
		out, err := a.c.Refund.Execute(services.RefundInput{
			OrderNo:   body.Str("orderNo"),
			Amount:    body.Num("amount"),
			Reason:    optStrResult(body, "reason"),
			RequestID: body.OptStr("requestId"),
		}, services.RefundOptions{
			GroupID: &group.ID,
			Actor:   "group:" + strconv.FormatInt(group.ID, 10),
			IP:      clientIP(r),
		})
		// A failed refund may still concern this project's real order. Keep
		// that association only after checking ownership; metadata lookup
		// failures must not replace the original refund result.
		if order, lookupErr := a.c.Store.Orders.FindByOrderNoAndGroup(body.Str("orderNo"), group.ID); lookupErr == nil && order != nil {
			httpx.BindUsageOrder(r.Context(), order.OrderNo)
		}
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}))
}

// channelPay 渠道专属下单：先按渠道名补上 payMethod，其余和 /pay 一样。
func (a *App) channelPay(payMethod string) httpx.Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		body, err := validate.Check(httpx.JSONBody(r), channelPaySchema)
		if err != nil {
			return err
		}
		out, err := a.createAndDispatch(r, body, inputWithMethod(body, payMethod))
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}
}

// createAndDispatch 建单，然后按渠道挑下单网关。
func (a *App) createAndDispatch(
	r *http.Request,
	body *validate.Result,
	input services.CreateOrderInput,
) (*gateways.PayResult, error) {
	order, err := a.c.Payment.CreateOrder(a.orderClient(r, callerOf(r)), input)
	if err != nil {
		return nil, err
	}
	httpx.BindUsageOrder(r.Context(), order.OrderNo)
	base := a.paymentBaseURL(r)
	stored, err := a.c.Payment.QueryByGroup(callerOf(r).GroupID, order.OrderNo)
	if err != nil {
		return nil, err
	}
	var result *gateways.PayResult
	status := stored.Status
	if status == db.StatusPending {
		if deadline, parseErr := time.Parse(time.RFC3339Nano, stored.ExpireAt); parseErr == nil && !deadline.After(time.Now()) {
			status = db.StatusExpired
		}
	}
	// Retrying an idempotent payment after its terminal state must return the
	// same local order and must never dispatch another request to the provider.
	if status != db.StatusPending {
		client := strings.TrimRight(strings.TrimSpace(a.cfg.Get("CLIENT_URL")), "/")
		if client == "" {
			client = strings.TrimRight(base, "/")
		}
		result = &gateways.PayResult{Type: "url", Data: client + "/cashier?orderNo=" + order.OrderNo}
	} else {

		switch input.PayMethod {
		case "paypal":
			result, err = a.c.PayPal.CreateOrder(order.OrderNo, base)
		case "alipay":
			result, err = a.c.Alipay.CreatePagePay(order.OrderNo, base)
		case "native":
			result, err = a.c.NativePay.CreateOrder(order.OrderNo, base)
		default:
			// validate 的 in 已经挡住了，留着兜底
			return nil, httperr.BadRequestf("Unsupported payment method: %s", input.PayMethod)
		}
	}
	if err != nil {
		return nil, err
	}
	result.OrderNo, result.Amount, result.ExpireAt, result.Status = order.OrderNo, stored.Amount, stored.ExpireAt, status
	result.PaymentURL = result.Data
	return result, nil
}

// inputWithMethod 组装下单入参，可选字符串未提供或为空时使用 nil。
func inputWithMethod(body *validate.Result, payMethod string) services.CreateOrderInput {
	return services.CreateOrderInput{
		Amount:          body.Num("amount"),
		ProductName:     body.Str("productName"),
		PayMethod:       payMethod,
		ExternalOrderNo: optStrResult(body, "externalOrderNo"),
		ReturnURL:       optStrResult(body, "returnUrl"),
		NotifyURL:       optStrResult(body, "notifyUrl"),
	}
}

// optStrResult 取可选字符串字段，空串/没传返回 nil。
func optStrResult(body *validate.Result, key string) *string {
	if !body.Has(key) {
		return nil
	}
	v := body.OptStr(key)
	if v == "" {
		return nil
	}
	return &v
}

// queryParam 取 query 并 trim —— service 那边按空串当「没传」处理。
func queryParam(r *http.Request, key string) string {
	return strings.TrimSpace(r.URL.Query().Get(key))
}

// trimOr 空串回落到 def。
func trimOr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return strings.TrimSpace(v)
}

// registerNativePay /v1/api/native-pay：自有存管收银台相关路由。
func (a *App) registerNativePay(r chi.Router) {
	log := logger.New("NativePayRoutes")
	r.Route("/native-pay", func(r chi.Router) {

		// 收银台信息，公开访问（付款人没有账号）
		r.Get("/cashier", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
			out, err := a.c.NativePay.GetCashierInfo(queryParam(r, "orderNo"), httpx.BaseURL(r))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}))

		/**
		 * 公开的沙箱测试下单，无需登录。只在 ENABLE_SANDBOX=true 时可用。
		 */
		r.With(httpx.RequireSandbox(a.cfg), httpx.RateLimit("public-test-pay", 10, time.Minute)).Post("/public-test-pay", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
			group, err := a.c.Groups.DefaultGroup()
			if err != nil {
				return err
			}
			// 同 /admin/test-pay：没有 Key 上下文，也不带 notifyUrl
			order, err := a.c.Payment.CreateOrder(a.orderClient(r, services.OrderCaller{GroupID: group.ID}),
				services.CreateOrderInput{
					Amount:      0.01,
					ProductName: "Mintweft Test Order",
					PayMethod:   "native",
				})
			if err != nil {
				return err
			}
			out, err := a.c.NativePay.CreateOrder(order.OrderNo, httpx.BaseURL(r))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}))

		// 管理员手动核销
		r.With(a.authMiddleware(), a.captureAdminAudit(), a.adminAccess()).Post("/confirm", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
			body, err := validate.Check(httpx.JSONBody(r), confirmSchema)
			if err != nil {
				return err
			}
			out, err := a.c.NativePay.ConfirmPayment(
				body.Str("orderNo"), body.OptStr("tradeNo"),
				body.Str("walletUser"), body.Str("walletPass"))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}))

		// 沙箱专用核销（生产关闭）
		r.With(httpx.RequireSandbox(a.cfg)).Post("/sandbox-confirm", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
			body, err := validate.Check(httpx.JSONBody(r), sandboxConfirmSchema)
			if err != nil {
				return err
			}
			out, err := a.c.NativePay.ConfirmPayment(
				body.Str("orderNo"), "SP_WALLET_"+strconv.FormatInt(time.Now().UnixMilli(), 10),
				body.OptStr("walletUser"), body.OptStr("walletPass"))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}))

		// 从收银台直连真实网关。5 次/分钟，避免刷上游。
		r.With(httpx.RateLimit("switch-channel", 5, time.Minute)).Post("/switch-channel", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
			body, err := validate.Check(httpx.JSONBody(r), switchChannelSchema)
			if err != nil {
				return err
			}
			orderNo := body.Str("orderNo")
			base := a.paymentBaseURL(r)
			channel := body.Str("channel")
			unlock := a.c.Payment.LockCashierChannel(orderNo)
			defer unlock()
			order, err := a.c.Payment.GetOrderForCashierChannel(orderNo, channel)
			if err != nil {
				return err
			}

			var out *gateways.PayResult
			if channel == "native" {
				err = a.c.Payment.SwitchPaymentMethod(orderNo, channel)
				if err == nil {
					out, err = a.c.NativePay.CreateOrder(orderNo, base)
				}
			} else if channel == "alipay" {
				// Page pay signs a URL locally. Publish the channel only after
				// generation succeeds and before returning that URL to a user.
				out, err = a.c.Alipay.CreatePagePay(orderNo, base)
				if err == nil {
					err = a.c.Payment.SwitchPaymentMethod(orderNo, channel)
				}
			} else {
				// Missing configuration or OAuth must not hide a usable wallet.
				err = a.c.PayPal.PrepareCreateOrder(orderNo)
				if err == nil {
					err = a.c.Payment.SwitchPaymentMethod(orderNo, channel)
					if err == nil {
						out, err = a.c.PayPal.CreateOrder(orderNo, base)
						if err != nil && gateways.ChannelCreationRejected(err) {
							if restoreErr := a.c.Payment.RestoreCashierChannel(order, channel); restoreErr != nil {
								log.Error("Restore cashier channel error: " + restoreErr.Error())
							}
						}
					}
				}
			}
			if err != nil {
				log.Error("Switch channel error: " + gateways.GatewayErrorSummary(err, channel) +
					" request=" + httpx.CorrelationID(r.Context()) + " order=" + orderNo)
				return gateways.GatewayError(err, channel)
			}
			httpx.OK(w, out)
			return nil
		}))
	})
}

func (a *App) paymentBaseURL(r *http.Request) string {
	if base := strings.TrimSpace(a.cfg.Get("CLIENT_URL")); base != "" {
		return strings.TrimRight(base, "/")
	}
	return httpx.BaseURL(r)
}

// registerAlipay /v1/api/alipay：支付宝异步回调。
func (a *App) registerAlipay(r chi.Router) {
	/**
	 * 支付宝发 application/x-www-form-urlencoded，
	 * 期望的响应是纯文本 "success" / "failure"，不是 JSON ——
	 * 所以这里不套信封。
	 */
	r.Post("/alipay/notify", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		if err := r.ParseForm(); err != nil {
			return httperr.BadRequest("解析支付宝回调失败")
		}
		params := make(map[string]string, len(r.PostForm))
		for k, vs := range r.PostForm {
			if len(vs) > 0 {
				params[k] = vs[0]
			}
		}
		result, err := a.c.Alipay.HandleNotify(params)
		if err != nil {
			return err
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if result == "success" {
			_, _ = w.Write([]byte("success"))
		} else {
			_, _ = w.Write([]byte("failure"))
		}
		return nil
	}))
}

// registerPayPal /v1/api/paypal：PayPal 同步返回回调，capture 后重定向回收银台。
func (a *App) registerPayPal(r chi.Router) {
	log := logger.New("PayPalRoutes")

	r.With(httpx.RateLimit("paypal-callback", 10, time.Minute)).Get("/paypal/callback", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		orderNo := queryParam(r, "orderNo")
		token := queryParam(r, "token")
		cancel := queryParam(r, "cancel")

		if orderNo == "" {
			redirect(w, r, "/?error=missing_order_no")
			return nil
		}

		// 通过 PaymentService 查询订单，不直接访问 repo
		order, err := a.c.Payment.QueryOrder(orderNo)
		if err != nil {
			redirect(w, r, "/?error=order_not_found")
			return nil
		}

		clientURL := strings.TrimSpace(a.c.Config.Get("CLIENT_URL"))
		returnURL := ""
		if order.ReturnURL != nil {
			returnURL = *order.ReturnURL
		}
		if returnURL == "" {
			if clientURL != "" {
				returnURL = clientURL + "/cashier"
			} else {
				returnURL = "/cashier"
			}
		}

		if cancel != "" {
			redirect(w, r, buildReturnURL(returnURL, orderNo, "cancelled"))
			return nil
		}

		if order.Status != db.StatusPending {
			log.Warn("PayPal callback for non-pending order " + orderNo +
				" (status: " + strconv.Itoa(order.Status) + "), skipping capture")
			status := "failed"
			if order.Status == db.StatusPaid {
				status = "success"
			}
			redirect(w, r, buildReturnURL(returnURL, orderNo, status))
			return nil
		}

		if token != "" {
			result, err := a.c.PayPal.CaptureOrder(orderNo, token)
			if err == nil && result != nil && result.Success {
				redirect(w, r, buildReturnURL(returnURL, orderNo, "success"))
				return nil
			}
			if err != nil {
				log.Error("PayPal capture failed for " + orderNo + ": " + err.Error())
			}
		}

		redirect(w, r, buildReturnURL(returnURL, orderNo, "failed"))
		return nil
	}))
}

// buildReturnURL preserves existing query parameters and fragments while
// replacing callback parameters, including on repeated provider returns.
func buildReturnURL(returnURL, orderNo, status string) string {
	target, err := url.Parse(returnURL)
	if err != nil {
		target = &url.URL{Path: "/cashier"}
	}
	query := target.Query()
	query.Set("orderNo", orderNo)
	query.Set("status", status)
	target.RawQuery = query.Encode()
	return target.String()
}

// redirect 302 跳转。
func redirect(w http.ResponseWriter, r *http.Request, location string) {
	http.Redirect(w, r, location, http.StatusFound)
}
