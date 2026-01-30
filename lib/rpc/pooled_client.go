package rpc

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/go-i2p/wireguard/lib/metrics"
	"github.com/go-i2p/wireguard/lib/pool"
)

// pooledConn wraps a net.Conn to implement pool.Connection.
type pooledConn struct {
	conn   net.Conn
	reader *bufio.Reader
}

func (p *pooledConn) Close() error {
	return p.conn.Close()
}

// PooledClient is an RPC client that uses connection pooling.
type PooledClient struct {
	pool      *pool.Pool
	cfg       ClientConfig
	authToken []byte
	requestID int
}

// PooledClientConfig extends ClientConfig with pool settings.
type PooledClientConfig struct {
	ClientConfig

	// PoolSize is the maximum number of pooled connections.
	// Default: 5
	PoolSize int
	// MaxIdleTime is how long idle connections stay in the pool.
	// Default: 5 minutes
	MaxIdleTime time.Duration
	// HealthCheckInterval is how often to check idle connections.
	// Default: 30 seconds
	HealthCheckInterval time.Duration
}

// DefaultPooledClientConfig returns a PooledClientConfig with sensible defaults.
func DefaultPooledClientConfig() PooledClientConfig {
	return PooledClientConfig{
		ClientConfig: ClientConfig{
			Timeout: 30 * time.Second,
		},
		PoolSize:            5,
		MaxIdleTime:         5 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
	}
}

// NewPooledClient creates a new pooled RPC client.
func NewPooledClient(cfg PooledClientConfig) (*PooledClient, error) {
	log.Debug("creating new pooled RPC client")

	if cfg.PoolSize <= 0 {
		cfg.PoolSize = 5
	}
	if cfg.MaxIdleTime <= 0 {
		cfg.MaxIdleTime = 5 * time.Minute
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	pc := &PooledClient{
		cfg: cfg.ClientConfig,
	}

	// Load auth token
	if err := pc.loadAuthToken(cfg.ClientConfig); err != nil {
		return nil, err
	}

	// Create pool configuration
	poolCfg := pool.Config{
		MaxSize:             cfg.PoolSize,
		MaxIdleTime:         cfg.MaxIdleTime,
		AcquireTimeout:      cfg.Timeout,
		HealthCheckInterval: cfg.HealthCheckInterval,
		HealthCheck:         pc.healthCheck,
	}

	// Create pool with factory
	pc.pool = pool.New(pc.createConnection, poolCfg)

	log.WithField("poolSize", cfg.PoolSize).Debug("pooled RPC client created")
	return pc, nil
}

// createConnection creates a new connection for the pool.
func (c *PooledClient) createConnection(ctx context.Context) (pool.Connection, error) {
	conn, err := dialConnection(c.cfg)
	if err != nil {
		return nil, err
	}

	pc := &pooledConn{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}

	// Authenticate if we have a token
	if c.authToken != nil {
		if err := c.authenticateConn(pc); err != nil {
			conn.Close()
			return nil, fmt.Errorf("authentication failed: %w", err)
		}
	}

	return pc, nil
}

// healthCheck verifies a pooled connection is still valid.
func (c *PooledClient) healthCheck(conn pool.Connection) bool {
	pc := conn.(*pooledConn)

	// Set a short deadline for the ping
	if err := pc.conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return false
	}

	// Try a status call as a ping
	req := &Request{
		JSONRPC: "2.0",
		Method:  "ping",
		ID:      json.RawMessage("0"),
	}

	data, _ := json.Marshal(req)
	data = append(data, '\n')

	if _, err := pc.conn.Write(data); err != nil {
		return false
	}

	if _, err := pc.reader.ReadBytes('\n'); err != nil {
		return false
	}

	// Reset deadline
	pc.conn.SetDeadline(time.Time{})
	return true
}

// loadAuthToken loads the authentication token from config or file.
func (c *PooledClient) loadAuthToken(cfg ClientConfig) error {
	if cfg.AuthToken != "" {
		token, err := hex.DecodeString(cfg.AuthToken)
		if err != nil {
			return fmt.Errorf("invalid auth token: %w", err)
		}
		c.authToken = token
		return nil
	}

	if cfg.AuthFile != "" {
		data, err := os.ReadFile(cfg.AuthFile)
		if err != nil {
			return fmt.Errorf("reading auth file: %w", err)
		}
		token, err := hex.DecodeString(string(data))
		if err != nil {
			return fmt.Errorf("invalid auth token in file: %w", err)
		}
		c.authToken = token
	}

	return nil
}

