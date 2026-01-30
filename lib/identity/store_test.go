package identity

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewInviteStore(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")
	if store == nil {
		t.Fatal("NewInviteStore returned nil")
	}

	if store.GeneratedCount() != 0 {
		t.Error("generated count should be 0")
	}
	if store.PendingCount() != 0 {
		t.Error("pending count should be 0")
	}
	if store.AcceptedCount() != 0 {
		t.Error("accepted count should be 0")
	}
}

func TestInviteStore_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "invites.json")

	// Create identity for invites
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("test-network")

	// Create store and add invites
	store := NewInviteStore(storePath)

	inv, err := NewInvite(id, DefaultInviteOptions())
	if err != nil {
		t.Fatalf("NewInvite failed: %v", err)
	}

	key := store.AddGenerated(inv)
	if key == "" {
		t.Error("AddGenerated should return a key")
	}

	if store.GeneratedCount() != 1 {
		t.Errorf("generated count should be 1, got %d", store.GeneratedCount())
	}

	// Save
	if err := store.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load
	loaded, err := LoadInviteStore(storePath)
	if err != nil {
		t.Fatalf("LoadInviteStore failed: %v", err)
	}

	if loaded.GeneratedCount() != 1 {
		t.Errorf("loaded generated count should be 1, got %d", loaded.GeneratedCount())
	}

	loadedInv, ok := loaded.GetGenerated(key)
	if !ok {
		t.Fatal("GetGenerated should find invite")
	}
	if loadedInv.I2PDest != inv.I2PDest {
		t.Errorf("I2P dest mismatch: got %q, want %q", loadedInv.I2PDest, inv.I2PDest)
	}
}

func TestLoadInviteStore_NotExists(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "nonexistent.json")

	store, err := LoadInviteStore(storePath)
	if err != nil {
		t.Fatalf("LoadInviteStore should not error on missing file: %v", err)
	}
	if store == nil {
		t.Fatal("store should not be nil")
	}
	if store.GeneratedCount() != 0 {
		t.Error("empty store should have 0 generated")
	}
}

func TestLoadInviteStore_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "invalid.json")

	if err := os.WriteFile(storePath, []byte("not valid json"), 0o600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := LoadInviteStore(storePath)
	if err == nil {
		t.Error("LoadInviteStore should error on invalid JSON")
	}
}

func TestInviteStore_PendingWorkflow(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("network-123")

	inv, err := NewInvite(id, DefaultInviteOptions())
	if err != nil {
		t.Fatalf("NewInvite failed: %v", err)
	}

	// Add as pending
	store.AddPending(inv)
	if store.PendingCount() != 1 {
		t.Errorf("pending count should be 1, got %d", store.PendingCount())
	}

	// Get pending
	pending, ok := store.GetPending("network-123")
	if !ok {
		t.Fatal("GetPending should find invite")
	}
	if pending.NetworkID != "network-123" {
		t.Errorf("network ID mismatch: got %q", pending.NetworkID)
	}

	// List pending
	pendingList := store.ListPending()
	if len(pendingList) != 1 {
		t.Errorf("ListPending should return 1, got %d", len(pendingList))
	}

	// Mark as accepted
	if !store.MarkAccepted("network-123") {
		t.Error("MarkAccepted should return true")
	}

	if store.PendingCount() != 0 {
		t.Errorf("pending count should be 0 after accept, got %d", store.PendingCount())
	}
	if store.AcceptedCount() != 1 {
		t.Errorf("accepted count should be 1, got %d", store.AcceptedCount())
	}

	// Get accepted
	accepted, ok := store.GetAccepted("network-123")
	if !ok {
		t.Fatal("GetAccepted should find invite")
	}
	if accepted.NetworkID != "network-123" {
		t.Errorf("network ID mismatch: got %q", accepted.NetworkID)
	}
}

func TestInviteStore_MarkAccepted_NotFound(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	if store.MarkAccepted("nonexistent") {
		t.Error("MarkAccepted should return false for nonexistent invite")
	}
}

