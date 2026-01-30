package tui

import (
	"strings"
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

func TestPeerStateStyle(t *testing.T) {
	tests := []struct {
		state    string
		expected string
	}{
		{"Connected", "Success"},
		{"Pending", "Warning"},
		{"Disconnected", "Error"},
		{"Rejected", "Error"},
		{"Unknown", "Muted"},
		{"", "Muted"},
	}

	for _, tc := range tests {
		style := PeerStateStyle(tc.state)
		// Just verify it returns a style without panicking
		_ = style.Render("test")
	}
}

func TestPeersModelUpdate(t *testing.T) {
	m := NewPeersModel()
	m.SetData(&rpc.PeersListResult{
		Peers: []rpc.PeerInfo{
			{NodeID: "node1", TunnelIP: "10.0.0.1", State: "Connected"},
			{NodeID: "node2", TunnelIP: "10.0.0.2", State: "Connected"},
			{NodeID: "node3", TunnelIP: "10.0.0.3", State: "Connected"},
		},
		Total: 3,
	})

	t.Run("move cursor down", func(t *testing.T) {
		m.cursor = 0
		m2, _ := m.Update(fakeKeyMsgWithType(tea.KeyDown), nil)
		if m2.cursor != 1 {
			t.Errorf("cursor = %d, want 1", m2.cursor)
		}
	})

	t.Run("move cursor up", func(t *testing.T) {
		m.cursor = 2
		m2, _ := m.Update(fakeKeyMsgWithType(tea.KeyUp), nil)
		if m2.cursor != 1 {
			t.Errorf("cursor = %d, want 1", m2.cursor)
		}
	})

	t.Run("cursor stays at bottom", func(t *testing.T) {
		m.cursor = 2
		m2, _ := m.Update(fakeKeyMsgWithType(tea.KeyDown), nil)
		if m2.cursor != 2 {
			t.Errorf("cursor = %d, want 2", m2.cursor)
		}
	})

	t.Run("cursor stays at top", func(t *testing.T) {
		m.cursor = 0
		m2, _ := m.Update(fakeKeyMsgWithType(tea.KeyUp), nil)
		if m2.cursor != 0 {
			t.Errorf("cursor = %d, want 0", m2.cursor)
		}
	})
}

func TestPeersModelView(t *testing.T) {
	t.Run("loading state", func(t *testing.T) {
		m := NewPeersModel()
		view := m.View()
		if !strings.Contains(view, "Loading") {
			t.Errorf("expected loading message, got: %s", view)
		}
	})

	t.Run("empty state", func(t *testing.T) {
		m := NewPeersModel()
		m.SetDimensions(80, 24)
		m.SetData(&rpc.PeersListResult{
			Peers: []rpc.PeerInfo{},
			Total: 0,
		})
		view := m.View()
		if !strings.Contains(view, "No Peers") {
			t.Errorf("expected empty state message, got: %s", view)
		}
	})

	t.Run("with peers", func(t *testing.T) {
		m := NewPeersModel()
		m.SetDimensions(100, 24)
		m.SetData(&rpc.PeersListResult{
			Peers: []rpc.PeerInfo{
				{NodeID: "node1", TunnelIP: "10.0.0.1", State: "Connected", LastSeen: "1m ago"},
			},
			Total: 1,
		})
		view := m.View()
		if !strings.Contains(view, "node1") {
			t.Errorf("expected node1 in view, got: %s", view)
		}
		if !strings.Contains(view, "10.0.0.1") {
			t.Errorf("expected tunnel IP in view, got: %s", view)
		}
	})
}

func TestPeersModelSetDimensions(t *testing.T) {
	m := NewPeersModel()
	m.SetDimensions(100, 50)

	if m.width != 100 {
		t.Errorf("width = %d, want 100", m.width)
	}
	if m.height != 50 {
		t.Errorf("height = %d, want 50", m.height)
	}
}

func TestRoutesModelView(t *testing.T) {
	t.Run("loading state", func(t *testing.T) {
		m := NewRoutesModel()
		view := m.View()
		if !strings.Contains(view, "Loading") {
			t.Errorf("expected loading message, got: %s", view)
		}
	})

	t.Run("empty state", func(t *testing.T) {
		m := NewRoutesModel()
		m.SetDimensions(80, 24)
		m.SetData(&rpc.RoutesListResult{
			Routes: []rpc.RouteInfo{},
			Total:  0,
		})
		view := m.View()
		if !strings.Contains(view, "No Routes") {
			t.Errorf("expected empty state message, got: %s", view)
		}
	})

	t.Run("with routes", func(t *testing.T) {
		m := NewRoutesModel()
		m.SetDimensions(100, 24)
		m.SetData(&rpc.RoutesListResult{
			Routes: []rpc.RouteInfo{
				{NodeID: "node1", TunnelIP: "10.0.0.1", HopCount: 1},
			},
			Total: 1,
		})
		view := m.View()
		if !strings.Contains(view, "node1") {
			t.Errorf("expected node1 in view, got: %s", view)
		}
	})
}

func TestRoutesModelUpdate(t *testing.T) {
	m := NewRoutesModel()
	m.SetData(&rpc.RoutesListResult{
		Routes: []rpc.RouteInfo{
			{NodeID: "node1", TunnelIP: "10.0.0.1", HopCount: 1},
			{NodeID: "node2", TunnelIP: "10.0.0.2", HopCount: 2},
		},
		Total: 2,
	})

	t.Run("move cursor down", func(t *testing.T) {
		m.cursor = 0
		m2, _ := m.Update(fakeKeyMsgWithType(tea.KeyDown))
		if m2.cursor != 1 {
			t.Errorf("cursor = %d, want 1", m2.cursor)
		}
	})

	t.Run("move cursor up", func(t *testing.T) {
		m.cursor = 1
		m2, _ := m.Update(fakeKeyMsgWithType(tea.KeyUp))
		if m2.cursor != 0 {
			t.Errorf("cursor = %d, want 0", m2.cursor)
		}
	})
}

func TestRoutesModelSetDimensions(t *testing.T) {
	m := NewRoutesModel()
	m.SetDimensions(100, 50)

	if m.width != 100 {
		t.Errorf("width = %d, want 100", m.width)
	}
	if m.height != 50 {
		t.Errorf("height = %d, want 50", m.height)
	}
}

func TestStatusModelView(t *testing.T) {
	t.Run("loading state", func(t *testing.T) {
		m := NewStatusModel()
		view := m.View()
		if !strings.Contains(view, "Loading") {
			t.Errorf("expected loading message, got: %s", view)
		}
	})

	t.Run("with status", func(t *testing.T) {
		m := NewStatusModel()
		m.SetDimensions(100, 24)
		m.SetData(&rpc.StatusResult{
			NodeName:       "test-node",
			NodeID:         "abc123",
			State:          "running",
			TunnelIP:       "10.0.0.1",
			PeerCount:      5,
			Uptime:         "2h",
			Version:        "1.0.0",
			I2PDestination: "dest.i2p",
		})
		view := m.View()
		if !strings.Contains(view, "test-node") {
			t.Errorf("expected node name in view, got: %s", view)
		}
		if !strings.Contains(view, "running") {
			t.Errorf("expected state in view, got: %s", view)
		}
	})
}

func TestStatusModelSetDimensions(t *testing.T) {
	m := NewStatusModel()
	m.SetDimensions(100, 50)

	if m.width != 100 {
		t.Errorf("width = %d, want 100", m.width)
	}
	if m.height != 50 {
		t.Errorf("height = %d, want 50", m.height)
	}
}

func TestInvitesModelView(t *testing.T) {
	t.Run("normal mode", func(t *testing.T) {
		m := NewInvitesModel()
		m.SetDimensions(80, 24)
		view := m.View()
		if view == "" {
			t.Error("expected non-empty view")
		}
	})

	t.Run("with created invite", func(t *testing.T) {
		m := NewInvitesModel()
		m.SetDimensions(80, 24)
		m.SetCreatedInvite(&rpc.InviteCreateResult{
			InviteCode: "i2plan://testcode123",
			ExpiresAt:  "2024-01-01T00:00:00Z",
			MaxUses:    5,
		})
		view := m.View()
		if !strings.Contains(view, "testcode123") {
			t.Errorf("expected invite code in view, got: %s", view)
		}
	})
}

func TestInvitesModelSetDimensions(t *testing.T) {
	m := NewInvitesModel()
	m.SetDimensions(100, 50)

	if m.width != 100 {
		t.Errorf("width = %d, want 100", m.width)
	}
	if m.height != 50 {
		t.Errorf("height = %d, want 50", m.height)
	}
}

func TestInvitesModelModes(t *testing.T) {
	t.Run("initial mode is normal", func(t *testing.T) {
		m := NewInvitesModel()
		if m.mode != InvitesModeNormal {
			t.Errorf("mode = %d, want %d", m.mode, InvitesModeNormal)
		}
	})

	t.Run("mode constants", func(t *testing.T) {
		if InvitesModeNormal != 0 {
			t.Errorf("InvitesModeNormal = %d, want 0", InvitesModeNormal)
		}
		if InvitesModeCreate != 1 {
			t.Errorf("InvitesModeCreate = %d, want 1", InvitesModeCreate)
		}
		if InvitesModeAccept != 2 {
			t.Errorf("InvitesModeAccept = %d, want 2", InvitesModeAccept)
		}
	})
}

func TestKeyMapBindings(t *testing.T) {
	// Verify all keys have valid bindings
	testCases := []struct {
		name    string
		binding interface{}
	}{
		{"Quit", keys.Quit},
		{"Tab", keys.Tab},
		{"ShiftTab", keys.ShiftTab},
		{"Refresh", keys.Refresh},
		{"Up", keys.Up},
		{"Down", keys.Down},
		{"Enter", keys.Enter},
		{"Escape", keys.Escape},
		{"Peers", keys.Peers},
		{"Routes", keys.Routes},
		{"Invites", keys.Invites},
		{"Status", keys.Status},
		{"Logs", keys.Logs},
		{"NewInvite", keys.NewInvite},
		{"Accept", keys.Accept},
		{"Connect", keys.Connect},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.binding == nil {
				t.Errorf("binding %s is nil", tc.name)
			}
		})
	}
}

