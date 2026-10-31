package db

import "testing"

func TestOrderSearchKeywordIncludesProductAndProviderTradeWithinFilters(t *testing.T) {
	s := newRepoStore(t)
	groupID := seedGroup(t, s)
	otherGroup, err := s.Groups.Insert("other", true)
	if err != nil {
		t.Fatal(err)
	}
	o := makeOrder()
	o.ProductName, o.ThirdPartyTradeNo, o.ExternalOrderNo = "排查支付商品", strPtr("TRADE-SEARCH-001"), strPtr("EXTERNAL-SEARCH-001")
	o.Status = StatusPaid
	insertOrder(t, s, o)
	other := makeOrder()
	other.GroupID, other.ProductName, other.ThirdPartyTradeNo, other.ExternalOrderNo = &otherGroup.ID, o.ProductName, o.ThirdPartyTradeNo, o.ExternalOrderNo
	other.Status = StatusPaid
	insertOrder(t, s, other)
	status := StatusPaid
	for _, keyword := range []string{o.OrderNo, "EXTERNAL-SEARCH", "支付商品", "TRADE-SEARCH"} {
		items, total, err := s.Orders.Search(SearchOpts{Keyword: keyword, GroupID: &groupID, Status: &status, PayMethod: "native", Limit: 20})
		if err != nil || total != 1 || len(items) != 1 || items[0].OrderNo != o.OrderNo {
			t.Fatalf("keyword %q did not respect combined filters: %+v total=%d err=%v", keyword, items, total, err)
		}
	}
	status = StatusPending
	items, total, err := s.Orders.Search(SearchOpts{Keyword: "支付商品", GroupID: &groupID, Status: &status, Limit: 20})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("keyword bypassed status filter: items=%+v total=%d err=%v", items, total, err)
	}
}

func TestOrderMetadataPreservesPendingNotificationStatusZero(t *testing.T) {
	s := newRepoStore(t)
	seedGroup(t, s)
	o := makeOrder()
	insertOrder(t, s, o)
	if err := s.Notifies.Insert(InsertNotifyInput{OrderNo: &o.OrderNo, URL: "https://merchant.example/notify", Body: "{}", Signature: "private", Status: NotifyPending}); err != nil {
		t.Fatal(err)
	}
	metadata, err := s.Orders.MetadataByOrderNos([]string{o.OrderNo})
	if err != nil {
		t.Fatal(err)
	}
	meta := metadata[o.OrderNo]
	if meta.NotifyStatus == nil || *meta.NotifyStatus != NotifyPending || meta.NotifyRetryCount == nil || *meta.NotifyRetryCount != 0 {
		t.Fatalf("pending status/retry zero confused with no notification: %+v", meta)
	}
	metaWithoutOrders, err := s.Orders.MetadataByOrderNos(nil)
	if err != nil || len(metaWithoutOrders) != 0 {
		t.Fatalf("empty page metadata: %+v err=%v", metaWithoutOrders, err)
	}
}

func TestNotificationOrderFilterCombinesWithStatusAndPagination(t *testing.T) {
	s := newRepoStore(t)
	seedGroup(t, s)
	o := makeOrder()
	insertOrder(t, s, o)
	other := makeOrder()
	insertOrder(t, s, other)
	for _, n := range []InsertNotifyInput{
		{OrderNo: &o.OrderNo, URL: "https://merchant.example/notify", Body: "matching-success", Signature: "private", Status: NotifySuccess},
		{OrderNo: &o.OrderNo, URL: "https://merchant.example/notify", Body: "matching-failed", Signature: "private", Status: NotifyFailed},
		{OrderNo: &other.OrderNo, URL: "https://other.example/notify", Body: "other-failed", Signature: "private", Status: NotifyFailed},
	} {
		if err := s.Notifies.Insert(n); err != nil {
			t.Fatal(err)
		}
	}
	status := NotifyFailed
	items, total, err := s.Notifies.Search(NotifySearchOpts{OrderNo: o.OrderNo, Status: &status, Limit: 1})
	if err != nil || total != 1 || len(items) != 1 || items[0].Body != "matching-failed" {
		t.Fatalf("combined order/status filter: items=%+v total=%d err=%v", items, total, err)
	}
	items, total, err = s.Notifies.Search(NotifySearchOpts{OrderNo: o.OrderNo, Status: &status, Limit: 1, Offset: 1})
	if err != nil || total != 1 || len(items) != 0 {
		t.Fatalf("filtered pagination count: items=%+v total=%d err=%v", items, total, err)
	}
}
