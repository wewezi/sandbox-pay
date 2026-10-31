package routes

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/httpx"
	"sandbox-pay/internal/services"
)

func usageFilterID(r *http.Request, name string) (*int64, error) {
	raw := queryParam(r, name)
	if raw == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 || id > 9007199254740991 {
		return nil, httperr.BadRequest(name + " 必须是正整数")
	}
	return &id, nil
}

func (a *App) registerUsageLogs(r chi.Router) {
	r.Get("/usage-logs", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		groupID, err := usageFilterID(r, "groupId")
		if err != nil {
			return err
		}
		keyID, err := usageFilterID(r, "apiKeyId")
		if err != nil {
			return err
		}
		utcOffset, err := parseUTCOffset(queryParam(r, "utcOffset"))
		if err != nil {
			return err
		}
		out, err := a.c.Usage.GetLogs(services.UsageLogOpts{
			ProjectIDs: httpx.ProjectScope(r.Context()),
			GroupID:    groupID, APIKeyID: keyID, UTCOffset: utcOffset,
			Page: queryParam(r, "page"), PageSize: queryParam(r, "pageSize"),
			Status: queryParam(r, "status"), Method: queryParam(r, "method"),
			Keyword: queryParam(r, "keyword"), DateFrom: queryParam(r, "dateFrom"), DateTo: queryParam(r, "dateTo"),
		})
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}))

	r.Get("/usage-logs/{id}", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		out, err := a.c.Usage.GetLog(pathInt64(r, "id"))
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}))
}
