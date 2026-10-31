package services

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
)

func TestGroupCreateRollsBackOnKeyFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "groups.db")
	store, err := db.Open(path, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if _, err := conn.Exec(`CREATE TRIGGER reject_key BEFORE INSERT ON "api_keys" BEGIN SELECT RAISE(ABORT, 'reject key'); END`); err != nil {
		t.Fatal(err)
	}
	service := NewGroupService(store, config.Map(map[string]string{}))
	created, err := service.CreateGroup("failed group", "first key")
	if err == nil || created != nil {
		t.Fatalf("failed key insertion returned group=%+v error=%v", created, err)
	}
	if count := groupCount(t, store); count != 0 {
		t.Fatalf("failed key insertion left %d empty groups", count)
	}
	if _, err := conn.Exec(`DROP TRIGGER reject_key`); err != nil {
		t.Fatal(err)
	}
	created, err = service.CreateGroup("successful retry", "first key")
	if err != nil || created == nil || len(created.Keys) != 1 {
		t.Fatalf("retry after failed insertion: group=%+v error=%v", created, err)
	}
	if count := groupCount(t, store); count != 1 {
		t.Fatalf("retry created %d groups, want 1", count)
	}
}

func TestGroupConcurrentToggles(t *testing.T) {
	for _, target := range []string{"key", "group"} {
		t.Run(target, func(t *testing.T) {
			fixture := createGroupService(t, map[string]string{"GROUP_API_KEY": "sp_concurrent"})
			group, key, _ := fixture.ensureSeed(t)
			toggle := func() (bool, error) {
				if target == "key" {
					result, err := fixture.service.ToggleKey(key.ID)
					if err != nil {
						return false, err
					}
					return result.IsActive, nil
				}
				result, err := fixture.service.ToggleGroup(group.ID)
				if err != nil {
					return false, err
				}
				return result.IsActive, nil
			}
			const requests = 32
			start := make(chan struct{})
			results := make(chan bool, requests)
			errors := make(chan error, requests)
			var wg sync.WaitGroup
			for i := 0; i < requests; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					active, err := toggle()
					results <- active
					errors <- err
				}()
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
			activeCount := 0
			for active := range results {
				if active {
					activeCount++
				}
			}
			if activeCount != requests/2 {
				t.Errorf("%d toggles returned %d enabled states, want %d; concurrent toggles were lost", requests, activeCount, requests/2)
			}
			storedGroup, err := fixture.store.Groups.FindByID(group.ID)
			if err != nil || storedGroup == nil || !storedGroup.IsActive {
				t.Fatalf("even toggles changed final group state: group=%+v error=%v", storedGroup, err)
			}
			storedKey, err := fixture.store.ApiKeys.FindByID(key.ID)
			if err != nil || storedKey == nil || !storedKey.IsActive {
				t.Fatalf("even toggles changed final key state: key=%+v error=%v", storedKey, err)
			}
		})
	}
}

func TestGroupKeyToggleReturnsUpdatedSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "groups.db")
	store, err := db.Open(path, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	service := NewGroupService(store, config.Map(map[string]string{"GROUP_API_KEY": "sp_snapshot"}))
	_, key, _, err := service.EnsureSeed()
	if err != nil {
		t.Fatal(err)
	}
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if _, err := conn.Exec(`UPDATE "api_keys" SET "updatedAt" = '2000-01-01 00:00:00.000' WHERE "id" = ?`, key.ID); err != nil {
		t.Fatal(err)
	}
	result, err := service.ToggleKey(key.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.ApiKeys.FindByID(key.ID)
	if err != nil || stored == nil {
		t.Fatalf("reload toggled key: %v", err)
	}
	if result.UpdatedAt != JsTime(stored.UpdatedAt) {
		t.Errorf("toggle response updatedAt=%s, stored=%s", result.UpdatedAt, JsTime(stored.UpdatedAt))
	}
}

func TestGroupIssueWhileDisabledDoesNotBypassGuard(t *testing.T) {
	fixture := createGroupService(t, map[string]string{"GROUP_API_KEY": "sp_disabled_group"})
	group, _, _ := fixture.ensureSeed(t)
	if _, err := fixture.service.ToggleGroup(group.ID); err != nil {
		t.Fatal(err)
	}
	issued, err := fixture.service.IssueKey(group.ID, "prepared rotation")
	if err != nil || issued == nil || !issued.IsActive {
		t.Fatalf("prepare key under disabled group: key=%+v error=%v", issued, err)
	}
	if hit, err := fixture.store.ApiKeys.FindActiveByKey(issued.Key); err != nil || hit != nil {
		t.Fatalf("disabled group accepted new key: hit=%+v error=%v", hit, err)
	}
	if _, err := fixture.service.ToggleGroup(group.ID); err != nil {
		t.Fatal(err)
	}
	if hit, err := fixture.store.ApiKeys.FindActiveByKey(issued.Key); err != nil || hit == nil || hit.Key.ID != issued.ID {
		t.Fatalf("restored group did not accept new key: hit=%+v error=%v", hit, err)
	}
}

func TestGroupCredentialsAndDisabledStatesSurviveReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "groups.db")
	store, err := db.Open(path, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	service := NewGroupService(store, config.Map(map[string]string{}))
	group, err := service.CreateGroup("persisted group", "first")
	if err != nil {
		t.Fatal(err)
	}
	issued, err := service.IssueKey(group.ID, "second")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ToggleKey(issued.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ToggleGroup(group.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := db.Open(path, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	restarted := NewGroupService(reopened, config.Map(map[string]string{"GROUP_API_KEY": "different_environment_key"}))
	if err := restarted.Init(); err != nil {
		t.Fatal(err)
	}
	list, err := restarted.ListGroups()
	if err != nil || len(list) != 1 || len(list[0].Keys) != 2 {
		t.Fatalf("reopened credentials: groups=%+v error=%v", list, err)
	}
	if list[0].IsActive || list[0].Keys[1].IsActive {
		t.Fatal("reopening restored disabled group or key")
	}
	if list[0].Keys[0].Key != "" || list[0].Keys[1].Key != "" {
		t.Fatal("reopening exposed credential plaintext")
	}
	for index, secret := range []string{group.Keys[0].Key, issued.Key} {
		stored, err := reopened.ApiKeys.FindByKey(secret)
		if err != nil || stored == nil || stored.ID != list[0].Keys[index].ID {
			t.Fatalf("reopening changed issued credential %d: %v", index, err)
		}
	}
}
