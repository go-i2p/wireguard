// Package testutil provides a multi-node test harness for integration testing
// mesh operations over real I2P connections. This harness requires a running
// I2P router with SAM enabled on localhost:7656.
package testutil

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/go-i2p/wireguard/lib/transport"
)

const (
	// DefaultSAMAddress is the default SAM bridge address for tests.
	DefaultSAMAddress = "127.0.0.1:7656"

	// DefaultDialTimeout is the timeout for SAM connectivity checks.
	DefaultDialTimeout = 5 * time.Second

	// DefaultNodeStartTimeout is how long to wait for a node to start.
	DefaultNodeStartTimeout = 30 * time.Second
)

// TestNode represents a single node in the multi-node test harness.
// Each node has its own I2P session and can communicate with other nodes.
type TestNode struct {
	mu sync.RWMutex

	// Identity
	ID   string
	Name string

	// Transport
	Transport *transport.Transport

	// State
	started   bool
	i2pDest   string
	localAddr string

	// Configuration
	samAddr string
	options []string
}

// TestNodeConfig configures a test node.
type TestNodeConfig struct {
	// ID is a unique identifier for this node (e.g., "node1", "node2").
	ID string
	// Name is the I2P tunnel name (defaults to "test-<ID>").
	Name string
	// SAMAddress is the SAM bridge address (defaults to localhost:7656).
	SAMAddress string
	// Options are SAM tunnel options.
	Options []string
}

// NewTestNode creates a new test node with the given configuration.
func NewTestNode(cfg TestNodeConfig) *TestNode {
	if cfg.Name == "" {
		cfg.Name = fmt.Sprintf("test-%s", cfg.ID)
	}
	if cfg.SAMAddress == "" {
		cfg.SAMAddress = DefaultSAMAddress
	}
	if cfg.Options == nil {
		// Use minimal tunnel settings for fast tests
		cfg.Options = []string{
			"inbound.length=1",
			"outbound.length=1",
			"inbound.quantity=1",
			"outbound.quantity=1",
		}
	}

	return &TestNode{
		ID:      cfg.ID,
		Name:    cfg.Name,
		samAddr: cfg.SAMAddress,
		options: cfg.Options,
	}
}

// Start initializes the I2P transport and obtains an I2P address.
func (n *TestNode) Start(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.started {
		return fmt.Errorf("node %s already started", n.ID)
	}

	log.Info("starting test node")

	// Create transport
	n.Transport = transport.NewTransport(n.Name, n.samAddr, n.options)

	// Open transport (this creates the I2P session)
	err := n.Transport.Open()
	if err != nil {
		return fmt.Errorf("failed to open transport: %w", err)
	}

	// Get our I2P address
	n.localAddr = n.Transport.LocalAddress()
	dest, err := n.Transport.LocalDestination()
	if err != nil {
		n.Transport.Close()
		return fmt.Errorf("failed to get destination: %w", err)
	}
	n.i2pDest = string(dest)

	n.started = true
	log.Info("test node started",
		"local_addr", n.localAddr,
		"dest_len", len(n.i2pDest))

	return nil
}

// Stop shuts down the test node.
func (n *TestNode) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.started {
		return nil
	}

	log.Info("stopping test node")

	var err error
	if n.Transport != nil {
		err = n.Transport.Close()
	}

	n.started = false
	n.i2pDest = ""
	n.localAddr = ""

	return err
}

// IsStarted returns true if the node is running.
func (n *TestNode) IsStarted() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.started
}

// LocalAddress returns the base32 I2P address.
func (n *TestNode) LocalAddress() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.localAddr
}

// I2PDestination returns the full I2P destination.
func (n *TestNode) I2PDestination() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.i2pDest
}

// MultiNodeHarness manages multiple test nodes for integration testing.
type MultiNodeHarness struct {
	mu sync.RWMutex

	nodes   map[string]*TestNode
	samAddr string

	// Track node creation order for cleanup
	nodeOrder []string
}

// MultiNodeHarnessConfig configures the multi-node harness.
type MultiNodeHarnessConfig struct {
	// SAMAddress is the SAM bridge address (defaults to localhost:7656).
	SAMAddress string
}

// NewMultiNodeHarness creates a new multi-node test harness.
func NewMultiNodeHarness(cfg MultiNodeHarnessConfig) *MultiNodeHarness {
	if cfg.SAMAddress == "" {
		cfg.SAMAddress = DefaultSAMAddress
	}

	return &MultiNodeHarness{
		nodes:   make(map[string]*TestNode),
		samAddr: cfg.SAMAddress,
	}
}

