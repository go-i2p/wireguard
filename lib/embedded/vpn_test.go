package embedded

import (
	"context"
	"testing"
	"time"
)

func TestNew_DefaultConfig(t *testing.T) {
	vpn, err := New(Config{})
	if err != nil {
		t.Fatalf("New with default config failed: %v", err)
	}
	if vpn == nil {
		t.Fatal("New returned nil VPN")
	}
	defer vpn.Close()

	if vpn.State() != StateInitial {
		t.Errorf("expected state Initial, got %s", vpn.State())
	}
}

func TestNew_CustomConfig(t *testing.T) {
	cfg := Config{
		NodeName:     "test-node",
		DataDir:      t.TempDir(),
		SAMAddress:   "127.0.0.1:7656",
		TunnelSubnet: "10.42.0.0/16",
		MaxPeers:     100,
	}

	vpn, err := New(cfg)
	if err != nil {
		t.Fatalf("New with custom config failed: %v", err)
	}
	defer vpn.Close()

	gotCfg := vpn.Config()
	if gotCfg.NodeName != "test-node" {
		t.Errorf("expected node name test-node, got %s", gotCfg.NodeName)
	}
	if gotCfg.MaxPeers != 100 {
		t.Errorf("expected max peers 100, got %d", gotCfg.MaxPeers)
	}
}

func TestNewWithOptions(t *testing.T) {
	vpn, err := NewWithOptions(
		WithNodeName("options-test"),
		WithDataDir(t.TempDir()),
		WithMaxPeers(25),
	)
	if err != nil {
		t.Fatalf("NewWithOptions failed: %v", err)
	}
	defer vpn.Close()

	cfg := vpn.Config()
	if cfg.NodeName != "options-test" {
		t.Errorf("expected node name options-test, got %s", cfg.NodeName)
	}
	if cfg.MaxPeers != 25 {
		t.Errorf("expected max peers 25, got %d", cfg.MaxPeers)
	}
}

func TestNew_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "empty config uses defaults",
			cfg:     Config{},
			wantErr: false,
		},
		{
			name: "invalid tunnel length",
			cfg: Config{
				NodeName:     "test",
				DataDir:      "/tmp/test",
				TunnelLength: 10, // Invalid: must be 0-7
			},
			wantErr: true,
		},
		{
			name: "invalid max peers",
			cfg: Config{
				NodeName: "test",
				DataDir:  "/tmp/test",
				MaxPeers: -1, // Invalid
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vpn, err := New(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					if vpn != nil {
						vpn.Close()
					}
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if vpn != nil {
					vpn.Close()
				}
			}
		})
	}
}

