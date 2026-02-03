package core

import (
	"context"
	"net/netip"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestNewExitClient(t *testing.T) {
	tests := []struct {
		name    string
		config  ClientExitConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config without kill switch",
			config: ClientExitConfig{
				Enabled:    true,
				KillSwitch: false,
			},
			wantErr: false,
		},
		{
			name: "valid config with kill switch",
			config: ClientExitConfig{
				Enabled:    true,
				KillSwitch: true,
			},
			wantErr: false, // May skip if firewall tools not available
		},
		{
			name: "disabled config",
			config: ClientExitConfig{
				Enabled: false,
			},
			wantErr: true,
			errMsg:  "not enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ec, err := NewExitClient(tt.config)
			if (err != nil) != tt.wantErr {
				// Special handling for firewall unavailable
				if tt.config.KillSwitch && err != nil && contains(err.Error(), "not available") {
					t.Skipf("Firewall not available: %v", err)
					return
				}
				t.Errorf("NewExitClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
				t.Errorf("NewExitClient() error = %v, want error containing %q", err, tt.errMsg)
			}
			if !tt.wantErr && ec == nil {
				t.Error("NewExitClient() returned nil without error")
			}
			if !tt.wantErr && ec != nil {
				// Verify platform-specific manager was created
				if ec.routeManager == nil {
					t.Error("route manager not initialized")
				}
				if tt.config.KillSwitch && ec.killSwitch == nil {
					t.Error("kill switch not initialized despite being enabled")
				}
			}
		})
	}
}

func TestNewExitClient_PlatformSupport(t *testing.T) {
	config := ClientExitConfig{
		Enabled:    true,
		KillSwitch: false,
	}

	ec, err := NewExitClient(config)
	if err != nil {
		// Some platforms may not be supported
		t.Logf("Platform %s: %v", runtime.GOOS, err)
		return
	}

	if ec == nil {
		t.Error("NewExitClient() returned nil without error")
	}

	t.Logf("Platform %s: successfully created exit client", runtime.GOOS)
}

func TestExitClient_IsActive(t *testing.T) {
	config := ClientExitConfig{
		Enabled:    true,
		KillSwitch: false,
	}

	ec, err := NewExitClient(config)
	if err != nil {
		t.Skipf("Cannot create exit client on %s: %v", runtime.GOOS, err)
	}

	// Should be inactive initially
	if ec.IsActive() {
		t.Error("IsActive() = true, want false for new exit client")
	}

	// Test concurrent access
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = ec.IsActive()
		}()
	}
	wg.Wait()
}

func TestExitClient_Start_InvalidIP(t *testing.T) {
	config := ClientExitConfig{
		Enabled:    true,
		KillSwitch: false,
	}

	ec, err := NewExitClient(config)
	if err != nil {
		t.Skipf("Cannot create exit client on %s: %v", runtime.GOOS, err)
	}

	tests := []struct {
		name   string
		meshIP string
		errMsg string
	}{
		{
			name:   "empty IP",
			meshIP: "",
			errMsg: "invalid",
		},
		{
			name:   "invalid format",
			meshIP: "not-an-ip",
			errMsg: "invalid",
		},
		{
			name:   "malformed IP",
			meshIP: "256.1.1.1",
			errMsg: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ec.Start(tt.meshIP)
			if err == nil {
				t.Error("Start() expected error, got nil")
				return
			}
			if !contains(err.Error(), tt.errMsg) {
				t.Errorf("Start() error = %v, want error containing %q", err, tt.errMsg)
			}
		})
	}
}

func TestExitClient_Stop_Idempotent(t *testing.T) {
	config := ClientExitConfig{
		Enabled:    true,
		KillSwitch: false,
	}

	ec, err := NewExitClient(config)
	if err != nil {
		t.Skipf("Cannot create exit client on %s: %v", runtime.GOOS, err)
	}

	// Stop should succeed even when not active
	if err := ec.Stop(); err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}

	// Multiple stops should not error
	if err := ec.Stop(); err != nil {
		t.Errorf("Stop() second call error = %v, want nil", err)
	}
}

func TestExitClient_ConcurrentOperations(t *testing.T) {
	config := ClientExitConfig{
		Enabled:    true,
		KillSwitch: false,
	}

	ec, err := NewExitClient(config)
	if err != nil {
		t.Skipf("Cannot create exit client on %s: %v", runtime.GOOS, err)
	}

	// Test concurrent access to exit client
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = ec.IsActive()
			_ = ec.Stop()
		}()
	}
	wg.Wait()
}

func TestKillSwitch_EnableDisable(t *testing.T) {
	config := ClientExitConfig{
		Enabled:    true,
		KillSwitch: true,
	}

	ec, err := NewExitClient(config)
	if err != nil {
		t.Skipf("Cannot create exit client with kill switch on %s: %v", runtime.GOOS, err)
	}

	if ec.killSwitch == nil {
		t.Fatal("kill switch is nil despite being enabled")
	}

	// Disable should succeed even when not enabled
	if err := ec.killSwitch.Disable(); err != nil {
		t.Errorf("Disable() error = %v, want nil", err)
	}

	// Multiple disables should not error
	if err := ec.killSwitch.Disable(); err != nil {
		t.Errorf("Disable() second call error = %v, want nil", err)
	}
}

func TestKillSwitch_ConcurrentAccess(t *testing.T) {
	config := ClientExitConfig{
		Enabled:    true,
		KillSwitch: true,
	}

	ec, err := NewExitClient(config)
	if err != nil {
		t.Skipf("Cannot create exit client with kill switch on %s: %v", runtime.GOOS, err)
	}

	if ec.killSwitch == nil {
		t.Fatal("kill switch is nil despite being enabled")
	}

	// Test concurrent access to kill switch
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = ec.killSwitch.Disable()
		}()
	}
	wg.Wait()
}

