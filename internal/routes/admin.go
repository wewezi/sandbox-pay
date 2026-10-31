package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"sandbox-pay/internal/httpx"
	"sandbox-pay/internal/services"
	"sandbox-pay/internal/validate"
)

// registerAdmin 在后台路由组统一挂载会话校验、操作审计和项目权限。
func (a *App) registerAdmin(r chi.Router) {
	r.Route("/admin", func(r chi.Router) {
		r.Use(a.authMiddleware(), a.captureAdminAudit())
		r = r.With(a.adminAccess())
		a.registerUsageLogs(r)
		a.registerTokens(r)
		a.registerCallbackDiagnostics(r)
		a.registerSandboxScenarios(r)
		a.registerOperations(r)
		a.registerDashboard(r)
		a.registerOrderDetails(r)
		a.registerAdminDeveloper(r)
		a.registerTeam(r)

		r.Get("/stats", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
			out, err := a.c.Admin.GetStats(optInt64(r, "groupId"), httpx.ProjectScope(r.Context()))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}))

		r.Get("/transactions", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
			utcOffset, err := parseUTCOffset(queryParam(r, "utcOffset"))
			if err != nil {
				return err
			}
			out, err := a.c.Admin.GetTransactions(services.TransactionOpts{
				GroupID:    optInt64(r, "groupId"),
				ProjectIDs: httpx.ProjectScope(r.Context()),
				Page:       queryParam(r, "page"),
				PageSize:   queryParam(r, "pageSize"),
				Status:     queryParam(r, "status"),
				PayMethod:  queryParam(r, "payMethod"),
				DateFrom:   queryParam(r, "dateFrom"),
				DateTo:     queryParam(r, "dateTo"),
				UTCOffset:  utcOffset,
				Keyword:    queryParam(r, "keyword"),
			})
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}))

		r.Delete("/transactions/{orderNo}", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
			out, err := a.c.Admin.DeleteTransaction(
				pathParam(r, "orderNo"), actorOf(r), clientIP(r))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}))

		// ── 分组与 API Key ───────────────────────────────────────────

		r.Get("/groups", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
			out, err := a.c.Groups.ListGroups(httpx.ProjectScope(r.Context()))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}))

		r.Post("/groups", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
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
		}))

		// 停用 / 启用切换。校验器没有 boolean 类型，做成 toggle 就不用传 body。
		r.Post("/groups/{id}/toggle", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
			out, err := a.c.Groups.ToggleGroup(pathInt64(r, "id"))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}))

		r.Post("/groups/{id}/keys", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
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
		}))

		r.Post("/api-keys/{id}/toggle", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
			out, err := a.c.Groups.ToggleKey(pathInt64(r, "id"))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}))

		r.With(httpx.RequireSandbox(a.cfg)).Post("/test-pay", httpx.H(a.testPay()))

		/**
		 * 后台发起的退款。只认 JWT（不要接入方签名），面板可以直接退。
		 * GroupID 传 nil，所以任意分组的订单都能退。
		 */
		r.Post("/refund", httpx.H(a.doRefund()))

		// ── Notifications ────────────────────────────────────────────

		r.Get("/notifications", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
			out, err := a.c.Admin.GetNotificationStatusScoped(
				httpx.ProjectScope(r.Context()), queryParam(r, "page"), queryParam(r, "pageSize"), queryParam(r, "status"), queryParam(r, "orderNo"))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}))

		r.Post("/notifications/{id}/replay", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
			out, err := a.c.Admin.ReplayNotification(pathInt64(r, "id"), actorOf(r), clientIP(r))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}))

		// ── Data Reset (Sandbox) ─────────────────────────────────────

		r.With(httpx.RequireSandbox(a.cfg)).Post("/reset-data", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
			out, err := a.c.Admin.ResetData(actorOf(r), clientIP(r))
			if err != nil {
				return err
			}
			httpx.OK(w, out)
			return nil
		}))
	})
}

// doRefund 后台退款：不传 GroupID，所以任意分组的订单都能退。
func (a *App) doRefund() httpx.Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		body, err := validate.Check(httpx.JSONBody(r), refundSchema)
		if err != nil {
			return err
		}
		out, err := a.c.Refund.Execute(services.RefundInput{
			OrderNo:   body.Str("orderNo"),
			Amount:    body.Num("amount"),
			Reason:    optStrResult(body, "reason"),
			RequestID: body.OptStr("requestId"),
		}, services.RefundOptions{Actor: actorOf(r), IP: clientIP(r)})
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}
}
