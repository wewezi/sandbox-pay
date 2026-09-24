package services

// internal/services 各测试共用的桩和工具。
//
// 对应 TS 的 testing/fakes.ts + testing/silence.ts：
// - fakeHTTP        —— 假 httpclient.Client（exchange-rate / notify 用）
// - fakeNotify      —— 假 NotifyEnqueuer（payment / refund 用）
// - fakeAlipay 等    —— 假退款网关（refund 用）
// - newTestStore    —— 每个测试一个 t.TempDir() 上的真 SQLite
// - TestMain        —— logger.SetSilent(true)，测试故意走失败分支，
//                      WARN/ERROR 混进 go test 输出会让全绿的运行看起来像炸了

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httpclient"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/logger"
)

func TestMain(m *testing.M) {
	logger.SetSilent(true)
	os.Exit(m.Run())
}

// newTestStore 一次性真库：t.TempDir() 建目录 + 建表，测完关闭。
func newTestStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.db"), "")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// seedGroup1 建种子分组 —— 新库第一条，id 必然是 1（订单 FK 要用）。
func seedGroup1(t *testing.T, store *db.Store) int64 {
	t.Helper()
	g, err := store.Groups.Insert("seed", true)
	if err != nil {
		t.Fatalf("seed group: %v", err)
	}
	return g.ID
}

// seedAPIKey 建一把已知明文的 Key（回调签名断言用）。
func seedAPIKey(t *testing.T, store *db.Store, groupID int64, key string) int64 {
	t.Helper()
	k, err := store.ApiKeys.Insert(db.InsertKeyInput{
		GroupID: groupID, Name: "测试 Key", Key: key, IsActive: true,
	})
	if err != nil {
		t.Fatalf("seed api key: %v", err)
	}
	return k.ID
}

// insertOrder 建单并要求成功。
func insertOrder(t *testing.T, store *db.Store, o *db.PaymentOrder) {
	t.Helper()
	if err := store.Orders.Insert(o); err != nil {
		t.Fatalf("insert order %s: %v", o.OrderNo, err)
	}
}

// reloadOrder 重新读回订单行（代替 TS 里断言 fake repo 的 update 调用）。
func reloadOrder(t *testing.T, store *db.Store, orderNo string) *db.PaymentOrder {
	t.Helper()
	o, err := store.Orders.FindByOrderNo(orderNo, false)
	if err != nil || o == nil {
		t.Fatalf("reload order %s = (%v, %v), want a row", orderNo, o, err)
	}
	return o
}

// requireErrStatus 断言 err 是带指定 HTTP 状态的 *httperr.Error
// （对应 TS 的 toThrow(BadRequestError / ConflictError / NotFoundError)）。
func requireErrStatus(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with status %d, got nil", want)
	}
	e := httperr.As(err)
	if e == nil {
		t.Fatalf("expected *httperr.Error with status %d, got %v", want, err)
	}
	if e.Status != want {
		t.Fatalf("error status = %d, want %d (err: %v)", e.Status, want, err)
	}
}

// requireErrMsg 断言错误消息包含子串（TS 的 toThrow('文案')）。
func requireErrMsg(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("error %q does not contain %q", err.Error(), substr)
	}
}

// requireCloseTo 对应 TS 的 toBeCloseTo(expected, digits)：
// |got-want| < 10^-digits / 2。
func requireCloseTo(t *testing.T, got, want float64, digits int) {
	t.Helper()
	if diff := math.Abs(got - want); diff >= math.Pow(10, -float64(digits))/2 {
		t.Errorf("got %v ≈ %v (%d digits), diff = %v", got, want, digits, diff)
	}
}

// ---------------------------------------------------------------------------
// fake httpclient.Client —— 对应 TS 的 fakeHttp()
// ---------------------------------------------------------------------------

type fakeHTTP struct {
	getFn  func(url string, timeout time.Duration) (*httpclient.Response, error)
	postFn func(url string, body any, headers map[string]string, timeout time.Duration) (*httpclient.Response, error)

	getCalls  int
	postCalls int

	lastPostURL     string
	lastPostBody    any
	lastPostHeaders map[string]string
}

// newFakeHTTP 默认两个方法都 resolve 成 200 + 空 body，按需覆盖 fn。
func newFakeHTTP() *fakeHTTP {
	f := &fakeHTTP{}
	f.getFn = func(string, time.Duration) (*httpclient.Response, error) {
		return &httpclient.Response{Status: 200, Data: nil}, nil
	}
	f.postFn = func(string, any, map[string]string, time.Duration) (*httpclient.Response, error) {
		return &httpclient.Response{Status: 200, Data: nil}, nil
	}
	return f
}

func (f *fakeHTTP) Get(url string, timeout time.Duration) (*httpclient.Response, error) {
	f.getCalls++
	return f.getFn(url, timeout)
}

func (f *fakeHTTP) Post(url string, body any, headers map[string]string, timeout time.Duration) (*httpclient.Response, error) {
	f.postCalls++
	f.lastPostURL = url
	f.lastPostBody = body
	f.lastPostHeaders = headers
	return f.postFn(url, body, headers, timeout)
}

// ---------------------------------------------------------------------------
// fake NotifyEnqueuer —— payment / refund 测试用
// ---------------------------------------------------------------------------

type notifyCall struct {
	url          string
	notification PaymentNotification
	signingKey   string
}

type fakeNotify struct {
	calls []notifyCall
}

func (f *fakeNotify) EnqueueNotification(url string, notification PaymentNotification, signingKey string) error {
	f.calls = append(f.calls, notifyCall{url: url, notification: notification, signingKey: signingKey})
	return nil
}

// ---------------------------------------------------------------------------
// fake 退款网关 —— refund 测试用（对应 TS 的 alipayService/paypalService mock）
// ---------------------------------------------------------------------------

type alipayRefundCall struct {
	orderNo string
	cents   int64
	reason  string
}

// fakeAlipay 模拟支付宝网关：记录调用，返回预设的退款单号。
// ret 为空串 = 上游失败 —— Go 网关契约用 "" 而不是 panic/throw。
type fakeAlipay struct {
	ret   string
	calls []alipayRefundCall
}

func newFakeAlipay(ret string) *fakeAlipay { return &fakeAlipay{ret: ret} }

func (f *fakeAlipay) Refund(orderNo string, refundCents int64, reason string) string {
	f.calls = append(f.calls, alipayRefundCall{orderNo: orderNo, cents: refundCents, reason: reason})
	return f.ret
}

type paypalRefundCall struct {
	captureID string
	cents     int64
	currency  string
	reason    string
}

type fakePayPal struct {
	ret   string
	calls []paypalRefundCall
}

func newFakePayPal(ret string) *fakePayPal { return &fakePayPal{ret: ret} }

func (f *fakePayPal) Refund(captureID string, refundCents int64, currency string, reason string) string {
	f.calls = append(f.calls, paypalRefundCall{captureID: captureID, cents: refundCents, currency: currency, reason: reason})
	return f.ret
}

// ---------------------------------------------------------------------------
// 其它小工具
// ---------------------------------------------------------------------------

func strPtr(s string) *string   { return &s }
func f64Ptr(v float64) *float64 { return &v }
func i64Ptr(v int64) *int64     { return &v }

// testFeeConfig 对应 payment.spec 的 fakeConfig({FEE_RATE_NATIVE: '0.01', FEE_MIN_CENTS: '0'})。
func testFeeConfig() config.Map {
	return config.Map{"FEE_RATE_NATIVE": "0.01", "FEE_MIN_CENTS": "0"}
}
