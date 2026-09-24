package apiurl

import (
	"strings"
	"testing"
)

// Port of server/src/lib/api-url.spec.ts
func TestAPIURL(t *testing.T) {
	t.Run("prefixes the versioned api path", func(t *testing.T) {
		got := URL("http://localhost:3399", "alipay/notify")
		want := "http://localhost:3399/v1/api/alipay/notify"
		if got != want {
			t.Errorf("URL = %q, want %q", got, want)
		}
	})

	t.Run("strips a trailing slash from baseUrl", func(t *testing.T) {
		got := URL("http://localhost:3399/", "alipay/notify")
		want := "http://localhost:3399/v1/api/alipay/notify"
		if got != want {
			t.Errorf("URL = %q, want %q", got, want)
		}
	})

	t.Run("strips a leading slash from path", func(t *testing.T) {
		got := URL("http://localhost:3399", "/paypal/callback")
		want := "http://localhost:3399/v1/api/paypal/callback"
		if got != want {
			t.Errorf("URL = %q, want %q", got, want)
		}
	})

	t.Run("does not double the api segment", func(t *testing.T) {
		// 回归用例：helper 自带 /v1/api，拼出 /v1/api/api/xxx 第三方回调直接 404。
		got := URL("http://x", "alipay/notify")
		if strings.Contains(got, "/api/api/") {
			t.Errorf("URL = %q, must not contain /api/api/", got)
		}
	})

	t.Run("keeps the prefix in sync with the route mounting", func(t *testing.T) {
		if Prefix != "/v1/api" {
			t.Errorf("Prefix = %q, want %q", Prefix, "/v1/api")
		}
	})
}
