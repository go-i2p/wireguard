// Package validation provides reusable input validation functions for the i2plan mesh VPN.
// All validators follow a consistent pattern: they return nil on success and a descriptive
// error on failure. Errors are designed to be safe to return to clients (no internal details).
package validation

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Common validation errors. These are sentinel errors that can be checked with errors.Is().
var (
	// ErrRequired indicates a required field is missing or empty.
	ErrRequired = errors.New("field is required")

	// ErrTooLong indicates a string exceeds the maximum length.
	ErrTooLong = errors.New("value exceeds maximum length")

	// ErrTooShort indicates a string is below the minimum length.
	ErrTooShort = errors.New("value is below minimum length")

	// ErrInvalidFormat indicates a value doesn't match the expected format.
	ErrInvalidFormat = errors.New("invalid format")

	// ErrOutOfRange indicates a numeric value is outside the allowed range.
	ErrOutOfRange = errors.New("value out of range")

	// ErrInvalidDuration indicates an invalid duration string.
	ErrInvalidDuration = errors.New("invalid duration")
)

// Constraints for common field types.
const (
	// MaxNodeNameLength is the maximum length for node names.
	MaxNodeNameLength = 64

	// MaxNodeIDLength is the maximum length for node IDs (hex-encoded Ed25519 public key).
	MaxNodeIDLength = 64

	// MaxReasonLength is the maximum length for ban reasons.
	MaxReasonLength = 256

	// MaxDescriptionLength is the maximum length for descriptions.
	MaxDescriptionLength = 1024

	// MaxConfigKeyLength is the maximum length for configuration keys.
	MaxConfigKeyLength = 128

	// MaxInviteCodeLength is the maximum length for invite codes.
	MaxInviteCodeLength = 4096

	// MinDuration is the minimum duration for time-based operations.
	MinDuration = time.Second

	// MaxDuration is the maximum duration for time-based operations (1 year).
	MaxDuration = 365 * 24 * time.Hour
)

// nodeIDPattern matches valid hex-encoded node IDs.
var nodeIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

