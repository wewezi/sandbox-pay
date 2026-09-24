// 登录 / 会话 / 登出的 e2e。
//
// 覆盖 JWT 鉴权中间件、cookie 签发、Bearer 与 cookie 两种带 token 的方式。
// login 路由硬编码 5 次/分钟/IP，每个用例换 IP（和 TS 版一样手写）才不会互相挤掉。
package routes

import (
	"net/http"
	"strings"
	"testing"
)

func TestAuthE2E(t *testing.T) {
	h := newHarness(t, nil)

	t.Run("POST /auth/login 正确用户名密码签发 JWT，并写 HttpOnly cookie", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "POST",
			Path:    "/v1/api/auth/login",
			Body:    map[string]any{"username": testAdmin, "password": testAdminPass},
			Headers: map[string]string{"Content-Type": "application/json"},
			IP:      "10.20.0.1",
		})
		eq(t, "status", r.Status, http.StatusOK)
		b := r.obj()
		eq(t, "code", num(t, b["code"], "code"), 200)
		d := r.data()
		eq(t, "tokenType", str(t, d["tokenType"], "tokenType"), "Bearer")
		eq(t, "username", str(t, d["username"], "username"), testAdmin)
		token := str(t, d["token"], "token")
		eq(t, "token 段数", len(strings.Split(token, ".")), 3)

		cookie := r.Header.Get("Set-Cookie")
		hasSubstr(t, cookie, "sandbox_pay_access_token=", "Set-Cookie")
		hasSubstr(t, cookie, "HttpOnly", "Set-Cookie")
		hasSubstr(t, cookie, "SameSite=Lax", "Set-Cookie")
		hasSubstr(t, cookie, "Path=/", "Set-Cookie")
	})

	t.Run("错误密码返回 401，文案故意笼统", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "POST",
			Path:    "/v1/api/auth/login",
			Body:    map[string]any{"username": testAdmin, "password": "wrong-password"},
			Headers: map[string]string{"Content-Type": "application/json"},
			IP:      "10.20.0.2",
		})
		eq(t, "status", r.Status, http.StatusUnauthorized)
		eq(t, "message", str(t, r.obj()["message"], "message"), "用户名或密码错误")
	})

	t.Run("不存在的用户名也返回同一条 401，不泄漏账号是否存在", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "POST",
			Path:    "/v1/api/auth/login",
			Body:    map[string]any{"username": "no-such-user", "password": "whatever1"},
			Headers: map[string]string{"Content-Type": "application/json"},
			IP:      "10.20.0.3",
		})
		eq(t, "status", r.Status, http.StatusUnauthorized)
		eq(t, "message", str(t, r.obj()["message"], "message"), "用户名或密码错误")
	})

	t.Run("缺字段被校验层挡下", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "POST",
			Path:    "/v1/api/auth/login",
			Body:    map[string]any{"username": "ab"},
			Headers: map[string]string{"Content-Type": "application/json"},
			IP:      "10.20.0.4",
		})
		eq(t, "status", r.Status, http.StatusBadRequest)
	})

	t.Run("GET /auth/session 没带 token 返回 401", func(t *testing.T) {
		r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/auth/session"})
		eq(t, "status", r.Status, http.StatusUnauthorized)
	})

	t.Run("GET /auth/session 伪造 token 返回 401", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "GET",
			Path:    "/v1/api/auth/session",
			Headers: map[string]string{"Authorization": "Bearer not-a-jwt"},
		})
		eq(t, "status", r.Status, http.StatusUnauthorized)
	})

	t.Run("GET /auth/session Bearer 能过", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "GET",
			Path:    "/v1/api/auth/session",
			Headers: h.auth,
		})
		eq(t, "status", r.Status, http.StatusOK)
		eq(t, "username", str(t, r.data()["username"], "username"), testAdmin)
	})

	t.Run("GET /auth/session HttpOnly cookie 也能过（前端不读 token，靠 cookie）", func(t *testing.T) {
		_, cookie := h.login(t, "10.20.0.5")
		r := h.do(t, reqSpec{
			Method:  "GET",
			Path:    "/v1/api/auth/session",
			Headers: map[string]string{"Cookie": cookie},
		})
		eq(t, "status", r.Status, http.StatusOK)
		eq(t, "username", str(t, r.data()["username"], "username"), testAdmin)
	})

	t.Run("POST /auth/logout 清掉 cookie", func(t *testing.T) {
		_, cookie := h.login(t, "10.20.0.6")
		r := h.do(t, reqSpec{
			Method:  "POST",
			Path:    "/v1/api/auth/logout",
			Headers: map[string]string{"Cookie": cookie},
		})
		eq(t, "status", r.Status, http.StatusOK)
		cleared := r.Header.Get("Set-Cookie")
		hasSubstr(t, cleared, "sandbox_pay_access_token=", "登出 Set-Cookie")
		// 删除语义靠 Max-Age=0。Go 的 net/http 只在 Cookie.MaxAge<0 时才写它，
		// 传 0 会整个属性都不输出 —— sessionCookie 负责把 <=0 折成 -1。
		hasSubstr(t, cleared, "Max-Age=0", "登出 Set-Cookie")
	})
}
