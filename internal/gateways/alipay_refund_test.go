package gateways

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smartwalle/alipay/v3"

	"sandbox-pay/internal/config"
)

type alipayRefundFunc func(context.Context, alipay.TradeRefund) (*alipay.TradeRefundRsp, error)

func (f alipayRefundFunc) TradeRefund(ctx context.Context, input alipay.TradeRefund) (*alipay.TradeRefundRsp, error) {
	return f(ctx, input)
}

func successfulAlipayRefund(input alipay.TradeRefund, fundChange string) *alipay.TradeRefundRsp {
	return &alipay.TradeRefundRsp{
		Error: alipay.Error{Code: alipay.CodeSuccess}, OutTradeNo: input.OutTradeNo,
		TradeNo: "ALIPAY-TRADE", RefundFee: input.RefundAmount, FundChange: fundChange,
	}
}

func TestAlipayRefundReusesDurableID(t *testing.T) {
	a := NewAlipay(config.Map{}, nil, nil)
	calls := 0
	a.refundSDK = alipayRefundFunc(func(ctx context.Context, input alipay.TradeRefund) (*alipay.TradeRefundRsp, error) {
		if input.OutRequestNo != "RF_OPERATION" || input.OutTradeNo != "PAY_ONE" || input.RefundAmount != "50.00" || input.RefundReason != "test reason" {
			t.Errorf("refund input changed: %+v", input)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 10*time.Second || time.Until(deadline) < 9*time.Second {
			t.Error("refund request lacks the 10 second deadline")
		}
		calls++
		fundChange := "Y"
		if calls > 1 {
			fundChange = "N"
		}
		return successfulAlipayRefund(input, fundChange), nil
	})
	for attempt := 0; attempt < 2; attempt++ {
		id, status, err := a.RefundWithStatus("PAY_ONE", 5000, "test reason", "RF_OPERATION")
		if id != "RF_OPERATION" || status != "COMPLETED" || err != nil {
			t.Fatalf("refund/replay: id=%s status=%s error=%v", id, status, err)
		}
	}
}

func TestAlipayRefundFailureClassification(t *testing.T) {
	cases := []struct {
		name     string
		response *alipay.TradeRefundRsp
		err      error
		want     string
	}{
		{name: "timeout", err: context.DeadlineExceeded, want: "UNKNOWN"},
		{name: "transport", err: errors.New("connection lost after request"), want: "UNKNOWN"},
		{name: "empty response", want: "UNKNOWN"},
		{name: "signature failure", err: errors.New("invalid provider signature"), want: "UNKNOWN"},
		{name: "system unavailable", response: &alipay.TradeRefundRsp{Error: alipay.Error{Code: alipay.CodeUnknowError}}, want: "UNKNOWN"},
		{name: "business system error", response: &alipay.TradeRefundRsp{Error: alipay.Error{Code: alipay.CodeBusinessFailed, SubCode: "ACQ.SYSTEM_ERROR"}}, want: "UNKNOWN"},
		{name: "business rejected", response: &alipay.TradeRefundRsp{Error: alipay.Error{Code: alipay.CodeBusinessFailed, SubCode: "ACQ.REFUND_AMT_NOT_EQUAL_TOTAL"}}, want: "FAILED"},
		{name: "business error return", err: &alipay.Error{Code: alipay.CodeInvalidParam}, want: "FAILED"},
		{name: "message alone", response: &alipay.TradeRefundRsp{Error: alipay.Error{Code: alipay.CodeBusinessFailed, Msg: "Success"}}, want: "FAILED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAlipay(config.Map{}, nil, nil)
			a.refundSDK = alipayRefundFunc(func(context.Context, alipay.TradeRefund) (*alipay.TradeRefundRsp, error) {
				return tc.response, tc.err
			})
			id, status, err := a.RefundWithStatus("PAY_ONE", 5000, "", "RF_OPERATION")
			if id != "" || status != tc.want || err == nil {
				t.Fatalf("refund outcome: id=%s status=%s error=%v, want %s/error", id, status, err, tc.want)
			}
		})
	}
}

func TestAlipayRefundResponseValidation(t *testing.T) {
	cases := map[string]func(*alipay.TradeRefundRsp){
		"wrong order":            func(r *alipay.TradeRefundRsp) { r.OutTradeNo = "PAY_OTHER" },
		"wrong amount":           func(r *alipay.TradeRefundRsp) { r.RefundFee = "49.99" },
		"missing amount":         func(r *alipay.TradeRefundRsp) { r.RefundFee = "" },
		"fractional cent":        func(r *alipay.TradeRefundRsp) { r.RefundFee = "50.001" },
		"invalid amount":         func(r *alipay.TradeRefundRsp) { r.RefundFee = "NaN" },
		"missing fund change":    func(r *alipay.TradeRefundRsp) { r.FundChange = "" },
		"unexpected fund change": func(r *alipay.TradeRefundRsp) { r.FundChange = "PENDING" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			a := NewAlipay(config.Map{}, nil, nil)
			a.refundSDK = alipayRefundFunc(func(_ context.Context, input alipay.TradeRefund) (*alipay.TradeRefundRsp, error) {
				response := successfulAlipayRefund(input, "Y")
				mutate(response)
				return response, nil
			})
			id, status, err := a.RefundWithStatus("PAY_ONE", 5000, "", "RF_OPERATION")
			if id != "" || status != "UNKNOWN" || err == nil {
				t.Fatalf("mismatched response confirmed refund: id=%s status=%s error=%v", id, status, err)
			}
		})
	}
}

func TestAlipayRefundConfigurationAndInputFailure(t *testing.T) {
	a := NewAlipay(config.Map{}, nil, nil)
	for _, requestID := range []string{"RF_OPERATION", "", strings.Repeat("x", 65), "bad\nID"} {
		id, status, err := a.RefundWithStatus("PAY_ONE", 5000, "", requestID)
		if id != "" || status != "FAILED" || err == nil {
			t.Fatalf("configuration/input outcome: id=%s status=%s error=%v", id, status, err)
		}
	}
}

func TestAlipayLegacyRefundIDsAreUnique(t *testing.T) {
	a := NewAlipay(config.Map{}, nil, nil)
	var mu sync.Mutex
	ids := make(map[string]bool)
	a.refundSDK = alipayRefundFunc(func(_ context.Context, input alipay.TradeRefund) (*alipay.TradeRefundRsp, error) {
		mu.Lock()
		if ids[input.OutRequestNo] || len(input.OutRequestNo) != 35 || !strings.HasPrefix(input.OutRequestNo, "RF_") {
			t.Errorf("legacy refund ID is not unique: %s", input.OutRequestNo)
		}
		ids[input.OutRequestNo] = true
		mu.Unlock()
		return successfulAlipayRefund(input, "Y"), nil
	})
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if id := a.Refund("PAY_ONE", 5000, ""); id == "" {
				t.Error("legacy refund unexpectedly failed")
			}
		}()
	}
	wg.Wait()
	if len(ids) != 32 {
		t.Fatalf("generated %d distinct IDs, want 32", len(ids))
	}
}
