package routes

import (
	"net/http"

	"sandbox-pay/internal/httpx"
	"sandbox-pay/internal/services"
	"sandbox-pay/internal/validate"
)

// registerAdmin /v1/api/admin。整个后台都要登录（守卫逐条挂，不挂子树 ——
// net/http 的子树模式没有「先中间件后路由」的概念，逐条挂才是显式的）。
func (a *App) registerAdmin() {
	auth := httpx.RequireAuth(a.cfg)

	a.api("GET "+apiPrefix("/admin/stats"),
		func(w http.ResponseWriter, r *http.Request) error {
			out, err := a.c.Admin.GetStats(optInt64(r, "groupId"))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}, auth)

	a.api("GET "+apiPrefix("/admin/transactions"),
		func(w http.ResponseWriter, r *http.Request) error {
			out, err := a.c.Admin.GetTransactions(services.TransactionOpts{
				GroupID:   optInt64(r, "groupId"),
				Page:      queryParam(r, "page"),
				PageSize:  queryParam(r, "pageSize"),
				Status:    queryParam(r, "status"),
				PayMethod: queryParam(r, "payMethod"),
				DateFrom:  queryParam(r, "dateFrom"),
				DateTo:    queryParam(r, "dateTo"),
				Keyword:   queryParam(r, "keyword"),
			})
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}, auth)

	a.api("DELETE "+apiPrefix("/admin/transactions/{orderNo}"),
		func(w http.ResponseWriter, r *http.Request) error {
			out, err := a.c.Admin.DeleteTransaction(
				r.PathValue("orderNo"), actorOf(r), clientIP(r))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}, auth)

	// ── 分组与 API Key ───────────────────────────────────────────

	a.api("GET "+apiPrefix("/admin/groups"),
		func(w http.ResponseWriter, r *http.Request) error {
			out, err := a.c.Groups.ListGroups()
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}, auth)

	a.api("POST "+apiPrefix("/admin/groups"),
		func(w http.ResponseWriter, r *http.Request) error {
			body, err := validate.Check(httpx.JSONBody(r), createGroupSchema)
			if err != nil {
				return err
			}
			out, err := a.c.Groups.CreateGroup(body.Str("name"), body.OptStr("keyName"))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}, auth)

	// 停用 / 启用切换。校验器没有 boolean 类型，做成 toggle 就不用传 body。
	a.api("POST "+apiPrefix("/admin/groups/{id}/toggle"),
		func(w http.ResponseWriter, r *http.Request) error {
			out, err := a.c.Groups.ToggleGroup(pathInt64(r, "id"))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}, auth)

	a.api("POST "+apiPrefix("/admin/groups/{id}/keys"),
		func(w http.ResponseWriter, r *http.Request) error {
			body, err := validate.Check(httpx.JSONBody(r), issueKeySchema)
			if err != nil {
				return err
			}
			out, err := a.c.Groups.IssueKey(pathInt64(r, "id"), body.OptStr("name"))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}, auth)

	a.api("POST "+apiPrefix("/admin/api-keys/{id}/toggle"),
		func(w http.ResponseWriter, r *http.Request) error {
			out, err := a.c.Groups.ToggleKey(pathInt64(r, "id"))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}, auth)

	a.api("POST "+apiPrefix("/admin/test-pay"),
		func(w http.ResponseWriter, r *http.Request) error {
			body, err := validate.Check(httpx.JSONBody(r), testPaySchema)
			if err != nil {
				return err
			}
			group, err := a.c.Groups.DefaultGroup()
			if err != nil {
				return err
			}
			// 没有 apiKeyId：后台下单没有 Key 上下文，也不带 notifyUrl，签不到名
			order, err := a.c.Payment.CreateOrder(services.OrderCaller{GroupID: group.ID},
				services.CreateOrderInput{
					Amount:      body.Num("amount"),
					ProductName: trimOr(body.OptStr("productName"), "Sandbox Pay 测试订单"),
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
		}, auth)

	/**
	 * 后台发起的退款。只认 JWT（不要接入方签名），面板可以直接退。
	 * GroupID 传 nil，所以任意分组的订单都能退。
	 */
	a.api("POST "+apiPrefix("/admin/refund"), a.doRefund(), auth)

	// ── Notifications ────────────────────────────────────────────

	a.api("GET "+apiPrefix("/admin/notifications"),
		func(w http.ResponseWriter, r *http.Request) error {
			out, err := a.c.Admin.GetNotificationStatus(
				queryParam(r, "page"), queryParam(r, "pageSize"), queryParam(r, "status"))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}, auth)

	a.api("POST "+apiPrefix("/admin/notifications/{id}/replay"),
		func(w http.ResponseWriter, r *http.Request) error {
			out, err := a.c.Admin.ReplayNotification(pathInt64(r, "id"), actorOf(r), clientIP(r))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}, auth)

	// ── Data Reset (Sandbox) ─────────────────────────────────────

	a.api("POST "+apiPrefix("/admin/reset-data"),
		func(w http.ResponseWriter, r *http.Request) error {
			out, err := a.c.Admin.ResetData(actorOf(r), clientIP(r))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}, auth, httpx.RequireSandbox(a.cfg))
}

// doRefund 后台退款：不传 GroupID，所以任意分组的订单都能退。
func (a *App) doRefund() httpx.Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		body, err := validate.Check(httpx.JSONBody(r), refundSchema)
		if err != nil {
			return err
		}
		out, err := a.c.Refund.Execute(services.RefundInput{
			OrderNo: body.Str("orderNo"),
			Amount:  body.Num("amount"),
			Reason:  optStrResult(body, "reason"),
		}, services.RefundOptions{Actor: actorOf(r), IP: clientIP(r)})
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}
}
