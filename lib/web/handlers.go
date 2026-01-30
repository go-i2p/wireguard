package web

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// handleDashboard renders the main dashboard page.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status, err := s.rpcClient.Status(ctx)
	if err != nil {
		log.Error("rpc status error", "error", err)
		s.renderTemplate(w, "dashboard", map[string]any{
			"Error": "Failed to connect to node",
		})
		return
	}

	peers, _ := s.rpcClient.PeersList(ctx)
	routes, _ := s.rpcClient.RoutesList(ctx)

	peerCount := 0
	if peers != nil {
		peerCount = peers.Total
	}
	routeCount := 0
	if routes != nil {
		routeCount = routes.Total
	}

	s.renderTemplate(w, "dashboard", map[string]any{
		"Status":     status,
		"PeerCount":  peerCount,
		"RouteCount": routeCount,
	})
}

// handlePeers renders the peers page.
func (s *Server) handlePeers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	peers, err := s.rpcClient.PeersList(ctx)
	if err != nil {
		log.Error("rpc peers error", "error", err)
		s.renderTemplate(w, "peers", map[string]any{
			"Error": "Failed to fetch peers",
		})
		return
	}

	s.renderTemplate(w, "peers", map[string]any{
		"Peers": peers.Peers,
		"Total": peers.Total,
	})
}

// handleInvites renders the invites page.
func (s *Server) handleInvites(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "invites", nil)
}

// handleRoutes renders the routes page.
func (s *Server) handleRoutes(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	routes, err := s.rpcClient.RoutesList(ctx)
	if err != nil {
		log.Error("rpc routes error", "error", err)
		s.renderTemplate(w, "routes", map[string]any{
			"Error": "Failed to fetch routes",
		})
		return
	}

	s.renderTemplate(w, "routes", map[string]any{
		"Routes": routes.Routes,
		"Total":  routes.Total,
	})
}

// handleSettings renders the settings page.
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	config, err := s.rpcClient.ConfigGet(ctx, "")
	if err != nil {
		log.Error("rpc config error", "error", err)
		s.renderTemplate(w, "settings", map[string]any{
			"Error": "Failed to fetch configuration",
		})
		return
	}

	s.renderTemplate(w, "settings", map[string]any{
		"Config": config.Value,
	})
}

// API Handlers

// handleAPICSRFToken generates and returns a new CSRF token.
// This token should be included in the X-CSRF-Token header for POST/PUT/DELETE requests.
func (s *Server) handleAPICSRFToken(w http.ResponseWriter, r *http.Request) {
	token, err := s.csrfManager.GenerateToken()
	if err != nil {
		log.Error("failed to generate CSRF token", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	// Set the cookie for the token
	SetCSRFCookie(w, token)

	s.writeJSON(w, http.StatusOK, map[string]string{
		"token": token,
	})
}

// handleAPIStatus returns node status as JSON.
func (s *Server) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status, err := s.rpcClient.Status(ctx)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, status)
}

// handleAPIPeers returns peers list as JSON.
func (s *Server) handleAPIPeers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	peers, err := s.rpcClient.PeersList(ctx)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, peers)
}

// handleAPIRoutes returns routes as JSON.
func (s *Server) handleAPIRoutes(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	routes, err := s.rpcClient.RoutesList(ctx)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, routes)
}

// InviteCreateRequest is the request body for creating an invite.
type InviteCreateRequest struct {
	Expiry  string `json:"expiry,omitempty"`
	MaxUses int    `json:"max_uses,omitempty"`
}

// handleAPIInviteCreate creates a new invite code.
func (s *Server) handleAPIInviteCreate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var req InviteCreateRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	// Default expiry
	if req.Expiry == "" {
		req.Expiry = "24h"
	}
	if req.MaxUses == 0 {
		req.MaxUses = 1
	}

	result, err := s.rpcClient.InviteCreate(ctx, req.Expiry, req.MaxUses)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, result)
}

// InviteAcceptRequest is the request body for accepting an invite.
type InviteAcceptRequest struct {
	InviteCode string `json:"invite_code"`
}

// handleAPIInviteAccept accepts an invite code.
func (s *Server) handleAPIInviteAccept(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var req InviteAcceptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.InviteCode == "" {
		s.writeError(w, http.StatusBadRequest, "invite_code is required")
		return
	}

	result, err := s.rpcClient.InviteAccept(ctx, req.InviteCode)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, result)
}

// HealthResponse contains the health check response.
type HealthResponse struct {
	Status    string            `json:"status"`
	Timestamp string            `json:"timestamp"`
	Version   string            `json:"version,omitempty"`
	Checks    map[string]string `json:"checks"`
}

// handleAPIHealth returns the health status of the node.
func (s *Server) handleAPIHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	checks := make(map[string]string)
	overallStatus := "healthy"

	s.checkRPCHealth(ctx, checks, &overallStatus)
	s.checkPeerHealth(ctx, checks)

	resp := HealthResponse{
		Status:    overallStatus,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Checks:    checks,
	}

	httpStatus := http.StatusOK
	if overallStatus != "healthy" {
		httpStatus = http.StatusServiceUnavailable
	}

	s.writeJSON(w, httpStatus, resp)
}

// checkRPCHealth verifies the RPC connection and node status.
func (s *Server) checkRPCHealth(ctx context.Context, checks map[string]string, overallStatus *string) {
	status, err := s.rpcClient.Status(ctx)
	if err != nil {
		checks["rpc"] = "unhealthy: " + err.Error()
		*overallStatus = "unhealthy"
		return
	}

	checks["rpc"] = "healthy"
	if status != nil {
		checks["node"] = "healthy"
		if status.NodeID != "" {
			checks["identity"] = "configured"
		}
	}
}

// checkPeerHealth checks peer connectivity status.
func (s *Server) checkPeerHealth(ctx context.Context, checks map[string]string) {
	peers, err := s.rpcClient.PeersList(ctx)
	if err != nil {
		checks["peers"] = "unknown"
		return
	}

	if peers.Total == 0 {
		checks["peers"] = "no_peers"
		return
	}

	connectedCount := 0
	for _, p := range peers.Peers {
		if p.State == "Connected" {
			connectedCount++
		}
	}

	if connectedCount > 0 {
		checks["peers"] = "connected"
	} else {
		checks["peers"] = "disconnected"
	}
}

// handleAPILiveness returns a simple liveness probe response.
// This is a lightweight check that the server is responding.
func (s *Server) handleAPILiveness(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{
		"status": "alive",
	})
}

// handleAPIReadiness returns a readiness probe response.
// This checks if the service is ready to handle requests.
func (s *Server) handleAPIReadiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	// Check if RPC is responding
	_, err := s.rpcClient.Status(ctx)
	if err != nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not_ready",
			"reason": "rpc_unavailable",
		})
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status": "ready",
	})
}
