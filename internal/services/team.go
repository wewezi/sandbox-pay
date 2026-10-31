package services

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
)

type TeamService struct{ store *db.Store }

func NewTeamService(store *db.Store) *TeamService { return &TeamService{store: store} }

type TeamUser struct {
	ID        int64   `json:"id"`
	Username  string  `json:"username"`
	Role      string  `json:"role"`
	IsActive  bool    `json:"isActive"`
	GroupIDs  []int64 `json:"groupIds"`
	CreatedAt string  `json:"createdAt"`
}

func teamUser(user *db.User, groups []int64) TeamUser {
	return TeamUser{ID: user.ID, Username: user.Username, Role: user.Role, IsActive: user.IsActive, GroupIDs: groups, CreatedAt: JsTime(user.CreatedAt)}
}

func requireSuper(tx *db.TxScope, actorID int64) error {
	actor, err := tx.Users.FindByID(actorID)
	if err != nil {
		return err
	}
	if actor == nil || !actor.IsActive || actor.Role != db.RoleSuperAdmin {
		return httperr.Forbidden("仅超级管理员可以管理团队")
	}
	return nil
}

func (s *TeamService) ListUsers(actorID int64) ([]TeamUser, error) {
	out := make([]TeamUser, 0)
	err := s.store.Do(func(tx *db.TxScope) error {
		if err := requireSuper(tx, actorID); err != nil {
			return err
		}
		users, err := tx.Users.List()
		if err != nil {
			return err
		}
		for _, user := range users {
			ids, err := tx.Users.ProjectIDs(user.ID)
			if err != nil {
				return err
			}
			out = append(out, teamUser(user, ids))
		}
		return nil
	})
	return out, err
}

func validTeamPassword(password string) error {
	if utf8.RuneCountInString(password) < 8 {
		return httperr.BadRequest("密码至少需要 8 个字符")
	}
	if len(password) > 72 {
		return httperr.BadRequest("密码过长，请缩短后重试")
	}
	return nil
}

func (s *TeamService) CreateUser(actorID int64, username, password, role string) (*TeamUser, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 20 || strings.ContainsAny(username, "\r\n\t ") {
		return nil, httperr.BadRequest("用户名需要 3–20 字节，不能包含空白字符")
	}
	if role != db.RoleAdmin && role != db.RoleSuperAdmin {
		return nil, httperr.BadRequest("角色无效")
	}
	if err := validTeamPassword(password); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	var out TeamUser
	err = s.store.Do(func(tx *db.TxScope) error {
		if err := requireSuper(tx, actorID); err != nil {
			return err
		}
		old, err := tx.Users.FindByUsername(username)
		if err != nil {
			return err
		}
		if old != nil {
			return httperr.Conflict("用户名已存在")
		}
		if err := tx.Users.Insert(username, string(hash), role); err != nil {
			return err
		}
		user, err := tx.Users.FindByUsername(username)
		if err != nil {
			return err
		}
		out = teamUser(user, []int64{})
		return nil
	})
	return &out, err
}

func (s *TeamService) SetAccess(actorID, userID int64, role *string, active *bool) error {
	return s.store.Do(func(tx *db.TxScope) error {
		if err := requireSuper(tx, actorID); err != nil {
			return err
		}
		user, err := tx.Users.FindByID(userID)
		if err != nil {
			return err
		}
		if user == nil {
			return httperr.NotFound("账号不存在")
		}
		newRole, newActive := user.Role, user.IsActive
		if role != nil {
			newRole = *role
		}
		if active != nil {
			newActive = *active
		}
		if newRole != db.RoleAdmin && newRole != db.RoleSuperAdmin {
			return httperr.BadRequest("角色无效")
		}
		if userID == actorID && !newActive {
			return httperr.Conflict("不能停用当前登录账号")
		}
		if user.Role == db.RoleSuperAdmin && user.IsActive && (newRole != db.RoleSuperAdmin || !newActive) {
			count, err := tx.Users.CountActiveSuperAdmins()
			if err != nil {
				return err
			}
			if count <= 1 {
				return httperr.Conflict("必须保留至少一位启用的超级管理员")
			}
		}
		if newRole == user.Role && newActive == user.IsActive {
			return nil
		}
		if err := tx.Users.SetAccess(userID, newRole, newActive); err != nil {
			return err
		}
		return tx.Users.RevokeUserSessions(userID)
	})
}

func (s *TeamService) ResetPassword(actorID, userID int64, password string) error {
	if err := validTeamPassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.store.Do(func(tx *db.TxScope) error {
		if err := requireSuper(tx, actorID); err != nil {
			return err
		}
		user, err := tx.Users.FindByID(userID)
		if err != nil {
			return err
		}
		if user == nil {
			return httperr.NotFound("账号不存在")
		}
		if err := tx.Users.SetPassword(userID, string(hash)); err != nil {
			return err
		}
		return tx.Users.RevokeUserSessions(userID)
	})
}

func (s *TeamService) SetProjects(actorID, userID int64, ids []int64) error {
	if len(ids) > 200 {
		return httperr.BadRequest("一次最多配置 200 个项目")
	}
	return s.store.Do(func(tx *db.TxScope) error {
		if err := requireSuper(tx, actorID); err != nil {
			return err
		}
		user, err := tx.Users.FindByID(userID)
		if err != nil {
			return err
		}
		if user == nil {
			return httperr.NotFound("账号不存在")
		}
		seen := make(map[int64]bool)
		for _, id := range ids {
			if id <= 0 || seen[id] {
				return httperr.BadRequest("项目 ID 无效或重复")
			}
			seen[id] = true
			project, err := tx.Groups.FindByID(id)
			if err != nil {
				return err
			}
			if project == nil {
				return httperr.NotFound("项目不存在")
			}
		}
		return tx.Users.SetProjectIDs(userID, ids)
	})
}

func (s *AuthService) ChangePassword(userID int64, current, next string) error {
	if err := validTeamPassword(next); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.store.Do(func(tx *db.TxScope) error {
		user, err := tx.Users.FindByID(userID)
		if err != nil {
			return err
		}
		if user == nil || !user.IsActive {
			return httperr.Unauthorized()
		}
		if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(current)) != nil {
			return httperr.BadRequest("当前密码不正确")
		}
		if err := tx.Users.SetPassword(userID, string(hash)); err != nil {
			return err
		}
		return tx.Users.RevokeUserSessions(userID)
	})
}

type SessionInfo struct {
	ID         string `json:"id"`
	ClientIP   string `json:"clientIp"`
	UserAgent  string `json:"userAgent"`
	CreatedAt  string `json:"createdAt"`
	LastSeenAt string `json:"lastSeenAt"`
	ExpiresAt  string `json:"expiresAt"`
	Current    bool   `json:"current"`
}

func (s *AuthService) Sessions(userID int64, currentID string) ([]SessionInfo, error) {
	user, err := s.store.Users.FindByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsActive {
		return nil, httperr.Unauthorized()
	}
	sessions, err := s.store.Users.ListSessions(userID, user.SessionVersion)
	if err != nil {
		return nil, err
	}
	out := make([]SessionInfo, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, SessionInfo{ID: session.ID, ClientIP: session.ClientIP, UserAgent: session.UserAgent, CreatedAt: JsTime(session.CreatedAt), LastSeenAt: JsTime(session.LastSeenAt), ExpiresAt: JsTime(session.ExpiresAt), Current: session.ID == currentID})
	}
	return out, nil
}
