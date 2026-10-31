package routes

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/httpx"
	"sandbox-pay/internal/services"
)

func requestUserID(r *http.Request) int64 {
	user := httpx.User(r.Context())
	if user == nil {
		return 0
	}
	switch id := user.UserID.(type) {
	case int64:
		return id
	case json.Number:
		n, _ := id.Int64()
		return n
	case float64:
		if id == math.Trunc(id) {
			return int64(id)
		}
	}
	return 0
}

func (a *App) authMiddleware() func(http.Handler) http.Handler {
	return httpx.RequireAuth(a.cfg, func(payload map[string]any) (*httpx.JwtUser, error) {
		user, _, err := a.c.Auth.ValidateSession(payload)
		if err != nil {
			return nil, err
		}
		return &httpx.JwtUser{UserID: json.Number(strconv.FormatInt(user.ID, 10)), Username: user.Username, Role: user.Role}, nil
	})
}

func (a *App) allowedProjectIDs(r *http.Request) ([]int64, error) {
	user := httpx.User(r.Context())
	if user == nil {
		return nil, httperr.Unauthorized()
	}
	if user.Role == db.RoleSuperAdmin {
		return nil, nil
	}
	return a.c.Store.Users.ProjectIDs(requestUserID(r))
}

func teamBody(r *http.Request, fields ...string) (map[string]any, error) {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	var body map[string]any
	if err := decoder.Decode(&body); err != nil || body == nil {
		return nil, httperr.BadRequest("请求体必须是 JSON 对象")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, httperr.BadRequest("请求体包含多余内容")
	}
	for name := range body {
		allowed := false
		for _, field := range fields {
			if name == field {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, httperr.BadRequest("包含未支持的字段：" + name)
		}
	}
	return body, nil
}

func requiredText(body map[string]any, field string) (string, error) {
	text, ok := body[field].(string)
	if !ok || text == "" {
		return "", httperr.BadRequest("请填写 " + field)
	}
	return text, nil
}

func (a *App) registerTeam(r chi.Router) {
	team := services.NewTeamService(a.c.Store)
	r.Get("/team/users", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		out, err := team.ListUsers(requestUserID(r))
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}))
	r.Post("/team/users", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		body, err := teamBody(r, "username", "password", "role")
		if err != nil {
			return err
		}
		name, err := requiredText(body, "username")
		if err != nil {
			return err
		}
		password, err := requiredText(body, "password")
		if err != nil {
			return err
		}
		role := db.RoleAdmin
		if value, exists := body["role"]; exists {
			var ok bool
			role, ok = value.(string)
			if !ok {
				return httperr.BadRequest("角色无效")
			}
		}
		out, err := team.CreateUser(requestUserID(r), name, password, role)
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}))
	r.Patch("/team/users/{id}", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		body, err := teamBody(r, "role", "isActive")
		if err != nil {
			return err
		}
		var role *string
		var active *bool
		if value, exists := body["role"]; exists {
			text, ok := value.(string)
			if !ok {
				return httperr.BadRequest("角色无效")
			}
			role = &text
		}
		if value, exists := body["isActive"]; exists {
			enabled, ok := value.(bool)
			if !ok {
				return httperr.BadRequest("isActive 必须是布尔值")
			}
			active = &enabled
		}
		if role == nil && active == nil {
			return httperr.BadRequest("请选择要修改的字段")
		}
		if err := team.SetAccess(requestUserID(r), pathInt64(r, "id"), role, active); err != nil {
			return err
		}
		httpx.OK(w, map[string]bool{"success": true})
		return nil
	}))
	r.Post("/team/users/{id}/password", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		body, err := teamBody(r, "password")
		if err != nil {
			return err
		}
		password, err := requiredText(body, "password")
		if err != nil {
			return err
		}
		if err := team.ResetPassword(requestUserID(r), pathInt64(r, "id"), password); err != nil {
			return err
		}
		httpx.OK(w, map[string]bool{"success": true})
		return nil
	}))
	r.Put("/team/users/{id}/projects", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		body, err := teamBody(r, "groupIds")
		if err != nil {
			return err
		}
		values, ok := body["groupIds"].([]any)
		if !ok {
			return httperr.BadRequest("groupIds 必须是数组")
		}
		ids := make([]int64, 0, len(values))
		for _, value := range values {
			id, ok := value.(float64)
			if !ok || math.IsNaN(id) || math.IsInf(id, 0) || id < 1 || id > 9007199254740991 || math.Trunc(id) != id {
				return httperr.BadRequest("项目 ID 无效")
			}
			ids = append(ids, int64(id))
		}
		if err := team.SetProjects(requestUserID(r), pathInt64(r, "id"), ids); err != nil {
			return err
		}
		httpx.OK(w, map[string]bool{"success": true})
		return nil
	}))
}

func (a *App) registerSessions(r chi.Router) {
	r.Get("/sessions", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		out, err := a.c.Auth.Sessions(requestUserID(r), httpx.SessionID(r.Context()))
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}))
	r = r.With(a.captureAdminAudit())
	r.Post("/sessions/revoke-others", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		if err := a.c.Store.Users.RevokeOtherSessions(requestUserID(r), httpx.SessionID(r.Context())); err != nil {
			return err
		}
		httpx.OK(w, map[string]bool{"success": true})
		return nil
	}))
	r.Post("/sessions/{id}/revoke", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		if err := a.c.Store.Users.RevokeSession(pathParam(r, "id"), requestUserID(r)); err != nil {
			return err
		}
		httpx.OK(w, map[string]bool{"success": true})
		return nil
	}))
	r.Post("/password", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		body, err := teamBody(r, "currentPassword", "newPassword")
		if err != nil {
			return err
		}
		current, err := requiredText(body, "currentPassword")
		if err != nil {
			return err
		}
		next, err := requiredText(body, "newPassword")
		if err != nil {
			return err
		}
		if err := a.c.Auth.ChangePassword(requestUserID(r), current, next); err != nil {
			return err
		}
		http.SetCookie(w, sessionCookie(a.cfg, "", 0))
		httpx.OK(w, map[string]bool{"success": true})
		return nil
	}))
}
