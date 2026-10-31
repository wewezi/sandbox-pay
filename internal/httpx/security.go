package httpx

import (
	"net/http"
	"net/url"
	"strings"
)

// 安全响应头。
//
// 这 11 个头是照着 helmet 的实际输出一条条抄下来的（含它默认补的
// base-uri / font-src / frame-ancestors / script-src-attr），不是凭印象写的。
// 要删某一条之前先想清楚它挡的是什么。
//
// CSP 放宽的三处，都是收银台需要的：
//   - img-src     二维码走 data: URL，兜底走 api.qrserver.com
//   - form-action 支付宝返回的是一段自提交表单，POST 到支付宝网关
//   - script-src  只允许自家 .js，支付宝表单里的 <script> 在前端已被剥掉
//
// 别加 'unsafe-inline' —— 静态页是靠「无内联脚本、无内联 style」写的，
// 放开等于把这条约束白扔了。
const csp = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self'; " +
	"img-src 'self' data: https://api.qrserver.com; " +
	"connect-src 'self'; " +
	"form-action 'self' https:; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"font-src 'self' https: data:; " +
	"frame-ancestors 'self'; " +
	"script-src-attr 'none'"

var securityHeaders = map[string]string{
	"Content-Security-Policy":           csp,
	"Cross-Origin-Resource-Policy":      "same-origin",
	"Origin-Agent-Cluster":              "?1",
	"Referrer-Policy":                   "no-referrer",
	"Strict-Transport-Security":         "max-age=31536000; includeSubDomains",
	"X-Content-Type-Options":            "nosniff",
	"X-DNS-Prefetch-Control":            "off",
	"X-Download-Options":                "noopen",
	"X-Frame-Options":                   "SAMEORIGIN",
	"X-Permitted-Cross-Domain-Policies": "none",
	"X-XSS-Protection":                  "0",
}

// Correlation 关联 ID：请求带了就沿用，没带就生成，回一份给响应头。
//
// 必须在 handler 之前写头 —— net/http 的 ResponseWriter 在第一次 WriteHeader
// 之后就不再接受改动，跑完再补等于白补。
func Correlation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, id := WithCorrelation(r.Context(), r.Header.Get("X-Correlation-ID"))
		w.Header().Set("X-Correlation-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SecurityHeaders 写安全响应头。
// 故意不发 COOP/COEP：收银台要跳第三方网关再跳回来，收严了会挡住跳转。
//
// 在 handler 之前写：body 一旦写出去 header 就改不动了（见 Correlation）。
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range securityHeaders {
			w.Header().Set(k, v)
		}
		next.ServeHTTP(w, r)
	})
}

// IsCorsAllowed 只放行 localhost（任意端口）和 *.we29.cn。
func IsCorsAllowed(origin string) bool {
	// 没有 Origin 的请求放过（服务端调用、curl、移动端）
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" ||
		host == "127.0.0.1" ||
		strings.HasSuffix(host, ".we29.cn") ||
		host == "we29.cn"
}

const (
	allowedMethods = "GET, POST, PUT, DELETE, PATCH, OPTIONS"
	allowedHeaders = "Content-Type, Authorization, X-Request-Id"
)

// CORS 预检直接回，不进业务 handler。
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := IsCorsAllowed(origin)

		if r.Method == http.MethodOptions {
			if !allowed {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
			w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// 同样要赶在 handler 写 body 之前，否则这两个头进不了响应。
		if allowed && origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		next.ServeHTTP(w, r)
	})
}
