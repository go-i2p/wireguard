package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-i2p/wireguard/lib/rpc"
)

// mockRPCClient is a mock implementation of RPCClient for testing.
type mockRPCClient struct {
	statusResult       *rpc.StatusResult
	statusErr          error
	peersResult        *rpc.PeersListResult
	peersErr           error
	routesResult       *rpc.RoutesListResult
	routesErr          error
	configResult       *rpc.ConfigGetResult
	configErr          error
	inviteCreateResult *rpc.InviteCreateResult
	inviteCreateErr    error
	inviteAcceptResult *rpc.InviteAcceptResult
	inviteAcceptErr    error
	closed             bool
}

func (m *mockRPCClient) Status(_ context.Context) (*rpc.StatusResult, error) {
	return m.statusResult, m.statusErr
}

func (m *mockRPCClient) PeersList(_ context.Context) (*rpc.PeersListResult, error) {
	return m.peersResult, m.peersErr
}

func (m *mockRPCClient) RoutesList(_ context.Context) (*rpc.RoutesListResult, error) {
	return m.routesResult, m.routesErr
}

func (m *mockRPCClient) ConfigGet(_ context.Context, _ string) (*rpc.ConfigGetResult, error) {
	return m.configResult, m.configErr
}

func (m *mockRPCClient) InviteCreate(_ context.Context, _ string, _ int) (*rpc.InviteCreateResult, error) {
	return m.inviteCreateResult, m.inviteCreateErr
}

func (m *mockRPCClient) InviteAccept(_ context.Context, _ string) (*rpc.InviteAcceptResult, error) {
	return m.inviteAcceptResult, m.inviteAcceptErr
}

func (m *mockRPCClient) Close() error {
	m.closed = true
	return nil
}

// newTestServer creates a server with a mock RPC client for testing.
func newTestServer(mock *mockRPCClient) *Server {
	return &Server{
		rpcClient:   mock,
		csrfManager: NewCSRFManager(),
	}
}

func TestHandleAPIStatus(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &mockRPCClient{
			statusResult: &rpc.StatusResult{
				NodeID:    "node-123",
				NodeName:  "test-node",
				State:     "running",
				PeerCount: 5,
				Uptime:    "1h30m",
				Version:   "1.0.0",
			},
		}
		s := newTestServer(mock)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/status", nil)

		s.handleAPIStatus(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var result rpc.StatusResult
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result.NodeID != "node-123" {
			t.Errorf("NodeID = %q, want %q", result.NodeID, "node-123")
		}
		if result.PeerCount != 5 {
			t.Errorf("PeerCount = %d, want %d", result.PeerCount, 5)
		}
	})

	t.Run("rpc error", func(t *testing.T) {
		mock := &mockRPCClient{
			statusErr: errors.New("connection failed"),
		}
		s := newTestServer(mock)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/status", nil)

		s.handleAPIStatus(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}

		var result map[string]string
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result["error"] != "connection failed" {
			t.Errorf("error = %q, want %q", result["error"], "connection failed")
		}
	})
}

