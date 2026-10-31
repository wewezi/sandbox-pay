package services

import (
	"fmt"
	"strings"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/logger"
)

// GenerateApiKey creates a 256-bit bearer credential. Only its hash is stored.
func GenerateApiKey() string {
	return "sp_sandbox_" + randomHex(32)
}

// ApiKeyDTO lists metadata. Key is populated only in a creation receipt.
type ApiKeyDTO struct {
	ID            int64   `json:"id"`
	GroupID       int64   `json:"groupId"`
	GroupName     string  `json:"groupName"`
	GroupIsActive bool    `json:"groupIsActive"`
	Name          string  `json:"name"`
	Key           string  `json:"key,omitempty"`
	KeyPrefix     string  `json:"keyPrefix"`
	KeySuffix     string  `json:"keySuffix"`
	IsActive      bool    `json:"isActive"`
	LastUsedAt    *string `json:"lastUsedAt,omitempty"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
}

type GroupKeyDTO = ApiKeyDTO

// GroupWithKeys returns project configuration and credential metadata.
type GroupWithKeys struct {
	ID                int64         `json:"id"`
	Name              string        `json:"name"`
	IsActive          bool          `json:"isActive"`
	CreatedAt         string        `json:"createdAt"`
	WebhookConfigured bool          `json:"webhookConfigured"`
	Keys              []GroupKeyDTO `json:"keys"`
}

// GroupService 分组与 API Key 管理。
//
// 多分组 × 多 Key：分组之间数据隔离（订单只能查自己的），
// 同分组多把 Key 用来轮换或区分环境。
//
// 首次启动种一个分组 + 一把 Key，Key 优先从 .env 的 GROUP_API_KEY 读，
// 没配就随机生成并持久化，只有首次生成时在启动日志里展示。
// 之后管理都在分组页上做；重启、读列表都不会覆盖人工停用。
type GroupService struct {
	log   *logger.Logger
	store *db.Store
	cfg   config.Config
}

// NewGroupService 构造。
func NewGroupService(store *db.Store, cfg config.Config) *GroupService {
	return &GroupService{
		log:   logger.New("GroupService"),
		store: store,
		cfg:   cfg,
	}
}

// Init 启动时调一次。只有空库会创建种子数据，已有凭证不再输出到日志。
func (s *GroupService) Init() error {
	group, key, generated, err := s.EnsureSeed()
	if err != nil {
		return err
	}
	if generated {
		// Only this initial receipt has the plaintext bearer credential.
		s.log.Warn(fmt.Sprintf("未配置 GROUP_API_KEY，首次生成的令牌仅此次完整展示，请保存：\n  GROUP_API_KEY=%s", key.Key))
		return nil
	}
	s.log.Log(fmt.Sprintf("分组就绪 (id=%d)", group.ID))
	return nil
}

// EnsureSeed 只为没有分组的空库创建种子分组和第一把 Key。
//
// 已有分组的人工启停状态始终保留；没有有效 Key 时返回 nil Key，
// 管理员仍能登录后台并主动签发或启用凭证。
//
// generated=true 表示这次首次初始化随机生成了 Key。
func (s *GroupService) EnsureSeed() (*db.Group, *db.APIKey, bool, error) {
	var group *db.Group
	var key *db.APIKey
	var generated bool
	err := s.store.Do(func(tx *db.TxScope) error {
		var err error
		group, err = tx.Groups.FindFirst()
		if err != nil {
			return err
		}
		if group != nil {
			keys, err := tx.ApiKeys.ListByGroup(group.ID)
			if err != nil {
				return err
			}
			for _, existing := range keys {
				if existing.IsActive {
					key = existing
					break
				}
			}
			return nil
		}

		name := strings.TrimSpace(s.cfg.Get("GROUP_NAME"))
		if name == "" {
			name = "Mintweft Default Project"
		}
		group, err = tx.Groups.Insert(name, true)
		if err != nil {
			return err
		}
		keyValue := strings.TrimSpace(s.cfg.Get("GROUP_API_KEY"))
		keyName := "来自 .env"
		generated = keyValue == ""
		if generated {
			keyValue = GenerateApiKey()
			keyName = "自动生成"
		}
		key, err = tx.ApiKeys.Insert(db.InsertKeyInput{
			GroupID: group.ID, Name: keyName, Key: keyValue, IsActive: true,
		})
		return err
	})
	if err != nil {
		return nil, nil, false, err
	}
	return group, key, generated, nil
}

// DefaultGroup selects the first project for sandbox requests without a token.
// Authenticated sandbox callbacks use that project's independent webhook secret.
func (s *GroupService) DefaultGroup() (*db.Group, error) {
	group, _, _, err := s.EnsureSeed()
	return group, err
}

// ListGroups never returns bearer credentials or webhook signing secrets.
func (s *GroupService) ListGroups(scopes ...[]int64) ([]GroupWithKeys, error) {
	groups, err := s.store.Groups.List(scopes...)
	if err != nil {
		return nil, err
	}
	out := make([]GroupWithKeys, 0, len(groups))
	for _, g := range groups {
		keys, err := s.store.ApiKeys.ListByGroup(g.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, *toGroupWithKeys(g, keys))
	}
	return out, nil
}

// CreateGroup 新建分组，顺带签发第一把 Key —— 没 Key 的分组没用。
func (s *GroupService) CreateGroup(name, keyName string) (*GroupWithKeys, error) {
	var result *GroupWithKeys
	err := s.store.Do(func(tx *db.TxScope) error {
		group, err := tx.Groups.Insert(name, true)
		if err != nil {
			return err
		}
		key, err := tx.ApiKeys.Insert(db.InsertKeyInput{
			GroupID:  group.ID,
			Name:     defaultName(keyName, "默认"),
			Key:      GenerateApiKey(),
			IsActive: true,
		})
		if err != nil {
			return err
		}
		result = toGroupWithKeys(group, []*db.APIKey{key})
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.log.Log(fmt.Sprintf("Group created: %s (id=%d)", name, result.ID))
	return result, nil
}

// IssueKey 给已有分组再签一把 Key。
func (s *GroupService) IssueKey(groupID int64, name string) (*ApiKeyDTO, error) {
	var key *db.APIKey
	var group *db.Group
	err := s.store.Do(func(tx *db.TxScope) error {
		var err error
		group, err = tx.Groups.FindByID(groupID)
		if err != nil {
			return err
		}
		if group == nil {
			return httperr.NotFound("分组不存在")
		}
		key, err = tx.ApiKeys.Insert(db.InsertKeyInput{
			GroupID:  groupID,
			Name:     defaultName(name, "新 Key"),
			Key:      GenerateApiKey(),
			IsActive: true,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	s.log.Log(fmt.Sprintf("API key issued for group %d (id=%d)", groupID, key.ID))
	return toApiKeyDTO(key, group), nil
}

// ToggleKey 停用 / 启用一把 Key。
//
// Disabling only blocks new API authentication. Existing order callbacks retain
// their own signing snapshot, independently of token lifecycle changes.
func (s *GroupService) ToggleKey(keyID int64) (*ApiKeyDTO, error) {
	var key *db.APIKey
	var group *db.Group
	err := s.store.Do(func(tx *db.TxScope) error {
		var err error
		key, err = tx.ApiKeys.FindByID(keyID)
		if err != nil {
			return err
		}
		if key == nil {
			return httperr.NotFound("API Key 不存在")
		}
		if err := tx.ApiKeys.Update(keyID, map[string]any{"isActive": !key.IsActive}); err != nil {
			return err
		}
		key, err = tx.ApiKeys.FindByID(keyID)
		if err != nil {
			return err
		}
		group, err = tx.Groups.FindByID(key.GroupID)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.log.Log(fmt.Sprintf("API key %d %s", keyID, activeWord(key.IsActive)))
	return toApiKeyDTO(key, group), nil
}

// ToggleGroup 停用 / 启用分组。停用后它名下所有 Key 都调不通。
func (s *GroupService) ToggleGroup(groupID int64) (*GroupWithKeys, error) {
	var result *GroupWithKeys
	err := s.store.Do(func(tx *db.TxScope) error {
		group, err := tx.Groups.FindByID(groupID)
		if err != nil {
			return err
		}
		if group == nil {
			return httperr.NotFound("分组不存在")
		}
		group.IsActive = !group.IsActive
		if err := tx.Groups.Update(groupID, map[string]any{"isActive": group.IsActive}); err != nil {
			return err
		}
		keys, err := tx.ApiKeys.ListByGroup(groupID)
		if err != nil {
			return err
		}
		result = toGroupWithKeys(group, keys)
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.log.Log(fmt.Sprintf("Group %d %s", groupID, activeWord(result.IsActive)))
	return result, nil
}

func toGroupWithKeys(group *db.Group, keys []*db.APIKey) *GroupWithKeys {
	result := &GroupWithKeys{
		ID:                group.ID,
		Name:              group.Name,
		IsActive:          group.IsActive,
		CreatedAt:         JsTime(group.CreatedAt),
		WebhookConfigured: group.WebhookSecret != "",
		Keys:              make([]GroupKeyDTO, 0, len(keys)),
	}
	for _, key := range keys {
		result.Keys = append(result.Keys, *toApiKeyDTO(key, group))
	}
	return result
}

func toApiKeyDTO(k *db.APIKey, group *db.Group) *ApiKeyDTO {
	out := &ApiKeyDTO{
		ID:         k.ID,
		GroupID:    k.GroupID,
		Name:       k.Name,
		Key:        k.Key,
		KeyPrefix:  k.KeyPrefix,
		KeySuffix:  k.KeySuffix,
		IsActive:   k.IsActive,
		LastUsedAt: jsTimePtr(k.LastUsedAt),
		CreatedAt:  JsTime(k.CreatedAt),
		UpdatedAt:  JsTime(k.UpdatedAt),
	}
	if group != nil {
		out.GroupName = group.Name
		out.GroupIsActive = group.IsActive
	}
	return out
}

// defaultName 备注名空/纯空格时回落默认值。
func defaultName(raw, fallback string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		return fallback
	}
	return name
}

func activeWord(active bool) string {
	if active {
		return "activated"
	}
	return "deactivated"
}
