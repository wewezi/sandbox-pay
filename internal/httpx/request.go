package httpx

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"sandbox-pay/internal/config"
)

type proxyInfoKey struct{}

type proxyInfo struct {
	ip   string
	base string
}

// ProxyHeaders only trusts headers supplied by an explicitly trusted peer.
// Invalid configuration and malformed address chains fail closed.
func ProxyHeaders(cfg config.Config) func(http.Handler) http.Handler {
	prefixes, _ := config.TrustedProxyCIDRs(cfg)
	trusted := func(addr netip.Addr) bool {
		for _, prefix := range prefixes {
			if prefix.Contains(addr.Unmap()) {
				return true
			}
		}
		return false
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			peer := remoteAddress(r)
			info := proxyInfo{ip: ClientIP(r), base: BaseURL(r)}
			if trusted(peer) {
				if raw := strings.Join(r.Header.Values("X-Forwarded-For"), ","); raw != "" {
					var chain []netip.Addr
					valid := true
					for _, item := range strings.Split(raw, ",") {
						addr, err := netip.ParseAddr(strings.TrimSpace(item))
						if err != nil || addr.Zone() != "" || addr.IsUnspecified() || addr.IsMulticast() {
							valid = false
							break
						}
						chain = append(chain, addr.Unmap())
					}
					if valid {
						client := peer
						for i := len(chain) - 1; i >= 0 && trusted(client); i-- {
							client = chain[i]
						}
						info.ip = client.String()
					}
				}
				host := requestHost(r.Host)
				if forwarded := lastHeaderValue(strings.Join(r.Header.Values("X-Forwarded-Host"), ",")); validHost(forwarded) {
					host = forwarded
				}
				proto := requestScheme(r)
				if forwarded := lastHeaderValue(strings.Join(r.Header.Values("X-Forwarded-Proto"), ",")); forwarded == "http" || forwarded == "https" {
					proto = forwarded
				}
				info.base = proto + "://" + host
			}
			ctx := context.WithValue(r.Context(), proxyInfoKey{}, info)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClientIP uses the connection address unless ProxyHeaders validated a chain.
func ClientIP(r *http.Request) string {
	if info, ok := r.Context().Value(proxyInfoKey{}).(proxyInfo); ok {
		return info.ip
	}
	if addr := remoteAddress(r); addr.IsValid() {
		return addr.String()
	}
	return "unknown"
}

// BaseURL uses validated forwarding metadata only when ProxyHeaders trusts it.
func BaseURL(r *http.Request) string {
	if info, ok := r.Context().Value(proxyInfoKey{}).(proxyInfo); ok {
		return info.base
	}
	return requestScheme(r) + "://" + requestHost(r.Host)
}

func remoteAddress(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, _ := netip.ParseAddr(host)
	return addr.Unmap()
}

func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func lastHeaderValue(raw string) string {
	items := strings.Split(raw, ",")
	return strings.TrimSpace(items[len(items)-1])
}

func requestHost(raw string) string {
	if validHost(raw) {
		return raw
	}
	return "localhost"
}

func validHost(raw string) bool {
	if raw == "" || strings.ContainsAny(raw, "\\/\r\n\t @#?%,") {
		return false
	}
	u, err := url.Parse("//" + raw)
	if err != nil || u.Host != raw || u.Hostname() == "" || u.User != nil {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	} else if strings.HasSuffix(raw, ":") {
		return false
	}
	if addr, err := netip.ParseAddr(u.Hostname()); err == nil {
		return addr.Zone() == ""
	}
	for _, c := range u.Hostname() {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-') {
			return false
		}
	}
	return true
}

// JSONBody 解析 JSON body，非 JSON 或空 body 一律当空对象（校验层再报字段缺失）。
// 数字按 float64 解 —— 和 JS 的 JSON.parse 一个语义，校验层两种都认。
func JSONBody(r *http.Request) map[string]any {
	body := map[string]any{}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil {
		return map[string]any{}
	}
	return body
}
