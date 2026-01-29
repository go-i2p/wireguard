package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTemplateFuncs(t *testing.T) {
	funcs := templateFuncs()

	t.Run("truncate", func(t *testing.T) {
		truncate := funcs["truncate"].(func(string, int) string)

		tests := []struct {
			input    string
			n        int
			expected string
		}{
			{"short", 10, "short"},
			{"exactly10!", 10, "exactly10!"},
			{"this is long", 10, "this is..."},
			{"abc", 3, "abc"},
			{"abcd", 3, "abc"},
		}

		for _, tc := range tests {
			got := truncate(tc.input, tc.n)
			if got != tc.expected {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.n, got, tc.expected)
			}
		}
	})

	t.Run("json", func(t *testing.T) {
		jsonFunc := funcs["json"].(func(any) string)

		result := jsonFunc(map[string]string{"key": "value"})
		if result != `{"key":"value"}` {
			t.Errorf("json(map) = %q, want %q", result, `{"key":"value"}`)
		}
	})
}

func TestWriteJSON(t *testing.T) {
	// Create a minimal server just to test writeJSON
	s := &Server{}

	t.Run("writes json response", func(t *testing.T) {
		w := httptest.NewRecorder()
		data := map[string]string{"message": "hello"}

		s.writeJSON(w, http.StatusOK, data)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}

		contentType := w.Header().Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
		}

		var result map[string]string
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result["message"] != "hello" {
			t.Errorf("message = %q, want %q", result["message"], "hello")
		}
	})
}

func TestWriteError(t *testing.T) {
	s := &Server{}

	w := httptest.NewRecorder()
	s.writeError(w, http.StatusBadRequest, "invalid request")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["error"] != "invalid request" {
		t.Errorf("error = %q, want %q", result["error"], "invalid request")
	}
}

func TestInviteCreateRequest(t *testing.T) {
	req := InviteCreateRequest{
		Expiry:  "24h",
		MaxUses: 5,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed InviteCreateRequest
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.Expiry != "24h" {
		t.Errorf("Expiry = %q, want %q", parsed.Expiry, "24h")
	}
	if parsed.MaxUses != 5 {
		t.Errorf("MaxUses = %d, want %d", parsed.MaxUses, 5)
	}
}

func TestInviteAcceptRequest(t *testing.T) {
	req := InviteAcceptRequest{
		InviteCode: "i2plan://test123",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if !strings.Contains(string(data), "i2plan://test123") {
		t.Errorf("expected invite code in json, got %s", string(data))
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := Config{
		ListenAddr:    "127.0.0.1:8080",
		RPCSocketPath: "/tmp/test.sock",
		RPCAuthFile:   "/tmp/auth.token",
	}

	if cfg.ListenAddr != "127.0.0.1:8080" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, "127.0.0.1:8080")
	}
}
