// Package jwt HS256 JWT 的签发与校验。
//
// 只支持 HS256 —— 自托管沙箱，签名和校验都是自己，没有非对称的必要。
// 校验时硬编码只接受 alg=HS256，避免 alg=none 之类的经典绕过。
package jwt

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

func b64url(input []byte) string {
	return base64.RawURLEncoding.EncodeToString(input)
}

func b64urlDecode(input string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(input, "="))
}

func computeSig(data, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return b64url(mac.Sum(nil))
}

// ParseExpiresIn 把 "1440m" / "24h" / "3600" 这类写法解析成秒。
// 认不出来的按秒当数字读，读不出来回落到 24 小时。
func ParseExpiresIn(value string) int {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return 86400
	}
	i := 0
	for i < len(raw) && raw[i] >= '0' && raw[i] <= '9' {
		i++
	}
	if i == 0 {
		return 86400
	}
	n := 0
	for _, c := range raw[:i] {
		n = n*10 + int(c-'0')
	}
	factor := map[byte]int{'s': 1, 'm': 60, 'h': 3600, 'd': 86400}[raw[i]]
	if i == len(raw) {
		factor = 1 // 没有单位后缀，当秒
	} else if factor == 0 {
		return 86400 // 后缀认不出来
	}
	return n * factor
}

// Sign 签发。claims 里的业务字段原样进 payload（sub/username/role 这些）。
func Sign(claims map[string]any, secret string, expiresInSeconds int) string {
	now := time.Now().Unix()
	body := make(map[string]any, len(claims)+2)
	for k, v := range claims {
		body[k] = v
	}
	body["iat"] = now
	body["exp"] = now + int64(expiresInSeconds)

	head := b64url([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	data := head + "." + b64url(payload)
	return data + "." + computeSig(data, secret)
}

// Verify 校验并解出 payload。签名不对、格式不对、过期都返回 nil，不报错 ——
// 调用方（RequireAuth）统一转 401。
//
// 数字保留成 json.Number，回给前端时不会被 float64 格式化带出 .0 之类的尾巴。
func Verify(token, secret string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil
	}
	head, body, sig := parts[0], parts[1], parts[2]

	expected := computeSig(head+"."+body, secret)
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return nil
	}

	var header map[string]any
	if err := json.Unmarshal(mustDecode(head), &header); err != nil {
		return nil
	}
	// 只认 HS256。不校验 alg 就会被 alg=none 之类的伪造 token 骗过去。
	if alg, _ := header["alg"].(string); alg != "HS256" {
		return nil
	}

	dec := json.NewDecoder(strings.NewReader(string(mustDecode(body))))
	dec.UseNumber()
	var payload map[string]any
	if err := dec.Decode(&payload); err != nil {
		return nil
	}

	if exp, ok := payload["exp"].(json.Number); ok {
		if secs, err := exp.Int64(); err == nil && secs*1000 <= time.Now().UnixMilli() {
			return nil
		}
	}
	return payload
}

func mustDecode(input string) []byte {
	b, err := b64urlDecode(input)
	if err != nil {
		return nil
	}
	return b
}
