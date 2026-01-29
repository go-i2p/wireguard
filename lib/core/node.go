package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// NodeState represents the current state of the node.
type NodeState int

const (
	// StateInitial is the initial state before Start is called.
	StateInitial NodeState = iota
	// StateStarting means the node is in the process of starting.
	StateStarting
	// StateRunning means the node is fully operational.
	StateRunning
	// StateStopping means the node is shutting down.
	StateStopping
	// StateStopped means the node has been stopped.
	StateStopped
)

func (s NodeState) String() string {
	switch s {
	case StateInitial:
		return "initial"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	case StateStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// Node is the main orchestrator for an i2plan mesh VPN node.
// It coordinates identity management, I2P transport, WireGuard device,
// gossip protocol, and user interfaces.
type Node struct {
	mu     sync.RWMutex
	config *Config
	logger *slog.Logger
	state  NodeState

	// cancel is used to signal shutdown to all goroutines
	cancel context.CancelFunc
	// done signals that the node has fully stopped
	done chan struct{}

	// startedAt tracks when the node started
	startedAt time.Time

	// Event callbacks for embedded API integration
	onStateChange func(oldState, newState NodeState)
	onError       func(err error, message string)
}

// NewNode creates a new Node with the given configuration.
// The node is not started until Start() is called.
func NewNode(cfg *Config, logger *slog.Logger) (*Node, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if logger == nil {
		logger = slog.Default()
	}

	return &Node{
		config: cfg,
		logger: logger.With("component", "node"),
		state:  StateInitial,
		done:   make(chan struct{}),
	}, nil
}

// Start initializes and starts all node components.
// This includes:
//   - Creating data directory
//   - Loading or generating identity
//   - Opening I2P transport
//   - Starting WireGuard device
//   - Starting gossip protocol
//   - Starting user interfaces (RPC, Web, TUI) if enabled
//
// Start blocks until the node is fully initialized or an error occurs.
func (n *Node) Start(ctx context.Context) error {
	n.mu.Lock()
	if n.state != StateInitial && n.state != StateStopped {
		n.mu.Unlock()
		return fmt.Errorf("cannot start node in state %s", n.state)
	}
	oldState := n.state
	n.state = StateStarting
	n.done = make(chan struct{})
	n.mu.Unlock()

	n.emitStateChange(oldState, StateStarting)

	// Create a cancellable context for the node's lifetime
	nodeCtx, cancel := context.WithCancel(ctx)
	n.cancel = cancel

	n.logger.Info("starting node",
		"name", n.config.Node.Name,
		"data_dir", n.config.Node.DataDir,
	)

	// Ensure data directory exists
	if err := n.config.EnsureDataDir(); err != nil {
		n.transitionToStopped()
		n.emitError(err, "failed to create data directory")
		return fmt.Errorf("creating data directory: %w", err)
	}

	// TODO (Phase 1): Load or generate identity
	// TODO (Phase 2): Open I2P transport and WireGuard device
	// TODO (Phase 3): Start gossip protocol
	// TODO (Phase 4+): Start RPC, Web, TUI interfaces

	n.mu.Lock()
	n.state = StateRunning
	n.startedAt = time.Now()
	n.mu.Unlock()

	n.emitStateChange(StateStarting, StateRunning)
	n.logger.Info("node started")

	// Start the main run loop in a goroutine
	go n.run(nodeCtx)

	return nil
}

// run is the main loop that runs until the context is cancelled.
func (n *Node) run(ctx context.Context) {
	defer close(n.done)

	<-ctx.Done()

	n.logger.Info("node shutting down")

	n.mu.Lock()
	oldState := n.state
	n.state = StateStopped
	n.mu.Unlock()

	n.emitStateChange(oldState, StateStopped)
}

// Stop gracefully shuts down the node.
// It blocks until all components have stopped or the context is cancelled.
func (n *Node) Stop(ctx context.Context) error {
	n.mu.Lock()
	if n.state != StateRunning {
		n.mu.Unlock()
		return fmt.Errorf("cannot stop node in state %s", n.state)
	}
	n.state = StateStopping
	cancel := n.cancel
	n.mu.Unlock()

	n.emitStateChange(StateRunning, StateStopping)
	n.logger.Info("stopping node")

	// Signal all goroutines to stop
	if cancel != nil {
		cancel()
	}

	// Wait for the run loop to finish or timeout
	select {
	case <-n.done:
		n.logger.Info("node stopped")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// transitionToStopped updates the state to stopped.
func (n *Node) transitionToStopped() {
	n.mu.Lock()
	n.state = StateStopped
	n.mu.Unlock()
}

// State returns the current state of the node.
func (n *Node) State() NodeState {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.state
}

// Config returns the node's configuration.
func (n *Node) Config() *Config {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.config
}

// Done returns a channel that is closed when the node has stopped.
func (n *Node) Done() <-chan struct{} {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.done
}

// StartedAt returns when the node was started.
// Returns zero time if not started.
func (n *Node) StartedAt() time.Time {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.startedAt
}

// Uptime returns how long the node has been running.
// Returns zero if not running.
func (n *Node) Uptime() time.Duration {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.startedAt.IsZero() || n.state != StateRunning {
		return 0
	}
	return time.Since(n.startedAt)
}

// SetOnStateChange sets a callback for state changes.
// The callback is invoked synchronously during state transitions.
func (n *Node) SetOnStateChange(callback func(oldState, newState NodeState)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.onStateChange = callback
}

// SetOnError sets a callback for error events.
// The callback is invoked when recoverable errors occur.
func (n *Node) SetOnError(callback func(err error, message string)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.onError = callback
}

// emitStateChange notifies the state change callback if set.
func (n *Node) emitStateChange(oldState, newState NodeState) {
	n.mu.RLock()
	callback := n.onStateChange
	n.mu.RUnlock()

	if callback != nil {
		callback(oldState, newState)
	}
}

// emitError notifies the error callback if set.
func (n *Node) emitError(err error, message string) {
	n.mu.RLock()
	callback := n.onError
	n.mu.RUnlock()

	if callback != nil {
		callback(err, message)
	}
}
