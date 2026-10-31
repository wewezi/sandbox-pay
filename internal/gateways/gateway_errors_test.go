package gateways

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/plutov/paypal"
	"github.com/smartwalle/alipay/v3"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/httperr"
)

func testProviderError(status int, path string) *paypal.ErrorResponse {
	return &paypal.ErrorResponse{
		Response: &http.Response{StatusCode: status, Request: &http.Request{Method: http.MethodPost, URL: &url.URL{Scheme: "https", Host: "provider.invalid", Path: path, RawQuery: "access_token=DO_NOT_LEAK_PROVIDER_SECRET"}}},
		Message:  "DO_NOT_LEAK_PROVIDER_SECRET", Name: "DO_NOT_LEAK_PROVIDER_SECRET", InformationLink: "DO_NOT_LEAK_PROVIDER_SECRET", DebugID: "DO_NOT_LEAK_PROVIDER_SECRET",
	}
}

type gatewayErrorPanicText struct{}

func (gatewayErrorPanicText) Error() string { panic("classifier must not call unknown Error()") }

func TestGatewayErrorClassificationAndRedaction(t *testing.T) {
	for _, scenario := range []struct {
		name, channel, part string
		err                 error
		status              int
	}{
		{name: "OAuth status401 without parsed body", channel: "paypal", err: testProviderError(401, "/v1/oauth2/token"), status: 400, part: "PAYPAL_CLIENT_ID"},
		{name: "OAuth status400", channel: "paypal", err: testProviderError(400, "/v1/oauth2/token"), status: 400, part: "同一个沙箱应用"},
		{name: "OAuth provider failure", channel: "paypal", err: testProviderError(503, "/v1/oauth2/token"), status: 502, part: "暂时不可用"},
		{name: "provider rate limit", channel: "paypal", err: testProviderError(429, "/v2/checkout/orders"), status: 503, part: "过于频繁"},
		{name: "provider reject", channel: "paypal", err: rejectedChannelCreation(testProviderError(422, "/v2/checkout/orders")), status: 400, part: "请求未成功"},
		{name: "bound provider query404", channel: "paypal", err: testProviderError(404, "/v2/checkout/orders/existing"), status: 400, part: "核对订单"},
		{name: "provider authentication", channel: "paypal", err: testProviderError(401, "/v2/checkout/orders"), status: 400, part: "身份验证"},
		{name: "provider permission", channel: "paypal", err: testProviderError(403, "/v2/checkout/orders"), status: 400, part: "权限"},
		{name: "provider conflict", channel: "paypal", err: testProviderError(409, "/v2/checkout/orders"), status: 409, part: "冲突"},
		{name: "deadline", channel: "paypal", err: fmt.Errorf("DO_NOT_LEAK_PROVIDER_SECRET: %w", context.DeadlineExceeded), status: 504, part: "超时"},
		{name: "connection", channel: "paypal", err: &url.Error{Op: "POST", URL: "https://provider.invalid?access_token=DO_NOT_LEAK_PROVIDER_SECRET", Err: &net.DNSError{Err: "DO_NOT_LEAK_PROVIDER_SECRET", Name: "DO_NOT_LEAK_PROVIDER_SECRET"}}, status: 502, part: "连接失败"},
		{name: "private key parse stage", channel: "alipay", err: gatewayConfigurationFailure("ALIPAY_PRIVATE_KEY", errors.New("DO_NOT_LEAK_PROVIDER_SECRET")), status: 400, part: "ALIPAY_PRIVATE_KEY"},
		{name: "public key parse stage", channel: "alipay", err: gatewayConfigurationFailure("ALIPAY_PUBLIC_KEY", errors.New("DO_NOT_LEAK_PROVIDER_SECRET")), status: 400, part: "ALIPAY_PUBLIC_KEY"},
		{name: "signature verification", channel: "alipay", err: fmt.Errorf("DO_NOT_LEAK_PROVIDER_SECRET: %w", rsa.ErrVerification), status: 502, part: "验签失败"},
		{name: "alipay error pointer", channel: "alipay", err: &alipay.Error{Code: alipay.CodeInvalidParam, SubMsg: "DO_NOT_LEAK_PROVIDER_SECRET"}, status: 400, part: "请求未完成"},
		{name: "alipay error value", channel: "alipay", err: alipay.Error{Code: alipay.CodePermissionDenied, SubMsg: "DO_NOT_LEAK_PROVIDER_SECRET"}, status: 400, part: "权限"},
		{name: "alipay system error", channel: "alipay", err: alipay.Error{Code: alipay.CodeBusinessFailed, SubCode: "ACQ.SYSTEM_ERROR", SubMsg: "DO_NOT_LEAK_PROVIDER_SECRET"}, status: 502, part: "暂时不可用"},
		{name: "alipay system busy", channel: "alipay", err: &alipay.Error{Code: alipay.CodeBusinessFailed, SubCode: "ACQ.SYSTEM_BUSY", SubMsg: "DO_NOT_LEAK_PROVIDER_SECRET"}, status: 502, part: "暂时不可用"},
		{name: "alipay already successful", channel: "alipay", err: alipay.Error{Code: alipay.CodeBusinessFailed, SubCode: "ACQ.TRADE_HAS_SUCCESS", SubMsg: "DO_NOT_LEAK_PROVIDER_SECRET"}, status: 400, part: "请求未完成"},
		{name: "invalid OAuth token response", channel: "paypal", err: invalidGatewayResponse("oauth"), status: 502, part: "认证响应"},
		{name: "missing approval link", channel: "paypal", err: invalidGatewayResponse("approval"), status: 502, part: "确认链接"},
		{name: "unknown panic text", channel: "paypal", err: gatewayErrorPanicText{}, status: 500, part: "处理失败"},
		{name: "unknown text containing secrets", channel: "alipay", err: errors.New("DO_NOT_LEAK_PROVIDER_SECRET"), status: 500, part: "处理失败"},
		{name: "malformed nil response", channel: "paypal", err: &paypal.ErrorResponse{Message: "DO_NOT_LEAK_PROVIDER_SECRET"}, status: 502, part: "异常响应"},
		{name: "malformed nil request", channel: "paypal", err: &paypal.ErrorResponse{Response: &http.Response{StatusCode: 401}, Message: "DO_NOT_LEAK_PROVIDER_SECRET"}, status: 400, part: "身份验证"},
		{name: "malformed nil request URL", channel: "paypal", err: &paypal.ErrorResponse{Response: &http.Response{StatusCode: 401, Request: &http.Request{}}, Message: "DO_NOT_LEAK_PROVIDER_SECRET"}, status: 400, part: "身份验证"},
		{name: "typed nil SDK error", channel: "paypal", err: (*paypal.ErrorResponse)(nil), status: 500, part: "处理失败"},
		{name: "typed nil network error", channel: "paypal", err: (*net.DNSError)(nil), status: 500, part: "处理失败"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			public := GatewayError(scenario.err, scenario.channel)
			if public == nil || public.Status != scenario.status || !strings.Contains(public.Message, scenario.part) {
				t.Fatalf("incorrect classification: public=%+v", public)
			}
			summary := GatewayErrorSummary(scenario.err, scenario.channel)
			for _, value := range []string{public.Message, summary} {
				for _, secret := range []string{"DO_NOT_LEAK_PROVIDER_SECRET", "https://", "provider.invalid", "access_token"} {
					if strings.Contains(value, secret) {
						t.Fatalf("classifier exposed sensitive provider content: %s", value)
					}
				}
			}
		})
	}
}

