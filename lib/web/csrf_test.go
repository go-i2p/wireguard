package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCSRFToken_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		createdAt time.Time
		want      bool
	}{
		{
			name:      "fresh token",
			createdAt: time.Now(),
			want:      false,
		},
		{
			name:      "token at expiry boundary minus 1s",
			createdAt: time.Now().Add(-CSRFTokenExpiry + time.Second),
			want:      false,
		},
		{
			name:      "expired token",
			createdAt: time.Now().Add(-CSRFTokenExpiry - time.Second),
			want:      true,
		},
		{
			name:      "very old token",
			createdAt: time.Now().Add(-24 * time.Hour),
			want:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			token := &CSRFToken{
				Token:     "test",
				CreatedAt: tc.createdAt,
			}
			if got := token.IsExpired(); got != tc.want {
				t.Errorf("IsExpired() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCSRFManager_GenerateToken(t *testing.T) {
	m := NewCSRFManager()

	t.Run("generates unique tokens", func(t *testing.T) {
		tokens := make(map[string]bool)
		for i := 0; i < 100; i++ {
			token, err := m.GenerateToken()
			if err != nil {
				t.Fatalf("GenerateToken() error = %v", err)
			}
			if token == "" {
				t.Error("GenerateToken() returned empty token")
			}
			if tokens[token] {
				t.Errorf("GenerateToken() returned duplicate token: %s", token)
			}
			tokens[token] = true
		}
	})

	t.Run("token length is consistent", func(t *testing.T) {
		token, err := m.GenerateToken()
		if err != nil {
			t.Fatalf("GenerateToken() error = %v", err)
		}
		// base64 of 32 bytes = 44 characters
		if len(token) != 44 {
			t.Errorf("token length = %d, want 44", len(token))
		}
	})

	t.Run("stores token", func(t *testing.T) {
		token, _ := m.GenerateToken()
		if m.TokenCount() == 0 {
			t.Error("token not stored")
		}
		if err := m.ValidateToken(token); err != nil {
			t.Errorf("stored token is invalid: %v", err)
		}
	})
}

func TestCSRFManager_ValidateToken(t *testing.T) {
	m := NewCSRFManager()

	t.Run("valid token", func(t *testing.T) {
		token, _ := m.GenerateToken()
		if err := m.ValidateToken(token); err != nil {
			t.Errorf("ValidateToken() error = %v", err)
		}
	})

	t.Run("empty token", func(t *testing.T) {
		err := m.ValidateToken("")
		if !errors.Is(err, ErrCSRFTokenMissing) {
			t.Errorf("ValidateToken('') error = %v, want %v", err, ErrCSRFTokenMissing)
		}
	})

	t.Run("unknown token", func(t *testing.T) {
		err := m.ValidateToken("nonexistent-token")
		if !errors.Is(err, ErrCSRFTokenInvalid) {
			t.Errorf("ValidateToken(unknown) error = %v, want %v", err, ErrCSRFTokenInvalid)
		}
	})

	t.Run("expired token", func(t *testing.T) {
		token, _ := m.GenerateToken()
		// Manually expire the token
		m.mu.Lock()
		m.tokens[token].CreatedAt = time.Now().Add(-CSRFTokenExpiry - time.Hour)
		m.mu.Unlock()

		err := m.ValidateToken(token)
		if !errors.Is(err, ErrCSRFTokenExpired) {
			t.Errorf("ValidateToken(expired) error = %v, want %v", err, ErrCSRFTokenExpired)
		}

		// Expired token should be cleaned up
		if err := m.ValidateToken(token); !errors.Is(err, ErrCSRFTokenInvalid) {
			t.Errorf("expired token should be removed, got error = %v", err)
		}
	})

	t.Run("token is reusable", func(t *testing.T) {
		token, _ := m.GenerateToken()

		// Validate multiple times
		for i := 0; i < 5; i++ {
			if err := m.ValidateToken(token); err != nil {
				t.Errorf("ValidateToken() iteration %d error = %v", i, err)
			}
		}
	})
}

func TestCSRFManager_ValidateTokenConstantTime(t *testing.T) {
	m := NewCSRFManager()

	t.Run("valid token", func(t *testing.T) {
		token, _ := m.GenerateToken()
		if err := m.ValidateTokenConstantTime(token); err != nil {
			t.Errorf("ValidateTokenConstantTime() error = %v", err)
		}
	})

	t.Run("empty token", func(t *testing.T) {
		err := m.ValidateTokenConstantTime("")
		if !errors.Is(err, ErrCSRFTokenMissing) {
			t.Errorf("error = %v, want %v", err, ErrCSRFTokenMissing)
		}
	})

	t.Run("unknown token", func(t *testing.T) {
		err := m.ValidateTokenConstantTime("nonexistent-token")
		if !errors.Is(err, ErrCSRFTokenInvalid) {
			t.Errorf("error = %v, want %v", err, ErrCSRFTokenInvalid)
		}
	})

	t.Run("expired token", func(t *testing.T) {
		token, _ := m.GenerateToken()
		m.mu.Lock()
		m.tokens[token].CreatedAt = time.Now().Add(-CSRFTokenExpiry - time.Hour)
		m.mu.Unlock()

		err := m.ValidateTokenConstantTime(token)
		if !errors.Is(err, ErrCSRFTokenExpired) {
			t.Errorf("error = %v, want %v", err, ErrCSRFTokenExpired)
		}
	})
}

func TestCSRFManager_Cleanup(t *testing.T) {
	m := NewCSRFManager()

	// Generate some tokens
	validToken, _ := m.GenerateToken()
	expiredToken1, _ := m.GenerateToken()
	expiredToken2, _ := m.GenerateToken()

	// Expire some tokens
	m.mu.Lock()
	m.tokens[expiredToken1].CreatedAt = time.Now().Add(-CSRFTokenExpiry - time.Hour)
	m.tokens[expiredToken2].CreatedAt = time.Now().Add(-CSRFTokenExpiry - time.Hour)
	m.mu.Unlock()

	// Run cleanup
	removed := m.Cleanup()

	if removed != 2 {
		t.Errorf("Cleanup() removed = %d, want 2", removed)
	}

	if m.TokenCount() != 1 {
		t.Errorf("TokenCount() = %d, want 1", m.TokenCount())
	}

	// Valid token should still work
	if err := m.ValidateToken(validToken); err != nil {
		t.Errorf("valid token should still work after cleanup: %v", err)
	}
}

func TestCSRFManager_StartCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cleanup test in short mode")
	}

	m := NewCSRFManager()

	// Generate an expired token
	token, _ := m.GenerateToken()
	m.mu.Lock()
	m.tokens[token].CreatedAt = time.Now().Add(-CSRFTokenExpiry - time.Hour)
	m.mu.Unlock()

	// Start cleanup with short interval
	stop := m.StartCleanup(50 * time.Millisecond)
	defer close(stop)

	// Wait for cleanup to run
	time.Sleep(100 * time.Millisecond)

	if m.TokenCount() != 0 {
		t.Errorf("expired token should be cleaned up, count = %d", m.TokenCount())
	}
}

func TestCSRFMiddleware(t *testing.T) {
	m := NewCSRFManager()
	token, _ := m.GenerateToken()

	// Create a simple handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	protected := m.CSRFMiddleware(handler)

	tests := []struct {
		name       string
		method     string
		csrfHeader string
		wantStatus int
	}{
		{
			name:       "GET without token passes",
			method:     http.MethodGet,
			csrfHeader: "",
			wantStatus: http.StatusOK,
		},
		{
			name:       "HEAD without token passes",
			method:     http.MethodHead,
			csrfHeader: "",
			wantStatus: http.StatusOK,
		},
		{
			name:       "OPTIONS without token passes",
			method:     http.MethodOptions,
			csrfHeader: "",
			wantStatus: http.StatusOK,
		},
		{
			name:       "POST without token fails",
			method:     http.MethodPost,
			csrfHeader: "",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "PUT without token fails",
			method:     http.MethodPut,
			csrfHeader: "",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "DELETE without token fails",
			method:     http.MethodDelete,
			csrfHeader: "",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "PATCH without token fails",
			method:     http.MethodPatch,
			csrfHeader: "",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "POST with valid token passes",
			method:     http.MethodPost,
			csrfHeader: token,
			wantStatus: http.StatusOK,
		},
		{
			name:       "PUT with valid token passes",
			method:     http.MethodPut,
			csrfHeader: token,
			wantStatus: http.StatusOK,
		},
		{
			name:       "DELETE with valid token passes",
			method:     http.MethodDelete,
			csrfHeader: token,
			wantStatus: http.StatusOK,
		},
		{
			name:       "POST with invalid token fails",
			method:     http.MethodPost,
			csrfHeader: "invalid-token",
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/api/test", nil)
			if tc.csrfHeader != "" {
				req.Header.Set(CSRFHeaderName, tc.csrfHeader)
			}

			w := httptest.NewRecorder()
			protected.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
		})
	}
}