func TestExitClient_ExcludeRoutes(t *testing.T) {
	config := ClientExitConfig{
		Enabled:       true,
		KillSwitch:    false,
		ExcludeRoutes: []string{"192.168.1.0/24", "10.10.0.0/16"},
	}

	ec, err := NewExitClient(config)
	if err != nil {
		t.Skipf("Cannot create exit client on %s: %v", runtime.GOOS, err)
	}

	// Verify config was stored
	if len(ec.config.ExcludeRoutes) != 2 {
		t.Errorf("ExcludeRoutes count = %d, want 2", len(ec.config.ExcludeRoutes))
	}
}

// Platform-specific tests

func TestRouteManager_GetDefaultGateway(t *testing.T) {
	rm, err := newPlatformRouteManager()
	if err != nil {
		t.Skipf("Cannot create route manager on %s: %v", runtime.GOOS, err)
	}

	gw, err := rm.GetDefaultGateway()
	if err != nil {
		t.Logf("GetDefaultGateway() error (acceptable in test env): %v", err)
		return
	}

	if !gw.IsValid() {
		t.Error("GetDefaultGateway() returned invalid address")
	}

	t.Logf("Platform %s default gateway: %s", runtime.GOOS, gw)
}

func TestPlatformManagers(t *testing.T) {
	t.Run("RouteManager", func(t *testing.T) {
		rm, err := newPlatformRouteManager()
		if err != nil {
			t.Errorf("newPlatformRouteManager() on %s: %v", runtime.GOOS, err)
			return
		}
		if rm == nil {
			t.Error("newPlatformRouteManager() returned nil without error")
		}
		t.Logf("Platform %s: route manager created successfully", runtime.GOOS)
	})

	t.Run("FirewallManager", func(t *testing.T) {
		fm, err := newPlatformFirewallManager()
		if err != nil {
			t.Errorf("newPlatformFirewallManager() on %s: %v", runtime.GOOS, err)
			return
		}
		if fm == nil {
			t.Error("newPlatformFirewallManager() returned nil without error")
		}
		t.Logf("Platform %s: firewall manager created successfully", runtime.GOOS)
	})
}

func TestExitClient_HealthCheck_NotActive(t *testing.T) {
	config := ClientExitConfig{
		Enabled: true,
	}

	ec, err := NewExitClient(config)
	if err != nil {
		t.Fatalf("NewExitClient() failed: %v", err)
	}

	ctx := context.Background()
	err = ec.HealthCheck(ctx)
	if err == nil {
		t.Error("HealthCheck() should fail when client is not active")
	}
	if !contains(err.Error(), "not active") {
		t.Errorf("expected 'not active' error, got: %v", err)
	}
}

func TestExitClient_PingExitNode(t *testing.T) {
	config := ClientExitConfig{
		Enabled: true,
	}

	ec, err := NewExitClient(config)
	if err != nil {
		t.Fatalf("NewExitClient() failed: %v", err)
	}

	ctx := context.Background()

	// Test unreachable IP (using TEST-NET-1 from RFC 5737)
	unreachableIP := netip.MustParseAddr("192.0.2.1")
	err = ec.pingExitNode(ctx, unreachableIP)
	if err == nil {
		t.Error("pingExitNode() should fail for unreachable IP")
	}

	// Test with localhost (should succeed if port 443 is open or fail gracefully)
	localhostIP := netip.MustParseAddr("127.0.0.1")
	err = ec.pingExitNode(ctx, localhostIP)
	// We expect this to fail since localhost:443 is unlikely to be listening
	// but we're testing the function doesn't panic or hang
	if err != nil {
		t.Logf("pingExitNode(localhost) failed as expected: %v", err)
	}
}

func TestExitClient_TestInternetAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping internet access test in short mode")
	}

	config := ClientExitConfig{
		Enabled: true,
	}

	ec, err := NewExitClient(config)
	if err != nil {
		t.Fatalf("NewExitClient() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = ec.testInternetAccess(ctx)
	if err != nil {
		// Internet access test may fail in CI/test environments
		// Log but don't fail the test
		t.Logf("testInternetAccess() failed (may be expected in test environment): %v", err)
	} else {
		t.Log("testInternetAccess() succeeded")
	}
}

func TestExitClient_MonitorConnection(t *testing.T) {
	config := ClientExitConfig{
		Enabled: true,
	}

	ec, err := NewExitClient(config)
	if err != nil {
		t.Fatalf("NewExitClient() failed: %v", err)
	}

	// Start monitoring with very short interval
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// MonitorConnection should return when context is canceled
	done := make(chan struct{})
	go func() {
		ec.MonitorConnection(ctx, 100*time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
		// Success - monitoring stopped when context canceled
	case <-time.After(2 * time.Second):
		t.Error("MonitorConnection() did not stop when context was canceled")
	}
}

func TestExitClient_MonitorConnection_DefaultInterval(t *testing.T) {
	config := ClientExitConfig{
		Enabled: true,
	}

	ec, err := NewExitClient(config)
	if err != nil {
		t.Fatalf("NewExitClient() failed: %v", err)
	}

	// Test with zero interval (should use default)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		ec.MonitorConnection(ctx, 0) // zero interval
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("MonitorConnection() with zero interval did not stop")
	}
}

// Helper functions

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || anyMatch(s, substr))
}

func anyMatch(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
