package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Node.Name == "" {
		t.Error("default config should have a node name")
	}
	if cfg.Node.DataDir == "" {
		t.Error("default config should have a data directory")
	}
	if cfg.I2P.SAMAddress == "" {
		t.Error("default config should have a SAM address")
	}
	if cfg.Mesh.TunnelSubnet == "" {
		t.Error("default config should have a tunnel subnet")
	}
	if cfg.Mesh.ShutdownTimeout != DefaultShutdownTimeout {
		t.Errorf("default config should have ShutdownTimeout=%v, got %v",
			DefaultShutdownTimeout, cfg.Mesh.ShutdownTimeout)
	}
	if cfg.Mesh.DrainTimeout != DefaultDrainTimeout {
		t.Errorf("default config should have DrainTimeout=%v, got %v",
			DefaultDrainTimeout, cfg.Mesh.DrainTimeout)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "valid default config",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name:    "empty node name",
			modify:  func(c *Config) { c.Node.Name = "" },
			wantErr: true,
		},
		{
			name:    "empty data dir",
			modify:  func(c *Config) { c.Node.DataDir = "" },
			wantErr: true,
		},
		{
			name:    "empty SAM address",
			modify:  func(c *Config) { c.I2P.SAMAddress = "" },
			wantErr: true,
		},
		{
			name:    "tunnel length too high",
			modify:  func(c *Config) { c.I2P.TunnelLength = 10 },
			wantErr: true,
		},
		{
			name:    "tunnel length negative",
			modify:  func(c *Config) { c.I2P.TunnelLength = -1 },
			wantErr: true,
		},
		{
			name:    "empty tunnel subnet",
			modify:  func(c *Config) { c.Mesh.TunnelSubnet = "" },
			wantErr: true,
		},
		{
			name:    "max peers zero",
			modify:  func(c *Config) { c.Mesh.MaxPeers = 0 },
			wantErr: true,
		},
		{
			name:    "shutdown timeout too short",
			modify:  func(c *Config) { c.Mesh.ShutdownTimeout = 500 * time.Millisecond },
			wantErr: true,
		},
		{
			name:    "shutdown timeout zero",
			modify:  func(c *Config) { c.Mesh.ShutdownTimeout = 0 },
			wantErr: true,
		},
		{
			name:    "shutdown timeout exactly 1 second",
			modify:  func(c *Config) { c.Mesh.ShutdownTimeout = time.Second },
			wantErr: false,
		},
		{
			name:    "shutdown timeout extended for slow I2P",
			modify:  func(c *Config) { c.Mesh.ShutdownTimeout = 30 * time.Second },
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadConfig_DefaultsWhenMissing(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nonexistent.toml")

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig should not error on missing file: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadConfig should return default config when file is missing")
	}
	if cfg.Node.Name == "" {
		t.Error("should have default node name")
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	// Create a custom config
	original := DefaultConfig()
	original.Node.Name = "test-node"
	original.I2P.TunnelLength = 3
	original.Mesh.HeartbeatInterval = 60 * time.Second
	original.Mesh.ShutdownTimeout = 15 * time.Second // Custom timeout for slow I2P

	// Save it
	if err := SaveConfig(original, configPath); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Load it back
	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify values
	if loaded.Node.Name != original.Node.Name {
		t.Errorf("node name mismatch: got %q, want %q", loaded.Node.Name, original.Node.Name)
	}
	if loaded.I2P.TunnelLength != original.I2P.TunnelLength {
		t.Errorf("tunnel length mismatch: got %d, want %d", loaded.I2P.TunnelLength, original.I2P.TunnelLength)
	}
	if loaded.Mesh.HeartbeatInterval != original.Mesh.HeartbeatInterval {
		t.Errorf("heartbeat interval mismatch: got %v, want %v", loaded.Mesh.HeartbeatInterval, original.Mesh.HeartbeatInterval)
	}
	if loaded.Mesh.ShutdownTimeout != original.Mesh.ShutdownTimeout {
		t.Errorf("shutdown timeout mismatch: got %v, want %v", loaded.Mesh.ShutdownTimeout, original.Mesh.ShutdownTimeout)
	}
}

func TestLoadConfig_InvalidTOML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.toml")

	// Write invalid TOML
	if err := os.WriteFile(configPath, []byte("this is not [valid toml"), 0o600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Error("LoadConfig should error on invalid TOML")
	}
}

func TestConfig_DataPath(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Node.DataDir = "/home/user/.i2plan"

	path := cfg.DataPath("identity.json")
	expected := "/home/user/.i2plan/identity.json"
	if path != expected {
		t.Errorf("DataPath mismatch: got %q, want %q", path, expected)
	}

	path = cfg.DataPath("peers", "cache.json")
	expected = "/home/user/.i2plan/peers/cache.json"
	if path != expected {
		t.Errorf("DataPath mismatch: got %q, want %q", path, expected)
	}
}

