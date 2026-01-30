package errors

import (
	"errors"
	"testing"
)

// TestSentinelErrors verifies all sentinel errors are properly defined.
func TestSentinelErrors(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrNotFound", ErrNotFound},
		{"ErrAlreadyExists", ErrAlreadyExists},
		{"ErrInvalidInput", ErrInvalidInput},
		{"ErrUnauthorized", ErrUnauthorized},
		{"ErrForbidden", ErrForbidden},
		{"ErrTimeout", ErrTimeout},
		{"ErrUnavailable", ErrUnavailable},
		{"ErrRateLimited", ErrRateLimited},
		{"ErrClosed", ErrClosed},
		{"ErrNotOpen", ErrNotOpen},
		{"ErrAlreadyOpen", ErrAlreadyOpen},
		{"ErrInvalidState", ErrInvalidState},
		{"ErrConnection", ErrConnection},
		{"ErrInternal", ErrInternal},
		{"ErrConfiguration", ErrConfiguration},
	}

	for _, tc := range sentinels {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil {
				t.Errorf("%s should not be nil", tc.name)
			}
			if tc.err.Error() == "" {
				t.Errorf("%s should have a non-empty message", tc.name)
			}
		})
	}
}

// TestTransportErrors verifies transport-specific errors.
func TestTransportErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wraps   error
		message string
	}{
		{
			name:    "ErrTransportNotOpen",
			err:     ErrTransportNotOpen,
			wraps:   ErrNotOpen,
			message: "transport: not open",
		},
		{
			name:    "ErrTransportAlreadyOpen",
			err:     ErrTransportAlreadyOpen,
			wraps:   ErrAlreadyOpen,
			message: "transport: already open",
		},
		{
			name:    "ErrBindNotAvailable",
			err:     ErrBindNotAvailable,
			message: "transport: bind not available",
		},
		{
			name:    "ErrBindNotI2P",
			err:     ErrBindNotI2P,
			message: "transport: not an I2P bind",
		},
		{
			name:    "ErrPeerNotRegistered",
			err:     ErrPeerNotRegistered,
			message: "transport: peer not registered",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil {
				t.Fatalf("%s should not be nil", tc.name)
			}
			if tc.err.Error() != tc.message {
				t.Errorf("expected message %q, got %q", tc.message, tc.err.Error())
			}
			if tc.wraps != nil && !errors.Is(tc.err, tc.wraps) {
				t.Errorf("%s should wrap %v", tc.name, tc.wraps)
			}
		})
	}
}

// TestI2PErrors verifies I2P-specific errors.
func TestI2PErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		message string
	}{
		{
			name:    "ErrI2PBindNotOpen",
			err:     ErrI2PBindNotOpen,
			message: "i2pbind: not open",
		},
		{
			name:    "ErrI2PDatagramTooLarge",
			err:     ErrI2PDatagramTooLarge,
			message: "i2pbind: datagram exceeds I2P maximum size",
		},
		{
			name:    "ErrI2PParseAddress",
			err:     ErrI2PParseAddress,
			message: "i2pbind: could not parse sender address",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil {
				t.Fatalf("%s should not be nil", tc.name)
			}
			if tc.err.Error() != tc.message {
				t.Errorf("expected message %q, got %q", tc.message, tc.err.Error())
			}
		})
	}
}

// TestNodeErrors verifies node-specific errors.
func TestNodeErrors(t *testing.T) {
	if !errors.Is(ErrNodeConfigRequired, ErrInvalidInput) {
		t.Error("ErrNodeConfigRequired should wrap ErrInvalidInput")
	}
	if !errors.Is(ErrNodeInvalidConfig, ErrConfiguration) {
		t.Error("ErrNodeInvalidConfig should wrap ErrConfiguration")
	}
	if !errors.Is(ErrNodeInvalidState, ErrInvalidState) {
		t.Error("ErrNodeInvalidState should wrap ErrInvalidState")
	}
}

