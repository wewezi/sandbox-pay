package routes

import (
	"math"
	"net/http"
	"strings"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/httpx"
	"sandbox-pay/internal/services"
	"sandbox-pay/internal/validate"
)

func (a *App) testPay() httpx.Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		body, err := validate.Check(httpx.JSONBody(r), testPaySchema)
		if err != nil {
			return err
		}
		var expireMinutes *int
		if body.Has("expireMinutes") {
			value := body.Num("expireMinutes")
			if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
				return httperr.BadRequest("expireMinutes 必须是 1–120 的整数分钟")
			}
			minutes := int(value)
			expireMinutes = &minutes
		}
		var keyID *int64
		if body.Has("apiKeyId") {
			value := body.Num("apiKeyId")
			if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
				return httperr.BadRequest("apiKeyId 必须是正整数")
			}
			id := int64(value)
			keyID = &id
		}
		caller, err := a.c.Groups.TestOrderCaller(keyID)
		if err != nil {
			return err
		}
		if scope := httpx.ProjectScope(r.Context()); scope != nil {
			allowed := false
			for _, id := range scope {
				if id == caller.GroupID {
					allowed = true
					break
				}
			}
			if !allowed {
				return httperr.NotFound("项目不存在")
			}
		}
		externalOrderNo := strings.TrimSpace(body.OptStr("externalOrderNo"))
		var external *string
		if externalOrderNo != "" {
			external = &externalOrderNo
		}
		order, err := a.c.Payment.CreateOrder(a.orderClient(r, caller), services.CreateOrderInput{
			Amount: body.Num("amount"), ProductName: trimOr(body.OptStr("productName"), "Mintweft Test Order"),
			PayMethod: trimOr(body.OptStr("payMethod"), "native"), ExpireMinutes: expireMinutes, ExternalOrderNo: external,
			ReturnURL: optStrResult(body, "returnUrl"), NotifyURL: optStrResult(body, "notifyUrl"),
		})
		if err != nil {
			return err
		}
		info, err := a.c.Groups.TestOrderInfo(order.OrderNo)
		if err != nil {
			return err
		}
		stored, err := a.c.Payment.QueryByGroup(caller.GroupID, order.OrderNo)
		if err != nil {
			return err
		}
		status := stored.Status
		if deadline, parseErr := time.Parse(time.RFC3339Nano, info.ExpireAt); status == db.StatusPending && parseErr == nil && !deadline.After(time.Now()) {
			status = db.StatusExpired
		}
		client := strings.TrimRight(strings.TrimSpace(a.cfg.Get("CLIENT_URL")), "/")
		if client == "" {
			client = strings.TrimRight(httpx.BaseURL(r), "/")
		}
		paymentURL := client + "/cashier?orderNo=" + order.OrderNo
		// A test order only preselects its channel. The cashier remains the
		// entry point; no provider transaction is created during this request.
		if status == db.StatusPending {
			out, err := a.c.NativePay.CreateOrder(order.OrderNo, httpx.BaseURL(r))
			if err != nil {
				return err
			}
			paymentURL = out.Data
		}
		httpx.OK(w, struct {
			Type       string `json:"type"`
			Data       string `json:"data"`
			PaymentURL string `json:"paymentUrl"`
			Status     int    `json:"status"`
			*services.TestOrderInfo
		}{"url", paymentURL, paymentURL, status, info})
		return nil
	}
}
