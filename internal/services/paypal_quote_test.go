package services

import (
	"encoding/json"
	"math"
	"strconv"
	"sync"
	"testing"
	"time"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
)

func testQuoteRates(rate string) *ExchangeRateService {
	return NewExchangeRateService(nil, config.Map{"EXCHANGE_RATE_PROVIDER": "fixed", "FALLBACK_CNY_TO_USD": rate, "EXCHANGE_RATE_MARGIN": "1"})
}

func TestPayPalQuoteUsesCNYCentsAndLocksDisplayAmount(t *testing.T) {
	store := newTestStore(t)
	svc, _ := createPaymentService(store)
	order := &db.PaymentOrder{ID: "quote", OrderNo: "PAY-QUOTE", Amount: 10000, PayMethod: "native", Status: db.StatusPending, ExpireAt: time.Now().Add(time.Hour)}
	insertOrder(t, store, order)
	quote, err := svc.EnsurePayPalQuote(order.OrderNo, testQuoteRates("0.14"))
	if err != nil || !quote.Available || quote.Amount == nil || *quote.Amount != 14 || quote.ExchangeRate == nil || *quote.ExchangeRate != 0.14 || quote.Currency != "USD" {
		t.Fatalf("CNY100 did not quote USD14: quote=%+v err=%v", quote, err)
	}
	stored := reloadOrder(t, store, order.OrderNo)
	if stored.Amount != 10000 || stored.PayMethod != "native" || stored.ForeignAmount == nil || *stored.ForeignAmount != 1400 {
		t.Fatalf("quote changed CNY amount/selection: %+v", stored)
	}
	quote, err = svc.EnsurePayPalQuote(order.OrderNo, testQuoteRates("0.30"))
	if err != nil || quote.Amount == nil || *quote.Amount != 14 || quote.ExchangeRate == nil || *quote.ExchangeRate != 0.14 {
		t.Fatalf("quoted price changed on retry: %+v err=%v", quote, err)
	}
	raw, err := json.Marshal(quote)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body["amount"] != float64(14) || body["exchangeRate"] != 0.14 || body["currency"] != "USD" || body["available"] != true {
		t.Fatalf("incorrect major-unit DTO: %s", raw)
	}
}

func TestPayPalQuoteRejectsRawAmountsBelowOneUSCent(t *testing.T) {
	for _, amount := range []int64{1, 4, 7} {
		t.Run(strconv.FormatInt(amount, 10)+" CNY cents", func(t *testing.T) {
			store := newTestStore(t)
			svc, _ := createPaymentService(store)
			order := &db.PaymentOrder{ID: "minimum", OrderNo: "PAY-MINIMUM", Amount: amount, PayMethod: "native", Status: db.StatusPending, ExpireAt: time.Now().Add(time.Hour)}
			insertOrder(t, store, order)
			quote, err := svc.EnsurePayPalQuote(order.OrderNo, testQuoteRates("0.14"))
			if err != nil || quote.Available || quote.Amount != nil || quote.Reason != paypalMinimumReason {
				t.Fatalf("below-minimum amount rounded up: amount=%d quote=%+v err=%v", amount, quote, err)
			}
			stored := reloadOrder(t, store, order.OrderNo)
			if stored.ForeignAmount != nil || stored.Amount != amount || stored.PayMethod != "native" {
				t.Fatalf("unpayable quote modified merchant amount: %+v", stored)
			}
		})
	}
}

func TestPayPalQuoteChecksLegacyMinimumAndMismatchedSnapshots(t *testing.T) {
	for _, scenario := range []struct {
		name        string
		amount, usd int64
		rate        float64
	}{
		{name: "legacy forced cent", amount: 1, usd: 1, rate: 0.14},
		{name: "wrong unit conversion", amount: 10000, usd: 10000, rate: 0.14},
		{name: "invalid rate", amount: 10000, usd: 1400, rate: math.Inf(1)},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			currency := "USD"
			order := &db.PaymentOrder{Amount: scenario.amount, ForeignAmount: &scenario.usd, ExchangeRate: &scenario.rate, ForeignCurrency: &currency}
			quote := PayPalQuoteForOrder(order)
			if quote.Available || quote.Amount != nil || quote.Reason == "" {
				t.Fatalf("invalid quote accepted: %+v", quote)
			}
			if _, err := json.Marshal(quote); err != nil {
				t.Fatalf("unavailable quote failed to serialize: %v", err)
			}
		})
	}
}

func TestPayPalConcurrentQuotesFreezeOneRate(t *testing.T) {
	store := newTestStore(t)
	svc, _ := createPaymentService(store)
	order := &db.PaymentOrder{ID: "quotes", OrderNo: "PAY-QUOTES", Amount: 10000, PayMethod: "native", Status: db.StatusPending, ExpireAt: time.Now().Add(time.Hour)}
	insertOrder(t, store, order)
	start := make(chan struct{})
	results := make(chan *PayPalQuote, 2)
	errors := make(chan error, 2)
	var wg sync.WaitGroup
	for _, rate := range []string{"0.14", "0.30"} {
		wg.Add(1)
		go func(rate string) {
			defer wg.Done()
			<-start
			quote, err := svc.EnsurePayPalQuote(order.OrderNo, testQuoteRates(rate))
			results <- quote
			errors <- err
		}(rate)
	}
	close(start)
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	stored := reloadOrder(t, store, order.OrderNo)
	for quote := range results {
		if quote == nil || !quote.Available || quote.Amount == nil || *quote.Amount != float64(*stored.ForeignAmount)/100 || quote.ExchangeRate == nil || *quote.ExchangeRate != *stored.ExchangeRate {
			t.Fatalf("display quote differs from frozen amount: quote=%+v stored=%+v", quote, stored)
		}
	}
}