// TestRPCErrors verifies RPC-specific errors.
func TestRPCErrors(t *testing.T) {
	if !errors.Is(ErrRPCUnavailable, ErrUnavailable) {
		t.Error("ErrRPCUnavailable should wrap ErrUnavailable")
	}
	if ErrRPCTooManyConnections == nil {
		t.Error("ErrRPCTooManyConnections should not be nil")
	}
}

// TestWebErrors verifies web-specific errors.
func TestWebErrors(t *testing.T) {
	webErrs := []struct {
		name    string
		err     error
		message string
	}{
		{"ErrCSRFTokenMissing", ErrCSRFTokenMissing, "csrf: token missing"},
		{"ErrCSRFTokenInvalid", ErrCSRFTokenInvalid, "csrf: token invalid"},
		{"ErrCSRFTokenExpired", ErrCSRFTokenExpired, "csrf: token expired"},
	}

	for _, tc := range webErrs {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil {
				t.Fatalf("%s should not be nil", tc.name)
			}
			if tc.err.Error() != tc.message {
				t.Errorf("expected message %q, got %q", tc.message, tc.err.Error())
			}
		})
	}
}

// TestErrorCodes verifies error codes are unique and properly defined.
func TestErrorCodes(t *testing.T) {
	codes := map[int]string{
		CodeParseError:       "CodeParseError",
		CodeInvalidRequest:   "CodeInvalidRequest",
		CodeMethodNotFound:   "CodeMethodNotFound",
		CodeInvalidParams:    "CodeInvalidParams",
		CodeInternal:         "CodeInternal",
		CodeAuthRequired:     "CodeAuthRequired",
		CodePermissionDenied: "CodePermissionDenied",
		CodeNotFound:         "CodeNotFound",
		CodeRateLimited:      "CodeRateLimited",
		CodeTimeout:          "CodeTimeout",
		CodeConflict:         "CodeConflict",
		CodeUnavailable:      "CodeUnavailable",
		CodeValidation:       "CodeValidation",
		CodeConnection:       "CodeConnection",
		CodeState:            "CodeState",
	}

	if len(codes) != 15 {
		t.Errorf("expected 15 unique codes, got %d", len(codes))
	}

	// Verify JSON-RPC 2.0 standard codes
	if CodeParseError != -32700 {
		t.Error("CodeParseError should be -32700")
	}
	if CodeInvalidRequest != -32600 {
		t.Error("CodeInvalidRequest should be -32600")
	}
	if CodeMethodNotFound != -32601 {
		t.Error("CodeMethodNotFound should be -32601")
	}
	if CodeInvalidParams != -32602 {
		t.Error("CodeInvalidParams should be -32602")
	}
	if CodeInternal != -32603 {
		t.Error("CodeInternal should be -32603")
	}
}

// TestNew creates a new structured error.
func TestNew(t *testing.T) {
	err := New(CodeNotFound, "resource not found")

	if err.Code != CodeNotFound {
		t.Errorf("expected code %d, got %d", CodeNotFound, err.Code)
	}
	if err.Message != "resource not found" {
		t.Errorf("expected message %q, got %q", "resource not found", err.Message)
	}
	if err.Err != nil {
		t.Error("Err should be nil")
	}
	if err.Error() != "resource not found" {
		t.Errorf("expected error string %q, got %q", "resource not found", err.Error())
	}
	if err.SafeMessage() != "resource not found" {
		t.Errorf("expected safe message %q, got %q", "resource not found", err.SafeMessage())
	}
}

// TestWrap wraps an existing error.
func TestWrap(t *testing.T) {
	underlying := errors.New("database connection failed: host=192.168.1.1 password=secret")
	err := Wrap(CodeInternal, "database error", underlying)

	if err.Code != CodeInternal {
		t.Errorf("expected code %d, got %d", CodeInternal, err.Code)
	}
	if err.Message != "database error" {
		t.Errorf("expected message %q, got %q", "database error", err.Message)
	}
	if err.Err != underlying {
		t.Error("Err should be the underlying error")
	}
	if err.SafeMessage() != "database error" {
		t.Errorf("SafeMessage should not include sensitive data, got %q", err.SafeMessage())
	}
}

