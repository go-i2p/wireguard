package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/go-i2p/wireguard/lib/rpc"
)

// fakeKeyMsg creates a tea.KeyMsg for testing.
func fakeKeyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func fakeKeyMsgWithType(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

func TestTabString(t *testing.T) {
	tests := []struct {
		tab      Tab
		expected string
	}{
		{TabPeers, "Peers"},
		{TabRoutes, "Routes"},
		{TabInvites, "Invites"},
		{TabStatus, "Status"},
		{Tab(99), "Unknown"},
	}

	for _, tc := range tests {
		if got := tc.tab.String(); got != tc.expected {
			t.Errorf("Tab(%d).String() = %q, want %q", tc.tab, got, tc.expected)
		}
	}
}

func TestPeersModelSetData(t *testing.T) {
	m := NewPeersModel()

	peers := &rpc.PeersListResult{
		Peers: []rpc.PeerInfo{
			{NodeID: "node1", TunnelIP: "10.0.0.2", State: "connected"},
			{NodeID: "node2", TunnelIP: "10.0.0.3", State: "disconnected"},
		},
		Total: 2,
	}

	m.SetData(peers)

	if m.peers == nil {
		t.Fatal("SetData: peers is nil")
	}
	if len(m.peers.Peers) != 2 {
		t.Errorf("SetData: got %d peers, want 2", len(m.peers.Peers))
	}
	if m.cursor != 0 {
		t.Errorf("SetData: cursor = %d, want 0", m.cursor)
	}
}

func TestPeersModelSelectedPeer(t *testing.T) {
	m := NewPeersModel()

	// No data - should return nil
	if peer := m.SelectedPeer(); peer != nil {
		t.Error("SelectedPeer: expected nil when no data")
	}

	m.SetData(&rpc.PeersListResult{
		Peers: []rpc.PeerInfo{
			{NodeID: "node1", TunnelIP: "10.0.0.2"},
			{NodeID: "node2", TunnelIP: "10.0.0.3"},
		},
		Total: 2,
	})

	peer := m.SelectedPeer()
	if peer == nil {
		t.Fatal("SelectedPeer: expected non-nil")
	}
	if peer.NodeID != "node1" {
		t.Errorf("SelectedPeer: got %q, want %q", peer.NodeID, "node1")
	}
}

func TestRoutesModelSetData(t *testing.T) {
	m := NewRoutesModel()

	routes := &rpc.RoutesListResult{
		Routes: []rpc.RouteInfo{
			{TunnelIP: "10.0.0.2", NodeID: "node1", HopCount: 1},
			{TunnelIP: "10.0.0.3", NodeID: "node2", HopCount: 2, ViaNodeID: "node1"},
		},
		Total: 2,
	}

	m.SetData(routes)

	if m.routes == nil {
		t.Fatal("SetData: routes is nil")
	}
	if len(m.routes.Routes) != 2 {
		t.Errorf("SetData: got %d routes, want 2", len(m.routes.Routes))
	}
}

func TestRoutesModelSelectedRoute(t *testing.T) {
	m := NewRoutesModel()

	// No data - should return nil
	if route := m.SelectedRoute(); route != nil {
		t.Error("SelectedRoute: expected nil when no data")
	}

	m.SetData(&rpc.RoutesListResult{
		Routes: []rpc.RouteInfo{
			{TunnelIP: "10.0.0.2", NodeID: "node1", HopCount: 0, WGPublicKey: "key1"},
			{TunnelIP: "10.0.0.3", NodeID: "node2", HopCount: 1, ViaNodeID: "node1"},
		},
		Total: 2,
	})

	route := m.SelectedRoute()
	if route == nil {
		t.Fatal("SelectedRoute: expected non-nil")
	}
	if route.NodeID != "node1" {
		t.Errorf("SelectedRoute: NodeID = %q, want %q", route.NodeID, "node1")
	}

	// Move cursor down and verify
	m.cursor = 1
	route = m.SelectedRoute()
	if route == nil {
		t.Fatal("SelectedRoute at cursor=1: expected non-nil")
	}
	if route.NodeID != "node2" {
		t.Errorf("SelectedRoute at cursor=1: NodeID = %q, want %q", route.NodeID, "node2")
	}
}

func TestRoutesModelRenderRouteDetail(t *testing.T) {
	m := NewRoutesModel()

	// No route selected
	detail := m.RenderRouteDetail()
	if detail == "" {
		t.Error("RenderRouteDetail: expected non-empty string for no selection")
	}

	// With route data
	m.SetData(&rpc.RoutesListResult{
		Routes: []rpc.RouteInfo{
			{
				TunnelIP:    "10.0.0.2",
				NodeID:      "node1",
				HopCount:    0,
				WGPublicKey: "abc123key",
				I2PDest:     "test.i2p.dest",
				CreatedAt:   "2024-01-01T00:00:00Z",
				LastSeen:    "2024-01-02T00:00:00Z",
			},
		},
		Total: 1,
	})

	detail = m.RenderRouteDetail()

	// Verify detail contains expected info
	if detail == "" {
		t.Fatal("RenderRouteDetail: expected non-empty detail")
	}
	// The detail should contain route info (actual formatting may vary)
	if len(detail) < 50 {
		t.Errorf("RenderRouteDetail: detail seems too short: %q", detail)
	}
}

func TestStatusModelSetData(t *testing.T) {
	m := NewStatusModel()

	status := &rpc.StatusResult{
		NodeName:       "test-node",
		NodeID:         "abc123",
		State:          "running",
		TunnelIP:       "10.0.0.1",
		I2PDestination: "abcd1234~...",
		PeerCount:      5,
		Uptime:         "1h30m",
		Version:        "1.0.0",
	}

	m.SetData(status)

	if m.status == nil {
		t.Fatal("SetData: status is nil")
	}
	if m.status.NodeName != "test-node" {
		t.Errorf("SetData: NodeName = %q, want %q", m.status.NodeName, "test-node")
	}
}

func TestInvitesModel(t *testing.T) {
	m := NewInvitesModel()

	// Initial state
	if m.mode != InvitesModeNormal {
		t.Errorf("Initial mode = %d, want %d (InvitesModeNormal)", m.mode, InvitesModeNormal)
	}

	// Test SetCreatedInvite
	invite := &rpc.InviteCreateResult{
		InviteCode: "test-invite-code",
		ExpiresAt:  "2024-01-01T00:00:00Z",
		MaxUses:    10,
	}
	m.SetCreatedInvite(invite)
	if m.createdInvite == nil {
		t.Error("SetCreatedInvite: createdInvite is nil")
	}
	if m.createdInvite.InviteCode != "test-invite-code" {
		t.Errorf("SetCreatedInvite: InviteCode = %q, want %q", m.createdInvite.InviteCode, "test-invite-code")
	}

	// Test SetAcceptResult
	m.mode = InvitesModeAccept
	result := &rpc.InviteAcceptResult{
		NetworkID:  "net1",
		PeerNodeID: "peer1",
		TunnelIP:   "10.0.0.5",
		Message:    "Connected successfully",
	}
	m.SetAcceptResult(result)
	if m.mode != InvitesModeNormal {
		t.Errorf("After SetAcceptResult: mode = %d, want %d (InvitesModeNormal)", m.mode, InvitesModeNormal)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a long string", 10, "this is..."},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"},
	}

	for _, tc := range tests {
		if got := truncate(tc.input, tc.maxLen); got != tc.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.expected)
		}
	}
}

func TestStylesExist(t *testing.T) {
	// Verify all expected styles are accessible (compile-time check)
	_ = styles.Title
	_ = styles.TabActive
	_ = styles.TabInactive
	_ = styles.Error
	_ = styles.Success
	_ = styles.Warning
	_ = styles.Muted
	_ = styles.TableHeader
	_ = styles.TableRow
	_ = styles.Selected
	_ = styles.BoxTitle
}

func TestKeysExist(t *testing.T) {
	// Verify all expected keys are accessible (compile-time check)
	_ = keys.Quit
	_ = keys.Tab
	_ = keys.ShiftTab
	_ = keys.Refresh
	_ = keys.Up
	_ = keys.Down
	_ = keys.Enter
	_ = keys.Escape
	_ = keys.Peers
	_ = keys.Routes
	_ = keys.Invites
	_ = keys.Status
	_ = keys.Logs
	_ = keys.NewInvite
	_ = keys.Accept
	_ = keys.Connect
}
