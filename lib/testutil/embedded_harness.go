package testutil

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-i2p/wireguard/lib/embedded"
)

// EmbeddedTestNode represents a test node using the embedded VPN API.
// This provides higher-level testing capabilities compared to TestNode,
// including full mesh functionality (peers, invites, routes).
type EmbeddedTestNode struct {
	mu sync.RWMutex

	// Identity
	ID   string
	Name string

	// Embedded VPN
	VPN *embedded.VPN

	// Configuration
	dataDir string
	samAddr string
	logger  *slog.Logger
}

// EmbeddedTestNodeConfig configures an embedded test node.
type EmbeddedTestNodeConfig struct {
	// ID is a unique identifier for this node (e.g., "node1", "node2").
	ID string
	// Name is the node name (defaults to "test-<ID>").
	Name string
	// SAMAddress is the SAM bridge address (defaults to localhost:7656).
	SAMAddress string
	// DataDir is the data directory (defaults to temp dir).
	DataDir string
	// Logger for node operations.
	Logger *slog.Logger
}

// NewEmbeddedTestNode creates a new embedded test node.
func NewEmbeddedTestNode(cfg EmbeddedTestNodeConfig) *EmbeddedTestNode {
	if cfg.Name == "" {
		cfg.Name = fmt.Sprintf("test-%s", cfg.ID)
	}
	if cfg.SAMAddress == "" {
		cfg.SAMAddress = DefaultSAMAddress
	}
	if cfg.DataDir == "" {
		cfg.DataDir = filepath.Join(os.TempDir(), "i2plan-test", cfg.ID)
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &EmbeddedTestNode{
		ID:      cfg.ID,
		Name:    cfg.Name,
		samAddr: cfg.SAMAddress,
		dataDir: cfg.DataDir,
		logger:  cfg.Logger.With("node", cfg.ID),
	}
}

// Start initializes and starts the embedded VPN.
func (n *EmbeddedTestNode) Start(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.VPN != nil {
		return fmt.Errorf("node %s already started", n.ID)
	}

	n.logger.Info("starting embedded test node")

	vpn, err := embedded.New(embedded.Config{
		NodeName:     n.Name,
		DataDir:      n.dataDir,
		SAMAddress:   n.samAddr,
		TunnelLength: 1, // Fast tunnels for tests
		MaxPeers:     10,
		Logger:       n.logger,
		EnableRPC:    false,
		EnableWeb:    false,
	})
	if err != nil {
		return fmt.Errorf("failed to create VPN: %w", err)
	}

	if err := vpn.Start(ctx); err != nil {
		return fmt.Errorf("failed to start VPN: %w", err)
	}

	n.VPN = vpn
	n.logger.Info("embedded test node started")

	return nil
}

// Stop shuts down the embedded test node.
func (n *EmbeddedTestNode) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.VPN == nil {
		return nil
	}

	n.logger.Info("stopping embedded test node")

	err := n.VPN.Close()
	n.VPN = nil

	return err
}

// IsStarted returns true if the node is running.
func (n *EmbeddedTestNode) IsStarted() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.VPN != nil && n.VPN.State() == embedded.StateRunning
}

// I2PDestination returns the full I2P destination.
func (n *EmbeddedTestNode) I2PDestination() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.VPN == nil {
		return ""
	}
	return n.VPN.I2PDestination()
}

// I2PAddress returns the short I2P address (base32.b32.i2p).
func (n *EmbeddedTestNode) I2PAddress() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.VPN == nil {
		return ""
	}
	return n.VPN.I2PAddress()
}

// TunnelIP returns the mesh tunnel IP address.
func (n *EmbeddedTestNode) TunnelIP() netip.Addr {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.VPN == nil {
		return netip.Addr{}
	}
	return n.VPN.TunnelIP()
}

// NodeID returns the unique node identifier.
func (n *EmbeddedTestNode) NodeID() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.VPN == nil {
		return ""
	}
	return n.VPN.NodeID()
}

// Status returns the current VPN status.
func (n *EmbeddedTestNode) Status() embedded.Status {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.VPN == nil {
		return embedded.Status{State: embedded.StateInitial}
	}
	return n.VPN.Status()
}

// Peers returns the list of connected peers.
func (n *EmbeddedTestNode) Peers() []embedded.PeerInfo {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.VPN == nil {
		return nil
	}
	return n.VPN.Peers()
}

// CreateInvite generates an invite code.
func (n *EmbeddedTestNode) CreateInvite(ctx context.Context, maxUses int) (string, error) {
	n.mu.RLock()
	vpn := n.VPN
	n.mu.RUnlock()

	if vpn == nil {
		return "", fmt.Errorf("node %s not started", n.ID)
	}

	return vpn.CreateInvite(DefaultNodeStartTimeout, maxUses)
}

// AcceptInvite joins a network using an invite code.
func (n *EmbeddedTestNode) AcceptInvite(ctx context.Context, inviteCode string) error {
	n.mu.RLock()
	vpn := n.VPN
	n.mu.RUnlock()

	if vpn == nil {
		return fmt.Errorf("node %s not started", n.ID)
	}

	return vpn.AcceptInvite(ctx, inviteCode)
}

// Events returns the VPN event channel.
func (n *EmbeddedTestNode) Events() <-chan embedded.Event {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.VPN == nil {
		return nil
	}
	return n.VPN.Events()
}

