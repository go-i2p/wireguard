package rpc

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// AuthTokenLength is the length of auth tokens in bytes.
	AuthTokenLength = 32

	// MaxRequestSize is the maximum size of a request in bytes (1MB).
	MaxRequestSize = 1024 * 1024

	// ConnectionTimeout is the default connection timeout.
	ConnectionTimeout = 30 * time.Second

	// ReadTimeout is the timeout for reading requests.
	ReadTimeout = 10 * time.Second

	// WriteTimeout is the timeout for writing responses.
	WriteTimeout = 10 * time.Second
)

// Handler is a function that handles an RPC request.
type Handler func(ctx context.Context, params json.RawMessage) (any, *Error)

// Server is an RPC server that listens on Unix socket and/or TCP.
type Server struct {
	mu           sync.RWMutex
	logger       *slog.Logger
	handlers     map[string]Handler
	unixListener net.Listener
	tcpListener  net.Listener
	authToken    []byte // shared secret for authentication
	running      bool
	wg           sync.WaitGroup
}

// ServerConfig configures the RPC server.
type ServerConfig struct {
	// UnixSocketPath is the path to the Unix socket (required for Unix socket mode).
	UnixSocketPath string
	// TCPAddress is the TCP address to listen on (optional).
	TCPAddress string
	// AuthFile is the path to the auth token file.
	AuthFile string
	// Logger is the logger to use.
	Logger *slog.Logger
}

// NewServer creates a new RPC server.
func NewServer(cfg ServerConfig) (*Server, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	s := &Server{
		logger:   cfg.Logger.With("component", "rpc-server"),
		handlers: make(map[string]Handler),
	}

	// Load or generate auth token
	if cfg.AuthFile != "" {
		token, err := s.loadOrCreateAuthToken(cfg.AuthFile)
		if err != nil {
			return nil, fmt.Errorf("auth token: %w", err)
		}
		s.authToken = token
	}

	return s, nil
}

// loadOrCreateAuthToken loads an existing auth token or creates a new one.
func (s *Server) loadOrCreateAuthToken(path string) ([]byte, error) {
	// Try to load existing token
	data, err := os.ReadFile(path)
	if err == nil {
		// Parse hex-encoded token
		token, err := hex.DecodeString(string(data))
		if err == nil && len(token) == AuthTokenLength {
			s.logger.Debug("loaded existing auth token", "path", path)
			return token, nil
		}
		// Invalid token file, regenerate
		s.logger.Warn("invalid auth token file, regenerating", "path", path)
	}

	// Generate new token
	token := make([]byte, AuthTokenLength)
	if _, err := rand.Read(token); err != nil {
		return nil, fmt.Errorf("generating token: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("creating auth dir: %w", err)
	}

	// Write token (hex-encoded)
	if err := os.WriteFile(path, []byte(hex.EncodeToString(token)), 0o600); err != nil {
		return nil, fmt.Errorf("writing token: %w", err)
	}

	s.logger.Info("generated new auth token", "path", path)
	return token, nil
}

// RegisterHandler registers a handler for an RPC method.
func (s *Server) RegisterHandler(method string, handler Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[method] = handler
}

// RegisterHandlers registers multiple handlers at once.
func (s *Server) RegisterHandlers(handlers map[string]Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for method, handler := range handlers {
		s.handlers[method] = handler
	}
}

// Start starts the RPC server.
func (s *Server) Start(ctx context.Context, cfg ServerConfig) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("server already running")
	}
	s.running = true
	s.mu.Unlock()

	// Start Unix socket listener
	if cfg.UnixSocketPath != "" {
		// Remove existing socket file
		os.Remove(cfg.UnixSocketPath)

		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(cfg.UnixSocketPath), 0o700); err != nil {
			return fmt.Errorf("creating socket dir: %w", err)
		}

		listener, err := net.Listen("unix", cfg.UnixSocketPath)
		if err != nil {
			return fmt.Errorf("listen unix: %w", err)
		}

		// Set socket permissions (owner only)
		if err := os.Chmod(cfg.UnixSocketPath, 0o600); err != nil {
			listener.Close()
			return fmt.Errorf("chmod socket: %w", err)
		}

		s.unixListener = listener
		s.wg.Add(1)
		go s.acceptLoop(ctx, listener, "unix")

		s.logger.Info("RPC server listening on Unix socket", "path", cfg.UnixSocketPath)
	}

	// Start TCP listener
	if cfg.TCPAddress != "" {
		listener, err := net.Listen("tcp", cfg.TCPAddress)
		if err != nil {
			if s.unixListener != nil {
				s.unixListener.Close()
			}
			return fmt.Errorf("listen tcp: %w", err)
		}

		s.tcpListener = listener
		s.wg.Add(1)
		go s.acceptLoop(ctx, listener, "tcp")

		s.logger.Info("RPC server listening on TCP", "address", cfg.TCPAddress)
	}

	if s.unixListener == nil && s.tcpListener == nil {
		return errors.New("no listeners configured")
	}

	return nil
}

