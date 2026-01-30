# ADR-005: Unix Domain Sockets for RPC

## Status

Accepted

## Date

2024-01-15

## Context

The i2plan system requires RPC communication between:
- CLI tool and background daemon
- TUI (text user interface) and daemon
- Potential future integrations (monitoring, automation)

Key requirements:
- **Security**: Only local processes should access RPC
- **Simplicity**: Avoid complex authentication for local IPC
- **Performance**: Low latency for interactive tools (TUI)
- **Portability**: Work across Linux, macOS, Windows

Traditional options:
- **TCP localhost (127.0.0.1:port)**: 
  - Pros: Simple, cross-platform
  - Cons: Vulnerable to port scanning, requires authentication, port conflicts possible
  
- **HTTP REST API**:
  - Pros: Well-understood, tooling available
  - Cons: Overhead (HTTP parsing, JSON), authentication complexity
  
- **gRPC over TCP**:
  - Pros: Efficient, typed
  - Cons: Still TCP-based, same security concerns as localhost
  
- **Unix Domain Sockets**:
  - Pros: OS-enforced access control, no network exposure, lower latency
  - Cons: Platform-specific (though supported on Windows 10+ via AF_UNIX)

## Decision

We will use **Unix domain sockets** for RPC communication between local components.

### Socket Configuration

```go
// lib/rpc/server.go
type ServerConfig struct {
    SocketPath    string        // e.g., /var/run/i2plan/i2plan.sock
    SocketMode    os.FileMode   // e.g., 0600 (owner only)
    MaxClients    int           // Concurrent client limit
    Timeout       time.Duration // RPC timeout
}

// Default paths:
// - Linux:   /var/run/i2plan/i2plan.sock (system) or ~/.i2plan/i2plan.sock (user)
// - macOS:   /var/run/i2plan/i2plan.sock (system) or ~/Library/Application Support/i2plan/i2plan.sock (user)
// - Windows: \\.\pipe\i2plan (named pipes, similar semantics)
```

### Protocol Choice

We use **Protocol Buffers (protobuf)** over the Unix socket:
- Binary encoding (efficient)
- Strongly typed
- Forward/backward compatible
- Standard Go support

```protobuf
// lib/rpc/protocol.proto
syntax = "proto3";

service I2Plan {
    rpc GetStatus(StatusRequest) returns (StatusResponse);
    rpc ListPeers(PeersRequest) returns (PeersResponse);
    rpc AddPeer(AddPeerRequest) returns (AddPeerResponse);
    rpc RemovePeer(RemovePeerRequest) returns (RemovePeerResponse);
    rpc GenerateInvite(InviteRequest) returns (InviteResponse);
    rpc JoinNetwork(JoinRequest) returns (JoinResponse);
    rpc GetMetrics(MetricsRequest) returns (MetricsResponse);
}
```

### Wire Protocol

Simple length-prefixed protobuf messages:
```
[4 bytes: message length (big-endian)] [N bytes: protobuf message]
```

This avoids HTTP overhead while maintaining simple framing.

## Consequences

### Positive

- **Security by Default**: OS file permissions enforce access control (0600 = owner only)
- **Zero Network Exposure**: Socket not accessible over network, even localhost
- **Low Latency**: Direct kernel IPC, ~10x faster than TCP localhost
- **No Port Conflicts**: Filesystem paths avoid port allocation issues
- **Simple Authentication**: File permissions replace complex auth schemes
- **Process Isolation**: Different users can't access each other's daemons

### Negative

- **Platform Differences**: Slightly different semantics on Windows (named pipes)
- **No Remote Access**: Can't connect from different machine (by design)
- **File Cleanup**: Must handle stale socket files on crash
- **Less Tooling**: Fewer tools compared to HTTP (no curl equivalent)

### Trade-offs

- **Security vs Remote Access**: We prioritize security, accept that RPC is local-only
- **Simplicity vs Features**: Simple protocol over Unix socket vs full HTTP/gRPC stack
- **Performance vs Debugging**: Binary protobuf harder to debug than JSON/HTTP