func TestConfig_EnsureDataDir(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Node.DataDir = filepath.Join(tmpDir, "new", "data", "dir")

	if err := cfg.EnsureDataDir(); err != nil {
		t.Fatalf("EnsureDataDir failed: %v", err)
	}

	info, err := os.Stat(cfg.Node.DataDir)
	if err != nil {
		t.Fatalf("data dir does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("data path is not a directory")
	}
}

func TestSaveConfig_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "new", "nested", "config.toml")

	cfg := DefaultConfig()
	if err := SaveConfig(cfg, configPath); err != nil {
		t.Fatalf("SaveConfig failed to create nested directory: %v", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		validate func(*testing.T, *Config)
	}{
		{
			name: "node configuration overrides",
			envVars: map[string]string{
				"I2PLAN_NODE_NAME": "test-node",
				"I2PLAN_DATA_DIR":  "/custom/data",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.Node.Name != "test-node" {
					t.Errorf("Node.Name = %q, want %q", cfg.Node.Name, "test-node")
				}
				if cfg.Node.DataDir != "/custom/data" {
					t.Errorf("Node.DataDir = %q, want %q", cfg.Node.DataDir, "/custom/data")
				}
			},
		},
		{
			name: "I2P configuration overrides",
			envVars: map[string]string{
				"I2PLAN_SAM_ADDRESS":   "192.168.1.1:7656",
				"I2PLAN_TUNNEL_LENGTH": "3",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.I2P.SAMAddress != "192.168.1.1:7656" {
					t.Errorf("I2P.SAMAddress = %q, want %q", cfg.I2P.SAMAddress, "192.168.1.1:7656")
				}
				if cfg.I2P.TunnelLength != 3 {
					t.Errorf("I2P.TunnelLength = %d, want %d", cfg.I2P.TunnelLength, 3)
				}
			},
		},
		{
			name: "mesh configuration overrides",
			envVars: map[string]string{
				"I2PLAN_TUNNEL_SUBNET":      "172.16.0.0/12",
				"I2PLAN_HEARTBEAT_INTERVAL": "60",
				"I2PLAN_PEER_TIMEOUT":       "600",
				"I2PLAN_MAX_PEERS":          "100",
				"I2PLAN_SHUTDOWN_TIMEOUT":   "10",
				"I2PLAN_DRAIN_TIMEOUT":      "20",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.Mesh.TunnelSubnet != "172.16.0.0/12" {
					t.Errorf("Mesh.TunnelSubnet = %q, want %q", cfg.Mesh.TunnelSubnet, "172.16.0.0/12")
				}
				if cfg.Mesh.HeartbeatInterval != 60*time.Second {
					t.Errorf("Mesh.HeartbeatInterval = %v, want %v", cfg.Mesh.HeartbeatInterval, 60*time.Second)
				}
				if cfg.Mesh.PeerTimeout != 600*time.Second {
					t.Errorf("Mesh.PeerTimeout = %v, want %v", cfg.Mesh.PeerTimeout, 600*time.Second)
				}
				if cfg.Mesh.MaxPeers != 100 {
					t.Errorf("Mesh.MaxPeers = %d, want %d", cfg.Mesh.MaxPeers, 100)
				}
				if cfg.Mesh.ShutdownTimeout != 10*time.Second {
					t.Errorf("Mesh.ShutdownTimeout = %v, want %v", cfg.Mesh.ShutdownTimeout, 10*time.Second)
				}
				if cfg.Mesh.DrainTimeout != 20*time.Second {
					t.Errorf("Mesh.DrainTimeout = %v, want %v", cfg.Mesh.DrainTimeout, 20*time.Second)
				}
			},
		},
		{
			name: "RPC configuration overrides",
			envVars: map[string]string{
				"I2PLAN_RPC_ENABLED":     "false",
				"I2PLAN_RPC_SOCKET":      "custom.sock",
				"I2PLAN_RPC_TCP_ADDRESS": "0.0.0.0:9090",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.RPC.Enabled != false {
					t.Errorf("RPC.Enabled = %v, want %v", cfg.RPC.Enabled, false)
				}
				if cfg.RPC.Socket != "custom.sock" {
					t.Errorf("RPC.Socket = %q, want %q", cfg.RPC.Socket, "custom.sock")
				}
				if cfg.RPC.TCPAddress != "0.0.0.0:9090" {
					t.Errorf("RPC.TCPAddress = %q, want %q", cfg.RPC.TCPAddress, "0.0.0.0:9090")
				}
			},
		},
		{
			name: "web configuration overrides",
			envVars: map[string]string{
				"I2PLAN_WEB_ENABLED": "false",
				"I2PLAN_WEB_LISTEN":  "0.0.0.0:8888",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.Web.Enabled != false {
					t.Errorf("Web.Enabled = %v, want %v", cfg.Web.Enabled, false)
				}
				if cfg.Web.Listen != "0.0.0.0:8888" {
					t.Errorf("Web.Listen = %q, want %q", cfg.Web.Listen, "0.0.0.0:8888")
				}
			},
		},
		{
			name: "invalid integer values ignored",
			envVars: map[string]string{
				"I2PLAN_TUNNEL_LENGTH": "invalid",
				"I2PLAN_MAX_PEERS":     "not-a-number",
			},
			validate: func(t *testing.T, cfg *Config) {
				// Should retain default values when parsing fails
				if cfg.I2P.TunnelLength != DefaultTunnelLength {
					t.Errorf("I2P.TunnelLength = %d, want default %d", cfg.I2P.TunnelLength, DefaultTunnelLength)
				}
				if cfg.Mesh.MaxPeers != DefaultMaxPeers {
					t.Errorf("Mesh.MaxPeers = %d, want default %d", cfg.Mesh.MaxPeers, DefaultMaxPeers)
				}
			},
		},
		{
			name: "invalid boolean values ignored",
			envVars: map[string]string{
				"I2PLAN_RPC_ENABLED": "maybe",
				"I2PLAN_WEB_ENABLED": "yes-please",
			},
			validate: func(t *testing.T, cfg *Config) {
				// Should retain default values when parsing fails
				if cfg.RPC.Enabled != true {
					t.Errorf("RPC.Enabled = %v, want default true", cfg.RPC.Enabled)
				}
				if cfg.Web.Enabled != true {
					t.Errorf("Web.Enabled = %v, want default true", cfg.Web.Enabled)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original environment
			origEnv := make(map[string]string)
			for key := range tt.envVars {
				origEnv[key] = os.Getenv(key)
			}

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			// Restore environment after test
			defer func() {
				for key := range tt.envVars {
					if orig, ok := origEnv[key]; ok {
						os.Setenv(key, orig)
					} else {
						os.Unsetenv(key)
					}
				}
			}()

			// Test applyEnvOverrides
			cfg := DefaultConfig()
			applyEnvOverrides(cfg)
			tt.validate(t, cfg)
		})
	}
}

func TestLoadConfig_WithEnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	// Create a config file with specific values
	cfg := DefaultConfig()
	cfg.Node.Name = "file-node"
	cfg.I2P.TunnelLength = 2
	cfg.Mesh.MaxPeers = 25

	if err := SaveConfig(cfg, configPath); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Set environment variables that should override file values
	os.Setenv("I2PLAN_NODE_NAME", "env-node")
	os.Setenv("I2PLAN_TUNNEL_LENGTH", "3")
	os.Setenv("I2PLAN_MAX_PEERS", "75")
	defer func() {
		os.Unsetenv("I2PLAN_NODE_NAME")
		os.Unsetenv("I2PLAN_TUNNEL_LENGTH")
		os.Unsetenv("I2PLAN_MAX_PEERS")
	}()

	// Load config - env vars should override file values
	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Node.Name != "env-node" {
		t.Errorf("Node.Name = %q, want %q (env override)", loaded.Node.Name, "env-node")
	}
	if loaded.I2P.TunnelLength != 3 {
		t.Errorf("I2P.TunnelLength = %d, want %d (env override)", loaded.I2P.TunnelLength, 3)
	}
	if loaded.Mesh.MaxPeers != 75 {
		t.Errorf("Mesh.MaxPeers = %d, want %d (env override)", loaded.Mesh.MaxPeers, 75)
	}
}

func TestLoadConfig_NoFile_WithEnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistentPath := filepath.Join(tmpDir, "does-not-exist.toml")

	// Set environment variables
	os.Setenv("I2PLAN_NODE_NAME", "env-only-node")
	os.Setenv("I2PLAN_SAM_ADDRESS", "10.0.0.1:7656")
	defer func() {
		os.Unsetenv("I2PLAN_NODE_NAME")
		os.Unsetenv("I2PLAN_SAM_ADDRESS")
	}()

	// Load config from non-existent file - should use defaults + env overrides
	cfg, err := LoadConfig(nonExistentPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Node.Name != "env-only-node" {
		t.Errorf("Node.Name = %q, want %q", cfg.Node.Name, "env-only-node")
	}
	if cfg.I2P.SAMAddress != "10.0.0.1:7656" {
		t.Errorf("I2P.SAMAddress = %q, want %q", cfg.I2P.SAMAddress, "10.0.0.1:7656")
	}
}
