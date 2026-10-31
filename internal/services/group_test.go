package services

import (
	"regexp"
	"testing"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
)

var apiKeyPattern = regexp.MustCompile(`^sp_sandbox_[0-9a-f]{64}$`)

type groupFixture struct {
	service *GroupService
	store   *db.Store
}

// createGroupService 每个用例一个真库（对应 TS 的数组假 repo）——
// 「写进去再读出来」的行为真库天然满足；env 是一张 key→value 的
// 字符串表，和 TS 的 fakeConfig(env) 一致。
func createGroupService(t *testing.T, env map[string]string) *groupFixture {
	t.Helper()
	store := newTestStore(t)
	svc := NewGroupService(store, config.Map(env))
	return &groupFixture{service: svc, store: store}
}

// groupKeyCount 数一下某分组名下的 Key 行数。
func groupKeyCount(t *testing.T, store *db.Store, groupID int64) int {
	t.Helper()
	keys, err := store.ApiKeys.ListByGroup(groupID)
	if err != nil {
		t.Fatalf("ListByGroup: %v", err)
	}
	return len(keys)
}

func groupCount(t *testing.T, store *db.Store) int {
	t.Helper()
	groups, err := store.Groups.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return len(groups)
}

// ensureSeed 要求成功并给出三元组。
func (f *groupFixture) ensureSeed(t *testing.T) (*db.Group, *db.APIKey, bool) {
	t.Helper()
	group, key, generated, err := f.service.EnsureSeed()
	if err != nil {
		t.Fatalf("EnsureSeed: %v", err)
	}
	return group, key, generated
}

