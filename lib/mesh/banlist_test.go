package mesh

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBanListBanUnban(t *testing.T) {
	bl := NewBanList(BanListConfig{})

	// Initially not banned
	if bl.IsBanned("node1") {
		t.Error("node1 should not be banned initially")
	}

	// Ban node1
	bl.Ban("node1", BanReasonManual, "test ban", 0)

	if !bl.IsBanned("node1") {
		t.Error("node1 should be banned")
	}

	// Unban
	if !bl.Unban("node1") {
		t.Error("unban should return true")
	}

	if bl.IsBanned("node1") {
		t.Error("node1 should not be banned after unban")
	}

	// Unban non-existent
	if bl.Unban("node2") {
		t.Error("unban should return false for non-existent node")
	}
}

func TestBanListExpiry(t *testing.T) {
	bl := NewBanList(BanListConfig{})

	// Ban with short expiry
	bl.Ban("node1", BanReasonManual, "short ban", 50*time.Millisecond)

	if !bl.IsBanned("node1") {
		t.Error("node1 should be banned")
	}

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	if bl.IsBanned("node1") {
		t.Error("ban should have expired")
	}
}

func TestBanListStrikes(t *testing.T) {
	bl := NewBanList(BanListConfig{
		StrikeThreshold: 3,
		AutoBanDuration: time.Hour,
	})

	// Record strikes
	for i := 0; i < 2; i++ {
		if bl.RecordStrike("node1", BanReasonMisbehavior, "strike") {
			t.Error("should not be banned yet")
		}
	}

	// Third strike should ban
	if !bl.RecordStrike("node1", BanReasonMisbehavior, "final strike") {
		t.Error("should be banned on third strike")
	}

	if !bl.IsBanned("node1") {
		t.Error("node1 should be banned")
	}
}

func TestBanListByDest(t *testing.T) {
	bl := NewBanList(BanListConfig{})

	i2pDest := "testdestination12345.b32.i2p"

	bl.BanWithDest("node1", i2pDest, BanReasonMisbehavior, "test", 0)

	if !bl.IsBannedByDest(i2pDest) {
		t.Error("dest should be banned")
	}

	if !bl.IsBanned("node1") {
		t.Error("node should be banned")
	}

	// Unban and check dest
	bl.Unban("node1")

	if bl.IsBannedByDest(i2pDest) {
		t.Error("dest should not be banned after unban")
	}
}

func TestBanListList(t *testing.T) {
	bl := NewBanList(BanListConfig{})

	bl.Ban("node1", BanReasonManual, "test1", 0)
	bl.Ban("node2", BanReasonHandshakeFailures, "test2", 0)

	bans := bl.List()
	if len(bans) != 2 {
		t.Errorf("expected 2 bans, got %d", len(bans))
	}
}

func TestBanListCleanup(t *testing.T) {
	bl := NewBanList(BanListConfig{})

	// Add expired ban
	bl.Ban("node1", BanReasonManual, "short", 10*time.Millisecond)
	bl.Ban("node2", BanReasonManual, "permanent", 0)

	time.Sleep(50 * time.Millisecond)

	removed := bl.CleanupExpired()
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	bans := bl.List()
	if len(bans) != 1 {
		t.Errorf("expected 1 remaining ban, got %d", len(bans))
	}

	if bans[0].NodeID != "node2" {
		t.Error("wrong ban remaining")
	}
}

func TestBanListPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bans.json")

	// Create and populate ban list
	bl1 := NewBanList(BanListConfig{PersistPath: path})
	bl1.Ban("node1", BanReasonManual, "test ban", 0)
	bl1.RecordStrike("node2", BanReasonMisbehavior, "strike 1")

	// Create new ban list from same file
	bl2 := NewBanList(BanListConfig{PersistPath: path})

	if !bl2.IsBanned("node1") {
		t.Error("ban should persist")
	}

	// Strikes should also persist
	bl2.RecordStrike("node2", BanReasonMisbehavior, "strike 2")
	// Need to check internal strike count
}

func TestBanListNoPersist(t *testing.T) {
	// Ensure no error when no persist path
	bl := NewBanList(BanListConfig{})
	bl.Ban("node1", BanReasonManual, "test", 0)
	bl.Unban("node1")
}

func TestBanListLoadNonExistent(t *testing.T) {
	// Should not error when file doesn't exist
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nonexistent.json")

	bl := NewBanList(BanListConfig{PersistPath: path})
	if bl.IsBanned("node1") {
		t.Error("should not be banned")
	}
}

func TestBanListLoadCorrupt(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "corrupt.json")

	// Write corrupt data
	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}

	// Should handle corrupt file gracefully
	bl := NewBanList(BanListConfig{PersistPath: path})
	if bl.IsBanned("node1") {
		t.Error("should not be banned")
	}
}
