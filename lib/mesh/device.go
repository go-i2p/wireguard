// Package mesh provides WireGuard device management for the i2plan mesh network.
// This file implements a wrapper around wireguard-go for dynamic peer configuration.
package mesh

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"sync"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/tun/netstack"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// DeviceConfig configures the WireGuard device.
type DeviceConfig struct {
	// PrivateKey is our WireGuard private key.
	PrivateKey wgtypes.Key
	// TunnelIP is our IP address in the mesh tunnel.
	TunnelIP netip.Addr
	// Subnet is the mesh network subnet.
	Subnet netip.Prefix
	// ListenPort is the WireGuard UDP port (0 for random).
	ListenPort uint16
	// MTU is the tunnel MTU.
	MTU int
	// Bind is the WireGuard network binding (e.g., i2pbind.I2PBind for I2P transport).
	// If nil, defaults to conn.NewDefaultBind() for standard UDP.
	Bind conn.Bind
}

// Device wraps a wireguard-go device for mesh network use.
type Device struct {
	mu  sync.RWMutex
	dev *device.Device
	net *netstack.Net
	tun tun.Device

	// Configuration
	privateKey wgtypes.Key
	tunnelIP   netip.Addr
	subnet     netip.Prefix
	listenPort uint16

	// Peer tracking
	peers map[wgtypes.Key]*DevicePeer

	// State
	closed bool
}

// DevicePeer tracks a WireGuard peer configured on the device.
type DevicePeer struct {
	PublicKey  wgtypes.Key
	AllowedIPs []netip.Prefix
	Endpoint   string // I2P endpoint via i2pbind
}

// NewDevice creates a new WireGuard device for the mesh network.
// normalizeDeviceConfig sets default values for missing configuration.
func normalizeDeviceConfig(cfg *DeviceConfig) {
	if cfg.MTU <= 0 {
		cfg.MTU = 1280 // Safe default for I2P
	}
}

// validateDeviceConfig checks that the device configuration is valid.
func validateDeviceConfig(cfg *DeviceConfig) error {
	if !cfg.TunnelIP.IsValid() {
		return errors.New("invalid tunnel IP")
	}
	if !cfg.Subnet.IsValid() {
		return errors.New("invalid subnet")
	}
	if !cfg.Subnet.Contains(cfg.TunnelIP) {
		return errors.New("tunnel IP not in subnet")
	}
	return nil
}

// createNetstackTUN creates the netstack TUN device with the given configuration.
func createNetstackTUN(cfg *DeviceConfig) (tun.Device, *netstack.Net, error) {
	tun, net, err := netstack.CreateNetTUN(
		[]netip.Addr{cfg.TunnelIP},
		[]netip.Addr{}, // No DNS
		cfg.MTU,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("creating netstack TUN: %w", err)
	}
	return tun, net, nil
}

// initializeWireGuardDevice creates and configures the WireGuard device.
func initializeWireGuardDevice(tunDev tun.Device, cfg *DeviceConfig) (*device.Device, error) {
	bind := cfg.Bind
	if bind == nil {
		bind = conn.NewDefaultBind()
	}

	dev := device.NewDevice(tunDev, bind, device.NewLogger(device.LogLevelSilent, ""))

	ipcConfig := fmt.Sprintf("private_key=%s\n", hexKey(cfg.PrivateKey))
	if cfg.ListenPort > 0 {
		ipcConfig += fmt.Sprintf("listen_port=%d\n", cfg.ListenPort)
	}

	if err := dev.IpcSet(ipcConfig); err != nil {
		dev.Close()
		return nil, fmt.Errorf("configuring device: %w", err)
	}

	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("bringing up device: %w", err)
	}

	return dev, nil
}

func NewDevice(cfg DeviceConfig) (*Device, error) {
	normalizeDeviceConfig(&cfg)

	if err := validateDeviceConfig(&cfg); err != nil {
		return nil, err
	}

	tunDev, net, err := createNetstackTUN(&cfg)
	if err != nil {
		return nil, err
	}

	dev, err := initializeWireGuardDevice(tunDev, &cfg)
	if err != nil {
		return nil, err
	}

	log.WithField("tunnel_ip", cfg.TunnelIP).WithField("subnet", cfg.Subnet).WithField("mtu", cfg.MTU).Info("created WireGuard device")

	return &Device{
		dev:        dev,
		net:        net,
		tun:        tunDev,
		privateKey: cfg.PrivateKey,
		tunnelIP:   cfg.TunnelIP,
		subnet:     cfg.Subnet,
		listenPort: cfg.ListenPort,
		peers:      make(map[wgtypes.Key]*DevicePeer),
	}, nil
}

