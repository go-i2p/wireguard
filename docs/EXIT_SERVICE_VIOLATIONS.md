# Exit Service Cross-Platform Violations Report

## Executive Summary

The exit service implementation has **CRITICAL cross-platform violations** in the exit node code. While the exit client was recently refactored to be cross-platform, the **exit node (`exit_node.go`) remains Linux-only** and will fail on macOS, Windows, and BSD systems.

## Critical Issues Found

### 1. Exit Node - Linux-Only Implementation ❌

**File:** `lib/core/exit_node.go`

**Violations:**
- ✗ No build tags - compiles on all platforms but only works on Linux
- ✗ Hard-coded `iptables` command usage
- ✗ Hard-coded `sysctl` command usage  
- ✗ Assumes Linux networking stack and sysctl parameters

**Problematic Code:**

```go
// Line 45-47: Checks for Linux-specific commands
if err := checkCommandAvailable("iptables"); err != nil {
    return nil, fmt.Errorf("iptables not available: %w", err)
}

// Line 50-52: sysctl is BSD/Linux only
if err := checkCommandAvailable("sysctl"); err != nil {
    return nil, fmt.Errorf("sysctl not available: %w", err)
}

// Line 135: Linux-specific sysctl parameter
if err := setSysctl("net.ipv4.ip_forward", "1"); err != nil {

// Line 167: iptables NAT rule
natRule := fmt.Sprintf("-t nat -A POSTROUTING -o %s -j MASQUERADE", e.publicInterface)

// Line 223: Direct iptables command execution
cmd := exec.Command("iptables", args...)

// Line 233: Direct sysctl command execution
cmd := exec.Command("sysctl", "-w", fmt.Sprintf("%s=%s", key, value))
```

**Impact:**
- **macOS**: Will fail - no iptables, different firewall (pfctl), different sysctl params
- **Windows**: Will fail - no iptables/sysctl, uses netsh/route
- **BSD**: Partial failure - has sysctl but different syntax, uses pf not iptables

### 2. Exit Client - Properly Cross-Platform ✅

**Files:** 
- `lib/core/exit_client.go` (interfaces)
- `lib/core/exit_client_linux.go`
- `lib/core/exit_client_darwin.go`
- `lib/core/exit_client_windows.go`
- `lib/core/exit_client_bsd.go`

**Strengths:**
- ✓ Clean interface-based design (`RouteManager`, `FirewallManager`)
- ✓ Platform-specific implementations with proper build tags
- ✓ Uses native APIs (netlink on Linux, route on macOS/BSD, route.exe on Windows)
- ✓ Graceful degradation when tools unavailable

**Minor Issues:**
- Uses `exec.Command` for route manipulation (acceptable, no stdlib alternative)
- Firewall implementations are placeholders (documented as MVP)

## Severity Assessment

| Component | Issue | Severity | Platforms Affected |
|-----------|-------|----------|-------------------|
| **Exit Node** | No build tags | 🔴 CRITICAL | All non-Linux |
| **Exit Node** | Hard-coded iptables | 🔴 CRITICAL | macOS, Windows, BSD |
| **Exit Node** | Hard-coded sysctl | 🔴 CRITICAL | Windows |
| **Exit Node** | Linux sysctl params | 🔴 CRITICAL | macOS, Windows, BSD |
| Exit Client | exec.Command usage | 🟡 ACCEPTABLE | None (proper per platform) |
| Exit Client | Placeholder firewalls | 🟢 DOCUMENTED | None (MVP limitation) |

## Recommended Actions

### Immediate (Critical)

1. **Add Build Tag to Exit Node**
   ```go
   //go:build linux
   
   package core
   ```
   This prevents compilation failures on other platforms.

2. **Create Platform-Specific Exit Node Implementations**
   - `exit_node_linux.go` - Current implementation
   - `exit_node_darwin.go` - macOS (pfctl, sysctl)
   - `exit_node_windows.go` - Windows (netsh, Windows Firewall)
   - `exit_node_bsd.go` - BSD (pf, sysctl)

3. **Extract Common Interface**
   ```go
   type NATManager interface {
       Enable(publicInterface string) error
       Disable() error
   }
   
   type ForwardingManager interface {
       EnableIPForwarding() error
       DisableIPForwarding() error
   }
   ```

### Short Term

4. **Refactor Exit Node Architecture**
   - Follow same pattern as exit client
   - Interface-based design
   - Platform-specific implementations
   - Proper error handling and cleanup

5. **Update Tests**
   - Add platform detection
   - Skip gracefully when tools unavailable
   - Test on multiple platforms in CI/CD

### Long Term

6. **Consider Library-Based Approach**
   - Linux: `github.com/google/nftables` for firewall
   - Linux: `github.com/vishvananda/netlink` for routing
   - Windows: COM API for Windows Firewall
   - Reduces external command dependencies

7. **Add Privilege Detection**
   - Check for CAP_NET_ADMIN on Linux
   - Check for Administrator on Windows
   - Provide clear error messages

## Comparison: Exit Client vs Exit Node

