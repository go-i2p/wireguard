# ADR-001: Use I2P for Transport Layer

## Status

Accepted

## Date

2024-01-15

## Context

We need a transport layer for WireGuard that provides:
- NAT traversal without manual port forwarding
- Network anonymity and resistance to traffic analysis
- Decentralized infrastructure
- Compatibility with existing WireGuard implementations

Traditional options considered:
- **Direct UDP**: Requires port forwarding, no anonymity
- **Tor**: TCP-only, high latency, designed for TCP streams
- **VPN tunneling**: Centralized, requires trusted third party
- **I2P**: UDP datagrams supported, distributed, NAT traversal built-in

## Decision

We will use the I2P network as the transport layer for WireGuard packets, implementing WireGuard's `conn.Bind` interface with I2P datagram sessions.

Key technical decisions:
- Use I2P's SAM (Simple Anonymous Messaging) bridge for connectivity
- Implement datagram sessions for UDP-like semantics
- Use I2P's base32 addresses for peer endpoints
- Maximum packet size limited to 31KB (I2P protocol limit)

## Consequences

### Positive

- **NAT Traversal**: Works automatically without port forwarding or STUN/TURN servers
- **Anonymity**: Traffic is routed through I2P tunnels, providing network-layer anonymity
- **No Central Infrastructure**: Fully decentralized, no reliance on third-party services
- **Standard WireGuard**: Remains compatible with wireguard-go device API
- **Existing Network**: Leverages mature I2P network with thousands of routers

### Negative

- **Latency**: Higher than direct UDP (50-200ms additional latency)
- **Throughput**: Lower than direct connections (typically 1-10 Mbps)
- **I2P Dependency**: Requires running I2P router locally
- **Tunnel Building**: Initial connection takes 1-3 minutes while tunnels build
- **Packet Size**: Limited to 31KB maximum (WireGuard packets typically <1.5KB, so not a practical issue)

### Trade-offs

- We prioritize **anonymity and NAT traversal** over raw performance
- Acceptable for use cases like:
  - Remote access to home networks
  - Small team collaboration networks
  - Privacy-focused mesh networks
- Not suitable for:
  - High-bandwidth applications
  - Low-latency real-time gaming
  - Large-scale video streaming

## Alternatives Considered

### Tor

- **Pros**: Larger network, more mature
- **Cons**: TCP-only, not designed for UDP datagrams, higher latency
- **Rejected**: Tor's TCP focus makes WireGuard integration complex and inefficient

### Direct UDP with NAT Traversal (STUN/TURN)

- **Pros**: Lower latency, higher throughput
- **Cons**: Requires central STUN/TURN servers, no anonymity, complex NAT traversal
- **Rejected**: Defeats the purpose of decentralized, anonymous VPN

### Custom Overlay Network

- **Pros**: Optimized for our specific use case
- **Cons**: Requires building/maintaining entire network infrastructure
- **Rejected**: Too complex, reinventing the wheel

## Implementation Notes

- I2P tunnels use 1-hop by default for the trusted peer model (see ADR-002)
- SAM bridge must be enabled in I2P router configuration
- Packet fragmentation is handled by I2P layer
- WireGuard's built-in encryption provides end-to-end security on top of I2P's transport encryption

## References

- [I2P Technical Documentation](https://geti2p.net/en/docs)
- [SAM Protocol Specification](https://geti2p.net/en/docs/api/samv3)
- [WireGuard conn.Bind Interface](https://pkg.go.dev/golang.zx2c4.com/wireguard/conn#Bind)
