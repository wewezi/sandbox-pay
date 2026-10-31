package db

import (
	"fmt"
	"testing"
	"time"
)

func TestSandboxTaskLeaseAndScopedList(t *testing.T) {
	store := newRepoStore(t)
	first, err := store.Groups.Insert("first", true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Groups.Insert("second", true)
	if err != nil {
		t.Fatal(err)
	}
	for i, group := range []*Group{first, second} {
		no := fmt.Sprintf("SCOPED-%d", i)
		if err := store.Orders.Insert(&PaymentOrder{ID: no, OrderNo: no, Amount: 100, ProductName: "scope", PayMethod: "native", Status: StatusPending, ExpireAt: time.Now().Add(time.Hour), GroupID: &group.ID}); err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 110; j++ {
			if err := store.SandboxTasks().Insert(&SandboxTask{OrderNo: no, Scenario: "delayed_paid", RunAt: time.Now().Add(time.Hour)}); err != nil {
				t.Fatal(err)
			}
		}
	}
	items, err := store.SandboxTasks().ListScoped("", []int64{first.ID})
	if err != nil || len(items) != 100 {
		t.Fatalf("scoped page len=%d err=%v", len(items), err)
	}
	for _, item := range items {
		if item.OrderNo != "SCOPED-0" {
			t.Fatal("another project leaked")
		}
	}
	items, err = store.SandboxTasks().ListScoped("", []int64{})
	if err != nil || len(items) != 0 {
		t.Fatalf("empty scope len=%d err=%v", len(items), err)
	}
	task := &SandboxTask{OrderNo: "SCOPED-0", Scenario: "paid", RunAt: time.Now().Add(-time.Second)}
	if err := store.SandboxTasks().Insert(task); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.SandboxTasks().Claim(task.ID)
	if err != nil || !claimed {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	claimed, err = store.SandboxTasks().Claim(task.ID)
	if err != nil || claimed {
		t.Fatalf("double claim=%v err=%v", claimed, err)
	}
	if _, err := store.db.Exec(`UPDATE "sandbox_tasks" SET "updatedAt"=? WHERE "id"=?`, toSQL(time.Now().Add(-3*time.Minute)), task.ID); err != nil {
		t.Fatal(err)
	}
	due, err := store.SandboxTasks().Due()
	if err != nil || len(due) != 1 || due[0].ID != task.ID {
		t.Fatalf("expired lease due=%#v err=%v", due, err)
	}
	claimed, err = store.SandboxTasks().Claim(task.ID)
	if err != nil || !claimed {
		t.Fatalf("recovery claim=%v err=%v", claimed, err)
	}
	if err := store.SandboxTasks().Finish(task.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	due, err = store.SandboxTasks().Due()
	if err != nil || len(due) != 0 {
		t.Fatalf("completed task scheduled again %#v err=%v", due, err)
	}
}
