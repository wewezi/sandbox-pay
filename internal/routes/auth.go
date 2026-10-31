package routes

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httpx"
	"sandbox-pay/internal/services"
	"sandbox-pay/internal/validate"
)

// 请求校验规则。未声明的字段由 validate 拒绝。
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
		"channel": validate.StrIn("alipay", "paypal", "native"),
	}

	testPaySchema = validate.Schema{
		"amount":          validate.Num(0.01, 0),
		"productName":     validate.OptStr(0, 200),
		"payMethod":       validate.OptStrIn("native", "alipay", "paypal"),
		"expireMinutes":   validate.OptNum(1, 120),
		"externalOrderNo": validate.OptStr(0, 200),
		"returnUrl":       validate.OptURLRule(),
		"notifyUrl":       validate.OptURLRule(),
		"apiKeyId":        validate.OptNum(1, 9007199254740991),
	}

	refundSchema = validate.Schema{
		"orderNo":   validate.Str(0, 0),
		"amount":    validate.Num(0.01, 0),
		"reason":    validate.OptStr(0, 255),
		"requestId": validate.OptStr(1, 64),
	}
)

// registerAuth 登录和会话管理；登录独立限制为每分钟 5 次。
func (a *App) registerAuth(r chi.Router) {
	r.Route("/auth", func(r chi.Router) {
		r.With(httpx.RateLimit("login", 5, time.Minute)).Post("/login", httpx.H(a.login()))
		r.Post("/logout", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
			if err := a.c.Auth.RevokeToken(httpx.RequestToken(r)); err != nil {
				return err
			}
			http.SetCookie(w, sessionCookie(a.cfg, "", 0))
			httpx.OK(w, map[string]any{"message": "Logged out"})
			return nil
		}))
		r.Group(func(r chi.Router) {
			r.Use(a.authMiddleware())
			r.Get("/session", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
				user := httpx.User(r.Context())
				defaultAllowed := user.Role == db.RoleSuperAdmin
				if !defaultAllowed {
					ids, err := a.allowedProjectIDs(r)
					if err != nil {
						return err
					}
					group, err := a.c.Store.Groups.FindFirst()
					if err != nil {
						return err
					}
					defaultAllowed = group != nil && memberProject(ids, group.ID)
				}
				httpx.OK(w, map[string]any{"userId": user.UserID, "username": user.Username, "role": user.Role, "sandboxDefaultProjectAllowed": defaultAllowed})
				return nil
			}))
			a.registerSessions(r)
		})
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
		result, err := a.c.Auth.IssueLocalToken(user, services.SessionMetadata{ClientIP: httpx.ClientIP(r), UserAgent: r.UserAgent()})
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
// session cookie 留在浏览器里，而不是被删掉。
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
