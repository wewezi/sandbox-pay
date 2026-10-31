package routes

import (
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/httpx"
	"sandbox-pay/internal/validate"
)

var issueTokenSchema = validate.Schema{
	"name":    validate.OptStr(0, 100),
	"groupId": validate.OptNum(1, 9007199254740991),
}

var createProjectSchema = validate.Schema{"name": validate.Str(1, 100)}

func (a *App) registerTokens(r chi.Router) {
	r.Get("/api-keys", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		var groupID *int64
		if raw := strings.TrimSpace(r.URL.Query().Get("groupId")); raw != "" {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || id <= 0 || id > 9007199254740991 {
				return httperr.BadRequest("groupId 必须是正整数")
			}
			groupID = &id
		}
		out, err := a.c.Groups.ListTokens(groupID, httpx.ProjectScope(r.Context()))
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}))

	r.Post("/api-keys", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		body, err := validate.Check(httpx.JSONBody(r), issueTokenSchema)
		if err != nil {
			return err
		}
		var groupID *int64
		if body.Has("groupId") {
			value := body.Num("groupId")
			if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
				return httperr.BadRequest("groupId 必须是正整数")
			}
			id := int64(value)
			groupID = &id
		}
		out, err := a.c.Groups.IssueToken(groupID, body.OptStr("name"))
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}))

	r.Post("/projects", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		body, err := validate.Check(httpx.JSONBody(r), createProjectSchema)
		if err != nil {
			return err
		}
		name := strings.TrimSpace(body.Str("name"))
		if name == "" {
			return httperr.BadRequest("项目名称不能为空")
		}
		out, err := a.c.Groups.CreateProject(name)
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}))

	r.Get("/groups/{id}/webhook-secret", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		out, err := a.c.Groups.WebhookSecret(pathInt64(r, "id"))
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}))
}
