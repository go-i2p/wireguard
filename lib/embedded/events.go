package embedded

import (
	"sync/atomic"
	"time"
)

// EventType categorizes VPN events.
type EventType int

const (
	// EventStarted is emitted when the VPN starts successfully.
	EventStarted EventType = iota
	// EventStopped is emitted when the VPN stops.
	EventStopped
	// EventPeerConnected is emitted when a peer handshake completes.
	EventPeerConnected
	// EventPeerDisconnected is emitted when a peer connection is lost.
	EventPeerDisconnected
	// EventPeerDiscovered is emitted when a new peer is discovered via gossip.
	EventPeerDiscovered
	// EventInviteAccepted is emitted when we successfully join via an invite.
	EventInviteAccepted
	// EventInviteUsed is emitted when someone uses our invite to join.
	EventInviteUsed
	// EventInviteCreated is emitted when we create a new invite code.
	EventInviteCreated
	// EventInviteRevoked is emitted when we revoke an invite code.
	EventInviteRevoked
	// EventRouteAdded is emitted when a new route is learned.
	EventRouteAdded
	// EventRouteRemoved is emitted when a route expires or is removed.
	EventRouteRemoved
	// EventError is emitted when a recoverable error occurs.
	EventError
	// EventStateChanged is emitted when the VPN state changes.
	EventStateChanged
)

// String returns a human-readable name for the event type.
func (t EventType) String() string {
	switch t {
	case EventStarted:
		return "started"
	case EventStopped:
		return "stopped"
	case EventPeerConnected:
		return "peer_connected"
	case EventPeerDisconnected:
		return "peer_disconnected"
	case EventPeerDiscovered:
		return "peer_discovered"
	case EventInviteAccepted:
		return "invite_accepted"
	case EventInviteUsed:
		return "invite_used"
	case EventInviteCreated:
		return "invite_created"
	case EventInviteRevoked:
		return "invite_revoked"
	case EventRouteAdded:
		return "route_added"
	case EventRouteRemoved:
		return "route_removed"
	case EventError:
		return "error"
	case EventStateChanged:
		return "state_changed"
	default:
		return "unknown"
	}
}

// Event represents a VPN lifecycle or network event.
type Event struct {
	// Type is the category of this event.
	Type EventType

	// Timestamp is when the event occurred.
	Timestamp time.Time

	// Peer contains peer information for peer-related events.
	// Nil for non-peer events.
	Peer *PeerInfo

	// Error contains the error for EventError events.
	// Nil for non-error events.
	Error error

	// Message is a human-readable description of the event.
	Message string

	// Data contains event-specific additional data.
	// For EventStateChanged: map[string]any{"old": State, "new": State}
	// For EventRouteAdded/Removed: RouteInfo
	// For EventInviteUsed: map[string]any{"peer_id": string}
	Data any
}

// eventEmitter manages event channels and emission.
type eventEmitter struct {
	events       chan Event
	bufferSize   int
	closed       bool
	droppedCount atomic.Uint64 // counts events dropped due to full buffer
}

// newEventEmitter creates a new event emitter with the given buffer size.
func newEventEmitter(bufferSize int) *eventEmitter {
	if bufferSize < 1 {
		bufferSize = 100
	}
	return &eventEmitter{
		events:     make(chan Event, bufferSize),
		bufferSize: bufferSize,
	}
}

// emit sends an event to the channel.
// If the channel is full, the event is dropped (non-blocking) and the
// dropped counter is incremented. Use DroppedEvents() to check for drops.
func (e *eventEmitter) emit(event Event) {
	if e.closed {
		return
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	select {
	case e.events <- event:
	default:
		// Drop event if channel is full and track it
		e.droppedCount.Add(1)
	}
}

// emitSimple emits a simple event with just type and message.
func (e *eventEmitter) emitSimple(eventType EventType, message string) {
	e.emit(Event{
		Type:    eventType,
		Message: message,
	})
}

// emitError emits an error event.
func (e *eventEmitter) emitError(err error, message string) {
	e.emit(Event{
		Type:    EventError,
		Error:   err,
		Message: message,
	})
}

// emitPeerEvent emits a peer-related event.
func (e *eventEmitter) emitPeerEvent(eventType EventType, peer *PeerInfo, message string) {
	e.emit(Event{
		Type:    eventType,
		Peer:    peer,
		Message: message,
	})
}

// emitStateChange emits a state change event.
func (e *eventEmitter) emitStateChange(oldState, newState State, message string) {
	e.emit(Event{
		Type:    EventStateChanged,
		Message: message,
		Data: map[string]any{
			"old": oldState,
			"new": newState,
		},
	})
}

// channel returns the event channel for consumers.
func (e *eventEmitter) channel() <-chan Event {
	return e.events
}

// droppedEvents returns the total count of events dropped due to a full buffer.
// This can be used to detect if the consumer is not keeping up with event emission.
func (e *eventEmitter) droppedEvents() uint64 {
	return e.droppedCount.Load()
}

// close closes the event channel.
func (e *eventEmitter) close() {
	if !e.closed {
		e.closed = true
		close(e.events)
	}
}
