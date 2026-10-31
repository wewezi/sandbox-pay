package httpx

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"sandbox-pay/internal/config"
)

func TestProxyHeaders(t *testing.T) {
	for _, test := range []struct {
		name, cidrs, peer, forwarded, host, proto, wantIP, wantBase string
	}{
		{"untrusted", "", "192.0.2.1:1234", "1.2.3.4", "evil.example", "https", "192.0.2.1", "http://origin.example"},
		{"trusted", "127.0.0.1/32", "127.0.0.1:1234", "1.2.3.4", "public.example", "https", "1.2.3.4", "https://public.example"},
		{"stop at first untrusted", "127.0.0.1/32,10.0.0.0/8", "127.0.0.1:1234", "6.6.6.6, 1.2.3.4, 10.0.0.5", "public.example", "https", "1.2.3.4", "https://public.example"},
		{"malformed chain", "127.0.0.1/32", "127.0.0.1:1234", "spoofed, 1.2.3.4", "", "", "127.0.0.1", "http://origin.example"},
		{"unspecified address", "127.0.0.1/32", "127.0.0.1:1234", "0.0.0.0", "", "", "127.0.0.1", "http://origin.example"},
		{"invalid forwarded metadata", "127.0.0.1/32", "127.0.0.1:1234", "1.2.3.4", "evil.example/path", "javascript", "1.2.3.4", "http://origin.example"},
		{"IPv6", "::1/128", "[::1]:1234", "2001:4860:4860::8888", "[2001:4860:4860::8888]:443", "https", "2001:4860:4860::8888", "https://[2001:4860:4860::8888]:443"},
		{"invalid config", "bad-cidr", "127.0.0.1:1234", "1.2.3.4", "evil.example", "https", "127.0.0.1", "http://origin.example"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://origin.example/", nil)
			r.RemoteAddr = test.peer
			r.Header.Set("X-Forwarded-For", test.forwarded)
			r.Header.Set("X-Forwarded-Host", test.host)
			r.Header.Set("X-Forwarded-Proto", test.proto)
			handler := ProxyHeaders(config.Map{"TRUSTED_PROXY_CIDRS": test.cidrs})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if ip, base := ClientIP(r), BaseURL(r); ip != test.wantIP || base != test.wantBase {
					t.Fatalf("got %q, %q; want %q, %q", ip, base, test.wantIP, test.wantBase)
				}
			}))
			handler.ServeHTTP(httptest.NewRecorder(), r)
		})
	}
}

func TestProxyHeadersIncludesMultipleHeaderLines(t *testing.T) {
	r := httptest.NewRequest("GET", "http://origin.example/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Add("X-Forwarded-For", "6.6.6.6")
	r.Header.Add("X-Forwarded-For", "1.2.3.4")
	r.Header.Add("X-Forwarded-Host", "evil.example")
	r.Header.Add("X-Forwarded-Host", "public.example")
	r.Header.Add("X-Forwarded-Proto", "http")
	r.Header.Add("X-Forwarded-Proto", "https")
	handler := ProxyHeaders(config.Map{"TRUSTED_PROXY_CIDRS": "127.0.0.1/32"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ClientIP(r) != "1.2.3.4" || BaseURL(r) != "https://public.example" {
			t.Fatalf("used forged headers preceding the proxy headers: %q %q", ClientIP(r), BaseURL(r))
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), r)
}

func TestBaseURLWithoutProxyMiddleware(t *testing.T) {
	r := httptest.NewRequest("GET", "https://origin.example/", nil)
	r.TLS = &tls.ConnectionState{}
	r.Header.Set("X-Forwarded-Host", "evil.example")
	r.Header.Set("X-Forwarded-Proto", "http")
	if got := BaseURL(r); got != "https://origin.example" {
		t.Fatal(got)
	}
}
