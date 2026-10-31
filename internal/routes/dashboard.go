package routes

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/httpx"
)

func (a *App) registerDashboard(r chi.Router) {
	r.Get("/dashboard", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		days, err := dashboardInt(r, "days", 7)
		if err != nil {
			return err
		}
		utcOffset, err := dashboardInt(r, "utcOffset", 0)
		if err != nil {
			return err
		}
		out, err := a.c.Admin.GetDashboard(days, utcOffset, httpx.ProjectScope(r.Context()))
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}))
}

func dashboardInt(r *http.Request, name string, fallback int) (int, error) {
	raw, present := r.URL.Query()[name]
	if !present {
		return fallback, nil
	}
	if len(raw) != 1 {
		return 0, httperr.BadRequest(name + " 只能指定一次")
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw[0]))
	if err != nil {
		return 0, httperr.BadRequest(name + " 须为整数")
	}
	return value, nil
}

func parseUTCOffset(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < -720 || value > 840 {
		return 0, httperr.BadRequest("时区偏移须为 -720 至 840 之间的整数分钟")
	}
	return value, nil
}