func TestHandleAPIPeers(t *testing.T) {
	t.Run("success with peers", func(t *testing.T) {
		mock := &mockRPCClient{
			peersResult: &rpc.PeersListResult{
				Peers: []rpc.PeerInfo{
					{NodeID: "peer-1", TunnelIP: "10.0.0.1", State: "Connected"},
					{NodeID: "peer-2", TunnelIP: "10.0.0.2", State: "Connecting"},
				},
				Total: 2,
			},
		}
		s := newTestServer(mock)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/peers", nil)

		s.handleAPIPeers(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var result rpc.PeersListResult
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result.Total != 2 {
			t.Errorf("Total = %d, want %d", result.Total, 2)
		}
		if len(result.Peers) != 2 {
			t.Errorf("len(Peers) = %d, want %d", len(result.Peers), 2)
		}
	})

	t.Run("success with no peers", func(t *testing.T) {
		mock := &mockRPCClient{
			peersResult: &rpc.PeersListResult{
				Peers: []rpc.PeerInfo{},
				Total: 0,
			},
		}
		s := newTestServer(mock)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/peers", nil)

		s.handleAPIPeers(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var result rpc.PeersListResult
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result.Total != 0 {
			t.Errorf("Total = %d, want %d", result.Total, 0)
		}
	})

	t.Run("rpc error", func(t *testing.T) {
		mock := &mockRPCClient{
			peersErr: errors.New("rpc timeout"),
		}
		s := newTestServer(mock)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/peers", nil)

		s.handleAPIPeers(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})
}

func TestHandleAPIRoutes(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &mockRPCClient{
			routesResult: &rpc.RoutesListResult{
				Routes: []rpc.RouteInfo{
					{NodeID: "node-1", TunnelIP: "10.0.0.1", HopCount: 1},
					{NodeID: "node-2", TunnelIP: "10.0.0.2", HopCount: 2, ViaNodeID: "node-1"},
				},
				Total: 2,
			},
		}
		s := newTestServer(mock)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/routes", nil)

		s.handleAPIRoutes(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var result rpc.RoutesListResult
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result.Total != 2 {
			t.Errorf("Total = %d, want %d", result.Total, 2)
		}
	})

	t.Run("rpc error", func(t *testing.T) {
		mock := &mockRPCClient{
			routesErr: errors.New("routes unavailable"),
		}
		s := newTestServer(mock)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/routes", nil)

		s.handleAPIRoutes(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})
}

func TestHandleAPIInviteCreate(t *testing.T) {
	t.Run("success with defaults", func(t *testing.T) {
		mock := &mockRPCClient{
			inviteCreateResult: &rpc.InviteCreateResult{
				InviteCode: "i2plan://abc123",
				ExpiresAt:  "2026-01-30T12:00:00Z",
				MaxUses:    1,
			},
		}
		s := newTestServer(mock)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/invite/create", nil)

		s.handleAPIInviteCreate(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var result rpc.InviteCreateResult
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result.InviteCode != "i2plan://abc123" {
			t.Errorf("InviteCode = %q, want %q", result.InviteCode, "i2plan://abc123")
		}
	})

	t.Run("success with custom params", func(t *testing.T) {
		mock := &mockRPCClient{
			inviteCreateResult: &rpc.InviteCreateResult{
				InviteCode: "i2plan://xyz789",
				ExpiresAt:  "2026-01-31T12:00:00Z",
				MaxUses:    5,
			},
		}
		s := newTestServer(mock)

		body := `{"expiry": "48h", "max_uses": 5}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/invite/create", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		s.handleAPIInviteCreate(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}
	})

	t.Run("rpc error", func(t *testing.T) {
		mock := &mockRPCClient{
			inviteCreateErr: errors.New("invite creation failed"),
		}
		s := newTestServer(mock)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/invite/create", nil)

		s.handleAPIInviteCreate(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})
}

func TestHandleAPIInviteAccept(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &mockRPCClient{
			inviteAcceptResult: &rpc.InviteAcceptResult{
				NetworkID:  "network-123",
				PeerNodeID: "peer-456",
				TunnelIP:   "10.0.0.5",
				Message:    "Successfully joined network",
			},
		}
		s := newTestServer(mock)

		body := `{"invite_code": "i2plan://abc123"}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/invite/accept", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		s.handleAPIInviteAccept(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var result rpc.InviteAcceptResult
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result.TunnelIP != "10.0.0.5" {
			t.Errorf("TunnelIP = %q, want %q", result.TunnelIP, "10.0.0.5")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		mock := &mockRPCClient{}
		s := newTestServer(mock)

		body := `{invalid json}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/invite/accept", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		s.handleAPIInviteAccept(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("missing invite code", func(t *testing.T) {
		mock := &mockRPCClient{}
		s := newTestServer(mock)

		body := `{}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/invite/accept", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		s.handleAPIInviteAccept(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}

		var result map[string]string
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result["error"] != "invite_code is required" {
			t.Errorf("error = %q, want %q", result["error"], "invite_code is required")
		}
	})

	t.Run("rpc error", func(t *testing.T) {
		mock := &mockRPCClient{
			inviteAcceptErr: errors.New("invalid invite"),
		}
		s := newTestServer(mock)

		body := `{"invite_code": "i2plan://invalid"}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/invite/accept", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		s.handleAPIInviteAccept(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})
}

func TestHandleAPIInviteQR(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &mockRPCClient{}
		s := newTestServer(mock)

		body := `{"invite_code": "i2plan://test-invite-code-123"}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/invite/qr", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		s.handleAPIInviteQR(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}

		// Verify Content-Type header
		contentType := w.Header().Get("Content-Type")
		if contentType != "image/png" {
			t.Errorf("Content-Type = %q, want %q", contentType, "image/png")
		}

		// Verify Cache-Control header
		cacheControl := w.Header().Get("Cache-Control")
		if cacheControl != "no-cache, no-store, must-revalidate" {
			t.Errorf("Cache-Control = %q, want %q", cacheControl, "no-cache, no-store, must-revalidate")
		}

		// Verify PNG content (check PNG magic bytes)
		responseBody := w.Body.Bytes()
		if len(responseBody) < 8 {
			t.Fatalf("body too short: %d bytes", len(responseBody))
		}
		pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		for i := 0; i < 8; i++ {
			if responseBody[i] != pngHeader[i] {
				t.Errorf("PNG header byte %d = 0x%02X, want 0x%02X", i, responseBody[i], pngHeader[i])
			}
		}

		// Verify image is not empty (should be at least a few hundred bytes for a 256x256 QR code)
		if len(responseBody) < 400 {
			t.Errorf("PNG too small: %d bytes, expected >400", len(responseBody))
		}
	})

	t.Run("empty invite code", func(t *testing.T) {
		mock := &mockRPCClient{}
		s := newTestServer(mock)

		body := `{"invite_code": ""}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/invite/qr", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		s.handleAPIInviteQR(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}

		var result map[string]string
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result["error"] != "invite_code is required" {
			t.Errorf("error = %q, want %q", result["error"], "invite_code is required")
		}
	})

	t.Run("missing invite code field", func(t *testing.T) {
		mock := &mockRPCClient{}
		s := newTestServer(mock)

		body := `{}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/invite/qr", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		s.handleAPIInviteQR(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}

		var result map[string]string
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result["error"] != "invite_code is required" {
			t.Errorf("error = %q, want %q", result["error"], "invite_code is required")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		mock := &mockRPCClient{}
		s := newTestServer(mock)

		body := `{invalid json`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/invite/qr", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		s.handleAPIInviteQR(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}

		var result map[string]string
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result["error"] != "invalid request body" {
			t.Errorf("error = %q, want %q", result["error"], "invalid request body")
		}
	})

	t.Run("QR code with special characters", func(t *testing.T) {
		mock := &mockRPCClient{}
		s := newTestServer(mock)

		// Test with various special characters to ensure QR library handles them
		body := `{"invite_code": "i2plan://test-code!@#$%^&*()_+-=[]{}|;:',.<>?/~"}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/invite/qr", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		s.handleAPIInviteQR(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}

		// Verify valid PNG was generated
		responseBody := w.Body.Bytes()
		if len(responseBody) < 8 {
			t.Fatalf("body too short: %d bytes", len(responseBody))
		}
		pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		for i := 0; i < 8; i++ {
			if responseBody[i] != pngHeader[i] {
				t.Errorf("PNG header byte %d = 0x%02X, want 0x%02X", i, responseBody[i], pngHeader[i])
			}
		}
	})

	t.Run("QR code with long invite code", func(t *testing.T) {
		mock := &mockRPCClient{}
		s := newTestServer(mock)

		// Test with a long invite code to ensure QR library handles it
		longCode := "i2plan://" + strings.Repeat("abcdefghijklmnopqrstuvwxyz0123456789", 10)
		body := `{"invite_code": "` + longCode + `"}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/invite/qr", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		s.handleAPIInviteQR(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}

		// Verify valid PNG was generated (long codes may create larger QR codes)
		responseBody := w.Body.Bytes()
		if len(responseBody) < 8 {
			t.Fatalf("body too short: %d bytes", len(responseBody))
		}
		pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		for i := 0; i < 8; i++ {
			if responseBody[i] != pngHeader[i] {
				t.Errorf("PNG header byte %d = 0x%02X, want 0x%02X", i, responseBody[i], pngHeader[i])
			}
		}
	})
}

func TestHandleAPIHealth(t *testing.T) {
	t.Run("healthy with peers", func(t *testing.T) {
		mock := &mockRPCClient{
			statusResult: &rpc.StatusResult{
				NodeID: "node-123",
			},
			peersResult: &rpc.PeersListResult{
				Peers: []rpc.PeerInfo{
					{NodeID: "peer-1", State: "Connected"},
				},
				Total: 1,
			},
		}
		s := newTestServer(mock)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/health", nil)

		s.handleAPIHealth(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var result HealthResponse
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result.Status != "healthy" {
			t.Errorf("Status = %q, want %q", result.Status, "healthy")
		}
		if result.Checks["rpc"] != "healthy" {
			t.Errorf("Checks[rpc] = %q, want %q", result.Checks["rpc"], "healthy")
		}
		if result.Checks["peers"] != "connected" {
			t.Errorf("Checks[peers] = %q, want %q", result.Checks["peers"], "connected")
		}
	})

	t.Run("healthy with no peers", func(t *testing.T) {
		mock := &mockRPCClient{
			statusResult: &rpc.StatusResult{
				NodeID: "node-123",
			},
			peersResult: &rpc.PeersListResult{
				Peers: []rpc.PeerInfo{},
				Total: 0,
			},
		}
		s := newTestServer(mock)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/health", nil)

		s.handleAPIHealth(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var result HealthResponse
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result.Checks["peers"] != "no_peers" {
			t.Errorf("Checks[peers] = %q, want %q", result.Checks["peers"], "no_peers")
		}
	})

	t.Run("unhealthy on rpc error", func(t *testing.T) {
		mock := &mockRPCClient{
			statusErr: errors.New("rpc down"),
			peersErr:  errors.New("rpc down"), // peer check also fails when RPC is down
		}
		s := newTestServer(mock)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/health", nil)

		s.handleAPIHealth(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
		}

		var result HealthResponse
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result.Status != "unhealthy" {
			t.Errorf("Status = %q, want %q", result.Status, "unhealthy")
		}
	})

	t.Run("peers disconnected", func(t *testing.T) {
		mock := &mockRPCClient{
			statusResult: &rpc.StatusResult{
				NodeID: "node-123",
			},
			peersResult: &rpc.PeersListResult{
				Peers: []rpc.PeerInfo{
					{NodeID: "peer-1", State: "Disconnected"},
				},
				Total: 1,
			},
		}
		s := newTestServer(mock)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/health", nil)

		s.handleAPIHealth(w, r)

		var result HealthResponse
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result.Checks["peers"] != "disconnected" {
			t.Errorf("Checks[peers] = %q, want %q", result.Checks["peers"], "disconnected")
		}
	})

	t.Run("peers error is not fatal", func(t *testing.T) {
		mock := &mockRPCClient{
			statusResult: &rpc.StatusResult{
				NodeID: "node-123",
			},
			peersErr: errors.New("peers unavailable"),
		}
		s := newTestServer(mock)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/health", nil)

		s.handleAPIHealth(w, r)

		// RPC health is OK, so overall should be OK even if peers check fails
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var result HealthResponse
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result.Checks["peers"] != "unknown" {
			t.Errorf("Checks[peers] = %q, want %q", result.Checks["peers"], "unknown")
		}
	})
}

func TestHandleAPIReadiness(t *testing.T) {
	t.Run("ready", func(t *testing.T) {
		mock := &mockRPCClient{
			statusResult: &rpc.StatusResult{
				NodeID: "node-123",
			},
		}
		s := newTestServer(mock)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/readyz", nil)

		s.handleAPIReadiness(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var result map[string]string
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result["status"] != "ready" {
			t.Errorf("status = %q, want %q", result["status"], "ready")
		}
	})

	t.Run("not ready on rpc error", func(t *testing.T) {
		mock := &mockRPCClient{
			statusErr: errors.New("not connected"),
		}
		s := newTestServer(mock)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/readyz", nil)

		s.handleAPIReadiness(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
		}

		var result map[string]string
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result["status"] != "not_ready" {
			t.Errorf("status = %q, want %q", result["status"], "not_ready")
		}
		if result["reason"] != "rpc_unavailable" {
			t.Errorf("reason = %q, want %q", result["reason"], "rpc_unavailable")
		}
	})
}

func TestCheckRPCHealth(t *testing.T) {
	t.Run("healthy with identity", func(t *testing.T) {
		mock := &mockRPCClient{
			statusResult: &rpc.StatusResult{
				NodeID: "node-123",
			},
		}
		s := newTestServer(mock)

		checks := make(map[string]string)
		overallStatus := "healthy"

		s.checkRPCHealth(context.Background(), checks, &overallStatus)

		if checks["rpc"] != "healthy" {
			t.Errorf("checks[rpc] = %q, want %q", checks["rpc"], "healthy")
		}
		if checks["node"] != "healthy" {
			t.Errorf("checks[node] = %q, want %q", checks["node"], "healthy")
		}
		if checks["identity"] != "configured" {
			t.Errorf("checks[identity] = %q, want %q", checks["identity"], "configured")
		}
		if overallStatus != "healthy" {
			t.Errorf("overallStatus = %q, want %q", overallStatus, "healthy")
		}
	})

	t.Run("unhealthy on error", func(t *testing.T) {
		mock := &mockRPCClient{
			statusErr: errors.New("connection lost"),
		}
		s := newTestServer(mock)

		checks := make(map[string]string)
		overallStatus := "healthy"

		s.checkRPCHealth(context.Background(), checks, &overallStatus)

		if !strings.HasPrefix(checks["rpc"], "unhealthy:") {
			t.Errorf("checks[rpc] = %q, want prefix 'unhealthy:'", checks["rpc"])
		}
		if overallStatus != "unhealthy" {
			t.Errorf("overallStatus = %q, want %q", overallStatus, "unhealthy")
		}
	})
}

func TestCheckPeerHealth(t *testing.T) {
	t.Run("connected", func(t *testing.T) {
		mock := &mockRPCClient{
			peersResult: &rpc.PeersListResult{
				Peers: []rpc.PeerInfo{
					{NodeID: "peer-1", State: "Connected"},
				},
				Total: 1,
			},
		}
		s := newTestServer(mock)

		checks := make(map[string]string)
		s.checkPeerHealth(context.Background(), checks)

		if checks["peers"] != "connected" {
			t.Errorf("checks[peers] = %q, want %q", checks["peers"], "connected")
		}
	})

	t.Run("no peers", func(t *testing.T) {
		mock := &mockRPCClient{
			peersResult: &rpc.PeersListResult{
				Peers: []rpc.PeerInfo{},
				Total: 0,
			},
		}
		s := newTestServer(mock)

		checks := make(map[string]string)
		s.checkPeerHealth(context.Background(), checks)

		if checks["peers"] != "no_peers" {
			t.Errorf("checks[peers] = %q, want %q", checks["peers"], "no_peers")
		}
	})

	t.Run("all disconnected", func(t *testing.T) {
		mock := &mockRPCClient{
			peersResult: &rpc.PeersListResult{
				Peers: []rpc.PeerInfo{
					{NodeID: "peer-1", State: "Disconnected"},
					{NodeID: "peer-2", State: "Connecting"},
				},
				Total: 2,
			},
		}
		s := newTestServer(mock)

		checks := make(map[string]string)
		s.checkPeerHealth(context.Background(), checks)

		if checks["peers"] != "disconnected" {
			t.Errorf("checks[peers] = %q, want %q", checks["peers"], "disconnected")
		}
	})

	t.Run("error", func(t *testing.T) {
		mock := &mockRPCClient{
			peersErr: errors.New("peers unavailable"),
		}
		s := newTestServer(mock)

		checks := make(map[string]string)
		s.checkPeerHealth(context.Background(), checks)

		if checks["peers"] != "unknown" {
			t.Errorf("checks[peers] = %q, want %q", checks["peers"], "unknown")
		}
	})
}
