// Package i2pbind provides a WireGuard Bind implementation that uses I2P
// for transport instead of regular UDP. This enables WireGuard tunnels to
// run over the I2P anonymizing network.
//
// Prerequisites:
//   - A running I2P router with SAM enabled (default port 7656)
//   - The onramp and sam3 libraries from github.com/go-i2p
//
// Example usage:
//
//	bind := i2pbind.NewI2PBind("my-wireguard-tunnel")
//	dev := device.NewDevice(tun, bind, logger)
package i2pbind

import (
	"errors"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"

	"github.com/go-i2p/i2pkeys"
	"github.com/go-i2p/onramp"

	"golang.zx2c4.com/wireguard/conn"
)

const (
	// MaxI2PDatagramSize is the maximum size for I2P datagrams (31KB)
	MaxI2PDatagramSize = 31 * 1024

	// DefaultSAMAddress is the default SAM bridge address
	DefaultSAMAddress = "127.0.0.1:7656"
)

var (
	// Ensure I2PBind implements conn.Bind
	_ conn.Bind = (*I2PBind)(nil)

	// Ensure I2PEndpoint implements conn.Endpoint
	_ conn.Endpoint = (*I2PEndpoint)(nil)
)

// I2PEndpoint represents an I2P destination as a WireGuard endpoint
type I2PEndpoint struct {
	dest i2pkeys.I2PAddr // Remote I2P destination
	src  i2pkeys.I2PAddr // Local source (for sticky routing, optional)
}

// NewI2PEndpoint creates a new I2P endpoint from a destination address
func NewI2PEndpoint(dest i2pkeys.I2PAddr) *I2PEndpoint {
	log.WithField("dest", dest.Base32()).Debug("creating new I2P endpoint")
	return &I2PEndpoint{dest: dest}
}

// ClearSrc clears the source address used for sticky routing
func (e *I2PEndpoint) ClearSrc() {
	e.src = ""
}

// SrcToString returns the source I2P address as a string
func (e *I2PEndpoint) SrcToString() string {
	if e.src == "" {
		return ""
	}
	return e.src.Base32()
}

// DstToString returns the destination I2P address as a base32 string
func (e *I2PEndpoint) DstToString() string {
	return e.dest.Base32()
}

// DstToBytes returns bytes representation of the destination for MAC2 cookies.
// Returns the raw 32-byte I2P destination hash for cryptographic operations.
func (e *I2PEndpoint) DstToBytes() []byte {
	// Return the raw hash bytes (not base32-encoded string) for cookie calculations
	hash := e.dest.DestHash()
	return hash[:]
}

// DstIP returns the destination "IP" - for I2P this is not meaningful
// Returns an invalid address since I2P doesn't use IP routing
func (e *I2PEndpoint) DstIP() netip.Addr {
	// Return an invalid address - I2P doesn't use IP addresses
	// Callers should check IsValid() before using
	return netip.Addr{}
}

// SrcIP returns the source "IP" - for I2P this is not meaningful
func (e *I2PEndpoint) SrcIP() netip.Addr {
	return netip.Addr{}
}

// Destination returns the underlying I2P destination address
func (e *I2PEndpoint) Destination() i2pkeys.I2PAddr {
	return e.dest
}

// MeshMessageHandler is a callback function for handling mesh protocol messages.
// It is called when a non-WireGuard message is received (e.g., gossip/handshake messages).
// The data parameter contains the raw message bytes, and from is the sender's I2P address.
type MeshMessageHandler func(data []byte, from i2pkeys.I2PAddr)

// I2PBind implements conn.Bind using I2P datagrams for transport
type I2PBind struct {
	mu sync.Mutex

	// Configuration
	name       string   // Tunnel name for I2P
	samAddr    string   // SAM bridge address
	samOptions []string // SAM session options (tunnel parameters)

	// I2P session components
	garlic     *onramp.Garlic
	packetConn net.PacketConn

	// Mesh message handling
	meshHandler MeshMessageHandler // Callback for mesh protocol messages

	// State
	closed bool

	// Tracking for handler ordering warning
	messagesReceived atomic.Bool
}

// NewI2PBind creates a new I2P Bind with default settings
func NewI2PBind(name string) *I2PBind {
	return NewI2PBindWithSAM(name, DefaultSAMAddress)
}

// NewI2PBindWithSAM creates a new I2P Bind with a custom SAM address
func NewI2PBindWithSAM(name, samAddr string) *I2PBind {
	return NewI2PBindWithOptions(name, samAddr, nil)
}