// acceptLoop accepts connections and handles them.
func (s *Server) acceptLoop(ctx context.Context, listener net.Listener, network string) {
	defer s.wg.Done()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				if !errors.Is(err, net.ErrClosed) {
					s.logger.Error("accept error", "network", network, "error", err)
				}
				return
			}
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConnection(ctx, conn, network)
		}()
	}
}

// handleConnection handles a single client connection.
func (s *Server) handleConnection(ctx context.Context, conn net.Conn, network string) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	s.logger.Debug("new connection", "network", network, "remote", remoteAddr)

	reader := bufio.NewReaderSize(conn, 64*1024)
	authenticated := s.authToken == nil

	for {
		if s.isContextCancelled(ctx) {
			return
		}

		req, err := s.readRequest(conn, reader, remoteAddr)
		if err != nil {
			return
		}
		if req == nil {
			continue
		}

		if req.Method == "auth" {
			resp := s.handleAuth(req, &authenticated)
			s.sendResponse(conn, resp)
			continue
		}

		if !authenticated && network == "tcp" {
			s.sendResponse(conn, NewErrorResponse(req.ID, ErrAuthRequired()))
			continue
		}

		resp := s.dispatch(ctx, req)
		s.sendResponse(conn, resp)
	}
}

// isContextCancelled checks if the context has been cancelled.
func (s *Server) isContextCancelled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// readRequest reads and parses a JSON-RPC request from the connection.
func (s *Server) readRequest(conn net.Conn, reader *bufio.Reader, remoteAddr string) (*Request, error) {
	conn.SetReadDeadline(time.Now().Add(ReadTimeout))

	line, err := reader.ReadBytes('\n')
	if err != nil {
		if err != io.EOF && !errors.Is(err, net.ErrClosed) {
			s.logger.Debug("read error", "remote", remoteAddr, "error", err)
		}
		return nil, err
	}

	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		s.sendResponse(conn, NewErrorResponse(nil, NewError(ErrCodeParse, "parse error", err.Error())))
		return nil, nil
	}

	if err := ValidateRequest(&req); err != nil {
		s.sendResponse(conn, NewErrorResponse(req.ID, NewError(ErrCodeInvalidRequest, "invalid request", err.Error())))
		return nil, nil
	}

	return &req, nil
}

// handleAuth handles the "auth" method.
func (s *Server) handleAuth(req *Request, authenticated *bool) *Response {
	if s.authToken == nil {
		*authenticated = true
		return NewSuccessResponse(req.ID, map[string]string{"message": "authentication not required"})
	}

	var params struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return NewErrorResponse(req.ID, ErrInvalidParams("token required"))
	}

	tokenBytes, err := hex.DecodeString(params.Token)
	if err != nil {
		return NewErrorResponse(req.ID, ErrInvalidParams("invalid token format"))
	}

	if subtle.ConstantTimeCompare(tokenBytes, s.authToken) != 1 {
		return NewErrorResponse(req.ID, ErrPermissionDenied("invalid token"))
	}

	*authenticated = true
	return NewSuccessResponse(req.ID, map[string]string{"message": "authenticated"})
}

// dispatch dispatches a request to the appropriate handler.
func (s *Server) dispatch(ctx context.Context, req *Request) *Response {
	s.mu.RLock()
	handler, ok := s.handlers[req.Method]
	s.mu.RUnlock()

	if !ok {
		return NewErrorResponse(req.ID, ErrMethodNotFound(req.Method))
	}

	// Call handler with timeout
	handlerCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	result, err := handler(handlerCtx, req.Params)
	if err != nil {
		return NewErrorResponse(req.ID, err)
	}

	return NewSuccessResponse(req.ID, result)
}

// sendResponse sends a response to the client.
func (s *Server) sendResponse(conn net.Conn, resp *Response) {
	conn.SetWriteDeadline(time.Now().Add(WriteTimeout))

	data, err := json.Marshal(resp)
	if err != nil {
		s.logger.Error("marshal response", "error", err)
		return
	}

	data = append(data, '\n')
	if _, err := conn.Write(data); err != nil {
		s.logger.Debug("write error", "error", err)
	}
}

// Stop stops the RPC server.
func (s *Server) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	// Close listeners
	if s.unixListener != nil {
		s.unixListener.Close()
	}
	if s.tcpListener != nil {
		s.tcpListener.Close()
	}

	// Wait for all handlers to finish
	s.wg.Wait()

	s.logger.Info("RPC server stopped")
	return nil
}

// IsRunning returns whether the server is running.
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// AuthToken returns the auth token (for display to user).
func (s *Server) AuthToken() string {
	if s.authToken == nil {
		return ""
	}
	return hex.EncodeToString(s.authToken)
}

// UnixSocketPath returns the Unix socket path if listening.
func (s *Server) UnixSocketPath() string {
	if s.unixListener != nil {
		return s.unixListener.Addr().String()
	}
	return ""
}

// TCPAddress returns the TCP address if listening.
func (s *Server) TCPAddress() string {
	if s.tcpListener != nil {
		return s.tcpListener.Addr().String()
	}
	return ""
}