// authenticateConn authenticates a connection.
func (c *PooledClient) authenticateConn(pc *pooledConn) error {
	c.requestID++
	req := &Request{
		JSONRPC: "2.0",
		Method:  "auth",
		ID:      json.RawMessage(fmt.Sprintf("%d", c.requestID)),
	}

	params := map[string]string{
		"token": hex.EncodeToString(c.authToken),
	}
	data, _ := json.Marshal(params)
	req.Params = data

	if err := c.sendRequestTo(pc, req); err != nil {
		return err
	}

	resp, err := c.readResponseFrom(pc)
	if err != nil {
		return err
	}

	if resp.Error != nil {
		return resp.Error
	}

	return nil
}

// Call makes an RPC call using a pooled connection.
func (c *PooledClient) Call(ctx context.Context, method string, params, result any) error {
	timer := metrics.NewTimer(pool.PoolAcquireLatency)
	conn, err := c.pool.Acquire(ctx)
	timer.ObserveDuration()

	if err != nil {
		log.WithError(err).WithField("method", method).Error("failed to acquire connection from pool")
		return fmt.Errorf("acquire connection: %w", err)
	}

	pc := conn.(*pooledConn)
	success := false
	defer func() {
		if success {
			c.pool.Release(conn)
		} else {
			c.pool.Discard(conn)
		}
	}()

	c.requestID++
	req, err := c.buildRequest(method, params)
	if err != nil {
		return err
	}

	if err := c.sendRequestTo(pc, req); err != nil {
		return err
	}

	resp, err := c.readResponseFrom(pc)
	if err != nil {
		return err
	}

	if resp.Error != nil {
		// Connection is still valid even on RPC errors
		success = true
		return resp.Error
	}

	success = true
	return c.unmarshalResult(resp, result)
}

// buildRequest creates a JSON-RPC request.
func (c *PooledClient) buildRequest(method string, params any) (*Request, error) {
	req := &Request{
		JSONRPC: "2.0",
		Method:  method,
		ID:      json.RawMessage(fmt.Sprintf("%d", c.requestID)),
	}

	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("marshal params: %w", err)
		}
		req.Params = data
	}

	return req, nil
}

// sendRequestTo sends a request on a specific connection.
func (c *PooledClient) sendRequestTo(pc *pooledConn, req *Request) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	data = append(data, '\n')

	if err := pc.conn.SetWriteDeadline(time.Now().Add(c.cfg.Timeout)); err != nil {
		return fmt.Errorf("set write deadline: %w", err)
	}
	if _, err := pc.conn.Write(data); err != nil {
		return fmt.Errorf("write request: %w", err)
	}

	return nil
}

// readResponseFrom reads a response from a specific connection.
func (c *PooledClient) readResponseFrom(pc *pooledConn) (*Response, error) {
	if err := pc.conn.SetReadDeadline(time.Now().Add(c.cfg.Timeout)); err != nil {
		return nil, fmt.Errorf("set read deadline: %w", err)
	}
	data, err := pc.reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &resp, nil
}

// unmarshalResult unmarshals the response result.
func (c *PooledClient) unmarshalResult(resp *Response, result any) error {
	if result == nil || resp.Result == nil {
		return nil
	}

	data, err := json.Marshal(resp.Result)
	if err != nil {
		return fmt.Errorf("re-marshal result: %w", err)
	}

	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("unmarshal result: %w", err)
	}

	return nil
}

// Stats returns pool statistics.
func (c *PooledClient) Stats() pool.Stats {
	return c.pool.Stats()
}

// Close closes the pooled client and all connections.
func (c *PooledClient) Close() error {
	log.Debug("closing pooled RPC client")
	return c.pool.Close()
}

// Status calls the "status" method.
func (c *PooledClient) Status(ctx context.Context) (*StatusResult, error) {
	var result StatusResult
	if err := c.Call(ctx, "status", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PeersList calls the "peers.list" method.
func (c *PooledClient) PeersList(ctx context.Context) (*PeersListResult, error) {
	var result PeersListResult
	if err := c.Call(ctx, "peers.list", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// InviteCreate calls the "invite.create" method.
func (c *PooledClient) InviteCreate(ctx context.Context, expiry string, maxUses int) (*InviteCreateResult, error) {
	params := InviteCreateParams{
		Expiry:  expiry,
		MaxUses: maxUses,
	}
	var result InviteCreateResult
	if err := c.Call(ctx, "invite.create", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// InviteAccept calls the "invite.accept" method.
func (c *PooledClient) InviteAccept(ctx context.Context, inviteCode string) (*InviteAcceptResult, error) {
	params := InviteAcceptParams{InviteCode: inviteCode}
	var result InviteAcceptResult
	if err := c.Call(ctx, "invite.accept", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RoutesList calls the "routes.list" method.
func (c *PooledClient) RoutesList(ctx context.Context) (*RoutesListResult, error) {
	var result RoutesListResult
	if err := c.Call(ctx, "routes.list", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// BansList calls the "bans.list" method.
func (c *PooledClient) BansList(ctx context.Context) (*BanListResult, error) {
	var result BanListResult
	if err := c.Call(ctx, "bans.list", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
