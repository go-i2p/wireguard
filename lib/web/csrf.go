// Package web provides CSRF protection for the management UI.
package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"sync"
	"time"
)

// CSRF configuration constants
const (
	// CSRFTokenLength is the byte length of the raw CSRF token
	CSRFTokenLength = 32

	// CSRFHeaderName is the HTTP header name for CSRF tokens
	CSRFHeaderName = "X-CSRF-Token"

	// CSRFCookieName is the cookie name for the CSRF token
	CSRFCookieName = "csrf_token"

	// CSRFFieldName is the form field name for the CSRF token
	CSRFFieldName = "csrf_token"

	// CSRFTokenExpiry is how long tokens remain valid
	CSRFTokenExpiry = 12 * time.Hour
)

// CSRF errors
var (
	// ErrCSRFTokenMissing indicates no CSRF token was provided
	ErrCSRFTokenMissing = errors.New("csrf: token missing")

	// ErrCSRFTokenInvalid indicates the token didn't match
	ErrCSRFTokenInvalid = errors.New("csrf: token invalid")

	// ErrCSRFTokenExpired indicates the token has expired
	ErrCSRFTokenExpired = errors.New("csrf: token expired")
)

// CSRFToken represents a CSRF token with metadata.
type CSRFToken struct {
	Token     string
	CreatedAt time.Time
}

// IsExpired returns true if the token has expired.
func (t *CSRFToken) IsExpired() bool {
	return time.Since(t.CreatedAt) > CSRFTokenExpiry
}

// CSRFManager handles CSRF token generation and validation.
// It maintains an in-memory store of valid tokens with automatic cleanup.
type CSRFManager struct {
	tokens map[string]*CSRFToken
	mu     sync.RWMutex
}

// NewCSRFManager creates a new CSRF manager.
// Call StartCleanup() to enable automatic token expiration cleanup.
func NewCSRFManager() *CSRFManager {
	return &CSRFManager{
		tokens: make(map[string]*CSRFToken),
	}
}

// GenerateToken creates a new CSRF token and stores it.
func (m *CSRFManager) GenerateToken() (string, error) {
	bytes := make([]byte, CSRFTokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	token := base64.URLEncoding.EncodeToString(bytes)

	m.mu.Lock()
	m.tokens[token] = &CSRFToken{
		Token:     token,
		CreatedAt: time.Now(),
	}
	m.mu.Unlock()

	return token, nil
}

// ValidateToken checks if a token is valid and not expired.
// Valid tokens are NOT consumed (they can be reused within the expiry period).
func (m *CSRFManager) ValidateToken(token string) error {
	if token == "" {
		return ErrCSRFTokenMissing
	}

	m.mu.RLock()
	stored, exists := m.tokens[token]
	m.mu.RUnlock()

	if !exists {
		return ErrCSRFTokenInvalid
	}

	if stored.IsExpired() {
		// Clean up expired token
		m.mu.Lock()
		delete(m.tokens, token)
		m.mu.Unlock()
		return ErrCSRFTokenExpired
	}

	return nil
}

// ValidateTokenConstantTime validates using constant-time comparison.
// This prevents timing attacks when comparing tokens.
func (m *CSRFManager) ValidateTokenConstantTime(provided string) error {
	if provided == "" {
		return ErrCSRFTokenMissing
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Search through all tokens with constant-time comparison
	var found *CSRFToken
	for storedToken, tokenData := range m.tokens {
		if subtle.ConstantTimeCompare([]byte(provided), []byte(storedToken)) == 1 {
			found = tokenData
			break
		}
	}

	if found == nil {
		return ErrCSRFTokenInvalid
	}

	if found.IsExpired() {
		return ErrCSRFTokenExpired
	}

	return nil
}

// Cleanup removes expired tokens from the store.
func (m *CSRFManager) Cleanup() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	removed := 0
	for token, data := range m.tokens {
		if data.IsExpired() {
			delete(m.tokens, token)
			removed++
		}
	}

	return removed
}

// StartCleanup begins periodic cleanup of expired tokens.
// Returns a channel that should be closed to stop the cleanup goroutine.
func (m *CSRFManager) StartCleanup(interval time.Duration) chan struct{} {
	stop := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				removed := m.Cleanup()
				if removed > 0 {
					log.Debug("csrf cleanup", "removed", removed)
				}
			case <-stop:
				return
			}
		}
	}()

	return stop
}

// TokenCount returns the number of stored tokens (for testing/metrics).
func (m *CSRFManager) TokenCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.tokens)
}

// CSRFMiddleware creates a middleware that enforces CSRF protection.
// It validates tokens on POST, PUT, DELETE, and PATCH requests.
// Safe methods (GET, HEAD, OPTIONS) pass through without validation.
func (m *CSRFManager) CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip CSRF for safe methods
		if isSafeMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		// Extract token from header or form
		token := r.Header.Get(CSRFHeaderName)
		if token == "" {
			token = r.FormValue(CSRFFieldName)
		}

		// Validate the token
		if err := m.ValidateTokenConstantTime(token); err != nil {
			log.Warn("csrf validation failed",
				"method", r.Method,
				"path", r.URL.Path,
				"error", err,
			)
			http.Error(w, "Forbidden - CSRF validation failed", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// isSafeMethod returns true for HTTP methods that don't modify state.
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

// SetCSRFCookie sets the CSRF token cookie on the response.
// The cookie is HttpOnly=false so JavaScript can read it for AJAX requests.
// SameSite=Strict prevents the cookie from being sent in cross-origin requests.
func SetCSRFCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false, // JS needs to read this
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(CSRFTokenExpiry.Seconds()),
	})
}

// GetCSRFFromCookie extracts the CSRF token from the request cookie.
func GetCSRFFromCookie(r *http.Request) string {
	cookie, err := r.Cookie(CSRFCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}