func TestCSRFMiddleware_FormValue(t *testing.T) {
	m := NewCSRFManager()
	token, _ := m.GenerateToken()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	protected := m.CSRFMiddleware(handler)

	// Test form value extraction
	req := httptest.NewRequest(http.MethodPost, "/api/test?"+CSRFFieldName+"="+token, nil)
	w := httptest.NewRecorder()
	protected.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("form value CSRF: status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestSetCSRFCookie(t *testing.T) {
	w := httptest.NewRecorder()
	token := "test-token-value"

	SetCSRFCookie(w, token)

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}

	cookie := cookies[0]
	if cookie.Name != CSRFCookieName {
		t.Errorf("cookie name = %q, want %q", cookie.Name, CSRFCookieName)
	}
	if cookie.Value != token {
		t.Errorf("cookie value = %q, want %q", cookie.Value, token)
	}
	if cookie.Path != "/" {
		t.Errorf("cookie path = %q, want %q", cookie.Path, "/")
	}
	if cookie.HttpOnly {
		t.Error("cookie should not be HttpOnly (JS needs to read it)")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie SameSite = %v, want %v", cookie.SameSite, http.SameSiteStrictMode)
	}
}

func TestGetCSRFFromCookie(t *testing.T) {
	t.Run("cookie exists", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{
			Name:  CSRFCookieName,
			Value: "test-token",
		})

		got := GetCSRFFromCookie(req)
		if got != "test-token" {
			t.Errorf("GetCSRFFromCookie() = %q, want %q", got, "test-token")
		}
	})

	t.Run("cookie missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		got := GetCSRFFromCookie(req)
		if got != "" {
			t.Errorf("GetCSRFFromCookie() = %q, want empty", got)
		}
	})
}

