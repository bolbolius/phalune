package clipboard

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStore_RingBuffer(t *testing.T) {
	tmp := t.TempDir()
	store := NewStore(3, false)
	store.SetPaths(filepath.Join(tmp, "history.json"), filepath.Join(tmp, "images"))

	for i := 1; i <= 5; i++ {
		// Use slight delay to bypass debounce
		time.Sleep(2 * time.Millisecond)
		store.Add(&Entry{
			Kind: KindText,
			Text: fmt.Sprintf("item-%d", i),
		})
	}

	entries := store.Entries()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Newest first: item-5, item-4, item-3
	if entries[0].Text != "item-5" {
		t.Errorf("expected item-5 at index 0, got %s", entries[0].Text)
	}
	if entries[1].Text != "item-4" {
		t.Errorf("expected item-4 at index 1, got %s", entries[1].Text)
	}
	if entries[2].Text != "item-3" {
		t.Errorf("expected item-3 at index 2, got %s", entries[2].Text)
	}

	// Resize to 2
	store.Resize(2)
	entries = store.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after resize, got %d", len(entries))
	}
	if entries[0].Text != "item-5" || entries[1].Text != "item-4" {
		t.Errorf("unexpected entries after resize: %s, %s", entries[0].Text, entries[1].Text)
	}
}

func TestStore_Deduplication(t *testing.T) {
	tmp := t.TempDir()
	store := NewStore(5, false)
	store.SetPaths(filepath.Join(tmp, "history.json"), filepath.Join(tmp, "images"))

	store.Add(&Entry{Kind: KindText, Text: "alpha"})
	time.Sleep(2 * time.Millisecond)
	store.Add(&Entry{Kind: KindText, Text: "beta"})
	time.Sleep(2 * time.Millisecond)

	// Rapid re-copy within 400ms debounce
	store.Add(&Entry{Kind: KindText, Text: "beta"})
	if len(store.Entries()) != 2 {
		t.Fatalf("rapid duplicate should have been ignored, got %d entries", len(store.Entries()))
	}

	// Simulate re-copy after debounce window by overriding lastAdded map
	store.mu.Lock()
	store.lastAdded["alpha"] = time.Now().UnixMilli() - 1000
	store.mu.Unlock()

	store.Add(&Entry{Kind: KindText, Text: "alpha"})
	entries := store.Entries()
	if len(entries) != 2 {
		t.Fatalf("re-copying alpha should move to top, not add new entry; count=%d", len(entries))
	}
	if entries[0].Text != "alpha" {
		t.Errorf("expected alpha at top, got %s", entries[0].Text)
	}
	if entries[1].Text != "beta" {
		t.Errorf("expected beta at index 1, got %s", entries[1].Text)
	}

	// Test image deduplication and cleanup
	img1, err := store.CreateImageFile([]byte("identical-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	store.Add(&Entry{Kind: KindImage, FilePath: img1, Width: 10, Height: 10})

	// Rapid duplicate of same image dimensions: should unlink the new file
	img2, err := store.CreateImageFile([]byte("identical-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	store.Add(&Entry{Kind: KindImage, FilePath: img2, Width: 10, Height: 10})
	if _, err := os.Stat(img2); !os.IsNotExist(err) {
		t.Errorf("debounced duplicate image file should have been cleaned up")
	}

	// Duplicate after debounce: moves to top and unlinks old file
	store.mu.Lock()
	store.lastAdded["img:10x10"] = time.Now().UnixMilli() - 1000
	store.mu.Unlock()

	img3, err := store.CreateImageFile([]byte("identical-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	store.Add(&Entry{Kind: KindImage, FilePath: img3, Width: 10, Height: 10})
	if _, err := os.Stat(img1); !os.IsNotExist(err) {
		t.Errorf("superseded image file should have been cleaned up")
	}
}

func TestStore_Persistence(t *testing.T) {
	tmp := t.TempDir()
	histFile := filepath.Join(tmp, "history.json")
	imgDir := filepath.Join(tmp, "images")

	store1 := NewStore(5, true)
	store1.SetPaths(histFile, imgDir)

	imgPath, err := store1.CreateImageFile([]byte("fake-png-data"))
	if err != nil {
		t.Fatalf("create image file failed: %v", err)
	}

	store1.Add(&Entry{Kind: KindText, Text: "hello world"})
	time.Sleep(2 * time.Millisecond)
	store1.Add(&Entry{
		Kind:     KindImage,
		FilePath: imgPath,
		Width:    100,
		Height:   100,
	})

	if _, err := os.Stat(histFile); err != nil {
		t.Fatalf("history file was not persisted: %v", err)
	}

	// Reload from disk into store2
	store2 := NewStore(5, true)
	store2.SetPaths(histFile, imgDir)

	entries := store2.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries reloaded, got %d", len(entries))
	}

	if entries[0].Kind != KindImage || entries[0].FilePath != imgPath {
		t.Errorf("expected image at top of reloaded store, got %+v", entries[0])
	}
	if entries[1].Kind != KindText || entries[1].Text != "hello world" {
		t.Errorf("expected text entry at index 1, got %+v", entries[1])
	}

	// Delete image file and verify missing image is pruned on reload
	_ = os.Remove(imgPath)
	store3 := NewStore(5, true)
	store3.SetPaths(histFile, imgDir)

	entries3 := store3.Entries()
	if len(entries3) != 1 {
		t.Fatalf("expected 1 entry after missing image prune, got %d", len(entries3))
	}
	if entries3[0].Kind != KindText {
		t.Errorf("expected text entry remaining, got %+v", entries3[0])
	}
}

func TestStore_TouchRemoveClear(t *testing.T) {
	tmp := t.TempDir()
	store := NewStore(5, false)
	store.SetPaths(filepath.Join(tmp, "history.json"), filepath.Join(tmp, "images"))

	imgPath, err := store.CreateImageFile([]byte("sample-data"))
	if err != nil {
		t.Fatalf("create image failed: %v", err)
	}

	store.Add(&Entry{Kind: KindText, Text: "entry-1"})
	time.Sleep(2 * time.Millisecond)
	store.Add(&Entry{Kind: KindImage, FilePath: imgPath, Width: 50, Height: 50})

	entries := store.Entries()
	imageID := entries[0].ID
	textID := entries[1].ID

	store.Touch(textID)
	entries = store.Entries()
	if entries[1].UseCount != 1 {
		t.Errorf("expected UseCount 1 after Touch, got %d", entries[1].UseCount)
	}

	store.Remove(imageID)
	entries = store.Entries()
	if len(entries) != 1 || entries[0].ID != textID {
		t.Fatalf("expected only text entry remaining after Remove")
	}
	if _, err := os.Stat(imgPath); !os.IsNotExist(err) {
		t.Errorf("image file should have been removed on entry deletion")
	}

	store.Clear()
	if len(store.Entries()) != 0 {
		t.Errorf("expected empty store after Clear")
	}
}
