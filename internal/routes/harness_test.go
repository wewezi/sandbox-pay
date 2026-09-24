// e2e 共享装配：真路由 + 真临时 SQLite + 真 HTTP 请求（httptest 进程内打）。
//
// 对应原 TS 的 src/testing/test-app.ts：每个 harness 一个临时库文件，
// 配置只认这里给的值（config.Map，不吃宿主机 .env），渠道凭证一律为空 ——
// e2e 只走 native 通道和后台接口，绝不碰支付宝 / PayPal / 汇率上游。
package routes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httpx"
	"sandbox-pay/internal/logger"
)

const (
	testAPIKey    = "sp_test_key"
	testAdmin     = "testadmin"
	testAdminPass = "test-pass-123"
)

// TestMain 测试会故意走失败分支（401/404/429/409），
// WARN/ERROR 混进 go test 输出会让一次全绿的运行看起来像炸了。
func TestMain(m *testing.M) {
	logger.SetSilent(true)
	os.Exit(m.Run())
}

// ipSeq 全局 IP 计数器：每个（未显式指定 IP 的）请求一个独立客户端 IP，
// httpx.DefaultLimiter 是包级单例，共享 IP 会让用例互相偷限流配额。
var ipSeq atomic.Uint64

// harness 一次 e2e 装配：临时库 + container + 真路由 handler。
type harness struct {
	t         *testing.T
	cfg       config.Map
	store     *db.Store
	c         *Container
	handler   http.Handler
	groupID   int64
	apiKey    string
	auth      map[string]string // 已登录管理员的 Bearer 头
	groupAuth map[string]string // 接入网关的请求头：Bearer apiKey + JSON
	staticOK  bool              // 静态资源目录在不在（app.css 用例需要）
}

// newHarness 起一套隔离的 app。env 覆盖 baseEnv 里同名的值。
func newHarness(t *testing.T, env map[string]string) *harness {
	t.Helper()

	dir := t.TempDir()
	values := map[string]string{
		"JWT_SECRET":          "test-jwt-secret-not-for-production",
		"JWT_EXPIRES_IN":      "60m",
		"GROUP_API_KEY":       testAPIKey,
		"ENABLE_SANDBOX":      "true",
		"THROTTLE_LIMIT":      "10000",
		"THROTTLE_TTL_MS":     "60000",
		"FEE_RATE_NATIVE":     "0",
		"CHANNEL_COST_NATIVE": "0",
		"DB_FILE":             filepath.Join(dir, "test.db"),
		"LOG_SILENT":          "1",
		// ALIPAY_* / PAYPAL_* / 汇率凭证刻意不配：e2e 不碰第三方。
	}
	for k, v := range env {
		values[k] = v
	}
	cfg := config.Map(values)

	store, err := db.Open(cfg.Get("DB_FILE"), "")
	if err != nil {
		t.Fatalf("打开临时库失败: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	c := NewContainer(cfg, store)
	// 种子分组 + 第一把 Key 按固定值（GROUP_API_KEY）建，网关用例才能拿已知 Key 调。
	if err := c.Groups.Init(); err != nil {
		t.Fatalf("初始化分组失败: %v", err)
	}
	group, key, _, err := c.Groups.EnsureSeed()
	if err != nil {
		t.Fatalf("EnsureSeed 失败: %v", err)
	}
	if key.Key != testAPIKey {
		t.Fatalf("种子 Key = %q, want %q", key.Key, testAPIKey)
	}

	// 管理员账号：cost 4 —— 只要能过 compare 就行，默认 10 会让每个用例多花几百毫秒。
	hash, err := bcrypt.GenerateFromPassword([]byte(testAdminPass), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt 失败: %v", err)
	}
	if err := store.Users.Insert(testAdmin, string(hash), db.RoleAdmin); err != nil {
		t.Fatalf("建管理员失败: %v", err)
	}
	// token 走生产代码签发（ValidateUser + IssueLocalToken），不手搓 JWT。
	user, err := c.Auth.ValidateUser(testAdmin, testAdminPass)
	if err != nil {
		t.Fatalf("ValidateUser 失败: %v", err)
	}
	token, err := c.Auth.IssueLocalToken(user)
	if err != nil {
		t.Fatalf("IssueLocalToken 失败: %v", err)
	}

	static, staticOK := testStatic()
	h := &harness{
		t:        t,
		cfg:      cfg,
		store:    store,
		c:        c,
		handler:  New(cfg, c, static),
		staticOK: staticOK,
		groupID:  group.ID,
		apiKey:   key.Key,
		auth: map[string]string{
			"Authorization": "Bearer " + token.Token,
		},
		groupAuth: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + key.Key,
		},
	}
	return h
}

// testStatic 静态资源：真 public/ 目录（app.css 用例要打真的 200 + no-cache），
// 目录不在就退回统一信封 404 的桩 —— 这些测试打的是 API，不是静态文件。
func testStatic() (http.Handler, bool) {
	if fsys := DiskFS(filepath.Join("..", "..", "public")); fsys != nil {
		return Static(fsys), true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteNotFound(w, r)
	}), false
}

// nextIP 分配一个从没用过的客户端 IP（限流按 IP + 路由名计数）。
func (h *harness) nextIP() string {
	n := ipSeq.Add(1)
	// 10.99.x.y 段：和各 spec 里手写的 IP（10.9.x / 10.1.x / 10.20.x）错开。
	return fmt.Sprintf("10.99.%d.%d", (n/254)%254, n%254+1)
}

