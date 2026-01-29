package mesh

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"time"
)

// BanReason indicates why a peer was banned.
type BanReason string

const (
	// BanReasonManual is for manual bans by the user.
	BanReasonManual BanReason = "manual"
	// BanReasonHandshakeFailures is for too many failed handshakes.
	BanReasonHandshakeFailures BanReason = "handshake_failures"
	// BanReasonInvalidMessages is for sending invalid messages.
	BanReasonInvalidMessages BanReason = "invalid_messages"
	// BanReasonMisbehavior is for general protocol violations.
	BanReasonMisbehavior BanReason = "misbehavior"
)

// BanEntry represents a banned peer.
type BanEntry struct {
	// NodeID is the banned node's identifier.
	NodeID string `json:"node_id"`
	// I2PDest is the banned node's I2P destination (optional).
	I2PDest string `json:"i2p_dest,omitempty"`
	// Reason is why the peer was banned.
	Reason BanReason `json:"reason"`
	// Description provides additional context.
	Description string `json:"description,omitempty"`
	// BannedAt is when the ban was created.
	BannedAt time.Time `json:"banned_at"`
	// ExpiresAt is when the ban expires (zero means permanent).
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	// StrikeCount tracks how many violations led to this ban.
	StrikeCount int `json:"strike_count"`
}

// IsExpired returns true if the ban has expired.
func (b *BanEntry) IsExpired() bool {
	if b.ExpiresAt.IsZero() {
		return false // Permanent ban
	}
	return time.Now().After(b.ExpiresAt)
}

// BanListConfig configures the BanList.
type BanListConfig struct {
	// PersistPath is where to save the ban list (empty = no persistence).
	PersistPath string
	// StrikeThreshold is how many strikes before auto-ban.
	StrikeThreshold int
	// AutoBanDuration is how long auto-bans last.
	AutoBanDuration time.Duration
	// Logger for logging ban events.
	Logger *slog.Logger
}

// BanList manages banned peers.
type BanList struct {
	mu     sync.RWMutex
	bans   map[string]*BanEntry // keyed by NodeID
	dests  map[string]string    // I2P dest -> NodeID mapping
	logger *slog.Logger

	// Strike tracking for auto-banning
	strikes map[string]int // NodeID -> strike count

	// Configuration
	persistPath     string
	strikeThreshold int
	autoBanDuration time.Duration
}

// NewBanList creates a new BanList.
func NewBanList(cfg BanListConfig) *BanList {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	strikeThreshold := cfg.StrikeThreshold
	if strikeThreshold <= 0 {
		strikeThreshold = 5 // Default: 5 strikes
	}

	autoBanDuration := cfg.AutoBanDuration
	if autoBanDuration <= 0 {
		autoBanDuration = 24 * time.Hour // Default: 24h
	}

	bl := &BanList{
		bans:            make(map[string]*BanEntry),
		dests:           make(map[string]string),
		strikes:         make(map[string]int),
		logger:          logger,
		persistPath:     cfg.PersistPath,
		strikeThreshold: strikeThreshold,
		autoBanDuration: autoBanDuration,
	}

	// Load persisted bans
	if cfg.PersistPath != "" {
		if err := bl.load(); err != nil && !os.IsNotExist(err) {
			logger.Warn("failed to load ban list", "error", err)
		}
	}

	return bl
}

// Ban adds a peer to the ban list.
func (bl *BanList) Ban(nodeID string, reason BanReason, description string, duration time.Duration) {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	entry := &BanEntry{
		NodeID:      nodeID,
		Reason:      reason,
		Description: description,
		BannedAt:    time.Now(),
		StrikeCount: bl.strikes[nodeID],
	}

	if duration > 0 {
		entry.ExpiresAt = time.Now().Add(duration)
	}

	bl.bans[nodeID] = entry
	bl.logger.Info("peer banned",
		"node_id", nodeID,
		"reason", reason,
		"duration", duration)

	bl.saveLocked()
}

// BanWithDest bans a peer and records their I2P destination.
func (bl *BanList) BanWithDest(nodeID, i2pDest string, reason BanReason, description string, duration time.Duration) {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	entry := &BanEntry{
		NodeID:      nodeID,
		I2PDest:     i2pDest,
		Reason:      reason,
		Description: description,
		BannedAt:    time.Now(),
		StrikeCount: bl.strikes[nodeID],
	}

	if duration > 0 {
		entry.ExpiresAt = time.Now().Add(duration)
	}

	bl.bans[nodeID] = entry
	if i2pDest != "" {
		bl.dests[i2pDest] = nodeID
	}

	bl.logger.Info("peer banned",
		"node_id", nodeID,
		"i2p_dest", truncateDest(i2pDest),
		"reason", reason,
		"duration", duration)

	bl.saveLocked()
}

