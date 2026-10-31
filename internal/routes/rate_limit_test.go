// 全局限流的 e2e。
//
// THROTTLE_LIMIT 在这个 harness 里压到 3（TS 版也是单独一个 spec 文件干这事）。
// httpx.DefaultLimiter 是包级单例：这里手写的 IP（10.9.x / 10.1.x / 10.20.x …）
// 和其他 spec 自动分配的 10.99.x 段错开，用例之间不会互相偷配额。
package routes

import (
	"fmt"
	"net/http"
	"testing"
)

func TestLoginIgnoresSpoofedForwardingHeaders(t *testing.T) {
	h := newHarness(t, nil)
	for i := 0; i < 6; i++ {
		r := h.do(t, reqSpec{
			Method: "POST", Path: "/v1/api/auth/login", IP: "10.98.50.1",
			Body:    map[string]any{"username": testAdmin, "password": "incorrect-password"},
			Headers: map[string]string{"X-Forwarded-For": fmt.Sprintf("1.2.3.%d", i+1)},
		})
		want := http.StatusUnauthorized
		if i == 5 {
			want = http.StatusTooManyRequests
		}
		eq(t, "status", r.Status, want)
	}
}

func TestGlobalRateLimit(t *testing.T) {
	h := newHarness(t, map[string]string{"THROTTLE_LIMIT": "3"})

	t.Run("超过窗口配额后返回 429", func(t *testing.T) {
		ip := "10.9.9.9"
		var codes []int
		for i := 0; i < 5; i++ {
			r := h.do(t, reqSpec{
				Method: "GET",
				Path:   "/v1/api/admin/stats",
				IP:     ip,
			})
			codes = append(codes, r.Status)
		}
		// 前 3 个放行（进到 requireAuth 拿 401），后面被限流挡住
		eqSlice(t, "codes[0:3]", codes[:3], []int{401, 401, 401})
		eqSlice(t, "codes[3:]", codes[3:], []int{429, 429})
	})

	t.Run("429 走的是统一错误信封", func(t *testing.T) {
		ip := "10.9.9.10"
		for i := 0; i < 3; i++ {
			h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/stats", IP: ip})
		}
		r := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/stats", IP: ip})
		eq(t, "status", r.Status, http.StatusTooManyRequests)
		b := r.obj()
		eq(t, "statusCode", num(t, b["statusCode"], "statusCode"), 429)
		hasSubstr(t, str(t, b["message"], "message"), "请求过于频繁", "429 message")
	})

	t.Run("按 IP 分桶，换 IP 不受影响", func(t *testing.T) {
		for i := 0; i < 4; i++ {
			h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/stats", IP: "10.1.1.1"})
		}
		other := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/stats", IP: "10.2.2.2"})
		eq(t, "other IP status", other.Status, http.StatusUnauthorized)
	})

	t.Run("/health 不在版本前缀下，不受全局限流影响", func(t *testing.T) {
		ip := "10.3.3.3"
		for i := 0; i < 7; i++ {
			r := h.do(t, reqSpec{Method: "GET", Path: "/health", IP: ip})
			eq(t, "health status", r.Status, http.StatusOK)
		}
	})
}
