package gateways

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"reflect"
	"strings"

	"github.com/plutov/paypal"
	"github.com/smartwalle/alipay/v3"

	"sandbox-pay/internal/httperr"
)

// These stage annotations are produced where an SDK operation fails. Their
// Error methods are safe for logging; Unwrap retains the original type for
// classification without exposing SDK request URLs or response content.
type gatewayConfigurationError struct {
	setting string
	cause   error
}

func (e *gatewayConfigurationError) Error() string {
	if e == nil {
		return configurationMessage("")
	}
	return configurationMessage(e.setting)
}
func (e *gatewayConfigurationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func gatewayConfigurationFailure(setting string, cause error) error {
	return &gatewayConfigurationError{setting: setting, cause: cause}
}

func configurationMessage(setting string) string {
	switch setting {
	case "ALIPAY_PRIVATE_KEY":
		return "支付宝应用私钥格式无效，请检查 ALIPAY_PRIVATE_KEY"
	case "ALIPAY_PUBLIC_KEY":
		return "支付宝公钥格式无效，请检查 ALIPAY_PUBLIC_KEY"
	default:
		return "支付通道配置无效，请检查通道配置"
	}
}

type gatewayResponseError struct{ reason string }

func (e *gatewayResponseError) Error() string {
	if e == nil {
		return "支付通道返回的数据格式无效，请稍后重试"
	}
	switch e.reason {
	case "oauth":
		return "PayPal 返回无效的认证响应，请稍后重试"
	case "order":
		return "PayPal 订单响应与本地订单不匹配，请重试或改用其他支付方式"
	case "approval":
		return "PayPal 未返回有效的支付确认链接，请重试或改用其他支付方式"
	default:
		return "支付通道返回的数据格式无效，请稍后重试"
	}
}

func invalidGatewayResponse(reason string) error { return &gatewayResponseError{reason: reason} }

type gatewayErrorClassification struct {
	public         *httperr.Error
	kind           string
	providerStatus int
}

// GatewayError returns only safe, actionable text. It never calls an unknown
// error's Error method, nor copies provider response messages, URLs or bodies.
// Classifying an error does not establish whether a payment was created;
// callers must keep using ChannelCreationRejected for recovery decisions.
func GatewayError(err error, channel string) *httperr.Error {
	return classifyGatewayError(err, channel).public
}

// GatewayErrorSummary contains a bounded set of categorical fields suitable
// for a correlation-ID log. Raw SDK Error() strings must not be logged because
// some SDKs include request URLs, provider messages and response contents.
func GatewayErrorSummary(err error, channel string) string {
	classification := classifyGatewayError(err, channel)
	if classification.public == nil {
		return ""
	}
	if channel != "alipay" && channel != "paypal" && channel != "native" {
		channel = "gateway"
	}
	return fmt.Sprintf("channel=%s class=%s status=%d provider_status=%d", channel, classification.kind, classification.public.Status, classification.providerStatus)
}

func classifyGatewayError(err error, channel string) gatewayErrorClassification {
	if err == nil {
		return gatewayErrorClassification{}
	}
	result := func(status int, kind, message string) gatewayErrorClassification {
		return gatewayErrorClassification{public: httperr.Newf(status, "%s", message), kind: kind}
	}
	name := "支付通道"
	if channel == "alipay" {
		name = "支付宝"
	} else if channel == "paypal" {
		name = "PayPal"
	}
	if value := reflect.ValueOf(err); value.Kind() == reflect.Pointer && value.IsNil() {
		return result(http.StatusInternalServerError, "internal", name+"支付处理失败，请稍后重试或改用其他支付方式")
	}
	var configuration *gatewayConfigurationError
	if errors.As(err, &configuration) && configuration != nil {
		return result(http.StatusBadRequest, "configuration", configuration.Error())
	}
	var response *gatewayResponseError
	if errors.As(err, &response) && response != nil {
		return result(http.StatusBadGateway, "invalid_response", response.Error())
	}
	var business *httperr.Error
	if errors.As(err, &business) && business != nil {
		return gatewayErrorClassification{public: business, kind: "business"}
	}
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout != nil && timeout.Timeout() {
		return result(http.StatusGatewayTimeout, "timeout", name+"连接超时，请稍后重试或改用其他支付方式")
	}
	var provider *paypal.ErrorResponse
	if channel == "paypal" && errors.As(err, &provider) && provider != nil {
		status := 0
		oauth := false
		if provider.Response != nil {
			status = provider.Response.StatusCode
			request := provider.Response.Request
			if request != nil && request.URL != nil {
				oauth = strings.TrimRight(request.URL.Path, "/") == "/v1/oauth2/token"
			}
		}
		classified := result(http.StatusBadGateway, "upstream_response", "PayPal 返回异常响应，请稍后重试或改用其他支付方式")
		switch {
		case oauth && (status == 400 || status == 401 || status == 403):
			classified = result(http.StatusBadRequest, "credentials", "PayPal 沙箱凭证校验失败，请检查 PAYPAL_CLIENT_ID / PAYPAL_CLIENT_SECRET 是否属于同一个沙箱应用")
		case status == http.StatusUnauthorized:
			classified = result(http.StatusBadRequest, "authentication", "PayPal 身份验证失败，请检查沙箱应用凭证后重试")
		case status == http.StatusForbidden:
			classified = result(http.StatusBadRequest, "permission", "PayPal 商户权限不足，请检查沙箱应用的支付权限")
		case status == http.StatusTooManyRequests:
			classified = result(http.StatusServiceUnavailable, "upstream_rate_limit", "PayPal 请求过于频繁，请稍后重试或改用其他支付方式")
		case status == http.StatusConflict:
			classified = result(http.StatusConflict, "provider_conflict", "PayPal 订单状态冲突，请刷新订单或改用其他支付方式")
		case status == http.StatusBadRequest || status == http.StatusNotFound || status == http.StatusUnprocessableEntity:
			classified = result(http.StatusBadRequest, "provider_request_error", "PayPal 支付请求未成功，请核对订单参数、重试或改用其他支付方式")
		case status >= 500:
			classified = result(http.StatusBadGateway, "upstream_failure", "PayPal 服务暂时不可用，请稍后重试或改用其他支付方式")
		}
		classified.providerStatus = status
		return classified
	}
	if channel == "alipay" {
		if errors.Is(err, alipay.ErrAliPublicKeyNotFound) {
			return result(http.StatusBadRequest, "configuration", "支付宝公钥未配置或无效，请检查 ALIPAY_PUBLIC_KEY")
		}
		if errors.Is(err, rsa.ErrVerification) || errors.Is(err, alipay.ErrSignNotFound) {
			return result(http.StatusBadGateway, "signature", "支付宝响应验签失败，请核对沙箱应用与支付宝公钥配置")
		}
		var pointer *alipay.Error
		var value alipay.Error
		var code alipay.Code
		var subCode string
		if errors.As(err, &pointer) && pointer != nil {
			code, subCode = pointer.Code, pointer.SubCode
		} else if errors.As(err, &value) {
			code, subCode = value.Code, value.SubCode
		}
		if code == alipay.CodeBusinessFailed && (strings.Contains(subCode, "SYSTEM_ERROR") || strings.Contains(subCode, "SYSTEM_BUSY")) {
			return result(http.StatusBadGateway, "upstream_failure", "支付宝服务暂时不可用，请稍后重试或改用其他支付方式")
		}
		switch code {
		case alipay.CodeInvalidAuthToken, alipay.CodePermissionDenied:
			return result(http.StatusBadRequest, "permission", "支付宝应用授权或商户权限不足，请检查沙箱应用配置")
		case alipay.CodeCallLimited:
			return result(http.StatusServiceUnavailable, "upstream_rate_limit", "支付宝请求过于频繁，请稍后重试或改用其他支付方式")
		case alipay.CodeUnknowError:
			return result(http.StatusBadGateway, "upstream_failure", "支付宝服务暂时不可用，请稍后重试或改用其他支付方式")
		case alipay.CodeMissingParam, alipay.CodeInvalidParam, alipay.CodeInsufficientConditions, alipay.CodeBusinessFailed:
			return result(http.StatusBadRequest, "provider_request_error", "支付宝支付请求未完成，请核对订单参数、重试或改用其他支付方式")
		}
	}
	var syntax *json.SyntaxError
	var incorrectType *json.UnmarshalTypeError
	if errors.As(err, &syntax) || errors.As(err, &incorrectType) || errors.Is(err, alipay.ErrBadResponse) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return result(http.StatusBadGateway, "invalid_response", name+"返回的数据格式无效，请稍后重试或改用其他支付方式")
	}
	var network net.Error
	if errors.As(err, &network) {
		return result(http.StatusBadGateway, "network", name+"连接失败，请稍后重试或改用其他支付方式")
	}
	return result(http.StatusInternalServerError, "internal", name+"支付处理失败，请稍后重试或改用其他支付方式")
}