func TestInviteStore_RemoveGenerated(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	id, _ := NewIdentity()
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("test")

	inv, _ := NewInvite(id, DefaultInviteOptions())
	key := store.AddGenerated(inv)

	if store.GeneratedCount() != 1 {
		t.Errorf("generated count should be 1, got %d", store.GeneratedCount())
	}

	store.RemoveGenerated(key)

	if store.GeneratedCount() != 0 {
		t.Errorf("generated count should be 0 after remove, got %d", store.GeneratedCount())
	}

	_, ok := store.GetGenerated(key)
	if ok {
		t.Error("GetGenerated should not find removed invite")
	}
}

func TestInviteStore_RemovePending(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	id, _ := NewIdentity()
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("network-abc")

	inv, _ := NewInvite(id, DefaultInviteOptions())
	store.AddPending(inv)

	if store.PendingCount() != 1 {
		t.Errorf("pending count should be 1, got %d", store.PendingCount())
	}

	store.RemovePending("network-abc")

	if store.PendingCount() != 0 {
		t.Errorf("pending count should be 0 after remove, got %d", store.PendingCount())
	}
}

func TestInviteStore_CleanExpired(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	id, _ := NewIdentity()
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("test")

	// Create an invite with very short expiry
	opts := InviteOptions{
		Expiry:  1 * time.Millisecond,
		MaxUses: 1,
	}
	inv, _ := NewInvite(id, opts)
	store.AddGenerated(inv)

	// Wait for expiry
	time.Sleep(5 * time.Millisecond)

	if store.GeneratedCount() != 1 {
		t.Errorf("generated count should be 1 before clean, got %d", store.GeneratedCount())
	}

	removed := store.CleanExpired()
	if removed != 1 {
		t.Errorf("CleanExpired should remove 1 invite, removed %d", removed)
	}

	if store.GeneratedCount() != 0 {
		t.Errorf("generated count should be 0 after clean, got %d", store.GeneratedCount())
	}
}

func TestInviteStore_ListGenerated(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	id, _ := NewIdentity()
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("test")

	inv1, _ := NewInvite(id, DefaultInviteOptions())
	inv2, _ := NewInvite(id, DefaultInviteOptions())

	store.AddGenerated(inv1)
	store.AddGenerated(inv2)

	list := store.ListGenerated()
	if len(list) != 2 {
		t.Errorf("ListGenerated should return 2, got %d", len(list))
	}
}

func TestInviteStore_AddAccepted(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	id, _ := NewIdentity()
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("direct-accept")

	inv, _ := NewInvite(id, DefaultInviteOptions())

	store.AddAccepted(inv)

	if store.AcceptedCount() != 1 {
		t.Errorf("accepted count should be 1, got %d", store.AcceptedCount())
	}

	accepted, ok := store.GetAccepted("direct-accept")
	if !ok {
		t.Error("GetAccepted should find directly added invite")
	}
	if accepted.NetworkID != "direct-accept" {
		t.Errorf("network ID mismatch: got %q", accepted.NetworkID)
	}
}

func TestInviteStore_ListAccepted(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	id, _ := NewIdentity()
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("network1")

	inv1, _ := NewInvite(id, DefaultInviteOptions())
	store.AddAccepted(inv1)

	id.SetNetworkID("network2")
	inv2, _ := NewInvite(id, DefaultInviteOptions())
	store.AddAccepted(inv2)

	list := store.ListAccepted()
	if len(list) != 2 {
		t.Errorf("ListAccepted should return 2, got %d", len(list))
	}
}