func TestGatewayErrorPreservesBusinessAndCreationCertainty(t *testing.T) {
	if GatewayError(nil, "paypal") != nil || GatewayErrorSummary(nil, "paypal") != "" {
		t.Fatal("nil error must remain successful")
	}
	business := httperr.Conflict("订单已支付，不能切换渠道")
	if GatewayError(fmt.Errorf("wrapped: %w", business), "paypal") != business {
		t.Fatal("safe business error was lost")
	}
	for _, status := range []int{401, 422, 500} {
		err := testProviderError(status, "/v2/checkout/orders")
		_ = GatewayError(err, "paypal")
		if ChannelCreationRejected(err) {
			t.Fatal("classification falsely proved a creation rejection")
		}
	}
	err := rejectedChannelCreation(testProviderError(422, "/v2/checkout/orders"))
	_ = GatewayError(err, "paypal")
	if !ChannelCreationRejected(err) {
		t.Fatal("classification lost a proven rejection")
	}
}

func TestGatewayAlipayKeyParsingUsesSafeConfigurationStages(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	privatePEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}))
	for _, scenario := range []struct{ name, private, public, setting string }{
		{name: "private", private: "DO_NOT_LEAK_PRIVATE_KEY", public: "unused", setting: "ALIPAY_PRIVATE_KEY"},
		{name: "public", private: privatePEM, public: "DO_NOT_LEAK_PUBLIC_KEY", setting: "ALIPAY_PUBLIC_KEY"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			a := NewAlipay(config.Map{"ALIPAY_APP_ID": "local-test-app", "ALIPAY_PRIVATE_KEY": scenario.private, "ALIPAY_PUBLIC_KEY": scenario.public}, nil, nil)
			_, err := a.client()
			public := GatewayError(err, "alipay")
			if public == nil || public.Status != 400 || !strings.Contains(public.Message, scenario.setting) {
				t.Fatalf("key parsing not classified safely: %+v", public)
			}
			if strings.Contains(public.Message, "DO_NOT_LEAK_") || strings.Contains(GatewayErrorSummary(err, "alipay"), "DO_NOT_LEAK_") {
				t.Fatal("key material leaked")
			}
		})
	}
}

func TestGatewayPayPalOAuthFailureClassifiedWithSDKBodyFieldsMissing(t *testing.T) {
	var providerPayments atomic.Int64
	p := testPayPalGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/oauth2/token" {
			providerPayments.Add(1)
			t.Error("an OAuth failure submitted a payment request")
		}
		w.WriteHeader(http.StatusUnauthorized)
		writePayPalJSON(t, w, map[string]string{"error": "invalid_client", "error_description": "DO_NOT_LEAK_PROVIDER_SECRET"})
	})
	err := p.PrepareCreateOrder()
	var provider *paypal.ErrorResponse
	if err == nil || !errors.As(err, &provider) || provider.Name != "" || provider.Message != "" {
		t.Fatal("test must reproduce the SDK dropping OAuth body fields")
	}
	public := GatewayError(err, "paypal")
	if public.Status != 400 || !strings.Contains(public.Message, "PAYPAL_CLIENT_ID") || providerPayments.Load() != 0 {
		t.Fatalf("OAuth failure not diagnosed safely: public=%+v payments=%d", public, providerPayments.Load())
	}
	if summary := GatewayErrorSummary(err, "paypal"); !strings.Contains(summary, "class=credentials") || !strings.Contains(summary, "provider_status=401") || strings.Contains(summary, "DO_NOT_LEAK") {
		t.Fatalf("incorrect safe OAuth diagnostics: %s", summary)
	}
}
