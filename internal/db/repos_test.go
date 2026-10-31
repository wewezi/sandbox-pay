package db

// Port of server/src/db/repos.spec.ts —— 数据层对着真库跑。
//
// 挑的都是「只有这一层知道」的约定：日期存成什么文本、nil 怎么落成 NULL、
// 过期扫描扫哪些行、绑参碰到对象要不要拦。这些错了不会让哪个 service 的
// 测试变红，只会在某天半夜静默写坏一列。
//
// 每个用例一个 t.TempDir() 上的独立库，不共享单例。

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TS 的 TEST_API_KEY（test-app.ts 的种子 Key）。这里自建种子，值等价。
const testAPIKey = "sp_test_repo_key"

// newRepoStore 打开一个一次性真库，测完自动关。
func newRepoStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"), "")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// seedGroup 建种子分组 —— 新库里的第一条，id 必然是 1（订单 FK 要用）。
func seedGroup(t *testing.T, s *Store) int64 {
	t.Helper()
	g, err := s.Groups.Insert("seed", true)
	if err != nil {
		t.Fatalf("seed group: %v", err)
	}
	return g.ID
}

// seedKey 建一把种子 Key。
func seedKey(t *testing.T, s *Store, groupID int64, key string) int64 {
	t.Helper()
	k, err := s.ApiKeys.Insert(InsertKeyInput{GroupID: groupID, Name: "测试 Key", Key: key, IsActive: true})
	if err != nil {
		t.Fatalf("seed api key: %v", err)
	}
	return k.ID
}

// 订单号/id 计数器，保证同库内不撞 UNIQUE。
var orderSeq atomic.Int64

// makeOrder 对应 TS 的 makeOrder(overrides)：先给默认值，调用方再改字段。
func makeOrder() *PaymentOrder {
	n := orderSeq.Add(1)
	now := time.Now().UTC()
	groupID := int64(1)
	return &PaymentOrder{
		ID:             fmt.Sprintf("ord%012d", n),
		OrderNo:        fmt.Sprintf("PAY%012d", n),
		Amount:         10000,
		ProductName:    "repo test",
		PayMethod:      "native",
		Status:         StatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
		ExpireAt:       now.Add(30 * time.Minute),
		RefundedAmount: 0,
		ChannelCost:    0,
		Fee:            0,
		SettleAmount:   0,
		UserID:         0,
		GroupID:        &groupID,
	}
}

func strPtr(s string) *string { return &s }
func i64Ptr(v int64) *int64   { return &v }

// insertOrder 建单并要求成功。
func insertOrder(t *testing.T, s *Store, o *PaymentOrder) {
	t.Helper()
	if err := s.Orders.Insert(o); err != nil {
		t.Fatalf("insert order %s: %v", o.OrderNo, err)
	}
}

