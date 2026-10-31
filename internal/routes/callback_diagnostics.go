package routes

import (
	"math"
	"net/http"

	"github.com/go-chi/chi/v5"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/httpx"
	"sandbox-pay/internal/validate"
)

var callbackSimulationSchema = validate.Schema{
	"mode":         validate.StrIn("duplicate", "delayed"),
	"delaySeconds": validate.OptNum(0, 3600),
}

func (a *App) registerCallbackDiagnostics(r chi.Router) {
	r.Get("/notifications/{id}/detail", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		out, err := a.c.Notify.GetNotificationDetail(pathInt64(r, "id"))
		if err != nil {
			return err
		}
		out.SimulationEnabled = config.Bool(a.cfg, "ENABLE_SANDBOX")
		httpx.OK(w, out)
		return nil
	}))
	r.With(httpx.RequireSandbox(a.cfg)).Post("/notifications/{id}/simulate", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		body, err := validate.Check(httpx.JSONBody(r), callbackSimulationSchema)
		if err != nil {
			return err
		}
		delay := body.Num("delaySeconds")
		if math.Trunc(delay) != delay {
			return httperr.BadRequest("延迟时间必须为整数秒")
		}
		out, err := a.c.Notify.SimulateNotification(pathInt64(r, "id"), body.Str("mode"), int(delay))
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}))
}