func TestInviteStore_UpdateGenerated(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("test-network")

	// Create invite with MaxUses = 3
	opts := InviteOptions{
		Expiry:  24 * time.Hour,
		MaxUses: 3,
	}
	inv, err := NewInvite(id, opts)
	if err != nil {
		t.Fatalf("NewInvite failed: %v", err)
	}

	key := store.AddGenerated(inv)

	// Verify initial state
	stored, ok := store.GetGenerated(key)
	if !ok {
		t.Fatal("GetGenerated should find invite")
	}
	if stored.UsedCount != 0 {
		t.Errorf("UsedCount should be 0, got %d", stored.UsedCount)
	}

	// Simulate usage - get the invite, mark it as used, update
	stored.Use()
	if stored.UsedCount != 1 {
		t.Errorf("UsedCount after Use() should be 1, got %d", stored.UsedCount)
	}

	// Update the invite in the store
	store.UpdateGenerated(key, stored)

	// Retrieve again and verify the update persisted
	updated, ok := store.GetGenerated(key)
	if !ok {
		t.Fatal("GetGenerated should still find invite after update")
	}
	if updated.UsedCount != 1 {
		t.Errorf("UsedCount after update should be 1, got %d", updated.UsedCount)
	}
	if updated.RemainingUses() != 2 {
		t.Errorf("RemainingUses should be 2, got %d", updated.RemainingUses())
	}

	// Use again
	updated.Use()
	store.UpdateGenerated(key, updated)

	final, _ := store.GetGenerated(key)
	if final.UsedCount != 2 {
		t.Errorf("UsedCount should be 2, got %d", final.UsedCount)
	}
	if final.RemainingUses() != 1 {
		t.Errorf("RemainingUses should be 1, got %d", final.RemainingUses())
	}
}

func TestInviteStore_UpdateGenerated_NonExistent(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	id, _ := NewIdentity()
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("test-network")

	inv, _ := NewInvite(id, DefaultInviteOptions())

	// Try to update a key that doesn't exist
	store.UpdateGenerated("nonexistent-key", inv)

	// Should not have added the invite
	if store.GeneratedCount() != 0 {
		t.Error("UpdateGenerated should not add new invites")
	}
}

func TestInviteStore_UseGenerated_Atomic(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	id, _ := NewIdentity()
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("test-network")

	// Create invite with 3 max uses
	opts := DefaultInviteOptions()
	opts.MaxUses = 3
	inv, _ := NewInvite(id, opts)

	key := store.AddGenerated(inv)

	// First use
	remaining, found := store.UseGenerated(key)
	if !found {
		t.Error("UseGenerated should find the invite")
	}
	if remaining != 2 {
		t.Errorf("remaining should be 2, got %d", remaining)
	}

	// Second use
	remaining, found = store.UseGenerated(key)
	if !found {
		t.Error("UseGenerated should still find the invite")
	}
	if remaining != 1 {
		t.Errorf("remaining should be 1, got %d", remaining)
	}

	// Third use (exhausts invite)
	remaining, found = store.UseGenerated(key)
	if !found {
		t.Error("UseGenerated should still find the invite")
	}
	if remaining != 0 {
		t.Errorf("remaining should be 0, got %d", remaining)
	}

	// Invite should be removed now
	if store.GeneratedCount() != 0 {
		t.Error("exhausted invite should be removed")
	}

	// Fourth use should fail
	remaining, found = store.UseGenerated(key)
	if found {
		t.Error("UseGenerated should not find exhausted invite")
	}
	if remaining != -1 {
		t.Errorf("remaining should be -1 for not found, got %d", remaining)
	}
}

func TestInviteStore_UseGenerated_NotFound(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	remaining, found := store.UseGenerated("nonexistent")
	if found {
		t.Error("UseGenerated should return false for nonexistent key")
	}
	if remaining != -1 {
		t.Errorf("remaining should be -1, got %d", remaining)
	}
}

func TestInviteStore_UseGenerated_Concurrent(t *testing.T) {
	store := NewInviteStore("/tmp/test.json")

	id, _ := NewIdentity()
	id.SetI2PDest("test.b32.i2p")
	id.SetNetworkID("test-network")

	// Create single-use invite
	opts := DefaultInviteOptions()
	opts.MaxUses = 1
	inv, _ := NewInvite(id, opts)

	key := store.AddGenerated(inv)

	// Try to use from multiple goroutines simultaneously
	const goroutines = 10
	results := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			remaining, found := store.UseGenerated(key)
			// Only one goroutine should get remaining=0 (successful use)
			results <- found && remaining == 0
		}()
	}

	successCount := 0
	for i := 0; i < goroutines; i++ {
		if <-results {
			successCount++
		}
	}

	// Exactly one goroutine should have successfully used the invite
	if successCount != 1 {
		t.Errorf("expected exactly 1 successful use, got %d", successCount)
	}
}
