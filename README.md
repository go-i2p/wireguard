# WireGuard over I2P

A Go library that enables WireGuard tunnels to run over the [I2P](https://geti2p.net/) anonymizing network. This package provides a `conn.Bind` implementation for [wireguard-go](https://git.zx2c4.com/wireguard-go/) that uses I2P datagrams instead of UDP.

In addition to the low-level bind interface, this library includes a complete **mesh VPN implementation** with peer discovery, gossip routing, invite-based authentication, and management interfaces (RPC, TUI, Web UI).

## Features

### Core Transport (`i2pbind`)
- Drop-in replacement for WireGuard's standard UDP transport
- Leverages I2P's anonymity and NAT traversal capabilities
- Maximum 31KB datagram size (I2P protocol limit; WireGuard packets are typically <1.5KB)
- Compatible with wireguard-go's device API

### Mesh VPN (`lib/*`)
- **Gossip-based peer discovery** - automatic mesh network formation
- **Invite system** - secure peer authentication with expiring invite codes
- **Routing table** - automatic IP-to-peer routing with collision detection
- **Multiple interfaces** - JSON-RPC, Terminal UI (BubbleTea), and Web UI

## Prerequisites

- Go 1.24 or later
- Running I2P router with SAM enabled (default port 7656)

## Installation

```bash
go get github.com/go-i2p/wireguard
```

## Quick Start

### Minimal Example (Transport Layer Only)

This example shows the minimum code to create a WireGuard device over I2P. **Note:** You must configure the device with `IpcSet()` and call `Up()` before it becomes functional.

```go
package main

import (
    "encoding/hex"
    "log"
    "net/netip"
    "os"
    "os/signal"

    "github.com/go-i2p/wireguard/i2pbind"
    "golang.zx2c4.com/wireguard/device"
    "golang.zx2c4.com/wireguard/tun/netstack"
    "golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func main() {
    // Create netstack TUN (userspace network stack)
    tun, _, err := netstack.CreateNetTUN(
        []netip.Addr{netip.MustParseAddr("10.0.0.2")},
        []netip.Addr{netip.MustParseAddr("8.8.8.8")},
        1280, // MTU reduced for I2P overhead
    )
    if err != nil {
        log.Fatalf("Failed to create TUN: %v", err)
    }

    // Create I2P bind with a tunnel name
    bind := i2pbind.NewI2PBind("my-wireguard-tunnel")

    // Create WireGuard device with I2P transport
    logger := device.NewLogger(device.LogLevelError, "(wg) ")
    dev := device.NewDevice(tun, bind, logger)

    // Generate or load a WireGuard private key
    privKey, _ := wgtypes.GeneratePrivateKey()
    
    // Configure the device with our private key
    config := "private_key=" + hex.EncodeToString(privKey[:]) + "\n"
    
    // Add a peer (if you have one - replace with actual peer details)
    // peerPubKey, _ := wgtypes.ParseKey("PEER_PUBLIC_KEY_BASE64")
    // config += "public_key=" + hex.EncodeToString(peerPubKey[:]) + "\n"
    // config += "endpoint=PEER_I2P_ADDRESS.b32.i2p\n"
    // config += "allowed_ip=10.0.0.0/24\n"
    // config += "persistent_keepalive_interval=25\n"
    
    if err := dev.IpcSet(config); err != nil {
        log.Fatalf("Failed to configure: %v", err)
    }

    // Bring up the device
    if err := dev.Up(); err != nil {
        log.Fatalf("Failed to start: %v", err)
    }

    // Print our I2P address for peer exchange
    if addr, err := bind.LocalAddress(); err == nil {
        log.Printf("I2P Address: %s", addr)
    }

    // Wait for interrupt
    sig := make(chan os.Signal, 1)
    signal.Notify(sig, os.Interrupt)
    <-sig
    dev.Close()
}
```

### Complete Example

For a full working example with peer configuration, signal handling, and command-line flags, see the [i2pbind/example](i2pbind/example) directory:

```bash
cd i2pbind/example
go build -o wg-i2p
./wg-i2p -help

# Example: Start a node
./wg-i2p -name peer1 -privkey $(wg genkey)

# Example: Connect to a peer
./wg-i2p -name peer2 -privkey $(wg genkey) \
    -peer-pubkey <peer1-public-key> \
    -peer-endpoint <peer1-i2p-address>.b32.i2p
```

## Mesh VPN Architecture

For higher-level mesh VPN functionality, use the `lib/` packages:

### Package Overview

| Package | Description |
|---------|-------------|
| `lib/core` | Main node orchestration - manages identity, transport, mesh, and RPC |
| `lib/embedded` | Embeddable VPN API for integrating into other applications |
| `lib/identity` | Cryptographic identity management (WireGuard + Ed25519 keys) |
| `lib/mesh` | Gossip protocol, peer discovery, and routing table |
| `lib/transport` | I2P transport with peer tracking and health monitoring |
| `lib/rpc` | JSON-RPC server/client for node control |
| `lib/web` | Browser-based management UI |
| `lib/tui` | Terminal UI using BubbleTea |
| `lib/metrics` | Prometheus exposition format metrics (lightweight, no external deps) |
| `lib/ratelimit` | Invite acceptance rate limiting |

### Using the Embedded VPN

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/go-i2p/wireguard/lib/embedded"
)

