package services

import (
	"fmt"
	"math"
	"net/url"
	"sync"
	"time"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/httpclient"
	"sandbox-pay/internal/logger"
)

type cachedRate struct {
	value     float64
	expiresAt int64 // Unix 毫秒
}

// ExchangeRateService 拉实时汇率 + 内存 TTL 缓存。
//
// 上游全挂时回落 .env 配的静态汇率 —— 沙箱不能因为一个免费 API 抽风就退不了款。
// 配置从 .env 读，默认值见 config.Defaults。
type ExchangeRateService struct {
	log  *logger.Logger
	http httpclient.Client
	cfg  config.Config

	mu    sync.Mutex
	cache map[string]cachedRate
}

// NewExchangeRateService 构造。
func NewExchangeRateService(http httpclient.Client, cfg config.Config) *ExchangeRateService {
	return &ExchangeRateService{
		log:   logger.New("ExchangeRateService"),
		http:  http,
		cfg:   cfg,
		cache: map[string]cachedRate{},
	}
}

// GetCnyToUsdRate CNY→USD 汇率。
func (s *ExchangeRateService) GetCnyToUsdRate() float64 { return s.GetRate("CNY", "USD") }

// GetRate 查 from→to 汇率。
//
// 缓存里存的是回落之后的值（含 margin），所以缓存命中就不会再打上游 ——
// 上游挂掉期间的失败也只在过期时重试一次。
//
// 原版是 async，这里改成同步：调用点（退款、下单）本来就要等汇率回来，
// 没有并发复用的场景，多一层 goroutine 只是噪声。
func (s *ExchangeRateService) GetRate(from, to string) float64 {
	key := from + "->" + to

	now := time.Now().UnixMilli() // 取在 fetch 之前：耗时不该缩短缓存有效期

	s.mu.Lock()
	cached, ok := s.cache[key]
	s.mu.Unlock()
	if ok && cached.expiresAt > now {
		return cached.value
	}

	provider := config.String(s.cfg, "EXCHANGE_RATE_PROVIDER")
	cacheTtlMs := config.Number(s.cfg, "EXCHANGE_RATE_CACHE_TTL_MS")
	fallback := config.Number(s.cfg, "FALLBACK_CNY_TO_USD")
	margin := config.Number(s.cfg, "EXCHANGE_RATE_MARGIN")

	var rate *float64
	if provider == "open-er-api" {
		rate = s.fetchOpenErApi(from, to)
	}
	if rate == nil || math.IsNaN(*rate) || math.IsInf(*rate, 0) || *rate <= 0 {
		s.log.Warn(fmt.Sprintf("Exchange rate fetch failed for %s, using fallback %v", key, fallback))
		rate = &fallback
	}

	effective := *rate * margin
	s.mu.Lock()
	s.cache[key] = cachedRate{value: effective, expiresAt: now + int64(cacheTtlMs)}
	s.mu.Unlock()
	return effective
}

// fetchOpenErApi 拉 open.er-api.com 的 /v6/latest/{from}。
//
// 判定只看 rates[to] 是不是数字 —— 不查响应里的 status 字段：
// 非 2xx 的响应体里 rates 要么缺失要么不含目标币种，结果一样是回落。
func (s *ExchangeRateService) fetchOpenErApi(from, to string) *float64 {
	fetchURL := "https://open.er-api.com/v6/latest/" + url.PathEscape(from)
	resp, err := s.http.Get(fetchURL, 5*time.Second)
	if err != nil {
		s.log.Warn(fmt.Sprintf("open.er-api fetch error: %s", err))
		return nil
	}
	if resp == nil {
		return nil
	}
	data, ok := resp.Data.(map[string]any)
	if !ok {
		return nil
	}
	rates, ok := data["rates"].(map[string]any)
	if !ok {
		return nil
	}
	v, ok := rates[to].(float64)
	if !ok {
		return nil
	}
	return &v
}
