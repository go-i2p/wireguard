package core

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/go-i2p/wireguard/lib/mesh"
)

// ExitNodeInfo represents a discovered exit node with its advertisement
// and last seen timestamp for staleness checking.
type ExitNodeInfo struct {
	Advertisement *mesh.ExitNodeAdvertisement
	PeerInfo      *mesh.PeerAnnounce
	LastSeen      time.Time
	Latency       time.Duration
}

// ExitDiscovery manages discovery and selection of exit nodes in the mesh network.
// It monitors peer announcements and maintains a list of available exit nodes.
type ExitDiscovery struct {
	mu         sync.RWMutex
	exitNodes  map[string]*ExitNodeInfo // keyed by NodeID
	staleAfter time.Duration            // how long before entry considered stale
}

// NewExitDiscovery creates a new exit discovery manager.
// staleAfter determines how long an exit node advertisement remains valid.
func NewExitDiscovery(staleAfter time.Duration) *ExitDiscovery {
	if staleAfter == 0 {
		staleAfter = 5 * time.Minute // default: 5 minutes
	}

	return &ExitDiscovery{
		exitNodes:  make(map[string]*ExitNodeInfo),
		staleAfter: staleAfter,
	}
}

// UpdateExitNode updates the discovery cache with an exit node advertisement.
// This should be called when receiving PeerAnnounce messages containing exit node info.
func (d *ExitDiscovery) UpdateExitNode(peerAnnounce *mesh.PeerAnnounce) {
	if peerAnnounce == nil || peerAnnounce.ExitNode == nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	nodeID := peerAnnounce.NodeID
	now := time.Now()

	// Update or create entry
	if info, exists := d.exitNodes[nodeID]; exists {
		info.Advertisement = peerAnnounce.ExitNode
		info.PeerInfo = peerAnnounce
		info.LastSeen = now
	} else {
		d.exitNodes[nodeID] = &ExitNodeInfo{
			Advertisement: peerAnnounce.ExitNode,
			PeerInfo:      peerAnnounce,
			LastSeen:      now,
		}
	}
}

// RemoveExitNode removes an exit node from the discovery cache.
// Useful when a peer leaves the network or stops being an exit node.
func (d *ExitDiscovery) RemoveExitNode(nodeID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.exitNodes, nodeID)
}

// CleanupStale removes exit nodes that haven't been seen recently.
// Returns the number of stale entries removed.
func (d *ExitDiscovery) CleanupStale() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	removed := 0

	for nodeID, info := range d.exitNodes {
		if now.Sub(info.LastSeen) > d.staleAfter {
			delete(d.exitNodes, nodeID)
			removed++
		}
	}

	return removed
}

// DiscoverExitNodes returns a list of available exit nodes, optionally filtered.
// The list is sorted by a combination of load and latency (best first).
func (d *ExitDiscovery) DiscoverExitNodes(filter *ExitNodeFilter) ([]*ExitNodeInfo, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.exitNodes) == 0 {
		return nil, fmt.Errorf("no exit nodes discovered")
	}

	// Collect and filter exit nodes
	var candidates []*ExitNodeInfo
	for _, info := range d.exitNodes {
		if filter != nil && !filter.Matches(info) {
			continue
		}
		candidates = append(candidates, info)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no exit nodes match filter criteria")
	}

	// Sort by score (lower is better)
	sort.Slice(candidates, func(i, j int) bool {
		return calculateScore(candidates[i]) < calculateScore(candidates[j])
	})

	return candidates, nil
}

// GetExitNode retrieves information about a specific exit node by ID.
func (d *ExitDiscovery) GetExitNode(nodeID string) (*ExitNodeInfo, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	info, exists := d.exitNodes[nodeID]
	if !exists {
		return nil, fmt.Errorf("exit node %s not found", nodeID)
	}

	// Check staleness
	if time.Since(info.LastSeen) > d.staleAfter {
		return nil, fmt.Errorf("exit node %s information is stale", nodeID)
	}

	return info, nil
}