func main() {
    cfg := embedded.Config{
        NodeName:    "my-node",
        DataDir:     "./data",
        SAMAddress:  "127.0.0.1:7656",
        EnableRPC:   true,
        RPCSocket:   "rpc.sock",  // Unix socket in DataDir
    }

    vpn, err := embedded.New(cfg)
    if err != nil {
        log.Fatal(err)
    }

    // Subscribe to events via channel
    go func() {
        for e := range vpn.Events() {
            log.Printf("Event: %s", e.Type)
        }
    }()

    ctx := context.Background()
    if err := vpn.Start(ctx); err != nil {
        log.Fatal(err)
    }
    defer vpn.Stop(ctx)

    // Create an invite for a peer (24h expiry, single use)
    invite, _ := vpn.CreateInvite(24*time.Hour, 1)
    log.Printf("Invite: %s", invite)

    // Get status
    status := vpn.Status()
    log.Printf("Node ID: %s, Peers: %d", status.NodeID, status.PeerCount)

    // Block forever
    select {}
}
```

### Invite System

Peers join the mesh using invite codes that contain:
- The inviter's I2P destination
- A one-time authentication token
- Network ID and optional metadata

When you create the first invite on a fresh node, a new mesh network is automatically created with a randomly generated Network ID. Subsequent nodes that accept this invite will inherit the same Network ID, forming a unified mesh network.

```go
// Create an invite (inviter side)
// If this is a new node, a Network ID is automatically generated
inviteCode, err := vpn.CreateInvite(24*time.Hour, 1) // 24h expiry, single use
// Returns: i2plan://eyJpMnBfZGVzdCI6Ii4uLiIsImF1dGhfdG9rZW4iOiIuLi4ifQ==

// Accept an invite (joiner side)
// The node inherits the Network ID from the invite
ctx := context.Background()
err = vpn.AcceptInvite(ctx, "i2plan://...")
```

### CLI Commands

The `i2plan` command provides subcommands for managing the mesh VPN:

```bash
# Start the node daemon (runs in foreground)
i2plan

# Launch the interactive Terminal UI (connects to running node via RPC)
i2plan tui

# Start the Web UI server (connects to running node via RPC)
i2plan web

# Execute RPC commands directly
i2plan rpc status
i2plan rpc peers.list
i2plan rpc invite.create
```

**Architecture Note:** The TUI and Web interfaces are **clients** that connect to the running node via RPC. They don't start their own node - they control an existing one. Start the node daemon first, then connect with `i2plan tui` or `i2plan web` in a separate terminal.

## How It Works

The library implements WireGuard's `conn.Bind` interface, replacing UDP sockets with I2P datagram sessions. Peer endpoints use I2P destination addresses (base32 format) instead of IP addresses.

```
┌─────────────────────────────────────────────────────────────┐
│                lib/web (UI) │ lib/tui (UI)                   │
│                   ↓ RPC client connection ↓                  │
├─────────────────────────────────────────────────────────────┤
│                      Application                             │
├─────────────────────────────────────────────────────────────┤
│                   lib/embedded (VPN API)                     │
├─────────────────────────────────────────────────────────────┤
│  lib/core    │  lib/mesh   │  lib/rpc                        │
│  (Node)      │  (Gossip)   │  (Control Server)               │
├──────────────┼─────────────┴────────────────────────────────┤
│ lib/identity │              lib/transport                    │
│  (Keys)      │              (Peer Tracking)                  │
├──────────────┴──────────────────────────────────────────────┤
│                      i2pbind (conn.Bind)                     │
├─────────────────────────────────────────────────────────────┤
│                   wireguard-go (WireGuard)                   │
├─────────────────────────────────────────────────────────────┤
│                      I2P SAM Bridge                          │
└─────────────────────────────────────────────────────────────┘
```

## Building with Version Info

```bash
go build -ldflags "-X github.com/go-i2p/wireguard/version.Version=1.0.0 \
    -X github.com/go-i2p/wireguard/version.GitCommit=$(git rev-parse --short HEAD) \
    -X github.com/go-i2p/wireguard/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```

## License

See [LICENSE](LICENSE) file.
