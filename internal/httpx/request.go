package httpx

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
)

// ClientIP 尽力取客户端 IP。优先 X-Forwarded-For（信任的反代后面），
// 回落到连接的远端地址。
func ClientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if first := strings.TrimSpace(strings.Split(fwd, ",")[0]); first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// BaseURL 拼当前请求的「协议://host」，给第三方回调地址用。
//
// 反代后面要认 X-Forwarded-Proto，否则本机 http 会把 notify_url 写成 http，
// 而线上其实是 https。
func BaseURL(r *http.Request) string {
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	if host == "" {
		host = "localhost"
	}

	proto := ""
	if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
		proto = strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	if proto == "" {
		// 服务端请求 r.URL.Scheme 通常是空的，回落到 TLS 判定
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	return proto + "://" + host
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
