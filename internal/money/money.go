// Package money 金额一律按整数分（或对应币种最小单位）计算。
//
// 不变式：代码里流转、存进数据库的金额都是整数分。元/分的换算只在
// API 与网关边界上做。规则：
//   - 永远不要对金额做浮点运算，一律先转成整数分；
//   - 调外部网关（支付宝/PayPal）时用 ToYuanString 转两位小数字符串；
//   - 从网关收回时用 YuanStringToCents / YuanToCents 在边界上转分。
package money

import (
	"math"
	"strconv"
	"strings"
)

// YuanToCents 元（数字）→ 整数分：10 → 1000。
func YuanToCents(yuan float64) int64 {
	return int64(math.Round(yuan * 100))
}

// YuanStringToCents 元（"10.00" 这类字符串）→ 整数分，空串/解析失败返回 0。
// 走浮点再 round，和原 TS 版 Number(value) * 100 的行为一致。
func YuanStringToCents(value string) int64 {
	s := strings.TrimSpace(value)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(math.Round(f * 100))
}

// ToYuan 整数分 → 元（数字）：1000 → 10。
func ToYuan(cents int64) float64 {
	return math.Round(float64(cents)) / 100
}

// ToYuanString 整数分 → 固定两位小数的字符串（给网关 API 用）。
func ToYuanString(cents int64) string {
	return strconv.FormatFloat(ToYuan(cents), 'f', 2, 64)
}