// TestWrapNil handles nil error.
func TestWrapNil(t *testing.T) {
	err := Wrap(CodeInternal, "test", nil)

	if err.Err != nil {
		t.Error("Err should be nil")
	}
	if err.Error() != "test" {
		t.Errorf("expected error string %q, got %q", "test", err.Error())
	}
}

// TestWrapInternal wraps with generic message.
func TestWrapInternal(t *testing.T) {
	sensitiveErr := errors.New("SQL error: SELECT * FROM users WHERE password='abc123'")
	err := WrapInternal(sensitiveErr)

	if err.Code != CodeInternal {
		t.Errorf("expected code %d, got %d", CodeInternal, err.Code)
	}
	if err.Message != "internal error" {
		t.Errorf("expected message %q, got %q", "internal error", err.Message)
	}
	if err.SafeMessage() != "internal error" {
		t.Errorf("SafeMessage should hide sensitive data, got %q", err.SafeMessage())
	}
	if !errors.Is(err, sensitiveErr) {
		t.Error("should wrap underlying error for debugging")
	}
}

// TestUnwrap verifies error unwrapping.
func TestUnwrap(t *testing.T) {
	underlying := errors.New("underlying")
	err := Wrap(CodeInternal, "wrapped", underlying)

	unwrapped := errors.Unwrap(err)
	if unwrapped != underlying {
		t.Error("Unwrap should return the underlying error")
	}
}

// TestFromSentinel creates error from sentinel.
func TestFromSentinel(t *testing.T) {
	tests := []struct {
		sentinel     error
		expectedCode int
	}{
		{ErrNotFound, CodeNotFound},
		{ErrUnauthorized, CodeAuthRequired},
		{ErrForbidden, CodePermissionDenied},
		{ErrRateLimited, CodeRateLimited},
		{ErrTimeout, CodeTimeout},
		{ErrUnavailable, CodeUnavailable},
		{ErrInvalidInput, CodeInvalidParams},
		{ErrInvalidState, CodeState},
		{ErrConnection, CodeConnection},
		{ErrAlreadyExists, CodeConflict},
		{ErrInternal, CodeInternal},
	}

	for _, tc := range tests {
		t.Run(tc.sentinel.Error(), func(t *testing.T) {
			err := FromSentinel(tc.sentinel)
			if err.Code != tc.expectedCode {
				t.Errorf("expected code %d, got %d", tc.expectedCode, err.Code)
			}
			if !errors.Is(err, tc.sentinel) {
				t.Error("should wrap sentinel error")
			}
		})
	}
}

// TestFromSentinelNil handles nil input.
func TestFromSentinelNil(t *testing.T) {
	err := FromSentinel(nil)
	if err != nil {
		t.Error("FromSentinel(nil) should return nil")
	}
}

// TestFromSentinelUnknown handles unknown errors.
func TestFromSentinelUnknown(t *testing.T) {
	unknownErr := errors.New("some unknown error")
	err := FromSentinel(unknownErr)

	if err.Code != CodeInternal {
		t.Errorf("unknown errors should get CodeInternal, got %d", err.Code)
	}
}

// TestIsHelpers verify error checking helpers.
func TestIsHelpers(t *testing.T) {
	tests := []struct {
		name   string
		fn     func(error) bool
		err    error
		expect bool
	}{
		{"IsNotFound-true", IsNotFound, ErrNotFound, true},
		{"IsNotFound-wrapped", IsNotFound, Wrap(CodeNotFound, "not found", ErrNotFound), true},
		{"IsNotFound-false", IsNotFound, ErrInternal, false},
		{"IsUnauthorized-true", IsUnauthorized, ErrUnauthorized, true},
		{"IsUnauthorized-false", IsUnauthorized, ErrInternal, false},
		{"IsForbidden-true", IsForbidden, ErrForbidden, true},
		{"IsForbidden-false", IsForbidden, ErrInternal, false},
		{"IsTimeout-true", IsTimeout, ErrTimeout, true},
		{"IsTimeout-false", IsTimeout, ErrInternal, false},
		{"IsUnavailable-true", IsUnavailable, ErrUnavailable, true},
		{"IsUnavailable-false", IsUnavailable, ErrInternal, false},
		{"IsRateLimited-true", IsRateLimited, ErrRateLimited, true},
		{"IsRateLimited-false", IsRateLimited, ErrInternal, false},
		{"IsInvalidInput-true", IsInvalidInput, ErrInvalidInput, true},
		{"IsInvalidInput-false", IsInvalidInput, ErrInternal, false},
		{"IsInvalidState-true", IsInvalidState, ErrInvalidState, true},
		{"IsInvalidState-false", IsInvalidState, ErrInternal, false},
		{"IsClosed-true", IsClosed, ErrClosed, true},
		{"IsClosed-false", IsClosed, ErrInternal, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.fn(tc.err); got != tc.expect {
				t.Errorf("expected %v, got %v", tc.expect, got)
			}
		})
	}
}

