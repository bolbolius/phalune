package notify

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreAddRemoveClear(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "notifications.json")

	store := NewStore(storePath)

	changedCount := 0
	unsub := store.Subscribe(func() {
		changedCount++
	})
	defer unsub()

	n1 := Notification{
		ID:      1,
		AppName: "Slack",
		Summary: "New message",
		Body:    "Hey there",
		Urgency: UrgencyNormal,
	}
	n2 := Notification{
		ID:      2,
		AppName: "Firefox",
		Summary: "Download complete",
		Body:    "file.iso",
		Urgency: UrgencyLow,
	}

	store.Add(n1)
	store.Add(n2)

	if store.Count() != 2 {
		t.Fatalf("expected 2 items, got %d", store.Count())
	}
	if changedCount != 2 {
		t.Fatalf("expected 2 change events, got %d", changedCount)
	}

	grouped := store.Grouped()
	if len(grouped) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(grouped))
	}

	// Test persistence reload
	store2 := NewStore(storePath)
	if store2.Count() != 2 {
		t.Fatalf("expected 2 items in reloaded store, got %d", store2.Count())
	}

	// Test Remove
	store.Remove(1)
	if store.Count() != 1 {
		t.Fatalf("expected 1 item after remove, got %d", store.Count())
	}

	// Test Clear
	store.Clear()
	if store.Count() != 0 {
		t.Fatalf("expected 0 items after clear, got %d", store.Count())
	}
}

func TestStoreGrouping(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "notifs.json")
	store := NewStore(storePath)

	store.Add(Notification{ID: 1, AppName: "AppA", Summary: "Msg 1"})
	time.Sleep(2 * time.Millisecond)
	store.Add(Notification{ID: 2, AppName: "AppB", Summary: "Msg 2"})
	time.Sleep(2 * time.Millisecond)
	store.Add(Notification{ID: 3, AppName: "AppA", Summary: "Msg 3"})

	grouped := store.Grouped()
	if len(grouped) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(grouped))
	}

	var appAGroup *GroupedNotifications
	for i := range grouped {
		if grouped[i].AppName == "AppA" {
			appAGroup = &grouped[i]
		}
	}
	if appAGroup == nil || len(appAGroup.Items) != 2 {
		t.Fatalf("expected 2 items in AppA group, got %v", appAGroup)
	}

	store.RemoveGroup("AppA")
	if store.Count() != 1 {
		t.Fatalf("expected 1 item after RemoveGroup, got %d", store.Count())
	}
}
