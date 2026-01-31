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

## Trust Model

This mesh VPN operates on a **trusted peer model**: all computers that join the mesh are considered trusted members of the private network. This influences several design decisions:

- **Default tunnel length is 1 hop** — provides consistent I2P connectivity without excessive latency; full anonymity is not required within a trusted mesh
- **Invite-based authentication** — only peers with valid invite codes can join the network
- **Focus on reliability over anonymity** — the mesh prioritizes stable connectivity between trusted nodes

If you require anonymity between mesh peers, increase the `TunnelLength` configuration to 2 or 3.

## Prerequisites

- **Go 1.24 or later** - For building from source
- **Running I2P router** with SAM enabled (default port 7656)
- **wireguard-go dependencies** - This project uses the Go userspace implementation, not kernel WireGuard
- **Elevated privileges** - Required for creating TUN/TAP network interfaces:
  - **Linux**: Use `sudo` or grant `CAP_NET_ADMIN` capability: `sudo setcap cap_net_admin=+ep ./i2plan`
  - **macOS**: Requires `sudo`
  - **Windows**: Run PowerShell as Administrator
  - **BSD**: Use `sudo` (FreeBSD) or `doas` (OpenBSD)

See [DEPLOYMENT.md](DEPLOYMENT.md) for detailed platform-specific setup instructions.

## Installation

```bash
go get github.com/go-i2p/wireguard
```

## Quick Start

