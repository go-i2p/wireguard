# Architecture Decision Records

This directory contains Architecture Decision Records (ADRs) for the i2plan project.

## What is an ADR?

An Architecture Decision Record (ADR) captures an important architectural decision made along with its context and consequences.

## Format

We use a simplified ADR format with the following sections:

- **Title**: Short descriptive title
- **Status**: proposed, accepted, deprecated, superseded
- **Date**: When the decision was made
- **Context**: What is the issue we're addressing?
- **Decision**: What is the change we're proposing/implementing?
- **Consequences**: What becomes easier or more difficult?

## Index

- [ADR-001: Use I2P for Transport Layer](001-use-i2p-for-transport.md)
- [ADR-002: Trusted Peer Model with 1-Hop Tunnels](002-trusted-peer-model.md)
- [ADR-003: Gossip Protocol for Peer Discovery](003-gossip-protocol.md)
- [ADR-004: Invite-Based Authentication](004-invite-based-authentication.md)
- [ADR-005: Unix Sockets for RPC](005-unix-sockets-rpc.md)
- [ADR-006: Go Standard Library First Approach](006-stdlib-first.md)

## Creating New ADRs

To create a new ADR:

1. Copy the template: `cp 000-template.md NNN-your-title.md`
2. Fill in the sections
3. Submit for review
4. Update this index when accepted

## Template

See [000-template.md](000-template.md) for the ADR template.