// configKeyPattern matches valid config keys (alphanumeric, dots, underscores).
var configKeyPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.]*$`)

// Result represents a validation result with field context.
type Result struct {
	Field   string
	Message string
	Err     error
}

// Error implements the error interface.
func (r *Result) Error() string {
	if r.Field != "" {
		return fmt.Sprintf("%s: %s", r.Field, r.Message)
	}
	return r.Message
}

// Unwrap returns the underlying error for errors.Is() support.
func (r *Result) Unwrap() error {
	return r.Err
}

// NewResult creates a validation result.
func NewResult(field, message string, err error) *Result {
	return &Result{
		Field:   field,
		Message: message,
		Err:     err,
	}
}

// Required validates that a string is non-empty.
func Required(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return NewResult(field, "is required", ErrRequired)
	}
	return nil
}

// MaxLength validates that a string doesn't exceed the maximum length.
func MaxLength(field, value string, max int) error {
	if utf8.RuneCountInString(value) > max {
		return NewResult(field, fmt.Sprintf("exceeds maximum length of %d characters", max), ErrTooLong)
	}
	return nil
}

// MinLength validates that a string meets the minimum length.
func MinLength(field, value string, min int) error {
	if utf8.RuneCountInString(value) < min {
		return NewResult(field, fmt.Sprintf("must be at least %d characters", min), ErrTooShort)
	}
	return nil
}

// LengthBetween validates that a string length is within the given range.
func LengthBetween(field, value string, min, max int) error {
	length := utf8.RuneCountInString(value)
	if length < min || length > max {
		return NewResult(field, fmt.Sprintf("must be between %d and %d characters", min, max), ErrOutOfRange)
	}
	return nil
}

// IntRange validates that an integer is within the given range (inclusive).
func IntRange(field string, value, min, max int) error {
	if value < min || value > max {
		return NewResult(field, fmt.Sprintf("must be between %d and %d", min, max), ErrOutOfRange)
	}
	return nil
}

// Positive validates that an integer is positive (> 0).
func Positive(field string, value int) error {
	if value <= 0 {
		return NewResult(field, "must be positive", ErrOutOfRange)
	}
	return nil
}

// NonNegative validates that an integer is non-negative (>= 0).
func NonNegative(field string, value int) error {
	if value < 0 {
		return NewResult(field, "must be non-negative", ErrOutOfRange)
	}
	return nil
}

// Duration validates a duration string and returns the parsed duration.
// Returns an error if the duration is invalid or outside the allowed range.
func Duration(field, value string) (time.Duration, error) {
	if value == "" {
		return 0, nil // Empty is valid (will use default)
	}

	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, NewResult(field, "invalid duration format", ErrInvalidDuration)
	}

	if d < 0 {
		return 0, NewResult(field, "duration cannot be negative", ErrOutOfRange)
	}

	return d, nil
}

// DurationRange validates a duration string and checks it's within bounds.
func DurationRange(field, value string, min, max time.Duration) (time.Duration, error) {
	d, err := Duration(field, value)
	if err != nil {
		return 0, err
	}

	if d != 0 && (d < min || d > max) {
		return 0, NewResult(field,
			fmt.Sprintf("must be between %s and %s", min, max),
			ErrOutOfRange)
	}

	return d, nil
}

// NodeID validates a node ID (hex-encoded Ed25519 public key).
func NodeID(field, value string) error {
	if err := Required(field, value); err != nil {
		return err
	}

	if !nodeIDPattern.MatchString(value) {
		return NewResult(field, "must be a 64-character hex string", ErrInvalidFormat)
	}

	return nil
}

// ConfigKey validates a configuration key.
func ConfigKey(field, value string) error {
	// Empty key is allowed (means "get all config")
	if value == "" {
		return nil
	}

	if err := MaxLength(field, value, MaxConfigKeyLength); err != nil {
		return err
	}

	if !configKeyPattern.MatchString(value) {
		return NewResult(field, "must start with a letter and contain only letters, numbers, dots, and underscores", ErrInvalidFormat)
	}

	return nil
}

// InviteCode validates an invite code format.
func InviteCode(field, value string) error {
	if err := Required(field, value); err != nil {
		return err
	}

	if err := MaxLength(field, value, MaxInviteCodeLength); err != nil {
		return err
	}

	if !strings.HasPrefix(value, "i2plan://") {
		return NewResult(field, "must start with 'i2plan://'", ErrInvalidFormat)
	}

	return nil
}

// Reason validates a ban reason.
func Reason(field, value string) error {
	// Reason is optional, but if provided, must be valid
	if value == "" {
		return nil
	}

	return MaxLength(field, value, MaxReasonLength)
}

// Description validates a description field.
func Description(field, value string) error {
	// Description is optional, but if provided, must be valid
	if value == "" {
		return nil
	}

	return MaxLength(field, value, MaxDescriptionLength)
}

// NodeName validates a node name.
func NodeName(field, value string) error {
	if err := Required(field, value); err != nil {
		return err
	}

	return MaxLength(field, value, MaxNodeNameLength)
}

// CIDR validates a CIDR notation string.
func CIDR(field, value string) error {
	if err := Required(field, value); err != nil {
		return err
	}

	_, _, err := net.ParseCIDR(value)
	if err != nil {
		return NewResult(field, "must be valid CIDR notation (e.g., 10.0.0.0/8)", ErrInvalidFormat)
	}

	return nil
}

// HostPort validates a host:port address.
func HostPort(field, value string) error {
	if err := Required(field, value); err != nil {
		return err
	}

	_, _, err := net.SplitHostPort(value)
	if err != nil {
		return NewResult(field, "must be in host:port format", ErrInvalidFormat)
	}

	return nil
}

// MaxUses validates the max_uses field for invites.
// -1 means unlimited, 0 means use default, positive means that many uses.
func MaxUses(field string, value int) error {
	if value < -1 {
		return NewResult(field, "must be -1 (unlimited), 0 (default), or positive", ErrOutOfRange)
	}
	return nil
}

// Port validates a network port number.
func Port(field string, value int) error {
	if value < 1 || value > 65535 {
		return NewResult(field, "must be between 1 and 65535", ErrOutOfRange)
	}
	return nil
}

// TunnelLength validates an I2P tunnel length.
func TunnelLength(field string, value int) error {
	if value < 0 || value > 7 {
		return NewResult(field, "must be between 0 and 7", ErrOutOfRange)
	}
	return nil
}

// All runs multiple validation functions and returns the first error.
func All(validators ...func() error) error {
	for _, v := range validators {
		if err := v(); err != nil {
			return err
		}
	}
	return nil
}

// Errors collects multiple validation errors.
type Errors []error

// Add appends an error to the collection (nil errors are ignored).
func (e *Errors) Add(err error) {
	if err != nil {
		*e = append(*e, err)
	}
}

// HasErrors returns true if any errors were collected.
func (e Errors) HasErrors() bool {
	return len(e) > 0
}

// Error returns all errors as a single error message.
func (e Errors) Error() string {
	if len(e) == 0 {
		return ""
	}
	if len(e) == 1 {
		return e[0].Error()
	}

	var b strings.Builder
	b.WriteString("multiple validation errors: ")
	for i, err := range e {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(err.Error())
	}
	return b.String()
}

// First returns the first error, or nil if none.
func (e Errors) First() error {
	if len(e) == 0 {
		return nil
	}
	return e[0]
}