func TestIsSafeMethod(t *testing.T) {
	safeTests := []struct {
		method string
		want   bool
	}{
		{http.MethodGet, true},
		{http.MethodHead, true},
		{http.MethodOptions, true},
		{http.MethodTrace, true},
		{http.MethodPost, false},
		{http.MethodPut, false},
		{http.MethodPatch, false},
		{http.MethodDelete, false},
		{"CUSTOM", false},
	}

	for _, tc := range safeTests {
		t.Run(tc.method, func(t *testing.T) {
			if got := isSafeMethod(tc.method); got != tc.want {
				t.Errorf("isSafeMethod(%q) = %v, want %v", tc.method, got, tc.want)
			}
		})
	}
}

func TestCSRFConstants(t *testing.T) {
	// Verify constants are set correctly
	if CSRFTokenLength != 32 {
		t.Errorf("CSRFTokenLength = %d, want 32", CSRFTokenLength)
	}
	if CSRFHeaderName != "X-CSRF-Token" {
		t.Errorf("CSRFHeaderName = %q, want %q", CSRFHeaderName, "X-CSRF-Token")
	}
	if CSRFCookieName != "csrf_token" {
		t.Errorf("CSRFCookieName = %q, want %q", CSRFCookieName, "csrf_token")
	}
	if CSRFFieldName != "csrf_token" {
		t.Errorf("CSRFFieldName = %q, want %q", CSRFFieldName, "csrf_token")
	}
	if CSRFTokenExpiry != 12*time.Hour {
		t.Errorf("CSRFTokenExpiry = %v, want %v", CSRFTokenExpiry, 12*time.Hour)
	}
}

func TestCSRFErrors(t *testing.T) {
	// Verify errors are distinct
	if ErrCSRFTokenMissing == ErrCSRFTokenInvalid {
		t.Error("ErrCSRFTokenMissing should not equal ErrCSRFTokenInvalid")
	}
	if ErrCSRFTokenInvalid == ErrCSRFTokenExpired {
		t.Error("ErrCSRFTokenInvalid should not equal ErrCSRFTokenExpired")
	}

	// Verify error messages
	if ErrCSRFTokenMissing.Error() != "csrf: token missing" {
		t.Errorf("ErrCSRFTokenMissing.Error() = %q", ErrCSRFTokenMissing.Error())
	}
	if ErrCSRFTokenInvalid.Error() != "csrf: token invalid" {
		t.Errorf("ErrCSRFTokenInvalid.Error() = %q", ErrCSRFTokenInvalid.Error())
	}
	if ErrCSRFTokenExpired.Error() != "csrf: token expired" {
		t.Errorf("ErrCSRFTokenExpired.Error() = %q", ErrCSRFTokenExpired.Error())
	}
}
