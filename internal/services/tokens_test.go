package services

import (
	"encoding/json"
	"strings"
	"testing"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/signature"
)

func TestTokenServiceDefaultCreationAndOneTimeReceipt(t *testing.T) {
	f := createGroupService(t, map[string]string{})
	issued, err := f.service.IssueToken(nil, "本地开发")
	if err != nil {
		t.Fatal(err)
	}
	if !apiKeyPattern.MatchString(issued.Key) || issued.Name != "本地开发" || !issued.GroupIsActive {
		t.Fatal("creation receipt did not include an active credential")
	}
	groups, err := f.service.ListGroups()
	if err != nil || len(groups) != 1 || len(groups[0].Keys) != 1 {
		t.Fatalf("default creation should create only one project and token: %v", err)
	}
	listed, err := f.service.ListTokens(nil)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list created token: %v", err)
	}
	if listed[0].Key != "" || listed[0].KeyPrefix != issued.KeyPrefix || listed[0].KeySuffix != issued.KeySuffix {
		t.Fatal("list should return credential hints without the credential")
	}
	encoded, err := json.Marshal(listed)
	if err != nil || strings.Contains(string(encoded), issued.Key) || strings.Contains(string(encoded), `"key":`) {
		t.Fatalf("credential appeared in list serialization: %v", err)
	}
	toggled, err := f.service.ToggleKey(issued.ID)
	if err != nil || toggled.Key != "" || toggled.IsActive {
		t.Fatalf("disable receipt must not repeat credential: %v", err)
	}
}

func TestTokenServiceProjectCreationAndPausedDefault(t *testing.T) {
	f := createGroupService(t, map[string]string{})
	project, err := f.service.CreateProject("  项目 A  ")
	if err != nil || project == nil || project.Name != "项目 A" || len(project.Keys) != 0 || !project.WebhookConfigured {
		t.Fatalf("project creation should be independent of token issuance: %v", err)
	}
	other, err := f.service.CreateProject("项目 B")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ToggleGroup(project.ID); err != nil {
		t.Fatal(err)
	}
	_, err = f.service.IssueToken(nil, "default")
	requireErrStatus(t, err, 400)
	listed, err := f.service.ListTokens(nil)
	if err != nil || len(listed) != 0 {
		t.Fatalf("paused default must not silently create tokens elsewhere: %v", err)
	}
	issued, err := f.service.IssueToken(&other.ID, "另一项目")
	if err != nil || issued.GroupID != other.ID {
		t.Fatalf("explicit active project issuance: %v", err)
	}
	listed, err = f.service.ListTokens(&project.ID)
	if err != nil || len(listed) != 0 {
		t.Fatalf("project filter crossed project boundary: %v", err)
	}
}

func TestTokenServiceWebhookSecretIsIndependentAndOrderSnapshotSurvivesRotation(t *testing.T) {
	f := createGroupService(t, map[string]string{})
	issued, err := f.service.IssueToken(nil, "原令牌")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := f.service.WebhookSecret(issued.GroupID)
	if err != nil || secret.Secret == issued.Key || !strings.HasPrefix(secret.Secret, "whsec_") {
		t.Fatalf("project should have a separate webhook secret: %v", err)
	}
	second, err := f.service.IssueToken(&issued.GroupID, "轮换令牌")
	if err != nil {
		t.Fatal(err)
	}
	payments, _ := createPaymentService(f.store)
	external := "independent-secret-order"
	notifyURL := "https://merchant.example/notify"
	input := CreateOrderInput{Amount: 2.5, ProductName: "测试", PayMethod: "native", ExternalOrderNo: &external, NotifyURL: &notifyURL}
	created, err := payments.CreateOrder(OrderCaller{GroupID: issued.GroupID, APIKeyID: &issued.ID}, input)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Groups.Update(issued.GroupID, map[string]any{"webhookSecret": "whsec_future_rotation"}); err != nil {
		t.Fatal(err)
	}
	retry, err := payments.CreateOrder(OrderCaller{GroupID: issued.GroupID, APIKeyID: &second.ID}, input)
	if err != nil || retry.OrderNo != created.OrderNo {
		t.Fatalf("rotation retry should retain order: %v", err)
	}
	if _, err := f.service.ToggleKey(issued.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ToggleGroup(issued.GroupID); err != nil {
		t.Fatal(err)
	}
	trade := "INDEPENDENT_PAID"
	if _, err := payments.MarkPaid(created.OrderNo, &trade, 250, "CNY"); err != nil {
		t.Fatal(err)
	}
	if _, err := payments.MarkRefunded(created.OrderNo, 50, "INDEPENDENT_REFUND", nil); err != nil {
		t.Fatal(err)
	}
	callbacks, total, err := f.store.Notifies.Search(db.NotifySearchOpts{OrderNo: created.OrderNo, Limit: 10})
	if err != nil || total != 2 {
		t.Fatalf("existing callbacks should survive token and project disable: %v", err)
	}
	for _, callback := range callbacks {
		meta := parseSignature(callback.Signature)
		payload := signature.BuildPayload(callback.Body, meta.Timestamp, meta.Nonce)
		if meta.Version != 2 || !signature.Verify(payload, secret.Secret, meta.Sign) {
			t.Fatal("new callback did not use original project signing snapshot/version")
		}
		if signature.Verify(payload, issued.Key, meta.Sign) || signature.Verify(payload, second.Key, meta.Sign) || signature.Verify(payload, "whsec_future_rotation", meta.Sign) {
			t.Fatal("callback used an API token or a changed project secret")
		}
	}
}

func TestCallbackSignatureVersionHeaderPreservesLegacyAndWebhookModes(t *testing.T) {
	for _, test := range []struct{ metadata, header string }{
		{`{"timestamp":"1234567890","nonce":"abc123","sign":"sig"}`, "1"},
		{`{"timestamp":"1234567890","nonce":"abc123","sign":"sig","version":2}`, "2"},
	} {
		service, repo, client := createNotifyService()
		item := mockNotifyQueue(func(n *db.NotifyQueue) { n.Signature = test.metadata })
		if err := service.processNotify(item); err != nil {
			t.Fatal(err)
		}
		if client.lastPostHeaders["X-Sandbox-Pay-Signature-Version"] != test.header || client.lastPostHeaders["X-Sandbox-Pay-Signature"] != "sig" {
			t.Fatal("callback did not retain its signature/key-selection metadata")
		}
		if len(repo.updateCalls) != 1 || repo.patchOf(0)["status"] != db.NotifySuccess {
			t.Fatal("callback delivery was not recorded")
		}
	}
}
