package httpx

import (
	"net/http"
	"strings"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/jwt"
)

// ACCESSCOOKIE 会话 cookie 名。前端不读它（HttpOnly），只有服务端认。
const AccessCookie = "sandbox_pay_access_token"

// bearerToken keeps the credential opaque while accepting case-insensitive
// authentication schemes, as required by HTTP.
func bearerToken(auth string) string {
	parts := strings.Fields(auth)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

// APIKeyLookupRepo RequireApiKey 用到的 repo 方法。
type APIKeyLookupRepo interface {
	FindActiveByKey(key string) (*db.ActiveKeyHit, error)
	Touch(id int64) error
}

// userFromPayload 从 JWT payload 组装 JwtUser（sub/username/role）。
func userFromPayload(payload map[string]any) *JwtUser {
	u := &JwtUser{}
	if sub, ok := payload["sub"]; ok {
		u.UserID = sub
	}
	if name, ok := payload["username"].(string); ok {
		u.Username = name
	}
	if role, ok := payload["role"].(string); ok {
		u.Role = role
	}
	return u
}

// RequireAuth JWT 会话校验。token 从 Authorization: Bearer 或 HttpOnly cookie 里取，两者都认。
type SessionValidator func(map[string]any) (*JwtUser, error)

func RequestToken(r *http.Request) string {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		if cookie, err := r.Cookie(AccessCookie); err == nil {
			token = cookie.Value
		}
	}
	return token
}

func RequireAuth(cfg config.Config, validators ...SessionValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := RequestToken(r)
			if token == "" {
				writeError(w, r, httperr.Unauthorized())
				return
			}

			payload := jwt.Verify(token, cfg.Get("JWT_SECRET"))
			if payload == nil {
				writeError(w, r, httperr.Unauthorized())
				return
			}

			user := userFromPayload(payload)
			if len(validators) != 0 {
				var err error
				user, err = validators[0](payload)
				if err != nil {
					writeError(w, r, err)
					return
				}
				if user == nil {
					writeError(w, r, httperr.Unauthorized())
					return
				}
			}
			ctx := WithUser(r.Context(), user)
			if id, ok := payload["jti"].(string); ok {
				ctx = WithSessionID(ctx, id)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireSandbox 沙箱开关。ENABLE_SANDBOX 不为 true 时挡掉演示接口。
func RequireSandbox(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !config.Bool(cfg, "ENABLE_SANDBOX") {
				writeError(w, r, httperr.Forbidden("沙箱接口已关闭"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAPIKey 接入方 API Key 校验：Authorization: Bearer <key>。
//
// 入站不验签 —— 一个 Key 直接调，curl / Postman / 任意语言都不用先写签名逻辑。
// 安全性靠 HTTPS 保证传输，和 Stripe / OpenAI 那类 API 一个路子。
//
// 一个分组可以有多把 Key，查出来的那把连同分组一起挂到 context 上：
// 分组用来做数据隔离（订单归属、查单过滤），Key 记录调用归属。
// 回调签名由分组独立的 WebhookSecret 负责，和入站凭证分开。
func RequireAPIKey(repo APIKeyLookupRepo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawKey := bearerToken(r.Header.Get("Authorization"))
			if rawKey == "" {
				writeError(w, r, httperr.Unauthorized("缺少 API Key，请带上 Authorization: Bearer <apiKey>"))
				return
			}

			hit, err := repo.FindActiveByKey(rawKey)
			if err != nil {
				writeError(w, r, err)
				return
			}
			if hit == nil {
				writeError(w, r, httperr.Unauthorized("无效的 API Key，或该 Key / 分组已停用"))
				return
			}

			// 刷新 lastUsedAt，失败不影响鉴权
			_ = repo.Touch(hit.Key.ID)
			BindUsageAPIKey(r.Context(), hit.Group, hit.Key)

			ctx := WithGroup(r.Context(), hit.Group)
			ctx = WithAPIKey(ctx, hit.Key)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
