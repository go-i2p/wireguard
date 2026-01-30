package mesh

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PersistentState represents the state that survives restarts.
type PersistentState struct {
	// Peers is the list of known peers
	Peers []PersistedPeer `json:"peers"`
	// Routes is the routing table
	Routes []PersistedRoute `json:"routes"`
	// LastSaved is when the state was last saved
	LastSaved time.Time `json:"last_saved"`
	// Version is the state format version
	Version int `json:"version"`
}

// PersistedPeer represents a peer for persistence.
type PersistedPeer struct {
	NodeID      string    `json:"node_id"`
	I2PDest     string    `json:"i2p_dest"`
	WGPublicKey string    `json:"wg_public_key"`
	TunnelIP    string    `json:"tunnel_ip"`
	LastSeen    time.Time `json:"last_seen"`
}

// PersistedRoute represents a route for persistence.
type PersistedRoute struct {
	Destination string    `json:"destination"`
	NextHop     string    `json:"next_hop"`
	Metric      int       `json:"metric"`
	LastUpdated time.Time `json:"last_updated"`
}

const (
	// StateVersion is the current state format version
	StateVersion = 1
	// StateFileName is the default state file name
	StateFileName = "state.json"
)

// StateManager handles persistent state storage.
type StateManager struct {
	mu       sync.RWMutex
	path     string
	state    *PersistentState
	dirty    bool
	interval time.Duration

	// Dependencies for gathering state
	peerManager  *PeerManager
	routingTable *RoutingTable

	// Control
	stopCh  chan struct{}
	doneCh  chan struct{}
	started bool
	stopped bool
}

// StateManagerConfig configures the state manager.
type StateManagerConfig struct {
	// Path is the file path for state storage
	Path string
	// SaveInterval is how often to auto-save state
	SaveInterval time.Duration
	// PeerManager for accessing peer data
	PeerManager *PeerManager
	// RoutingTable for accessing route data
	RoutingTable *RoutingTable
}

// NewStateManager creates a new state manager.
func NewStateManager(cfg StateManagerConfig) *StateManager {
	interval := cfg.SaveInterval
	if interval == 0 {
		interval = 5 * time.Minute
	}

	return &StateManager{
		path:         cfg.Path,
		state:        &PersistentState{Version: StateVersion},
		interval:     interval,
		peerManager:  cfg.PeerManager,
		routingTable: cfg.RoutingTable,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
}

// Load loads state from disk.
func (sm *StateManager) Load() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	data, err := os.ReadFile(sm.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No state to load
		}
		return fmt.Errorf("reading state file: %w", err)
	}

	var state PersistentState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("parsing state file: %w", err)
	}

	sm.state = &state
	return nil
}

// Save saves state to disk.
// This method gathers state from dependencies without holding the StateManager lock,
// then acquires the lock only for updating internal state and writing to disk.
// This design prevents potential deadlocks from nested lock acquisition.
func (sm *StateManager) Save() error {
	// Gather state from dependencies BEFORE acquiring our lock.
	// This eliminates nested lock acquisition (holding sm.mu while calling
	// dependency methods that acquire their own locks), preventing any
	// potential for deadlock if dependencies ever need to call back to us.
	peers, routes := sm.gatherState()

	sm.mu.Lock()
	defer sm.mu.Unlock()

	return sm.saveLocked(peers, routes)
}

// gatherState collects state from dependencies without holding sm.mu.
// This is called before acquiring the StateManager lock to prevent nested
// lock acquisition patterns that could lead to deadlock.
// Returns nil slices if no dependencies are configured (preserves existing state).
func (sm *StateManager) gatherState() ([]PersistedPeer, []PersistedRoute) {
	var persistedPeers []PersistedPeer
	var persistedRoutes []PersistedRoute

	// Gather peers - PeerManager.ListPeers() acquires its own RLock
	if sm.peerManager != nil {
		peers := sm.peerManager.ListPeers()
		persistedPeers = make([]PersistedPeer, 0, len(peers))
		for _, p := range peers {
			if p.State == PeerStateConnected {
				persistedPeers = append(persistedPeers, PersistedPeer{
					NodeID:      p.NodeID,
					I2PDest:     p.I2PDest,
					WGPublicKey: p.WGPublicKey.String(),
					TunnelIP:    p.TunnelIP.String(),
					LastSeen:    p.LastSeen,
				})
			}
		}
	}

	// Gather routes - RoutingTable.ListRoutes() acquires its own RLock
	if sm.routingTable != nil {
		routes := sm.routingTable.ListRoutes()
		persistedRoutes = make([]PersistedRoute, 0, len(routes))
		for _, r := range routes {
			persistedRoutes = append(persistedRoutes, PersistedRoute{
				Destination: r.TunnelIP.String(),
				NextHop:     r.ViaNodeID,
				Metric:      r.HopCount,
				LastUpdated: r.LastSeen,
			})
		}
	}

	return persistedPeers, persistedRoutes
}

// saveLocked saves state to disk. Must be called with lock held.
// The peers and routes are pre-gathered to avoid nested lock acquisition.
// If peers or routes are nil, the existing state is preserved (for testing
// or when dependencies are not configured).
func (sm *StateManager) saveLocked(peers []PersistedPeer, routes []PersistedRoute) error {
	// Only update state if dependencies provided data
	if peers != nil {
		sm.state.Peers = peers
	}
	if routes != nil {
		sm.state.Routes = routes
	}
	sm.state.LastSaved = time.Now()
	sm.state.Version = StateVersion

	data, err := json.MarshalIndent(sm.state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(sm.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	// Write atomically via temp file
	tmpPath := sm.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("writing state file: %w", err)
	}

	if err := os.Rename(tmpPath, sm.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming state file: %w", err)
	}

	sm.dirty = false
	return nil
}

// Start begins automatic state saving.
// It is safe to call Start multiple times; only the first call has effect.
func (sm *StateManager) Start() {
	sm.mu.Lock()
	if sm.started {
		sm.mu.Unlock()
		return
	}
	sm.started = true
	sm.mu.Unlock()

	go func() {
		defer close(sm.doneCh)
		ticker := time.NewTicker(sm.interval)
		defer ticker.Stop()

		for {
			select {
			case <-sm.stopCh:
				// Final save before exit - error is logged at caller level
				_ = sm.Save()
				return
			case <-ticker.C:
				// Periodic save - errors are transient, will retry next interval
				_ = sm.Save()
			}
		}
	}()
}

// Stop halts automatic state saving and does a final save.
// It is safe to call Stop multiple times or before Start; extra calls are no-ops.
func (sm *StateManager) Stop() {
	sm.mu.Lock()
	if sm.stopped || !sm.started {
		sm.mu.Unlock()
		return
	}
	sm.stopped = true
	sm.mu.Unlock()

	close(sm.stopCh)
	<-sm.doneCh
}

// GetPeers returns the persisted peers.
func (sm *StateManager) GetPeers() []PersistedPeer {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state.Peers
}

// GetRoutes returns the persisted routes.
func (sm *StateManager) GetRoutes() []PersistedRoute {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state.Routes
}

// MarkDirty marks the state as needing to be saved.
func (sm *StateManager) MarkDirty() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.dirty = true
}

// IsDirty returns whether the state needs saving.
func (sm *StateManager) IsDirty() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.dirty
}
