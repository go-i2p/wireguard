# WireGuard over I2P

A Go library that enables WireGuard tunnels to run over the [I2P](https://geti2p.net/) anonymizing network. This package provides a `conn.Bind` implementation for [wireguard-go](https://git.zx2c4.com/wireguard-go/) that uses I2P datagrams instead of UDP.

## Features

- Drop-in replacement for WireGuard's standard UDP transport
- Leverages I2P's anonymity and NAT traversal capabilities
- Supports up to 31KB datagram payloads (I2P limit)
- Compatible with wireguard-go's device API

## Prerequisites

- Go 1.25 or later
- Running I2P router with SAM enabled (default port 7656)

## Installation

```bash
go get github.com/go-i2p/wireguard
```

## Quick Start

```go
package main

import (
    "log"
    "net/netip"

    "github.com/go-i2p/wireguard/i2pbind"
    "golang.zx2c4.com/wireguard/device"
    "golang.zx2c4.com/wireguard/tun/netstack"
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

    // Configure with IpcSet, then bring up with dev.Up()
    // See example directory for complete configuration
    _ = dev
}
```

**For a complete working example** including key configuration, peer setup, and signal handling, see the [example](i2pbind/example) directory:

```bash
cd i2pbind/example
go build -o wg-i2p
./wg-i2p -help
```

## How It Works

The library implements WireGuard's `conn.Bind` interface, replacing UDP sockets with I2P datagram sessions. Peer endpoints use I2P destination addresses (base32 format) instead of IP addresses.

## License

See [LICENSE](LICENSE) file.
