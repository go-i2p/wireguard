# ADR-003: Gossip Protocol for Mesh State

## Status

Accepted

## Date

2024-01-15

## Context

In a mesh VPN, each node needs to:
- Discover other peers in the network
- Learn routing information (which peers can reach which destinations)
- Synchronize trusted identities across the network
- Propagate bans and revocations
- Handle network partitions and reconnections

Traditional approaches:
- **Centralized registry**: Single point of failure, doesn't align with decentralized goals
- **DHT (Distributed Hash Table)**: Complex, designed for large networks, high overhead
- **Flooding**: Simple but creates network storms, doesn't scale
- **Gossip protocol**: Balance of simplicity, scalability, and eventual consistency

## Decision

We implement a **gossip-based protocol** for propagating mesh state, similar to systems like Consul, Cassandra, and Ethereum's peer discovery.

### Core Mechanism

1. **Periodic Gossip**: Each node periodically (default: 30s) sends state to random subset of peers
2. **Message Types**:
   - `PeerAnnouncement`: I2P destination, WireGuard key, endpoints
   - `IdentitySync`: Trusted identities with invitation lineage
   - `BanlistUpdate`: Revoked identities and banned destinations
   - `RouteAdvertisement`: Reachable destinations through this peer

3. **Anti-Entropy**: On receiving gossip:
   - Compare local state to received state
   - Request missing information
   - Merge state using conflict resolution rules

4. **Epidemic Broadcast**: When node receives new information:
   - Forward to subset of peers immediately (fanout=3)
   - Include in next periodic gossip cycle

### State Versioning

Each piece of state has version/timestamp:
```go
type GossipMessage struct {
    Type      MessageType
    Sender    string        // I2P destination
    Version   uint64        // Monotonic version counter
    Timestamp time.Time     // Wall clock time
    Data      []byte        // Protobuf-encoded payload
    Signature []byte        // Ed25519 signature
}
```

Conflict resolution: higher version wins, timestamp as tiebreaker.

## Consequences

### Positive

- **Eventual Consistency**: All nodes converge to same state given enough time
- **Fault Tolerance**: No single point of failure
- **Partition Resilience**: Network heals automatically after splits
- **Scalability**: O(log N) message complexity
- **Simplicity**: Easy to implement and reason about

### Negative

- **Convergence Time**: State propagation not instant (30s-2min typical)
- **Bandwidth Overhead**: Periodic gossip consumes bandwidth even when idle
- **Stale State**: Nodes may briefly operate on outdated information
- **Conflict Resolution**: Requires careful design of merge semantics

### Trade-offs

- **Consistency vs Availability**: We choose availability and partition tolerance (AP in CAP theorem)
- **Latency vs Bandwidth**: Tunable gossip interval trades convergence speed for bandwidth
- **Simplicity vs Optimality**: Probabilistic protocol is simpler than optimal flooding algorithms

## Implementation Details

### Gossip Configuration

```go
type GossipConfig struct {
    Interval         time.Duration // How often to gossip (default: 30s)
    Fanout           int          // How many peers to gossip to (default: 3)
    MaxMessageSize   int          // Max gossip message size (default: 64KB)
    RetransmitMult   int          // Retransmission multiplier (default: 4)
}
```

### Peer Selection

Random peer selection with bias:
- 70% probability: random peer from full mesh
- 20% probability: least-recently-contacted peer
- 10% probability: most-recently-contacted peer (to reinforce active connections)

This balances exploration (discovering new state) and exploitation (reinforcing active peers).

### Message Batching

Multiple state updates batched into single gossip message:
- Reduces packet overhead
- Limited by `MaxMessageSize`
- Prioritizes recent/important updates

### Anti-Entropy Mechanism

Full state exchange every N gossip cycles (default: N=10):
- Node sends summary of local state (hashes/versions)
- Peer compares with local state
- Missing/outdated information requested explicitly
- Catches missed updates from probabilistic gossip

## Protocol Messages

### PeerAnnouncement

```go
type PeerAnnouncement struct {
    I2PDestination string
    WGPublicKey    string
    Endpoints      []string  // I2P addresses
    LastSeen       time.Time
    Version        uint64
}
```

### IdentitySync

```go
type IdentitySync struct {
    Identities []Identity   // See ADR-002
    Version    uint64
}
```

### BanlistUpdate

```go
type BanlistUpdate struct {
    BannedIdentities []string    // I2P destinations
    Reason           string
    BannedBy         string       // Admin I2P destination
    Timestamp        time.Time
    Version          uint64
}
```

### RouteAdvertisement

```go
type RouteAdvertisement struct {
    Destination string       // I2P address or subnet
    NextHop     string       // I2P destination of next hop
    Metric      int          // Distance metric
    Version     uint64
}
```

## Alternatives Considered

### Centralized Registry

- **Pros**: Simple, consistent, low latency
- **Cons**: Single point of failure, doesn't scale, requires trust
- **Rejected**: Violates decentralization goal

### DHT (Kademlia/Chord)

- **Pros**: Proven at scale (BitTorrent, IPFS)
- **Cons**: Complex, designed for millions of nodes, high overhead
- **Rejected**: Over-engineered for mesh VPNs (10-1000 nodes)

### Blockchain Consensus

- **Pros**: Strong consistency, immutable log
- **Cons**: High latency, energy consumption, complexity
- **Rejected**: Overkill for soft-state mesh coordination

### Flooding

- **Pros**: Very simple, fast propagation
- **Cons**: O(N²) messages, creates network storms
- **Rejected**: Doesn't scale beyond ~10 nodes

## Security Considerations

1. **Message Authentication**: All gossip messages signed with I2P private key
2. **Replay Prevention**: Version numbers and timestamps prevent replays
3. **Flood Prevention**: Rate limiting on gossip message acceptance
4. **Poison State**: Invalid state rejected via validation checks
5. **Sybil Resistance**: Identities must be invited (see ADR-002)

## Performance Characteristics

Expected behavior in a 100-node mesh:
- **Convergence Time**: 90% of nodes see new state within 60s
- **Bandwidth**: ~5-10 KB/s per node gossip overhead
- **CPU**: Negligible (signature verification is main cost)
- **Memory**: ~1 MB for full mesh state

## References

- `lib/mesh/gossip.go` - Gossip protocol implementation
- `lib/mesh/messages.go` - Protobuf message definitions
- `lib/mesh/peer.go` - Peer management
- [Epidemic Algorithms for Replicated Database Maintenance](https://dl.acm.org/doi/10.1145/41840.41841)
- [SWIM: Scalable Weakly-consistent Infection-style Process Group Membership Protocol](https://ieeexplore.ieee.org/document/1028914)
- [Consul Gossip Protocol](https://developer.hashicorp.com/consul/docs/architecture/gossip)