func TestDatetimeTextFormat(t *testing.T) {
	orig := time.Date(2026, 3, 4, 5, 6, 7, 8_000_000, time.UTC)

	t.Run("存的是空格分隔而不是 ISO 的 T（否则文本排序会乱）", func(t *testing.T) {
		text := toSQL(orig)
		if text != "2026-03-04 05:06:07.008" {
			t.Errorf("toSQL = %q, want %q", text, "2026-03-04 05:06:07.008")
		}
		if strings.Contains(text, "T") {
			t.Errorf("toSQL = %q, must not contain T", text)
		}
		if strings.Contains(text, "Z") {
			t.Errorf("toSQL = %q, must not contain Z", text)
		}
	})

	t.Run("往返不丢毫秒，也不偏时区", func(t *testing.T) {
		if got := fromSQL(toSQL(orig)); !got.Equal(orig) {
			t.Errorf("fromSQL(toSQL(orig)) = %v, want %v", got, orig)
		}
	})

	t.Run("读老库里带 T 的值也认", func(t *testing.T) {
		got := fromSQL("2026-03-04T05:06:07.008Z")
		if !got.Equal(orig) {
			t.Errorf(`fromSQL("2026-03-04T05:06:07.008Z") = %v, want %v`, got, orig)
		}
	})

	t.Run("空格格式排序在 T 格式之前 —— 混存就会乱序", func(t *testing.T) {
		// 这条钉住的是「为什么不能改成 ISO」：0x20 < 0x54
		if !("2026-03-04 05:06:07.008" < "2026-03-04T00:00:00.000") {
			t.Error("space format must sort before T format")
		}
	})

	t.Run("nowSql 产出的就是同一种格式", func(t *testing.T) {
		if !regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]{3}$`).
			MatchString(nowSQL()) {
			t.Errorf("nowSQL = %q, want YYYY-MM-DD HH:MM:SS.sss", nowSQL())
		}
	})
}

func TestOrdersRoundTrip(t *testing.T) {
	t.Run("写进去再读出来，日期是时间、可空列是 nil", func(t *testing.T) {
		s := newRepoStore(t)
		seedGroup(t, s)
		o := makeOrder()
		notifyURL := "https://example.com/hook"
		o.NotifyURL = strPtr(notifyURL)
		insertOrder(t, s, o)

		found, err := s.Orders.FindByOrderNo(o.OrderNo, false)
		if err != nil || found == nil {
			t.Fatalf("FindByOrderNo = (%v, %v), want a row", found, err)
		}
		if found.OrderNo != o.OrderNo {
			t.Errorf("OrderNo = %q, want %q", found.OrderNo, o.OrderNo)
		}
		if found.Amount != 10000 {
			t.Errorf("Amount = %d, want 10000", found.Amount)
		}
		// 日期列读回来必须是解析过的有效时间（fromSQL 失败会给零值）
		if found.CreatedAt.IsZero() {
			t.Error("CreatedAt is zero — datetime text did not round-trip")
		}
		if found.ExpireAt.IsZero() {
			t.Error("ExpireAt is zero — datetime text did not round-trip")
		}
		// 可空列统一是 nil，不混用
		if found.PayAt != nil {
			t.Errorf("PayAt = %v, want nil", found.PayAt)
		}
		if found.ThirdPartyTradeNo != nil {
			t.Errorf("ThirdPartyTradeNo = %v, want nil", found.ThirdPartyTradeNo)
		}
		if found.NotifyURL == nil || *found.NotifyURL != notifyURL {
			t.Errorf("NotifyURL = %v, want %q", found.NotifyURL, notifyURL)
		}
	})

	t.Run("withApiKey 才 JOIN 出 apiKey，默认不带", func(t *testing.T) {
		s := newRepoStore(t)
		seedGroup(t, s)
		keyID := seedKey(t, s, 1, testAPIKey)
		o := makeOrder()
		o.APIKeyID = i64Ptr(keyID)
		insertOrder(t, s, o)

		plain, err := s.Orders.FindByOrderNo(o.OrderNo, false)
		if err != nil || plain == nil {
			t.Fatalf("FindByOrderNo = (%v, %v), want a row", plain, err)
		}
		if plain.APIKey != nil {
			t.Errorf("default query returned apiKey %+v, want nil", plain.APIKey)
		}

		withKey, err := s.Orders.FindByOrderNo(o.OrderNo, true)
		if err != nil || withKey == nil {
			t.Fatalf("FindByOrderNo(withAPIKey) = (%v, %v), want a row", withKey, err)
		}
		if withKey.APIKey == nil {
			t.Fatal("withAPIKey query returned nil apiKey, want the joined row")
		}
		if withKey.APIKey.Key != "" || withKey.APIKey.KeyHash != HashAPIKey(testAPIKey) {
			t.Error("joined API metadata must retain only the token hash")
		}
	})

	t.Run("没挂 apiKeyId 的订单 JOIN 也拿不到 Key", func(t *testing.T) {
		s := newRepoStore(t)
		seedGroup(t, s)
		seedKey(t, s, 1, testAPIKey) // Key 存在，但订单没挂它
		o := makeOrder()
		insertOrder(t, s, o)

		found, err := s.Orders.FindByOrderNo(o.OrderNo, true)
		if err != nil || found == nil {
			t.Fatalf("FindByOrderNo(withAPIKey) = (%v, %v), want a row", found, err)
		}
		if found.APIKey != nil {
			t.Errorf("apiKey = %+v, want nil for an order without apiKeyId", found.APIKey)
		}
	})

	t.Run("按分组查单查不到别人的订单", func(t *testing.T) {
		s := newRepoStore(t)
		seedGroup(t, s)
		o := makeOrder()
		insertOrder(t, s, o)

		hit, err := s.Orders.FindByOrderNoAndGroup(o.OrderNo, 1)
		if err != nil || hit == nil {
			t.Fatalf("FindByOrderNoAndGroup(1) = (%v, %v), want a row", hit, err)
		}
		miss, err := s.Orders.FindByOrderNoAndGroup(o.OrderNo, 999)
		if err != nil {
			t.Fatalf("FindByOrderNoAndGroup(999): %v", err)
		}
		if miss != nil {
			t.Errorf("FindByOrderNoAndGroup(999) = %+v, want nil", miss)
		}
	})

	t.Run("update 只改传进来的列", func(t *testing.T) {
		s := newRepoStore(t)
		seedGroup(t, s)
		o := makeOrder()
		insertOrder(t, s, o)

		if err := s.Orders.Update(o.OrderNo, map[string]any{
			"status": StatusPaid, "thirdPartyTradeNo": "T1",
		}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		found, err := s.Orders.FindByOrderNo(o.OrderNo, false)
		if err != nil || found == nil {
			t.Fatalf("FindByOrderNo = (%v, %v), want a row", found, err)
		}
		if found.Status != StatusPaid {
			t.Errorf("Status = %d, want %d", found.Status, StatusPaid)
		}
		if found.ThirdPartyTradeNo == nil || *found.ThirdPartyTradeNo != "T1" {
			t.Errorf("ThirdPartyTradeNo = %v, want T1", found.ThirdPartyTradeNo)
		}
		if found.ProductName != "repo test" {
			t.Errorf("ProductName = %q, want %q (update must not touch it)", found.ProductName, "repo test")
		}
	})
}

func TestExpirePending(t *testing.T) {
	t.Run("只关掉已过期的 Pending，返回条数", func(t *testing.T) {
		s := newRepoStore(t)
		seedGroup(t, s)

		stale := makeOrder()
		stale.ExpireAt = time.Now().UTC().Add(-time.Minute)
		fresh := makeOrder()
		paidButStale := makeOrder()
		paidButStale.Status = StatusPaid
		paidButStale.ExpireAt = time.Now().UTC().Add(-time.Minute)
		for _, o := range []*PaymentOrder{stale, fresh, paidButStale} {
			insertOrder(t, s, o)
		}

		closed, err := s.Orders.ExpirePending(100)
		if err != nil {
			t.Fatalf("ExpirePending: %v", err)
		}
		if closed != 1 {
			t.Errorf("ExpirePending = %d, want 1", closed)
		}

		check := func(o *PaymentOrder, want int, label string) {
			t.Helper()
			found, err := s.Orders.FindByOrderNo(o.OrderNo, false)
			if err != nil || found == nil {
				t.Fatalf("%s:FindByOrderNo = (%v, %v), want a row", label, found, err)
			}
			if found.Status != want {
				t.Errorf("%s status = %d, want %d", label, found.Status, want)
			}
		}
		check(stale, StatusExpired, "过期的 Pending")
		check(fresh, StatusPending, "没到期的不动")
		check(paidButStale, StatusPaid, "已付款的不管过没过期都不动")
	})
}

func TestSearchFilters(t *testing.T) {
	t.Run("按 keyword 模糊匹配 orderNo / externalOrderNo", func(t *testing.T) {
		s := newRepoStore(t)
		seedGroup(t, s)
		a := makeOrder()
		a.ExternalOrderNo = strPtr("SHOP-AAA-1")
		b := makeOrder()
		b.ExternalOrderNo = strPtr("SHOP-BBB-2")
		insertOrder(t, s, a)
		insertOrder(t, s, b)

		items, total, err := s.Orders.Search(SearchOpts{Keyword: "AAA", Limit: 50})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
		if len(items) != 1 || items[0].ExternalOrderNo == nil || *items[0].ExternalOrderNo != "SHOP-AAA-1" {
			t.Errorf("items = %+v, want one row with externalOrderNo SHOP-AAA-1", items)
		}
	})

	t.Run("按 status / payMethod 过滤", func(t *testing.T) {
		s := newRepoStore(t)
		seedGroup(t, s)
		paid := makeOrder()
		paid.Status = StatusPaid
		alipay := makeOrder()
		alipay.PayMethod = "alipay"
		insertOrder(t, s, paid)
		insertOrder(t, s, alipay)

		_, totalPaid, err := s.Orders.Search(SearchOpts{Status: intPtr(StatusPaid), Limit: 50})
		if err != nil {
			t.Fatalf("Search(status): %v", err)
		}
		if totalPaid != 1 {
			t.Errorf("status filter total = %d, want 1", totalPaid)
		}

		_, totalAlipay, err := s.Orders.Search(SearchOpts{PayMethod: "alipay", Limit: 50})
		if err != nil {
			t.Fatalf("Search(payMethod): %v", err)
		}
		if totalAlipay != 1 {
			t.Errorf("payMethod filter total = %d, want 1", totalAlipay)
		}
	})

	t.Run("stats 只把 Paid 计入成功和金额", func(t *testing.T) {
		s := newRepoStore(t)
		seedGroup(t, s)
		paid := makeOrder()
		paid.Status = StatusPaid
		paid.Amount = 500
		pending := makeOrder()
		pending.Amount = 700
		insertOrder(t, s, paid)
		insertOrder(t, s, pending)

		stats, err := s.Orders.Stats(nil)
		if err != nil {
			t.Fatalf("Stats: %v", err)
		}
		if stats.TotalCount != 2 {
			t.Errorf("totalCount = %d, want 2", stats.TotalCount)
		}
		if stats.SuccessCount != 1 {
			t.Errorf("successCount = %d, want 1", stats.SuccessCount)
		}
		if stats.TotalAmountCents != 500 {
			t.Errorf("totalAmountCents = %d, want 500", stats.TotalAmountCents)
		}
	})
}

// intPtr SearchOpts.Status 是 *int。
func intPtr(v int) *int { return &v }

func TestApiKeys(t *testing.T) {
	t.Run("findActiveByKey 一次 JOIN 把 Key 和分组都拿回来", func(t *testing.T) {
		s := newRepoStore(t)
		seedGroup(t, s)
		seedKey(t, s, 1, testAPIKey)

		hit, err := s.ApiKeys.FindActiveByKey(testAPIKey)
		if err != nil || hit == nil {
			t.Fatalf("FindActiveByKey = (%v, %v), want a hit", hit, err)
		}
		if hit.Key.Key != "" || hit.Key.KeyHash != HashAPIKey(testAPIKey) {
			t.Error("authentication lookup returned plaintext or the wrong hash")
		}
		if hit.Group.ID != hit.Key.GroupID {
			t.Errorf("group.id = %d, want key.groupId %d", hit.Group.ID, hit.Key.GroupID)
		}
		if !hit.Group.IsActive {
			t.Error("group.isActive = false, want true")
		}
	})

	t.Run("Key 停用后 findActiveByKey 查不到", func(t *testing.T) {
		s := newRepoStore(t)
		seedGroup(t, s)
		extra, err := s.ApiKeys.Insert(InsertKeyInput{
			GroupID: 1, Name: "临时", Key: "sp_temp_inactive", IsActive: true,
		})
		if err != nil {
			t.Fatalf("Insert: %v", err)
		}
		if err := s.ApiKeys.Update(extra.ID, map[string]any{"isActive": false}); err != nil {
			t.Fatalf("Update: %v", err)
		}

		hit, err := s.ApiKeys.FindActiveByKey("sp_temp_inactive")
		if err != nil {
			t.Fatalf("FindActiveByKey: %v", err)
		}
		if hit != nil {
			t.Errorf("FindActiveByKey = %+v, want nil for a disabled key", hit)
		}
		// Credential metadata remains; order callback secrets are separate.
		row, err := s.ApiKeys.FindByID(extra.ID)
		if err != nil || row == nil {
			t.Fatalf("FindByID = (%v, %v), want the row kept", row, err)
		}
		if row.Key != "" || row.KeyHash != HashAPIKey("sp_temp_inactive") {
			t.Error("disabled token metadata must retain only the token hash")
		}
	})

	t.Run("分组停用后它名下的 Key 也查不到", func(t *testing.T) {
		s := newRepoStore(t)
		seedGroup(t, s)
		seedKey(t, s, 1, testAPIKey)

		if _, err := s.db.Exec(`UPDATE "groups" SET "isActive" = 0 WHERE "id" = 1`); err != nil {
			t.Fatalf("deactivate group: %v", err)
		}
		defer func() {
			if _, err := s.db.Exec(`UPDATE "groups" SET "isActive" = 1 WHERE "id" = 1`); err != nil {
				t.Errorf("restore group: %v", err)
			}
		}()

		hit, err := s.ApiKeys.FindActiveByKey(testAPIKey)
		if err != nil {
			t.Fatalf("FindActiveByKey: %v", err)
		}
		if hit != nil {
			t.Errorf("FindActiveByKey = %+v, want nil while the group is disabled", hit)
		}
	})

	t.Run("key 列 UNIQUE，重复插入直接炸", func(t *testing.T) {
		s := newRepoStore(t)
		seedGroup(t, s)
		seedKey(t, s, 1, testAPIKey)

		if _, err := s.ApiKeys.Insert(InsertKeyInput{
			GroupID: 1, Name: "撞车", Key: testAPIKey, IsActive: true,
		}); err == nil {
			t.Error("duplicate key insert must fail on UNIQUE constraint")
		}
	})

	t.Run("touch 只刷 lastUsedAt，不改别的列", func(t *testing.T) {
		s := newRepoStore(t)
		seedGroup(t, s)
		extra, err := s.ApiKeys.Insert(InsertKeyInput{
			GroupID: 1, Name: "touch 测", Key: "sp_temp_touch", IsActive: true,
		})
		if err != nil {
			t.Fatalf("Insert: %v", err)
		}
		if extra.LastUsedAt != nil {
			t.Errorf("lastUsedAt = %v right after insert, want nil", extra.LastUsedAt)
		}

		if err := s.ApiKeys.Touch(extra.ID); err != nil {
			t.Fatalf("Touch: %v", err)
		}

		after, err := s.ApiKeys.FindByID(extra.ID)
		if err != nil || after == nil {
			t.Fatalf("FindByID = (%v, %v), want a row", after, err)
		}
		if after.LastUsedAt == nil || after.LastUsedAt.IsZero() {
			t.Errorf("lastUsedAt = %v, want a set time", after.LastUsedAt)
		}
		if after.Name != "touch 测" {
			t.Errorf("name = %q, want %q", after.Name, "touch 测")
		}
		if !after.IsActive {
			t.Error("isActive = false, want true")
		}
	})
}

func TestNotifyQueue(t *testing.T) {
	insertNotify := func(t *testing.T, s *Store, status int) {
		t.Helper()
		if err := s.Notifies.Insert(InsertNotifyInput{
			URL: "https://x.test/h", Body: "{}", Signature: "{}", Status: status,
		}); err != nil {
			t.Fatalf("insert notify: %v", err)
		}
	}

	t.Run("findRetryable 只取 Pending / Failed", func(t *testing.T) {
		s := newRepoStore(t)
		for _, status := range []int{NotifyPending, NotifyFailed, NotifySuccess, NotifyExhausted} {
			insertNotify(t, s, status)
		}

		items, err := s.Notifies.FindRetryable(50)
		if err != nil {
			t.Fatalf("FindRetryable: %v", err)
		}
		if len(items) != 2 {
			t.Errorf("FindRetryable len = %d, want 2", len(items))
		}
	})

	t.Run("显式传 nil 会把列写成 NULL（重放要靠这个清 lastError）", func(t *testing.T) {
		s := newRepoStore(t)
		insertNotify(t, s, NotifyFailed)
		items, err := s.Notifies.FindRetryable(50)
		if err != nil || len(items) != 1 {
			t.Fatalf("FindRetryable = (%v, %v), want one row", items, err)
		}
		id := items[0].ID

		if err := s.Notifies.Update(id, map[string]any{"lastError": "boom"}); err != nil {
			t.Fatalf("Update(lastError=boom): %v", err)
		}
		row, err := s.Notifies.FindByID(id)
		if err != nil || row == nil {
			t.Fatalf("FindByID = (%v, %v), want a row", row, err)
		}
		if row.LastError == nil || *row.LastError != "boom" {
			t.Errorf("lastError = %v, want boom", row.LastError)
		}

		if err := s.Notifies.Update(id, map[string]any{"lastError": nil}); err != nil {
			t.Fatalf("Update(lastError=nil): %v", err)
		}
		row, err = s.Notifies.FindByID(id)
		if err != nil || row == nil {
			t.Fatalf("FindByID = (%v, %v), want a row", row, err)
		}
		if row.LastError != nil {
			t.Errorf("lastError = %v, want nil", row.LastError)
		}
		// 底层必须是字面 NULL，不是空串
		var raw sql.NullString
		if err := s.db.QueryRow(`SELECT "lastError" FROM "notify_queues" WHERE "id" = ?`, id).Scan(&raw); err != nil {
			t.Fatalf("raw select: %v", err)
		}
		if raw.Valid {
			t.Errorf("raw lastError = %q, want SQL NULL", raw.String)
		}
	})

	t.Run("clear 把自增序列一起归零", func(t *testing.T) {
		s := newRepoStore(t)
		insertNotify(t, s, NotifyPending)
		if err := s.Notifies.Clear(); err != nil {
			t.Fatalf("Clear: %v", err)
		}
		insertNotify(t, s, NotifyPending)

		items, err := s.Notifies.FindRetryable(50)
		if err != nil || len(items) != 1 {
			t.Fatalf("FindRetryable = (%v, %v), want one row", items, err)
		}
		if items[0].ID != 1 {
			t.Errorf("id = %d, want 1 (sequence must reset with clear)", items[0].ID)
		}
	})
}

func TestSQLBind(t *testing.T) {
	t.Run("nil 都变 NULL", func(t *testing.T) {
		if got := bindValue(nil); got != nil {
			t.Errorf("bindValue(nil) = %#v, want nil", got)
		}
	})

	t.Run("Date 转成 SQL 文本，boolean 转 0/1", func(t *testing.T) {
		got := bindValue(time.Date(2026, 3, 4, 5, 6, 7, 8_000_000, time.UTC))
		if s, ok := got.(string); !ok || s != "2026-03-04 05:06:07.008" {
			t.Errorf("bindValue(time) = %#v, want %q", got, "2026-03-04 05:06:07.008")
		}
		if v := bindValue(true); v != 1 {
			t.Errorf("bindValue(true) = %#v, want 1", v)
		}
		if v := bindValue(false); v != 0 {
			t.Errorf("bindValue(false) = %#v, want 0", v)
		}
	})

	t.Run("传对象直接抛，不静默写成 \"[object Object]\"", func(t *testing.T) {
		// Go 版等价不变式：bindValue 不把对象字符串化成 "[object Object]"，
		// 并且驱动在绑定不支持的类型时必须报错、拒绝写入。
		obj := map[string]any{"a": 1}
		got := bindValue(obj)
		if s, ok := got.(string); ok && s == "[object Object]" {
			t.Fatalf("bindValue must not stringify objects, got %q", s)
		}

		s := newRepoStore(t)
		if _, err := s.db.Exec(`INSERT INTO "notify_queues" ("url", "body", "signature") VALUES (?, ?, ?)`,
			obj, "{}", "{}"); err == nil {
			t.Error("binding an object must be rejected by the driver, not silently coerced")
		}
	})

	t.Run("setClause 只取显式出现的 key", func(t *testing.T) {
		set, args := setClause(map[string]any{"a": 1, "b": nil})
		// Go 的 map 无序，两种列序都算对；但两列必须都出现。
		if set != `"a" = ?, "b" = ?` && set != `"b" = ?, "a" = ?` {
			t.Fatalf("setClause sql = %q", set)
		}
		if len(args) != 2 {
			t.Fatalf("setClause args = %#v, want 2 args", args)
		}
		// 位置必须和列对应：a → 1，b → NULL
		if strings.HasPrefix(set, `"a"`) {
			if args[0] != int64(1) || args[1] != nil {
				t.Errorf("args = %#v, want [int64(1) nil]", args)
			}
		} else {
			if args[0] != nil || args[1] != int64(1) {
				t.Errorf("args = %#v, want [nil int64(1)]", args)
			}
		}
	})

	t.Run("Where 空条件不拼出光秃秃的 WHERE", func(t *testing.T) {
		if got := (&Where{}).SQL(); got != "" {
			t.Errorf("empty Where.SQL() = %q, want \"\"", got)
		}
		w := &Where{}
		w.AddIf("", `"a" = ?`, "")
		if got := w.SQL(); got != "" {
			t.Errorf("AddIf(\"\").SQL() = %q, want \"\"", got)
		}
		w2 := &Where{}
		w2.AddIf("x", `"a" = ?`, "x")
		if got := w2.SQL(); got != ` WHERE "a" = ?` {
			t.Errorf("AddIf(x).SQL() = %q, want %q", got, ` WHERE "a" = ?`)
		}
	})
}
