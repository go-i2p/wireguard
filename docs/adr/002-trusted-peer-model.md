# ADR-002: Trusted Peer Model

## Status

Accepted

## Date

2024-01-15

## Context

Mesh VPN networks face challenges with:
- Open networks vulnerable to abuse (spam, DoS, resource exhaustion)
- Fully closed networks difficult to scale and onboard new users
- Traditional PKI complex and requires centralized certificate authorities
- Web-of-trust models have poor UX and unclear trust boundaries

We need an access control model that:
- Prevents unauthorized network access
- Enables controlled growth of the network
- Works in a decentralized environment
- Provides good user experience

## Decision

We implement a **trusted peer model** with invite-based authentication:

1. **Invite System**: Existing network members generate invite codes that encode:
   - Creator's I2P public key
   - Invitee's allocated WireGuard public key
   - Cryptographic signature binding them together
   - Optional expiration time

2. **Identity Store**: Each node maintains local database of trusted identities:
   - I2P destination (base32 address)
   - WireGuard public key
   - Trust level (admin, member)
   - Invitation lineage

3. **Authentication Flow**:
   - New peer presents invite code + their I2P destination
   - Receiving peer validates signature against known identities
   - If valid, peer is added to local identity store
   - Gossip protocol propagates new identity to mesh

4. **Trust Propagation**: Identities are gossiped with "trust by association":
   - Admin invites are fully trusted
   - Member invites are trusted by members who know the inviter
   - Transitive trust up to configurable depth (default: 2 hops)

## Consequences

### Positive

- **Controlled Access**: Only invited peers can join
- **Decentralized**: No central authority needed
- **Simple UX**: Share invite codes (strings or QR codes)
- **Accountability**: Invitation lineage creates social accountability
- **Flexible**: Support for multiple trust levels (admin/member)
- **Revocation**: Identities can be banned via banlist gossip

### Negative

- **Initial Bootstrap**: Someone must create the first admin identity
- **Trust Decisions**: Users must make trust decisions when generating invites
- **Lineage Tracking**: Requires storing invitation graph
- **Gossip Complexity**: Trust propagation adds protocol complexity

### Trade-offs

- **Security vs Convenience**: We prioritize ease of onboarding while maintaining security through cryptographic invite codes
- **Centralization vs Autonomy**: Each node makes own trust decisions, but gossip creates eventual consistency
- **Simplicity vs Flexibility**: Simple trust levels (admin/member) vs complex role systems

## Alternatives Considered

### Open Network with Rate Limiting

- **Pros**: Easy to join, no access control
- **Cons**: Vulnerable to abuse, spam, DoS attacks
- **Rejected**: Unacceptable security posture

### Pre-shared Keys (PSK)

- **Pros**: Simple, built into WireGuard
- **Cons**: Single shared secret, difficult to revoke, no granular access control
- **Rejected**: Doesn't scale beyond small teams

### X.509 PKI with Certificate Authority

- **Pros**: Industry standard, mature tooling
- **Cons**: Requires running CA, centralized, complex key management
- **Rejected**: Too complex and centralized for mesh VPN

### Web of Trust (PGP-style)

- **Pros**: Fully decentralized
- **Cons**: Poor UX, unclear trust boundaries, slow adoption
- **Rejected**: Historical failure in user-facing systems

### Blockchain/Smart Contract

- **Pros**: Decentralized consensus, immutable audit log
- **Cons**: High complexity, external dependency, poor performance
- **Rejected**: Overengineered for the use case

## Implementation Notes

```go
// Identity represents a trusted peer
type Identity struct {
    I2PDestination string        // base32 I2P address
    WGPublicKey    string        // WireGuard public key
    TrustLevel     TrustLevel    // admin or member
    InvitedBy      string        // I2P destination of inviter
    CreatedAt      time.Time
}

// Invite encodes invitation cryptographically
type Invite struct {
    InviterI2P     string    // Inviter's I2P destination
    InviteeWGKey   string    // Pre-allocated WG key for invitee
    Expiration     time.Time // Optional expiry
    Signature      []byte    // Ed25519 signature
}
```

### Trust Levels

- **Admin**: Can invite admins and members, configure network parameters, initiate bans
- **Member**: Can invite members only, participate in mesh

### Invite Code Format

Base64-encoded protobuf or JSON structure:
```
i2plan://invite/v1?data=<base64-encoded-invite>&sig=<base64-signature>
```

Can be rendered as QR code for mobile devices.

### Revocation

Identities can be revoked through:
- Local removal from identity store
- Banlist gossip (see `lib/mesh/banlist.go`)
- Network-wide ban requires admin consensus

## Security Considerations

1. **Invite Theft**: Invite codes should be transmitted securely (encrypted channels)
2. **Signature Verification**: All invites validated against known identities before acceptance
3. **Replay Prevention**: Invites include nonce and expiration
4. **Trust Depth**: Limit transitive trust to prevent long-chain attacks
5. **Banlist Priority**: Bans override trust (revoked identities cannot rejoin)

## References

- `lib/identity/invite.go` - Invite generation and validation
- `lib/identity/store.go` - Identity database
- `lib/mesh/messages.go` - Gossip protocol messages
- ADR-003: Gossip Protocol (trust propagation mechanism)
- ADR-004: Invite-Based Authentication (detailed authentication flow)
