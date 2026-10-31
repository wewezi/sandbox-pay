package db

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestCredentialSecretsCannotLeakThroughModelJSON(t *testing.T) {
	secret := "private-signing-secret"
	models := []any{
		&Group{Name: "project", WebhookSecret: secret},
		&APIKey{Name: "token", Key: "private-token", KeyHash: "private-token-hash"},
		&PaymentOrder{OrderNo: "order", SigningSecret: &secret},
	}
	for _, model := range models {
		encoded, err := json.Marshal(model)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "private-") {
			t.Fatal("private credential material was serialized")
		}
	}
}

func TestAPIKeyPlaintextOnlyInIssuanceReceipt(t *testing.T) {
	store := newRepoStore(t)
	groupID := seedGroup(t, store)
	raw := "sp_sandbox_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	issued, err := store.ApiKeys.Insert(InsertKeyInput{GroupID: groupID, Name: "CI", Key: raw, IsActive: true})
	if err != nil || issued == nil || issued.Key != raw {
		t.Fatalf("issuance did not return the newly generated token: %v", err)
	}
	var stored string
	if err := store.db.QueryRow(`SELECT "key" FROM "api_keys" WHERE "id" = ?`, issued.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == raw || stored != HashAPIKey(raw) {
		t.Fatal("token was not stored as its one-way hash")
	}
	byID, err := store.ApiKeys.FindByID(issued.ID)
	if err != nil {
		t.Fatal(err)
	}
	byKey, err := store.ApiKeys.FindByKey(raw)
	if err != nil {
		t.Fatal(err)
	}
	hit, err := store.ApiKeys.FindActiveByKey(raw)
	if err != nil || hit == nil {
		t.Fatalf("original credential did not authenticate: %v", err)
	}
	listed, err := store.ApiKeys.ListByGroup(groupID)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list credential metadata: %v", err)
	}
	for _, key := range []*APIKey{byID, byKey, hit.Key, listed[0]} {
		if key == nil || key.Key != "" || key.KeyHash != stored {
			t.Fatal("subsequent lookup returned plaintext or incorrect metadata")
		}
		if key.KeyPrefix != "sp_sandbox_01234" || key.KeySuffix != "cdef" {
			t.Fatal("display metadata lost the recognizable token prefix/suffix")
		}
	}
	if hit, err := store.ApiKeys.FindActiveByKey(stored); err != nil || hit != nil {
		t.Fatalf("stored digest was accepted as a credential: %v", err)
	}
	if hit, err := store.ApiKeys.FindByKey(stored); err != nil || hit != nil {
		t.Fatalf("stored digest matched a plaintext lookup: %v", err)
	}
	if hit, err := store.ApiKeys.FindActiveByKey(strings.ToUpper(raw)); err != nil || hit != nil {
		t.Fatalf("credential case was ignored: %v", err)
	}
}

func TestLegacyShortTokenDisplayDoesNotRevealWholeCredential(t *testing.T) {
	store := newRepoStore(t)
	groupID := seedGroup(t, store)
	for _, raw := range []string{"a", "ab", "short", "sp_test_repo_key", "令牌示例值"} {
		issued, err := store.ApiKeys.Insert(InsertKeyInput{GroupID: groupID, Name: "legacy", Key: raw, IsActive: true})
		if err != nil {
			t.Fatal(err)
		}
		listed, err := store.ApiKeys.FindByID(issued.ID)
		if err != nil || listed == nil {
			t.Fatalf("read short credential metadata: %v", err)
		}
		if listed.Key != "" || len([]rune(listed.KeyPrefix))+len([]rune(listed.KeySuffix)) >= len([]rune(raw)) {
			t.Fatal("short credential was fully exposed by its display metadata")
		}
	}
}

