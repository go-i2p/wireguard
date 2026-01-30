// Example demonstrates basic usage of the embedded VPN API.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-i2p/wireguard/lib/embedded"
)

func main() {
	vpn := createVPN()
	defer vpn.Close()

	go handleEvents(vpn)

	startVPN(vpn)
	printStatus(vpn)
	tryCreateInvite(vpn)
	waitForShutdown()
}

// createVPN creates and returns a new VPN instance.
func createVPN() *embedded.VPN {
	vpn, err := embedded.New(embedded.Config{
		NodeName: "example-vpn",
		DataDir:  "./vpn-data",
	})
	if err != nil {
		log.Fatalf("Failed to create VPN: %v", err)
	}
	return vpn
}

// handleEvents processes VPN events in the background.
func handleEvents(vpn *embedded.VPN) {
	for event := range vpn.Events() {
		handleEvent(event)
	}
}

// handleEvent processes a single VPN event.
func handleEvent(event embedded.Event) {
	switch event.Type {
	case embedded.EventStarted:
		fmt.Println("🟢 VPN started")
	case embedded.EventStopped:
		fmt.Println("🔴 VPN stopped")
	case embedded.EventPeerConnected:
		if event.Peer != nil {
			fmt.Printf("👤 Peer connected: %s (%s)\n", event.Peer.NodeID, event.Peer.TunnelIP)
		}
	case embedded.EventPeerDisconnected:
		if event.Peer != nil {
			fmt.Printf("👤 Peer disconnected: %s\n", event.Peer.NodeID)
		}
	case embedded.EventError:
		fmt.Printf("⚠️  Error: %v\n", event.Error)
	default:
		fmt.Printf("📢 Event: %s - %s\n", event.Type, event.Message)
	}
}

// startVPN starts the VPN service.
func startVPN(vpn *embedded.VPN) {
	ctx := context.Background()
	if err := vpn.Start(ctx); err != nil {
		log.Fatalf("Failed to start VPN: %v", err)
	}
}

// printStatus prints the current VPN status.
func printStatus(vpn *embedded.VPN) {
	status := vpn.Status()
	fmt.Printf("\n=== VPN Status ===\n")
	fmt.Printf("State:     %s\n", status.State)
	fmt.Printf("Node:      %s\n", status.NodeName)
	fmt.Printf("Tunnel IP: %s\n", vpn.TunnelIP())
	fmt.Printf("I2P Addr:  %s\n", vpn.I2PAddress())
}

// tryCreateInvite attempts to create an invite code.
func tryCreateInvite(vpn *embedded.VPN) {
	invite, err := vpn.CreateInvite(24*time.Hour, 1)
	if err != nil {
		fmt.Printf("Note: Invite creation pending mesh implementation: %v\n", err)
	} else {
		fmt.Printf("\n=== Invite Code ===\n%s\n", invite)
	}
}

// waitForShutdown waits for a shutdown signal.
func waitForShutdown() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("\nPress Ctrl+C to stop...")
	<-sigCh

	fmt.Println("\nShutting down...")
}
