// Package identity manages node identity for the i2plan mesh network.
// Each node has a WireGuard keypair for tunnel encryption, an Ed25519
// keypair for message signing, and an I2P destination for anonymous
// transport. The identity is persisted to disk and loaded on startup.
package identity

import (
	"crypto/ed25519"
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
// It contains the WireGuard keypair for VPN encryption, an Ed25519 keypair
// for message signing, and the I2P destination for anonymous transport.
type Identity struct {
	mu sync.RWMutex

	// WireGuard keys (Curve25519)
	privateKey wgtypes.Key
	publicKey  wgtypes.Key

	// Ed25519 signing keys
	signingKey   ed25519.PrivateKey
	verifyingKey ed25519.PublicKey

	// Derived node ID (first 16 bytes of SHA-256 of WG public key)
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
	PrivateKey   string    `json:"private_key"`
	PublicKey    string    `json:"public_key"`
	SigningKey   string    `json:"signing_key"`
	VerifyingKey string    `json:"verifying_key"`
	NodeID       string    `json:"node_id"`
	I2PDest      string    `json:"i2p_dest,omitempty"`
	NetworkID    string    `json:"network_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// NewIdentity generates a new random identity with fresh WireGuard and Ed25519 keypairs.
func NewIdentity() (*Identity, error) {
	log.Debug("generating new identity")
	privateKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		log.WithError(err).Error("failed to generate WireGuard private key")
		return nil, fmt.Errorf("generating WireGuard private key: %w", err)
	}

	// Generate Ed25519 signing keypair
	verifyingKey, signingKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.WithError(err).Error("failed to generate Ed25519 signing key")
		return nil, fmt.Errorf("generating Ed25519 signing key: %w", err)
	}

	id := newIdentityFromKeys(privateKey, signingKey, verifyingKey)
	log.WithField("nodeID", id.nodeID).Debug("identity generated successfully")
	return id, nil
}

// newIdentityFromKeys creates an identity from existing keys.
func newIdentityFromKeys(privateKey wgtypes.Key, signingKey ed25519.PrivateKey, verifyingKey ed25519.PublicKey) *Identity {
	publicKey := privateKey.PublicKey()
	nodeID := deriveNodeID(publicKey)

	return &Identity{
		privateKey:   privateKey,
		publicKey:    publicKey,
		signingKey:   signingKey,
		verifyingKey: verifyingKey,
		nodeID:       nodeID,
		createdAt:    time.Now(),
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
	log.WithField("path", path).Debug("loading identity from file")

	data, err := readIdentityFile(path)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil // File doesn't exist
	}

	p, err := parseIdentityJSON(data)
	if err != nil {
		return nil, err
	}

	return buildIdentityFromPersisted(p)
}

// readIdentityFile reads the identity file, returning nil if it doesn't exist.
func readIdentityFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.WithField("path", path).Debug("identity file does not exist")
			return nil, nil
		}
		log.WithError(err).WithField("path", path).Error("failed to read identity file")
		return nil, fmt.Errorf("reading identity file: %w", err)
	}
	return data, nil
}

// parseIdentityJSON unmarshals the JSON identity data.
func parseIdentityJSON(data []byte) (*persistedIdentity, error) {
	var p persistedIdentity
	if err := json.Unmarshal(data, &p); err != nil {
		log.WithError(err).Error("failed to parse identity file")
		return nil, fmt.Errorf("parsing identity file: %w", err)
	}
	return &p, nil
}

// buildIdentityFromPersisted constructs an Identity from persisted data with validation.
func buildIdentityFromPersisted(p *persistedIdentity) (*Identity, error) {
	privateKey, err := parseWireGuardKey(*p)
	if err != nil {
		return nil, err
	}

	signingKey, verifyingKey, err := parseSigningKeys(*p)
	if err != nil {
		return nil, err
	}

	if err := validateNodeID(privateKey, p.NodeID); err != nil {
		return nil, err
	}

	return createIdentityFromKeys(privateKey, signingKey, verifyingKey, p), nil
}

// validateNodeID verifies that the stored node ID matches the derived value.
func validateNodeID(privateKey wgtypes.Key, storedNodeID string) error {
	derivedPublicKey := privateKey.PublicKey()
	derivedNodeID := deriveNodeID(derivedPublicKey)
	if storedNodeID != derivedNodeID {
		return errors.New("node ID mismatch in identity file")
	}
	return nil
}

// createIdentityFromKeys creates an Identity instance from keys and persisted metadata.
func createIdentityFromKeys(privateKey wgtypes.Key, signingKey ed25519.PrivateKey, verifyingKey ed25519.PublicKey, p *persistedIdentity) *Identity {
	derivedPublicKey := privateKey.PublicKey()
	derivedNodeID := deriveNodeID(derivedPublicKey)

	return &Identity{
		privateKey:   privateKey,
		publicKey:    derivedPublicKey,
		signingKey:   signingKey,
		verifyingKey: verifyingKey,
		nodeID:       derivedNodeID,
		i2pDest:      p.I2PDest,
		networkID:    p.NetworkID,
		createdAt:    p.CreatedAt,
	}
}

// parseWireGuardKey parses and validates the WireGuard private key from persisted identity.
func parseWireGuardKey(p persistedIdentity) (wgtypes.Key, error) {
	log.Debug("parsing WireGuard key")
	privateKey, err := wgtypes.ParseKey(p.PrivateKey)
	if err != nil {
		log.WithError(err).Error("failed to parse WireGuard private key")
		return wgtypes.Key{}, fmt.Errorf("parsing WireGuard private key: %w", err)
	}

	derivedPublicKey := privateKey.PublicKey()
	if p.PublicKey != derivedPublicKey.String() {
		log.Error("WireGuard public key mismatch in identity file")
		return wgtypes.Key{}, errors.New("WireGuard public key mismatch in identity file")
	}

	return privateKey, nil
}

// parseSigningKeys parses and validates the Ed25519 signing keys from persisted identity.
func parseSigningKeys(p persistedIdentity) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	log.Debug("parsing Ed25519 signing keys")
	signingKeyBytes, err := hex.DecodeString(p.SigningKey)
	if err != nil {
		log.WithError(err).Error("failed to parse signing key")
		return nil, nil, fmt.Errorf("parsing signing key: %w", err)
	}
	if len(signingKeyBytes) != ed25519.PrivateKeySize {
		log.Error("invalid signing key size")
		return nil, nil, errors.New("invalid signing key size")
	}
	signingKey := ed25519.PrivateKey(signingKeyBytes)

	verifyingKeyBytes, err := hex.DecodeString(p.VerifyingKey)
	if err != nil {
		log.WithError(err).Error("failed to parse verifying key")
		return nil, nil, fmt.Errorf("parsing verifying key: %w", err)
	}
	if len(verifyingKeyBytes) != ed25519.PublicKeySize {
		log.Error("invalid verifying key size")
		return nil, nil, errors.New("invalid verifying key size")
	}
	verifyingKey := ed25519.PublicKey(verifyingKeyBytes)

	if !verifyingKey.Equal(signingKey.Public()) {
		log.Error("Ed25519 key mismatch in identity file")
		return nil, nil, errors.New("Ed25519 key mismatch in identity file")
	}

	return signingKey, verifyingKey, nil
}

// Save persists the identity to a JSON file.
// Creates the parent directory if it doesn't exist.
func (id *Identity) Save(path string) error {
	log.WithField("path", path).Debug("saving identity to file")
	id.mu.RLock()
	p := persistedIdentity{
		PrivateKey:   id.privateKey.String(),
		PublicKey:    id.publicKey.String(),
		SigningKey:   hex.EncodeToString(id.signingKey),
		VerifyingKey: hex.EncodeToString(id.verifyingKey),
		NodeID:       id.nodeID,
		I2PDest:      id.i2pDest,
		NetworkID:    id.networkID,
		CreatedAt:    id.createdAt,
	}
	id.mu.RUnlock()

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		log.WithError(err).Error("failed to marshal identity")
		return fmt.Errorf("marshaling identity: %w", err)
	}

	// Create parent directory
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.WithError(err).WithField("dir", dir).Error("failed to create identity directory")
		return fmt.Errorf("creating identity directory: %w", err)
	}

	// Write atomically via temp file
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		log.WithError(err).Error("failed to write identity file")
		return fmt.Errorf("writing identity file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		if rmErr := os.Remove(tmpPath); rmErr != nil {
			log.WithError(rmErr).Warn("failed to remove temp file after rename error")
			// Log but don't fail on cleanup error - the rename error is more important
			// Stale temp files will be overwritten on next save attempt
		}
		log.WithError(err).Error("failed to rename identity file")
		return fmt.Errorf("renaming identity file: %w", err)
	}

	log.WithField("path", path).Debug("identity saved successfully")
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
	log.WithField("dest", dest).Debug("setting I2P destination")
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
	log.WithField("networkID", networkID).Debug("setting network ID")
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
	log.Debug("generating new network ID")
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		log.WithError(err).Error("failed to generate network ID")
		return "", fmt.Errorf("generating network ID: %w", err)
	}
	networkID := hex.EncodeToString(bytes)
	log.WithField("networkID", networkID).Debug("network ID generated")
	return networkID, nil
}

// SignatureLength is the length of Ed25519 signatures in bytes.
const SignatureLength = ed25519.SignatureSize // 64 bytes

// Sign creates an Ed25519 signature for the given data.
// The signature can be verified by any peer using VerifyWithPublicKey.
func (id *Identity) Sign(data []byte) []byte {
	id.mu.RLock()
	defer id.mu.RUnlock()
	return ed25519.Sign(id.signingKey, data)
}

// Verify checks if a signature is valid for the given data using our own key.
func (id *Identity) Verify(data, signature []byte) bool {
	id.mu.RLock()
	defer id.mu.RUnlock()
	return ed25519.Verify(id.verifyingKey, data, signature)
}

// VerifyingKey returns the Ed25519 public key for signature verification.
// This key can be shared with peers so they can verify our signatures.
func (id *Identity) VerifyingKey() ed25519.PublicKey {
	id.mu.RLock()
	defer id.mu.RUnlock()
	return id.verifyingKey
}

// VerifyingKeyHex returns the Ed25519 public key as a hex string.
func (id *Identity) VerifyingKeyHex() string {
	id.mu.RLock()
	defer id.mu.RUnlock()
	return hex.EncodeToString(id.verifyingKey)
}

// VerifyWithPublicKey verifies a signature using a peer's Ed25519 public key.
// Use this to verify messages from other nodes.
func VerifyWithPublicKey(publicKey ed25519.PublicKey, data, signature []byte) bool {
	if len(signature) != SignatureLength {
		return false
	}
	return ed25519.Verify(publicKey, data, signature)
}

// VerifyWithPublicKeyHex verifies a signature using a hex-encoded Ed25519 public key.
func VerifyWithPublicKeyHex(publicKeyHex string, data, signature []byte) bool {
	publicKeyBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil || len(publicKeyBytes) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(publicKeyBytes), data, signature)
}

// SignMessage creates a signed message envelope containing the signature and data.
// Returns: [signature (64 bytes)][data...]
func (id *Identity) SignMessage(data []byte) []byte {
	signature := id.Sign(data)
	result := make([]byte, SignatureLength+len(data))
	copy(result[:SignatureLength], signature)
	copy(result[SignatureLength:], data)
	return result
}

// ExtractSignedMessage splits a signed message into signature and data.
// Returns signature, data, and whether the format was valid.
func ExtractSignedMessage(signedMsg []byte) (signature, data []byte, ok bool) {
	if len(signedMsg) < SignatureLength {
		return nil, nil, false
	}
	return signedMsg[:SignatureLength], signedMsg[SignatureLength:], true
}

// DeriveDiscoveryToken generates a deterministic discovery token from a NetworkID.
// All nodes in the same network can derive this token, allowing peers discovered
// via gossip to authenticate without explicit invites.
func DeriveDiscoveryToken(networkID string) []byte {
	// Use SHA-256 hash of networkID + magic string to derive token
	h := sha256.New()
	h.Write([]byte("i2plan-discovery-v1:"))
	h.Write([]byte(networkID))
	return h.Sum(nil)
}