// Port of server/src/services/group.service.spec.ts
func TestGroupServiceEnsureSeed(t *testing.T) {
	t.Run("空库时建出分组和第一把 Key，Key 来自 .env", func(t *testing.T) {
		f := createGroupService(t, map[string]string{
			"GROUP_NAME":    "My Sandbox",
			"GROUP_API_KEY": "sp_from_env",
		})

		group, key, generated := f.ensureSeed(t)

		if group.Name != "My Sandbox" {
			t.Errorf("group.name = %q, want My Sandbox", group.Name)
		}
		if !group.IsActive {
			t.Error("group.isActive = false, want true")
		}
		if key.Key != "sp_from_env" {
			t.Errorf("key.key = %q, want sp_from_env", key.Key)
		}
		if key.GroupID != group.ID {
			t.Errorf("key.groupId = %d, want group.id %d", key.GroupID, group.ID)
		}
		// 配了就不算「生成」，启动日志走 log 而不是 warn
		if generated {
			t.Error("generated = true, want false when the key comes from .env")
		}
	})

	t.Run(".env 没配就随机生成一把", func(t *testing.T) {
		f := createGroupService(t, map[string]string{})

		_, key, generated := f.ensureSeed(t)

		// Key 同时当 HMAC 密钥用，所以是 32 字节
		if !apiKeyPattern.MatchString(key.Key) {
			t.Errorf("key.key = %q, want sp_sandbox_ + 64 hex chars", key.Key)
		}
		if !generated {
			t.Error("generated = false, want true for a random key")
		}
	})

	t.Run(".env 里是纯空格按没配处理", func(t *testing.T) {
		f := createGroupService(t, map[string]string{
			"GROUP_API_KEY": "   ",
			"GROUP_NAME":    "",
		})

		group, key, _ := f.ensureSeed(t)

		if !apiKeyPattern.MatchString(key.Key) {
			t.Errorf("key.key = %q, want sp_sandbox_ + 64 hex chars", key.Key)
		}
		if group.Name != "Mintweft Default Project" {
			t.Errorf("group.name = %q, want Mintweft Default Project", group.Name)
		}
	})

	t.Run("已有分组和 Key 时不重复建", func(t *testing.T) {
		f := createGroupService(t, map[string]string{
			"GROUP_API_KEY": "sp_from_env",
		})
		group1, key1, _ := f.ensureSeed(t)

		group2, key2, generated := f.ensureSeed(t)

		// 对应 TS 对 insertGroup/insertKey 的调用计数：真库看行数 + 复用同一行
		if n := groupCount(t, f.store); n != 1 {
			t.Errorf("group rows = %d, want 1", n)
		}
		if n := groupKeyCount(t, f.store, group1.ID); n != 1 {
			t.Errorf("key rows = %d, want 1", n)
		}
		if group2.ID != group1.ID || key2.ID != key1.ID {
			t.Errorf("second EnsureSeed created new rows (group %d→%d, key %d→%d)",
				group1.ID, group2.ID, key1.ID, key2.ID)
		}
		if generated {
			t.Error("second EnsureSeed reported generated=true, want false")
		}
	})

	t.Run("随机 Key 全被停用时不自动补发", func(t *testing.T) {
		f := createGroupService(t, map[string]string{})
		group, first, _ := f.ensureSeed(t)
		if _, err := f.service.ToggleKey(first.ID); err != nil {
			t.Fatalf("ToggleKey: %v", err)
		}

		_, key, generated := f.ensureSeed(t)
		if key != nil || generated {
			t.Errorf("EnsureSeed returned key=%v generated=%v, want nil/false", key, generated)
		}
		if n := groupKeyCount(t, f.store, group.ID); n != 1 {
			t.Errorf("key rows = %d, want 1", n)
		}
	})

	t.Run(".env Key 的停用经过列表查询默认分组和重启仍保持", func(t *testing.T) {
		f := createGroupService(t, map[string]string{
			"GROUP_API_KEY": "sp_from_env",
		})
		group, first, _ := f.ensureSeed(t)
		if _, err := f.service.ToggleKey(first.ID); err != nil {
			t.Fatalf("ToggleKey: %v", err)
		}

		list, err := f.service.ListGroups()
		if err != nil {
			t.Fatalf("ListGroups: %v", err)
		}
		if len(list) != 1 || len(list[0].Keys) != 1 || list[0].Keys[0].IsActive {
			t.Fatalf("list changed disabled key: %+v", list)
		}
		if _, err := f.service.DefaultGroup(); err != nil {
			t.Fatalf("DefaultGroup: %v", err)
		}
		// 新 service 执行与进程启动相同的 Init，凭证状态只来自数据库。
		restarted := NewGroupService(f.store, config.Map(map[string]string{
			"GROUP_API_KEY": "sp_from_env",
		}))
		if err := restarted.Init(); err != nil {
			t.Fatalf("Init: %v", err)
		}
		_, key, generated, err := restarted.EnsureSeed()
		if err != nil {
			t.Fatalf("EnsureSeed: %v", err)
		}
		if key != nil || generated {
			t.Errorf("EnsureSeed returned key=%v generated=%v, want nil/false", key, generated)
		}
		stored, err := f.store.ApiKeys.FindByID(first.ID)
		if err != nil || stored == nil || stored.IsActive {
			t.Fatalf("disabled key changed: key=%v err=%v", stored, err)
		}
		// 复用原来那行，不新增
		if n := groupKeyCount(t, f.store, group.ID); n != 1 {
			t.Errorf("key rows = %d, want 1", n)
		}
	})

	t.Run("已有分组的 Key 被移动后不按 .env 补发或夺回", func(t *testing.T) {
		f := createGroupService(t, map[string]string{
			"GROUP_API_KEY": "sp_from_env",
		})
		// 先让种子分组存在，再把 .env 那把 Key 挂到另一个分组名下
		seeded, first, _ := f.ensureSeed(t)
		if _, err := f.service.ToggleKey(first.ID); err != nil {
			t.Fatalf("ToggleKey: %v", err)
		}
		// TS 是先 update groupId=999 再插「别人」—— 假 repo 没有外键。
		// 真库 FK 开着：必须先建目标分组再挪 Key（测试侧顺序适配，
		// 断言的语义不变：这把 Key 挂在别的分组名下）。
		other, err := f.store.Groups.Insert("别人", true)
		if err != nil {
			t.Fatalf("insert other group: %v", err)
		}
		if err := f.store.ApiKeys.Update(first.ID, map[string]any{"groupId": other.ID}); err != nil {
			t.Fatalf("move key to other group: %v", err)
		}

		_, key, generated := f.ensureSeed(t)

		if key != nil || generated {
			t.Errorf("EnsureSeed returned key=%v generated=%v, want nil/false", key, generated)
		}
		if n := groupKeyCount(t, f.store, seeded.ID); n != 0 {
			t.Errorf("seed key rows = %d, want 0", n)
		}
		if n := groupKeyCount(t, f.store, other.ID); n != 1 {
			t.Errorf("other key rows = %d, want 1", n)
		}
	})
}

