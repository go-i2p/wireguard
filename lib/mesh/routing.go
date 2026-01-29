package mesh

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// RouteEntry represents a routing table entry.
type RouteEntry struct {
	// TunnelIP is the mesh tunnel IP address.
	TunnelIP netip.Addr `json:"tunnel_ip"`
	// WGPublicKey is the WireGuard public key (base64).
	WGPublicKey string `json:"wg_public_key"`
	// I2PDest is the I2P destination address.
	I2PDest string `json:"i2p_dest"`
	// NodeID is the unique node identifier.
	NodeID string `json:"node_id"`
	// LastSeen is when this route was last confirmed.
	LastSeen time.Time `json:"last_seen"`
	// CreatedAt is when this route was first learned.
	CreatedAt time.Time `json:"created_at"`
	// HopCount is the number of hops to reach this node (0 = direct).
	HopCount int `json:"hop_count"`
	// ViaNodeID is the next hop node ID (empty for direct routes).
	ViaNodeID string `json:"via_node_id,omitempty"`
}

// RoutingTable manages the mesh network routing table.
type RoutingTable struct {
	mu       sync.RWMutex
	routes   map[netip.Addr]*RouteEntry // keyed by TunnelIP
	byNodeID map[string]*RouteEntry     // secondary index by NodeID
	subnet   netip.Prefix
	filePath string
}

// RoutingTableConfig configures the routing table.
type RoutingTableConfig struct {
	// Subnet is the mesh network subnet.
	Subnet netip.Prefix
	// FilePath is the path to persist routes (optional).
	FilePath string
}

// NewRoutingTable creates a new routing table.
func NewRoutingTable(cfg RoutingTableConfig) *RoutingTable {
	subnet := cfg.Subnet
	if !subnet.IsValid() {
		subnet = TunnelSubnet
	}

	return &RoutingTable{
		routes:   make(map[netip.Addr]*RouteEntry),
		byNodeID: make(map[string]*RouteEntry),
		subnet:   subnet,
		filePath: cfg.FilePath,
	}
}

// AddRoute adds or updates a route in the table.
func (rt *RoutingTable) AddRoute(entry *RouteEntry) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if !entry.TunnelIP.IsValid() {
		return errors.New("invalid tunnel IP")
	}
	if !rt.subnet.Contains(entry.TunnelIP) {
		return fmt.Errorf("IP %s not in subnet %s", entry.TunnelIP, rt.subnet)
	}

	// Check for collision - same IP, different node
	if existing, ok := rt.routes[entry.TunnelIP]; ok {
		if existing.NodeID != entry.NodeID {
			return &IPCollisionError{
				IP:           entry.TunnelIP,
				ExistingNode: existing.NodeID,
				NewNode:      entry.NodeID,
			}
		}
	}

	// Update or create
	now := time.Now()
	if existing, ok := rt.routes[entry.TunnelIP]; ok {
		// Update existing entry
		existing.LastSeen = now
		existing.I2PDest = entry.I2PDest
		existing.HopCount = entry.HopCount
		existing.ViaNodeID = entry.ViaNodeID
	} else {
		// New entry
		entry.CreatedAt = now
		entry.LastSeen = now
		rt.routes[entry.TunnelIP] = entry
		rt.byNodeID[entry.NodeID] = entry
	}

	return nil
}

// RemoveRoute removes a route by tunnel IP.
func (rt *RoutingTable) RemoveRoute(ip netip.Addr) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	entry, ok := rt.routes[ip]
	if !ok {
		return false
	}

	delete(rt.routes, ip)
	delete(rt.byNodeID, entry.NodeID)
	return true
}

// RemoveByNodeID removes a route by node ID.
func (rt *RoutingTable) RemoveByNodeID(nodeID string) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	entry, ok := rt.byNodeID[nodeID]
	if !ok {
		return false
	}

	delete(rt.routes, entry.TunnelIP)
	delete(rt.byNodeID, nodeID)
	return true
}

// GetRoute returns a route by tunnel IP.
func (rt *RoutingTable) GetRoute(ip netip.Addr) (*RouteEntry, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	entry, ok := rt.routes[ip]
	if !ok {
		return nil, false
	}

	// Return a copy
	entryCopy := *entry
	return &entryCopy, true
}

// GetByNodeID returns a route by node ID.
func (rt *RoutingTable) GetByNodeID(nodeID string) (*RouteEntry, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	entry, ok := rt.byNodeID[nodeID]
	if !ok {
		return nil, false
	}

	entryCopy := *entry
	return &entryCopy, true
}

// ListRoutes returns all routes.
func (rt *RoutingTable) ListRoutes() []*RouteEntry {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	result := make([]*RouteEntry, 0, len(rt.routes))
	for _, entry := range rt.routes {
		entryCopy := *entry
		result = append(result, &entryCopy)
	}
	return result
}

// RouteCount returns the number of routes.
func (rt *RoutingTable) RouteCount() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return len(rt.routes)
}

