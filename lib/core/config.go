// Package core provides the main orchestration logic for an I2P-based
// WireGuard mesh VPN node. It coordinates identity management, peer discovery,
// gossip protocols, and user interfaces.
package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// Default configuration values
const (
	DefaultSAMAddress        = "127.0.0.1:7656"
	DefaultTunnelLength      = 2
	DefaultTunnelSubnet      = "10.42.0.0/16"
	DefaultHeartbeatInterval = 30 * time.Second
	DefaultPeerTimeout       = 5 * time.Minute
	DefaultMaxPeers          = 50
	DefaultRPCSocket         = "rpc.sock"
	DefaultWebListen         = "127.0.0.1:8080"
)

// Config holds all configuration for an i2plan node.
type Config struct {
	Node NodeConfig `toml:"node"`
	I2P  I2PConfig  `toml:"i2p"`
	Mesh MeshConfig `toml:"mesh"`
	RPC  RPCConfig  `toml:"rpc"`
	Web  WebConfig  `toml:"web"`
	TUI  TUIConfig  `toml:"tui"`
}

// NodeConfig contains basic node identification settings.
type NodeConfig struct {
	// Name is a human-readable identifier for this node
	Name string `toml:"name"`
	// DataDir is the directory where persistent data is stored
	DataDir string `toml:"data_dir"`
}

// I2PConfig contains I2P transport settings.
type I2PConfig struct {
	// SAMAddress is the SAM bridge address (host:port)
	SAMAddress string `toml:"sam_address"`
	// TunnelLength is the number of hops for I2P tunnels (lower = faster, less anonymous)
	TunnelLength int `toml:"tunnel_length"`
}

// MeshConfig contains mesh network settings.
type MeshConfig struct {
	// TunnelSubnet is the IP range used for mesh IPs (e.g., "10.42.0.0/16")
	TunnelSubnet string `toml:"tunnel_subnet"`
	// HeartbeatInterval is how often to announce presence to peers
	HeartbeatInterval time.Duration `toml:"heartbeat_interval"`
	// PeerTimeout is how long before a peer is considered stale
	PeerTimeout time.Duration `toml:"peer_timeout"`
	// MaxPeers is the maximum number of peers to maintain connections with
	MaxPeers int `toml:"max_peers"`
}

// RPCConfig contains RPC server settings.
type RPCConfig struct {
	// Enabled controls whether the RPC server is started
	Enabled bool `toml:"enabled"`
	// Socket is the path to the Unix socket for RPC (relative to DataDir)
	Socket string `toml:"socket"`
	// TCPAddress is an optional TCP address for RPC (e.g., "127.0.0.1:9090")
	TCPAddress string `toml:"tcp_address,omitempty"`
}

// WebConfig contains web UI settings.
type WebConfig struct {
	// Enabled controls whether the web UI is started
	Enabled bool `toml:"enabled"`
	// Listen is the address to bind the web server to
	Listen string `toml:"listen"`
}

// TUIConfig contains terminal UI settings.
type TUIConfig struct {
	// Enabled controls whether the TUI is available
	Enabled bool `toml:"enabled"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	dataDir := filepath.Join(homeDir, ".i2plan")

	return &Config{
		Node: NodeConfig{
			Name:    "my-node",
			DataDir: dataDir,
		},
		I2P: I2PConfig{
			SAMAddress:   DefaultSAMAddress,
			TunnelLength: DefaultTunnelLength,
		},
		Mesh: MeshConfig{
			TunnelSubnet:      DefaultTunnelSubnet,
			HeartbeatInterval: DefaultHeartbeatInterval,
			PeerTimeout:       DefaultPeerTimeout,
			MaxPeers:          DefaultMaxPeers,
		},
		RPC: RPCConfig{
			Enabled: true,
			Socket:  DefaultRPCSocket,
		},
		Web: WebConfig{
			Enabled: true,
			Listen:  DefaultWebListen,
		},
		TUI: TUIConfig{
			Enabled: true,
		},
	}
}

// LoadConfig reads configuration from a TOML file.
// If the file doesn't exist, it returns the default configuration.
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

// SaveConfig writes the configuration to a TOML file.
// It creates the parent directory if it doesn't exist.
func SaveConfig(cfg *Config, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if c.Node.Name == "" {
		return errors.New("node.name is required")
	}
	if c.Node.DataDir == "" {
		return errors.New("node.data_dir is required")
	}
	if c.I2P.SAMAddress == "" {
		return errors.New("i2p.sam_address is required")
	}
	if c.I2P.TunnelLength < 0 || c.I2P.TunnelLength > 7 {
		return errors.New("i2p.tunnel_length must be between 0 and 7")
	}
	if c.Mesh.TunnelSubnet == "" {
		return errors.New("mesh.tunnel_subnet is required")
	}
	if c.Mesh.MaxPeers < 1 {
		return errors.New("mesh.max_peers must be at least 1")
	}
	return nil
}

// DataPath returns an absolute path within the data directory.
func (c *Config) DataPath(elem ...string) string {
	parts := append([]string{c.Node.DataDir}, elem...)
	return filepath.Join(parts...)
}

// EnsureDataDir creates the data directory if it doesn't exist.
func (c *Config) EnsureDataDir() error {
	return os.MkdirAll(c.Node.DataDir, 0700)
}
