// Package routes 使用 chi 装配路由、权限分组和 HTTP 中间件。
package routes

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"sandbox-pay/internal/apiurl"
	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/httpx"
	"sandbox-pay/internal/logger"
	"sandbox-pay/internal/services"
)

// App 组装好的整套 HTTP 处理链。
type App struct {
	cfg config.Config
	c   *Container
	log *logger.Logger
}

// New 组装应用。网关、后台和登录接口的中间件按路由分组声明。
func New(cfg config.Config, c *Container, static http.Handler) http.Handler {
	a := &App{
		cfg: cfg,
		c:   c,
		log: logger.New("App"),
	}
	r := chi.NewRouter()
	r.Use(httpx.ProxyHeaders(cfg), httpx.Correlation, httpx.SecurityHeaders, httpx.CORS, middleware.GetHead)
	r.NotFound(static.ServeHTTP)
	r.MethodNotAllowed(methodNotAllowed(r))

	globalLimit := httpx.RateLimit(
		"global", int(config.Number(cfg, "THROTTLE_LIMIT")),
		time.Duration(config.Number(cfg, "THROTTLE_TTL_MS"))*time.Millisecond,
	)
	r.Route(apiurl.Prefix, func(r chi.Router) {
		r.NotFound(httpx.WriteNotFound)
		// 支付宝通知独立验签，不与页面及接入方请求共享限流配额。
		a.registerAlipay(r)
		r.Route("/gateway", func(r chi.Router) {
			r.Use(a.captureGatewayUsage(), globalLimit)
			a.registerGateway(r.With(httpx.RequireAPIKey(c.Store.ApiKeys)))
		})
		r.Group(func(r chi.Router) {
			r.Use(globalLimit)
			a.registerAuth(r)
			a.registerAdmin(r)
			a.registerNativePay(r)
			a.registerPayPal(r)
			a.registerDeveloper(r)
		})
	})

	// 探活直接检查数据库，不消耗业务请求的限流配额。
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := a.c.Store.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"status":"unavailable"}`+"\n")
			return
		}
		_, _ = io.WriteString(w, `{"status":"ok"}`+"\n")
	})

	return r
}

// methodNotAllowed 保持统一错误响应，并声明该地址支持的方法。
func methodNotAllowed(router chi.Router) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if r.URL.RawPath != "" {
			path = r.URL.RawPath
		}
		var allowed []string
		for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace} {
			matched := router.Match(chi.NewRouteContext(), method, path)
			if method == http.MethodHead && !matched {
				matched = router.Match(chi.NewRouteContext(), http.MethodGet, path)
			}
			if matched {
				allowed = append(allowed, method)
			}
		}
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		httpx.WriteMethodNotAllowed(w, r)
	}
}

// pathParam 读取 chi 参数，仅在路由匹配使用 RawPath 时解码一次。
func pathParam(r *http.Request, name string) string {
	value := chi.URLParam(r, name)
	if r.URL.RawPath != "" {
		decoded, err := url.PathUnescape(value)
		if err != nil {
			return ""
		}
		return decoded
	}
	return value
}

// callerOf 把守卫塞进 context 的「这次请求属于谁」取出来。
//
// RequireAPIKey 保证分组和 Key 都在；这里还是判一次 nil ——
// 路由写错守卫时宁可当匿名请求（分组 0、无 Key），也别 panic 成 500。
func callerOf(r *http.Request) services.OrderCaller {
	out := services.OrderCaller{}
	if g := httpx.Group(r.Context()); g != nil {
		out.GroupID = g.ID
	}
	if k := httpx.APIKey(r.Context()); k != nil {
		id := k.ID
		out.APIKeyID = &id
	}
	return out
}

// mustGroup 取 Key 所属分组。没有则说明守卫没挂，回 401 而不是 panic。
func mustGroup(r *http.Request) (*db.Group, error) {
	g := httpx.Group(r.Context())
	if g == nil {
		return nil, httperr.Unauthorized()
	}
	return g, nil
}

// actorOf 从 JWT 里解出一个稳定的操作者标识，只用于审计日志。
func actorOf(r *http.Request) string {
	u := httpx.User(r.Context())
	if u == nil {
		return "admin:admin"
	}
	if u.Username != "" {
		return "admin:" + u.Username
	}
	return "admin:admin"
}

// clientIP 客户端 IP，空串当「没拿到」（消费方要的是 undefined 语义）。
func clientIP(r *http.Request) *string {
	ip := httpx.ClientIP(r)
	if ip == "" {
		return nil
	}
	return &ip
}

// optInt64 读取整数查询参数，空值或非法值表示未指定过滤条件。
func optInt64(r *http.Request, key string) *int64 {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f != float64(int64(f)) {
		return nil // NaN / 非整数 → 过滤条件没意义，当没传
	}
	v := int64(f)
	return &v
}

// optStr query 参数，trim 后空串返回 nil。
func optStr(r *http.Request, key string) *string {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return nil
	}
	return &v
}

// pathInt64 取路径参数里的整数 id。解析不出来给 -1 —— 永远匹配不到行，
// 让 service 抛它自己的「记录不存在」，文案才对得上每个接口。
func pathInt64(r *http.Request, name string) int64 {
	v, err := strconv.ParseInt(pathParam(r, name), 10, 64)
	if err != nil {
		return -1
	}
	return v
}
