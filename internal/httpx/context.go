// Package httpx HTTP 中间件层：请求上下文、统一信封、错误处理、守卫、限流、安全头。
//
// 对应原版 src/http/ 下那几个文件，Hono 的 Context 变量改成了 context.Context。
package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"sandbox-pay/internal/db"
)

type ctxKey int

const (
	keyCorrelation ctxKey = iota
	keyUser
	keyGroup
	keyKey
)

// JwtUser JWT 里取出来的用户信息，/auth/session 直接回给前端。
type JwtUser struct {
	// UserID 原样来自 JWT 的 sub（json.Number，回出去是个数字而不是 "1"）。
	UserID   any    `json:"userId"`
	Username string `json:"username,omitempty"`
	Role     string `json:"role,omitempty"`
}

// WithCorrelation 把关联 ID 放进 context。请求带了就沿用，没带就生成。
func WithCorrelation(parent context.Context, incoming string) (context.Context, string) {
	id := incoming
	if id == "" {
		id = newID()
	}
	return context.WithValue(parent, keyCorrelation, id), id
}

// CorrelationID 取当前请求的关联 ID（可能为空串）。
func CorrelationID(ctx context.Context) string {
	v, _ := ctx.Value(keyCorrelation).(string)
	return v
}

// WithUser RequireAuth 校验通过后写入。
func WithUser(ctx context.Context, u *JwtUser) context.Context {
	return context.WithValue(ctx, keyUser, u)
}

// User 取登录用户，没有则 nil。
func User(ctx context.Context) *JwtUser {
	u, _ := ctx.Value(keyUser).(*JwtUser)
	return u
}

// WithGroup RequireApiKey 校验通过后写入：Key 所属的分组。
func WithGroup(ctx context.Context, g *db.Group) context.Context {
	return context.WithValue(ctx, keyGroup, g)
}

// Group 取分组，没有则 nil。
func Group(ctx context.Context) *db.Group {
	g, _ := ctx.Value(keyGroup).(*db.Group)
	return g
}

// WithAPIKey RequireApiKey 校验通过后写入：本次请求用的那把 Key。
func WithAPIKey(ctx context.Context, k *db.APIKey) context.Context {
	return context.WithValue(ctx, keyKey, k)
}

// APIKey 取 Key，没有则 nil。
func APIKey(ctx context.Context) *db.APIKey {
	k, _ := ctx.Value(keyKey).(*db.APIKey)
	return k
}

func newID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}
