package signature

import (
	"regexp"
	"strings"
	"testing"
)

// Port of server/src/lib/signature.spec.ts
func TestSignature(t *testing.T) {
	t.Run("sign/produces a lowercase hex HMAC-SHA256", func(t *testing.T) {
		if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(Sign("hello", "secret")) {
			t.Errorf("Sign(hello, secret) = %q, want 64 lowercase hex chars", Sign("hello", "secret"))
		}
	})

	t.Run("sign/is deterministic", func(t *testing.T) {
		if Sign("payload", "key") != Sign("payload", "key") {
			t.Error("Sign should be deterministic for the same payload and secret")
		}
	})

	t.Run("sign/differs for different payloads", func(t *testing.T) {
		if Sign("payload1", "key") == Sign("payload2", "key") {
			t.Error("Sign should differ for different payloads")
		}
	})

	t.Run("sign/differs for different secrets", func(t *testing.T) {
		if Sign("payload", "key1") == Sign("payload", "key2") {
			t.Error("Sign should differ for different secrets")
		}
	})

	t.Run("verify/returns true for a valid signature", func(t *testing.T) {
		secret, payload := "test-secret", "test-payload"
		if !Verify(payload, secret, Sign(payload, secret)) {
			t.Error("Verify should return true for a valid signature")
		}
	})

	t.Run("verify/returns false for an invalid signature", func(t *testing.T) {
		if Verify("payload", "secret", "invalid-signature") {
			t.Error("Verify should return false for an invalid signature")
		}
	})

	t.Run("verify/returns false for empty signature", func(t *testing.T) {
		if Verify("payload", "secret", "") {
			t.Error("Verify should return false for an empty signature")
		}
	})

	t.Run("verify/returns false for null/undefined signature", func(t *testing.T) {
		// Go 的 string 没有 null/undefined —— 空串就是零值，已由上一个用例覆盖。
		var nullSig string
		if Verify("payload", "secret", nullSig) {
			t.Error("Verify should return false for a zero-value (null-equivalent) signature")
		}
	})

	t.Run("verify/is case-insensitive (normalizes to lowercase)", func(t *testing.T) {
		secret, payload := "test-secret", "test-payload"
		sig := Sign(payload, secret)
		if !Verify(payload, secret, strings.ToUpper(sig)) {
			t.Error("Verify should accept an uppercased signature")
		}
	})

	t.Run("buildPayload/concatenates body, timestamp, and nonce", func(t *testing.T) {
		got := BuildPayload(`{"amount":100}`, "1234567890", "nonce-abc")
		want := `{"amount":100}&timestamp=1234567890&nonce=nonce-abc`
		if got != want {
			t.Errorf("BuildPayload = %q, want %q", got, want)
		}
	})

	t.Run("createOrderNo/uppercases the prefix and appends hex", func(t *testing.T) {
		if !regexp.MustCompile(`^PAY[0-9A-F]{24}$`).MatchString(CreateOrderNo("pay", 12)) {
			t.Errorf("CreateOrderNo(pay, 12) = %q, want ^PAY[0-9A-F]{24}$", CreateOrderNo("pay", 12))
		}
	})

	t.Run("createOrderNo/does not repeat", func(t *testing.T) {
		seen := map[string]struct{}{}
		for i := 0; i < 200; i++ {
			// byteLength 传 0 走默认 12 字节 —— 等价于 TS 的缺省参数。
			seen[CreateOrderNo("PAY", 0)] = struct{}{}
		}
		if len(seen) != 200 {
			t.Errorf("expected 200 unique order numbers, got %d", len(seen))
		}
	})
}
