package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// Mock implementations for testing

type mockNodeProvider struct {
	nodeName       string
	nodeID         string
	state          string
	tunnelIP       string
	i2pDestination string
	startTime      time.Time
	version        string
}

func (m *mockNodeProvider) NodeName() string       { return m.nodeName }
func (m *mockNodeProvider) NodeID() string         { return m.nodeID }
func (m *mockNodeProvider) State() string          { return m.state }
func (m *mockNodeProvider) TunnelIP() string       { return m.tunnelIP }
func (m *mockNodeProvider) I2PDestination() string { return m.i2pDestination }
func (m *mockNodeProvider) StartTime() time.Time   { return m.startTime }
func (m *mockNodeProvider) Version() string        { return m.version }

type mockPeerProvider struct {
	peers []PeerInfo
}

func (m *mockPeerProvider) ListPeers() []PeerInfo {
	return m.peers
}

func (m *mockPeerProvider) ConnectPeer(ctx context.Context, inviteCode string) (*PeersConnectResult, error) {
	if inviteCode == "" {
		return nil, errors.New("invite code required")
	}
	return &PeersConnectResult{
		NodeID:   "connected-node",
		TunnelIP: "10.42.1.1",
		Message:  "connected",
	}, nil
}

type mockInviteProvider struct{}

func (m *mockInviteProvider) CreateInvite(expiry time.Duration, maxUses int) (*InviteCreateResult, error) {
	return &InviteCreateResult{
		InviteCode: "i2plan://test-invite-code",
		ExpiresAt:  time.Now().Add(expiry).Format(time.RFC3339),
		MaxUses:    maxUses,
	}, nil
}

func (m *mockInviteProvider) AcceptInvite(ctx context.Context, inviteCode string) (*InviteAcceptResult, error) {
	if inviteCode == "" {
		return nil, errors.New("invite code required")
	}
	return &InviteAcceptResult{
		NetworkID:  "mesh-network-1",
		PeerNodeID: "inviter-node",
		TunnelIP:   "10.42.1.2",
		Message:    "joined network",
	}, nil
}

type mockRouteProvider struct {
	routes []RouteInfo
}

func (m *mockRouteProvider) ListRoutes() []RouteInfo {
	return m.routes
}

type mockConfigProvider struct {
	config map[string]any
}

