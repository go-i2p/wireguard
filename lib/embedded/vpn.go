package embedded

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"sync"
	"time"

	"github.com/go-i2p/wireguard/lib/core"
)

// State represents the VPN lifecycle state.
type State string

const (
	// StateInitial is the state before Start is called.
	StateInitial State = "initial"
	// StateStarting means the VPN is initializing.
	StateStarting State = "starting"
	// StateRunning means the VPN is fully operational.
	StateRunning State = "running"
	// StateStopping means the VPN is shutting down.
	StateStopping State = "stopping"
	// StateStopped means the VPN has been stopped.
	StateStopped State = "stopped"
)

// Status contains current VPN status information.
type Status struct {
	// State is the current VPN lifecycle state.
	State State
	// NodeID is the unique identifier for this node.
	NodeID string
	// NodeName is the human-readable node name.
	NodeName string
	// TunnelIP is this node's mesh tunnel IP address.
	TunnelIP netip.Addr
	// I2PDestination is the full I2P destination (base64).
	I2PDestination string
	// I2PAddress is the short I2P address (base32.b32.i2p).
	I2PAddress string
	// PeerCount is the number of connected peers.
	PeerCount int
	// Uptime is how long the VPN has been running.
	Uptime time.Duration
	// StartedAt is when the VPN was started.
	StartedAt time.Time
	// Version is the software version.
	Version string
}

// VPN is the main embedded VPN controller.
// It provides a high-level API for VPN lifecycle and operations.
type VPN struct {
	mu sync.RWMutex

	config    Config
	logger    *slog.Logger
	node      *core.Node
	state     State
	emitter   *eventEmitter
	done      chan struct{}
	startedAt time.Time

	// Context for the VPN's lifetime
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new embedded VPN instance with the given configuration.
// The VPN is not started until Start() is called.
func New(cfg Config) (*VPN, error) {
	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &VPN{
		config:  cfg,
		logger:  logger.With("component", "embedded-vpn"),
		state:   StateInitial,
		emitter: newEventEmitter(cfg.EventBufferSize),
		done:    make(chan struct{}),
	}, nil
}

// NewWithOptions creates a VPN with functional options.
// This is an alternative to New(Config{...}) for more ergonomic usage.
func NewWithOptions(opts ...Option) (*VPN, error) {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return New(cfg)
}

// Start initializes and starts the VPN.
// This includes:
//   - Creating/loading identity
//   - Opening I2P transport
//   - Starting WireGuard device
//   - Joining mesh network (if previously connected)
//
// The context controls the startup timeout, not the VPN lifetime.
func (v *VPN) Start(ctx context.Context) error {
	v.mu.Lock()
	if v.state != StateInitial && v.state != StateStopped {
		v.mu.Unlock()
		return fmt.Errorf("cannot start VPN in state %s", v.state)
	}
	oldState := v.state
	v.state = StateStarting
	v.done = make(chan struct{})
	v.mu.Unlock()

	v.emitter.emitStateChange(oldState, StateStarting, "VPN starting")

	// Create internal context for VPN lifetime
	v.ctx, v.cancel = context.WithCancel(context.Background())

	// Convert embedded config to core config
	coreConfig := v.config.toCoreConfig()

	// Create the core node
	node, err := core.NewNode(coreConfig, v.logger)
	if err != nil {
		v.transitionTo(StateStopped)
		v.emitter.emitError(err, "Failed to create node")
		return fmt.Errorf("failed to create node: %w", err)
	}
	v.node = node

	// Start the node with the provided context for timeout
	if err := node.Start(ctx); err != nil {
		v.transitionTo(StateStopped)
		v.emitter.emitError(err, "Failed to start node")
		return fmt.Errorf("failed to start node: %w", err)
	}

	v.mu.Lock()
	v.state = StateRunning
	v.startedAt = time.Now()
	v.mu.Unlock()

	v.emitter.emitStateChange(StateStarting, StateRunning, "VPN started")
	v.emitter.emitSimple(EventStarted, "VPN is now running")

	// Start background monitor
	go v.monitor()

	return nil
}

// Stop gracefully shuts down the VPN.
// It notifies peers, saves state, and closes connections.
// The context controls the shutdown timeout.
func (v *VPN) Stop(ctx context.Context) error {
	v.mu.Lock()
	if v.state != StateRunning {
		v.mu.Unlock()
		return fmt.Errorf("cannot stop VPN in state %s", v.state)
	}
	v.state = StateStopping
	v.mu.Unlock()

	v.emitter.emitStateChange(StateRunning, StateStopping, "VPN stopping")

	// Cancel internal context
	if v.cancel != nil {
		v.cancel()
	}

	// Stop the core node
	var err error
	if v.node != nil {
		err = v.node.Stop(ctx)
	}

	v.transitionTo(StateStopped)
	v.emitter.emitStateChange(StateStopping, StateStopped, "VPN stopped")
	v.emitter.emitSimple(EventStopped, "VPN has stopped")

	// Close the done channel
	close(v.done)

	return err
}

// Close is an alias for Stop with a default 30-second timeout.
// Suitable for use with defer.
func (v *VPN) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// If not running, just clean up
	v.mu.RLock()
	state := v.state
	v.mu.RUnlock()

	if state == StateInitial || state == StateStopped {
		v.emitter.close()
		return nil
	}

	err := v.Stop(ctx)
	v.emitter.close()
	return err
}

