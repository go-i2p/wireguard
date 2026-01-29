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
	"log"
	"net/netip"
	"os"
	"os/signal"
	"syscall"

	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"

	"github.com/go-i2p/wireguard/i2pbind"
)

func main() {
	// Parse flags
	name := flag.String("name", "wg-i2p", "Tunnel name for I2P")
	samAddr := flag.String("sam", "127.0.0.1:7656", "SAM bridge address")
	privKey := flag.String("privkey", "", "WireGuard private key (base64)")
	localIP := flag.String("ip", "10.0.0.2", "Local tunnel IP address")
	peerPubKey := flag.String("peer-pubkey", "", "Peer's WireGuard public key (base64)")
	peerEndpoint := flag.String("peer-endpoint", "", "Peer's I2P address (xxx.b32.i2p)")
	peerAllowedIPs := flag.String("peer-allowed", "10.0.0.0/24", "Peer's allowed IPs")
	mtu := flag.Int("mtu", 1280, "MTU (reduced for I2P overhead)")
	verbose := flag.Bool("v", false, "Verbose logging")
	flag.Parse()

	if *privKey == "" {
		// Generate a new key if none provided
		log.Println("No private key provided. Generate one with: wg genkey")
		log.Println("Then derive public key with: echo <privkey> | wg pubkey")
		os.Exit(1)
	}

	// Set up logging
	logLevel := device.LogLevelError
	if *verbose {
		logLevel = device.LogLevelVerbose
	}
	logger := device.NewLogger(logLevel, fmt.Sprintf("(%s) ", *name))

	// Create netstack TUN (userspace network stack)
	tunIP := netip.MustParseAddr(*localIP)
	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{tunIP},
		[]netip.Addr{netip.MustParseAddr("8.8.8.8")}, // DNS
		*mtu,
	)
	if err != nil {
		log.Fatalf("Failed to create TUN: %v", err)
	}
	_ = tnet // Can be used for making connections through the tunnel

	// Create I2P bind
	bind := i2pbind.NewI2PBindWithSAM(*name, *samAddr)

	// Create WireGuard device
	dev := device.NewDevice(tun, bind, logger)

	// Configure the device
	config := fmt.Sprintf("private_key=%s\n", toHex(*privKey))

	// Add peer if specified
	if *peerPubKey != "" && *peerEndpoint != "" {
		config += fmt.Sprintf("public_key=%s\n", toHex(*peerPubKey))
		config += fmt.Sprintf("endpoint=%s\n", *peerEndpoint)
		config += fmt.Sprintf("allowed_ip=%s\n", *peerAllowedIPs)
		config += "persistent_keepalive_interval=25\n"
	}

	if err := dev.IpcSet(config); err != nil {
		log.Fatalf("Failed to configure device: %v", err)
	}

	// Bring up the device
	if err := dev.Up(); err != nil {
		log.Fatalf("Failed to bring up device: %v", err)
	}

	// Print our I2P address
	localAddr, err := bind.LocalAddress()
	if err != nil {
		log.Printf("Warning: could not get local I2P address: %v", err)
	} else {
		log.Printf("===========================================")
		log.Printf("WireGuard over I2P is running!")
		log.Printf("Our I2P address: %s", localAddr)
		log.Printf("Share this address with peers to connect")
		log.Printf("===========================================")
	}

	// Wait for shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
	dev.Close()
}

// toHex converts a base64 key to hex format (WireGuard UAPI uses hex)
func toHex(b64 string) string {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		log.Fatalf("Invalid base64 key: %v", err)
	}
	return fmt.Sprintf("%x", data)
}

/**/
