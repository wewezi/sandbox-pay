package services

import (
	"fmt"
	"strings"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/logger"
)

// GenerateApiKey 32 字节 hex，够长到不用担心被猜 —— Key 同时当 HMAC 密钥用，
// 不能按「反正还有个 secret」的思路缩短。
// 前缀方便一眼认出这是什么东西，也方便日志里做打码匹配。
func GenerateApiKey() string {
	return "sp_sandbox_" + randomHex(32)
}

// GroupKeyDTO 沙箱页展示用的 Key（含明文，要给人抄）。
type GroupKeyDTO struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Key        string  `json:"key"`
	IsActive   bool    `json:"isActive"`
	LastUsedAt *string `json:"lastUsedAt,omitempty"`
	CreatedAt  string  `json:"createdAt"`
}

// ApiKeyDTO 签发/启停 Key 的回执，字段比列表版多 groupId 和 updatedAt。
type ApiKeyDTO struct {
	ID         int64   `json:"id"`
	GroupID    int64   `json:"groupId"`
	Name       string  `json:"name"`
	Key        string  `json:"key"`
	IsActive   bool    `json:"isActive"`
	LastUsedAt *string `json:"lastUsedAt,omitempty"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
}

// GroupWithKeys 沙箱页展示用：分组 + 它名下所有 Key（含明文）。
type GroupWithKeys struct {
	ID        int64         `json:"id"`
	Name      string        `json:"name"`
	IsActive  bool          `json:"isActive"`
	CreatedAt string        `json:"createdAt"`
	Keys      []GroupKeyDTO `json:"keys"`
}

// GroupService 分组与 API Key 管理。
//
// 多分组 × 多 Key：分组之间数据隔离（订单只能查自己的），
// 同分组多把 Key 用来轮换或区分环境。
//
// 首次启动种一个分组 + 一把 Key，Key 优先从 .env 的 GROUP_API_KEY 读，
// 没配就随机生成（启动日志会打出来，抄到 .env 即可固定）。
// 之后增删都在沙箱页上做，不用重启。
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

// Init 启动时调一次：确保种子分组和它的第一把 Key 存在，并把凭证打到日志里。
func (s *GroupService) Init() error {
	_, key, generated, err := s.EnsureSeed()
	if err != nil {
		return err
	}
	if generated {
		// Key 就是密钥；只在随机生成时打一次，好让人抄进 .env 固定下来。
		s.log.Warn(fmt.Sprintf("未配置 GROUP_API_KEY，已随机生成。要固定下来请写进 .env：\n  GROUP_API_KEY=%s", key.Key))
		return nil
	}
	s.log.Log(fmt.Sprintf("✅ 分组就绪 apiKey=%s", key.Key))
	return nil
}

// EnsureSeed 确保种子分组存在，且至少有一把可用的 Key。
//
// 两步分开判：库里已有分组但 Key 全被停用时，还得补一把出来，
// 否则谁都调不通网关，而沙箱页又要先登录才进得去。
//
// generated=true 表示这把 Key 是随机生成的，启动日志要打出来给人抄。
func (s *GroupService) EnsureSeed() (*db.Group, *db.APIKey, bool, error) {
	group, err := s.store.Groups.FindFirst()
	if err != nil {
		return nil, nil, false, err
	}
	if group == nil {
		name := strings.TrimSpace(s.cfg.Get("GROUP_NAME"))
		if name == "" {
			name = "Sandbox Pay Group"
		}
		group, err = s.store.Groups.Insert(name, true)
		if err != nil {
			return nil, nil, false, err
		}
	}

	keys, err := s.store.ApiKeys.ListByGroup(group.ID)
	if err != nil {
		return nil, nil, false, err
	}
	for _, k := range keys {
		if k.IsActive {
			return group, k, false, nil
		}
	}

	fromEnv := strings.TrimSpace(s.cfg.Get("GROUP_API_KEY"))
	// .env 配了 Key，但库里可能已经有这一行（被停用了）。
	// key 上有唯一约束，直接 insert 会 500 —— 而 listGroups 每次都会走到
	// 这里，那样沙箱页会彻底打不开，也就没法在界面上把它重新启用。
	if fromEnv != "" {
		existing, err := s.store.ApiKeys.FindByKey(fromEnv)
		if err != nil {
			return nil, nil, false, err
		}
		if existing != nil {
			if existing.GroupID == group.ID {
				if err := s.store.ApiKeys.Update(existing.ID, map[string]any{"isActive": true}); err != nil {
					return nil, nil, false, err
				}
				existing.IsActive = true
				return group, existing, false, nil
			}
			// 这把 Key 挂在别的分组名下，抢过来会把那边的入站鉴权搞乱。
			// 退回随机生成，且当成「生成」上报，让启动日志把新 Key 打出来。
			key, err := s.store.ApiKeys.Insert(db.InsertKeyInput{
				GroupID: group.ID, Name: "自动生成", Key: GenerateApiKey(), IsActive: true,
			})
			if err != nil {
				return nil, nil, false, err
			}
			return group, key, true, nil
		}
	}

	name := "自动生成"
	keyValue := GenerateApiKey()
	if fromEnv != "" {
		name = "来自 .env"
		keyValue = fromEnv
	}
	key, err := s.store.ApiKeys.Insert(db.InsertKeyInput{
		GroupID: group.ID, Name: name, Key: keyValue, IsActive: true,
	})
	if err != nil {
		return nil, nil, false, err
	}
	return group, key, fromEnv == "", nil
}

// DefaultGroup 种子分组。给后台测试下单、公开测试下单这类没有 Key 上下文的路径用 ——
// 它们不带 notifyUrl，所以不需要 Key 去签回调。
func (s *GroupService) DefaultGroup() (*db.Group, error) {
	group, _, _, err := s.EnsureSeed()
	return group, err
}

// ListGroups 沙箱页的列表：所有分组 + 各自的 Key，Key 是明文。
func (s *GroupService) ListGroups() ([]GroupWithKeys, error) {
	// 确保至少有一个分组，否则删库重建后页面是空的、也没法新建 Key
	if _, _, _, err := s.EnsureSeed(); err != nil {
		return nil, err
	}
	groups, err := s.store.Groups.List()
	if err != nil {
		return nil, err
	}
	out := make([]GroupWithKeys, 0, len(groups))
	for _, g := range groups {
		keys, err := s.store.ApiKeys.ListByGroup(g.ID)
		if err != nil {
			return nil, err
		}
		item := GroupWithKeys{
			ID:        g.ID,
			Name:      g.Name,
			IsActive:  g.IsActive,
			CreatedAt: JsTime(g.CreatedAt),
			Keys:      make([]GroupKeyDTO, 0, len(keys)),
		}
		for _, k := range keys {
			item.Keys = append(item.Keys, GroupKeyDTO{
				ID:         k.ID,
				Name:       k.Name,
				Key:        k.Key,
				IsActive:   k.IsActive,
				LastUsedAt: jsTimePtr(k.LastUsedAt),
				CreatedAt:  JsTime(k.CreatedAt),
			})
		}
		out = append(out, item)
	}
	return out, nil
}

// CreateGroup 新建分组，顺带签发第一把 Key —— 没 Key 的分组没用。
func (s *GroupService) CreateGroup(name, keyName string) (*GroupWithKeys, error) {
	group, err := s.store.Groups.Insert(name, true)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.ApiKeys.Insert(db.InsertKeyInput{
		GroupID:  group.ID,
		Name:     defaultName(keyName, "默认"),
		Key:      GenerateApiKey(),
		IsActive: true,
	}); err != nil {
		return nil, err
	}
	s.log.Log(fmt.Sprintf("Group created: %s (id=%d)", name, group.ID))
	return s.requireGroupWithKeys(group.ID)
}

// IssueKey 给已有分组再签一把 Key。
func (s *GroupService) IssueKey(groupID int64, name string) (*ApiKeyDTO, error) {
	group, err := s.store.Groups.FindByID(groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, httperr.NotFound("分组不存在")
	}
	key, err := s.store.ApiKeys.Insert(db.InsertKeyInput{
		GroupID:  groupID,
		Name:     defaultName(name, "新 Key"),
		Key:      GenerateApiKey(),
		IsActive: true,
	})
	if err != nil {
		return nil, err
	}
	s.log.Log(fmt.Sprintf("API key issued for group %d (id=%d)", groupID, key.ID))
	return toApiKeyDTO(key), nil
}

// ToggleKey 停用 / 启用一把 Key。
//
// 只改 isActive，不删行：已有订单的回调还要拿这把 Key 签名。
// 也就是说停用只挡入站，不影响那些单的出站签名 —— 接入方那边验签逻辑不用动。
func (s *GroupService) ToggleKey(keyID int64) (*ApiKeyDTO, error) {
	key, err := s.store.ApiKeys.FindByID(keyID)
	if err != nil {
		return nil, err
	}
	if key == nil {
		return nil, httperr.NotFound("API Key 不存在")
	}
	// 先把目标值定下来再 update：避免 update 之后再读到的已是新值。
	next := !key.IsActive
	if err := s.store.ApiKeys.Update(keyID, map[string]any{"isActive": next}); err != nil {
		return nil, err
	}
	key.IsActive = next
	s.log.Log(fmt.Sprintf("API key %d %s", keyID, activeWord(next)))
	return toApiKeyDTO(key), nil
}

// ToggleGroup 停用 / 启用分组。停用后它名下所有 Key 都调不通。
func (s *GroupService) ToggleGroup(groupID int64) (*GroupWithKeys, error) {
	group, err := s.store.Groups.FindByID(groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, httperr.NotFound("分组不存在")
	}
	next := !group.IsActive
	if err := s.store.Groups.Update(groupID, map[string]any{"isActive": next}); err != nil {
		return nil, err
	}
	s.log.Log(fmt.Sprintf("Group %d %s", groupID, activeWord(next)))
	return s.requireGroupWithKeys(groupID)
}

// requireGroupWithKeys 刚写完就读回来，拼给前端。
// 找不到说明上一步写失败了，不该静默返回空。
func (s *GroupService) requireGroupWithKeys(groupID int64) (*GroupWithKeys, error) {
	list, err := s.ListGroups()
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == groupID {
			return &list[i], nil
		}
	}
	return nil, httperr.NotFound("分组不存在")
}

func toApiKeyDTO(k *db.APIKey) *ApiKeyDTO {
	return &ApiKeyDTO{
		ID:         k.ID,
		GroupID:    k.GroupID,
		Name:       k.Name,
		Key:        k.Key,
		IsActive:   k.IsActive,
		LastUsedAt: jsTimePtr(k.LastUsedAt),
		CreatedAt:  JsTime(k.CreatedAt),
		UpdatedAt:  JsTime(k.UpdatedAt),
	}
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
