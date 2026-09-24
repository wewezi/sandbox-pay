// Package services 业务服务层：支付、退款、回调通知、分组、鉴权、后台统计。
//
// 服务层只依赖 db.Store 和 config，不碰 HTTP —— 路由负责把请求解成入参、
// 把返回值写进统一信封。错误一律走 httperr，由全局错误处理转成错误信封。
package services

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"
)

// JsTime 把 time 转成前端吃的 JS 形状：2006-01-02T15:04:05.000Z。
//
// 用 UTC + Z07:00：UTC 时间的 offset 打出来就是 "Z"，和 JS Date.toISOString
// 一致；非零 offset 会带 +08:00 之类的尾巴，所以我们一律先 UTC。
func JsTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z07:00")
}

// jsTimePtr 指针版：nil（SQL NULL）返回 nil，由 omitempty 省略字段 ——
// 对齐 TS 里 `undefined` 被 JSON.stringify 丢掉的行为。
func jsTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := JsTime(*t)
	return &s
}

// tsOpt 复刻 TS 模板串里的 `${reason}`：nil 打成 "undefined"，不是空串。
// 原版日志就是这么打的，改成空串会丢掉「这个 reason 压根没传」的信息。
func tsOpt(p *string) string {
	if p == nil {
		return "undefined"
	}
	return *p
}

// tsOptInt 同 tsOpt，用在 `${expectedForeignCents ?? 'n/a'}` 这种位置。
func tsOptInt(p *int64) string {
	if p == nil {
		return "n/a"
	}
	return strconv.FormatInt(*p, 10)
}

// strDeref 解引用，空指针回落到 def（对应 TS 的 `x ?? ”` / `x || ”`）。
func strDeref(p *string, def string) string {
	if p == nil {
		return def
	}
	return *p
}

// randomHex n 个随机字节 → 小写 hex。订单 id 用 16 字节（32 字符），
// 和原版 randomUUID().replace(/-/g, ”) 同长度。
func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(err) // 熵源挂了没有降级方案
	}
	return hex.EncodeToString(buf)
}

// base36 字母表，和 JS 的 toString(36) 一致（0-9 在前）。
const base36 = "0123456789abcdefghijklmnopqrstuvwxyz"

// nonce8 回调 nonce：8 个 base36 字符。
//
// 复刻 `Math.random().toString(36).slice(2, 10)` 的形状 —— 长度和字符集
// 都一样，只是熵源换成 crypto/rand。取模有一点点偏置，但 nonce 不是秘密，
// 只要不重复即可。
func nonce8() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	for i := range buf {
		buf[i] = base36[int(buf[i])%len(base36)]
	}
	return string(buf)
}

// abs64 绝对值。金额比较的容差逻辑都用它。
func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