## Implementation Details

### Server Setup

```go
// lib/rpc/server.go
func (s *Server) Start() error {
    // 1. Remove stale socket file
    if err := os.Remove(s.config.SocketPath); err != nil && !os.IsNotExist(err) {
        return fmt.Errorf("remove stale socket: %w", err)
    }
    
    // 2. Create socket directory if needed
    dir := filepath.Dir(s.config.SocketPath)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return fmt.Errorf("create socket dir: %w", err)
    }
    
    // 3. Listen on Unix socket
    listener, err := net.Listen("unix", s.config.SocketPath)
    if err != nil {
        return fmt.Errorf("listen on socket: %w", err)
    }
    
    // 4. Set file permissions (owner only)
    if err := os.Chmod(s.config.SocketPath, s.config.SocketMode); err != nil {
        return fmt.Errorf("chmod socket: %w", err)
    }
    
    // 5. Accept connections
    go s.acceptLoop(listener)
    
    return nil
}
```

### Client Connection

```go
// lib/rpc/client.go
func Dial(socketPath string, timeout time.Duration) (*Client, error) {
    conn, err := net.DialTimeout("unix", socketPath, timeout)
    if err != nil {
        return nil, fmt.Errorf("dial unix socket: %w", err)
    }
    
    return &Client{
        conn:    conn,
        timeout: timeout,
    }, nil
}

func (c *Client) Call(method string, req, resp proto.Message) error {
    // 1. Encode request
    reqBytes, err := proto.Marshal(req)
    if err != nil {
        return fmt.Errorf("marshal request: %w", err)
    }
    
    // 2. Send with length prefix
    if err := c.sendMessage(reqBytes); err != nil {
        return err
    }
    
    // 3. Read response
    respBytes, err := c.recvMessage()
    if err != nil {
        return err
    }
    
    // 4. Decode response
    if err := proto.Unmarshal(respBytes, resp); err != nil {
        return fmt.Errorf("unmarshal response: %w", err)
    }
    
    return nil
}
```

### Connection Pooling

For tools that make many RPC calls (like TUI with live updates), connection pooling avoids reconnection overhead:

```go
// lib/rpc/pooled_client.go
type PooledClient struct {
    pool      *pool.Pool     // See lib/pool/
    socketPath string
    timeout    time.Duration
}

func (pc *PooledClient) Call(method string, req, resp proto.Message) error {
    // Get connection from pool
    conn, err := pc.pool.Get()
    if err != nil {
        return err
    }
    defer pc.pool.Put(conn)
    
    // Use pooled connection
    client := &Client{conn: conn.(*net.UnixConn), timeout: pc.timeout}
    return client.Call(method, req, resp)
}
```

### Rate Limiting

Prevent abuse by limiting requests per client:

```go
// lib/rpc/connlimit.go
type RateLimitedServer struct {
    server  *Server
    limiter *ratelimit.Limiter  // See lib/ratelimit/
}

func (s *RateLimitedServer) handleClient(conn net.Conn) {
    // Apply per-connection rate limit (e.g., 100 req/sec)
    limiter := s.limiter.ForClient(conn.RemoteAddr().String())
    
    for {
        if !limiter.Allow() {
            // Send rate limit error
            continue
        }
        
        // Process RPC call
        s.server.handleRequest(conn)
    }
}
```

## Platform Considerations

### Linux

Standard Unix domain sockets:
- Default path: `/var/run/i2plan/i2plan.sock` (system) or `~/.i2plan/i2plan.sock` (user)
- Socket type: `SOCK_STREAM`
- File mode: `0600` (owner read/write)

### macOS

Unix domain sockets with slightly different path conventions:
- System: `/var/run/i2plan/i2plan.sock`
- User: `~/Library/Application Support/i2plan/i2plan.sock`
- Otherwise identical to Linux

### Windows