**IMPORTANT:** WireGuard requires elevated privileges to create network interfaces. See [Prerequisites](#prerequisites) above.

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

# Run with elevated privileges (see Prerequisites)
# Linux (with capabilities):
sudo setcap cap_net_admin=+ep ./wg-i2p
./wg-i2p -name peer1 -privkey $(wg genkey)

# Or with sudo:
sudo ./wg-i2p -name peer1 -privkey $(wg genkey)

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

// Create unlimited-use invite (not recommended for security)
// Use identity.UnlimitedUses constant (-1) for unlimited uses
inviteCode, err := vpn.CreateInvite(24*time.Hour, identity.UnlimitedUses)

// Accept an invite (joiner side)
// The node inherits the Network ID from the invite
ctx := context.Background()
err = vpn.AcceptInvite(ctx, "i2plan://...")
```

**Invite Usage Limits:**
- **Single-use** (default): `maxUses: 1` - Most secure, recommended for production
- **Limited uses**: `maxUses: N` - For small groups, specify positive integer
- **Unlimited**: `maxUses: identity.UnlimitedUses` (value: -1) - Not recommended for security
- **Invalid**: `maxUses: 0` - Explicitly rejected with error message

**QR Code Sharing:**
Both TUI and Web UI support QR codes for easy invite sharing:
- **TUI**: Terminal-rendered QR codes displayed when creating invites
- **Web UI**: PNG QR codes with webcam scanning support
- See [QR_FEATURES.md](QR_FEATURES.md) for detailed documentation

### CLI Commands

The `i2plan` command provides subcommands for managing the mesh VPN:

```bash
# Build i2plan
go build -o i2plan cmd/i2plan/main.go

# Grant capabilities (Linux only)
sudo setcap cap_net_admin=+ep ./i2plan

# Start the node daemon (runs in foreground) - default when no subcommand is provided
./i2plan
# Or with sudo on macOS/Windows/BSD: sudo ./i2plan

# Check version
./i2plan --version

# Launch the interactive Terminal UI (connects to running node via RPC)
# Includes terminal-rendered QR codes for invite sharing
./i2plan tui

# Start the Web UI server (connects to running node via RPC)
# Includes PNG QR codes and webcam scanning for invites
./i2plan web

# Execute RPC commands directly
./i2plan rpc status
./i2plan rpc peers.list
./i2plan rpc invite.create
```

**Architecture Note:** The TUI and Web interfaces are **clients** that connect to the running node via RPC. They don't start their own node - they control an existing one. Start the node daemon first, then connect with `i2plan tui` or `i2plan web` in a separate terminal.

## How It Works

The library implements WireGuard's `conn.Bind` interface, replacing UDP sockets with I2P datagram sessions. Peer endpoints use I2P destination addresses (base32 format) instead of IP addresses.

```
┌─────────────────────────────────────────────────────────────┐
│                lib/web (UI) │ lib/tui (UI)                  │
│                   ↓ RPC client connection ↓                 │
├─────────────────────────────────────────────────────────────┤
│                      Application                            │
├─────────────────────────────────────────────────────────────┤
│                   lib/embedded (VPN API)                    │
├─────────────────────────────────────────────────────────────┤
│  lib/core    │  lib/mesh   │  lib/rpc                       │
│  (Node)      │  (Gossip)   │  (Control Server)              │
├──────────────┼─────────────┴────────────────────────────────┤
│ lib/identity │              lib/transport                   │
│  (Keys)      │              (Peer Tracking)                 │
├──────────────┴──────────────────────────────────────────────┤
│                      i2pbind (conn.Bind)                    │
├─────────────────────────────────────────────────────────────┤
│                   wireguard-go (WireGuard)                  │
├─────────────────────────────────────────────────────────────┤
│                      I2P SAM Bridge                         │
└─────────────────────────────────────────────────────────────┘
```

## Configuration

i2plan can be configured through TOML files, environment variables, or programmatically.

### Configuration Files

Configuration files use TOML format. See example configurations:
- **`config.example.toml`** - Complete configuration with all options and defaults
- **`config.minimal.toml`** - Minimal configuration for quick starts
- **`config.production.toml`** - Production-ready settings with recommended values

Default configuration locations:
- **Unix/Linux:** `~/.i2plan/config.toml`
- **Windows:** `%USERPROFILE%\.i2plan\config.toml`

Example minimal configuration:
```toml
[node]
name = "my-node"

[i2p]
sam_address = "127.0.0.1:7656"
tunnel_length = 1

[mesh]
tunnel_subnet = "10.42.0.0/16"
max_peers = 50
```

### Environment Variables

Environment variables with the `I2PLAN_` prefix override configuration file values. This is useful for containerized deployments, CI/CD pipelines, and testing.

**Available environment variables:**

| Variable | Type | Description | Example |
|----------|------|-------------|---------|
| `I2PLAN_NODE_NAME` | string | Node identifier | `my-node` |
| `I2PLAN_DATA_DIR` | string | Data directory path | `/var/lib/i2plan` |
| `I2PLAN_SAM_ADDRESS` | string | SAM bridge address | `127.0.0.1:7656` |
| `I2PLAN_TUNNEL_LENGTH` | int | I2P tunnel hops (0-7) | `1` |
| `I2PLAN_TUNNEL_SUBNET` | string | Mesh IP range (CIDR) | `10.42.0.0/16` |
| `I2PLAN_HEARTBEAT_INTERVAL` | int | Heartbeat interval (seconds) | `30` |
| `I2PLAN_PEER_TIMEOUT` | int | Peer timeout (seconds) | `300` |
| `I2PLAN_MAX_PEERS` | int | Maximum peer count | `50` |
| `I2PLAN_SHUTDOWN_TIMEOUT` | int | Shutdown timeout (seconds) | `5` |
| `I2PLAN_DRAIN_TIMEOUT` | int | Drain timeout (seconds) | `10` |
| `I2PLAN_RPC_ENABLED` | bool | Enable RPC server | `true` |
| `I2PLAN_RPC_SOCKET` | string | RPC socket path | `rpc.sock` |
| `I2PLAN_RPC_TCP_ADDRESS` | string | RPC TCP address | `127.0.0.1:9090` |
| `I2PLAN_WEB_ENABLED` | bool | Enable Web UI | `true` |
| `I2PLAN_WEB_LISTEN` | string | Web UI listen address | `127.0.0.1:8080` |

**Example usage:**
```bash
# Override node name and increase tunnel length
I2PLAN_NODE_NAME=prod-node-01 I2PLAN_TUNNEL_LENGTH=2 i2plan

# Container deployment with environment variables
docker run -e I2PLAN_NODE_NAME=container-node \
           -e I2PLAN_DATA_DIR=/data \
           -e I2PLAN_SAM_ADDRESS=i2p-router:7656 \
           i2plan
```

**Precedence order:** Environment variables > Configuration file > Built-in defaults

### Configuration Options

See the [complete configuration reference](config.example.toml) for detailed descriptions of all options.

## Building with Version Info

```bash
go build -ldflags "-X github.com/go-i2p/wireguard/version.Version=1.0.0 \
    -X github.com/go-i2p/wireguard/version.GitCommit=$(git rev-parse --short HEAD) \
    -X github.com/go-i2p/wireguard/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```

## Logging Configuration

This library uses the [`github.com/go-i2p/logger`](https://github.com/go-i2p/logger) package for structured logging. Log output is controlled via environment variables:

### Environment Variables

| Variable | Values | Description |
|----------|--------|-------------|
| `DEBUG_I2P` | `debug`, `warn`, `error` | Sets the minimum log level. Default shows only errors. |
| `WARNFAIL_I2P` | `true` | Fast-fail mode: any Warn or Error becomes Fatal, useful for testing. |

### Log Levels

- **Debug** - Detailed tracing information (function entry/exit, state changes, internal operations)
- **Info** - Significant lifecycle events (startup, shutdown, connections)
- **Warn** - Recoverable issues that may need attention
- **Error** - Failures that prevent normal operation

### Examples

```bash
# Show debug-level logging
DEBUG_I2P=debug ./i2plan

# Show warnings and above
DEBUG_I2P=warn ./i2plan

# Run tests with debug output
DEBUG_I2P=debug go test ./...

# Fast-fail on any warnings (for CI/testing)
WARNFAIL_I2P=true go test ./...
```

### Structured Logging

The logging uses structured fields for easy parsing:

```go
log.WithField("peer_id", peerID).Debug("Connecting to peer")
log.WithFields(map[string]interface{}{
    "node_id": nodeID,
    "state":   state,
}).Info("State transition")
log.WithError(err).Error("Failed to connect")
```

### Log Rotation for Production

For production deployments, configure log rotation to prevent disk space exhaustion:

#### Using systemd (Linux)

When running as a systemd service, logs are automatically captured by journald. Configure retention with `/etc/systemd/journald.conf`:

```ini
[Journal]
SystemMaxUse=1G
SystemMaxFileSize=100M
MaxRetentionSec=7day
```

Apply changes:
```bash
sudo systemctl restart systemd-journald
```

Query logs:
```bash
# View logs with filtering
journalctl -u i2plan.service -f

# View logs from specific time range
journalctl -u i2plan.service --since "1 hour ago"

# Export logs to file
journalctl -u i2plan.service > i2plan.log
```

#### Using logrotate (Linux)

If logging to files, use logrotate. Create `/etc/logrotate.d/i2plan`:

```
/var/log/i2plan/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    create 0640 i2plan i2plan
    sharedscripts
    postrotate
        # Send HUP signal to reopen log files (if implemented)
        # or restart the service
        systemctl reload-or-restart i2plan.service > /dev/null 2>&1 || true
    endscript
}
```

Test the configuration:
```bash
sudo logrotate -d /etc/logrotate.d/i2plan
sudo logrotate -f /etc/logrotate.d/i2plan
```

#### Container Deployments

For Docker/Kubernetes, configure log drivers to limit storage:

**Docker Compose:**
```yaml
services:
  i2plan:
    image: i2plan:latest
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

**Kubernetes:**
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: i2plan
spec:
  containers:
  - name: i2plan
    image: i2plan:latest
    # Logs are rotated automatically by kubelet
    # Configure retention in kubelet flags:
    # --container-log-max-size=10Mi
    # --container-log-max-files=5
```

#### Centralized Logging

For production environments, consider centralized logging:

**Syslog:**
Pipe i2plan output to syslog for centralized collection:
```bash
i2plan 2>&1 | logger -t i2plan -p local0.info
```

**ELK Stack / Loki:**
Configure log aggregators to collect structured JSON logs:
```bash
# Run with JSON output (if supported by logger)
DEBUG_I2P=info i2plan | filebeat -c filebeat.yml
```

### Security: Sensitive Data Protection

i2plan is designed to never log sensitive information:

- **Private keys** - Never logged in any context
- **Authentication tokens** - Only token IDs logged for debugging, never token values
- **CSRF tokens** - Not logged
- **Invite codes** - Only metadata logged (expiry, usage count), not the full code
- **I2P destinations** - Public identifiers only (safe to log)

All log entries use structured fields to ensure consistent, parseable output without exposing secrets.

## License

See [LICENSE](LICENSE) file.
