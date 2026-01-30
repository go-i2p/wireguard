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
	handlers     map[string]Handler
	unixListener net.Listener
	tcpListener  net.Listener
	authToken    []byte // shared secret for authentication
	connLimiter  *ConnectionLimiter
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
	// MaxConnections is the maximum concurrent connections (0 = default of 100).
	MaxConnections int
}

// NewServer creates a new RPC server.
func NewServer(cfg ServerConfig) (*Server, error) {
	s := &Server{
		handlers:    make(map[string]Handler),
		connLimiter: NewConnectionLimiter(cfg.MaxConnections),
	}

	// Set up connection limit rejection logging
	s.connLimiter.SetOnReject(func(addr net.Addr) {
		log.Warn("connection rejected: too many connections",
			"remote", addr.String(),
			"active", s.connLimiter.ActiveConnections(),
			"max", s.connLimiter.MaxConnections(),
		)
	})

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
			log.Debug("loaded existing auth token", "path", path)
			return token, nil
		}
		// Invalid token file, regenerate
		log.Warn("invalid auth token file, regenerating", "path", path)
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

	log.Info("generated new auth token", "path", path)
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
// startUnixListener creates and starts the Unix socket listener.
func (s *Server) startUnixListener(ctx context.Context, socketPath string) error {
	os.Remove(socketPath)

	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		return fmt.Errorf("creating socket dir: %w", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen unix: %w", err)
	}

	if err := os.Chmod(socketPath, 0o600); err != nil {
		listener.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}

	s.unixListener = listener
	s.wg.Add(1)
	go s.acceptLoop(ctx, listener, "unix")

	log.Info("RPC server listening on Unix socket", "path", socketPath)
	return nil
}

// startTCPListener creates and starts the TCP listener.
func (s *Server) startTCPListener(ctx context.Context, address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen tcp: %w", err)
	}

	s.tcpListener = listener
	s.wg.Add(1)
	go s.acceptLoop(ctx, listener, "tcp")

	log.Info("RPC server listening on TCP", "address", address)
	return nil
}

func (s *Server) Start(ctx context.Context, cfg ServerConfig) error {
	if err := s.validateAndSetRunning(); err != nil {
		return err
	}

	if err := s.startListeners(ctx, cfg); err != nil {
		return err
	}

	return s.validateListenersConfigured()
}

// validateAndSetRunning checks if the server is already running and sets the running flag.
func (s *Server) validateAndSetRunning() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return errors.New("server already running")
	}
	s.running = true
	return nil
}

// startListeners starts Unix and TCP listeners based on configuration.
func (s *Server) startListeners(ctx context.Context, cfg ServerConfig) error {
	if cfg.UnixSocketPath != "" {
		if err := s.startUnixListener(ctx, cfg.UnixSocketPath); err != nil {
			return err
		}
	}

	if cfg.TCPAddress != "" {
		if err := s.startTCPListener(ctx, cfg.TCPAddress); err != nil {
			if s.unixListener != nil {
				s.unixListener.Close()
			}
			return err
		}
	}
	return nil
}

// validateListenersConfigured ensures at least one listener is configured.
func (s *Server) validateListenersConfigured() error {
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
			if s.handleAcceptError(ctx, network, err) {
				return
			}
			continue
		}

		s.spawnConnectionHandler(ctx, conn, network)
	}
}

// handleAcceptError processes accept errors and returns true if the loop should exit.
func (s *Server) handleAcceptError(ctx context.Context, network string, err error) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		if !errors.Is(err, net.ErrClosed) {
			log.Error("accept error", "network", network, "error", err)
		}
		return true
	}
}

// spawnConnectionHandler starts a goroutine to handle a connection.
func (s *Server) spawnConnectionHandler(ctx context.Context, conn net.Conn, network string) {
	// Check connection limit
	conn = s.connLimiter.TryAccept(conn)
	if conn == nil {
		// Connection was rejected due to limit
		return
	}

	// Wrap connection to auto-release slot on close
	limitedConn := s.connLimiter.WrapConn(conn)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.handleConnection(ctx, limitedConn, network)
	}()
}

// handleConnection handles a single client connection.
func (s *Server) handleConnection(ctx context.Context, conn net.Conn, network string) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	log.Debug("new connection", "network", network, "remote", remoteAddr)

	reader := bufio.NewReaderSize(conn, 64*1024)
	authenticated := s.authToken == nil

	s.processRequests(ctx, conn, reader, remoteAddr, network, &authenticated)
}

// processRequests continuously reads and handles requests from a connection.
func (s *Server) processRequests(ctx context.Context, conn net.Conn, reader *bufio.Reader, remoteAddr, network string, authenticated *bool) {
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

		s.handleRequest(ctx, conn, req, network, authenticated)
	}
}

// handleRequest processes a single RPC request and sends the appropriate response.
func (s *Server) handleRequest(ctx context.Context, conn net.Conn, req *Request, network string, authenticated *bool) {
	if req.Method == "auth" {
		resp := s.handleAuth(req, authenticated)
		s.sendResponse(conn, resp)
		return
	}

	if !*authenticated && network == "tcp" {
		s.sendResponse(conn, NewErrorResponse(req.ID, ErrAuthRequired()))
		return
	}

	resp := s.dispatch(ctx, req)
	s.sendResponse(conn, resp)
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
	if err := conn.SetReadDeadline(time.Now().Add(ReadTimeout)); err != nil {
		log.Warn("failed to set read deadline", "remote", remoteAddr, "error", err)
	}

	line, err := reader.ReadBytes('\n')
	if err != nil {
		if err != io.EOF && !errors.Is(err, net.ErrClosed) {
			log.Debug("read error", "remote", remoteAddr, "error", err)
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
	if err := conn.SetWriteDeadline(time.Now().Add(WriteTimeout)); err != nil {
		log.Warn("failed to set write deadline", "error", err)
	}

	data, err := json.Marshal(resp)
	if err != nil {
		log.Error("marshal response", "error", err)
		return
	}

	data = append(data, '\n')
	if _, err := conn.Write(data); err != nil {
		log.Debug("write error", "error", err)
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

	log.Info("RPC server stopped")
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

// ActiveConnections returns the current number of active connections.
func (s *Server) ActiveConnections() int {
	return s.connLimiter.ActiveConnections()
}

// MaxConnections returns the maximum allowed connections.
func (s *Server) MaxConnections() int {
	return s.connLimiter.MaxConnections()
}

// SetMaxConnections updates the maximum connection limit at runtime.
func (s *Server) SetMaxConnections(max int) {
	s.connLimiter.SetMaxConnections(max)
}
