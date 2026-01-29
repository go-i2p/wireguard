package identity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// InviteStore manages pending and accepted invites.
// It persists invites to disk and tracks their usage.
type InviteStore struct {
	mu   sync.RWMutex
	path string

	// Generated invites (ones we created for others to use)
	Generated map[string]*Invite `json:"generated"`

	// Pending invites (ones we received but haven't acted on)
	Pending map[string]*Invite `json:"pending"`

	// Accepted invites (ones we've successfully used to join)
	Accepted map[string]*Invite `json:"accepted"`
}

// NewInviteStore creates a new invite store.
func NewInviteStore(path string) *InviteStore {
	return &InviteStore{
		path:      path,
		Generated: make(map[string]*Invite),
		Pending:   make(map[string]*Invite),
		Accepted:  make(map[string]*Invite),
	}
}

// LoadInviteStore loads an invite store from disk.
// Returns an empty store if the file doesn't exist.
func LoadInviteStore(path string) (*InviteStore, error) {
	store := NewInviteStore(path)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, fmt.Errorf("reading invite store: %w", err)
	}

	if err := json.Unmarshal(data, store); err != nil {
		return nil, fmt.Errorf("parsing invite store: %w", err)
	}

	return store, nil
}

// Save persists the invite store to disk.
func (s *InviteStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling invite store: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating invite store directory: %w", err)
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("writing invite store: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming invite store: %w", err)
	}

	return nil
}

// AddGenerated adds a generated invite to the store.
// Uses the auth token (hex-encoded) as the key.
func (s *InviteStore) AddGenerated(inv *Invite) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%x", inv.AuthToken)
	s.Generated[key] = inv
	return key
}

// GetGenerated returns a generated invite by key.
func (s *InviteStore) GetGenerated(key string) (*Invite, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	inv, ok := s.Generated[key]
	if !ok {
		return nil, false
	}
	// Return a copy
	copy := *inv
	return &copy, true
}

// RemoveGenerated removes a generated invite.
func (s *InviteStore) RemoveGenerated(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Generated, key)
}

// AddPending adds a pending invite (one we received).
// Uses the network ID as the key.
func (s *InviteStore) AddPending(inv *Invite) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Pending[inv.NetworkID] = inv
}

// GetPending returns a pending invite by network ID.
func (s *InviteStore) GetPending(networkID string) (*Invite, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	inv, ok := s.Pending[networkID]
	if !ok {
		return nil, false
	}
	copy := *inv
	return &copy, true
}

// RemovePending removes a pending invite.
func (s *InviteStore) RemovePending(networkID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Pending, networkID)
}

// MarkAccepted moves an invite from pending to accepted.
func (s *InviteStore) MarkAccepted(networkID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	inv, ok := s.Pending[networkID]
	if !ok {
		return false
	}

	delete(s.Pending, networkID)
	s.Accepted[networkID] = inv
	return true
}

// AddAccepted directly adds an accepted invite.
func (s *InviteStore) AddAccepted(inv *Invite) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Accepted[inv.NetworkID] = inv
}

// GetAccepted returns an accepted invite by network ID.
func (s *InviteStore) GetAccepted(networkID string) (*Invite, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	inv, ok := s.Accepted[networkID]
	if !ok {
		return nil, false
	}
	copy := *inv
	return &copy, true
}

// ListGenerated returns all generated invites.
func (s *InviteStore) ListGenerated() []*Invite {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Invite, 0, len(s.Generated))
	for _, inv := range s.Generated {
		copy := *inv
		result = append(result, &copy)
	}
	return result
}

// ListPending returns all pending invites.
func (s *InviteStore) ListPending() []*Invite {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Invite, 0, len(s.Pending))
	for _, inv := range s.Pending {
		copy := *inv
		result = append(result, &copy)
	}
	return result
}

// ListAccepted returns all accepted invites.
func (s *InviteStore) ListAccepted() []*Invite {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Invite, 0, len(s.Accepted))
	for _, inv := range s.Accepted {
		copy := *inv
		result = append(result, &copy)
	}
	return result
}

// CleanExpired removes all expired invites from the store.
// Returns the number of invites removed.
func (s *InviteStore) CleanExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0

	for key, inv := range s.Generated {
		if inv.IsExpired() {
			delete(s.Generated, key)
			count++
		}
	}

	for key, inv := range s.Pending {
		if inv.IsExpired() {
			delete(s.Pending, key)
			count++
		}
	}

	// Don't clean accepted invites - they're historical records

	return count
}

// GeneratedCount returns the number of generated invites.
func (s *InviteStore) GeneratedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.Generated)
}

// PendingCount returns the number of pending invites.
func (s *InviteStore) PendingCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.Pending)
}

// AcceptedCount returns the number of accepted invites.
func (s *InviteStore) AcceptedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.Accepted)
}