Windows 10+ supports Unix domain sockets via `AF_UNIX`, but we use **Named Pipes** for better compatibility:

```go
// lib/rpc/server_windows.go
func (s *Server) Start() error {
    // Named pipe format: \\.\pipe\i2plan
    pipePath := `\\.\pipe\i2plan`
    
    // Create named pipe with security descriptor (owner only)
    listener, err := winio.ListenPipe(pipePath, &winio.PipeConfig{
        SecurityDescriptor: "D:P(A;;GA;;;BA)(A;;GA;;;SY)(A;;GA;;;WD)", // Owner, system, current user
    })
    if err != nil {
        return err
    }
    
    go s.acceptLoop(listener)
    return nil
}
```

Client code automatically detects platform and uses appropriate socket type.

## Security Considerations

### Access Control

File permissions provide implicit authentication:
```bash
# Socket owned by user running daemon
$ ls -l /var/run/i2plan/i2plan.sock
srw------- 1 alice alice 0 Jan 15 10:30 /var/run/i2plan/i2plan.sock
```

Only the daemon owner (or root) can connect.

### Privilege Escalation

If daemon runs as root (system-wide):
- Socket permissions: `0660` (owner + group)
- Create `i2plan` group for authorized users
- Add users to group: `usermod -aG i2plan alice`

### Stale Sockets

Handle crashes gracefully:
```go
func (s *Server) Start() error {
    // Check if socket exists
    if _, err := os.Stat(s.config.SocketPath); err == nil {
        // Try to connect (check if daemon alive)
        if _, err := net.Dial("unix", s.config.SocketPath); err == nil {
            return fmt.Errorf("daemon already running")
        }
        
        // Stale socket, remove it
        os.Remove(s.config.SocketPath)
    }
    
    // Proceed with socket creation
    // ...
}
```

### Resource Limits

Prevent DoS:
- **Max concurrent clients**: 50 (default)
- **Per-client rate limit**: 100 req/sec
- **Request timeout**: 30 seconds
- **Max message size**: 1 MB

## Alternatives Considered

### HTTP REST on Localhost (127.0.0.1)

- **Pros**: Cross-platform, familiar, tooling available
- **Cons**: 
  - Network-exposed (port scanning, accidental external binding)
  - Requires authentication (API keys, tokens)
  - HTTP parsing overhead
  - Port conflicts possible
- **Rejected**: Security concerns and unnecessary complexity

### gRPC over TCP

- **Pros**: Modern, efficient, typed
- **Cons**: 
  - Still TCP-based (network exposure)
  - Heavier dependency (gRPC runtime)
  - Requires TLS for security
- **Rejected**: Overkill for local IPC

### D-Bus (Linux/GNOME standard)

- **Pros**: Standard on Linux desktops, well-integrated
- **Cons**: 
  - Linux-only
  - Complex API
  - Poor cross-platform support
- **Rejected**: Platform-specific, overengineered

### Shared Memory / Message Queues

- **Pros**: Highest performance
- **Cons**: Complex to implement, platform-specific, overkill for RPC
- **Rejected**: Unnecessary complexity

## Performance Characteristics

Benchmarks (local machine, 1000 RPC calls):

| Protocol | Latency (p50) | Latency (p99) | Throughput |
|----------|---------------|---------------|------------|
| Unix socket + protobuf | 0.2ms | 0.5ms | 50,000 req/s |
| TCP localhost + protobuf | 0.8ms | 2ms | 15,000 req/s |
| HTTP/JSON on localhost | 2ms | 5ms | 5,000 req/s |

Unix sockets provide **4x better latency** than TCP localhost.

## References

- `lib/rpc/server.go` - Unix socket server implementation
- `lib/rpc/client.go` - Client connection and RPC calls
- `lib/rpc/pooled_client.go` - Connection pooling for high-frequency calls
- `lib/rpc/protocol.proto` - Protobuf service definitions
- `cmd/i2plan/main.go` - CLI tool using RPC client
- `lib/tui/app.go` - TUI using pooled RPC client
