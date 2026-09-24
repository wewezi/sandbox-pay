// Package signature 出站回调的 HMAC-SHA256 签名。
//
// 只服务一个方向：我们回调接入方 notifyUrl 时签名，接入方验签。
// 入站（接入方调我们）只认 Authorization: Bearer <apiKey>，不验签 ——
// 见 httpx 的 RequireApiKey。
package signature

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Sign HMAC-SHA256 hex（小写）。
func Sign(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return strings.ToLower(hex.EncodeToString(mac.Sum(nil)))
}

// Verify 定长比较，避免按字节提前返回泄漏签名信息。
//
// 服务端自己不调它（入站不验签了）—— 这是接入方那侧验我们回调的参考实现，
// notify 测试用它来证明签出来的东西验得回去。
func Verify(payload, secret, sig string) bool {
	if sig == "" {
		return false
	}
	expected := Sign(payload, secret)
	got := strings.ToLower(sig)
	return hmac.Equal([]byte(expected), []byte(got))
}

// BuildPayload 拼待签名串：body + "&timestamp=" + ts + "&nonce=" + nonce。
func BuildPayload(body, timestamp, nonce string) string {
	return body + "&timestamp=" + timestamp + "&nonce=" + nonce
}

// CreateOrderNo 生成订单号：前缀（转大写）+ 随机 hex（大写）。
func CreateOrderNo(prefix string, byteLength int) string {
	if byteLength <= 0 {
		byteLength = 12
	}
	buf := make([]byte, byteLength)
	if _, err := rand.Read(buf); err != nil {
		panic(err) // 熵源挂了没有降级方案
	}
	return strings.ToUpper(prefix) + strings.ToUpper(hex.EncodeToString(buf))
}