// TestJoin combines multiple errors.
func TestJoin(t *testing.T) {
	err1 := errors.New("first")
	err2 := errors.New("second")

	joined := Join(err1, err2)
	if joined == nil {
		t.Fatal("Join should return a non-nil error")
	}
	if !errors.Is(joined, err1) {
		t.Error("joined error should contain err1")
	}
	if !errors.Is(joined, err2) {
		t.Error("joined error should contain err2")
	}
}

// TestJoinAllNil returns nil when all are nil.
func TestJoinAllNil(t *testing.T) {
	if Join(nil, nil, nil) != nil {
		t.Error("Join of all nils should return nil")
	}
}

// TestIsAs test Is and As wrappers.
func TestIsAs(t *testing.T) {
	underlying := ErrNotFound
	wrapped := Wrap(CodeNotFound, "wrapped", underlying)

	if !Is(wrapped, underlying) {
		t.Error("Is should find wrapped error")
	}

	var target *Error
	if !As(wrapped, &target) {
		t.Error("As should find *Error type")
	}
	if target.Code != CodeNotFound {
		t.Error("As target should have correct code")
	}
}

// TestErrorWithUnderlying shows full error with underlying.
func TestErrorWithUnderlying(t *testing.T) {
	underlying := errors.New("connection refused")
	err := Wrap(CodeConnection, "connection failed", underlying)

	errorStr := err.Error()
	expected := "connection failed: connection refused"
	if errorStr != expected {
		t.Errorf("expected %q, got %q", expected, errorStr)
	}

	// SafeMessage should not include underlying
	if err.SafeMessage() != "connection failed" {
		t.Errorf("SafeMessage should be just %q, got %q", "connection failed", err.SafeMessage())
	}
}

// TestErrorSensitiveDataNotExposed verifies sensitive data is not leaked.
func TestErrorSensitiveDataNotExposed(t *testing.T) {
	// Simulate sensitive internal error
	sensitiveErr := errors.New("postgresql://user:password123@db.internal:5432/prod?sslmode=disable")
	err := Wrap(CodeInternal, "database connection failed", sensitiveErr)

	safeMsg := err.SafeMessage()
	if containsStr(safeMsg, "password123") {
		t.Error("SafeMessage should not contain password")
	}
	if containsStr(safeMsg, "db.internal") {
		t.Error("SafeMessage should not contain internal hostname")
	}
	if containsStr(safeMsg, "5432") {
		t.Error("SafeMessage should not contain port")
	}
	if safeMsg != "database connection failed" {
		t.Errorf("expected safe message, got %q", safeMsg)
	}
}

// TestWrapInternalHidesSensitiveData verifies WrapInternal hides all details.
func TestWrapInternalHidesSensitiveData(t *testing.T) {
	sensitiveErr := errors.New("API_KEY=sk_live_abc123 failed for user admin@example.com")
	err := WrapInternal(sensitiveErr)

	safeMsg := err.SafeMessage()
	if containsStr(safeMsg, "API_KEY") {
		t.Error("SafeMessage should not contain API key")
	}
	if containsStr(safeMsg, "admin@example.com") {
		t.Error("SafeMessage should not contain email")
	}
	if safeMsg != "internal error" {
		t.Errorf("expected 'internal error', got %q", safeMsg)
	}
}

// containsStr checks if s contains substr.
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