func TestStylesRendering(t *testing.T) {
	// Test that styles can render without panicking
	testCases := []struct {
		name  string
		style interface{ Render(...string) string }
	}{
		{"Title", styles.Title},
		{"TabActive", styles.TabActive},
		{"TabInactive", styles.TabInactive},
		{"Error", styles.Error},
		{"Success", styles.Success},
		{"Warning", styles.Warning},
		{"Muted", styles.Muted},
		{"TableHeader", styles.TableHeader},
		{"TableRow", styles.TableRow},
		{"Selected", styles.Selected},
		{"BoxTitle", styles.BoxTitle},
		{"Bold", styles.Bold},
		{"Input", styles.Input},
		{"InputFocus", styles.InputFocus},
		{"Box", styles.Box},
		{"HelpText", styles.HelpText},
		{"StatusText", styles.StatusText},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rendered := tc.style.Render("test content")
			if rendered == "" {
				t.Errorf("style %s rendered empty string", tc.name)
			}
		})
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := Config{
		RPCSocketPath:   "/tmp/test.sock",
		RPCAuthFile:     "/tmp/auth.token",
		RefreshInterval: 0,
	}

	if cfg.RPCSocketPath != "/tmp/test.sock" {
		t.Errorf("RPCSocketPath = %q, want %q", cfg.RPCSocketPath, "/tmp/test.sock")
	}
}