// Unban removes a peer from the ban list.
func (bl *BanList) Unban(nodeID string) bool {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	entry, exists := bl.bans[nodeID]
	if !exists {
		return false
	}

	// Remove from dest mapping if present
	if entry.I2PDest != "" {
		delete(bl.dests, entry.I2PDest)
	}

	delete(bl.bans, nodeID)
	delete(bl.strikes, nodeID) // Clear strikes on unban

	bl.logger.Info("peer unbanned", "node_id", nodeID)
	bl.saveLocked()

	return true
}

// IsBanned checks if a node is banned (by NodeID or I2P dest).
func (bl *BanList) IsBanned(nodeID string) bool {
	bl.mu.RLock()
	defer bl.mu.RUnlock()

	entry, exists := bl.bans[nodeID]
	if !exists {
		return false
	}

	// Check expiration
	if entry.IsExpired() {
		return false // Will be cleaned up by cleanup routine
	}

	return true
}

// IsBannedByDest checks if an I2P destination is banned.
func (bl *BanList) IsBannedByDest(i2pDest string) bool {
	bl.mu.RLock()
	defer bl.mu.RUnlock()

	nodeID, exists := bl.dests[i2pDest]
	if !exists {
		return false
	}

	entry, exists := bl.bans[nodeID]
	if !exists {
		return false
	}

	return !entry.IsExpired()
}

// RecordStrike records a strike against a node.
// Returns true if the node was auto-banned as a result.
func (bl *BanList) RecordStrike(nodeID string, reason BanReason, description string) bool {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	// Already banned?
	if entry, exists := bl.bans[nodeID]; exists && !entry.IsExpired() {
		return true
	}

	bl.strikes[nodeID]++
	strikes := bl.strikes[nodeID]

	bl.logger.Debug("strike recorded",
		"node_id", nodeID,
		"strikes", strikes,
		"threshold", bl.strikeThreshold)

	if strikes >= bl.strikeThreshold {
		// Auto-ban
		entry := &BanEntry{
			NodeID:      nodeID,
			Reason:      reason,
			Description: description,
			BannedAt:    time.Now(),
			ExpiresAt:   time.Now().Add(bl.autoBanDuration),
			StrikeCount: strikes,
		}
		bl.bans[nodeID] = entry

		bl.logger.Warn("peer auto-banned after strikes",
			"node_id", nodeID,
			"strikes", strikes,
			"reason", reason,
			"duration", bl.autoBanDuration)

		bl.saveLocked()
		return true
	}

	return false
}

// List returns all active ban entries.
func (bl *BanList) List() []*BanEntry {
	bl.mu.RLock()
	defer bl.mu.RUnlock()

	result := make([]*BanEntry, 0, len(bl.bans))
	for _, entry := range bl.bans {
		if !entry.IsExpired() {
			result = append(result, entry)
		}
	}
	return result
}

// CleanupExpired removes expired bans.
func (bl *BanList) CleanupExpired() int {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	removed := 0
	for nodeID, entry := range bl.bans {
		if entry.IsExpired() {
			if entry.I2PDest != "" {
				delete(bl.dests, entry.I2PDest)
			}
			delete(bl.bans, nodeID)
			removed++
			bl.logger.Info("expired ban removed", "node_id", nodeID)
		}
	}

	if removed > 0 {
		bl.saveLocked()
	}

	return removed
}

// banListData is the JSON structure for persistence.
type banListData struct {
	Bans    []*BanEntry    `json:"bans"`
	Strikes map[string]int `json:"strikes,omitempty"`
}

// load loads the ban list from disk.
func (bl *BanList) load() error {
	if bl.persistPath == "" {
		return nil
	}

	data, err := os.ReadFile(bl.persistPath)
	if err != nil {
		return err
	}

	var bld banListData
	if err := json.Unmarshal(data, &bld); err != nil {
		return err
	}

	for _, entry := range bld.Bans {
		if !entry.IsExpired() {
			bl.bans[entry.NodeID] = entry
			if entry.I2PDest != "" {
				bl.dests[entry.I2PDest] = entry.NodeID
			}
		}
	}

	if bld.Strikes != nil {
		bl.strikes = bld.Strikes
	}

	bl.logger.Info("loaded ban list", "bans", len(bl.bans))
	return nil
}

// saveLocked saves the ban list to disk. Must be called with lock held.
func (bl *BanList) saveLocked() {
	if bl.persistPath == "" {
		return
	}

	bans := make([]*BanEntry, 0, len(bl.bans))
	for _, entry := range bl.bans {
		bans = append(bans, entry)
	}

	bld := banListData{
		Bans:    bans,
		Strikes: bl.strikes,
	}

	data, err := json.MarshalIndent(bld, "", "  ")
	if err != nil {
		bl.logger.Error("failed to marshal ban list", "error", err)
		return
	}

	if err := os.WriteFile(bl.persistPath, data, 0o600); err != nil {
		bl.logger.Error("failed to save ban list", "error", err)
	}
}