// Status returns current VPN status.
func (v *VPN) Status() Status {
	v.mu.RLock()
	defer v.mu.RUnlock()

	status := Status{
		State:    v.state,
		NodeName: v.config.NodeName,
		Version:  "dev", // TODO: inject version at build time
	}

	if !v.startedAt.IsZero() {
		status.StartedAt = v.startedAt
		status.Uptime = time.Since(v.startedAt)
	}

	// Get info from core node if available
	if v.node != nil {
		// TODO: Expose these from core.Node once implemented
		// status.NodeID = v.node.NodeID()
		// status.TunnelIP = v.node.TunnelIP()
		// status.I2PDestination = v.node.I2PDestination()
		// status.PeerCount = v.node.PeerCount()
	}

	return status
}

// State returns the current VPN state.
func (v *VPN) State() State {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.state
}

// Events returns a channel that receives VPN events.
// The channel is buffered and may drop events if not consumed.
// Close the VPN to close this channel.
func (v *VPN) Events() <-chan Event {
	return v.emitter.channel()
}

// Done returns a channel that is closed when the VPN stops.
func (v *VPN) Done() <-chan struct{} {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.done
}

// TunnelIP returns this node's mesh tunnel IP address.
// Returns an invalid address if the VPN is not running.
func (v *VPN) TunnelIP() netip.Addr {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// TODO: Get from identity once core.Node exposes it
	return netip.Addr{}
}

// I2PDestination returns this node's full I2P destination.
// Returns empty string if the VPN is not running.
func (v *VPN) I2PDestination() string {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// TODO: Get from transport once core.Node exposes it
	return ""
}

// I2PAddress returns this node's short I2P address (base32.b32.i2p).
// Returns empty string if the VPN is not running.
func (v *VPN) I2PAddress() string {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// TODO: Get from transport once core.Node exposes it
	return ""
}

// NodeID returns this node's unique identifier.
// Returns empty string if the VPN is not running.
func (v *VPN) NodeID() string {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// TODO: Get from identity once core.Node exposes it
	return ""
}

// Config returns the VPN configuration (read-only copy).
func (v *VPN) Config() Config {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.config
}

// Node returns the underlying core.Node for advanced operations.
// Returns nil if the VPN is not started.
// Use with caution - direct manipulation may cause unexpected behavior.
func (v *VPN) Node() *core.Node {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.node
}

// transitionTo changes the VPN state.
func (v *VPN) transitionTo(newState State) {
	v.mu.Lock()
	v.state = newState
	v.mu.Unlock()
}

// monitor watches the core node and emits events.
func (v *VPN) monitor() {
	if v.node == nil {
		return
	}

	select {
	case <-v.ctx.Done():
		// Normal shutdown
	case <-v.node.Done():
		// Node stopped unexpectedly
		v.mu.Lock()
		if v.state == StateRunning {
			v.state = StateStopped
			v.mu.Unlock()
			v.emitter.emitError(errors.New("node stopped unexpectedly"), "VPN stopped unexpectedly")
			v.emitter.emitSimple(EventStopped, "VPN stopped unexpectedly")
			close(v.done)
		} else {
			v.mu.Unlock()
		}
	}
}