func TestGroupServiceListGroups(t *testing.T) {
	t.Run("列表只返回令牌标识，不再返回明文", func(t *testing.T) {
		f := createGroupService(t, map[string]string{
			"GROUP_API_KEY": "sp_visible",
		})
		f.ensureSeed(t)

		list, err := f.service.ListGroups()
		if err != nil {
			t.Fatalf("ListGroups: %v", err)
		}

		if len(list) != 1 {
			t.Fatalf("len(list) = %d, want 1", len(list))
		}
		if len(list[0].Keys) != 1 {
			t.Fatalf("len(keys) = %d, want 1", len(list[0].Keys))
		}
		if list[0].Keys[0].Key != "" || list[0].Keys[0].KeyPrefix == "" {
			t.Error("token list must contain display metadata without plaintext")
		}
	})

	t.Run("空库列表只读不创建分组或凭证", func(t *testing.T) {
		f := createGroupService(t, map[string]string{})

		list, err := f.service.ListGroups()
		if err != nil {
			t.Fatalf("ListGroups: %v", err)
		}
		if len(list) != 0 || groupCount(t, f.store) != 0 {
			t.Errorf("list query created groups: %+v", list)
		}
	})
}

func TestGroupServiceCreateGroup(t *testing.T) {
	t.Run("新分组自带第一把 Key —— 没 Key 的分组没用", func(t *testing.T) {
		f := createGroupService(t, map[string]string{})

		created, err := f.service.CreateGroup("分组B", "生产")
		if err != nil {
			t.Fatalf("CreateGroup: %v", err)
		}

		if created.Name != "分组B" {
			t.Errorf("name = %q, want 分组B", created.Name)
		}
		if len(created.Keys) != 1 {
			t.Fatalf("len(keys) = %d, want 1", len(created.Keys))
		}
		if created.Keys[0].Name != "生产" {
			t.Errorf("keys[0].name = %q, want 生产", created.Keys[0].Name)
		}
		if !apiKeyPattern.MatchString(created.Keys[0].Key) {
			t.Errorf("keys[0].key = %q, want sp_sandbox_ + 64 hex chars", created.Keys[0].Key)
		}
	})

	t.Run("不传 Key 备注名给个默认的", func(t *testing.T) {
		f := createGroupService(t, map[string]string{})

		created, err := f.service.CreateGroup("分组C", "")
		if err != nil {
			t.Fatalf("CreateGroup: %v", err)
		}
		if len(created.Keys) != 1 {
			t.Fatalf("len(keys) = %d, want 1", len(created.Keys))
		}
		if created.Keys[0].Name != "默认" {
			t.Errorf("keys[0].name = %q, want 默认", created.Keys[0].Name)
		}
	})
}

