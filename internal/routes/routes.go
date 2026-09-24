// Package routes 路由与中间件装配。
//
// 对应原版 src/app.ts + src/routes/*.routes.ts：把 Container 里的服务
// 挂到 net/http 的 ServeMux 上，中间件链在下面几处叠：
//
//	最外层  correlation → security headers → CORS
//	业务层  全局限流（/v1/api/*）→ 路由级限流/守卫 → handler
//
// handler 统一是 func(w, r) error，返回 error 即由 httpx.H 转成统一错误信封。
package routes

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	mux *http.ServeMux
	log *logger.Logger
	// 全局限流挂在这里，每条 /v1/api 路由都在最外层套一次
	// （同一个 name 计数，行为等价于「给整个前缀挂一个」）。
	globalLimit httpx.Middleware
}

// New 组装应用。
//
// 静态资源是 mux 的兜底（"/"），API 路由都比它更具体，会先被匹配到；
// 静态资源找不到文件时走统一信封 404，和原版 Hono 的 notFound 一致。
func New(cfg config.Config, c *Container, static http.Handler) http.Handler {
	a := &App{
		cfg: cfg,
		c:   c,
		mux: http.NewServeMux(),
		log: logger.New("App"),
		globalLimit: httpx.RateLimit(
			"global",
			int(config.Number(cfg, "THROTTLE_LIMIT")),
			time.Duration(config.Number(cfg, "THROTTLE_TTL_MS"))*time.Millisecond,
		),
	}

	a.registerAuth()
	a.registerAdmin()
	a.registerGateway()
	a.registerNativePay()
	a.registerAlipay()
	a.registerPayPal()

	// 探针，不走版本前缀 —— 探活不该被限流，也不该被业务鉴权挡住。
	a.mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, `{"status":"ok"}`+"\n")
	})

	// 兜底：找不到的路径先当静态资源找，找不到再回 404 信封。
	a.mux.Handle("/", static)

	return httpx.Chain(a.mux, httpx.Correlation, httpx.SecurityHeaders, httpx.CORS)
}

// api 挂一条 /v1/api 路由：先套全局限流，再套调用方给的中间件。
//
// 顺序和原版一致 —— 全局限流在最外层，路由级限流/守卫在它里面，
// 这样被限流挡住的请求根本不会走到守卫。
func (a *App) api(pattern string, h httpx.Handler, mws ...httpx.Middleware) {
	all := make([]httpx.Middleware, 0, len(mws)+1)
	all = append(all, a.globalLimit)
	all = append(all, mws...)
	a.mux.Handle(pattern, httpx.Chain(httpx.H(h), all...))
}

// plain 挂一条不套全局限流的 /v1/api 路由。
//
// 支付宝的 notify 可能在一秒内打好几次，和页面请求共用一个
// 60/分钟的桶会把回调误伤掉 —— 它自己有自己的验签兜底，不需要这层。
func (a *App) plain(pattern string, h httpx.Handler, mws ...httpx.Middleware) {
	a.mux.Handle(pattern, httpx.Chain(httpx.H(h), mws...))
}

// apiPrefix 业务路由统一前缀。
func apiPrefix(suffix string) string { return apiurl.Prefix + suffix }

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

// optInt64 数字型 query 参数：空值/解析不出来给 nil，别让 Number(”) 变成 0。
// 对应原版 `value ? Number(value) : undefined`。
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
	v, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		return -1
	}
	return v
}