// NewI2PBindWithOptions creates a new I2P Bind with custom SAM address and tunnel options.
// The options parameter allows configuring I2P tunnel parameters such as:
//   - inbound.length, outbound.length (tunnel hop count, application default is 2)
//   - inbound.quantity, outbound.quantity (number of tunnels)
//   - inbound.backupQuantity, outbound.backupQuantity (backup tunnels)
//
// If options is nil or empty, onramp.OPT_DEFAULTS will be used.
// See github.com/go-i2p/sam3 for available options.
func NewI2PBindWithOptions(name, samAddr string, options []string) *I2PBind {
	log.WithField("name", name).WithField("samAddr", samAddr).Debug("creating new I2P bind")
	return &I2PBind{
		name:       name,
		samAddr:    samAddr,
		samOptions: options,
	}
}

// SetMeshHandler sets the callback for handling mesh protocol messages.
// When a non-WireGuard packet is received (e.g., gossip or handshake messages),
// the handler will be called with the raw message data and sender address.
// This should be called before Open() to ensure no messages are dropped.
// A warning is logged if called after messages have already been received.
func (b *I2PBind) SetMeshHandler(handler MeshMessageHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.messagesReceived.Load() {
		log.Warn("SetMeshHandler called after messages received, earlier mesh messages may have been dropped")
	}

	log.Debug("mesh handler set")
	b.meshHandler = handler
}

// Open initializes the I2P datagram session and returns receive functions
func (b *I2PBind) Open(port uint16) ([]conn.ReceiveFunc, uint16, error) {
	log.WithField("name", b.name).Debug("opening I2P bind")
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.packetConn != nil {
		log.Warn("bind already open")
		return nil, 0, conn.ErrBindAlreadyOpen
	}

	// Use configured options or fall back to defaults
	options := b.samOptions
	if len(options) == 0 {
		options = onramp.OPT_DEFAULTS
	}

	// Create a new Garlic session with configured options
	log.WithField("samAddr", b.samAddr).Debug("creating garlic session")
	garlic, err := onramp.NewGarlic(b.name, b.samAddr, options)
	if err != nil {
		log.WithError(err).Error("failed to create garlic session")
		return nil, 0, err
	}
	b.garlic = garlic
	log.Debug("garlic session created successfully")

	// Get a PacketConn (uses hybrid2 protocol internally)
	log.Debug("listening for packets")
	packetConn, err := garlic.ListenPacket()
	if err != nil {
		log.WithError(err).Error("failed to listen for packets")
		garlic.Close()
		return nil, 0, err
	}
	b.packetConn = packetConn
	b.closed = false

	// Create receive function
	recvFunc := b.makeReceiveFunc()

	log.WithField("name", b.name).Debug("I2P bind opened successfully")
	// Port is not meaningful for I2P, return 0
	return []conn.ReceiveFunc{recvFunc}, 0, nil
}

// makeReceiveFunc creates the receive function for incoming I2P datagrams.
// It demultiplexes incoming packets between WireGuard traffic and mesh protocol messages.
// WireGuard messages have a 4-byte little-endian type header (1-4).
// Mesh protocol messages are JSON and start with '{' (0x7B).
// parseI2PAddress converts a network address to an I2P address.
func parseI2PAddress(addr net.Addr) (i2pkeys.I2PAddr, error) {
	i2pAddr, ok := addr.(i2pkeys.I2PAddr)
	if ok {
		return i2pAddr, nil
	}
	return i2pkeys.NewI2PAddrFromString(addr.String())
}

// isMeshProtocolMessage checks if the packet is a mesh protocol message (JSON format).
func isMeshProtocolMessage(data []byte) bool {
	return len(data) > 0 && data[0] == '{'
}

// handleMeshMessage dispatches a mesh protocol message to the handler.
func (b *I2PBind) handleMeshMessage(data []byte, numBytes int, addr i2pkeys.I2PAddr, handler MeshMessageHandler) {
	if handler == nil {
		return
	}
	msgCopy := make([]byte, numBytes)
	copy(msgCopy, data[:numBytes])
	go handler(msgCopy, addr)
}

func (b *I2PBind) makeReceiveFunc() conn.ReceiveFunc {
	return func(packets [][]byte, sizes []int, eps []conn.Endpoint) (n int, err error) {
		b.mu.Lock()
		pc := b.packetConn
		meshHandler := b.meshHandler
		b.mu.Unlock()

		if pc == nil {
			return 0, net.ErrClosed
		}

		if len(packets) == 0 || len(sizes) == 0 || len(eps) == 0 {
			return 0, nil
		}

		for {
			numBytes, addr, err := pc.ReadFrom(packets[0])
			if err != nil {
				return 0, err
			}

			// Mark that we've received messages (for handler ordering warning)
			b.messagesReceived.Store(true)

			i2pAddr, err := parseI2PAddress(addr)
			if err != nil {
				return 0, errors.New("i2pbind: could not parse sender address")
			}

			if isMeshProtocolMessage(packets[0][:numBytes]) {
				b.handleMeshMessage(packets[0], numBytes, i2pAddr, meshHandler)
				continue
			}

			sizes[0] = numBytes
			eps[0] = &I2PEndpoint{dest: i2pAddr}
			return 1, nil
		}
	}
}

