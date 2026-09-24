package routes

import (
	"net/http"
	"strings"
	"time"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/httpx"
	"sandbox-pay/internal/validate"
)

// 校验 schema。形状照原版 src/routes/*.routes.ts 的 *_SCHEMA，
// 未声明的字段会被 validate 直接报错（不是静默丢掉）。
var (
	loginSchema = validate.Schema{
		"username": validate.Str(3, 20),
		// bcrypt 最大有效长度 72
		"password": validate.Str(6, 72),
	}

	createGroupSchema = validate.Schema{
		"name": validate.Str(1, 100),
		// 第一把 Key 的备注名，不传就叫「默认」
		"keyName": validate.OptStr(0, 100),
	}

	issueKeySchema = validate.Schema{
		"name": validate.OptStr(0, 100),
	}

	confirmSchema = validate.Schema{
		"orderNo":    validate.Str(0, 0),
		"tradeNo":    validate.OptStr(0, 0),
		"walletUser": validate.Str(0, 0),
		"walletPass": validate.Str(0, 0),
	}

	sandboxConfirmSchema = validate.Schema{
		"orderNo":    validate.Str(0, 0),
		"walletUser": validate.OptStr(0, 0),
		"walletPass": validate.OptStr(0, 0),
	}

	switchChannelSchema = validate.Schema{
		"orderNo": validate.Str(0, 0),
		"channel": validate.StrIn("alipay", "paypal"),
	}

	testPaySchema = validate.Schema{
		"amount":      validate.Num(0.01, 0),
		"productName": validate.OptStr(0, 200),
	}

	refundSchema = validate.Schema{
		"orderNo": validate.Str(0, 0),
		"amount":  validate.Num(0.01, 0),
		"reason":  validate.OptStr(0, 255),
	}
)

// registerAuth /v1/api/auth。登录挂 5 次/分钟，挡暴力破解。
func (a *App) registerAuth() {
	a.api("POST "+apiPrefix("/auth/login"),
		a.login(),
		httpx.RateLimit("login", 5, time.Minute))

	a.api("GET "+apiPrefix("/auth/session"),
		func(w http.ResponseWriter, r *http.Request) error {
			httpx.OK(w, httpx.User(r.Context()))
			return nil
		},
		httpx.RequireAuth(a.cfg))

	a.api("POST "+apiPrefix("/auth/logout"), func(w http.ResponseWriter, r *http.Request) error {
		// 属性必须和登录时写一致（见 cookieAttrs），否则 clear 不掉。
		http.SetCookie(w, sessionCookie(a.cfg, "", 0))
		httpx.OK(w, map[string]any{"message": "Logged out"})
		return nil
	})
}

// login 登录：校验账号密码 → 签发 JWT → 同时回 cookie 和 body。
//
// 会话写成 HttpOnly cookie，浏览器自动带上；同时在 body 里回 token，
// 方便走 Bearer 的调用方。
func (a *App) login() httpx.Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		body, err := validate.Check(httpx.JSONBody(r), loginSchema)
		if err != nil {
			return err
		}
		user, err := a.c.Auth.ValidateUser(body.Str("username"), body.Str("password"))
		if err != nil {
			return err
		}
		result, err := a.c.Auth.IssueLocalToken(user)
		if err != nil {
			return err
		}
		http.SetCookie(w, sessionCookie(a.cfg, result.Token, result.ExpiresInSeconds))
		httpx.OK(w, map[string]any{
			"token":     result.Token,
			"tokenType": result.TokenType,
			"username":  result.Username,
			"role":      result.Role,
		})
		return nil
	}
}

// sessionCookie 会话 cookie 的属性，登录和登出必须写一致，否则 clear 不掉。
//
// maxAge <= 0 表示「删除」。这里必须折成 -1：Go 的 net/http 只在 MaxAge<0 时
// 才写 "Max-Age=0"，传 0 的话整个 Max-Age 属性都不会输出 —— 结果是个空值的
// session cookie 留在浏览器里，而不是被删掉。原版 cookieAttrs(0) 推的正是 Max-Age=0。
func sessionCookie(cfg config.Config, value string, maxAge int) *http.Cookie {
	if maxAge <= 0 {
		maxAge = -1
	}
	return &http.Cookie{
		Name:     httpx.AccessCookie,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   strings.EqualFold(strings.TrimSpace(cfg.Get("NODE_ENV")), "production"),
		MaxAge:   maxAge,
	}
}
