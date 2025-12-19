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

	"github.com/go-i2p/i2pkeys"
	"github.com/go-i2p/onramp"
	"github.com/go-i2p/sam3"

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

// DstToBytes returns bytes representation of the destination for MAC2 cookies
func (e *I2PEndpoint) DstToBytes() []byte {
	// Return the hash of the destination for cookie calculations
	hash := e.dest.DestHash()
	return []byte(hash.Hash())
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

// I2PBind implements conn.Bind using I2P datagrams for transport
type I2PBind struct {
	mu sync.Mutex

	// Configuration
	name    string // Tunnel name for I2P
	samAddr string // SAM bridge address

	// I2P session components
	garlic          *onramp.Garlic
	datagramSession *sam3.DatagramSession

	// State
	closed bool
}

// NewI2PBind creates a new I2P Bind with default settings
func NewI2PBind(name string) *I2PBind {
	return NewI2PBindWithSAM(name, DefaultSAMAddress)
}

// NewI2PBindWithSAM creates a new I2P Bind with a custom SAM address
func NewI2PBindWithSAM(name, samAddr string) *I2PBind {
	return &I2PBind{
		name:    name,
		samAddr: samAddr,
	}
}

// Open initializes the I2P datagram session and returns receive functions
func (b *I2PBind) Open(port uint16) ([]conn.ReceiveFunc, uint16, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.datagramSession != nil {
		return nil, 0, conn.ErrBindAlreadyOpen
	}

	// Create a new Garlic session
	garlic, err := onramp.NewGarlic(b.name, b.samAddr, onramp.OPT_DEFAULTS)
	if err != nil {
		return nil, 0, err
	}
	b.garlic = garlic

	// Get a PacketConn (which is a DatagramSession internally)
	packetConn, err := garlic.ListenPacket()
	if err != nil {
		garlic.Close()
		return nil, 0, err
	}

	// Type assert to get the DatagramSession for full functionality
	ds, ok := packetConn.(*sam3.DatagramSession)
	if !ok {
		packetConn.Close()
		garlic.Close()
		return nil, 0, errors.New("i2pbind: expected DatagramSession from ListenPacket")
	}
	b.datagramSession = ds
	b.closed = false

	// Create receive function
	recvFunc := b.makeReceiveFunc()

	// Port is not meaningful for I2P, return 0
	return []conn.ReceiveFunc{recvFunc}, 0, nil
}

// makeReceiveFunc creates the receive function for incoming I2P datagrams
func (b *I2PBind) makeReceiveFunc() conn.ReceiveFunc {
	return func(packets [][]byte, sizes []int, eps []conn.Endpoint) (n int, err error) {
		b.mu.Lock()
		ds := b.datagramSession
		b.mu.Unlock()

		if ds == nil {
			return 0, net.ErrClosed
		}

		// Read one datagram (I2P doesn't support batch reading like UDP GSO)
		if len(packets) == 0 {
			return 0, nil
		}

		numBytes, addr, err := ds.ReadFrom(packets[0])
		if err != nil {
			return 0, err
		}

		sizes[0] = numBytes

		// Convert the address to our I2P endpoint type
		i2pAddr, ok := addr.(i2pkeys.I2PAddr)
		if !ok {
			// Try string conversion as fallback
			addrStr := addr.String()
			i2pAddr, err = i2pkeys.NewI2PAddrFromString(addrStr)
			if err != nil {
				return 0, errors.New("i2pbind: could not parse sender address")
			}
		}

		eps[0] = &I2PEndpoint{dest: i2pAddr}
		return 1, nil
	}
}

// Close shuts down the I2P session
func (b *I2PBind) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}
	b.closed = true

	var errs []error

	if b.datagramSession != nil {
		if err := b.datagramSession.Close(); err != nil {
			errs = append(errs, err)
		}
		b.datagramSession = nil
	}

	if b.garlic != nil {
		if err := b.garlic.Close(); err != nil {
			errs = append(errs, err)
		}
		b.garlic = nil
	}

	if len(errs) > 0 {
		return errs[0]
	}
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
	ds := b.datagramSession
	b.mu.Unlock()

	if ds == nil {
		return net.ErrClosed
	}

	i2pEp, ok := endpoint.(*I2PEndpoint)
	if !ok {
		return conn.ErrWrongEndpointType
	}

	// Send each buffer as a separate I2P datagram
	for _, buf := range bufs {
		if len(buf) > MaxI2PDatagramSize {
			return errors.New("i2pbind: datagram exceeds I2P maximum size (31KB)")
		}

		_, err := ds.WriteTo(buf, i2pEp.dest)
		if err != nil {
			return err
		}
	}

	return nil
}

// ParseEndpoint parses an I2P address string into an endpoint
// Accepts base32 addresses (xxx.b32.i2p) or full base64 destinations
func (b *I2PBind) ParseEndpoint(s string) (conn.Endpoint, error) {
	addr, err := i2pkeys.NewI2PAddrFromString(s)
	if err != nil {
		return nil, err
	}
	return &I2PEndpoint{dest: addr}, nil
}

// BatchSize returns the number of packets to batch - I2P doesn't support batching
func (b *I2PBind) BatchSize() int {
	return 1
}

// LocalAddress returns this node's I2P destination address
func (b *I2PBind) LocalAddress() (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.datagramSession == nil {
		return "", errors.New("i2pbind: bind not open")
	}

	return b.datagramSession.LocalI2PAddr().Base32(), nil
}

// LocalDestination returns the full I2P destination
func (b *I2PBind) LocalDestination() (i2pkeys.I2PAddr, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.datagramSession == nil {
		return "", errors.New("i2pbind: bind not open")
	}

	return b.datagramSession.LocalI2PAddr(), nil
}