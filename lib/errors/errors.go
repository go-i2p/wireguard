// Package errors provides structured error types for the i2plan mesh VPN.
// All errors are designed to be safe to return to clients without exposing
// internal implementation details.
//
// This package provides:
//   - Sentinel errors for common error conditions
//   - Error codes for RPC response categorization
//   - Error wrapping with context preservation
//   - Safe error messages that don't leak sensitive information
package errors

import (
	"errors"
	"fmt"

	"github.com/go-i2p/logger"
)

var log = logger.GetGoI2PLogger()

// Error codes for categorizing errors. These align with JSON-RPC 2.0 error codes
// where applicable, with custom codes in the -32000 to -32099 range.
const (
	// Standard JSON-RPC 2.0 error codes
	CodeParseError     = -32700 // Invalid JSON
	CodeInvalidRequest = -32600 // Invalid request object
	CodeMethodNotFound = -32601 // Method not found
	CodeInvalidParams  = -32602 // Invalid method parameters
	CodeInternal       = -32603 // Internal error

	// Application-specific error codes (-32000 to -32099)
	CodeAuthRequired     = -32001 // Authentication required
	CodePermissionDenied = -32002 // Permission denied
	CodeNotFound         = -32003 // Resource not found
	CodeRateLimited      = -32004 // Rate limit exceeded
	CodeTimeout          = -32005 // Operation timeout
	CodeConflict         = -32006 // Resource conflict
	CodeUnavailable      = -32007 // Service unavailable
	CodeValidation       = -32008 // Validation failed
	CodeConnection       = -32009 // Connection error
	CodeState            = -32010 // Invalid state
)

// Sentinel errors for common error conditions.
// Use errors.Is() to check for these conditions.
var (
	// ErrNotFound indicates a resource was not found.
	ErrNotFound = errors.New("not found")

	// ErrAlreadyExists indicates a resource already exists.
	ErrAlreadyExists = errors.New("already exists")

	// ErrInvalidInput indicates invalid input was provided.
	ErrInvalidInput = errors.New("invalid input")

	// ErrUnauthorized indicates authentication is required.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden indicates the operation is not permitted.
	ErrForbidden = errors.New("forbidden")

	// ErrTimeout indicates an operation timed out.
	ErrTimeout = errors.New("operation timed out")

	// ErrUnavailable indicates a service is unavailable.
	ErrUnavailable = errors.New("service unavailable")

	// ErrRateLimited indicates a rate limit was exceeded.
	ErrRateLimited = errors.New("rate limit exceeded")

	// ErrClosed indicates a resource is closed.
	ErrClosed = errors.New("closed")

	// ErrNotOpen indicates a resource is not open.
	ErrNotOpen = errors.New("not open")

	// ErrAlreadyOpen indicates a resource is already open.
	ErrAlreadyOpen = errors.New("already open")

	// ErrInvalidState indicates an invalid state transition.
	ErrInvalidState = errors.New("invalid state")

	// ErrConnection indicates a connection error.
	ErrConnection = errors.New("connection error")

	// ErrInternal indicates an internal error.
	ErrInternal = errors.New("internal error")

	// ErrConfiguration indicates a configuration error.
	ErrConfiguration = errors.New("configuration error")

	// ErrCircuitOpen indicates the circuit breaker is open.
	ErrCircuitOpen = errors.New("circuit breaker is open")
)

// Transport-specific errors
var (
	// ErrTransportNotOpen indicates the transport is not open.
	ErrTransportNotOpen = fmt.Errorf("transport: %w", ErrNotOpen)

	// ErrTransportAlreadyOpen indicates the transport is already open.
	ErrTransportAlreadyOpen = fmt.Errorf("transport: %w", ErrAlreadyOpen)

	// ErrBindNotAvailable indicates the bind is not available.
	ErrBindNotAvailable = errors.New("transport: bind not available")

	// ErrBindNotI2P indicates the bind is not an I2P bind.
	ErrBindNotI2P = errors.New("transport: not an I2P bind")

	// ErrPeerNotRegistered indicates a peer is not registered.
	ErrPeerNotRegistered = errors.New("transport: peer not registered")
)

// I2P-specific errors
var (
	// ErrI2PBindNotOpen indicates the I2P bind is not open.
	ErrI2PBindNotOpen = errors.New("i2pbind: not open")

	// ErrI2PDatagramTooLarge indicates the datagram exceeds I2P limits.
	ErrI2PDatagramTooLarge = errors.New("i2pbind: datagram exceeds I2P maximum size")

	// ErrI2PParseAddress indicates a sender address could not be parsed.
	ErrI2PParseAddress = errors.New("i2pbind: could not parse sender address")
)

// Node errors
var (
	// ErrNodeConfigRequired indicates a configuration is required.
	ErrNodeConfigRequired = fmt.Errorf("node: config %w", ErrInvalidInput)

	// ErrNodeInvalidConfig indicates an invalid configuration.
	ErrNodeInvalidConfig = fmt.Errorf("node: %w", ErrConfiguration)

	// ErrNodeInvalidState indicates an invalid node state.
	ErrNodeInvalidState = fmt.Errorf("node: %w", ErrInvalidState)
)