// HasCollision checks if adding a route for the given key would cause a collision.
func (rt *RoutingTable) HasCollision(publicKey wgtypes.Key, nodeID string) bool {
	ip := AllocateTunnelIPWithSubnet(publicKey, rt.subnet)

	rt.mu.RLock()
	defer rt.mu.RUnlock()

	if existing, ok := rt.routes[ip]; ok {
		return existing.NodeID != nodeID
	}
	return false
}

// CheckAndAllocateIP checks for collision and returns the allocated IP.
func (rt *RoutingTable) CheckAndAllocateIP(publicKey wgtypes.Key, nodeID string) (netip.Addr, error) {
	ip := AllocateTunnelIPWithSubnet(publicKey, rt.subnet)

	rt.mu.RLock()
	defer rt.mu.RUnlock()

	if existing, ok := rt.routes[ip]; ok && existing.NodeID != nodeID {
		return netip.Addr{}, &IPCollisionError{
			IP:           ip,
			ExistingNode: existing.NodeID,
			NewNode:      nodeID,
		}
	}

	return ip, nil
}

// ExpireStale removes routes not seen within the TTL.
func (rt *RoutingTable) ExpireStale(ttl time.Duration) int {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	cutoff := time.Now().Add(-ttl)
	var expired int

	for ip, entry := range rt.routes {
		if entry.LastSeen.Before(cutoff) {
			delete(rt.routes, ip)
			delete(rt.byNodeID, entry.NodeID)
			expired++
		}
	}

	return expired
}

// UpdateLastSeen updates the last seen time for a route.
func (rt *RoutingTable) UpdateLastSeen(ip netip.Addr) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	entry, ok := rt.routes[ip]
	if !ok {
		return false
	}

	entry.LastSeen = time.Now()
	return true
}

// Merge merges routes from another source, preferring newer entries.
func (rt *RoutingTable) Merge(entries []*RouteEntry) (added, updated int, collisions []error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	for _, entry := range entries {
		if !entry.TunnelIP.IsValid() || !rt.subnet.Contains(entry.TunnelIP) {
			continue
		}

		if existing, ok := rt.routes[entry.TunnelIP]; ok {
			if existing.NodeID != entry.NodeID {
				collisions = append(collisions, &IPCollisionError{
					IP:           entry.TunnelIP,
					ExistingNode: existing.NodeID,
					NewNode:      entry.NodeID,
				})
				continue
			}

			// Update if newer
			if entry.LastSeen.After(existing.LastSeen) {
				existing.LastSeen = entry.LastSeen
				existing.I2PDest = entry.I2PDest
				existing.HopCount = entry.HopCount
				existing.ViaNodeID = entry.ViaNodeID
				updated++
			}
		} else {
			// Add new
			entryCopy := *entry
			rt.routes[entry.TunnelIP] = &entryCopy
			rt.byNodeID[entry.NodeID] = &entryCopy
			added++
		}
	}

	return added, updated, collisions
}

// Save persists the routing table to disk.
func (rt *RoutingTable) Save() error {
	if rt.filePath == "" {
		return nil // No persistence configured
	}

	rt.mu.RLock()
	defer rt.mu.RUnlock()

	routes := make([]*RouteEntry, 0, len(rt.routes))
	for _, entry := range rt.routes {
		routes = append(routes, entry)
	}

	data, err := json.MarshalIndent(routes, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling routes: %w", err)
	}

	dir := filepath.Dir(rt.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	if err := os.WriteFile(rt.filePath, data, 0600); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}

// Load loads the routing table from disk.
func (rt *RoutingTable) Load() error {
	if rt.filePath == "" {
		return nil
	}

	data, err := os.ReadFile(rt.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No file yet
		}
		return fmt.Errorf("reading file: %w", err)
	}

	var routes []*RouteEntry
	if err := json.Unmarshal(data, &routes); err != nil {
		return fmt.Errorf("unmarshaling routes: %w", err)
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Clear and reload
	rt.routes = make(map[netip.Addr]*RouteEntry)
	rt.byNodeID = make(map[string]*RouteEntry)

	for _, entry := range routes {
		if entry.TunnelIP.IsValid() && rt.subnet.Contains(entry.TunnelIP) {
			rt.routes[entry.TunnelIP] = entry
			rt.byNodeID[entry.NodeID] = entry
		}
	}

	return nil
}

// IPCollisionError indicates an IP address collision.
type IPCollisionError struct {
	IP           netip.Addr
	ExistingNode string
	NewNode      string
}

func (e *IPCollisionError) Error() string {
	return fmt.Sprintf("IP collision for %s: existing node %s, new node %s",
		e.IP, e.ExistingNode, e.NewNode)
}

// IsIPCollision checks if an error is an IP collision error.
func IsIPCollision(err error) bool {
	var collisionErr *IPCollisionError
	return errors.As(err, &collisionErr)
}
