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

## Usage

```go
package main

import (
    "github.com/go-i2p/wireguard/i2pbind"
    "golang.zx2c4.com/wireguard/device"
)

func main() {
    // Create I2P bind with a tunnel name
    bind := i2pbind.NewI2PBind("my-wireguard-tunnel")
    
    // Create WireGuard device with I2P transport
    dev := device.NewDevice(tun, bind, logger)
    
    // Configure and use the device normally...
}
```

See the [example](i2pbind/example) directory for a complete working example.

## How It Works

The library implements WireGuard's `conn.Bind` interface, replacing UDP sockets with I2P datagram sessions. Peer endpoints use I2P destination addresses (base32 format) instead of IP addresses.

## License

See [LICENSE](LICENSE) file.
