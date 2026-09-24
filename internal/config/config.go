// Package config 配置读取：.env 覆盖 Defaults，全项目只有这一处配置。
//
// 沙箱自用不需要运行时改配置，全部走 .env + Defaults 里的默认值。
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Defaults 与原 TS 版 CONFIG_DEFAULTS 一一对应。
var Defaults = map[string]string{
	// 沙箱
	"ENABLE_SANDBOX": "true",

	// 限流
	"THROTTLE_TTL_MS": "60000",
	"THROTTLE_LIMIT":  "60",

	// 手续费（小数：0.006 = 0.6%）
	"FEE_RATE_ALIPAY":     "0.006",
	"FEE_RATE_PAYPAL":     "0.044",
	"FEE_RATE_NATIVE":     "0",
	"CHANNEL_COST_ALIPAY": "0.006",
	"CHANNEL_COST_PAYPAL": "0.044",
	"CHANNEL_COST_NATIVE": "0",
	"FEE_MIN_CENTS":       "0",

	// 汇率
	"EXCHANGE_RATE_PROVIDER":     "open-er-api",
	"EXCHANGE_RATE_CACHE_TTL_MS": "3600000",
	"FALLBACK_CNY_TO_USD":        "0.14",
	"EXCHANGE_RATE_MARGIN":       "1",

	// 支付宝
	"ALIPAY_SERVER_URL": "https://openapi-sandbox.dl.alipaydev.com/gateway.do",

	// PayPal
	"PAYPAL_ENVIRONMENT": "sandbox",

	// 官方存管（自有渠道）
	"NATIVE_PAY_QR_URL":       "",
	"NATIVE_PAY_ACCOUNT_NAME": "Sandbox Pay Official",
	"NATIVE_PAY_ACCOUNT_NO":   "",
	"NATIVE_PAY_BANK_NAME":    "",
}

// Config 只要一个 Get。测试里传 Map(...) 就能当假配置用。
type Config interface {
	Get(key string) string
}

// EnvConfig 读 process.env 的默认实现。
type EnvConfig struct{}

func (EnvConfig) Get(key string) string { return os.Getenv(key) }

// Map 读一张 key→value 的表（值都是字符串，和 .env 一致），测试用。
type Map map[string]string

func (m Map) Get(key string) string { return m[key] }

// String 读字符串，空串按未设置处理，回落到默认值。
func String(c Config, key string) string {
	if v := strings.TrimSpace(c.Get(key)); v != "" {
		return v
	}
	return Defaults[key]
}

// Number 读数字，非法值回落到默认值。
func Number(c Config, key string) float64 {
	if v := strings.TrimSpace(c.Get(key)); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	}
	n, _ := strconv.ParseFloat(Defaults[key], 64)
	return n
}

// Int 读整数，非法值回落到默认值。
func Int(c Config, key string) int { return int(Number(c, key)) }

// Bool 读布尔，只认 "true" / "1"。
func Bool(c Config, key string) bool {
	v := strings.TrimSpace(c.Get(key))
	if v == "" {
		v = Defaults[key]
	}
	return v == "true" || v == "1"
}

// Rate 读带渠道后缀的费率 key（如 FEE_RATE_ALIPAY）。
// 渠道未知时返回 0，不报错。
func Rate(c Config, prefix, method string) float64 {
	key := prefix + "_" + strings.ToUpper(method)
	if _, ok := Defaults[key]; !ok {
		v := strings.TrimSpace(c.Get(key))
		if v == "" {
			return 0
		}
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0
		}
		return n
	}
	return Number(c, key)
}

// LoadEnv 加载 .env。文件不存在时静默跳过（环境变量可能已经在 shell 里设好了）。
// 已经存在的环境变量不覆盖 —— 部署时显式注入的值优先级更高。
func LoadEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // 没有 .env 文件，用现成的环境变量
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		// 去掉成对的引号（单双都认），行内注释不支持 —— .env.example 里没有这种写法
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') ||
			(val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		if key != "" && os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}