// RequireSAM checks that SAM is available and returns an error if not.
func (h *MultiNodeHarness) RequireSAM() error {
	conn, err := net.DialTimeout("tcp", h.samAddr, DefaultDialTimeout)
	if err != nil {
		return fmt.Errorf("SAM bridge unavailable at %s: %w\n"+
			"Integration tests require a running I2P router with SAM enabled.\n"+
			"Start your I2P router and ensure SAM is listening on port 7656.",
			h.samAddr, err)
	}
	conn.Close()
	return nil
}

// CreateNode creates and starts a new test node.
func (h *MultiNodeHarness) CreateNode(ctx context.Context, id string) (*TestNode, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.nodes[id]; exists {
		return nil, fmt.Errorf("node %s already exists", id)
	}

	node := NewTestNode(TestNodeConfig{
		ID:         id,
		SAMAddress: h.samAddr,
		Logger:     h.logger,
	})

	if err := node.Start(ctx); err != nil {
		return nil, err
	}

	h.nodes[id] = node
	h.nodeOrder = append(h.nodeOrder, id)

	log.Info("created node", "id", id, "addr", node.LocalAddress())
	return node, nil
}

// CreateNodes creates and starts multiple nodes.
func (h *MultiNodeHarness) CreateNodes(ctx context.Context, count int) ([]*TestNode, error) {
	nodes := make([]*TestNode, 0, count)

	for i := 0; i < count; i++ {
		id := fmt.Sprintf("node%d", i+1)
		node, err := h.CreateNode(ctx, id)
		if err != nil {
			// Clean up already created nodes
			h.Cleanup()
			return nil, fmt.Errorf("failed to create node %s: %w", id, err)
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// GetNode returns a node by ID.
func (h *MultiNodeHarness) GetNode(id string) *TestNode {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.nodes[id]
}

// ListNodes returns all nodes.
func (h *MultiNodeHarness) ListNodes() []*TestNode {
	h.mu.RLock()
	defer h.mu.RUnlock()

	nodes := make([]*TestNode, 0, len(h.nodes))
	for _, id := range h.nodeOrder {
		if node, ok := h.nodes[id]; ok {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// NodeCount returns the number of nodes.
func (h *MultiNodeHarness) NodeCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.nodes)
}

// ConnectPeers adds peer tracking between two nodes.
// This registers each node as a peer of the other, but doesn't establish
// a WireGuard tunnel (that requires the full mesh implementation).
func (h *MultiNodeHarness) ConnectPeers(nodeA, nodeB string) error {
	h.mu.RLock()
	a, okA := h.nodes[nodeA]
	b, okB := h.nodes[nodeB]
	h.mu.RUnlock()

	if !okA {
		return fmt.Errorf("node %s not found", nodeA)
	}
	if !okB {
		return fmt.Errorf("node %s not found", nodeB)
	}
	if !a.IsStarted() || !b.IsStarted() {
		return fmt.Errorf("both nodes must be started")
	}

	// Add each node as a peer of the other in their transports
	if err := a.Transport.AddPeer(b.LocalAddress(), nodeB); err != nil {
		return fmt.Errorf("failed to add %s to %s: %w", nodeB, nodeA, err)
	}
	if err := b.Transport.AddPeer(a.LocalAddress(), nodeA); err != nil {
		return fmt.Errorf("failed to add %s to %s: %w", nodeA, nodeB, err)
	}

	log.Info("connected peers", "nodeA", nodeA, "nodeB", nodeB)
	return nil
}

// Cleanup stops all nodes and cleans up resources.
// Should be called with defer after creating the harness.
func (h *MultiNodeHarness) Cleanup() {
	h.mu.Lock()
	defer h.mu.Unlock()

	log.Info("cleaning up harness", "nodes", len(h.nodes))

	// Stop nodes in reverse order
	for i := len(h.nodeOrder) - 1; i >= 0; i-- {
		id := h.nodeOrder[i]
		if node, ok := h.nodes[id]; ok {
			if err := node.Stop(); err != nil {
				log.Warn("error stopping node", "id", id, "error", err)
			}
		}
	}

	h.nodes = make(map[string]*TestNode)
	h.nodeOrder = nil
}

// WaitForNodes waits for all nodes to be addressable.
// This is useful after creating nodes to ensure they're fully initialized.
func (h *MultiNodeHarness) WaitForNodes(ctx context.Context) error {
	nodes := h.ListNodes()
	for _, node := range nodes {
		if node.LocalAddress() == "" {
			return fmt.Errorf("node %s has no address", node.ID)
		}
	}
	return nil
}

// GetNodeAddresses returns a map of node IDs to their I2P addresses.
func (h *MultiNodeHarness) GetNodeAddresses() map[string]string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	addrs := make(map[string]string, len(h.nodes))
	for id, node := range h.nodes {
		addrs[id] = node.LocalAddress()
	}
	return addrs
}
