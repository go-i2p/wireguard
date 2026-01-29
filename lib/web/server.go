// Package web provides a browser-based management UI for i2plan.
// It serves HTML pages for node management and JSON API endpoints
// for programmatic access.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/go-i2p/wireguard/lib/metrics"
	"github.com/go-i2p/wireguard/lib/rpc"
)

//go:embed templates/*.html static/*
var content embed.FS

// Server is the web UI HTTP server.
type Server struct {
	httpServer *http.Server
	rpcClient  *rpc.Client
	templates  *template.Template
	logger     *slog.Logger
	mu         sync.RWMutex
	running    bool
}

// Config holds web server configuration.
type Config struct {
	// ListenAddr is the address to listen on (e.g., "127.0.0.1:8080")
	ListenAddr string
	// RPCSocketPath is the path to the RPC Unix socket
	RPCSocketPath string
	// RPCAuthFile is the path to the RPC auth token file
	RPCAuthFile string
	// Logger is the structured logger
	Logger *slog.Logger
}

// New creates a new web server.
// If Start() fails, this function cleans up the RPC client automatically.
// When the server is no longer needed, call Stop() to release resources.
// The server owns the RPC client connection and will close it when Stop() is called.
func New(cfg Config) (*Server, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	// Create RPC client
	client, err := rpc.NewClient(rpc.ClientConfig{
		UnixSocketPath: cfg.RPCSocketPath,
		AuthFile:       cfg.RPCAuthFile,
	})
	if err != nil {
		return nil, fmt.Errorf("connecting to RPC: %w", err)
	}

	// Parse templates
	tmpl, err := template.New("").Funcs(templateFuncs()).ParseFS(content, "templates/*.html")
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("parsing templates: %w", err)
	}

	s := &Server{
		rpcClient: client,
		templates: tmpl,
		logger:    cfg.Logger,
	}

	// Create HTTP mux
	mux := http.NewServeMux()

	// Static files
	staticFS, err := fs.Sub(content, "static")
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("static fs: %w", err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// HTML pages
	mux.HandleFunc("GET /", s.handleDashboard)
	mux.HandleFunc("GET /peers", s.handlePeers)
	mux.HandleFunc("GET /invites", s.handleInvites)
	mux.HandleFunc("GET /routes", s.handleRoutes)
	mux.HandleFunc("GET /settings", s.handleSettings)

	// API endpoints
	mux.HandleFunc("GET /api/status", s.handleAPIStatus)
	mux.HandleFunc("GET /api/peers", s.handleAPIPeers)
	mux.HandleFunc("GET /api/routes", s.handleAPIRoutes)
	mux.HandleFunc("POST /api/invite/create", s.handleAPIInviteCreate)
	mux.HandleFunc("POST /api/invite/accept", s.handleAPIInviteAccept)

	// Health check endpoints
	mux.HandleFunc("GET /api/health", s.handleAPIHealth)
	mux.HandleFunc("GET /health", s.handleAPIHealth)
	mux.HandleFunc("GET /healthz", s.handleAPILiveness)
	mux.HandleFunc("GET /readyz", s.handleAPIReadiness)

	// Metrics endpoint (Prometheus format)
	mux.Handle("GET /metrics", metrics.Handler())

	s.httpServer = &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           s.withMiddleware(mux),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return s, nil
}

// Start starts the web server.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}
	s.running = true
	s.mu.Unlock()

	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		// Reset running state and close RPC client on failure
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		if closeErr := s.rpcClient.Close(); closeErr != nil {
			s.logger.Error("failed to close RPC client after start failure", "error", closeErr)
		}
		return fmt.Errorf("listen: %w", err)
	}

	s.logger.Info("web server started", "addr", s.httpServer.Addr)

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.logger.Error("server error", "error", err)
		}
	}()

	return nil
}

// Stop stops the web server gracefully.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	if err := s.rpcClient.Close(); err != nil {
		return fmt.Errorf("close rpc: %w", err)
	}

	s.logger.Info("web server stopped")
	return nil
}

// withMiddleware wraps the handler with common middleware.
func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Log request
		s.logger.Debug("request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
		)

		// Set common headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")

		next.ServeHTTP(w, r)

		s.logger.Debug("response",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
		)
	})
}

// templateFuncs returns custom template functions.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			if n <= 3 {
				return s[:n]
			}
			return s[:n-3] + "..."
		},
		"json": func(v any) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
	}
}

// renderTemplate renders a template with common data.
func (s *Server) renderTemplate(w http.ResponseWriter, name string, data map[string]any) {
	if data == nil {
		data = make(map[string]any)
	}

	// Add common data
	data["CurrentPage"] = name

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, name+".html", data); err != nil {
		s.logger.Error("template error", "template", name, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// writeJSON writes a JSON response.
func (s *Server) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("json encode error", "error", err)
	}
}

// writeError writes a JSON error response.
func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]string{"error": message})
}
