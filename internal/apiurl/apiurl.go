// Package apiurl URI 版本前缀。所有业务路由都挂在这个前缀下（见 routes 挂载），
// 只有 /health 例外。
package apiurl

import "strings"

// Prefix 业务路由统一前缀。
const Prefix = "/v1/api"

// URL 拼给第三方回调用的绝对地址。
//
// 第三方（支付宝 notify_url、PayPal return_url）拿到的必须是带版本前缀的完整
// 路径，否则请求会打到 404，支付结果永远回不来。base 通常是
// "protocol://host"，不含前缀。
//
//	base   协议 + host，末尾不带斜杠
//	path   不带 api 前缀的路径，如 alipay/notify
func URL(base, path string) string {
	return strings.TrimRight(base, "/") + Prefix + "/" + strings.TrimLeft(path, "/")
}
