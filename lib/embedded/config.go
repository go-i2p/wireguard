// Package embedded provides an embeddable i2plan VPN API for third-party applications.
package embedded

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/go-i2p/wireguard/lib/core"
)

// Default configuration values for embedded VPN.
const (
	DefaultSAMAddress   = "127.0.0.1:7656"
	DefaultTunnelSubnet = "10.42.0.0/16"
	DefaultWebListen    = "127.0.0.1:8080"
)

// Config configures an embedded VPN instance.
// Fields with zero values use sensible defaults.
type Config struct {
	// NodeName is a human-readable identifier for this node.
	// Default: hostname or "embedded-node"
	NodeName string

	// DataDir is where persistent data (identity, config) is stored.
	// Default: OS-specific temp directory with unique suffix
	DataDir string

	// SAMAddress is the I2P SAM bridge address.
	// Default: "127.0.0.1:7656"
	SAMAddress string

	// TunnelSubnet is the mesh IP range (CIDR notation).
	// Default: "10.42.0.0/16"
	TunnelSubnet string

	// TunnelLength is the number of I2P tunnel hops (0-7).
	// Lower values are faster but less anonymous.
	// Default: 2
	TunnelLength int

	// MaxPeers is the maximum number of peers to connect with.
	// Default: 50
	MaxPeers int

	// Logger for VPN operations.
	// Default: slog.Default()
	Logger *slog.Logger

	// EnableRPC starts the RPC server for external control.
	// Default: false (embedded apps typically control directly)
	EnableRPC bool

	// RPCSocket is the path to the Unix socket for RPC (relative to DataDir).
	// Default: "rpc.sock"
	RPCSocket string

	// EnableWeb starts the web UI server.
	// Default: false
	EnableWeb bool

	// WebListenAddr is the address for the web server if enabled.
	// Default: "127.0.0.1:8080"
	WebListenAddr string

	// EventBufferSize is the size of the event channel buffer.
	// Default: 100
	EventBufferSize int
}

// Option is a functional option for configuring a VPN.
type Option func(*Config)

// WithNodeName sets the node name.
func WithNodeName(name string) Option {
	return func(c *Config) {
		c.NodeName = name
	}
}

// WithDataDir sets the data directory.
func WithDataDir(dir string) Option {
	return func(c *Config) {
		c.DataDir = dir
	}
}

// WithSAMAddress sets the SAM bridge address.
func WithSAMAddress(addr string) Option {
	return func(c *Config) {
		c.SAMAddress = addr
	}
}

// WithTunnelSubnet sets the mesh tunnel subnet.
func WithTunnelSubnet(subnet string) Option {
	return func(c *Config) {
		c.TunnelSubnet = subnet
	}
}

// WithTunnelLength sets the I2P tunnel hop count.
func WithTunnelLength(length int) Option {
	return func(c *Config) {
		c.TunnelLength = length
	}
}

// WithMaxPeers sets the maximum peer count.
func WithMaxPeers(max int) Option {
	return func(c *Config) {
		c.MaxPeers = max
	}
}

// WithLogger sets the logger.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Config) {
		c.Logger = logger
	}
}

// WithRPC enables the RPC server.
func WithRPC(enabled bool) Option {
	return func(c *Config) {
		c.EnableRPC = enabled
	}
}

// WithWeb enables the web UI server.
func WithWeb(enabled bool, listenAddr string) Option {
	return func(c *Config) {
		c.EnableWeb = enabled
		if listenAddr != "" {
			c.WebListenAddr = listenAddr
		}
	}
}

// WithEventBufferSize sets the event channel buffer size.
func WithEventBufferSize(size int) Option {
	return func(c *Config) {
		c.EventBufferSize = size
	}
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	nodeName := "embedded-node"
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		nodeName = hostname
	}

	// Use a temp directory by default
	dataDir := filepath.Join(os.TempDir(), "i2plan-embedded")

	return Config{
		NodeName:        nodeName,
		DataDir:         dataDir,
		SAMAddress:      DefaultSAMAddress,
		TunnelSubnet:    DefaultTunnelSubnet,
		TunnelLength:    2,
		MaxPeers:        50,
		Logger:          nil, // Will use slog.Default() if nil
		EnableRPC:       false,
		RPCSocket:       "rpc.sock",
		EnableWeb:       false,
		WebListenAddr:   DefaultWebListen,
		EventBufferSize: 100,
	}
}

// applyDefaults fills in zero values with defaults.
func (c *Config) applyDefaults() {
	defaults := DefaultConfig()

	if c.NodeName == "" {
		c.NodeName = defaults.NodeName
	}
	if c.DataDir == "" {
		c.DataDir = defaults.DataDir
	}
	if c.SAMAddress == "" {
		c.SAMAddress = defaults.SAMAddress
	}
	if c.TunnelSubnet == "" {
		c.TunnelSubnet = defaults.TunnelSubnet
	}
	if c.TunnelLength == 0 {
		c.TunnelLength = defaults.TunnelLength
	}
	if c.MaxPeers == 0 {
		c.MaxPeers = defaults.MaxPeers
	}
	if c.RPCSocket == "" {
		c.RPCSocket = defaults.RPCSocket
	}
	if c.WebListenAddr == "" {
		c.WebListenAddr = defaults.WebListenAddr
	}
	if c.EventBufferSize == 0 {
		c.EventBufferSize = defaults.EventBufferSize
	}
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if c.NodeName == "" {
		return errors.New("node name is required")
	}
	if c.DataDir == "" {
		return errors.New("data directory is required")
	}
	if c.SAMAddress == "" {
		return errors.New("SAM address is required")
	}
	if c.TunnelLength < 0 || c.TunnelLength > 7 {
		return errors.New("tunnel length must be between 0 and 7")
	}
	if c.MaxPeers < 1 {
		return errors.New("max peers must be at least 1")
	}
	if c.EventBufferSize < 1 {
		return errors.New("event buffer size must be at least 1")
	}
	return nil
}

// toCoreConfig converts embedded.Config to core.Config.
func (c *Config) toCoreConfig() *core.Config {
	return &core.Config{
		Node: core.NodeConfig{
			Name:    c.NodeName,
			DataDir: c.DataDir,
		},
		I2P: core.I2PConfig{
			SAMAddress:   c.SAMAddress,
			TunnelLength: c.TunnelLength,
		},
		Mesh: core.MeshConfig{
			TunnelSubnet:      c.TunnelSubnet,
			HeartbeatInterval: core.DefaultHeartbeatInterval,
			PeerTimeout:       core.DefaultPeerTimeout,
			MaxPeers:          c.MaxPeers,
		},
		RPC: core.RPCConfig{
			Enabled: c.EnableRPC,
			Socket:  c.RPCSocket,
		},
		Web: core.WebConfig{
			Enabled: c.EnableWeb,
			Listen:  c.WebListenAddr,
		},
		TUI: core.TUIConfig{
			Enabled: false, // TUI not used in embedded mode
		},
	}
}