// ListExitNodes returns all currently known exit nodes (including stale ones).
func (d *ExitDiscovery) ListExitNodes() []*ExitNodeInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()

	nodes := make([]*ExitNodeInfo, 0, len(d.exitNodes))
	for _, info := range d.exitNodes {
		nodes = append(nodes, info)
	}

	return nodes
}

// ExitNodeFilter defines criteria for filtering exit nodes.
type ExitNodeFilter struct {
	// RequireCapabilities filters nodes that must have all listed capabilities
	RequireCapabilities []string
	// MaxLoad filters nodes with load below this threshold (0.0-1.0)
	MaxLoad float64
	// MinBandwidth filters nodes with at least this bandwidth (Mbps, 0 = unlimited OK)
	MinBandwidth int
	// AllowedNodeIDs filters to only these specific node IDs (empty = all)
	AllowedNodeIDs []string
	// ExcludeNodeIDs filters out these specific node IDs
	ExcludeNodeIDs []string
	// RequireLocation filters nodes in this location (empty = any)
	RequireLocation string
	// CheckClientAllowed is the client node ID to check against AllowedClients whitelist
	CheckClientAllowed string
}

// Matches returns true if the exit node info matches all filter criteria.
func (f *ExitNodeFilter) Matches(info *ExitNodeInfo) bool {
	if info == nil || info.Advertisement == nil {
		return false
	}

	ad := info.Advertisement

	// Check required capabilities
	if len(f.RequireCapabilities) > 0 {
		capMap := make(map[string]bool)
		for _, cap := range ad.Capabilities {
			capMap[cap] = true
		}
		for _, required := range f.RequireCapabilities {
			if !capMap[required] {
				return false
			}
		}
	}

	// Check max load
	if f.MaxLoad > 0 && ad.CurrentLoad > f.MaxLoad {
		return false
	}

	// Check min bandwidth
	if f.MinBandwidth > 0 {
		// 0 means unlimited, which is acceptable
		if ad.BandwidthLimitMbps > 0 && ad.BandwidthLimitMbps < f.MinBandwidth {
			return false
		}
	}

	// Check allowed node IDs
	if len(f.AllowedNodeIDs) > 0 {
		allowed := false
		for _, nodeID := range f.AllowedNodeIDs {
			if ad.NodeID == nodeID {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	// Check excluded node IDs
	for _, excludedID := range f.ExcludeNodeIDs {
		if ad.NodeID == excludedID {
			return false
		}
	}

	// Check location requirement
	if f.RequireLocation != "" && ad.Location != f.RequireLocation {
		return false
	}

	// Check client whitelist
	if f.CheckClientAllowed != "" && len(ad.AllowedClients) > 0 {
		allowed := false
		for _, clientID := range ad.AllowedClients {
			if clientID == f.CheckClientAllowed {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	return true
}

// calculateScore computes a score for an exit node (lower is better).
// Considers load, active clients, and latency.
func calculateScore(info *ExitNodeInfo) float64 {
	// Base score from load (0.0-1.0)
	score := info.Advertisement.CurrentLoad

	// Penalty for active clients (small factor)
	clientPenalty := float64(info.Advertisement.ActiveClients) * 0.01
	score += clientPenalty

	// Penalty for latency if available (milliseconds converted to score)
	if info.Latency > 0 {
		latencyPenalty := float64(info.Latency.Milliseconds()) * 0.001
		score += latencyPenalty
	}

	return score
}

// SelectBestExitNode selects the best available exit node based on the filter criteria.
// Returns nil if no suitable exit node is found.
func (d *ExitDiscovery) SelectBestExitNode(filter *ExitNodeFilter) (*ExitNodeInfo, error) {
	candidates, err := d.DiscoverExitNodes(filter)
	if err != nil {
		return nil, err
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no suitable exit nodes found")
	}

	// Return the first candidate (best score)
	return candidates[0], nil
}
