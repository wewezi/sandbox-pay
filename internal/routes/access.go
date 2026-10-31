package routes

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"sandbox-pay/internal/apiurl"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/httpx"
)

// adminAccess runs after authentication and leaf routing, so resource checks
// use the matched route and its parameters rather than interpreting the URL.
func (a *App) adminAccess() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return httpx.H(func(w http.ResponseWriter, r *http.Request) error {
			ids, err := a.allowedProjectIDs(r)
			if err != nil {
				return err
			}
			r = r.WithContext(httpx.WithProjectScope(r.Context(), ids))
			if httpx.User(r.Context()).Role == db.RoleSuperAdmin {
				next.ServeHTTP(w, r)
				return nil
			}
			if err := a.checkAdminResource(r, ids); err != nil {
				return err
			}
			next.ServeHTTP(w, r)
			return nil
		})
	}
}

func memberProject(ids []int64, id int64) bool {
	for _, allowed := range ids {
		if id == allowed {
			return true
		}
	}
	return false
}

func (a *App) memberOrder(ids []int64, orderNo string) error {
	order, err := a.c.Store.Orders.FindByOrderNo(orderNo, false)
	if err != nil {
		return err
	}
	if order == nil || order.GroupID == nil || !memberProject(ids, *order.GroupID) {
		return httperr.NotFound("订单不存在")
	}
	return nil
}

func (a *App) memberKey(ids []int64, id int64) error {
	key, err := a.c.Store.ApiKeys.FindByID(id)
	if err != nil {
		return err
	}
	if key == nil || !memberProject(ids, key.GroupID) {
		return httperr.NotFound("令牌不存在")
	}
	return nil
}

func accessID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 || id > 9007199254740991 {
		return 0, httperr.BadRequest("ID 必须是正整数")
	}
	return id, nil
}

func inspectAdminBody(r *http.Request) (map[string]any, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return map[string]any{}, nil
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil {
		return nil, httperr.BadRequest("无法读取请求体")
	}
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(data))
	if len(data) > 1<<20 {
		return nil, httperr.BadRequest("请求体过大")
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var body map[string]any
	if json.Unmarshal(data, &body) != nil || body == nil {
		return nil, httperr.BadRequest("请求体必须是 JSON 对象")
	}
	return body, nil
}

func bodyAccessID(body map[string]any, name string) (int64, bool, error) {
	raw, exists := body[name]
	if !exists {
		return 0, false, nil
	}
	id, ok := raw.(float64)
	if !ok || math.IsNaN(id) || math.IsInf(id, 0) || id < 1 || id > 9007199254740991 || math.Trunc(id) != id {
		return 0, true, httperr.BadRequest(name + " 必须是正整数")
	}
	return int64(id), true, nil
}

func (a *App) checkAdminResource(r *http.Request, ids []int64) error {
	path := strings.TrimPrefix(chi.RouteContext(r.Context()).RoutePattern(), apiurl.Prefix)
	if path == "/admin/developer" {
		return nil
	}
	if strings.HasPrefix(path, "/admin/team/") || strings.HasPrefix(path, "/admin/operations/") || path == "/admin/audit-logs" || strings.HasPrefix(path, "/admin/audit-logs/") || path == "/admin/reset-data" {
		return httperr.Forbidden("此操作仅允许超级管理员执行")
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && (path == "/admin/projects" || path == "/admin/groups") {
		return httperr.Forbidden("仅超级管理员可以创建项目")
	}
	query := r.URL.Query()
	if raw := query.Get("groupId"); raw != "" {
		id, err := accessID(raw)
		if err != nil {
			return err
		}
		if !memberProject(ids, id) {
			return httperr.NotFound("项目不存在")
		}
	}
	if raw := query.Get("apiKeyId"); raw != "" {
		id, err := accessID(raw)
		if err != nil {
			return err
		}
		if err := a.memberKey(ids, id); err != nil {
			return err
		}
	}
	if orderNo := query.Get("orderNo"); orderNo != "" {
		if err := a.memberOrder(ids, orderNo); err != nil {
			return err
		}
	}
	switch path {
	case "/admin/stats", "/admin/dashboard", "/admin/test-pay", "/admin/refund",
		"/admin/groups", "/admin/api-keys", "/admin/transactions", "/admin/notifications",
		"/admin/usage-logs", "/admin/sandbox/scenarios", "/native-pay/confirm":
	case "/admin/groups/{id}/toggle", "/admin/groups/{id}/keys", "/admin/groups/{id}/webhook-secret":
		id, err := accessID(pathParam(r, "id"))
		if err != nil {
			return err
		}
		if !memberProject(ids, id) {
			return httperr.NotFound("项目不存在")
		}
	case "/admin/api-keys/{id}/toggle":
		id, err := accessID(pathParam(r, "id"))
		if err != nil {
			return err
		}
		if err := a.memberKey(ids, id); err != nil {
			return err
		}
	case "/admin/transactions/{orderNo}":
		if err := a.memberOrder(ids, pathParam(r, "orderNo")); err != nil {
			return err
		}
	case "/admin/notifications/{id}/replay", "/admin/notifications/{id}/detail", "/admin/notifications/{id}/simulate":
		id, err := accessID(pathParam(r, "id"))
		if err != nil {
			return err
		}
		notification, err := a.c.Store.Notifies.FindByID(id)
		if err != nil {
			return err
		}
		if notification == nil || notification.OrderNo == nil {
			return httperr.NotFound("通知不存在")
		}
		if err := a.memberOrder(ids, *notification.OrderNo); err != nil {
			return err
		}
	case "/admin/usage-logs/{id}":
		id, err := accessID(pathParam(r, "id"))
		if err != nil {
			return err
		}
		log, err := a.c.Store.UsageLogs.FindByID(id)
		if err != nil {
			return err
		}
		if log == nil || log.GroupID == nil || !memberProject(ids, *log.GroupID) {
			return httperr.NotFound("使用日志不存在")
		}
	case "/admin/sandbox/scenarios/{id}/cancel":
		id, err := accessID(pathParam(r, "id"))
		if err != nil {
			return err
		}
		task, err := a.c.Store.SandboxTasks().Find(id)
		if err != nil {
			return err
		}
		if task == nil {
			return httperr.NotFound("场景任务不存在")
		}
		if err := a.memberOrder(ids, task.OrderNo); err != nil {
			return err
		}
	default:
		return httperr.Forbidden("此管理接口仅允许超级管理员访问")
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodDelete {
		return nil
	}
	body, err := inspectAdminBody(r)
	if err != nil {
		return err
	}
	if orderNo, ok := body["orderNo"].(string); ok && orderNo != "" {
		if err := a.memberOrder(ids, orderNo); err != nil {
			return err
		}
	}
	groupID, hasGroup, err := bodyAccessID(body, "groupId")
	if err != nil {
		return err
	}
	if hasGroup && !memberProject(ids, groupID) {
		return httperr.NotFound("项目不存在")
	}
	keyID, hasKey, err := bodyAccessID(body, "apiKeyId")
	if err != nil {
		return err
	}
	if hasKey {
		if err := a.memberKey(ids, keyID); err != nil {
			return err
		}
	}
	if (path == "/admin/test-pay" && !hasKey) || (path == "/admin/api-keys" && !hasGroup) {
		group, err := a.c.Groups.DefaultGroup()
		if err != nil {
			return err
		}
		if !memberProject(ids, group.ID) {
			return httperr.Forbidden("无权使用默认项目，请明确选择已加入的项目")
		}
	}
	return nil
}