func TestGroupServiceIssueKey(t *testing.T) {
	t.Run("同一分组可以有多把 Key", func(t *testing.T) {
		f := createGroupService(t, map[string]string{})
		group, _, _ := f.ensureSeed(t)

		if _, err := f.service.IssueKey(group.ID, "第二把"); err != nil {
			t.Fatalf("IssueKey: %v", err)
		}

		list, err := f.service.ListGroups()
		if err != nil {
			t.Fatalf("ListGroups: %v", err)
		}
		keys := list[0].Keys
		if len(keys) != 2 {
			t.Fatalf("len(keys) = %d, want 2", len(keys))
		}
		if keys[1].Name != "第二把" {
			t.Errorf("keys[1].name = %q, want 第二把", keys[1].Name)
		}
		first, err := f.store.ApiKeys.FindByID(keys[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		second, err := f.store.ApiKeys.FindByID(keys[1].ID)
		if err != nil || first.KeyHash == second.KeyHash {
			t.Fatalf("issued tokens must have distinct stored hashes: %v", err)
		}
	})

	t.Run("分组不存在时 404", func(t *testing.T) {
		f := createGroupService(t, map[string]string{})

		_, err := f.service.IssueKey(999, "")
		requireErrStatus(t, err, 404)
		requireErrMsg(t, err, "分组不存在")
	})
}

func TestGroupServiceToggleKey(t *testing.T) {
	t.Run("来回切 isActive，不删行", func(t *testing.T) {
		f := createGroupService(t, map[string]string{})
		group, key, _ := f.ensureSeed(t)

		off, err := f.service.ToggleKey(key.ID)
		if err != nil {
			t.Fatalf("ToggleKey: %v", err)
		}
		if off.IsActive {
			t.Error("isActive = true after first toggle, want false")
		}
		on, err := f.service.ToggleKey(key.ID)
		if err != nil {
			t.Fatalf("ToggleKey: %v", err)
		}
		if !on.IsActive {
			t.Error("isActive = false after second toggle, want true")
		}
		// 停用不是删除：已有订单的回调还要拿这把 Key 签名
		if n := groupKeyCount(t, f.store, group.ID); n != 1 {
			t.Errorf("key rows = %d, want 1 (disable is not delete)", n)
		}
	})

	t.Run("Key 不存在时 404", func(t *testing.T) {
		f := createGroupService(t, map[string]string{})

		_, err := f.service.ToggleKey(999)
		requireErrStatus(t, err, 404)
		requireErrMsg(t, err, "API Key 不存在")
	})
}

func TestGroupServiceToggleGroup(t *testing.T) {
	t.Run("停用分组不动它名下的 Key", func(t *testing.T) {
		// 守卫连表查 isActive，所以停分组就够了，不用逐把 Key 去停
		f := createGroupService(t, map[string]string{})
		group, _, _ := f.ensureSeed(t)

		off, err := f.service.ToggleGroup(group.ID)
		if err != nil {
			t.Fatalf("ToggleGroup: %v", err)
		}
		if off.IsActive {
			t.Error("isActive = true after toggle off, want false")
		}
		if len(off.Keys) == 0 || !off.Keys[0].IsActive {
			t.Errorf("keys[0].isActive = false, want untouched true (keys=%+v)", off.Keys)
		}

		on, err := f.service.ToggleGroup(group.ID)
		if err != nil {
			t.Fatalf("ToggleGroup: %v", err)
		}
		if !on.IsActive {
			t.Error("isActive = false after toggle on, want true")
		}
	})

	t.Run("分组不存在时 404", func(t *testing.T) {
		f := createGroupService(t, map[string]string{})

		_, err := f.service.ToggleGroup(999)
		requireErrStatus(t, err, 404)
		requireErrMsg(t, err, "分组不存在")
	})
}

func TestGroupServiceDefaultGroup(t *testing.T) {
	t.Run("回种子分组，给没有 Key 上下文的测试下单用", func(t *testing.T) {
		f := createGroupService(t, map[string]string{})
		seeded, _, _ := f.ensureSeed(t)

		group, err := f.service.DefaultGroup()
		if err != nil {
			t.Fatalf("DefaultGroup: %v", err)
		}
		if group.ID != seeded.ID {
			t.Errorf("defaultGroup.id = %d, want seeded id %d", group.ID, seeded.ID)
		}
	})
}
