package routes

import (
	"net/http"
	"strconv"
	"strings"
	"time"

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
// 渠道专属端点（/gateway/alipay/pay 等）不需要传 payMethod，
// 所以拆成两份 —— 和原版用 rest 解构省掉一个字段是一回事。
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
// 一个分组可以有多把 Key，用哪把下的单就记在订单上，
// 出站回调用那把签 HMAC，见 services/notify.go。
func (a *App) registerGateway() {
	guard := httpx.RequireAPIKey(a.c.Store.ApiKeys)

	a.api("POST "+apiPrefix("/gateway/pay"),
		func(w http.ResponseWriter, r *http.Request) error {
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
		}, guard)

	// ── 渠道专属端点（无需传 payMethod） ────────────────────────

	a.api("POST "+apiPrefix("/gateway/alipay/pay"),
		a.channelPay("alipay"), guard)

	a.api("POST "+apiPrefix("/gateway/paypal/pay"),
		a.channelPay("paypal"), guard)

	a.api("POST "+apiPrefix("/gateway/native/pay"),
		a.channelPay("native"), guard)

	// ── 通用查询与退款 ──────────────────────────────────────────

	a.api("GET "+apiPrefix("/gateway/query"),
		func(w http.ResponseWriter, r *http.Request) error {
			group, err := mustGroup(r)
			if err != nil {
				return err
			}
			out, err := a.c.Payment.QueryByGroup(group.ID, queryParam(r, "orderNo"))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}, guard)

	a.api("POST "+apiPrefix("/gateway/refund"),
		func(w http.ResponseWriter, r *http.Request) error {
			body, err := validate.Check(httpx.JSONBody(r), refundSchema)
			if err != nil {
				return err
			}
			group, err := mustGroup(r)
			if err != nil {
				return err
			}
			out, err := a.c.Refund.Execute(services.RefundInput{
				OrderNo: body.Str("orderNo"),
				Amount:  body.Num("amount"),
				Reason:  optStrResult(body, "reason"),
			}, services.RefundOptions{
				GroupID: &group.ID,
				Actor:   "group:" + strconv.FormatInt(group.ID, 10),
				IP:      clientIP(r),
			})
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}, guard)
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
	order, err := a.c.Payment.CreateOrder(callerOf(r), input)
	if err != nil {
		return nil, err
	}
	base := httpx.BaseURL(r)

	switch input.PayMethod {
	case "paypal":
		return a.c.PayPal.CreateOrder(order.OrderNo, base)
	case "alipay":
		return a.c.Alipay.CreatePagePay(order.OrderNo, base)
	case "native":
		return a.c.NativePay.CreateOrder(order.OrderNo, base)
	default:
		// validate 的 in 已经挡住了，留着兜底
		return nil, httperr.BadRequestf("Unsupported payment method: %s", input.PayMethod)
	}
}

// inputWithMethod 把校验结果摊成下单入参，可选字段空串转 nil ——
// service 那边按 `*string` 的 undefined 语义用。
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
func (a *App) registerNativePay() {
	log := logger.New("NativePayRoutes")

	// 收银台信息，公开访问（付款人没有账号）
	a.api("GET "+apiPrefix("/native-pay/cashier"),
		func(w http.ResponseWriter, r *http.Request) error {
			out, err := a.c.NativePay.GetCashierInfo(queryParam(r, "orderNo"), httpx.BaseURL(r))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		})

	/**
	 * 公开的沙箱测试下单，无需登录。只在 ENABLE_SANDBOX=true 时可用。
	 */
	a.api("POST "+apiPrefix("/native-pay/public-test-pay"),
		func(w http.ResponseWriter, r *http.Request) error {
			group, err := a.c.Groups.DefaultGroup()
			if err != nil {
				return err
			}
			// 同 /admin/test-pay：没有 Key 上下文，也不带 notifyUrl
			order, err := a.c.Payment.CreateOrder(services.OrderCaller{GroupID: group.ID},
				services.CreateOrderInput{
					Amount:      0.01,
					ProductName: "Sandbox Pay 公开测试订单",
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
		},
		httpx.RequireSandbox(a.cfg),
		httpx.RateLimit("public-test-pay", 10, time.Minute))

	// 管理员手动核销
	a.api("POST "+apiPrefix("/native-pay/confirm"),
		func(w http.ResponseWriter, r *http.Request) error {
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
		}, httpx.RequireAuth(a.cfg))

	// 沙箱专用核销（生产关闭）
	a.api("POST "+apiPrefix("/native-pay/sandbox-confirm"),
		func(w http.ResponseWriter, r *http.Request) error {
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
		}, httpx.RequireSandbox(a.cfg))

	// 从收银台直连真实网关。5 次/分钟，避免刷上游。
	a.api("POST "+apiPrefix("/native-pay/switch-channel"),
		func(w http.ResponseWriter, r *http.Request) error {
			body, err := validate.Check(httpx.JSONBody(r), switchChannelSchema)
			if err != nil {
				return err
			}
			orderNo := body.Str("orderNo")
			base := httpx.BaseURL(r)

			var out *gateways.PayResult
			if body.Str("channel") == "alipay" {
				out, err = a.c.Alipay.CreatePagePay(orderNo, base)
			} else {
				out, err = a.c.PayPal.CreateOrder(orderNo, base)
			}
			if err != nil {
				// 只把已知业务错误的文案原样抛给调用方；未知错误（SQL、
				// SDK 内部）一律换成兜底文案 —— 别把内部细节漏到公网。
				message := ""
				if he := httperr.As(err); he != nil {
					message = he.Message
				}
				log.Error("Switch channel error: " + err.Error())
				if message == "" {
					message = "网关调用失败"
				}
				return httperr.BadRequest(message)
			}
			httpx.OK(w, out)
			return nil
		}, httpx.RateLimit("switch-channel", 5, time.Minute))
}

// registerAlipay /v1/api/alipay：支付宝异步回调。
func (a *App) registerAlipay() {
	/**
	 * 支付宝发 application/x-www-form-urlencoded，
	 * 期望的响应是纯文本 "success" / "failure"，不是 JSON ——
	 * 所以这里不套信封。
	 */
	a.plain("POST "+apiPrefix("/alipay/notify"), func(w http.ResponseWriter, r *http.Request) error {
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
	})
}

// registerPayPal /v1/api/paypal：PayPal 同步返回回调，capture 后重定向回收银台。
func (a *App) registerPayPal() {
	log := logger.New("PayPalRoutes")

	a.api("GET "+apiPrefix("/paypal/callback"),
		func(w http.ResponseWriter, r *http.Request) error {
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
					returnURL = clientURL + "/cashier.html"
				} else {
					returnURL = "/cashier.html"
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
		},
		httpx.RateLimit("paypal-callback", 10, time.Minute))
}

// buildReturnURL 拼回调落地地址：returnUrl 已带 ? 时用 & 接。
func buildReturnURL(returnURL, orderNo, status string) string {
	sep := "?"
	if strings.Contains(returnURL, "?") {
		sep = "&"
	}
	return returnURL + sep + "orderNo=" + orderNo + "&status=" + status
}

// redirect 302 跳转。
func redirect(w http.ResponseWriter, r *http.Request, location string) {
	http.Redirect(w, r, location, http.StatusFound)
}
