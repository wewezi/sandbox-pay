package services

import (
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestPaymentExpiryInputValidatesBeforeCreatingOrder(t *testing.T) {
	for _, minutes := range []int{-1, 0, 121, int(^uint(0) >> 1)} {
		t.Run(strconv.Itoa(minutes), func(t *testing.T) {
			store := newTestStore(t)
			groupID := seedGroup1(t, store)
			payment := NewPaymentService(store, nil, nil)
			_, err := payment.CreateOrder(OrderCaller{GroupID: groupID}, CreateOrderInput{
				Amount: 1, ProductName: "Invalid expiry regression", PayMethod: "native", ExpireMinutes: &minutes,
			})
			requireErrStatus(t, err, http.StatusBadRequest)
			count, err := store.Orders.Count()
			if err != nil || count != 0 {
				t.Fatal("invalid service-level expiration input created an order")
			}
		})
	}
}

func TestPaymentExpiryInputDefaultsAndBoundsPersistActualDeadline(t *testing.T) {
	minimum, maximum := 1, 120
	for _, test := range []struct {
		name    string
		input   *int
		minutes int
	}{
		{name: "default", minutes: 30},
		{name: "minimum", input: &minimum, minutes: minimum},
		{name: "maximum", input: &maximum, minutes: maximum},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newTestStore(t)
			groupID := seedGroup1(t, store)
			payment := NewPaymentService(store, nil, nil)
			created, err := payment.CreateOrder(OrderCaller{GroupID: groupID}, CreateOrderInput{
				Amount: 1, ProductName: "Service expiration regression", PayMethod: "native", ExpireMinutes: test.input,
			})
			if err != nil {
				t.Fatal(err)
			}
			stored := reloadOrder(t, store, created.OrderNo)
			deviation := stored.ExpireAt.Sub(stored.CreatedAt) - time.Duration(test.minutes)*time.Minute
			if deviation < -time.Second || deviation > time.Second {
				t.Fatalf("stored duration does not match %d minutes: %s", test.minutes, stored.ExpireAt.Sub(stored.CreatedAt))
			}
			if created.ExpireAt != JsTime(stored.ExpireAt) {
				t.Fatal("service receipt does not report the persisted expiration time")
			}
		})
	}
}

func TestPaymentExpiryInputIdempotentRetryNeverExtendsDeadline(t *testing.T) {
	store := newTestStore(t)
	groupID := seedGroup1(t, store)
	payment := NewPaymentService(store, nil, nil)
	external := "SERVICE-EXPIRATION-42"
	firstMinutes, retryMinutes := 1, 120
	originalInput := CreateOrderInput{
		Amount: 1, ProductName: "Original service product", PayMethod: "native",
		ExpireMinutes: &firstMinutes, ExternalOrderNo: &external,
	}
	first, err := payment.CreateOrder(OrderCaller{GroupID: groupID}, originalInput)
	if err != nil {
		t.Fatal(err)
	}
	original := reloadOrder(t, store, first.OrderNo)
	retryInput := originalInput
	retryInput.ExpireMinutes = &retryMinutes
	retryInput.ProductName = "Changed service product"
	retry, err := payment.CreateOrder(OrderCaller{GroupID: groupID}, retryInput)
	if err != nil {
		t.Fatal(err)
	}
	if retry.OrderNo != first.OrderNo || retry.ExpireAt != first.ExpireAt {
		t.Fatal("retry replaced the order or extended its deadline")
	}
	stored := reloadOrder(t, store, first.OrderNo)
	if !stored.ExpireAt.Equal(original.ExpireAt) || !stored.CreatedAt.Equal(original.CreatedAt) || stored.ProductName != original.ProductName {
		t.Fatal("retry changed immutable original order context")
	}
	retryInput.PayMethod = "alipay"
	_, err = payment.CreateOrder(OrderCaller{GroupID: groupID}, retryInput)
	requireErrStatus(t, err, http.StatusConflict)
	count, err := store.Orders.Count()
	if err != nil || count != 1 {
		t.Fatal("retry or conflicting method inserted another order")
	}
}
