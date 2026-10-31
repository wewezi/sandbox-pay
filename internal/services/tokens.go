package services

import (
	"fmt"
	"strings"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
)

// IssueToken provides the primary creation flow without requiring a new project.
// A missing project selects the existing default, retaining its paused state.
func (s *GroupService) IssueToken(groupID *int64, name string) (*ApiKeyDTO, error) {
	var group *db.Group
	var key *db.APIKey
	err := s.store.Do(func(tx *db.TxScope) error {
		var err error
		if groupID == nil {
			group, err = tx.Groups.FindFirst()
			if err != nil {
				return err
			}
			if group == nil {
				projectName := defaultName(s.cfg.Get("GROUP_NAME"), "默认项目")
				group, err = tx.Groups.Insert(projectName, true)
			}
		} else {
			group, err = tx.Groups.FindByID(*groupID)
		}
		if err != nil {
			return err
		}
		if group == nil {
			return httperr.NotFound("项目不存在")
		}
		if !group.IsActive {
			return httperr.BadRequest("项目已停用，请先启用项目或选择其他项目")
		}
		key, err = tx.ApiKeys.Insert(db.InsertKeyInput{
			GroupID: group.ID, Name: defaultName(name, "新令牌"),
			Key: GenerateApiKey(), IsActive: true,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	s.log.Log(fmt.Sprintf("API token issued for project %d (id=%d)", group.ID, key.ID))
	return toApiKeyDTO(key, group), nil
}

// ListTokens flattens project metadata while keeping the credentials private.
func (s *GroupService) ListTokens(groupID *int64, scopes ...[]int64) ([]ApiKeyDTO, error) {
	groups, err := s.ListGroups(scopes...)
	if err != nil {
		return nil, err
	}
	result := make([]ApiKeyDTO, 0)
	for _, group := range groups {
		if groupID != nil && group.ID != *groupID {
			continue
		}
		result = append(result, group.Keys...)
	}
	return result, nil
}

// CreateProject is independent of credential issuance.
func (s *GroupService) CreateProject(name string) (*GroupWithKeys, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, httperr.BadRequest("项目名称不能为空")
	}
	var result *GroupWithKeys
	err := s.store.Do(func(tx *db.TxScope) error {
		group, err := tx.Groups.Insert(name, true)
		if err != nil {
			return err
		}
		result = toGroupWithKeys(group, nil)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

type WebhookSecretDTO struct {
	Secret string `json:"secret"`
}

// WebhookSecret is available only through the authenticated administration API.
// Paused projects retain their signing secret for existing order callbacks.
func (s *GroupService) WebhookSecret(groupID int64) (*WebhookSecretDTO, error) {
	group, err := s.store.Groups.FindByID(groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, httperr.NotFound("项目不存在")
	}
	if group.WebhookSecret == "" {
		return nil, httperr.BadRequest("项目尚未配置回调签名密钥")
	}
	return &WebhookSecretDTO{Secret: group.WebhookSecret}, nil
}
