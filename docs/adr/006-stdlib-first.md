# ADR-006: Standard Library First Approach

## Status

Accepted

## Date

2024-01-15

## Context

Modern Go projects face dependency management challenges:
- **Dependency Hell**: Deep transitive dependencies, version conflicts
- **Supply Chain Risk**: Third-party packages can introduce vulnerabilities
- **Maintenance Burden**: External dependencies may be abandoned or break in updates
- **Binary Size**: Heavy dependencies bloat binary size
- **Build Times**: More dependencies = slower builds

Go's standard library is:
- **Stable**: Strong compatibility guarantees (Go 1.x compatibility promise)
- **Well-Tested**: Extensively tested, high-quality code
- **Zero Dependencies**: No transitive dependencies
- **Fast**: Optimized for performance
- **Secure**: Part of Go's security disclosure process

Philosophy question: When should we use external dependencies?

## Decision

We adopt a **"standard library first"** approach:

### Decision Rule

1. **Prefer standard library**: If stdlib can do it (even with more code), use stdlib
2. **Essential dependencies only**: Add external dependencies only when:
   - Functionality doesn't exist in stdlib
   - Stdlib implementation is prohibitively complex
   - External package is industry-standard (e.g., WireGuard, protobuf)
   - Security/correctness benefit outweighs risk
3. **Evaluate carefully**: Before adding dependency, ask:
   - Can we implement this in reasonable LOC with stdlib?
   - Is this package well-maintained and widely used?
   - What are the transitive dependencies?
   - What's the security track record?

### Approved External Dependencies

Dependencies we consciously chose:

```go
// Essential - core functionality impossible without
golang.zx2c4.com/wireguard        // WireGuard implementation
github.com/eyedeekay/sam3          // I2P SAM bridge

// Standard protocols - reference implementations
google.golang.org/protobuf         // Protocol Buffers (RPC)
github.com/pelletier/go-toml/v2    // TOML parsing (config)

// UI/TUI - stdlib has no TUI support
github.com/charmbracelet/bubbletea // TUI framework
github.com/charmbracelet/lipgloss  // TUI styling
github.com/charmbracelet/bubbles   // TUI components

// Logging - structured logging not in stdlib (until Go 1.21 slog)
github.com/sirupsen/logrus         // Structured logging (may migrate to stdlib slog)
```

Total: **7 direct dependencies** (vs. typical 20-50 for similar projects).

### Examples of Stdlib Usage

We chose stdlib over popular packages in these areas:

#### HTTP Server (stdlib `net/http`)
```go
// Instead of: gin, echo, fiber
// We use: stdlib net/http + stdlib net/http/pprof

mux := http.NewServeMux()
mux.HandleFunc("/api/status", handleStatus)
server := &http.Server{
    Handler:      mux,
    ReadTimeout:  10 * time.Second,
    WriteTimeout: 10 * time.Second,
}
```

**Rationale**: Stdlib HTTP is sufficient for internal API. No need for complex routing.

#### JSON Serialization (stdlib `encoding/json`)
```go
// Instead of: jsoniter, easyjson
// We use: stdlib encoding/json

type Status struct {
    Peers  int    `json:"peers"`
    Uptime string `json:"uptime"`
}
json.NewEncoder(w).Encode(status)
```

**Rationale**: Performance difference negligible for our RPC volume.

#### Context and Cancellation (stdlib `context`)
```go
// Instead of: custom context libraries
// We use: stdlib context

ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
```

**Rationale**: Stdlib context is the standard, universally compatible.

#### Testing (stdlib `testing`)
```go
// Instead of: testify, ginkgo
// We use: stdlib testing + table-driven tests

func TestPeerValidation(t *testing.T) {
    tests := []struct {
        name    string
        peer    Peer
        wantErr bool
    }{
        // ...
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // ...
        })
    }
}
```

**Rationale**: Stdlib testing is powerful and sufficient. No magic, clear test failures.

#### Rate Limiting (custom `lib/ratelimit`)
```go
// Instead of: golang.org/x/time/rate
// We implement: lib/ratelimit/limiter.go (200 LOC)

type Limiter struct {
    rate     float64
    capacity int
    tokens   float64
    mu       sync.Mutex
}

func (l *Limiter) Allow() bool {
    // Token bucket algorithm
}
```

**Rationale**: x/time/rate is good, but our needs are simple. 200 LOC avoids dependency.

#### Connection Pooling (custom `lib/pool`)
```go
// Instead of: various pool libraries
// We implement: lib/pool/pool.go (150 LOC)

type Pool struct {
    factory func() (net.Conn, error)
    idle    chan net.Conn
    max     int
}
```