// RPC errors
var (
	// ErrRPCUnavailable indicates RPC service is not available.
	ErrRPCUnavailable = fmt.Errorf("rpc: %w", ErrUnavailable)

	// ErrRPCTooManyConnections indicates too many connections.
	ErrRPCTooManyConnections = errors.New("rpc: too many connections")
)

// Web errors
var (
	// ErrCSRFTokenMissing indicates a CSRF token is missing.
	ErrCSRFTokenMissing = errors.New("csrf: token missing")

	// ErrCSRFTokenInvalid indicates an invalid CSRF token.
	ErrCSRFTokenInvalid = errors.New("csrf: token invalid")

	// ErrCSRFTokenExpired indicates an expired CSRF token.
	ErrCSRFTokenExpired = errors.New("csrf: token expired")
)

// Error is a structured error with a code and safe message.
// It implements the error interface and provides methods for
// error handling and response generation.
type Error struct {
	// Code is the error code for categorization
	Code int `json:"code"`
	// Message is a safe, user-facing error message
	Message string `json:"message"`
	// Err is the underlying error (not exposed to clients)
	Err error `json:"-"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap returns the underlying error for errors.Is/As.
func (e *Error) Unwrap() error {
	return e.Err
}

// SafeMessage returns a client-safe error message without internal details.
func (e *Error) SafeMessage() string {
	return e.Message
}

// New creates a new structured error with the given code and message.
// The message should be safe to return to clients.
func New(code int, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// Wrap wraps an existing error with a code and safe message.
// The original error is preserved for debugging but not exposed to clients.
func Wrap(code int, message string, err error) *Error {
	if err != nil {
		log.WithField("code", code).WithError(err).Debug("wrapping error")
	}
	return &Error{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// WrapInternal wraps an internal error with a generic message.
// Use this when the original error contains sensitive information.
func WrapInternal(err error) *Error {
	if err != nil {
		log.WithError(err).Debug("wrapping internal error")
	}
	return &Error{
		Code:    CodeInternal,
		Message: "internal error",
		Err:     err,
	}
}

// FromSentinel creates a structured error from a sentinel error.
// It automatically assigns an appropriate error code based on the error type.
func FromSentinel(err error) *Error {
	if err == nil {
		return nil
	}

	code := codeFromError(err)
	return &Error{
		Code:    code,
		Message: err.Error(),
		Err:     err,
	}
}

// codeFromError maps sentinel errors to error codes.
func codeFromError(err error) int {
	switch {
	case errors.Is(err, ErrNotFound):
		return CodeNotFound
	case errors.Is(err, ErrUnauthorized):
		return CodeAuthRequired
	case errors.Is(err, ErrForbidden):
		return CodePermissionDenied
	case errors.Is(err, ErrRateLimited):
		return CodeRateLimited
	case errors.Is(err, ErrTimeout):
		return CodeTimeout
	case errors.Is(err, ErrUnavailable):
		return CodeUnavailable
	case errors.Is(err, ErrInvalidInput):
		return CodeInvalidParams
	case errors.Is(err, ErrInvalidState):
		return CodeState
	case errors.Is(err, ErrConnection):
		return CodeConnection
	case errors.Is(err, ErrAlreadyExists):
		return CodeConflict
	default:
		return CodeInternal
	}
}

// IsNotFound returns true if the error indicates a resource was not found.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsUnauthorized returns true if the error indicates authentication is required.
func IsUnauthorized(err error) bool {
	return errors.Is(err, ErrUnauthorized)
}

// IsForbidden returns true if the error indicates the operation is not permitted.
func IsForbidden(err error) bool {
	return errors.Is(err, ErrForbidden)
}

// IsTimeout returns true if the error indicates a timeout.
func IsTimeout(err error) bool {
	return errors.Is(err, ErrTimeout)
}

// IsUnavailable returns true if the error indicates a service is unavailable.
func IsUnavailable(err error) bool {
	return errors.Is(err, ErrUnavailable)
}

// IsRateLimited returns true if the error indicates rate limiting.
func IsRateLimited(err error) bool {
	return errors.Is(err, ErrRateLimited)
}

// IsInvalidInput returns true if the error indicates invalid input.
func IsInvalidInput(err error) bool {
	return errors.Is(err, ErrInvalidInput)
}

// IsInvalidState returns true if the error indicates an invalid state.
func IsInvalidState(err error) bool {
	return errors.Is(err, ErrInvalidState)
}

// IsClosed returns true if the error indicates a resource is closed.
func IsClosed(err error) bool {
	return errors.Is(err, ErrClosed)
}

// Join combines multiple errors into a single error.
// Returns nil if all errors are nil.
func Join(errs ...error) error {
	return errors.Join(errs...)
}

// Is reports whether any error in err's tree matches target.
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// As finds the first error in err's tree that matches target,
// and if so, sets target to that error value and returns true.
func As(err error, target any) bool {
	return errors.As(err, target)
}
