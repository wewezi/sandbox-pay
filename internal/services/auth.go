package services

import (
	"sync"

	"golang.org/x/crypto/bcrypt"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/jwt"
)

// AuthService 本地账号密码登录 + 会话 JWT。
//
// 没有第三方 IdP：管理员账号就是 users 表里的一行，密码是 bcrypt hash。
type AuthService struct {
	store *db.Store
	cfg   config.Config
}

// NewAuthService 构造。
func NewAuthService(store *db.Store, cfg config.Config) *AuthService {
	return &AuthService{store: store, cfg: cfg}
}

// TokenResult 登录成功的回执。expiresInSeconds 和 cookie 的 maxAge 保持一致。
type TokenResult struct {
	Token            string `json:"token"`
	TokenType        string `json:"tokenType"`
	Username         string `json:"username"`
	Role             string `json:"role"`
	ExpiresInSeconds int    `json:"expiresInSeconds"`
}

var (
	dummyHashOnce  sync.Once
	dummyHashValue string
)

// dummyHash 一个恒定的真实 bcrypt hash。
//
// 用户不存在时也得跑一次 compare，让耗时保持一致，不通过响应时间泄漏
// 账号是否存在。用 sync.Once 惰性生成，避免启动就付一次 bcrypt 的钱。
func dummyHash() string {
	dummyHashOnce.Do(func() {
		h, err := bcrypt.GenerateFromPassword([]byte("sandbox-pay-timing-equalizer"), bcrypt.DefaultCost)
		if err != nil {
			panic(err) // 熵源/bcrypt 挂了没有降级方案
		}
		dummyHashValue = string(h)
	})
	return dummyHashValue
}

// ValidateUser 校验用户名密码。
//
// 任何不匹配都抛 401，消息故意写得笼统，免得被用来枚举有效用户名。
func (s *AuthService) ValidateUser(username, password string) (*db.User, error) {
	user, err := s.store.Users.FindByUsername(username)
	if err != nil {
		return nil, err
	}
	hash := dummyHash()
	if user != nil {
		hash = user.Password
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil || user == nil {
		return nil, httperr.Unauthorized("用户名或密码错误")
	}
	return user, nil
}

// IssueLocalToken 签一个本地 HS256 会话 JWT。
func (s *AuthService) IssueLocalToken(user *db.User) (*TokenResult, error) {
	secret := s.cfg.Get("JWT_SECRET")
	ttl := jwt.ParseExpiresIn(s.cfg.Get("JWT_EXPIRES_IN"))
	token := jwt.Sign(map[string]any{
		"sub":      user.ID,
		"username": user.Username,
		"role":     user.Role,
	}, secret, ttl)
	return &TokenResult{
		Token:            token,
		TokenType:        "Bearer",
		Username:         user.Username,
		Role:             user.Role,
		ExpiresInSeconds: ttl,
	}, nil
}