func TestProjectWebhookSecretsAreIndependentAndStable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "webhooks.db")
	store, err := Open(path, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	group, err := store.Groups.Insert("project", true)
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.Groups.Insert("other project", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(group.WebhookSecret, "whsec_") || len(group.WebhookSecret) != 70 || group.WebhookSecret == other.WebhookSecret {
		t.Fatal("projects did not get independent webhook secrets")
	}
	key, err := store.ApiKeys.Insert(InsertKeyInput{GroupID: group.ID, Name: "old token", Key: "sp_old_token", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ApiKeys.Update(key.ID, map[string]any{"isActive": false}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApiKeys.Insert(InsertKeyInput{GroupID: group.ID, Name: "replacement", Key: "sp_new_token", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.Groups.Update(group.ID, map[string]any{"isActive": false}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	stored, err := reopened.Groups.FindByID(group.ID)
	if err != nil || stored == nil || stored.WebhookSecret != group.WebhookSecret || stored.IsActive {
		t.Fatalf("token rotation or reopening changed webhook configuration: %v", err)
	}
}

func version2CredentialsDatabase(t *testing.T) string {
	t.Helper()
	path := version1Database(t)
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Exec(`ALTER TABLE "payment_orders" ADD COLUMN "clientIp" TEXT;
ALTER TABLE "payment_orders" ADD COLUMN "clientLocation" TEXT;
UPDATE "api_keys" SET "isActive" = 1 WHERE "id" = 1;
UPDATE "payment_orders" SET "notifyUrl" = 'https://merchant.example/notify' WHERE "orderNo" = 'A';
INSERT INTO "api_keys" ("groupId", "name", "key", "isActive") VALUES (1, 'disabled token', 'disabled-old-key', 0);
INSERT INTO "payment_orders" ("id", "orderNo", "amount", "productName", "payMethod", "expireAt", "groupId", "apiKeyId", "notifyUrl")
VALUES ('b', 'B', 200, 'disabled-token order', 'native', '2030-01-01 00:00:00.000', 1, 2, 'https://merchant.example/notify');
PRAGMA user_version = 2;`); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVersion2CredentialsMigrationPreservesLegacyCallbacks(t *testing.T) {
	path := version2CredentialsDatabase(t)
	store, err := Open(path, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	hit, err := store.ApiKeys.FindActiveByKey("old-key")
	if err != nil || hit == nil || hit.Key.Key != "" || hit.Key.KeyHash != HashAPIKey("old-key") {
		t.Fatalf("original Bearer token was not preserved: %v", err)
	}
	if denied, err := store.ApiKeys.FindActiveByKey("disabled-old-key"); err != nil || denied != nil {
		t.Fatalf("migration enabled a disabled token: %v", err)
	}
	for _, test := range []struct{ orderNo, secret string }{{"A", "old-key"}, {"B", "disabled-old-key"}} {
		order, err := store.Orders.FindByOrderNo(test.orderNo, true)
		if err != nil || order == nil || order.SigningSecret == nil || *order.SigningSecret != test.secret || order.SignatureVersion != 1 {
			t.Fatalf("legacy order %s lost signing compatibility: %v", test.orderNo, err)
		}
		if order.APIKey == nil || order.APIKey.Key != "" || order.APIKey.KeyHash != HashAPIKey(test.secret) {
			t.Fatal("joined credential returned plaintext after migration")
		}
	}
	var rawMatches, version int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM "api_keys" WHERE "key" IN ('old-key', 'disabled-old-key')`).Scan(&rawMatches); err != nil || rawMatches != 0 {
		t.Fatalf("plaintext credentials remained in API storage: count=%d err=%v", rawMatches, err)
	}
	if err := store.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 5 {
		t.Fatalf("credential migration version=%d err=%v", version, err)
	}
	group, err := store.Groups.FindByID(1)
	if err != nil || group == nil || !strings.HasPrefix(group.WebhookSecret, "whsec_") || group.WebhookSecret == "old-key" {
		t.Fatalf("independent webhook secret missing: %v", err)
	}
	notifications, err := store.Notifies.ListByOrderNo("A")
	if err != nil || len(notifications) != 1 || notifications[0].Signature != "old-signature" {
		t.Fatalf("queued callback changed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	after, err := reopened.Groups.FindByID(1)
	if err != nil || after == nil || after.WebhookSecret != group.WebhookSecret {
		t.Fatalf("reopening regenerated project secret: %v", err)
	}
	order, err := reopened.Orders.FindByOrderNo("A", false)
	if err != nil || order == nil || order.SigningSecret == nil || *order.SigningSecret != "old-key" {
		t.Fatalf("reopening changed legacy callback snapshot: %v", err)
	}
	if hit, err := reopened.ApiKeys.FindActiveByKey("old-key"); err != nil || hit == nil {
		t.Fatalf("reopening double-hashed old token: %v", err)
	}
}

func TestCredentialMigrationFailureRollsBackSnapshotsAndHashes(t *testing.T) {
	path := version2CredentialsDatabase(t)
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`CREATE TRIGGER reject_hash BEFORE UPDATE OF "key" ON "api_keys" BEGIN SELECT RAISE(ABORT, 'reject hash'); END;`); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	store, err := Open(path, "")
	if err == nil {
		store.Close()
		t.Fatal("forced hashing failure did not abort migration")
	}
	conn, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var version, newColumns int
	var key, signature string
	if err := conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 2 {
		t.Fatalf("failed credential migration changed version: %d %v", version, err)
	}
	if err := conn.QueryRow(`SELECT "key" FROM "api_keys" WHERE "id" = 1`).Scan(&key); err != nil || key != "old-key" {
		t.Fatalf("failed migration partially hashed credentials: %v", err)
	}
	if err := conn.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('payment_orders') WHERE name IN ('signingSecret', 'signatureVersion')`).Scan(&newColumns); err != nil || newColumns != 0 {
		t.Fatalf("failed migration kept signing columns: %d %v", newColumns, err)
	}
	if err := conn.QueryRow(`SELECT "signature" FROM "notify_queues" WHERE "orderNo" = 'A'`).Scan(&signature); err != nil || signature != "old-signature" {
		t.Fatalf("failed migration changed queued callback: %v", err)
	}
}
