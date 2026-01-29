// Package testutil provides testing utilities for i2plan integration tests.
package testutil

import (
	"context"
	"errors"
	"net"
	"sync"
)

// MockSAM provides a mock SAM bridge for testing without a real I2P router.
type MockSAM struct {
	mu       sync.RWMutex
	listener net.Listener
	sessions map[string]*mockSession
	running  bool
	addr     string
}

// mockSession represents a SAM session.
type mockSession struct {
	id        string
	dest      string
	privKey   string
	connected bool
}

// NewMockSAM creates a new mock SAM bridge listening on a random port.
func NewMockSAM() (*MockSAM, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	m := &MockSAM{
		listener: ln,
		sessions: make(map[string]*mockSession),
		running:  true,
		addr:     ln.Addr().String(),
	}

	go m.acceptLoop()

	return m, nil
}

// Addr returns the address of the mock SAM bridge.
func (m *MockSAM) Addr() string {
	return m.addr
}

// Close shuts down the mock SAM bridge.
func (m *MockSAM) Close() error {
	m.mu.Lock()
	m.running = false
	m.mu.Unlock()
	return m.listener.Close()
}

func (m *MockSAM) acceptLoop() {
	for {
		conn, err := m.listener.Accept()
		if err != nil {
			return
		}
		go m.handleConnection(conn)
	}
}

func (m *MockSAM) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Simple mock - just accept connections and respond with OK
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return
		}

		// Echo a simple response
		_ = n
		conn.Write([]byte("RESULT=OK\n"))
	}
}

// MockTransport provides an in-memory transport for testing mesh operations.
type MockTransport struct {
	mu       sync.RWMutex
	nodeID   string
	dest     string
	peers    map[string]*MockPeer
	messages chan *MockMessage
}

// MockPeer represents a peer in the mock transport.
type MockPeer struct {
	NodeID    string
	Dest      string
	Connected bool
}

// MockMessage represents a message in the mock transport.
type MockMessage struct {
	From    string
	To      string
	Payload []byte
}

// NewMockTransport creates a new mock transport for testing.
func NewMockTransport(nodeID, dest string) *MockTransport {
	return &MockTransport{
		nodeID:   nodeID,
		dest:     dest,
		peers:    make(map[string]*MockPeer),
		messages: make(chan *MockMessage, 100),
	}
}

// NodeID returns the mock node ID.
func (t *MockTransport) NodeID() string {
	return t.nodeID
}

// Dest returns the mock destination.
func (t *MockTransport) Dest() string {
	return t.dest
}

// AddPeer adds a peer to the mock transport.
func (t *MockTransport) AddPeer(nodeID, dest string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.peers[nodeID] = &MockPeer{
		NodeID:    nodeID,
		Dest:      dest,
		Connected: true,
	}
}

// RemovePeer removes a peer from the mock transport.
func (t *MockTransport) RemovePeer(nodeID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.peers, nodeID)
}

// GetPeers returns all connected peers.
func (t *MockTransport) GetPeers() []*MockPeer {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]*MockPeer, 0, len(t.peers))
	for _, p := range t.peers {
		result = append(result, p)
	}
	return result
}

// SendTo sends a message to a peer.
func (t *MockTransport) SendTo(nodeID string, data []byte) error {
	t.mu.RLock()
	_, ok := t.peers[nodeID]
	t.mu.RUnlock()

	if !ok {
		return errors.New("peer not found")
	}

	msg := &MockMessage{
		From:    t.nodeID,
		To:      nodeID,
		Payload: data,
	}

	select {
	case t.messages <- msg:
		return nil
	default:
		return errors.New("message queue full")
	}
}

// Broadcast sends a message to all peers.
func (t *MockTransport) Broadcast(data []byte) error {
	t.mu.RLock()
	peers := make([]string, 0, len(t.peers))
	for id := range t.peers {
		peers = append(peers, id)
	}
	t.mu.RUnlock()

	for _, id := range peers {
		if err := t.SendTo(id, data); err != nil {
			return err
		}
	}
	return nil
}

// ReceiveMessage receives a message from the mock transport.
// Returns nil if context is cancelled.
func (t *MockTransport) ReceiveMessage(ctx context.Context) *MockMessage {
	select {
	case <-ctx.Done():
		return nil
	case msg := <-t.messages:
		return msg
	}
}

// Messages returns the message channel for direct access.
func (t *MockTransport) Messages() <-chan *MockMessage {
	return t.messages
}

// MockMeshNetwork creates an interconnected set of mock transports for testing.
type MockMeshNetwork struct {
	mu         sync.RWMutex
	transports map[string]*MockTransport
}

// NewMockMeshNetwork creates a new mock mesh network.
func NewMockMeshNetwork() *MockMeshNetwork {
	return &MockMeshNetwork{
		transports: make(map[string]*MockTransport),
	}
}

// AddNode adds a node to the mock mesh network.
func (n *MockMeshNetwork) AddNode(nodeID, dest string) *MockTransport {
	n.mu.Lock()
	defer n.mu.Unlock()

	t := NewMockTransport(nodeID, dest)
	n.transports[nodeID] = t
	return t
}

// ConnectNodes connects two nodes in the mock network.
func (n *MockMeshNetwork) ConnectNodes(nodeA, nodeB string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	a, ok := n.transports[nodeA]
	if !ok {
		return errors.New("node A not found")
	}
	b, ok := n.transports[nodeB]
	if !ok {
		return errors.New("node B not found")
	}

	a.AddPeer(nodeB, b.Dest())
	b.AddPeer(nodeA, a.Dest())
	return nil
}

// DisconnectNodes disconnects two nodes in the mock network.
func (n *MockMeshNetwork) DisconnectNodes(nodeA, nodeB string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if a, ok := n.transports[nodeA]; ok {
		a.RemovePeer(nodeB)
	}
	if b, ok := n.transports[nodeB]; ok {
		b.RemovePeer(nodeA)
	}
}

// GetTransport returns the transport for a node.
func (n *MockMeshNetwork) GetTransport(nodeID string) *MockTransport {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.transports[nodeID]
}

// NodeCount returns the number of nodes in the network.
func (n *MockMeshNetwork) NodeCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.transports)
}
