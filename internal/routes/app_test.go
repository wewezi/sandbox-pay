// 应用外壳的 e2e：探针、错误信封、安全头、CORS、关联 ID、静态资源。
//
// 对应原 TS src/http/app.e2e.spec.ts —— 这些都是中间件链的行为，
// 必须真的走一遍 handler 才测得到。
package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sandbox-pay/internal/httpx"
)

func TestAppShell(t *testing.T) {
	h := newHarness(t, nil)

	t.Run("GET /health 不带版本前缀就能打到，返回 ok", func(t *testing.T) {
		r := h.do(t, reqSpec{Method: "GET", Path: "/health"})
		eq(t, "status", r.Status, http.StatusOK)

		var body map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(r.Raw)), &body); err != nil {
			t.Fatalf("health body 不是 JSON: %v (raw=%q)", err, r.Raw)
		}
		eq(t, "body 字段数", len(body), 1)
		eq(t, "status", str(t, body["status"], "status"), "ok")
	})

	t.Run("未命中的接口返回 404 信封而不是纯文本", func(t *testing.T) {
		r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/nope"})
		eq(t, "status", r.Status, http.StatusNotFound)
		b := r.obj()
		eq(t, "statusCode", num(t, b["statusCode"], "statusCode"), 404)
		eq(t, "code", num(t, b["code"], "code"), 404)
		// TS 版 msg 是 'Not Found'；Go 的 httpx.WriteNotFound 把 message 和 msg
		// 都写成 "Cannot <METHOD> <path>"（Go 侧字面量为准，见最终报告）。
		eq(t, "msg", str(t, b["msg"], "msg"), "Cannot GET /v1/api/nope")
		eq(t, "path", str(t, b["path"], "path"), "/v1/api/nope")
		// 前端 pickMessage 同时看 message 和 msg，两个都得有
		if _, ok := b["message"].(string); !ok {
			t.Fatalf("message 不是字符串: %#v", b["message"])
		}
	})

	t.Run("业务错误带上 correlationId 和 path", func(t *testing.T) {
		r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/stats"})
		eq(t, "status", r.Status, http.StatusUnauthorized)
		b := r.obj()
		eq(t, "statusCode", num(t, b["statusCode"], "statusCode"), 401)
		eq(t, "path", str(t, b["path"], "path"), "/v1/api/admin/stats")
		cid, ok := b["correlationId"].(string)
		if !ok || cid == "" {
			t.Fatalf("correlationId 不是非空字符串: %#v", b["correlationId"])
		}
	})

	t.Run("非 HttpError 一律 500，且不把内部原文吐给客户端", func(t *testing.T) {
		// 拿真的 httpx.H（错误处理器的 500 分支）挂一个会抛的 handler ——
		// 没法从正常业务路径稳定触发，但它的行为（不泄漏原文）值得钉住。
		probe := httpx.Correlation(httpx.H(func(w http.ResponseWriter, r *http.Request) error {
			return errors.New("DB password is hunter2")
		}))
		rec := httptest.NewRecorder()
		probe.ServeHTTP(rec, httptest.NewRequest("GET", "/boom", nil))
		r := respFromRecorder(t, rec)

		eq(t, "status", r.Status, http.StatusInternalServerError)
		eq(t, "message", str(t, r.obj()["message"], "message"), "Internal server error")
		lacksSubstr(t, r.Raw, "hunter2", "500 响应体")
	})

	t.Run("关联 ID：请求没带就生成一个并回写", func(t *testing.T) {
		r := h.do(t, reqSpec{Method: "GET", Path: "/health"})
		if r.Header.Get("X-Correlation-ID") == "" {
			t.Fatal("响应缺少生成的 X-Correlation-ID")
		}
	})

	t.Run("关联 ID：请求带了就沿用同一个", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "GET",
			Path:    "/health",
			Headers: map[string]string{"X-Correlation-ID": "trace-me-123"},
		})
		eq(t, "X-Correlation-ID", r.Header.Get("X-Correlation-ID"), "trace-me-123")
	})

	t.Run("11 个安全头都在，CSP 不含 unsafe-inline", func(t *testing.T) {
		r := h.do(t, reqSpec{Method: "GET", Path: "/health"})
		for _, key := range []string{
			"Content-Security-Policy",
			"Cross-Origin-Resource-Policy",
			"Origin-Agent-Cluster",
			"Referrer-Policy",
			"Strict-Transport-Security",
			"X-Content-Type-Options",
			"X-DNS-Prefetch-Control",
			"X-Download-Options",
			"X-Frame-Options",
			"X-Permitted-Cross-Domain-Policies",
			"X-XSS-Protection",
		} {
			if r.Header.Get(key) == "" {
				t.Errorf("安全头缺失: %s", key)
			}
		}
		// 静态页是按「无内联」写的，放开等于白扔这条约束
		lacksSubstr(t, r.Header.Get("Content-Security-Policy"), "unsafe-inline", "CSP")
	})

	t.Run("CORS：放行 localhost 的预检", func(t *testing.T) {
		origin := "http://localhost:5173"
		r := h.do(t, reqSpec{
			Method:  "OPTIONS",
			Path:    "/v1/api/admin/stats",
			Headers: map[string]string{"Origin": origin},
		})
		eq(t, "status", r.Status, http.StatusNoContent)
		eq(t, "ACAO", r.Header.Get("Access-Control-Allow-Origin"), origin)
		eq(t, "Allow-Credentials", r.Header.Get("Access-Control-Allow-Credentials"), "true")
	})

	t.Run("CORS：挡掉不在白名单里的 Origin", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "OPTIONS",
			Path:    "/v1/api/admin/stats",
			Headers: map[string]string{"Origin": "https://evil.example.com"},
		})
		eq(t, "status", r.Status, http.StatusForbidden)
	})

	t.Run("CORS：非预检请求不给不在白名单的 Origin 回 ACAO", func(t *testing.T) {
		r := h.do(t, reqSpec{
			Method:  "GET",
			Path:    "/health",
			Headers: map[string]string{"Origin": "https://evil.example.com"},
		})
		eq(t, "ACAO", r.Header.Get("Access-Control-Allow-Origin"), "")
	})

	t.Run("静态资源带 Cache-Control: no-cache，让浏览器每次回源验证", func(t *testing.T) {
		// 少了这个头，浏览器只看到 last-modified 就按启发式自己定过期，
		// 改完前端刷新看不到新的（得手动硬刷）。README 承诺「改完刷新即可」。
		if !h.staticOK {
			t.Fatal("web/dist 目录不存在，请先运行 npm run build")
		}
		r := h.do(t, reqSpec{Method: "GET", Path: "/app.css"})
		eq(t, "status", r.Status, http.StatusOK)
		eq(t, "Cache-Control", r.Header.Get("Cache-Control"), "no-cache")
	})
}
