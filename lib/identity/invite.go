package identity

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// InviteScheme is the URL scheme for invite codes.
	InviteScheme = "i2plan://"

	// DefaultInviteExpiry is the default expiration time for invites.
	DefaultInviteExpiry = 24 * time.Hour

	// DefaultMaxUses is the default maximum uses for an invite.
	DefaultMaxUses = 1

	// AuthTokenLength is the length of the auth token in bytes.
	AuthTokenLength = 32
)

var (
	// ErrInvalidInviteFormat is returned when an invite code has invalid format.
	ErrInvalidInviteFormat = errors.New("invalid invite format")

	// ErrInviteExpired is returned when an invite has expired.
	ErrInviteExpired = errors.New("invite has expired")

	// ErrInviteExhausted is returned when an invite has no remaining uses.
	ErrInviteExhausted = errors.New("invite has no remaining uses")
)

// Invite represents an invitation to join a mesh network.
// Invites contain the inviter's I2P destination, an auth token for
// verification, and the network ID to join.
type Invite struct {
	// I2PDest is the inviter's I2P destination (base32)
	I2PDest string `json:"i2p_dest"`

	// AuthToken is a one-time authentication token
	AuthToken []byte `json:"auth_token"`

	// NetworkID identifies which mesh network to join
	NetworkID string `json:"network_id"`

	// ExpiresAt is when this invite becomes invalid
	ExpiresAt time.Time `json:"expires_at"`

	// MaxUses is the maximum number of times this invite can be used
	MaxUses int `json:"max_uses"`

	// UsedCount tracks how many times this invite has been used
	UsedCount int `json:"used_count"`

	// CreatedAt is when this invite was generated
	CreatedAt time.Time `json:"created_at"`

	// CreatedBy is the node ID of the inviter
	CreatedBy string `json:"created_by,omitempty"`
}

// InviteOptions configures invite generation.
type InviteOptions struct {
	// Expiry is how long until the invite expires (default: 24h)
	Expiry time.Duration

	// MaxUses is the maximum number of uses (default: 1)
	MaxUses int
}

// DefaultInviteOptions returns the default invite options.
func DefaultInviteOptions() InviteOptions {
	return InviteOptions{
		Expiry:  DefaultInviteExpiry,
		MaxUses: DefaultMaxUses,
	}
}

// NewInvite creates a new invite from the given identity.
// The identity must have I2PDest and NetworkID set.
func NewInvite(id *Identity, opts InviteOptions) (*Invite, error) {
	if id.I2PDest() == "" {
		return nil, errors.New("identity has no I2P destination")
	}
	if id.NetworkID() == "" {
		return nil, errors.New("identity has no network ID")
	}

	// Generate random auth token
	authToken := make([]byte, AuthTokenLength)
	if _, err := rand.Read(authToken); err != nil {
		return nil, fmt.Errorf("generating auth token: %w", err)
	}

	// Apply defaults
	if opts.Expiry <= 0 {
		opts.Expiry = DefaultInviteExpiry
	}
	// Note: MaxUses=0 means unlimited uses, only default negative values
	if opts.MaxUses < 0 {
		opts.MaxUses = DefaultMaxUses
	}

	now := time.Now()
	return &Invite{
		I2PDest:   id.I2PDest(),
		AuthToken: authToken,
		NetworkID: id.NetworkID(),
		ExpiresAt: now.Add(opts.Expiry),
		MaxUses:   opts.MaxUses,
		UsedCount: 0,
		CreatedAt: now,
		CreatedBy: id.NodeID(),
	}, nil
}

// Encode serializes the invite to a URL-safe string.
// Format: i2plan://<base64url(json)>
func (inv *Invite) Encode() (string, error) {
	// Create a minimal payload for the invite code
	payload := struct {
		I2PDest   string    `json:"d"`
		AuthToken []byte    `json:"t"`
		NetworkID string    `json:"n"`
		ExpiresAt time.Time `json:"e"`
	}{
		I2PDest:   inv.I2PDest,
		AuthToken: inv.AuthToken,
		NetworkID: inv.NetworkID,
		ExpiresAt: inv.ExpiresAt,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encoding invite: %w", err)
	}

	encoded := base64.URLEncoding.EncodeToString(data)
	return InviteScheme + encoded, nil
}