// EmbeddedHarness manages multiple embedded test nodes.
type EmbeddedHarness struct {
	mu sync.RWMutex

	nodes     map[string]*EmbeddedTestNode
	samAddr   string
	baseDir   string
	logger    *slog.Logger
	nodeOrder []string
}

// EmbeddedHarnessConfig configures the embedded harness.
type EmbeddedHarnessConfig struct {
	// SAMAddress is the SAM bridge address (defaults to localhost:7656).
	SAMAddress string
	// BaseDir is the base directory for node data (defaults to temp dir).
	BaseDir string
	// Logger for harness operations.
	Logger *slog.Logger
}

// NewEmbeddedHarness creates a new embedded test harness.
func NewEmbeddedHarness(cfg EmbeddedHarnessConfig) *EmbeddedHarness {
	if cfg.SAMAddress == "" {
		cfg.SAMAddress = DefaultSAMAddress
	}
	if cfg.BaseDir == "" {
		cfg.BaseDir = filepath.Join(os.TempDir(), "i2plan-test-harness")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &EmbeddedHarness{
		nodes:   make(map[string]*EmbeddedTestNode),
		samAddr: cfg.SAMAddress,
		baseDir: cfg.BaseDir,
		logger:  cfg.Logger.With("component", "embedded-harness"),
	}
}

// RequireSAM checks that SAM is available.
func (h *EmbeddedHarness) RequireSAM() error {
	// Reuse the existing RequireSAM logic
	mnh := NewMultiNodeHarness(MultiNodeHarnessConfig{SAMAddress: h.samAddr})
	return mnh.RequireSAM()
}

// CreateNode creates and starts a new embedded test node.
func (h *EmbeddedHarness) CreateNode(ctx context.Context, id string) (*EmbeddedTestNode, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.nodes[id]; exists {
		return nil, fmt.Errorf("node %s already exists", id)
	}

	node := NewEmbeddedTestNode(EmbeddedTestNodeConfig{
		ID:         id,
		SAMAddress: h.samAddr,
		DataDir:    filepath.Join(h.baseDir, id),
		Logger:     h.logger,
	})

	if err := node.Start(ctx); err != nil {
		return nil, err
	}

	h.nodes[id] = node
	h.nodeOrder = append(h.nodeOrder, id)

	h.logger.Info("created embedded node", "id", id)
	return node, nil
}

// CreateNodes creates and starts multiple nodes.
func (h *EmbeddedHarness) CreateNodes(ctx context.Context, count int) ([]*EmbeddedTestNode, error) {
	nodes := make([]*EmbeddedTestNode, 0, count)

	for i := 0; i < count; i++ {
		id := fmt.Sprintf("node%d", i+1)
		node, err := h.CreateNode(ctx, id)
		if err != nil {
			h.Cleanup()
			return nil, fmt.Errorf("failed to create node %s: %w", id, err)
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// GetNode returns a node by ID.
func (h *EmbeddedHarness) GetNode(id string) *EmbeddedTestNode {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.nodes[id]
}

// ListNodes returns all nodes.
func (h *EmbeddedHarness) ListNodes() []*EmbeddedTestNode {
	h.mu.RLock()
	defer h.mu.RUnlock()

	nodes := make([]*EmbeddedTestNode, 0, len(h.nodes))
	for _, id := range h.nodeOrder {
		if node, ok := h.nodes[id]; ok {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// NodeCount returns the number of nodes.
func (h *EmbeddedHarness) NodeCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.nodes)
}

// ConnectViaInvite connects two nodes using the invite system.
func (h *EmbeddedHarness) ConnectViaInvite(ctx context.Context, inviterID, inviteeID string) error {
	h.mu.RLock()
	inviter, okA := h.nodes[inviterID]
	invitee, okB := h.nodes[inviteeID]
	h.mu.RUnlock()

	if !okA {
		return fmt.Errorf("node %s not found", inviterID)
	}
	if !okB {
		return fmt.Errorf("node %s not found", inviteeID)
	}
	if !inviter.IsStarted() || !invitee.IsStarted() {
		return fmt.Errorf("both nodes must be started")
	}

	// Create invite from inviter
	inviteCode, err := inviter.CreateInvite(ctx, 1)
	if err != nil {
		return fmt.Errorf("failed to create invite: %w", err)
	}

	// Invitee accepts the invite
	if err := invitee.AcceptInvite(ctx, inviteCode); err != nil {
		return fmt.Errorf("failed to accept invite: %w", err)
	}

	h.logger.Info("connected nodes via invite", "inviter", inviterID, "invitee", inviteeID)
	return nil
}

// Cleanup stops all nodes and cleans up resources.
func (h *EmbeddedHarness) Cleanup() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.logger.Info("cleaning up embedded harness", "nodes", len(h.nodes))

	// Stop nodes in reverse order
	for i := len(h.nodeOrder) - 1; i >= 0; i-- {
		id := h.nodeOrder[i]
		if node, ok := h.nodes[id]; ok {
			if err := node.Stop(); err != nil {
				h.logger.Warn("error stopping node", "id", id, "error", err)
			}
		}
	}

	h.nodes = make(map[string]*EmbeddedTestNode)
	h.nodeOrder = nil

	// Clean up data directory
	if h.baseDir != "" {
		os.RemoveAll(h.baseDir)
	}
}
