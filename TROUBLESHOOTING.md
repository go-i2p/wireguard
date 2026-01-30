# Troubleshooting Guide

This guide helps diagnose and fix common issues with i2plan mesh VPN on Linux, macOS, Windows, and BSD systems.

## Table of Contents

1. [Quick Diagnostics](#quick-diagnostics)
2. [Permission Issues](#permission-issues)
3. [I2P Router Issues](#i2p-router-issues)
4. [WireGuard Issues](#wireguard-issues)
5. [Peer Connection Issues](#peer-connection-issues)
6. [Performance Issues](#performance-issues)
7. [Configuration Issues](#configuration-issues)
8. [Build and Installation Issues](#build-and-installation-issues)
9. [Platform-Specific Issues](#platform-specific-issues)
10. [Getting Debug Information](#getting-debug-information)

---

## Quick Diagnostics

### Check i2plan Status

```bash
# Linux/macOS/BSD
./i2plan rpc status

# Windows (PowerShell)
.\i2plan.exe rpc status
```

### Check I2P Router

```bash
# All platforms - test I2P router console
curl -I http://127.0.0.1:7657

# Test SAM bridge
# Linux/macOS/BSD
nc -zv 127.0.0.1 7656

# Windows (PowerShell)
Test-NetConnection -ComputerName 127.0.0.1 -Port 7656
```

### Check WireGuard

```bash
# Linux/BSD
ip link show wg0
sudo wg show

# macOS
ifconfig wg0

# Windows (PowerShell as Administrator)
Get-NetAdapter | Where-Object {$_.Name -like "wg*"}
```

### Check Logs

```bash
# Linux/BSD/macOS
tail -n 100 ~/.i2plan/logs/i2plan.log

# Linux (systemd)
sudo journalctl -u i2plan.service -n 100

# macOS (launchd)
tail -n 100 /usr/local/var/log/i2plan.log

# Windows (PowerShell)
Get-Content $env:APPDATA\i2plan\logs\i2plan.log -Tail 100
```

---

## Permission Issues

### Linux: "Permission denied" when creating WireGuard interface

**Symptoms:**
- Error: `operation not permitted`
- Cannot create wg0 interface

**Solutions:**

**Option 1: Run with sudo**
```bash
sudo ./i2plan start
```

**Option 2: Use capabilities (preferred)**
```bash
# Grant CAP_NET_ADMIN to i2plan binary
sudo setcap cap_net_admin=+ep /usr/local/bin/i2plan

# Verify capabilities were set
getcap /usr/local/bin/i2plan
# Should show: /usr/local/bin/i2plan = cap_net_admin+ep

# Now run without sudo
./i2plan start
```

**Option 3: Check if capabilities were lost after rebuild**
```bash
# Capabilities are tied to the binary's inode
# Rebuild clears them, so reapply:
go build -o i2plan cmd/i2plan/main.go
sudo cp i2plan /usr/local/bin/
sudo setcap cap_net_admin=+ep /usr/local/bin/i2plan
```

### macOS: "Operation not permitted"

**Symptoms:**
- Cannot create TUN/TAP device
- Permission errors

**Solution:**
macOS always requires root for network interface creation:
```bash
sudo ./i2plan start
```

Or install as launchd service (see DEPLOYMENT.md).

### Windows: "Access is denied"

**Symptoms:**
- Cannot create WireGuard tunnel
- "You must be an administrator" error

**Solution:**
Run PowerShell as Administrator:
1. Right-click PowerShell icon
2. Select "Run as Administrator"
3. Navigate to i2plan directory
4. Run: `.\i2plan.exe start`

Or install as Windows Service (see DEPLOYMENT.md).

### BSD: Permission errors

**FreeBSD:**
```bash
sudo ./i2plan start
```

**OpenBSD:**
```bash
# Configure doas if not already
echo "permit nopass $USER" | sudo tee -a /etc/doas.conf
doas ./i2plan start
```

---

## I2P Router Issues

### I2P Router Not Running

**Symptoms:**
- `connection refused` on port 7657 or 7656
- Can't access http://127.0.0.1:7657

**Diagnosis:**
```bash
# Check if I2P is running
# Linux
systemctl status i2p
ps aux | grep i2p

# macOS
ps aux | grep i2prouter

# Windows (PowerShell)
Get-Process | Where-Object {$_.Name -like "*i2p*"}

# BSD
service i2p status  # FreeBSD
rcctl check i2pd    # OpenBSD
```

**Solutions:**

**Linux:**
```bash
sudo systemctl start i2p
sudo systemctl enable i2p
```

**macOS:**
```bash
i2prouter start
```

**Windows:**
- Start Menu → I2P → Start I2P Router
- Or check Windows Services for "I2P Service"

**FreeBSD:**
```bash
sudo service i2p start
sudo sysrc i2p_enable="YES"
```

**OpenBSD:**
```bash
doas rcctl start i2pd
doas rcctl enable i2pd
```

### SAM Bridge Not Enabled

**Symptoms:**
- I2P router running but port 7656 not listening
- Error: `connection refused` on port 7656

**Solution:**
1. Open I2P router console: http://127.0.0.1:7657
2. Navigate to: Configure → Clients
3. Find "SAM application bridge"
4. Check the box to enable it
5. Verify port is 7656 (default)
6. Click "Save" and restart I2P

**Verify SAM is listening:**
```bash
# Linux/BSD/macOS
netstat -ln | grep 7656
# Should show: tcp 0 0 127.0.0.1:7656 0.0.0.0:* LISTEN

# Windows (PowerShell)
netstat -an | findstr 7656
```

### I2P Tunnels Not Building

**Symptoms:**
- SAM bridge enabled but i2plan can't connect
- Error: `tunnel not ready`
- Peers not discovered

**Diagnosis:**
Check I2P router console → Tunnels:
- Need at least 1-2 participating tunnels
- Tunnel build success rate should be >70%

**Solutions:**

1. **Wait for tunnel initialization** (1-3 minutes after I2P starts)

2. **Check I2P network connectivity:**
   - Router console → Network → Status
   - Should show "Network: OK" with active peers

3. **Firewall blocking I2P:**
   ```bash
   # Linux (temporarily disable to test)
   sudo ufw disable
   sudo iptables -F
   
   # Windows
   # Disable Windows Firewall temporarily in Control Panel
   
   # If this fixes it, add I2P firewall rules instead
   ```

4. **Increase I2P tunnel quantity:**
   - Router console → Configure → Tunnels
   - Increase inbound/outbound tunnel quantity to 3-4

5. **Restart I2P:**
   ```bash
   # Linux
   sudo systemctl restart i2p
   
   # macOS
   i2prouter restart
   
   # Windows
   # Restart from Start Menu or Services
   
   # FreeBSD
   sudo service i2p restart
   
   # OpenBSD
   doas rcctl restart i2pd
   ```

---

## WireGuard Issues

### WireGuard Kernel Module Not Loaded (Linux)

**Symptoms:**
- Error: `module not found`
- `lsmod | grep wireguard` returns nothing

**Solution:**
```bash
# Load module
sudo modprobe wireguard

# Verify
lsmod | grep wireguard

# Load automatically on boot
echo "wireguard" | sudo tee -a /etc/modules

# If module doesn't exist, install WireGuard
# Debian/Ubuntu
sudo apt install wireguard wireguard-dkms linux-headers-$(uname -r)

# Fedora/RHEL
sudo dnf install wireguard-tools kernel-devel

# Arch
sudo pacman -S wireguard-tools wireguard-dkms
```

### WireGuard Not Installed (macOS)

**Symptoms:**
- Command not found: `wg`
- Interface creation fails

**Solution:**
```bash
brew install wireguard-tools
```

### WireGuard Service Not Running (Windows)

**Symptoms:**
- Cannot create tunnel
- Service errors

**Solution:**
1. Open Services (services.msc)
2. Find "WireGuard Tunnel Service"
3. Set to "Automatic" startup
4. Start the service

Or reinstall WireGuard from https://www.wireguard.com/install/

### WireGuard Interface Doesn't Come Up

**Symptoms:**
- i2plan starts but no wg0 interface
- No IP address assigned

**Diagnosis:**
```bash
# Linux/BSD
ip addr show wg0
# or
ifconfig wg0

# macOS
ifconfig wg0

# Windows
ipconfig /all
```

**Solution:**
Check i2plan logs for errors:
```bash
# Look for:
# - "failed to create interface"
# - "invalid configuration"
# - Permission errors

tail -n 50 ~/.i2plan/logs/i2plan.log | grep -i "error\|fail"
```

---

## Peer Connection Issues

### Cannot Discover Peers

**Symptoms:**
- `./i2plan peers list` shows no peers
- Mesh appears empty

**Diagnosis:**
1. **Verify I2P connectivity:**
   ```bash
   # Check I2P has built tunnels
   # Visit http://127.0.0.1:7657 → Tunnels
   ```

2. **Check invite was accepted:**
   ```bash
   ./i2plan rpc status
   # Should show you're member of mesh
   ```

**Solutions:**

1. **Wait for gossip protocol** (30-60 seconds):
   The mesh uses eventual consistency. Give it time.

2. **Manually check I2P destination:**
   ```bash
   # Get your I2P destination
   ./i2plan identity show
   
   # Verify it's reachable (on another node)
   ./i2plan peers list
   ```

3. **Restart i2plan:**
   ```bash
   # Force reconnection
   ./i2plan stop
   ./i2plan start
   ```

### Peers Discovered But Can't Connect

**Symptoms:**
- `peers list` shows peers
- No WireGuard handshake
- Cannot ping peer IPs

**Diagnosis:**
```bash
# Check WireGuard handshake status
sudo wg show wg0

# Should show:
#   peer: <pubkey>
#   latest handshake: <time>   # Should be recent
#   transfer: <bytes> received, <bytes> sent
```

**Solutions:**

1. **Check for recent handshake:**
   - If "latest handshake" is missing or old (>3 minutes), connection is down

2. **Verify I2P destinations are reachable:**
   ```bash
   # On node A, get destination
   ./i2plan identity show
   
   # On node B, try to connect
   # (this happens automatically, but can force reconnect)
   ./i2plan stop && ./i2plan start
   ```

3. **Check WireGuard keys match:**
   ```bash
   # Your public key
   ./i2plan identity show | grep "WireGuard"
   
   # Should match what peer sees
   # (on peer): ./i2plan peers list
   ```

### Peer Connection Drops Frequently

**Symptoms:**
- Peers connect then disconnect
- Intermittent connectivity

**Diagnosis:**
```bash
# Check I2P tunnel stability
# Visit http://127.0.0.1:7657 → Graphs
# Look for tunnel build failures

# Check logs for errors
grep -i "disconnect\|timeout\|error" ~/.i2plan/logs/i2plan.log
```

**Solutions:**

1. **I2P tunnel instability:**
   - Increase tunnel quantity in I2P console
   - Ensure good network connectivity
   - Check firewall isn't blocking I2P

2. **Increase heartbeat intervals:**
   Edit `config.toml`:
   ```toml
   [mesh]
   heartbeat_interval = "60s"  # Increase from 30s
   peer_timeout = "15m"        # Increase from 10m
   ```

3. **Check system resources:**
   ```bash
   # Linux/macOS
   top
   # Watch CPU and memory usage
   
   # Windows
   # Task Manager → Performance
   ```

---

## Performance Issues

### High Latency

**Expected:** I2P adds 50-200ms latency vs. direct connection.

**Symptoms:**
- Latency >500ms
- Slow response times

**Diagnosis:**
```bash
# Test latency to peer
ping -c 10 10.42.0.2  # Replace with peer IP

# Check I2P tunnel latency
# Visit http://127.0.0.1:7657 → Network → Peers
```

**Solutions:**

1. **Reduce I2P tunnel length:**
   Edit `config.toml`:
   ```toml
   [i2p]
   tunnel_length = 0  # Minimum (zero-hop)
   ```
   **Note:** This reduces anonymity but increases speed.

2. **Use faster I2P router settings:**
   - Router console → Configure → Bandwidth
   - Set bandwidth limit higher if you have fast internet

3. **Check I2P participating traffic:**
   - Router console → Configure → Bandwidth
   - Reduce participating bandwidth allocation

### Low Throughput

**Expected:** I2P throughput typically 1-10 Mbps.

**Symptoms:**
- Very slow file transfers
- Downloads taking forever

**Solutions:**

1. **This is normal for I2P.** The network prioritizes anonymity over speed.

2. **Optimize I2P settings:**
   - Increase bandwidth limits in I2P console
   - Increase tunnel quantity (3-4 tunnels)

3. **For high-bandwidth needs, I2P may not be suitable.**
   Consider direct VPN for large transfers.

### High CPU Usage

**Symptoms:**
- i2plan process using >50% CPU
- System becomes sluggish

**Diagnosis:**
```bash
# Linux/macOS
top
# Press 'P' to sort by CPU
# Look for i2plan process

# Windows
# Task Manager → Details tab → Sort by CPU
```

**Solutions:**

1. **Normal during tunnel building:**
   - High CPU for first 1-2 minutes is normal
   - Should stabilize after tunnels are built

2. **Check for gossip storm:**
   ```bash
   # Look for excessive gossip messages
   grep "gossip" ~/.i2plan/logs/i2plan.log | tail -n 100
   ```

3. **Increase gossip intervals:**
   Edit `config.toml`:
   ```toml
   [mesh]
   gossip_interval = "60s"  # Increase from 30s
   ```

### High Memory Usage

**Symptoms:**
- i2plan using >500MB RAM
- System running out of memory

**Solutions:**

1. **Reduce max peers:**
   Edit `config.toml`:
   ```toml
   [mesh]
   max_peers = 20  # Reduce from 50
   ```

2. **Check for memory leak:**
   ```bash
   # Monitor memory over time
   watch -n 10 'ps aux | grep i2plan'
   
   # If memory keeps growing, file a bug report
   ```

3. **Restart i2plan periodically:**
   ```bash
   # Add to cron (Linux/macOS)
   # Daily restart at 3 AM
   0 3 * * * systemctl restart i2plan
   ```

---

## Configuration Issues

### "Config file not found"

**Symptoms:**
- Error on startup: `config file not found`

**Solution:**
Create config file in the expected location:

```bash
# Linux/BSD/macOS
mkdir -p ~/.i2plan
cp config.minimal.toml ~/.i2plan/config.toml

# Windows (PowerShell)
mkdir $env:APPDATA\i2plan
Copy-Item config.minimal.toml $env:APPDATA\i2plan\config.toml
```

Or specify config location:
```bash
./i2plan start --config /path/to/config.toml

# Or via environment variable
export I2PLAN_CONFIG=/path/to/config.toml
./i2plan start
```

### "Invalid configuration"

**Symptoms:**
- Error: `invalid configuration`
- Specific field errors

**Solution:**
Validate your config file:

```bash
# Check TOML syntax
# Linux/macOS (if you have toml-cli)
toml-cli validate ~/.i2plan/config.toml

# Manual check - look for:
# - Missing quotes around strings
# - Invalid section names
# - Typos in field names
```

Common issues:
```toml
# WRONG - no quotes
name = my-node

# CORRECT
name = "my-node"

# WRONG - invalid section
[mesh_configuration]

# CORRECT
[mesh]

# WRONG - invalid field name
maximum_peers = 50

# CORRECT
max_peers = 50
```

Use example configs as reference:
- `config.minimal.toml` - Quick start
- `config.example.toml` - All options documented
- `config.production.toml` - Production settings

---

## Build and Installation Issues

### "Go version too old"

**Symptoms:**
- Error: `requires go 1.21 or later`

**Solution:**
```bash
# Check current version
go version

# Update Go
# Linux
wget https://go.dev/dl/go1.24.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.24.linux-amd64.tar.gz

# macOS
brew upgrade go

# Windows
# Download and install from https://go.dev/dl/
```

### "Cannot find package"

**Symptoms:**
- Build fails with `cannot find package`
- Missing dependencies

**Solution:**
```bash
# Download dependencies
go mod download

# Verify go.mod and go.sum exist
ls -la go.mod go.sum

# If missing, initialize
go mod tidy

# Rebuild
go build -o i2plan cmd/i2plan/main.go
```

### Build succeeds but binary doesn't run

**Symptoms:**
- Binary created but won't execute
- "Permission denied" or "File not found"

**Solution:**

**Linux/macOS/BSD:**
```bash
# Make executable
chmod +x i2plan

# If "File not found" on Linux, check architecture
file i2plan
uname -m
# Should match (e.g., both x86_64 or both aarch64)

# Rebuild for correct architecture
GOARCH=amd64 go build -o i2plan cmd/i2plan/main.go  # For x86_64
GOARCH=arm64 go build -o i2plan cmd/i2plan/main.go  # For ARM64
```

**Windows:**
```powershell
# Ensure .exe extension
go build -o i2plan.exe cmd\i2plan\main.go

# Run
.\i2plan.exe version
```

---

## Platform-Specific Issues

### Linux: "iptables: no chain/target/match by that name"

**Symptoms:**
- Error related to iptables during startup

**Solution:**
```bash
# Update iptables
sudo apt install iptables

# Or use nftables
sudo apt install nftables

# Check iptables is loaded
sudo iptables -L -n
```

### macOS: "Developer cannot be verified"

**Symptoms:**
- macOS blocks i2plan from running
- "Unidentified developer" warning

**Solution:**
```bash
# Remove quarantine attribute
xattr -d com.apple.quarantine i2plan

# Or allow in System Preferences:
# Security & Privacy → General → "Allow Anyway"
```

### macOS: "Library not loaded"

**Symptoms:**
- Error about missing dylib files

**Solution:**
```bash
# Reinstall dependencies
brew reinstall wireguard-tools go

# Check for broken symlinks
brew doctor
```

### Windows: "vcruntime140.dll missing"

**Symptoms:**
- DLL not found errors

**Solution:**
1. Install Visual C++ Redistributable
2. Download from: https://aka.ms/vs/17/release/vc_redist.x64.exe
3. Run installer
4. Restart system

### Windows: "The system cannot find the path specified"

**Symptoms:**
- Path errors in PowerShell

**Solution:**
```powershell
# Use full paths
cd C:\Users\YourName\wireguard
.\i2plan.exe start

# Or add to PATH
$env:PATH += ";C:\Users\YourName\wireguard"
```

### FreeBSD: "if_wg.ko not found"

**Symptoms:**
- WireGuard kernel module missing

**Solution:**
```bash
# Install kernel module
sudo pkg install wireguard-kmod

# Load module
sudo kldload if_wg

# Add to boot loader
echo 'if_wg_load="YES"' | sudo tee -a /boot/loader.conf
```

### OpenBSD: "pledge: operation not permitted"

**Symptoms:**
- pledge() system call restrictions

**Solution:**
OpenBSD's security features may restrict operations. Run with proper permissions:
```bash
doas ./i2plan start
```

Or install as rc.d service.

---

## Getting Debug Information

### Enable Debug Logging

**Method 1: Environment variable**
```bash
# Linux/macOS/BSD
DEBUG_I2P=debug ./i2plan start

# Windows (PowerShell)
$env:DEBUG_I2P="debug"
.\i2plan.exe start
```

**Method 2: Configuration file**
Edit `config.toml`:
```toml
[logging]
level = "debug"  # Options: error, warn, info, debug
```

### Collect Diagnostic Information

```bash
# System information
uname -a  # Linux/macOS/BSD
systeminfo  # Windows

# i2plan version
./i2plan version

# Configuration (hide sensitive data!)
cat ~/.i2plan/config.toml

# I2P status
curl -I http://127.0.0.1:7657
nc -zv 127.0.0.1 7656

# WireGuard status
sudo wg show  # Linux/BSD/macOS
# (Windows: as Administrator)

# Recent logs
tail -n 200 ~/.i2plan/logs/i2plan.log

# Network interfaces
ip addr  # Linux
ifconfig  # BSD/macOS
ipconfig  # Windows

# Process information
ps aux | grep i2plan  # Linux/macOS/BSD
Get-Process | Where-Object {$_.Name -eq "i2plan"}  # Windows
```

### Profile Performance Issues

```bash
# CPU profiling
./i2plan start --cpuprofile=cpu.prof

# Memory profiling
./i2plan start --memprofile=mem.prof

# Analyze profiles
go tool pprof cpu.prof
go tool pprof mem.prof
```

### Report a Bug

When reporting issues, include:

1. **Platform**: OS and version (`uname -a` or `systeminfo`)
2. **i2plan version**: `./i2plan version`
3. **Go version**: `go version`
4. **I2P version**: Check router console
5. **Configuration**: Your `config.toml` (hide private keys!)
6. **Logs**: Last 200 lines with debug logging enabled
7. **Steps to reproduce**
8. **Expected vs actual behavior**

Submit to: https://github.com/go-i2p/wireguard/issues

---

## Common Error Messages

| Error Message | Cause | Solution |
|---------------|-------|----------|
| `permission denied` | Need elevated privileges | Use sudo or setcap (Linux) |
| `connection refused` (7656) | SAM bridge not enabled | Enable in I2P console |
| `connection refused` (7657) | I2P router not running | Start I2P router |
| `tunnel not ready` | I2P tunnels still building | Wait 1-3 minutes |
| `invalid invite code` | Malformed or expired invite | Get new invite |
| `failed to create interface` | WireGuard issue | Check WireGuard installation |
| `config file not found` | Missing config.toml | Create config file |
| `bind: address already in use` | Port conflict | Change listen port or stop conflicting service |
| `no route to host` | Peer unreachable | Check I2P connectivity |
| `handshake timeout` | WireGuard can't connect | Check WireGuard keys match |

---

## Still Having Issues?

1. **Check documentation**:
   - [README.md](README.md) - Overview and quick start
   - [DEPLOYMENT.md](DEPLOYMENT.md) - Detailed setup instructions
   - Architecture Decision Records in `docs/adr/`

2. **Search existing issues**:
   - https://github.com/go-i2p/wireguard/issues

3. **Ask for help**:
   - GitHub Issues (preferred for bugs)
   - I2P Forum: https://i2pforum.net/
   - Matrix: #i2p-dev:matrix.org

4. **Commercial support**:
   - Contact repository maintainers for professional support options

Remember: Include debug logs and system information when asking for help!