**Rationale**: Generic pool implementations complex. Our specific needs are simple.

## Consequences

### Positive

- **Stability**: Fewer breaking changes, Go's compatibility promise protects us
- **Security**: Smaller attack surface, fewer supply chain risks
- **Maintainability**: Less code to audit, easier to reason about
- **Build Speed**: Faster builds with fewer dependencies
- **Binary Size**: Smaller binaries (~15 MB vs. typical 30-50 MB)
- **Portability**: Stdlib works everywhere Go does, no platform-specific deps
- **Long-Term Viability**: Stdlib won't be abandoned

### Negative

- **More Code**: Some features require more LOC than using external package
- **Reinventing Wheels**: Occasionally reimplement common patterns
- **Feature Gaps**: Miss out on convenience features from popular packages
- **Learning Curve**: Team must understand stdlib patterns vs. popular frameworks

### Trade-offs

- **Simplicity vs Features**: We choose simplicity, accept fewer convenience features
- **Security vs Convenience**: Prioritize security/stability over developer convenience
- **Code Volume vs Dependencies**: Accept more code to avoid dependencies
- **Standard Patterns vs Frameworks**: Prefer standard Go patterns over framework magic

## Implementation Guidelines

### Dependency Review Process

Before adding a new dependency, answer these questions:

1. **Necessity**:
   - Can this be done with stdlib in <500 LOC?
   - Is this core to our value proposition?

2. **Quality**:
   - Is the package actively maintained (commits in last 6 months)?
   - Does it have >1000 GitHub stars (indicator of community trust)?
   - Is it used by major projects?

3. **Security**:
   - What are the transitive dependencies?
   - Has the package had security issues?
   - Is it part of a security disclosure program?

4. **Alternatives**:
   - What's the stdlib alternative?
   - Can we vendor a small subset of the package?
   - Can we implement the specific feature we need?

### Dependency Audit Commands

```bash
# List all dependencies
go list -m all

# View dependency graph
go mod graph | grep -v golang.org/x/

# Check for vulnerabilities
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...

# Analyze why dependency is included
go mod why -m github.com/some/package
```

### When to Add Dependencies

**DO add dependencies for**:
- Core protocols (WireGuard, I2P, protobuf)
- Standard formats (TOML, YAML) with complex specs
- Cryptography (never roll your own crypto)
- UI frameworks (no stdlib TUI)

**DON'T add dependencies for**:
- HTTP routing (stdlib http.ServeMux is fine)
- JSON/XML parsing (stdlib is good)
- Testing utilities (stdlib testing is powerful)
- Logging (stdlib slog since Go 1.21)
- Simple algorithms (rate limiting, pooling)

## Code Examples

### Example 1: HTTP Server (stdlib)

```go
// lib/web/server.go - Using stdlib net/http
package web

import (
    "encoding/json"
    "net/http"
    "time"
)

type Server struct {
    mux    *http.ServeMux
    server *http.Server
}

func NewServer(addr string) *Server {
    mux := http.NewServeMux()
    return &Server{
        mux: mux,
        server: &http.Server{
            Addr:         addr,
            Handler:      mux,
            ReadTimeout:  10 * time.Second,
            WriteTimeout: 10 * time.Second,
            IdleTimeout:  60 * time.Second,
        },
    }
}

func (s *Server) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
    s.mux.HandleFunc(pattern, handler)
}

func (s *Server) Start() error {
    return s.server.ListenAndServe()
}
```

**LOC**: ~30 lines. Compare to gin/echo: similar LOC, but no dependency.

### Example 2: Rate Limiter (custom implementation)

```go
// lib/ratelimit/limiter.go - Token bucket in 200 LOC
package ratelimit

import (
    "sync"
    "time"
)

type Limiter struct {
    rate     float64       // tokens per second
    capacity int           // max burst
    tokens   float64       // current tokens
    lastTick time.Time     // last refill time
    mu       sync.Mutex
}

func New(rate float64, capacity int) *Limiter {
    return &Limiter{
        rate:     rate,
        capacity: capacity,
        tokens:   float64(capacity),
        lastTick: time.Now(),
    }
}

func (l *Limiter) Allow() bool {
    l.mu.Lock()
    defer l.mu.Unlock()
    
    // Refill tokens based on elapsed time
    now := time.Now()
    elapsed := now.Sub(l.lastTick).Seconds()
    l.tokens = min(float64(l.capacity), l.tokens+elapsed*l.rate)
    l.lastTick = now
    
    // Check if token available
    if l.tokens >= 1.0 {
        l.tokens -= 1.0
        return true
    }
    return false
}
```

