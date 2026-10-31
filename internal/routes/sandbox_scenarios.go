package routes

import (
	"math"
	"net/http"

	"github.com/go-chi/chi/v5"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/httpx"
	"sandbox-pay/internal/services"
	"sandbox-pay/internal/validate"
)

var sandboxScenarioSchema = validate.Schema{
	"orderNo":      validate.Str(1, 200),
	"scenario":     validate.StrIn("paid", "failed", "expired", "delayed_paid", "refund_failed", "refund_pending"),
	"delaySeconds": validate.OptNum(0, 3600),
	"amount":       validate.OptNum(0.01, 0),
}

func (a *App) registerSandboxScenarios(r chi.Router) {
	r = r.With(httpx.RequireSandbox(a.cfg))
	r.Post("/sandbox/scenarios", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		body, err := validate.Check(httpx.JSONBody(r), sandboxScenarioSchema)
		if err != nil {
			return err
		}
		delay := body.Num("delaySeconds")
		if math.Trunc(delay) != delay {
			return httperr.BadRequest("延迟时间必须为整数秒")
		}
		if err := a.sandboxOrderAccess(r, body.Str("orderNo")); err != nil {
			return err
		}
		task, err := services.NewSandboxScenarioService(a.c.Store, a.c.Payment).Create(services.SandboxScenarioInput{OrderNo: body.Str("orderNo"), Scenario: body.Str("scenario"), DelaySeconds: int(delay), Amount: body.Num("amount")})
		if err != nil {
			return err
		}
		httpx.OK(w, task)
		return nil
	}))
	r.Get("/sandbox/scenarios", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		items, err := a.c.Store.SandboxTasks().ListScoped(queryParam(r, "orderNo"), httpx.ProjectScope(r.Context()))
		if err != nil {
			return err
		}
		httpx.OK(w, items)
		return nil
	}))
	r.Post("/sandbox/scenarios/{id}/cancel", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id := pathInt64(r, "id")
		task, err := a.c.Store.SandboxTasks().Find(id)
		if err != nil {
			return err
		}
		if task == nil {
			return httperr.NotFound("场景任务不存在")
		}
		if err := a.sandboxOrderAccess(r, task.OrderNo); err != nil {
			return err
		}
		task, err = services.NewSandboxScenarioService(a.c.Store, a.c.Payment).Cancel(id)
		if err != nil {
			return err
		}
		httpx.OK(w, task)
		return nil
	}))
}

func (a *App) sandboxOrderAccess(r *http.Request, orderNo string) error {
	scope := httpx.ProjectScope(r.Context())
	if scope == nil {
		return nil
	}
	order, err := a.c.Store.Orders.FindByOrderNo(orderNo, false)
	if err != nil {
		return err
	}
	if order != nil && order.GroupID != nil {
		for _, id := range scope {
			if id == *order.GroupID {
				return nil
			}
		}
	}
	return httperr.NotFound("订单不存在")
}

// RunSandboxTasks performs local state transitions only; callback delivery is
// handled by the existing notification dispatcher after its transaction commits.
func (c *Container) RunSandboxTasks() error {
	if !config.Bool(c.Config, "ENABLE_SANDBOX") {
		return nil
	}
	return services.NewSandboxScenarioService(c.Store, c.Payment).RunDue()
}