// reqSpec 一次请求的全部要素。Body 为 string 时当原始 JSON，其余 json.Marshal，nil 无 body。
type reqSpec struct {
	Method  string
	Path    string
	Body    any
	Headers map[string]string
	IP      string // 空 → 每次分配一个新 IP（默认即隔离）
}

// do 打一发请求，返回状态码 / 响应头 / 原始 body / 解出来的 JSON。
func (h *harness) do(t *testing.T, spec reqSpec) *resp {
	t.Helper()

	var body io.Reader
	switch b := spec.Body.(type) {
	case nil:
	case string:
		body = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("marshal 请求体失败: %v", err)
		}
		body = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(spec.Method, spec.Path, body)
	ip := spec.IP
	if ip == "" {
		ip = h.nextIP()
	}
	req.Header.Set("X-Forwarded-For", ip)
	for k, v := range spec.Headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return respFromRecorder(t, rec)
}

// login 走真实的 POST /auth/login，返回 Bearer 头和会话 Cookie（"name=value"）。
// ip 传空则自动分配 —— login 路由硬编码 5 次/分钟/IP，每个用例换 IP 才不会互相挤掉。
func (h *harness) login(t *testing.T, ip string) (map[string]string, string) {
	t.Helper()
	r := h.do(t, reqSpec{
		Method:  "POST",
		Path:    "/v1/api/auth/login",
		Body:    map[string]any{"username": testAdmin, "password": testAdminPass},
		Headers: map[string]string{"Content-Type": "application/json"},
		IP:      ip,
	})
	if r.Status != http.StatusOK {
		t.Fatalf("login 状态 = %d, want 200; body=%s", r.Status, r.Raw)
	}
	token, ok := r.data()["token"].(string)
	if !ok || token == "" {
		t.Fatalf("login 响应里没有 token: %s", r.Raw)
	}
	setCookie := r.Header.Get("Set-Cookie")
	cookie := strings.Split(setCookie, ";")[0]
	return map[string]string{"Authorization": "Bearer " + token}, cookie
}

// createOrder 用接入网关下一张 native 单，返回响应和收银台 URL 里的 orderNo。
func (h *harness) createOrder(t *testing.T, body map[string]any) (*resp, string) {
	t.Helper()
	r := h.do(t, reqSpec{
		Method:  "POST",
		Path:    "/v1/api/gateway/pay",
		Body:    body,
		Headers: h.groupAuth,
	})
	if r.Status != http.StatusOK {
		t.Fatalf("下单状态 = %d, want 200; body=%s", r.Status, r.Raw)
	}
	return r, cashierOrderNo(t, r)
}

// cashierOrderNo 从收银台 URL（data 字段）里解出 orderNo。
func cashierOrderNo(t *testing.T, r *resp) string {
	t.Helper()
	raw, ok := r.data()["data"].(string)
	if !ok {
		t.Fatalf("data.data 不是字符串: %#v", r.data()["data"])
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("解析收银台 URL 失败: %v", err)
	}
	return parsed.Query().Get("orderNo")
}

// withHeaders 合并多份请求头（不改原 map）。
func withHeaders(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// resp 一次请求的完整回执。
type resp struct {
	t      *testing.T
	Status int
	Header http.Header
	Raw    string
	JSON   map[string]any // body 不是 JSON 对象时为 nil
}

func respFromRecorder(t *testing.T, rec *httptest.ResponseRecorder) *resp {
	t.Helper()
	res := rec.Result()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("读响应体失败: %v", err)
	}
	r := &resp{t: t, Status: res.StatusCode, Header: res.Header, Raw: string(raw)}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(r.Raw)), &parsed); err == nil {
		r.JSON = parsed
	}
	return r
}

// obj 整个 JSON 响应体（错误信封 / 成功信封都适用）。
func (r *resp) obj() map[string]any {
	r.t.Helper()
	if r.JSON == nil {
		r.t.Fatalf("响应不是 JSON 对象 (status=%d): %s", r.Status, r.Raw)
	}
	return r.JSON
}

// data 成功信封的 data 对象。
func (r *resp) data() map[string]any {
	r.t.Helper()
	v, ok := r.obj()["data"].(map[string]any)
	if !ok {
		r.t.Fatalf("envelope.data 不是对象: %#v", r.obj()["data"])
	}
	return v
}

// dataArr 成功信封的 data 数组。
func (r *resp) dataArr() []any {
	r.t.Helper()
	v, ok := r.obj()["data"].([]any)
	if !ok {
		r.t.Fatalf("envelope.data 不是数组: %#v", r.obj()["data"])
	}
	return v
}

// ── 断言小工具 ────────────────────────────────────────────────

func eq[T comparable](t *testing.T, what string, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: got %v, want %v", what, got, want)
	}
}

func eqSlice[T comparable](t *testing.T, what string, got, want []T) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %v, want %v", what, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s[%d]: got %v, want %v", what, i, got[i], want[i])
		}
	}
}

func num(t *testing.T, v any, what string) float64 {
	t.Helper()
	n, ok := v.(float64)
	if !ok {
		t.Fatalf("%s: 不是数字: %#v", what, v)
	}
	return n
}

func str(t *testing.T, v any, what string) string {
	t.Helper()
	s, ok := v.(string)
	if !ok {
		t.Fatalf("%s: 不是字符串: %#v", what, v)
	}
	return s
}

func hasSubstr(t *testing.T, s, sub, what string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Fatalf("%s: %q 不含 %q", what, s, sub)
	}
}

func lacksSubstr(t *testing.T, s, sub, what string) {
	t.Helper()
	if strings.Contains(s, sub) {
		t.Fatalf("%s: %q 不该含 %q", what, s, sub)
	}
}
