package embedded

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/go-i2p/wireguard/lib/core"
	"github.com/go-i2p/wireguard/version"
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
	log.WithField("nodeName", cfg.NodeName).Debug("creating new VPN instance")
	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		log.WithError(err).Debug("VPN configuration validation failed")
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	log.WithField("nodeName", cfg.NodeName).Debug("VPN instance created")
	return &VPN{
		config:  cfg,
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
	log.WithField("nodeName", v.config.NodeName).Info("starting VPN")
	v.mu.Lock()
	if v.state != StateInitial && v.state != StateStopped {
		v.mu.Unlock()
		log.WithField("state", v.state).Warn("cannot start VPN in current state")
		return fmt.Errorf("cannot start VPN in state %s", v.state)
	}
	oldState := v.state
	v.state = StateStarting
	v.done = make(chan struct{})
	v.mu.Unlock()

	log.WithField("oldState", oldState).WithField("newState", StateStarting).Debug("VPN state transition")
	v.emitter.emitStateChange(oldState, StateStarting, "VPN starting")

	// Create internal context for VPN lifetime
	v.ctx, v.cancel = context.WithCancel(context.Background())

	// Convert embedded config to core config
	coreConfig := v.config.toCoreConfig()

	// Create the core node
	log.Debug("creating core node")
	node, err := core.NewNode(coreConfig)
	if err != nil {
		log.WithError(err).Error("failed to create core node")
		v.transitionTo(StateStopped)
		v.emitter.emitError(err, "Failed to create node")
		return fmt.Errorf("failed to create node: %w", err)
	}
	v.node = node

	// Start the node with the provided context for timeout
	log.Debug("starting core node")
	if err := node.Start(ctx); err != nil {
		log.WithError(err).Error("failed to start core node")
		v.transitionTo(StateStopped)
		v.emitter.emitError(err, "Failed to start node")
		return fmt.Errorf("failed to start node: %w", err)
	}

	v.mu.Lock()
	v.state = StateRunning
	v.startedAt = time.Now()
	v.mu.Unlock()

	log.WithField("oldState", StateStarting).WithField("newState", StateRunning).Debug("VPN state transition")
	v.emitter.emitStateChange(StateStarting, StateRunning, "VPN started")
	v.emitter.emitSimple(EventStarted, "VPN is now running")

	// Start background monitor
	go v.monitor()

	log.WithField("nodeName", v.config.NodeName).Info("VPN started successfully")
	return nil
}

// Stop gracefully shuts down the VPN.
// It notifies peers, saves state, and closes connections.
// The context controls the shutdown timeout.
func (v *VPN) Stop(ctx context.Context) error {
	log.Info("stopping VPN")
	v.mu.Lock()
	if v.state != StateRunning {
		v.mu.Unlock()
		log.WithField("state", v.state).Warn("cannot stop VPN in current state")
		return fmt.Errorf("cannot stop VPN in state %s", v.state)
	}
	v.state = StateStopping
	v.mu.Unlock()

	log.WithField("oldState", StateRunning).WithField("newState", StateStopping).Debug("VPN state transition")
	v.emitter.emitStateChange(StateRunning, StateStopping, "VPN stopping")

	// Cancel internal context
	if v.cancel != nil {
		v.cancel()
	}

	// Stop the core node
	var err error
	if v.node != nil {
		log.Debug("stopping core node")
		err = v.node.Stop(ctx)
		if err != nil {
			log.WithError(err).Warn("error stopping core node")
		}
	}

	v.transitionTo(StateStopped)
	log.WithField("oldState", StateStopping).WithField("newState", StateStopped).Debug("VPN state transition")
	v.emitter.emitStateChange(StateStopping, StateStopped, "VPN stopped")
	v.emitter.emitSimple(EventStopped, "VPN has stopped")

	// Close the done channel
	close(v.done)

	log.Info("VPN stopped")
	return err
}

// Close is an alias for Stop with a default 30-second timeout.
// Suitable for use with defer.
func (v *VPN) Close() error {
	log.Debug("closing VPN")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// If not running, just clean up
	v.mu.RLock()
	state := v.state
	v.mu.RUnlock()

	if state == StateInitial || state == StateStopped {
		log.WithField("state", state).Debug("VPN already stopped, cleaning up emitter")
		v.emitter.close()
		return nil
	}

	err := v.Stop(ctx)
	v.emitter.close()
	log.Debug("VPN closed")
	return err
}

// Status returns current VPN status.
func (v *VPN) Status() Status {
	v.mu.RLock()
	defer v.mu.RUnlock()

	status := Status{
		State:    v.state,
		NodeName: v.config.NodeName,
		Version:  version.Version,
	}

	if !v.startedAt.IsZero() {
		status.StartedAt = v.startedAt
		status.Uptime = time.Since(v.startedAt)
	}

	// Get info from core node if available
	if v.node != nil {
		status.NodeID = v.node.NodeID()
		status.TunnelIP = v.node.TunnelIPAddr()
		status.I2PDestination = v.node.I2PDestination()
		status.I2PAddress = v.node.I2PAddress()
		status.PeerCount = v.node.PeerCount()
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
// Use DroppedEventCount() to check if events have been dropped.
// Close the VPN to close this channel.
func (v *VPN) Events() <-chan Event {
	return v.emitter.channel()
}

// DroppedEventCount returns the total number of events dropped due to a full buffer.
// If this value is non-zero, the event consumer is not keeping up with event emission.
// This can help detect missed critical events.
func (v *VPN) DroppedEventCount() uint64 {
	return v.emitter.droppedEvents()
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

	if v.node == nil {
		return netip.Addr{}
	}
	return v.node.TunnelIPAddr()
}

// I2PDestination returns this node's full I2P destination.
// Returns empty string if the VPN is not running.
func (v *VPN) I2PDestination() string {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.node == nil {
		return ""
	}
	return v.node.I2PDestination()
}

// I2PAddress returns this node's short I2P address (base32.b32.i2p).
// Returns empty string if the VPN is not running.
func (v *VPN) I2PAddress() string {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.node == nil || v.node.Transport() == nil {
		return ""
	}
	return v.node.Transport().LocalAddress()
}

// NodeID returns this node's unique identifier.
// Returns empty string if the VPN is not running.
func (v *VPN) NodeID() string {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.node == nil {
		return ""
	}
	return v.node.NodeID()
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
	oldState := v.state
	v.state = newState
	v.mu.Unlock()
	log.WithField("oldState", oldState).WithField("newState", newState).Debug("VPN state transition")
}

// monitor watches the core node and emits events.
func (v *VPN) monitor() {
	log.Debug("VPN monitor started")
	if v.node == nil {
		log.Warn("VPN monitor: node is nil")
		return
	}

	select {
	case <-v.ctx.Done():
		// Normal shutdown
		log.Debug("VPN monitor: normal shutdown")
	case <-v.node.Done():
		// Node stopped unexpectedly
		log.Warn("VPN monitor: node stopped unexpectedly")
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
	log.Debug("VPN monitor stopped")
}