// ParseInvite decodes an invite code string into an Invite.
// Does NOT validate expiry or uses - call Validate() for that.
func ParseInvite(code string) (*Invite, error) {
	data, err := decodeInviteCode(code)
	if err != nil {
		return nil, err
	}

	payload, err := unmarshalInvitePayload(data)
	if err != nil {
		return nil, err
	}

	return buildInviteFromPayload(payload)
}

// decodeInviteCode extracts and decodes the base64 payload from an invite code.
func decodeInviteCode(code string) ([]byte, error) {
	if !strings.HasPrefix(code, InviteScheme) {
		return nil, ErrInvalidInviteFormat
	}

	encoded := strings.TrimPrefix(code, InviteScheme)
	if encoded == "" {
		return nil, ErrInvalidInviteFormat
	}

	data, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid base64", ErrInvalidInviteFormat)
	}
	return data, nil
}

// invitePayload is the wire format for invite codes.
type invitePayload struct {
	I2PDest   string    `json:"d"`
	AuthToken []byte    `json:"t"`
	NetworkID string    `json:"n"`
	ExpiresAt time.Time `json:"e"`
}

// unmarshalInvitePayload parses JSON data into an invite payload.
func unmarshalInvitePayload(data []byte) (*invitePayload, error) {
	var payload invitePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON", ErrInvalidInviteFormat)
	}
	return &payload, nil
}

// buildInviteFromPayload validates and constructs an Invite from a payload.
func buildInviteFromPayload(payload *invitePayload) (*Invite, error) {
	if payload.I2PDest == "" {
		return nil, fmt.Errorf("%w: missing I2P destination", ErrInvalidInviteFormat)
	}
	if len(payload.AuthToken) == 0 {
		return nil, fmt.Errorf("%w: missing auth token", ErrInvalidInviteFormat)
	}
	if payload.NetworkID == "" {
		return nil, fmt.Errorf("%w: missing network ID", ErrInvalidInviteFormat)
	}

	return &Invite{
		I2PDest:   payload.I2PDest,
		AuthToken: payload.AuthToken,
		NetworkID: payload.NetworkID,
		ExpiresAt: payload.ExpiresAt,
		MaxUses:   1,
		UsedCount: 0,
	}, nil
}

// Validate checks if the invite is still valid (not expired, has remaining uses).
func (inv *Invite) Validate() error {
	if time.Now().After(inv.ExpiresAt) {
		return ErrInviteExpired
	}
	if inv.MaxUses > 0 && inv.UsedCount >= inv.MaxUses {
		return ErrInviteExhausted
	}
	return nil
}

// IsExpired returns true if the invite has expired.
func (inv *Invite) IsExpired() bool {
	return time.Now().After(inv.ExpiresAt)
}

// RemainingUses returns the number of remaining uses.
// Returns -1 for unlimited uses.
func (inv *Invite) RemainingUses() int {
	if inv.MaxUses <= 0 {
		return -1 // unlimited
	}
	remaining := inv.MaxUses - inv.UsedCount
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Use marks the invite as used once.
// Returns an error if the invite is invalid.
func (inv *Invite) Use() error {
	if err := inv.Validate(); err != nil {
		return err
	}
	inv.UsedCount++
	return nil
}

// VerifyToken checks if the provided token matches the invite's auth token.
func (inv *Invite) VerifyToken(token []byte) bool {
	if len(token) != len(inv.AuthToken) {
		return false
	}
	// Constant-time comparison to prevent timing attacks
	var mismatch byte
	for i := range inv.AuthToken {
		mismatch |= inv.AuthToken[i] ^ token[i]
	}
	return mismatch == 0
}