| Aspect | Exit Client ✅ | Exit Node ❌ |
|--------|---------------|-------------|
| Build tags | Yes, per platform | **No tags** |
| Interfaces | RouteManager, FirewallManager | **None** |
| Platform support | Linux, macOS, Windows, BSD | **Linux only** |
| Command usage | Platform-appropriate | **iptables/sysctl hardcoded** |
| Error handling | Graceful degradation | **Fails on other platforms** |
| Documentation | Comprehensive | Limited |

## Detailed Analysis by Platform

### Linux ✅
**Current Status:** Works
- iptables available
- sysctl available
- net.ipv4.ip_forward parameter exists
- No issues

### macOS ❌
**Current Status:** Will fail
**Issues:**
1. No iptables command
2. Uses `pfctl` for firewall/NAT
3. sysctl parameter names differ:
   - Linux: `net.ipv4.ip_forward`
   - macOS: `net.inet.ip.forwarding`
4. Different NAT configuration (pf.conf)

**Required:**
```bash
# Enable forwarding
sysctl -w net.inet.ip.forwarding=1

# NAT rule via pfctl
echo "nat on en0 from 10.42.0.0/16 to any -> (en0)" | pfctl -ef -
```

### Windows ❌
**Current Status:** Will fail
**Issues:**
1. No iptables command
2. No sysctl command
3. Uses `netsh` for firewall and IP forwarding
4. Different networking model

**Required:**
```powershell
# Enable forwarding
netsh interface ipv4 set interface "Ethernet" forwarding=enabled

# NAT rule
netsh advfirewall firewall add rule ...
```

### BSD ❌
**Current Status:** Partial failure
**Issues:**
1. No iptables (uses pf)
2. Has sysctl but different parameters
3. Different firewall syntax

**Required:**
```bash
# Enable forwarding
sysctl -w net.inet.ip.forwarding=1

# NAT via pf
echo "nat on em0 from 10.42.0.0/16 to any -> (em0)" | pfctl -f -
```

## Code Snippets: Proper Cross-Platform Pattern

### Current Exit Node (Bad) ❌
```go
// No build tags - compiles everywhere but only works on Linux!
package core

func (e *ExitNode) setupNAT() error {
    // Hard-coded iptables - fails on macOS/Windows/BSD
    natRule := fmt.Sprintf("-t nat -A POSTROUTING -o %s -j MASQUERADE", e.publicInterface)
    cmd := exec.Command("iptables", args...)
    // ...
}
```

### Recommended Exit Node (Good) ✅
```go
//go:build linux
package core

type natManager interface {
    Enable(publicInterface string) error
    Disable() error
}

func newPlatformNATManager() (natManager, error) {
    switch runtime.GOOS {
    case "linux":
        return newLinuxNATManager()
    case "darwin":
        return newDarwinNATManager()
    // ...
    }
}
```

## Testing Implications

### Current Test Coverage
```bash
$ go test ./lib/core -run ExitNode
# Tests PASS on Linux
# Tests FAIL on macOS/Windows/BSD (commands not found)
```

### Required Test Updates
```go
func TestExitNode(t *testing.T) {
    // Need to skip on unsupported platforms
    if runtime.GOOS != "linux" {
        t.Skip("exit node currently Linux-only")
    }
    // ...
}
```

## Migration Path

### Phase 1: Isolate (Immediate)
```bash
# Add build tag to existing file
echo "//go:build linux" | cat - lib/core/exit_node.go > temp && mv temp lib/core/exit_node.go
```

### Phase 2: Extract Interfaces (Short Term)
```go
// exit_node.go (no build tag - interfaces only)
type NATManager interface { /* ... */ }
type ForwardingManager interface { /* ... */ }
type ExitNode struct { /* ... */ }

// exit_node_linux.go
//go:build linux
type linuxNATManager struct { /* ... */ }

// exit_node_darwin.go
//go:build darwin
type darwinNATManager struct { /* ... */ }
```

### Phase 3: Implement (Medium Term)
- Implement platform-specific managers
- Add comprehensive tests
- Update documentation

## Recommendations Summary

**Priority 1 (Immediate):**
1. Add `//go:build linux` to `exit_node.go`
2. Document Linux-only limitation in comments and README
3. Add skip logic to tests on non-Linux platforms

**Priority 2 (Short Term):**
4. Refactor exit_node.go to use interface-based design
5. Create platform-specific implementations
6. Follow exit_client.go pattern

**Priority 3 (Long Term):**
7. Consider library-based approach (netlink, nftables)
8. Add privilege detection and helpful error messages
9. Implement complete cross-platform support

## Conclusion

The exit node implementation has **critical cross-platform violations** that prevent it from working on macOS, Windows, and BSD. While the exit client was properly refactored with cross-platform support, the exit node was left with Linux-only assumptions.

**Immediate action required:** Add build tags to prevent compilation failures on non-Linux platforms, then follow the exit client pattern for a proper cross-platform implementation.

The exit client provides an excellent template for how to do this correctly with its interface-based design and platform-specific implementations.
