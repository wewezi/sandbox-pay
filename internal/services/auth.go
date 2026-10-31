package services

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

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
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil || user == nil || !user.IsActive {
		return nil, httperr.Unauthorized("用户名或密码错误")
	}
	return user, nil
}

// IssueLocalToken 签一个本地 HS256 会话 JWT。
type SessionMetadata struct {
	ClientIP  string
	UserAgent string
}

func (s *AuthService) IssueLocalToken(user *db.User, metadata ...SessionMetadata) (*TokenResult, error) {
	secret := s.cfg.Get("JWT_SECRET")
	ttl := jwt.ParseExpiresIn(s.cfg.Get("JWT_EXPIRES_IN"))
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return nil, err
	}
	id := hex.EncodeToString(buf[:])
	now := time.Now().UTC()
	meta := SessionMetadata{}
	if len(metadata) != 0 {
		meta = metadata[0]
	}
	if len(meta.UserAgent) > 500 {
		meta.UserAgent = meta.UserAgent[:500]
	}
	var current *db.User
	err := s.store.Do(func(tx *db.TxScope) error {
		var err error
		current, err = tx.Users.FindByID(user.ID)
		if err != nil {
			return err
		}
		if current == nil || !current.IsActive || current.Password != user.Password || current.SessionVersion != user.SessionVersion {
			return httperr.Unauthorized("账号状态已变更，请重新登录")
		}
		return tx.Users.InsertSession(&db.AdminSession{ID: id, UserID: current.ID, SessionVersion: current.SessionVersion, ClientIP: meta.ClientIP, UserAgent: meta.UserAgent, CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Duration(ttl) * time.Second)})
	})
	if err != nil {
		return nil, err
	}
	token := jwt.Sign(map[string]any{
		"sub": current.ID, "username": current.Username, "role": current.Role,
		"jti": id, "sv": current.SessionVersion,
	}, secret, ttl)
	return &TokenResult{
		Token:            token,
		TokenType:        "Bearer",
		Username:         current.Username,
		Role:             current.Role,
		ExpiresInSeconds: ttl,
	}, nil
}

func ClaimID(payload map[string]any, key string) int64 {
	if value, ok := payload[key].(json.Number); ok {
		n, err := value.Int64()
		if err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// ValidateSession rejects pre-upgrade JWTs and checks current account state on every request.
func (s *AuthService) ValidateSession(payload map[string]any) (*db.User, string, error) {
	userID, version := ClaimID(payload, "sub"), ClaimID(payload, "sv")
	id, _ := payload["jti"].(string)
	if userID == 0 || version == 0 || len(id) != 64 {
		return nil, "", httperr.Unauthorized("会话已失效，请重新登录")
	}
	var user *db.User
	err := s.store.Do(func(tx *db.TxScope) error {
		var err error
		user, err = tx.Users.FindByID(userID)
		if err != nil {
			return err
		}
		if user == nil || !user.IsActive || user.SessionVersion != version {
			return httperr.Unauthorized("账号状态已变更，请重新登录")
		}
		session, err := tx.Users.FindSession(id)
		if err != nil {
			return err
		}
		if session == nil || session.UserID != userID || session.SessionVersion != version || session.RevokedAt != nil || !session.ExpiresAt.After(time.Now()) {
			return httperr.Unauthorized("会话已退出，请重新登录")
		}
		return tx.Users.TouchSession(id)
	})
	return user, id, err
}

func (s *AuthService) RevokeToken(token string) error {
	payload := jwt.Verify(strings.TrimSpace(token), s.cfg.Get("JWT_SECRET"))
	if payload == nil {
		return nil
	}
	id, _ := payload["jti"].(string)
	if id == "" {
		return nil
	}
	return s.store.Users.RevokeSession(id, ClaimID(payload, "sub"))
}