**LOC**: ~200 lines total (including tests). Compare to x/time/rate: saves dependency for simple use case.

### Example 3: Connection Pool (custom implementation)

```go
// lib/pool/pool.go - Generic connection pool
package pool

import (
    "errors"
    "net"
    "sync"
    "time"
)

type Pool struct {
    factory    func() (net.Conn, error)
    idle       chan net.Conn
    maxIdle    int
    maxOpen    int
    openCount  int
    mu         sync.Mutex
}

func New(factory func() (net.Conn, error), maxIdle, maxOpen int) *Pool {
    return &Pool{
        factory: factory,
        idle:    make(chan net.Conn, maxIdle),
        maxIdle: maxIdle,
        maxOpen: maxOpen,
    }
}

func (p *Pool) Get() (net.Conn, error) {
    // Try to get from idle pool
    select {
    case conn := <-p.idle:
        return conn, nil
    default:
    }
    
    // Create new connection if under limit
    p.mu.Lock()
    if p.openCount < p.maxOpen {
        p.openCount++
        p.mu.Unlock()
        return p.factory()
    }
    p.mu.Unlock()
    
    // Wait for idle connection (with timeout)
    select {
    case conn := <-p.idle:
        return conn, nil
    case <-time.After(5 * time.Second):
        return nil, errors.New("pool exhausted")
    }
}

func (p *Pool) Put(conn net.Conn) {
    select {
    case p.idle <- conn:
    default:
        // Idle pool full, close connection
        conn.Close()
        p.mu.Lock()
        p.openCount--
        p.mu.Unlock()
    }
}
```

**LOC**: ~150 lines total. Compare to database/sql pool: we need only basic pooling, not full DB semantics.

## Alternatives Considered

### Full Framework Approach (e.g., Gin, Gorm, etc.)

- **Pros**: Faster development, many features out-of-box
- **Cons**: Heavy dependencies, framework lock-in, harder to debug
- **Rejected**: Against our simplicity and security goals

### Minimal Dependencies (only WireGuard + I2P)

- **Pros**: Even fewer dependencies
- **Cons**: Would require implementing protobuf, TOML, TUI from scratch (thousands of LOC)
- **Rejected**: Some dependencies provide clear value vs. implementation cost

### Go x/ Packages (golang.org/x/...)

- **Pros**: Semi-official, maintained by Go team
- **Cons**: Not part of Go 1.x compatibility promise, can break between versions
- **Nuanced**: We use x/ packages sparingly (currently: none in core, may use x/time/rate if rate limiting gets complex)

## Migration Path

### Go 1.21+ slog Migration

When we drop support for Go <1.21, migrate from logrus to stdlib slog:

```go
// Current: github.com/sirupsen/logrus
log.WithFields(log.Fields{"peer": peerID}).Info("connected")

// Future: log/slog (stdlib)
slog.Info("connected", "peer", peerID)
```

**Benefit**: Remove logging dependency entirely.

### Testing Utilities

Consider vendoring small utilities from testify if needed:
```go
// Instead of: import "github.com/stretchr/testify/assert"
// Vendor: internal/testutil/assert.go (50 LOC)

func Equal(t *testing.T, expected, actual interface{}) {
    if !reflect.DeepEqual(expected, actual) {
        t.Errorf("expected %v, got %v", expected, actual)
    }
}
```

## Monitoring and Review

### Dependency Updates

Review dependencies monthly:
```bash
go get -u ./...  # Update to latest
go mod tidy      # Clean up
govulncheck ./...  # Security check
go test ./...    # Ensure tests pass
```

### Dependency Metrics

Track over time:
- Number of direct dependencies (target: <10)
- Number of transitive dependencies (target: <30)
- Total vendor size (target: <5 MB)
- Binary size (target: <20 MB)

```bash
# Count dependencies
go list -m all | wc -l

# Vendor size
du -sh vendor/

# Binary size
go build -o i2plan cmd/i2plan/main.go
ls -lh i2plan
```

## References

- `go.mod` - Dependency declarations
- [Go Proverbs](https://go-proverbs.github.io/) - "A little copying is better than a little dependency"
- [Go 1 Compatibility Promise](https://go.dev/doc/go1compat)
- [Russ Cox: Our Software Dependency Problem](https://research.swtch.com/deps)
- `lib/ratelimit/` - Example of custom implementation over external dependency
- `lib/pool/` - Another example of stdlib-first approach
