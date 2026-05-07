# Exit Node Configuration Guide

This guide explains how to configure and use exit nodes in the i2plan mesh VPN. Exit nodes allow mesh members to route their internet traffic through designated nodes, similar to traditional VPN services.

## Table of Contents

1. [Overview](#overview)
2. [System Requirements](#system-requirements)
3. [Exit Node Configuration](#exit-node-configuration)
4. [Upstream VPN Configuration (Double-VPN)](#upstream-vpn-configuration-double-vpn)
5. [Exit Client Configuration](#exit-client-configuration)
6. [Security Considerations](#security-considerations)
7. [Performance Tuning](#performance-tuning)
8. [Monitoring and Metrics](#monitoring-and-metrics)
9. [Troubleshooting](#troubleshooting)

---

## Overview

### What is an Exit Node?

An exit node is a mesh peer that forwards internet traffic for other members. When you configure a node as an exit:

- It enables IP forwarding on the system
- It sets up Network Address Translation (NAT) for mesh traffic
- It advertises its availability to other mesh members via the gossip protocol
- It tracks metrics (bandwidth, connected clients, etc.)

### What is an Exit Client?

An exit client is a mesh peer that routes all its internet traffic through a designated exit node. When configured as an exit client:

- All internet traffic is routed through the WireGuard mesh interface
- DNS queries can be routed through the exit node to prevent leaks
- A "kill switch" can block traffic if the exit connection fails
- Specific local routes can be excluded (e.g., printers, file shares)

### Use Cases

- **Privacy**: Route traffic through a trusted peer in a different location
- **Censorship Bypass**: Access restricted content through a peer in another region
- **Network Testing**: Test services from different network perspectives
- **Development**: Simulate different network conditions or locations

---

## System Requirements

### Exit Node Requirements

**Operating System Support:**
- Linux (kernel 3.10+, recommended 4.19+)
- macOS 10.15+
- Windows 10/11 (with administrator privileges)
- FreeBSD 12+, OpenBSD 6.9+

**Privileges:**
- **Linux**: `CAP_NET_ADMIN` capability or root access
- **macOS**: Root access (`sudo`)
- **Windows**: Administrator privileges
- **BSD**: Root access or `doas` on OpenBSD

**System Tools:**

Linux requires **one of**:
- `iptables` (traditional, widely available)
- `nftables` (modern, kernel 3.13+)

Check if tools are available:
```bash
# Check for iptables
which iptables

# Check for nftables
which nft

# Check kernel modules (Linux)
lsmod | grep -E 'ip_tables|nf_tables'
```

**Network:**
- Public IP address or port forwarding capability (for optimal routing)
- Adequate bandwidth for expected client load
- Stable internet connection

### Exit Client Requirements

**Operating System Support:**
- Same as exit node requirements

**Privileges:**
- **Linux**: `CAP_NET_ADMIN` for route manipulation
- **macOS**: Root access for route changes
- **Windows**: Administrator for route modification
- **BSD**: Root access for routing table changes

---

## Exit Node Configuration

### Basic Exit Node Setup

Add the `[ExitNode]` section to your configuration file (e.g., `~/.i2plan/config.toml`):

```toml
[ExitNode]
# Enable exit node functionality
Enabled = true

# Network interface with internet access (required)
# Use 'ip addr' (Linux/BSD) or 'ifconfig' (macOS) to find your interface
PublicInterface = "eth0"  # Common: eth0, ens3, wlan0, en0

# Whitelist of allowed client peer IDs (optional)
# Empty list = all mesh members can use this exit
AllowedClients = []

# Bandwidth limit in Mbps (optional)
# 0 = unlimited, useful for preventing abuse
BandwidthLimitMbps = 0

# Log client connections for debugging (optional)
LogConnections = true
```

### Finding Your Public Interface

**Linux:**
```bash
# List all interfaces
ip addr show

# Find interface with internet access (look for one with a public/private IP and default route)
ip route show default

# Example output: "default via 192.168.1.1 dev eth0"
# In this case, use "eth0"
```

**macOS:**
```bash
# List interfaces
ifconfig

# Find default route
netstat -rn | grep default

# Example: "default  192.168.1.1  UGSc  en0"
# Use "en0"
```

**Windows:**
```powershell
# List interfaces
ipconfig /all

# Use the adapter name with internet access, typically:
# - "Ethernet" for wired
# - "Wi-Fi" for wireless
```

### Advanced Exit Node Configuration

```toml
[ExitNode]
Enabled = true
PublicInterface = "eth0"

# Whitelist specific clients by their mesh peer ID
# Find peer IDs with: i2plan rpc peers list
AllowedClients = [
    "peer-abc123def456",
    "peer-789xyz012uvw"
]

# Limit bandwidth to 100 Mbps
BandwidthLimitMbps = 100

# Enable detailed connection logging
LogConnections = true
```

### Starting the Exit Node

```bash
# Start with elevated privileges
sudo i2plan start

# Or grant CAP_NET_ADMIN (Linux only, persistent)
sudo setcap cap_net_admin=+ep $(which i2plan)
i2plan start

# Check exit node status
i2plan rpc exit-node status
```

### Verifying Exit Node Setup

```bash
# Check if IP forwarding is enabled
# Linux/BSD:
sysctl net.ipv4.ip_forward
# Should output: net.ipv4.ip_forward = 1

# macOS:
sysctl net.inet.ip.forwarding
# Should output: net.inet.ip.forwarding: 1

# Check NAT rules (Linux with iptables):
sudo iptables -t nat -L -n -v

# You should see MASQUERADE rule like:
# MASQUERADE  all  --  *  eth0  0.0.0.0/0  0.0.0.0/0

# Check NAT rules (Linux with nftables):
sudo nft list ruleset

# Check if exit node is advertising
i2plan rpc peers list
# Look for your node with ExitNode: true
```

---

## Upstream VPN Configuration (Double-VPN)

Exit nodes can now route mesh traffic through an upstream commercial VPN provider, creating a "double-VPN" configuration for enhanced privacy and routing flexibility.

### Overview

When an exit node is configured with an upstream VPN:
- Mesh traffic is routed through: `Client → I2P → Exit Node → Upstream VPN → Internet`
- Provides three layers of encryption: I2P + WireGuard + Upstream VPN
- Exit node advertises available routes (direct and VPN) via gossip protocol
- Clients can select preferred routes based on requirements
- Automatic health monitoring ensures VPN connectivity

### Upstream VPN Detection

The exit node can automatically detect upstream VPN interfaces:

```toml
[ExitNode]
Enabled = true
PublicInterface = "eth0"

# Automatic VPN detection (recommended)
AutoDetectVPN = true

# Fallback behavior when VPN disconnects
FallbackBehavior = "direct"  # Options: "direct" (default), "block"
```

**Auto-detection recognizes these VPN providers:**
- **Mullvad**: wg-mullvad, wg-se1, wg-us1, etc.
- **ProtonVPN**: proton0, protonvpn0
- **NordVPN**: nordlynx, nordvpn
- **IVPN**: ivpn0, tun-ivpn
- **TunnelBear**: tun0 (with specific patterns)
- **Generic WireGuard**: wg0-wg9
- **Generic OpenVPN**: tun0-tun9, tap0-tap4

### Manual VPN Configuration

For better control, specify the exact VPN interface:

```toml
[ExitNode]
Enabled = true
PublicInterface = "eth0"

# Specify upstream VPN interface manually
UpstreamVPN = "wg-mullvad"

# Auto-detect disabled when interface explicitly set
AutoDetectVPN = false

# Block traffic if VPN goes down (recommended for privacy)
FallbackBehavior = "block"
```

### Popular VPN Provider Examples

#### Mullvad WireGuard

```toml
[ExitNode]
Enabled = true
PublicInterface = "eth0"
UpstreamVPN = "wg-mullvad"  # Mullvad's interface name
FallbackBehavior = "block"   # Block if Mullvad disconnects

# Mullvad uses wg-quick config at /etc/wireguard/wg-mullvad.conf
# Start Mullvad before i2plan: sudo wg-quick up wg-mullvad
```

#### ProtonVPN

```toml
[ExitNode]
Enabled = true
PublicInterface = "eth0"
UpstreamVPN = "proton0"      # ProtonVPN's interface
FallbackBehavior = "block"

# ProtonVPN CLI: protonvpn-cli connect
# Then start i2plan
```

#### NordVPN

```toml
[ExitNode]
Enabled = true
PublicInterface = "eth0"
UpstreamVPN = "nordlynx"     # NordVPN's WireGuard interface
FallbackBehavior = "block"

# NordVPN: nordvpn connect
# Then start i2plan
```

#### Generic WireGuard VPN

```toml
[ExitNode]
Enabled = true
PublicInterface = "eth0"
UpstreamVPN = "wg0"          # Your WireGuard interface
FallbackBehavior = "direct"  # Allow direct if VPN fails

# Start your VPN: sudo wg-quick up wg0
# Then start i2plan
```

#### OpenVPN

```toml
[ExitNode]
Enabled = true
PublicInterface = "eth0"
UpstreamVPN = "tun0"         # OpenVPN's tun interface
FallbackBehavior = "block"

# OpenVPN: sudo openvpn --config your-config.ovpn
# Then start i2plan
```

### Policy Routing (Linux Only)

**Note**: Policy routing for upstream VPN is currently **Linux-only**. macOS, Windows, and BSD support is planned for future releases.

On Linux, i2plan automatically configures policy routing to direct mesh traffic through the upstream VPN:

```bash
# i2plan creates:
# 1. Custom routing table (ID 100) for mesh traffic
# 2. Route rule directing mesh traffic to VPN interface
# 3. Cleanup on shutdown

# View policy routing (after i2plan starts):
ip rule show
# Output includes: "from all iif wg0 lookup 100"

ip route show table 100
# Output shows: "default dev wg-mullvad"
```

**Platform Limitations:**
- **Linux**: Full policy routing support ✅
- **macOS**: Policy routing planned (currently returns "not implemented")
- **Windows**: Policy routing planned (currently returns "not implemented")
- **BSD**: Policy routing planned (currently returns "not implemented")

### VPN Health Checks

Exit nodes automatically monitor upstream VPN health every 30 seconds:

```bash
# Health check verifies:
# 1. VPN interface exists
# 2. Interface is in "UP" state
# 3. Interface has assigned IP addresses

# View health check logs
journalctl -u i2plan -f | grep -i "vpn"

# Example logs:
# INFO exit_node: VPN health check passed interface=wg-mullvad
# WARN exit_node: VPN interface down, using fallback interface=wg-mullvad
```

Health check failures trigger fallback behavior:
- `FallbackBehavior = "direct"`: Routes traffic directly through PublicInterface
- `FallbackBehavior = "block"`: Stops forwarding mesh traffic (privacy mode)

### Route Advertisement

Exit nodes with upstream VPNs advertise multiple routes to clients:

```bash
# Example advertisement from exit node:
{
  "available_routes": [
    {
      "name": "direct",
      "priority": 100,
      "bandwidth": 100,  # Mbps
      "latency": 50      # ms
    },
    {
      "name": "mullvad-sweden",
      "priority": 50,
      "bandwidth": 50,
      "latency": 100
    }
  ],
  "upstream_vpn": {
    "provider": "mullvad",
    "country": "SE",
    "verified": true
  }
}

# Clients can select "direct" or "mullvad-sweden" route
```

### Verifying Upstream VPN Setup

```bash
# 1. Start your upstream VPN first
sudo wg-quick up wg-mullvad  # Example for Mullvad

# 2. Verify VPN interface is up
ip addr show wg-mullvad
# Should show interface with IP address and "UP" state

# 3. Start i2plan with exit node enabled
sudo i2plan start

# 4. Check exit node status
i2plan rpc exit-node status
# Should show upstream VPN detected

# 5. Verify policy routing (Linux)
ip rule show | grep wg0
# Should show rule directing mesh traffic

# 6. Check advertisement (from another mesh peer)
i2plan rpc peers list
# Should show your exit node with VPN routes advertised
```

### Upstream VPN Workflows

#### Start Sequence

```bash
# 1. Start upstream VPN
sudo wg-quick up wg-mullvad

# 2. Verify VPN is connected
ping -c 3 -I wg-mullvad 1.1.1.1

# 3. Start i2plan
sudo i2plan start

# 4. Verify exit node initialized
i2plan rpc exit-node status
```

#### Stop Sequence

```bash
# 1. Stop i2plan (cleans up routing rules)
sudo i2plan stop

# 2. Stop upstream VPN
sudo wg-quick down wg-mullvad
```

#### Automatic Startup (systemd)

```bash
# Make upstream VPN start before i2plan
sudo systemctl edit i2plan.service

# Add dependency:
[Unit]
After=wg-quick@wg-mullvad.service
Requires=wg-quick@wg-mullvad.service

# Enable both services
sudo systemctl enable wg-quick@wg-mullvad
sudo systemctl enable i2plan

# Start/stop together
sudo systemctl start i2plan  # Starts Mullvad first automatically
```

---

## Exit Client Configuration

### Basic Exit Client Setup

Add the `[ExitClient]` section to your configuration:

```toml
[ExitClient]
# Enable exit client mode
Enabled = true

# Specific exit node ID to use (optional)
# Leave empty for automatic selection based on load/latency
ExitNodeID = ""

# Block all traffic if exit node is unavailable (recommended)
KillSwitch = true

# DNS servers to use (prevents DNS leaks)
# Leave empty to use exit node's DNS
DNSServers = []

# Local network routes to exclude from exit routing
# Useful for accessing local printers, file shares, etc.
ExcludeRoutes = [
    "192.168.1.0/24",    # Local LAN
    "10.0.0.0/8",        # Private networks
]
```

### Automatic Exit Node Selection

When `ExitNodeID` is empty, the client automatically selects the best exit node based on:

1. **Current Load**: Nodes with lower CPU/memory usage preferred
2. **Active Clients**: Nodes with fewer clients preferred
3. **Latency**: Nodes with lower ping times preferred (when available)
4. **Bandwidth**: Nodes with higher available bandwidth preferred

```toml
[ExitClient]
Enabled = true
ExitNodeID = ""  # Automatic selection
KillSwitch = true
```

### Manual Exit Node Selection

To use a specific exit node, find its peer ID and configure it:

```bash
# List available exit nodes
i2plan rpc exit-nodes list

# Example output:
# peer-abc123  Load: 0.15  Clients: 3  Bandwidth: 1000 Mbps
# peer-xyz789  Load: 0.45  Clients: 8  Bandwidth: 500 Mbps

# Configure specific node
```

Then update your config:

```toml
[ExitClient]
Enabled = true
ExitNodeID = "peer-abc123"  # Specific node
KillSwitch = true
```

### Route Selection

**New in Phase 2**: Clients can now select specific routes based on preferences when exit nodes advertise multiple options (direct and VPN-backed).

#### Preferred Route Selection

Request a specific route by name:

```toml
[ExitClient]
Enabled = true

# Request specific route (e.g., "mullvad-sweden", "protonvpn-us", "direct")
PreferredRoute = "mullvad-sweden"

# Fallback to any route if preferred unavailable
RequireVPN = false
```

**Route Name Examples:**
- `"direct"` - Direct routing through exit node's public interface
- `"mullvad-sweden"` - Traffic routed through Mullvad VPN in Sweden
- `"protonvpn-us"` - Traffic routed through ProtonVPN in US
- `"nordvpn-uk"` - Traffic routed through NordVPN in UK

#### Require VPN-Backed Routes

Mandate VPN-backed exit nodes for maximum privacy:

```toml
[ExitClient]
Enabled = true

# Only use exit nodes with upstream VPNs
RequireVPN = true

# Optional: prefer specific VPN route
PreferredRoute = "mullvad-sweden"

# Block traffic if no VPN routes available (recommended)
KillSwitch = true
```

When `RequireVPN = true`:
- Client filters out exit nodes without upstream VPNs
- Only routes through exit nodes advertising VPN backing
- Returns error if no VPN-backed exit nodes available
- Provides triple encryption: I2P + WireGuard + Upstream VPN

#### Route Selection Priority

When multiple exit nodes match preferences:

1. **Exact Match**: PreferredRoute name matches exactly (highest priority)
2. **VPN Requirement**: RequireVPN filters non-VPN routes
3. **Load Balancing**: Lower load preferred (fewer active clients)
4. **Route Priority**: Higher route priority value preferred
5. **Bandwidth**: Higher bandwidth routes preferred

```toml
[ExitClient]
Enabled = true

# Priority order:
# 1. Exit nodes advertising "mullvad-sweden" route
# 2. Must have VPN backing
# 3. Select node with lowest current load
PreferredRoute = "mullvad-sweden"
RequireVPN = true
```

#### Example Configurations

**Maximum Privacy (Triple Encryption):**
```toml
[ExitClient]
Enabled = true
RequireVPN = true         # Must use VPN-backed exit
PreferredRoute = ""       # Any VPN provider
KillSwitch = true         # Block if connection fails
DNSServers = ["1.1.1.1"]  # Encrypted DNS
```

**Specific VPN Provider:**
```toml
[ExitClient]
Enabled = true
RequireVPN = true
PreferredRoute = "mullvad-sweden"  # Prefer Mullvad Sweden
# Falls back to other VPN routes if Mullvad unavailable
KillSwitch = true
```

**Direct Routing (No VPN):**
```toml
[ExitClient]
Enabled = true
RequireVPN = false
PreferredRoute = "direct"  # Explicitly request direct routing
KillSwitch = true
```

**Automatic Selection:**
```toml
[ExitClient]
Enabled = true
# No preferences - automatic load-based selection
# May select VPN or direct routes based on availability
KillSwitch = true
```

### Kill Switch Configuration

The kill switch prevents traffic from bypassing the exit node if the connection fails:

```toml
[ExitClient]
Enabled = true
KillSwitch = true  # Recommended for privacy

# When enabled:
# - If exit connection fails, all internet traffic is blocked
# - Only local network traffic (ExcludeRoutes) is allowed
# - Prevents accidental exposure of your real IP
```

**Security Recommendation**: Always enable the kill switch unless you need graceful degradation.

### DNS Leak Prevention

Configure custom DNS servers to prevent DNS leaks:

```toml
[ExitClient]
Enabled = true
DNSServers = [
    "1.1.1.1",      # Cloudflare
    "8.8.8.8",      # Google
]

# Alternative: Use privacy-focused DNS
DNSServers = [
    "9.9.9.9",      # Quad9
    "149.112.112.112",  # Quad9 secondary
]

# Or use exit node's DNS (leave empty)
DNSServers = []
```

### Excluding Local Routes

Prevent local network traffic from going through the exit:

```toml
[ExitClient]
Enabled = true
ExcludeRoutes = [
    "192.168.1.0/24",      # Your home network
    "10.0.0.0/8",          # Docker/VMs
    "172.16.0.0/12",       # More private networks
    "192.168.122.0/24",    # KVM default network
]

# Excluded routes bypass the exit node and use your normal gateway
```

### Starting the Exit Client

```bash
# Start i2plan with elevated privileges
sudo i2plan start

# Verify exit mode is active
i2plan rpc exit-client status

# Check your public IP (should show exit node's IP)
curl ifconfig.me

# Test DNS (should show configured DNS servers)
cat /etc/resolv.conf  # Linux/macOS
```

---

## Security Considerations

### Exit Node Operator Responsibilities

**You are responsible for the traffic exiting your node:**

1. **Legal Liability**: Traffic exits with your IP address
2. **Abuse Prevention**: Configure `BandwidthLimitMbps` to prevent abuse
3. **Client Filtering**: Use `AllowedClients` to restrict access to trusted peers
4. **Monitoring**: Enable `LogConnections` to track usage
5. **ToS Compliance**: Ensure your ISP allows hosting VPN services

**Best Practices:**

```toml
[ExitNode]
Enabled = true
PublicInterface = "eth0"

# Restrict to known peers only
AllowedClients = [
    "trusted-peer-1",
    "trusted-peer-2",
]

# Limit bandwidth to reasonable amount
BandwidthLimitMbps = 100

# Log all connections
LogConnections = true
```

### Exit Client Security

**Privacy Risks:**

1. **Exit Node Trust**: The exit node can see your unencrypted traffic (same as any VPN)
2. **DNS Leaks**: Always configure `DNSServers` or verify exit node's DNS
3. **IPv6 Leaks**: Ensure IPv6 is disabled or routed through exit
4. **WebRTC Leaks**: WebRTC can expose your real IP in browsers

**Mitigation:**

```toml
[ExitClient]
Enabled = true
KillSwitch = true  # Critical for preventing leaks

# Use encrypted DNS
DNSServers = ["1.1.1.1", "8.8.8.8"]

# Exclude nothing unless necessary
ExcludeRoutes = []
```

**Browser Security:**

```bash
# Test for leaks
# 1. Check IP: https://ifconfig.me
# 2. Check DNS: https://www.dnsleaktest.com
# 3. Check WebRTC: https://browserleaks.com/webrtc

# Firefox: Disable WebRTC
# Navigate to about:config
# Set media.peerconnection.enabled = false

# Chrome: Use extension to disable WebRTC
# Search for "WebRTC Leak Prevent" in Chrome Web Store
```

### Firewall Recommendations

**Exit Node (iptables example):**

```bash
# Allow only mesh traffic to be forwarded
sudo iptables -A FORWARD -i wg0 -o eth0 -j ACCEPT
sudo iptables -A FORWARD -i eth0 -o wg0 -m state --state RELATED,ESTABLISHED -j ACCEPT

# Drop all other forwarding
sudo iptables -A FORWARD -j DROP

# Rate limit to prevent DoS
sudo iptables -A FORWARD -i wg0 -m limit --limit 100/s --limit-burst 200 -j ACCEPT
```

---

## Performance Tuning

### Exit Node Optimization

**Bandwidth Limits:**

```toml
[ExitNode]
BandwidthLimitMbps = 100  # Adjust based on your connection

# Calculate appropriate limit:
# - Check your upload speed: speedtest-cli --simple
# - Set limit to 70-80% of upload speed
# - Example: 150 Mbps upload -> set limit to 100-120
```

**System Tuning (Linux):**

```bash
# Increase network buffer sizes
sudo sysctl -w net.core.rmem_max=26214400
sudo sysctl -w net.core.wmem_max=26214400
sudo sysctl -w net.ipv4.tcp_rmem="10240 87380 26214400"
sudo sysctl -w net.ipv4.tcp_wmem="10240 87380 26214400"

# Enable TCP BBR congestion control (kernel 4.9+)
sudo sysctl -w net.core.default_qdisc=fq
sudo sysctl -w net.ipv4.tcp_congestion_control=bbr

# Make persistent
sudo tee -a /etc/sysctl.conf <<EOF
net.core.rmem_max=26214400
net.core.wmem_max=26214400
net.ipv4.tcp_rmem=10240 87380 26214400
net.ipv4.tcp_wmem=10240 87380 26214400
net.core.default_qdisc=fq
net.ipv4.tcp_congestion_control=bbr
EOF
```

**Connection Tracking:**

```bash
# Increase connection tracking table size (for many clients)
sudo sysctl -w net.netfilter.nf_conntrack_max=1000000
sudo sysctl -w net.netfilter.nf_conntrack_tcp_timeout_established=7200

# Make persistent
echo "net.netfilter.nf_conntrack_max=1000000" | sudo tee -a /etc/sysctl.conf
```

### Exit Client Optimization

**MTU Configuration:**

WireGuard over I2P has a maximum packet size of 31KB, but optimal MTU is lower:

```bash
# Check current MTU
ip link show wg0

# Set optimal MTU (1280 recommended for I2P)
sudo ip link set dev wg0 mtu 1280

# Or configure in i2plan
```

**DNS Performance:**

```toml
[ExitClient]
# Use fast, nearby DNS servers
DNSServers = [
    "1.1.1.1",          # Cloudflare (fast, privacy-focused)
    "1.0.0.1",          # Cloudflare backup
]

# Or run local DNS cache
# Install: dnsmasq, unbound, or systemd-resolved
# Then point to localhost:
DNSServers = ["127.0.0.1"]
```

### Performance Testing

**Bandwidth Test:**

```bash
# Install iperf3 on exit node and client
sudo apt-get install iperf3  # Debian/Ubuntu

# On exit node (after enabling exit mode)
iperf3 -s

# On exit client (after connecting to exit)
iperf3 -c <exit-node-public-ip>

# Expected throughput depends on:
# - I2P tunnel performance (typically 1-50 Mbps)
# - Exit node bandwidth limit
# - Network conditions
```

**Latency Test:**

```bash
# Test exit node latency (from client)
i2plan rpc exit-nodes list

# Test internet latency through exit
ping -c 100 8.8.8.8

# Compare with direct connection
# Higher latency is expected due to I2P routing
```

---

## Monitoring and Metrics

### Exit Node Metrics

Access metrics via RPC:

```bash
# Get current metrics
i2plan rpc exit-node metrics

# Example output:
# {
#   "active_clients": 3,
#   "bytes_forwarded": 15728640,
#   "bytes_received": 3932160,
#   "current_bandwidth_mbps": 12.5,
#   "dropped_packets": 0,
#   "average_latency": "45ms",
#   "uptime": "3h25m",
#   "load": 0.25
# }
```

**Metrics Explanation:**

- `active_clients`: Number of currently connected exit clients
- `bytes_forwarded`: Total bytes sent to clients
- `bytes_received`: Total bytes received from clients
- `current_bandwidth_mbps`: Instantaneous bandwidth usage
- `dropped_packets`: Packets dropped due to errors or limits
- `average_latency`: Average round-trip time to clients
- `uptime`: Time since exit node was started
- `load`: Calculated load (0.0-1.0), affects auto-selection

**Per-Connection Statistics:**

```bash
# Get detailed per-client stats
i2plan rpc exit-node connections

# Example output:
# [
#   {
#     "client_id": "peer-abc123",
#     "bytes_sent": 5242880,
#     "bytes_received": 1310720,
#     "packets_dropped": 0,
#     "latency_avg": "42ms",
#     "connected_at": "2026-02-02T10:15:30Z"
#   }
# ]
```

### Exit Client Health Monitoring

Monitor connection health:

```bash
# Check exit client status
i2plan rpc exit-client status

# Example output:
# {
#   "enabled": true,
#   "active": true,
#   "exit_node_id": "peer-abc123",
#   "exit_node_reachable": true,
#   "internet_accessible": true,
#   "dns_leak_detected": false,
#   "kill_switch_active": true
# }
```

**Health Checks:**

The exit client performs periodic health checks:

1. **Exit Node Reachability**: TCP connection to exit node (every 30s)
2. **Internet Connectivity**: HTTP request to reliable endpoints (every 30s)
3. **DNS Leak Detection**: Verifies DNS configuration (every 60s)

**Monitoring Loop:**

```bash
# Watch health status continuously
watch -n 5 'i2plan rpc exit-client status'

# Check logs for health check failures
tail -f ~/.i2plan/logs/i2plan.log | grep -i health
```

### Prometheus Metrics (Optional)

If you're using Prometheus for monitoring:

```toml
# Enable Prometheus metrics endpoint
[Metrics]
Enabled = true
Address = "127.0.0.1:9090"
```

Then scrape from Prometheus:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'i2plan'
    static_configs:
      - targets: ['localhost:9090']
```

---

## Troubleshooting

### Exit Node Issues

**Problem: Exit node fails to start with "permission denied"**

```bash
# Check current permissions
id

# Solution 1: Run with sudo
sudo i2plan start

# Solution 2: Grant CAP_NET_ADMIN (Linux)
sudo setcap cap_net_admin=+ep $(which i2plan)
i2plan start

# Solution 3: Run as root user
sudo su
i2plan start
```

**Problem: "iptables not found" or "nftables not found"**

```bash
# Check if iptables is installed (Debian/Ubuntu)
sudo apt-get install iptables

# Check if nftables is installed
sudo apt-get install nftables

# RHEL/CentOS/Fedora
sudo yum install iptables nftables

# macOS (not applicable, uses pfctl)
# Windows (not applicable, uses netsh)
```

**Problem: IP forwarding doesn't persist after reboot**

```bash
# Make IP forwarding permanent (Linux)
echo "net.ipv4.ip_forward = 1" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p

# macOS (requires each boot)
# Add to launch daemon or system service

# Windows (persistent by default when enabled)
```

**Problem: No clients can connect to exit node**

```bash
# Check if exit node is advertising
i2plan rpc peers list | grep -A 5 "$(hostname)"

# Should show: "exit_node": true

# Check AllowedClients configuration
cat ~/.i2plan/config.toml | grep -A 5 "\[ExitNode\]"

# If AllowedClients is set, add client peer IDs:
# AllowedClients = ["peer-abc123", "peer-xyz789"]

# Restart i2plan to apply changes
sudo i2plan restart
```

**Problem: High CPU usage on exit node**

```bash
# Check current metrics
i2plan rpc exit-node metrics

# If many clients, reduce bandwidth limit
# Edit config.toml:
# BandwidthLimitMbps = 50  # Lower value

# Or restrict clients
# AllowedClients = ["trusted-peer-1"]

# Restart to apply
sudo i2plan restart
```

### Exit Client Issues

**Problem: Exit client mode doesn't activate**

```bash
# Check if client is running with proper privileges
ps aux | grep i2plan

# Restart with sudo
sudo i2plan restart

# Check configuration
cat ~/.i2plan/config.toml | grep -A 10 "\[ExitClient\]"

# Ensure Enabled = true
# Check logs
tail -100 ~/.i2plan/logs/i2plan.log | grep -i exit
```

**Problem: Public IP doesn't change after enabling exit client**

```bash
# Check exit client status
i2plan rpc exit-client status

# If not active, check for available exit nodes
i2plan rpc exit-nodes list

# If no exit nodes, none are available in your mesh
# Ask a trusted peer to enable exit node

# If specific ExitNodeID set, ensure that node is online
i2plan rpc peers list | grep "peer-abc123"

# Try automatic selection instead
# Edit config.toml: ExitNodeID = ""
sudo i2plan restart

# Verify routing
ip route show  # Linux
netstat -rn    # macOS/BSD
route print    # Windows

# Should see default route pointing to wg0 interface
```

**Problem: DNS leaks detected**

```bash
# Test for DNS leaks
# Visit: https://www.dnsleaktest.com

# Check current DNS configuration
cat /etc/resolv.conf  # Linux/macOS

# Should show DNS servers from config.toml
# If not, check DNS manager status
i2plan rpc exit-client status | grep dns

# Manually set DNS servers
# Edit config.toml:
[ExitClient]
DNSServers = ["1.1.1.1", "8.8.8.8"]

# Restart i2plan
sudo i2plan restart

# Verify DNS
nslookup google.com
# Should use configured DNS servers
```

**Problem: Kill switch blocks all traffic**

```bash
# Check kill switch status
i2plan rpc exit-client status

# If exit node is unreachable, kill switch activates
# This is expected behavior for privacy

# Option 1: Wait for exit node to come back online
# Option 2: Disable kill switch temporarily
# Edit config.toml: KillSwitch = false
sudo i2plan restart

# Option 3: Disable exit client mode
# Edit config.toml: Enabled = false (under [ExitClient])
sudo i2plan restart

# After network restored, re-enable kill switch
```

**Problem: Can't access local network devices**

```bash
# Local devices need to be excluded from exit routing

# Edit config.toml:
[ExitClient]
ExcludeRoutes = [
    "192.168.1.0/24",    # Your local network
]

# Find your local network range
ip route  # Linux
netstat -rn  # macOS

# Add all local networks to ExcludeRoutes
sudo i2plan restart

# Test local access
ping 192.168.1.1  # Your router
```

### Upstream VPN Issues

**Problem: VPN interface not detected with AutoDetectVPN**

```bash
# Find your VPN interface name manually
ip addr show  # Linux/BSD
ifconfig      # macOS

# Look for tun/tap/wg interfaces with IP addresses
# Common patterns: tun0, wg0, wg-mullvad, proton0, nordlynx

# Configure manually instead of auto-detect:
# Edit config.toml:
[ExitNode]
UpstreamVPN = "wg-mullvad"  # Use exact interface name
AutoDetectVPN = false

# Restart
sudo i2plan restart
```

**Problem: VPN health checks failing**

```bash
# Check if VPN interface is actually up
ip link show wg-mullvad | grep "state UP"

# Ensure VPN has IP address assigned
ip addr show wg-mullvad

# Look for:
# 1. "UP" in flags: <UP,POINTOPOINT,RUNNING>
# 2. inet address: inet 10.64.0.2/32

# If interface down, start VPN first:
sudo wg-quick up wg-mullvad  # WireGuard
sudo openvpn --config vpn.ovpn  # OpenVPN
nordvpn connect  # NordVPN
protonvpn-cli connect  # ProtonVPN

# Then start i2plan
sudo i2plan start

# Check health check logs
journalctl -u i2plan -f | grep -i "health\|vpn"
```

**Problem: Policy routing not working (Linux)**

```bash
# Verify routing rules exist
ip rule show | grep "lookup 100"
# Should see: from all iif wg0 lookup 100

# Check custom routing table
ip route show table 100
# Should see: default dev wg-mullvad

# If missing, restart i2plan to recreate
sudo i2plan restart

# Or add manually (temporary):
sudo ip rule add from all iif wg0 table 100
sudo ip route add default dev wg-mullvad table 100

# Test routing
# Traffic from wg0 should go through wg-mullvad
```

**Problem: Policy routing not available on macOS/Windows/BSD**

```bash
# Policy routing is currently Linux-only
# Workarounds:

# Option 1: Use Linux for exit node
# Set up exit node on Linux machine

# Option 2: Wait for platform support
# macOS/Windows/BSD support planned for Phase 2.1

# Option 3: Manual system routing (advanced, not recommended)
# Configure routing tables manually outside i2plan
```

**Problem: Fallback behavior not activating**

```bash
# Check FallbackBehavior configuration
grep FallbackBehavior ~/.i2plan/config.toml

# Should show:
# FallbackBehavior = "direct"  # or "block"

# Test fallback by disconnecting VPN:
sudo wg-quick down wg-mullvad

# Watch logs for fallback activation:
journalctl -u i2plan -f | grep -i "fallback\|vpn"

# Should see: "VPN interface down, using fallback"

# If fallback = "direct": Traffic routes through PublicInterface
# If fallback = "block": Forwarding stops

# Reconnect VPN:
sudo wg-quick up wg-mullvad
```

### Client Route Selection Issues

**Problem: "No suitable exit node found" error**

```bash
# List available exit nodes and routes
i2plan rpc exit-nodes list

# Check if any exit nodes are available
# Check what routes they advertise

# If preferences too restrictive, relax:
# Edit config.toml:
[ExitClient]
RequireVPN = false       # Accept any exit node
PreferredRoute = ""      # No specific route

# Restart
sudo i2plan restart

# Wait for gossip propagation (10-30 seconds)
sleep 30 && i2plan rpc exit-nodes list
```

**Problem: Route selection ignores PreferredRoute**

```bash
# Check current route selection
i2plan rpc exit-client status

# Should show:
# ExitNodeID: peer-abc123
# RouteName: <route-name>

# Verify preferred route is available
i2plan rpc exit-nodes list | grep -A 10 "peer-abc123"

# Look for preferred route in available_routes

# If not found, route is unavailable
# Options:
# 1. Use different PreferredRoute
# 2. Remove PreferredRoute for automatic selection
# 3. Configure exit node to provide that route

# Edit config.toml with correct route name:
[ExitClient]
PreferredRoute = "mullvad-sweden"  # Must match exactly

sudo i2plan restart
```

**Problem: No VPN routes available with RequireVPN=true**

```bash
# Check if any exit nodes have upstream VPNs
i2plan rpc exit-nodes list | grep -i "vpn\|upstream"

# If no VPN-backed exit nodes:
# Option 1: Configure exit node with UpstreamVPN
# Option 2: Disable RequireVPN requirement

# Edit config.toml:
[ExitClient]
RequireVPN = false  # Accept direct routes too

sudo i2plan restart
```

### General Debugging

**Enable Debug Logging:**

```toml
# Edit config.toml
[Logging]
Level = "debug"  # Options: debug, info, warn, error

# Restart i2plan
```

**Check System Logs:**

```bash
# Linux (systemd)
sudo journalctl -u i2plan -f

# Linux (traditional)
tail -f /var/log/syslog | grep i2plan

# macOS
tail -f ~/Library/Logs/i2plan.log

# Windows
# Check Event Viewer -> Application logs
```

**Network Debugging:**

```bash
# Check WireGuard interface
ip link show wg0  # Linux
ifconfig wg0      # macOS/BSD

# Check routing table
ip route show     # Linux
netstat -rn       # macOS/BSD/Windows

# Test connectivity through exit
curl -v https://ifconfig.me

# Check firewall rules (Linux)
sudo iptables -L -n -v
sudo iptables -t nat -L -n -v

# Check firewall rules (macOS)
sudo pfctl -sr

# Packet capture
sudo tcpdump -i wg0 -n
```

---

## Additional Resources

- [PLAN.md](../PLAN.md) - Implementation details and design decisions
- [DEPLOYMENT.md](../DEPLOYMENT.md) - Platform-specific deployment guide
- [README.md](../README.md) - General project documentation
- [WireGuard Documentation](https://www.wireguard.com/) - Official WireGuard docs
- [I2P Documentation](https://geti2p.net/en/docs) - I2P network documentation

---

## Support

If you encounter issues not covered here:

1. Check GitHub Issues: https://github.com/go-i2p/wireguard/issues
2. Review existing troubleshooting discussions
3. Create a new issue with:
   - Operating system and version
   - i2plan version (`i2plan --version`)
   - Configuration file (redact sensitive data)
   - Relevant log excerpts
   - Steps to reproduce

---

**Security Notice**: Exit nodes can see unencrypted traffic. Only use exit nodes you trust, just as you would trust a commercial VPN provider. Enable the kill switch to prevent accidental IP leaks.