// AddPeer adds a WireGuard peer to the device.
func (d *Device) AddPeer(publicKey wgtypes.Key, allowedIPs []netip.Prefix, endpoint string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return errors.New("device is closed")
	}

	// Build IPC configuration for the peer
	var ipc strings.Builder
	ipc.WriteString(fmt.Sprintf("public_key=%s\n", hexKey(publicKey)))

	// Set endpoint if provided
	if endpoint != "" {
		ipc.WriteString(fmt.Sprintf("endpoint=%s\n", endpoint))
	}

	// Set allowed IPs
	ipc.WriteString("replace_allowed_ips=true\n")
	for _, ip := range allowedIPs {
		ipc.WriteString(fmt.Sprintf("allowed_ip=%s\n", ip.String()))
	}

	// Enable persistent keepalive for I2P
	ipc.WriteString("persistent_keepalive_interval=25\n")

	log.Debug("adding peer", "public_key", publicKey.String()[:8]+"...", "allowed_ips", allowedIPs)

	if err := d.dev.IpcSet(ipc.String()); err != nil {
		return fmt.Errorf("adding peer: %w", err)
	}

	// Track the peer
	d.peers[publicKey] = &DevicePeer{
		PublicKey:  publicKey,
		AllowedIPs: allowedIPs,
		Endpoint:   endpoint,
	}

	log.Info("added peer",
		"public_key", publicKey.String()[:8]+"...",
		"allowed_ips", allowedIPs,
		"endpoint", endpoint)

	return nil
}

// RemovePeer removes a WireGuard peer from the device.
func (d *Device) RemovePeer(publicKey wgtypes.Key) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return errors.New("device is closed")
	}

	ipc := fmt.Sprintf("public_key=%s\nremove=true\n", hexKey(publicKey))

	if err := d.dev.IpcSet(ipc); err != nil {
		return fmt.Errorf("removing peer: %w", err)
	}

	delete(d.peers, publicKey)

	log.Info("removed peer", "public_key", publicKey.String()[:8]+"...")

	return nil
}

// UpdatePeerEndpoint updates a peer's endpoint.
func (d *Device) UpdatePeerEndpoint(publicKey wgtypes.Key, endpoint string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return errors.New("device is closed")
	}

	peer, ok := d.peers[publicKey]
	if !ok {
		return errors.New("peer not found")
	}

	ipc := fmt.Sprintf("public_key=%s\nendpoint=%s\n", hexKey(publicKey), endpoint)

	if err := d.dev.IpcSet(ipc); err != nil {
		return fmt.Errorf("updating endpoint: %w", err)
	}

	peer.Endpoint = endpoint

	log.Debug("updated peer endpoint", "public_key", publicKey.String()[:8]+"...", "endpoint", endpoint)

	return nil
}

// GetPeer returns information about a configured peer.
func (d *Device) GetPeer(publicKey wgtypes.Key) (*DevicePeer, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	peer, ok := d.peers[publicKey]
	if !ok {
		return nil, false
	}

	// Return a copy
	return &DevicePeer{
		PublicKey:  peer.PublicKey,
		AllowedIPs: append([]netip.Prefix{}, peer.AllowedIPs...),
		Endpoint:   peer.Endpoint,
	}, true
}

// ListPeers returns all configured peers.
func (d *Device) ListPeers() []*DevicePeer {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]*DevicePeer, 0, len(d.peers))
	for _, peer := range d.peers {
		result = append(result, &DevicePeer{
			PublicKey:  peer.PublicKey,
			AllowedIPs: append([]netip.Prefix{}, peer.AllowedIPs...),
			Endpoint:   peer.Endpoint,
		})
	}
	return result
}

// PeerCount returns the number of configured peers.
func (d *Device) PeerCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.peers)
}

// Net returns the netstack network for making connections through the tunnel.
func (d *Device) Net() *netstack.Net {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.net
}

// TunnelIP returns our tunnel IP address.
func (d *Device) TunnelIP() netip.Addr {
	return d.tunnelIP
}

// Subnet returns the mesh network subnet.
func (d *Device) Subnet() netip.Prefix {
	return d.subnet
}

// PublicKey returns our WireGuard public key.
func (d *Device) PublicKey() wgtypes.Key {
	return d.privateKey.PublicKey()
}

// Close shuts down the WireGuard device.
func (d *Device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}

	d.closed = true
	d.dev.Close()

	log.Info("closed WireGuard device")

	return nil
}

// IsClosed returns whether the device is closed.
func (d *Device) IsClosed() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.closed
}

// hexKey converts a WireGuard key to hex format for IPC.
func hexKey(key wgtypes.Key) string {
	return fmt.Sprintf("%x", key[:])
}
