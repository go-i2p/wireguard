package rpc

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

// Client is an RPC client that connects to Unix socket or TCP.
type Client struct {
	conn      net.Conn
	reader    *bufio.Reader
	authToken []byte
	requestID int
	timeout   time.Duration
}

// ClientConfig configures the RPC client.
type ClientConfig struct {
	// UnixSocketPath is the path to the Unix socket.
	UnixSocketPath string
	// TCPAddress is the TCP address to connect to.
	TCPAddress string
	// AuthToken is the authentication token (hex-encoded).
	AuthToken string
	// AuthFile is the path to read the auth token from.
	AuthFile string
	// Timeout is the connection and request timeout.
	Timeout time.Duration
}

// NewClient creates a new RPC client and connects to the server.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	conn, err := dialConnection(cfg)
	if err != nil {
		return nil, err
	}

	c := &Client{
		conn:    conn,
		reader:  bufio.NewReader(conn),
		timeout: cfg.Timeout,
	}

	if err := c.loadAuthToken(cfg); err != nil {
		conn.Close()
		return nil, err
	}

	if c.authToken != nil {
		if err := c.authenticate(); err != nil {
			conn.Close()
			return nil, fmt.Errorf("authentication failed: %w", err)
		}
	}

	return c, nil
}

// dialConnection establishes a connection using Unix socket or TCP.
func dialConnection(cfg ClientConfig) (net.Conn, error) {
	if cfg.UnixSocketPath != "" {
		conn, err := net.DialTimeout("unix", cfg.UnixSocketPath, cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("connect unix: %w", err)
		}
		return conn, nil
	}

	if cfg.TCPAddress != "" {
		conn, err := net.DialTimeout("tcp", cfg.TCPAddress, cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("connect tcp: %w", err)
		}
		return conn, nil
	}

	return nil, errors.New("no connection address specified")
}

// loadAuthToken loads the authentication token from config or file.
func (c *Client) loadAuthToken(cfg ClientConfig) error {
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

// authenticate sends the auth token to the server.
func (c *Client) authenticate() error {
	params := map[string]string{
		"token": hex.EncodeToString(c.authToken),
	}

	var result map[string]string
	if err := c.Call(context.Background(), "auth", params, &result); err != nil {
		return err
	}

	return nil
}

// Call makes an RPC call and unmarshals the result.
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	c.requestID++

	req, err := c.buildRequest(method, params)
	if err != nil {
		return err
	}

	if err := c.sendRequest(req); err != nil {
		return err
	}

	resp, err := c.readResponse()
	if err != nil {
		return err
	}

	if resp.Error != nil {
		return resp.Error
	}

	return c.unmarshalResult(resp, result)
}

// buildRequest creates a JSON-RPC request.
func (c *Client) buildRequest(method string, params any) (*Request, error) {
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

// sendRequest sends a JSON-RPC request to the server.
func (c *Client) sendRequest(req *Request) error {
	reqData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	reqData = append(reqData, '\n')

	c.conn.SetWriteDeadline(time.Now().Add(c.timeout))
	if _, err := c.conn.Write(reqData); err != nil {
		return fmt.Errorf("write request: %w", err)
	}

	return nil
}

// readResponse reads and parses a JSON-RPC response from the server.
func (c *Client) readResponse() (*Response, error) {
	c.conn.SetReadDeadline(time.Now().Add(c.timeout))
	respData, err := c.reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(respData, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &resp, nil
}

// unmarshalResult unmarshals the response result into the provided destination.
func (c *Client) unmarshalResult(resp *Response, result any) error {
	if result == nil || resp.Result == nil {
		return nil
	}

	resultData, err := json.Marshal(resp.Result)
	if err != nil {
		return fmt.Errorf("re-marshal result: %w", err)
	}

	if err := json.Unmarshal(resultData, result); err != nil {
		return fmt.Errorf("unmarshal result: %w", err)
	}

	return nil
}

// Close closes the connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Status calls the "status" method.
func (c *Client) Status(ctx context.Context) (*StatusResult, error) {
	var result StatusResult
	if err := c.Call(ctx, "status", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PeersList calls the "peers.list" method.
func (c *Client) PeersList(ctx context.Context) (*PeersListResult, error) {
	var result PeersListResult
	if err := c.Call(ctx, "peers.list", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PeersConnect calls the "peers.connect" method.
func (c *Client) PeersConnect(ctx context.Context, inviteCode string) (*PeersConnectResult, error) {
	params := PeersConnectParams{InviteCode: inviteCode}
	var result PeersConnectResult
	if err := c.Call(ctx, "peers.connect", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// InviteCreate calls the "invite.create" method.
func (c *Client) InviteCreate(ctx context.Context, expiry string, maxUses int) (*InviteCreateResult, error) {
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
func (c *Client) InviteAccept(ctx context.Context, inviteCode string) (*InviteAcceptResult, error) {
	params := InviteAcceptParams{InviteCode: inviteCode}
	var result InviteAcceptResult
	if err := c.Call(ctx, "invite.accept", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RoutesList calls the "routes.list" method.
func (c *Client) RoutesList(ctx context.Context) (*RoutesListResult, error) {
	var result RoutesListResult
	if err := c.Call(ctx, "routes.list", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ConfigGet calls the "config.get" method.
func (c *Client) ConfigGet(ctx context.Context, key string) (*ConfigGetResult, error) {
	params := ConfigGetParams{Key: key}
	var result ConfigGetResult
	if err := c.Call(ctx, "config.get", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ConfigSet calls the "config.set" method.
func (c *Client) ConfigSet(ctx context.Context, key string, value any) (*ConfigSetResult, error) {
	params := ConfigSetParams{Key: key, Value: value}
	var result ConfigSetResult
	if err := c.Call(ctx, "config.set", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// BansList calls the "bans.list" method.
func (c *Client) BansList(ctx context.Context) (*BanListResult, error) {
	var result BanListResult
	if err := c.Call(ctx, "bans.list", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// BansAdd calls the "bans.add" method.
func (c *Client) BansAdd(ctx context.Context, nodeID, reason, description string, duration string) (*BanAddResult, error) {
	params := BanAddParams{
		NodeID:      nodeID,
		Reason:      reason,
		Description: description,
		Duration:    duration,
	}
	var result BanAddResult
	if err := c.Call(ctx, "bans.add", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// BansRemove calls the "bans.remove" method.
func (c *Client) BansRemove(ctx context.Context, nodeID string) (*BanRemoveResult, error) {
	params := BanRemoveParams{NodeID: nodeID}
	var result BanRemoveResult
	if err := c.Call(ctx, "bans.remove", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
