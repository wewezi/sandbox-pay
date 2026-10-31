package db

import "testing"

func TestNotifySearchProjectsFiltersBeforeCountAndPagination(t *testing.T) {
	store := newRepoStore(t)
	first := seedGroup(t, store)
	secondGroup, err := store.Groups.Insert("second", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range []int64{first, secondGroup.ID} {
		order := makeOrder()
		order.GroupID = &group
		if err := store.Orders.Insert(order); err != nil {
			t.Fatal(err)
		}
		if err := store.Notifies.Insert(InsertNotifyInput{OrderNo: &order.OrderNo, URL: "https://example.test/callback", Body: "{}", Signature: "{}", Status: NotifyPending}); err != nil {
			t.Fatal(err)
		}
	}
	unknown := "removed-order"
	if err := store.Notifies.Insert(InsertNotifyInput{OrderNo: &unknown, URL: "https://example.test/callback", Body: "{}", Signature: "{}", Status: NotifyPending}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		ids   []int64
		total int64
	}{{nil, 3}, {[]int64{first}, 1}, {[]int64{secondGroup.ID}, 1}, {[]int64{}, 0}} {
		items, total, err := store.Notifies.Search(NotifySearchOpts{ProjectIDs: test.ids, Limit: 1})
		if err != nil || total != test.total {
			t.Fatalf("scope=%v total=%d err=%v", test.ids, total, err)
		}
		if len(items) > 1 || test.total == 0 && len(items) != 0 {
			t.Fatalf("scope=%v items=%v", test.ids, items)
		}
	}
	items, total, err := store.Notifies.Search(NotifySearchOpts{ProjectIDs: []int64{first}, Offset: 1, Limit: 1})
	if err != nil || total != 1 || len(items) != 0 {
		t.Fatalf("filtered pagination items=%v total=%d err=%v", items, total, err)
	}
}