// Close shuts down the I2P session
func (b *I2PBind) Close() error {
	log.WithField("name", b.name).Debug("closing I2P bind")
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		log.Debug("bind already closed")
		return nil
	}
	b.closed = true

	var errs []error

	if b.packetConn != nil {
		log.Debug("closing packet connection")
		if err := b.packetConn.Close(); err != nil {
			log.WithError(err).Warn("error closing packet connection")
			errs = append(errs, err)
		}
		b.packetConn = nil
	}

	if b.garlic != nil {
		log.Debug("closing garlic session")
		if err := b.garlic.Close(); err != nil {
			log.WithError(err).Warn("error closing garlic session")
			errs = append(errs, err)
		}
		b.garlic = nil
	}

	if len(errs) > 0 {
		log.WithField("errorCount", len(errs)).Warn("bind closed with errors")
		return errs[0]
	}
	log.Debug("bind closed successfully")
	return nil
}

// SetMark sets the socket mark - not applicable for I2P
func (b *I2PBind) SetMark(mark uint32) error {
	// I2P doesn't support socket marks, this is a no-op
	return nil
}

// Send transmits packets to the specified I2P endpoint
func (b *I2PBind) Send(bufs [][]byte, endpoint conn.Endpoint) error {
	b.mu.Lock()
	pc := b.packetConn
	b.mu.Unlock()

	if pc == nil {
		log.Debug("send called on closed bind")
		return net.ErrClosed
	}

	i2pEp, ok := endpoint.(*I2PEndpoint)
	if !ok {
		log.Warn("wrong endpoint type for send")
		return conn.ErrWrongEndpointType
	}

	log.WithField("dest", i2pEp.dest.Base32()).WithField("buffers", len(bufs)).Debug("sending packets")

	// Send each buffer as a separate I2P datagram
	for _, buf := range bufs {
		if len(buf) > MaxI2PDatagramSize {
			log.WithField("size", len(buf)).Error("datagram exceeds I2P maximum size")
			return errors.New("i2pbind: datagram exceeds I2P maximum size (31KB)")
		}

		_, err := pc.WriteTo(buf, i2pEp.dest)
		if err != nil {
			log.WithError(err).Error("failed to send datagram")
			return err
		}
	}

	return nil
}

// ParseEndpoint parses an I2P address string into an endpoint
// Accepts base32 addresses (xxx.b32.i2p) or full base64 destinations
func (b *I2PBind) ParseEndpoint(s string) (conn.Endpoint, error) {
	log.WithField("endpoint", s).Debug("parsing endpoint")
	addr, err := i2pkeys.NewI2PAddrFromString(s)
	if err != nil {
		log.WithError(err).WithField("endpoint", s).Error("failed to parse endpoint")
		return nil, err
	}
	return &I2PEndpoint{dest: addr}, nil
}

// BatchSize returns the number of packets to batch - I2P doesn't support batching
func (b *I2PBind) BatchSize() int {
	return 1
}

// LocalAddress returns this node's I2P destination address in base32 format.
//
// This method will return an error if:
//   - The bind has not been opened yet (call Open() first)
//   - The SAM bridge is not running or accessible
//   - The I2P session failed to establish
//
// After a successful Open() call, LocalAddress() should always succeed.
// If it fails after Open(), the I2P session may have been terminated.
func (b *I2PBind) LocalAddress() (string, error) {
	log.Debug("getting local I2P address")
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.garlic == nil {
		log.Warn("cannot get local address: bind not open")
		return "", errors.New("i2pbind: bind not open")
	}

	// Get the I2P keys from the Garlic session
	keys, err := b.garlic.Keys()
	if err != nil {
		log.WithError(err).Error("failed to get I2P keys")
		return "", err
	}

	addr := keys.Addr().Base32()
	log.WithField("address", addr).Debug("local I2P address retrieved")
	return addr, nil
}

// LocalDestination returns the full I2P destination.
//
// This method will return an error if:
//   - The bind has not been opened yet (call Open() first)
//
// After a successful Open() call, LocalDestination() should always succeed.
func (b *I2PBind) LocalDestination() (i2pkeys.I2PAddr, error) {
	log.Debug("getting local I2P destination")
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.garlic == nil {
		log.Warn("cannot get local destination: bind not open")
		return "", errors.New("i2pbind: bind not open")
	}

	// Get the I2P keys from the Garlic session
	keys, err := b.garlic.Keys()
	if err != nil {
		log.WithError(err).Error("failed to get I2P keys")
		return "", err
	}

	log.Debug("local I2P destination retrieved")
	return keys.Addr(), nil
}
