// Example: WireGuard over I2P
//
// This example demonstrates how to set up a WireGuard tunnel over I2P
// using the i2pbind package and netstack for userspace networking.
//
// Prerequisites:
//   - Running I2P router with SAM enabled on port 7656
//   - Exchange I2P addresses with peer out-of-band
//
// Build:
//
//	go build -o wg-i2p ./i2pbind/example
//
// Usage:
//
//	# On peer 1:
//	./wg-i2p -name peer1 -privkey <base64-private-key>
//
//	# On peer 2 (after getting peer1's I2P address):
//	./wg-i2p -name peer2 -privkey <base64-private-key> \
//	    -peer-pubkey <peer1-public-key> \
//	    -peer-endpoint <peer1-i2p-address>.b32.i2p

package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-i2p/logger"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"

	"github.com/go-i2p/wireguard/i2pbind"
)

var log = logger.GetGoI2PLogger()

// cliConfig holds parsed command-line configuration.
type cliConfig struct {
	name           string
	samAddr        string
	privKey        string
	localIP        string
	peerPubKey     string
	peerEndpoint   string
	peerAllowedIPs string
	mtu            int
	verbose        bool
}

func main() {
	cfg := parseFlags()
	validatePrivateKey(cfg.privKey)

	logger := createLogger(cfg)
	tun := createTUN(cfg)
	bind := i2pbind.NewI2PBindWithSAM(cfg.name, cfg.samAddr)
	dev := createDevice(tun, bind, logger)

	configureDevice(dev, cfg)
	startDevice(dev)
	printI2PAddress(bind)
	waitForShutdown(dev)
}

// parseFlags parses command-line flags and returns configuration.
func parseFlags() *cliConfig {
	cfg := &cliConfig{}
	flag.StringVar(&cfg.name, "name", "wg-i2p", "Tunnel name for I2P")
	flag.StringVar(&cfg.samAddr, "sam", "127.0.0.1:7656", "SAM bridge address")
	flag.StringVar(&cfg.privKey, "privkey", "", "WireGuard private key (base64)")
	flag.StringVar(&cfg.localIP, "ip", "10.0.0.2", "Local tunnel IP address")
	flag.StringVar(&cfg.peerPubKey, "peer-pubkey", "", "Peer's WireGuard public key (base64)")
	flag.StringVar(&cfg.peerEndpoint, "peer-endpoint", "", "Peer's I2P address (xxx.b32.i2p)")
	flag.StringVar(&cfg.peerAllowedIPs, "peer-allowed", "10.0.0.0/24", "Peer's allowed IPs")
	flag.IntVar(&cfg.mtu, "mtu", 1280, "MTU (reduced for I2P overhead)")
	flag.BoolVar(&cfg.verbose, "v", false, "Verbose logging")
	flag.Parse()
	return cfg
}

// validatePrivateKey exits if no private key is provided.
func validatePrivateKey(privKey string) {
	if privKey == "" {
		log.Warn("No private key provided. Generate one with: wg genkey")
		log.Warn("Then derive public key with: echo <privkey> | wg pubkey")
		os.Exit(1)
	}
}

// createLogger creates a WireGuard device logger.
func createLogger(cfg *cliConfig) *device.Logger {
	logLevel := device.LogLevelError
	if cfg.verbose {
		logLevel = device.LogLevelVerbose
	}
	return device.NewLogger(logLevel, fmt.Sprintf("(%s) ", cfg.name))
}

// createTUN creates a netstack TUN device.
func createTUN(cfg *cliConfig) *netstack.Net {
	tunIP := netip.MustParseAddr(cfg.localIP)
	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{tunIP},
		[]netip.Addr{netip.MustParseAddr("8.8.8.8")},
		cfg.mtu,
	)
	if err != nil {
		log.WithError(err).Fatal("Failed to create TUN")
	}
	_ = tun
	return tnet
}

// createDevice creates a WireGuard device with I2P transport.
func createDevice(tnet *netstack.Net, bind *i2pbind.I2PBind, logger *device.Logger) *device.Device {
	// Note: we need the tun.Device, not the Net
	tunIP := netip.MustParseAddr("10.0.0.2")
	tun, _, err := netstack.CreateNetTUN(
		[]netip.Addr{tunIP},
		[]netip.Addr{netip.MustParseAddr("8.8.8.8")},
		1280,
	)
	if err != nil {
		log.WithError(err).Fatal("Failed to create TUN")
	}
	return device.NewDevice(tun, bind, logger)
}

// configureDevice configures the WireGuard device with keys and peer info.
func configureDevice(dev *device.Device, cfg *cliConfig) {
	config := buildDeviceConfig(cfg)
	if err := dev.IpcSet(config); err != nil {
		log.WithError(err).Fatal("Failed to configure device")
	}
}

// buildDeviceConfig builds the WireGuard UAPI configuration string.
func buildDeviceConfig(cfg *cliConfig) string {
	config := fmt.Sprintf("private_key=%s\n", toHex(cfg.privKey))

	if cfg.peerPubKey != "" && cfg.peerEndpoint != "" {
		config += fmt.Sprintf("public_key=%s\n", toHex(cfg.peerPubKey))
		config += fmt.Sprintf("endpoint=%s\n", cfg.peerEndpoint)
		config += fmt.Sprintf("allowed_ip=%s\n", cfg.peerAllowedIPs)
		config += "persistent_keepalive_interval=25\n"
	}

	return config
}

// startDevice brings up the WireGuard device.
func startDevice(dev *device.Device) {
	if err := dev.Up(); err != nil {
		log.WithError(err).Fatal("Failed to bring up device")
	}
}

// printI2PAddress prints the local I2P address.
func printI2PAddress(bind *i2pbind.I2PBind) {
	localAddr, err := bind.LocalAddress()
	if err != nil {
		log.WithError(err).Warn("Could not get local I2P address")
		return
	}
	log.Info("===========================================")
	log.Info("WireGuard over I2P is running!")
	log.WithField("address", localAddr).Info("Our I2P address")
	log.Info("Share this address with peers to connect")
	log.Info("===========================================")
}

// waitForShutdown waits for shutdown signal and closes the device.
func waitForShutdown(dev *device.Device) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Info("Shutting down...")
	dev.Close()
}

// toHex converts a base64 key to hex format (WireGuard UAPI uses hex)
func toHex(b64 string) string {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		log.WithError(err).Fatal("Invalid base64 key")
	}
	return fmt.Sprintf("%x", data)
}

/**/
