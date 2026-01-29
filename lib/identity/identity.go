// Package identity manages node identity for the i2plan mesh network.
// Each node has a WireGuard keypair for tunnel encryption and an I2P
// destination for anonymous transport. The identity is persisted to disk
// and loaded on startup.
package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

const (
	// IdentityFileName is the default filename for persisted identity.
	IdentityFileName = "identity.json"

	// NodeIDLength is the length of the derived node ID in bytes.
	NodeIDLength = 16
)

// Identity represents a node's cryptographic identity in the mesh network.
// It contains both the WireGuard keypair for VPN encryption and the I2P
// destination for anonymous transport.
type Identity struct {
	mu sync.RWMutex

	// WireGuard keys
	privateKey wgtypes.Key
	publicKey  wgtypes.Key

	// Derived node ID (first 16 bytes of SHA-256 of public key)
	nodeID string

	// I2P destination (set after transport opens)
	i2pDest string

	// Network identifier (which mesh network this identity belongs to)
	networkID string

	// Metadata
	createdAt time.Time
}

// persistedIdentity is the JSON-serializable form of Identity.
type persistedIdentity struct {
	PrivateKey string    `json:"private_key"`
	PublicKey  string    `json:"public_key"`
	NodeID     string    `json:"node_id"`
	I2PDest    string    `json:"i2p_dest,omitempty"`
	NetworkID  string    `json:"network_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// NewIdentity generates a new random identity with a fresh WireGuard keypair.
func NewIdentity() (*Identity, error) {
	privateKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return nil, fmt.Errorf("generating private key: %w", err)
	}

	return newIdentityFromPrivateKey(privateKey), nil
}

// newIdentityFromPrivateKey creates an identity from an existing private key.
func newIdentityFromPrivateKey(privateKey wgtypes.Key) *Identity {
	publicKey := privateKey.PublicKey()
	nodeID := deriveNodeID(publicKey)

	return &Identity{
		privateKey: privateKey,
		publicKey:  publicKey,
		nodeID:     nodeID,
		createdAt:  time.Now(),
	}
}

// deriveNodeID computes a stable node ID from the WireGuard public key.
// Uses first 16 bytes of SHA-256 hash, encoded as hex (32 chars).
func deriveNodeID(publicKey wgtypes.Key) string {
	hash := sha256.Sum256(publicKey[:])
	return hex.EncodeToString(hash[:NodeIDLength])
}

// LoadIdentity loads an identity from a JSON file.
// Returns nil, nil if the file doesn't exist (caller should create new identity).
func LoadIdentity(path string) (*Identity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading identity file: %w", err)
	}

	var p persistedIdentity
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing identity file: %w", err)
	}

	// Parse private key
	privateKey, err := wgtypes.ParseKey(p.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}

	// Verify public key matches
	derivedPublicKey := privateKey.PublicKey()
	if p.PublicKey != derivedPublicKey.String() {
		return nil, errors.New("public key mismatch in identity file")
	}

	// Verify node ID matches
	derivedNodeID := deriveNodeID(derivedPublicKey)
	if p.NodeID != derivedNodeID {
		return nil, errors.New("node ID mismatch in identity file")
	}

	return &Identity{
		privateKey: privateKey,
		publicKey:  derivedPublicKey,
		nodeID:     derivedNodeID,
		i2pDest:    p.I2PDest,
		networkID:  p.NetworkID,
		createdAt:  p.CreatedAt,
	}, nil
}

// Save persists the identity to a JSON file.
// Creates the parent directory if it doesn't exist.
func (id *Identity) Save(path string) error {
	id.mu.RLock()
	p := persistedIdentity{
		PrivateKey: id.privateKey.String(),
		PublicKey:  id.publicKey.String(),
		NodeID:     id.nodeID,
		I2PDest:    id.i2pDest,
		NetworkID:  id.networkID,
		CreatedAt:  id.createdAt,
	}
	id.mu.RUnlock()

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling identity: %w", err)
	}

	// Create parent directory
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating identity directory: %w", err)
	}

	// Write atomically via temp file
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("writing identity file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming identity file: %w", err)
	}

	return nil
}

// PrivateKey returns the WireGuard private key.
func (id *Identity) PrivateKey() wgtypes.Key {
	id.mu.RLock()
	defer id.mu.RUnlock()
	return id.privateKey
}

// PublicKey returns the WireGuard public key.
func (id *Identity) PublicKey() wgtypes.Key {
	id.mu.RLock()
	defer id.mu.RUnlock()
	return id.publicKey
}

// NodeID returns the derived node identifier (hex-encoded).
func (id *Identity) NodeID() string {
	id.mu.RLock()
	defer id.mu.RUnlock()
	return id.nodeID
}

// I2PDest returns the I2P destination address (base32).
// Returns empty string if not yet set.
func (id *Identity) I2PDest() string {
	id.mu.RLock()
	defer id.mu.RUnlock()
	return id.i2pDest
}

// SetI2PDest sets the I2P destination address.
// This is typically called after the transport layer opens.
func (id *Identity) SetI2PDest(dest string) {
	id.mu.Lock()
	defer id.mu.Unlock()
	id.i2pDest = dest
}

// NetworkID returns the network identifier this identity belongs to.
func (id *Identity) NetworkID() string {
	id.mu.RLock()
	defer id.mu.RUnlock()
	return id.networkID
}

// SetNetworkID sets the network identifier.
func (id *Identity) SetNetworkID(networkID string) {
	id.mu.Lock()
	defer id.mu.Unlock()
	id.networkID = networkID
}

// CreatedAt returns when this identity was created.
func (id *Identity) CreatedAt() time.Time {
	id.mu.RLock()
	defer id.mu.RUnlock()
	return id.createdAt
}

// GenerateNetworkID creates a new random network identifier.
// Used when creating a new mesh network.
func GenerateNetworkID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generating network ID: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
