package httpx

import (
	"net/http"
	"sync"
	"time"

	"sandbox-pay/internal/httperr"
)

// 内存滑动窗口限流。
//
// 单进程自托管，计数放内存就够 —— 换 Redis 只是给自己加运维负担。
// 重启后计数清零，沙箱场景无所谓。

// Limiter 按「路由名 + IP」计数的滑动窗口。
type Limiter struct {
	mu      sync.Mutex
	buckets map[string][]time.Time
}

// NewLimiter 建一个限流器。
func NewLimiter() *Limiter {
	return &Limiter{buckets: make(map[string][]time.Time)}
}

// DefaultLimiter 全局默认实例，路由各传自己的 name 分开计数。
var DefaultLimiter = NewLimiter()

// Allow 窗口内还没超过配额就记一次并放行，否则抛 429。
func (l *Limiter) Allow(name, ip string, limit int, window time.Duration) error {
	key := name + ":" + ip
	now := time.Now()
	cutoff := now.Add(-window)

	l.mu.Lock()
	defer l.mu.Unlock()

	hits := l.buckets[key]
	// 时间戳天然升序，从头切掉过期的即可
	i := 0
	for i < len(hits) && !hits[i].After(cutoff) {
		i++
	}
	hits = hits[i:]
	if len(hits) >= limit {
		l.buckets[key] = hits
		return httperr.TooManyRequests()
	}
	l.buckets[key] = append(hits, now)
	return nil
}

// Cleanup 清掉 5 分钟没有任何请求的桶，别让 map 无限增长。
func (l *Limiter) Cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-5 * time.Minute)
	for key, hits := range l.buckets {
		if len(hits) == 0 || hits[len(hits)-1].Before(cutoff) {
			delete(l.buckets, key)
		}
	}
}

// StartCleanup 定期清空空桶，由 app 启动时调一次。返回的函数用于关停。
func (l *Limiter) StartCleanup(interval time.Duration) (stop func()) {
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				l.Cleanup()
			case <-done:
				return
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}

// RateLimit 按 IP + 路由名限流的中间件。
//
//	name  计数键的前缀，不同路由用不同名字才能各自计数
//	limit 窗口内允许的请求数
//	window 窗口长度
func RateLimit(name string, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := DefaultLimiter.Allow(name, ClientIP(r), limit, window); err != nil {
				writeError(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