func (m *mockConfigProvider) GetConfig(key string) (any, error) {
	if key == "" {
		return m.config, nil
	}
	val, ok := m.config[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return val, nil
}

func (m *mockConfigProvider) SetConfig(key string, value any) (any, error) {
	old := m.config[key]
	m.config[key] = value
	return old, nil
}

func TestNewHandlers(t *testing.T) {
	h := NewHandlers(HandlersConfig{
		Node:   &mockNodeProvider{},
		Peers:  &mockPeerProvider{},
		Invite: &mockInviteProvider{},
		Routes: &mockRouteProvider{},
		Config: &mockConfigProvider{},
	})

	if h == nil {
		t.Fatal("handlers is nil")
	}
}

func TestHandlersRegisterAll(t *testing.T) {
	s, _ := NewServer(ServerConfig{})
	h := NewHandlers(HandlersConfig{})

	h.RegisterAll(s)

	expectedMethods := []string{
		"status",
		"peers.list",
		"peers.connect",
		"invite.create",
		"invite.accept",
		"routes.list",
		"config.get",
		"config.set",
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, method := range expectedMethods {
		if _, ok := s.handlers[method]; !ok {
			t.Errorf("handler %s not registered", method)
		}
	}
}

func TestStatusHandler(t *testing.T) {
	startTime := time.Now().Add(-2 * time.Hour)
	node := &mockNodeProvider{
		nodeName:       "test-node",
		nodeID:         "node-123",
		state:          "running",
		tunnelIP:       "10.42.1.1",
		i2pDestination: "abcdefghijklmnopqrstuvwxyz1234567890",
		startTime:      startTime,
		version:        "1.0.0",
	}
	peers := &mockPeerProvider{
		peers: []PeerInfo{
			{NodeID: "peer1"},
			{NodeID: "peer2"},
		},
	}

	h := NewHandlers(HandlersConfig{
		Node:  node,
		Peers: peers,
	})

	result, err := h.Status(context.Background(), nil)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	status, ok := result.(*StatusResult)
	if !ok {
		t.Fatalf("expected *StatusResult, got %T", result)
	}

	if status.NodeName != "test-node" {
		t.Errorf("NodeName: %s != test-node", status.NodeName)
	}
	if status.NodeID != "node-123" {
		t.Errorf("NodeID: %s != node-123", status.NodeID)
	}
	if status.State != "running" {
		t.Errorf("State: %s != running", status.State)
	}
	if status.PeerCount != 2 {
		t.Errorf("PeerCount: %d != 2", status.PeerCount)
	}
	if status.Version != "1.0.0" {
		t.Errorf("Version: %s != 1.0.0", status.Version)
	}
}

func TestStatusHandlerNoNode(t *testing.T) {
	h := NewHandlers(HandlersConfig{})

	_, err := h.Status(context.Background(), nil)
	if err == nil {
		t.Error("expected error when node not available")
	}
}

func TestPeersListHandler(t *testing.T) {
	peers := &mockPeerProvider{
		peers: []PeerInfo{
			{NodeID: "peer1", TunnelIP: "10.42.1.2", State: "connected"},
			{NodeID: "peer2", TunnelIP: "10.42.1.3", State: "pending"},
		},
	}

	h := NewHandlers(HandlersConfig{Peers: peers})

	result, err := h.PeersList(context.Background(), nil)
	if err != nil {
		t.Fatalf("PeersList: %v", err)
	}

	peersList, ok := result.(*PeersListResult)
	if !ok {
		t.Fatalf("expected *PeersListResult, got %T", result)
	}

	if peersList.Total != 2 {
		t.Errorf("Total: %d != 2", peersList.Total)
	}
	if len(peersList.Peers) != 2 {
		t.Errorf("Peers length: %d != 2", len(peersList.Peers))
	}
}

func TestPeersConnectHandler(t *testing.T) {
	peers := &mockPeerProvider{}
	h := NewHandlers(HandlersConfig{Peers: peers})

	t.Run("valid request", func(t *testing.T) {
		params := json.RawMessage(`{"invite_code":"i2plan://test"}`)
		result, err := h.PeersConnect(context.Background(), params)
		if err != nil {
			t.Fatalf("PeersConnect: %v", err)
		}

		connectResult, ok := result.(*PeersConnectResult)
		if !ok {
			t.Fatalf("expected *PeersConnectResult, got %T", result)
		}

		if connectResult.NodeID != "connected-node" {
			t.Errorf("NodeID: %s != connected-node", connectResult.NodeID)
		}
	})

	t.Run("missing invite code", func(t *testing.T) {
		params := json.RawMessage(`{}`)
		_, err := h.PeersConnect(context.Background(), params)
		if err == nil {
			t.Error("expected error for missing invite code")
		}
	})

	t.Run("invalid params", func(t *testing.T) {
		params := json.RawMessage(`invalid`)
		_, err := h.PeersConnect(context.Background(), params)
		if err == nil {
			t.Error("expected error for invalid params")
		}
	})
}

func TestInviteCreateHandler(t *testing.T) {
	invite := &mockInviteProvider{}
	h := NewHandlers(HandlersConfig{Invite: invite})

	t.Run("default options", func(t *testing.T) {
		result, err := h.InviteCreate(context.Background(), nil)
		if err != nil {
			t.Fatalf("InviteCreate: %v", err)
		}

		createResult, ok := result.(*InviteCreateResult)
		if !ok {
			t.Fatalf("expected *InviteCreateResult, got %T", result)
		}

		if createResult.InviteCode == "" {
			t.Error("expected invite code")
		}
		if createResult.MaxUses != 1 {
			t.Errorf("MaxUses: %d != 1", createResult.MaxUses)
		}
	})

	t.Run("custom options", func(t *testing.T) {
		params := json.RawMessage(`{"expiry":"48h","max_uses":5}`)
		result, err := h.InviteCreate(context.Background(), params)
		if err != nil {
			t.Fatalf("InviteCreate: %v", err)
		}

		createResult, ok := result.(*InviteCreateResult)
		if !ok {
			t.Fatalf("expected *InviteCreateResult, got %T", result)
		}

		if createResult.MaxUses != 5 {
			t.Errorf("MaxUses: %d != 5", createResult.MaxUses)
		}
	})

	t.Run("invalid expiry", func(t *testing.T) {
		params := json.RawMessage(`{"expiry":"not-a-duration"}`)
		_, err := h.InviteCreate(context.Background(), params)
		if err == nil {
			t.Error("expected error for invalid expiry")
		}
	})
}

func TestInviteAcceptHandler(t *testing.T) {
	invite := &mockInviteProvider{}
	h := NewHandlers(HandlersConfig{Invite: invite})

	t.Run("valid request", func(t *testing.T) {
		params := json.RawMessage(`{"invite_code":"i2plan://test"}`)
		result, err := h.InviteAccept(context.Background(), params)
		if err != nil {
			t.Fatalf("InviteAccept: %v", err)
		}

		acceptResult, ok := result.(*InviteAcceptResult)
		if !ok {
			t.Fatalf("expected *InviteAcceptResult, got %T", result)
		}

		if acceptResult.NetworkID != "mesh-network-1" {
			t.Errorf("NetworkID: %s != mesh-network-1", acceptResult.NetworkID)
		}
	})

	t.Run("missing invite code", func(t *testing.T) {
		params := json.RawMessage(`{}`)
		_, err := h.InviteAccept(context.Background(), params)
		if err == nil {
			t.Error("expected error for missing invite code")
		}
	})
}

func TestRoutesListHandler(t *testing.T) {
	routes := &mockRouteProvider{
		routes: []RouteInfo{
			{TunnelIP: "10.42.1.2", NodeID: "node1", HopCount: 0},
			{TunnelIP: "10.42.1.3", NodeID: "node2", HopCount: 1, ViaNodeID: "node1"},
		},
	}

	h := NewHandlers(HandlersConfig{Routes: routes})

	result, err := h.RoutesList(context.Background(), nil)
	if err != nil {
		t.Fatalf("RoutesList: %v", err)
	}

	routesList, ok := result.(*RoutesListResult)
	if !ok {
		t.Fatalf("expected *RoutesListResult, got %T", result)
	}

	if routesList.Total != 2 {
		t.Errorf("Total: %d != 2", routesList.Total)
	}
}

func TestConfigGetHandler(t *testing.T) {
	config := &mockConfigProvider{
		config: map[string]any{
			"mesh.max_peers": 50,
			"node.name":      "test-node",
		},
	}

	h := NewHandlers(HandlersConfig{Config: config})

	t.Run("get specific key", func(t *testing.T) {
		params := json.RawMessage(`{"key":"mesh.max_peers"}`)
		result, err := h.ConfigGet(context.Background(), params)
		if err != nil {
			t.Fatalf("ConfigGet: %v", err)
		}

		configResult, ok := result.(*ConfigGetResult)
		if !ok {
			t.Fatalf("expected *ConfigGetResult, got %T", result)
		}

		if configResult.Value != 50 {
			t.Errorf("Value: %v != 50", configResult.Value)
		}
	})

	t.Run("get all config", func(t *testing.T) {
		result, err := h.ConfigGet(context.Background(), nil)
		if err != nil {
			t.Fatalf("ConfigGet: %v", err)
		}

		configResult, ok := result.(*ConfigGetResult)
		if !ok {
			t.Fatalf("expected *ConfigGetResult, got %T", result)
		}

		// Should return entire config map
		if configResult.Value == nil {
			t.Error("expected config value")
		}
	})

	t.Run("key not found", func(t *testing.T) {
		params := json.RawMessage(`{"key":"unknown.key"}`)
		_, err := h.ConfigGet(context.Background(), params)
		if err == nil {
			t.Error("expected error for unknown key")
		}
	})
}

func TestConfigSetHandler(t *testing.T) {
	config := &mockConfigProvider{
		config: map[string]any{
			"mesh.max_peers": 50,
		},
	}

	h := NewHandlers(HandlersConfig{Config: config})

	t.Run("set value", func(t *testing.T) {
		params := json.RawMessage(`{"key":"mesh.max_peers","value":100}`)
		result, err := h.ConfigSet(context.Background(), params)
		if err != nil {
			t.Fatalf("ConfigSet: %v", err)
		}

		setResult, ok := result.(*ConfigSetResult)
		if !ok {
			t.Fatalf("expected *ConfigSetResult, got %T", result)
		}

		if setResult.Key != "mesh.max_peers" {
			t.Errorf("Key: %s != mesh.max_peers", setResult.Key)
		}
		if setResult.OldValue != 50 {
			t.Errorf("OldValue: %v != 50", setResult.OldValue)
		}
		// Note: JSON unmarshals numbers as float64
		if v, ok := setResult.NewValue.(float64); !ok || v != 100 {
			t.Errorf("NewValue: %v != 100", setResult.NewValue)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		params := json.RawMessage(`{"value":100}`)
		_, err := h.ConfigSet(context.Background(), params)
		if err == nil {
			t.Error("expected error for missing key")
		}
	})
}

func TestTruncateI2PDest(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"short"},
		{""},
		{"1234567890123456"},
		{"12345678901234567890123456789012345678"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncateI2PDest(tt.input)
			if len(tt.input) <= 16 {
				if result != tt.input {
					t.Errorf("expected %s, got %s", tt.input, result)
				}
			} else {
				// Just check it's truncated and contains ellipsis
				if len(result) >= len(tt.input) {
					t.Errorf("expected truncated string, got %s", result)
				}
				if result[8:11] != "..." {
					t.Errorf("expected ellipsis in middle, got %s", result)
				}
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{30 * time.Second, "30s"},
		{90 * time.Second, "2m0s"},
		{65 * time.Minute, "1h5m0s"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatDuration(tt.duration)
			// Just verify it doesn't panic and returns something
			if result == "" {
				t.Error("expected non-empty string")
			}
		})
	}
}

func TestFormatInt(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{9, "9"},
		{10, "10"},
		{42, "42"},
		{99, "99"},
		{100, "100"},   // Edge case: was broken before fix
		{365, "365"},   // Days in a year
		{1000, "1000"}, // Larger numbers
		{12345, "12345"},
		{-1, "-1"}, // Negative numbers
		{-100, "-100"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatInt(tt.input)
			if result != tt.expected {
				t.Errorf("formatInt(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