func TestVPN_StateTransitions(t *testing.T) {
	vpn, err := New(Config{
		DataDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer vpn.Close()

	// Initial state
	if vpn.State() != StateInitial {
		t.Errorf("expected Initial state, got %s", vpn.State())
	}

	// Cannot stop before starting
	ctx := context.Background()
	err = vpn.Stop(ctx)
	if err == nil {
		t.Error("Stop should fail when not running")
	}

	// Start the VPN
	err = vpn.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if vpn.State() != StateRunning {
		t.Errorf("expected Running state, got %s", vpn.State())
	}

	// Cannot start twice
	err = vpn.Start(ctx)
	if err == nil {
		t.Error("Second Start should fail")
	}

	// Stop the VPN
	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err = vpn.Stop(stopCtx)
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	if vpn.State() != StateStopped {
		t.Errorf("expected Stopped state, got %s", vpn.State())
	}
}

func TestVPN_Status(t *testing.T) {
	vpn, err := New(Config{
		NodeName: "status-test",
		DataDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer vpn.Close()

	status := vpn.Status()
	if status.State != StateInitial {
		t.Errorf("expected Initial state, got %s", status.State)
	}
	if status.NodeName != "status-test" {
		t.Errorf("expected node name status-test, got %s", status.NodeName)
	}
	if status.Uptime != 0 {
		t.Errorf("expected zero uptime before start, got %v", status.Uptime)
	}

	// Start and check uptime
	if err := vpn.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	status = vpn.Status()

	if status.State != StateRunning {
		t.Errorf("expected Running state, got %s", status.State)
	}
	if status.Uptime < 100*time.Millisecond {
		t.Errorf("expected uptime >= 100ms, got %v", status.Uptime)
	}
	if status.StartedAt.IsZero() {
		t.Error("StartedAt should not be zero after start")
	}
}

func TestVPN_Events(t *testing.T) {
	vpn, err := New(Config{
		DataDir:         t.TempDir(),
		EventBufferSize: 10,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer vpn.Close()

	events := vpn.Events()

	// Start VPN to generate events
	if err := vpn.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Should receive state change and started events
	receivedEvents := make([]Event, 0)
	timeout := time.After(1 * time.Second)

collectEvents:
	for {
		select {
		case event, ok := <-events:
			if !ok {
				break collectEvents
			}
			receivedEvents = append(receivedEvents, event)
			if len(receivedEvents) >= 3 {
				break collectEvents
			}
		case <-timeout:
			break collectEvents
		}
	}

	if len(receivedEvents) < 2 {
		t.Errorf("expected at least 2 events, got %d", len(receivedEvents))
	}

	// Check for expected event types
	hasStateChange := false
	hasStarted := false
	for _, e := range receivedEvents {
		if e.Type == EventStateChanged {
			hasStateChange = true
		}
		if e.Type == EventStarted {
			hasStarted = true
		}
	}

	if !hasStateChange {
		t.Error("expected EventStateChanged event")
	}
	if !hasStarted {
		t.Error("expected EventStarted event")
	}
}

func TestVPN_Done(t *testing.T) {
	vpn, err := New(Config{
		DataDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	if err := vpn.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	done := vpn.Done()

	// Done should not be closed yet
	select {
	case <-done:
		t.Error("Done channel should not be closed while running")
	default:
		// Good
	}

	// Stop the VPN
	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	vpn.Stop(stopCtx)

	// Done should be closed now
	select {
	case <-done:
		// Good
	case <-time.After(1 * time.Second):
		t.Error("Done channel should be closed after stop")
	}
}

func TestVPN_RestartAfterStop(t *testing.T) {
	vpn, err := New(Config{
		DataDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer vpn.Close()

	ctx := context.Background()

	// First cycle
	if err := vpn.Start(ctx); err != nil {
		t.Fatalf("First Start failed: %v", err)
	}

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	if err := vpn.Stop(stopCtx); err != nil {
		t.Fatalf("First Stop failed: %v", err)
	}
	cancel()

	// Second cycle
	if err := vpn.Start(ctx); err != nil {
		t.Fatalf("Second Start failed: %v", err)
	}

	if vpn.State() != StateRunning {
		t.Errorf("expected Running state after restart, got %s", vpn.State())
	}
}

func TestVPN_CloseIdempotent(t *testing.T) {
	vpn, err := New(Config{
		DataDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Close before start should work
	err = vpn.Close()
	if err != nil {
		t.Errorf("Close before start should not error: %v", err)
	}

	// Start fresh VPN
	vpn2, _ := New(Config{
		DataDir: t.TempDir(),
	})
	vpn2.Start(context.Background())

	// Multiple closes should not panic
	vpn2.Close()
	vpn2.Close() // Should not panic
}

func TestVPN_PeersWhenNotRunning(t *testing.T) {
	vpn, err := New(Config{
		DataDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer vpn.Close()

	// Should return empty, not panic
	peers := vpn.Peers()
	if len(peers) > 0 {
		t.Error("expected empty peers when not running")
	}

	routes := vpn.Routes()
	if len(routes) > 0 {
		t.Error("expected empty routes when not running")
	}
}

func TestConfig_ApplyDefaults(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()

	if cfg.NodeName == "" {
		t.Error("NodeName should have default")
	}
	if cfg.DataDir == "" {
		t.Error("DataDir should have default")
	}
	if cfg.SAMAddress != DefaultSAMAddress {
		t.Errorf("expected SAM address %s, got %s", DefaultSAMAddress, cfg.SAMAddress)
	}
	if cfg.TunnelSubnet != DefaultTunnelSubnet {
		t.Errorf("expected tunnel subnet %s, got %s", DefaultTunnelSubnet, cfg.TunnelSubnet)
	}
	if cfg.MaxPeers != 50 {
		t.Errorf("expected max peers 50, got %d", cfg.MaxPeers)
	}
	if cfg.EventBufferSize != 100 {
		t.Errorf("expected event buffer size 100, got %d", cfg.EventBufferSize)
	}
}

func TestEventType_String(t *testing.T) {
	tests := []struct {
		eventType EventType
		want      string
	}{
		{EventStarted, "started"},
		{EventStopped, "stopped"},
		{EventPeerConnected, "peer_connected"},
		{EventPeerDisconnected, "peer_disconnected"},
		{EventError, "error"},
		{EventType(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.eventType.String()
			if got != tt.want {
				t.Errorf("expected %s, got %s", tt.want, got)
			}
		})
	}
}
