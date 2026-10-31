package routes

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestCashierInvalidAlipayKeyExplainsFailureWithoutChangingOrder(t *testing.T) {
	h := newHarness(t, map[string]string{
		"ALIPAY_APP_ID": "test-app", "ALIPAY_PRIVATE_KEY": "invalid-private-value",
		"ALIPAY_PUBLIC_KEY": "invalid-public-value",
	})
	_, orderNo := h.createOrder(t, map[string]any{
		"amount": 1.5, "productName": "gateway credentials regression", "payMethod": "native",
	})
	failed := h.do(t, reqSpec{
		Method: "POST", Path: "/v1/api/native-pay/switch-channel",
		Body: map[string]any{"orderNo": orderNo, "channel": "alipay"},
	})
	eq(t, "invalid key status", failed.Status, http.StatusBadRequest)
	message := str(t, failed.obj()["message"], "message")
	hasSubstr(t, message, "ALIPAY_PRIVATE_KEY", "actionable field")
	if strings.Contains(message, "invalid-private-value") || strings.Contains(message, "invalid-public-value") {
		t.Fatal("credential values must not appear in the public error")
	}
	if str(t, failed.obj()["correlationId"], "correlationId") == "" {
		t.Fatal("gateway failure requires a traceable request ID")
	}
	stored, err := h.store.Orders.FindByOrderNo(orderNo, false)
	if err != nil || stored == nil || stored.PayMethod != "native" || stored.Amount != 150 {
		t.Fatalf("key parsing failure changed the order: %+v err=%v", stored, err)
	}
}

func TestCashierUnconfiguredGatewayPreservesWallet(t *testing.T) {
	for _, channel := range []string{"alipay", "paypal"} {
		t.Run(channel, func(t *testing.T) {
			h := newHarness(t, nil)
			_, orderNo := h.createOrder(t, map[string]any{
				"amount": 1.5, "productName": "cashier recovery", "payMethod": "native",
				"returnUrl": "https://merchant.example/complete?cart=42#receipt",
			})
			switched := h.do(t, reqSpec{
				Method: "POST", Path: "/v1/api/native-pay/switch-channel",
				Body: map[string]any{"orderNo": orderNo, "channel": channel},
			})
			eq(t, "unconfigured channel", switched.Status, http.StatusBadRequest)
			cashier := h.do(t, reqSpec{
				Method: "GET", Path: "/v1/api/native-pay/cashier?orderNo=" + orderNo,
			})
			eq(t, "cashier status", cashier.Status, http.StatusOK)
			data := cashier.data()
			eq(t, "channel after failed creation", str(t, data["payMethod"], "payMethod"), "native")
			eq(t, "return destination", str(t, data["returnUrl"], "returnUrl"), "https://merchant.example/complete?cart=42#receipt")
			channels := data["availableChannels"].([]any)
			if len(channels) != 3 || channels[0] != "native" {
				t.Fatalf("wallet unavailable after failed creation: %v", channels)
			}
			confirmed := h.do(t, reqSpec{
				Method: "POST", Path: "/v1/api/native-pay/sandbox-confirm",
				Body: map[string]any{"orderNo": orderNo, "walletUser": "demo_buyer@sandbox-pay.local", "walletPass": "123456"},
			})
			eq(t, "wallet can still confirm", confirmed.Status, http.StatusOK)
		})
	}
}

func TestCashierGatewayReturnPreservesQueryAndFragment(t *testing.T) {
	value := buildReturnURL("https://merchant.example/complete?cart=42&orderNo=old&status=old#receipt", "PAY +/42", "success")
	target, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	if target.Fragment != "receipt" || target.Query().Get("cart") != "42" || target.Query().Get("orderNo") != "PAY +/42" || target.Query().Get("status") != "success" {
		t.Fatalf("incorrect provider return: %s", value)
	}
	if len(target.Query()["orderNo"]) != 1 || len(target.Query()["status"]) != 1 {
		t.Fatalf("duplicate callback parameters: %s", value)
	}
}

func TestCashierPendingExternalSelectionCanReturnToWalletAfterGatewayFailure(t *testing.T) {
	for _, channel := range []string{"alipay", "paypal"} {
		t.Run(channel, func(t *testing.T) {
			h := newHarness(t, nil)
			_, orderNo := h.createOrder(t, map[string]any{
				"amount": 1.5, "productName": "cashier locked", "payMethod": "native",
			})
			patch := map[string]any{"payMethod": channel}
			if channel == "paypal" {
				patch["providerOrderId"], patch["foreignAmount"], patch["foreignCurrency"], patch["exchangeRate"] = "PP-EXISTING", int64(21), "USD", 0.14
			}
			if err := h.store.Orders.Update(orderNo, patch); err != nil {
				t.Fatal(err)
			}
			other := "alipay"
			if channel == other {
				other = "paypal"
			}
			switched := h.do(t, reqSpec{
				Method: "POST", Path: "/v1/api/native-pay/switch-channel",
				Body: map[string]any{"orderNo": orderNo, "channel": other},
			})
			eq(t, "other method reaches gateway instead of permanent lock", switched.Status, http.StatusBadRequest)
			confirmed := h.do(t, reqSpec{
				Method: "POST", Path: "/v1/api/native-pay/sandbox-confirm",
				Body: map[string]any{"orderNo": orderNo, "walletUser": "demo_buyer@sandbox-pay.local", "walletPass": "123456"},
			})
			eq(t, "external order cannot use wallet", confirmed.Status, http.StatusBadRequest)
			cashier := h.do(t, reqSpec{
				Method: "GET", Path: "/v1/api/native-pay/cashier?orderNo=" + orderNo,
			})
			data := cashier.data()
			channels := data["availableChannels"].([]any)
			if len(channels) != 3 || data["channelLockReason"] != nil {
				t.Fatalf("pending order permanently restricted its methods: %v", data)
			}
			// Retrying the selected channel reaches its configuration check;
			// it is not rejected as a forbidden channel switch.
			retry := h.do(t, reqSpec{
				Method: "POST", Path: "/v1/api/native-pay/switch-channel",
				Body: map[string]any{"orderNo": orderNo, "channel": channel},
			})
			eq(t, "same-channel retry permitted", retry.Status, http.StatusBadRequest)
			native := h.do(t, reqSpec{
				Method: "POST", Path: "/v1/api/native-pay/switch-channel",
				Body: map[string]any{"orderNo": orderNo, "channel": "native"},
			})
			eq(t, "can select wallet", native.Status, http.StatusOK)
			eq(t, "same merchant order", cashierOrderNo(t, native), orderNo)
			stored, err := h.store.Orders.FindByOrderNo(orderNo, false)
			if err != nil || stored == nil || stored.PayMethod != "native" || stored.Amount != 150 {
				t.Fatalf("selection replaced or changed order: %+v err=%v", stored, err)
			}
			if channel == "paypal" && (stored.ProviderOrderID == nil || *stored.ProviderOrderID != "PP-EXISTING" || stored.ForeignAmount == nil || *stored.ForeignAmount != 21) {
				t.Fatalf("selection lost existing PayPal retry binding: %+v", stored)
			}
			paid := h.do(t, reqSpec{
				Method: "POST", Path: "/v1/api/native-pay/sandbox-confirm",
				Body: map[string]any{"orderNo": orderNo, "walletUser": "demo_buyer@sandbox-pay.local", "walletPass": "123456"},
			})
			eq(t, "wallet payment after selection", paid.Status, http.StatusOK)
		})
	}
}
