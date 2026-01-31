# Deployment Guide

This guide covers deploying i2plan (WireGuard over I2P mesh VPN) on desktop computers and servers.

## Table of Contents

1. [Installation](#installation)
2. [Prerequisites](#prerequisites)
3. [Platform-Specific Setup](#platform-specific-setup)
   - [Linux](#linux)
   - [macOS](#macos)
   - [Windows](#windows)
   - [BSD (FreeBSD/OpenBSD)](#bsd-freebsdopenbsd)
4. [Building i2plan](#building-i2plan)
5. [Single-Node Setup](#single-node-setup)
6. [Multi-Node Mesh Setup](#multi-node-mesh-setup)
7. [Running as a Service](#running-as-a-service)
8. [Docker Deployment](#docker-deployment)
9. [Security Hardening](#security-hardening)
10. [Monitoring and Logging](#monitoring-and-logging)
11. [Maintenance](#maintenance)

---

## Installation

### Pre-built Packages

Download platform-specific packages from the [Releases](https://github.com/go-i2p/wireguard/releases) page:

**Linux:**
```bash
# Debian/Ubuntu
wget https://github.com/go-i2p/wireguard/releases/latest/download/i2plan_VERSION_amd64.deb
sudo dpkg -i i2plan_VERSION_amd64.deb

# RHEL/CentOS/Fedora
wget https://github.com/go-i2p/wireguard/releases/latest/download/i2plan-VERSION-1.x86_64.rpm
sudo rpm -i i2plan-VERSION-1.x86_64.rpm

# Verify installation
i2plan --version
```

**macOS:**
```bash
# Download and open DMG
wget https://github.com/go-i2p/wireguard/releases/latest/download/i2plan-VERSION-darwin-amd64.dmg
open i2plan-VERSION-darwin-amd64.dmg

# After installation, verify
/Applications/i2plan.app/Contents/MacOS/i2plan --version
```

**Windows:**
1. Download `i2plan-VERSION-windows-amd64.msi` from [Releases](https://github.com/go-i2p/wireguard/releases)
2. Double-click the MSI file to install
3. Installer will add i2plan to PATH
4. Verify in PowerShell:
```powershell
i2plan.exe --version
```

**Docker:**
```bash
# Pull latest image
docker pull ghcr.io/go-i2p/wireguard:latest

# Run with network capabilities
docker run --cap-add=NET_ADMIN ghcr.io/go-i2p/wireguard:latest --version
```

### From Source

If pre-built packages are unavailable for your platform, see [Building i2plan](#building-i2plan) below.

---

## Prerequisites

### System Requirements

- **RAM**: Minimum 512 MB, recommended 1 GB+
- **Disk**: 100 MB for application + space for logs
- **Network**: Internet connection (I2P handles NAT traversal, no port forwarding needed)

### Required Software

You'll need Go (for building), I2P router, and WireGuard installed on your system.

---

## Platform-Specific Setup

### Linux

#### 1. Install Go

```bash
# Download and install
wget https://go.dev/dl/go1.24.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version
```

#### 2. Install I2P Router

**Debian/Ubuntu:**
```bash
sudo apt-add-repository ppa:i2p-maintainers/i2p
sudo apt update
sudo apt install i2p
sudo systemctl start i2p
sudo systemctl enable i2p
```

**Fedora/RHEL:**
```bash
sudo dnf install i2p
sudo systemctl start i2p
sudo systemctl enable i2p
```

**Enable SAM Bridge:**
1. Open I2P console: http://127.0.0.1:7657
2. Go to: Configure → Clients
3. Enable "SAM application bridge" (port 7656)
4. Click Save and restart I2P

#### 3. TUN/TAP Support

**Note:** i2plan uses **wireguard-go** (userspace implementation), not kernel WireGuard. You don't need the kernel module.

```bash
# Verify TUN support (should already be present on modern Linux)
lsmod | grep tun

# If missing, load TUN module
sudo modprobe tun

# Load automatically on boot (optional)
echo "tun" | sudo tee -a /etc/modules
```

#### 4. WireGuard Permissions

**IMPORTANT**: WireGuard requires privileges to create network interfaces.

**Option A: Run with sudo (simplest for testing)**
```bash
sudo ./i2plan start
```

**Option B: Use capabilities (recommended)**
```bash
# Grant permission to i2plan binary
sudo setcap cap_net_admin=+ep ./i2plan

# Or if installed to /usr/local/bin
sudo setcap cap_net_admin=+ep /usr/local/bin/i2plan

# Now run without sudo
./i2plan start
```

**Option C: systemd service (recommended for always-on)**
See [Running as a Service](#running-as-a-service) section.

#### File Locations

- Config: `~/.i2plan/config.toml` or `/etc/i2plan/config.toml`
- Keys: `~/.i2plan/keys/`
- Logs: `~/.i2plan/logs/` (or journald if using systemd)
- Socket: `~/.i2plan/i2plan.sock`

---

### macOS

#### 1. Install Homebrew

```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

#### 2. Install Dependencies

**Note:** i2plan uses **wireguard-go** (userspace implementation) which is included in the Go build. You don't need to install WireGuard separately.

```bash
# Install Go and I2P
brew install go i2p

# Start I2P
i2prouter start

# Enable SAM bridge in I2P console
open http://127.0.0.1:7657
# Navigate to Configure → Clients → Enable "SAM application bridge"
```

#### 3. WireGuard Permissions

**Option A: Run with sudo**
```bash
sudo ./i2plan start
```

**Option B: launchd service (recommended)**
See [Running as a Service](#running-as-a-service) section.

#### File Locations

- Config: `~/Library/Application Support/i2plan/config.toml`
- Keys: `~/Library/Application Support/i2plan/keys/`
- Logs: `~/Library/Logs/i2plan/`
- Socket: `~/Library/Application Support/i2plan/i2plan.sock`

---

### Windows

#### 1. Install Go

1. Download installer from https://go.dev/dl/ (go1.24.windows-amd64.msi)
2. Run the installer
3. Open PowerShell and verify:
   ```powershell
   go version
   ```

#### 2. Install I2P Router

1. Download installer from https://geti2p.net/en/download
2. Run the installer
3. Start I2P from Start Menu → I2P → Start I2P
4. Open router console: http://127.0.0.1:7657
5. Navigate to Configure → Clients
6. Enable "SAM application bridge" (port 7656)
7. Click Save and restart I2P

#### 3. TUN/TAP Support

**Note:** i2plan uses **wireguard-go** (userspace implementation) which is included in the Go build. You don't need to install WireGuard separately.

Windows includes TUN/TAP support natively. No additional installation required.

#### 4. Administrator Permissions

**IMPORTANT**: WireGuard requires administrator privileges on Windows.

**Option A: Run PowerShell as Administrator**
```powershell
# Right-click PowerShell → "Run as Administrator"
cd C:\path\to\i2plan
.\i2plan.exe start
```

**Option B: Windows Service (recommended)**
See [Running as a Service](#running-as-a-service) section.

#### File Locations

- Config: `%APPDATA%\i2plan\config.toml` (typically `C:\Users\YourName\AppData\Roaming\i2plan\`)
- Keys: `%APPDATA%\i2plan\keys\`
- Logs: `%APPDATA%\i2plan\logs\`
- Named Pipe: `\\.\pipe\i2plan`

---

### BSD (FreeBSD/OpenBSD)

#### FreeBSD

**Note:** i2plan uses **wireguard-go** (userspace implementation) which is included in the Go build.

```bash
# Install packages
pkg install go i2p

# Verify TUN support (should be built-in)
kldstat | grep if_tun

# Start I2P
service i2p start
sysrc i2p_enable="YES"

# Enable SAM bridge
# Visit http://127.0.0.1:7657 → Configure → Clients → Enable "SAM"
```

**Permissions:**
```bash
# Run with sudo
sudo ./i2plan start

# Or install as service (see below)
```

#### OpenBSD

**Note:** i2plan uses **wireguard-go** (userspace implementation) which is included in the Go build, not the kernel WireGuard.

```bash
# Install packages
pkg_add go i2p

# Verify TUN support (built into OpenBSD)
ifconfig tun0 create  # Test
ifconfig tun0 destroy  # Cleanup

# Start I2P
rcctl enable i2pd
rcctl start i2pd

# Enable SAM bridge
# Visit http://127.0.0.1:7657 → Configure → Clients → Enable "SAM"
```

**Permissions:**
```bash
# Run with doas
doas ./i2plan start

# Or install as service (see below)
```

#### File Locations

- Config: `~/.i2plan/config.toml` or `/usr/local/etc/i2plan/config.toml`
- Keys: `~/.i2plan/keys/`
- Logs: `~/.i2plan/logs/` or `/var/log/i2plan/`
- Socket: `~/.i2plan/i2plan.sock` or `/var/run/i2plan/i2plan.sock`

---

## Building i2plan

```bash
# Clone repository
git clone https://github.com/go-i2p/wireguard.git
cd wireguard

# Build
go build -o i2plan cmd/i2plan/main.go

# Verify
./i2plan version

# Optional: Install system-wide
# Linux/BSD/macOS:
sudo cp i2plan /usr/local/bin/

# Windows (PowerShell as Administrator):
# Copy-Item i2plan.exe "C:\Program Files\i2plan\"
```

---

## Single-Node Setup

This is useful for testing or creating the first node in a mesh.

### 1. Create Configuration

```bash
# Linux/BSD/macOS
mkdir -p ~/.i2plan
cp config.minimal.toml ~/.i2plan/config.toml

# Windows (PowerShell)
mkdir $env:APPDATA\i2plan
Copy-Item config.minimal.toml $env:APPDATA\i2plan\config.toml
```

### 2. Edit Configuration

Edit `config.toml` with your preferred editor:

```toml
[node]
name = "my-first-node"

[i2p]
sam_address = "127.0.0.1:7656"
tunnel_length = 1  # Trusted mesh model

[mesh]
tunnel_subnet = "10.42.0.0/16"
max_peers = 50

[rpc]
enabled = true

[web]
enabled = true
listen = "127.0.0.1:8080"
```

### 3. Start Node

```bash
# With sudo/capabilities (see platform-specific setup)
./i2plan start

# Check logs
tail -f ~/.i2plan/logs/i2plan.log
```

### 4. Verify

Open web UI in browser: http://127.0.0.1:8080

Or use CLI:
```bash
./i2plan rpc status
```

---

## Multi-Node Mesh Setup

### Scenario: Connect 3 Computers

Let's connect three computers: **Alice** (Linux), **Bob** (macOS), **Carol** (Windows)

### Step 1: Alice Creates First Node

On Alice's computer:

```bash
# Build and start i2plan (with sudo or capabilities)
sudo ./i2plan start

# Generate invite for Bob
./i2plan invite create --label "Bob's Mac" --expires 1h

# Output will be:
# i2plan://invite/v1?d=eyJpbnZp...&s=mKE3V...
```

Copy this invite code and send it to Bob securely (Signal, email, etc.)

### Step 2: Bob Joins the Mesh

On Bob's computer:

```bash
# Build and start i2plan
sudo ./i2plan start

# Join using Alice's invite
./i2plan join "i2plan://invite/v1?d=eyJpbnZp...&s=mKE3V..."

# Verify connection
./i2plan peers list
# Should show Alice's node
```

### Step 3: Alice Invites Carol

Back on Alice's computer:

```bash
# Generate another invite for Carol
./i2plan invite create --label "Carol's Windows PC" --expires 1h
```

Send this invite to Carol.

### Step 4: Carol Joins the Mesh

On Carol's Windows PC (PowerShell as Administrator):

```powershell
# Build and start i2plan
.\i2plan.exe start

# Join using Alice's invite
.\i2plan.exe join "i2plan://invite/v1?d=..."

# Verify
.\i2plan.exe peers list
# Should show Alice and Bob
```

### Step 5: Verify Full Mesh

On any computer:

```bash
# List all peers
./i2plan peers list

# Show routes
./i2plan routes list

# Test connectivity (ping over WireGuard)
ping 10.42.0.2  # Example peer IP
```

All three nodes should now see each other and be able to communicate!

---

## Running as a Service

### Linux (systemd)

#### 1. Create System User (optional but recommended)

```bash
sudo useradd -r -s /bin/false i2plan
```

#### 2. Create Service File

Create `/etc/systemd/system/i2plan.service`:

```ini
[Unit]
Description=i2plan Mesh VPN
Documentation=https://github.com/go-i2p/wireguard
After=network.target i2p.service
Wants=i2p.service

[Service]
Type=simple
User=i2plan
Group=i2plan
WorkingDirectory=/var/lib/i2plan
ExecStart=/usr/local/bin/i2plan start
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/i2plan

# Environment
Environment=I2PLAN_CONFIG=/etc/i2plan/config.toml

[Install]
WantedBy=multi-user.target
```

#### 3. Set Up Directories

```bash
sudo mkdir -p /var/lib/i2plan /etc/i2plan
sudo chown i2plan:i2plan /var/lib/i2plan
sudo cp config.production.toml /etc/i2plan/config.toml
sudo chown root:i2plan /etc/i2plan/config.toml
sudo chmod 640 /etc/i2plan/config.toml
```

#### 4. Enable and Start

```bash
sudo systemctl daemon-reload
sudo systemctl enable i2plan
sudo systemctl start i2plan
sudo systemctl status i2plan
```

#### 5. View Logs

```bash
# Follow logs
sudo journalctl -u i2plan.service -f

# View recent logs
sudo journalctl -u i2plan.service -n 100
```

---

### macOS (launchd)

#### 1. Create Launch Daemon

Create `/Library/LaunchDaemons/net.i2p.i2plan.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>net.i2p.i2plan</string>
    
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/i2plan</string>
        <string>start</string>
    </array>
    
    <key>RunAtLoad</key>
    <true/>
    
    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>
    
    <key>StandardOutPath</key>
    <string>/usr/local/var/log/i2plan.log</string>
    
    <key>StandardErrorPath</key>
    <string>/usr/local/var/log/i2plan.error.log</string>
    
    <key>EnvironmentVariables</key>
    <dict>
        <key>I2PLAN_CONFIG</key>
        <string>/usr/local/etc/i2plan/config.toml</string>
    </dict>
</dict>
</plist>
```

#### 2. Set Permissions

```bash
sudo chown root:wheel /Library/LaunchDaemons/net.i2p.i2plan.plist
sudo chmod 644 /Library/LaunchDaemons/net.i2p.i2plan.plist
```

#### 3. Create Directories

```bash
sudo mkdir -p /usr/local/etc/i2plan /usr/local/var/log
sudo cp config.production.toml /usr/local/etc/i2plan/config.toml
```

#### 4. Load and Start

```bash
sudo launchctl load /Library/LaunchDaemons/net.i2p.i2plan.plist
sudo launchctl start net.i2p.i2plan

# Check status
sudo launchctl list | grep i2plan

# View logs
tail -f /usr/local/var/log/i2plan.log
```

---

### Windows (Windows Service)

#### Using NSSM (Non-Sucking Service Manager)

This is the easiest method for Windows.

#### 1. Download NSSM

- Download from https://nssm.cc/download
- Extract to `C:\nssm\`

#### 2. Install Service

Open PowerShell as Administrator:

```powershell
# Navigate to NSSM
cd C:\nssm\win64

# Install i2plan as service
.\nssm.exe install i2plan "C:\Program Files\i2plan\i2plan.exe" start

# Configure service
.\nssm.exe set i2plan AppDirectory "C:\Program Files\i2plan"
.\nssm.exe set i2plan AppStdout "C:\ProgramData\i2plan\logs\service.log"
.\nssm.exe set i2plan AppStderr "C:\ProgramData\i2plan\logs\error.log"
.\nssm.exe set i2plan AppRotateFiles 1
.\nssm.exe set i2plan AppRotateBytes 10485760

# Set to start automatically
.\nssm.exe set i2plan Start SERVICE_AUTO_START
```

#### 3. Create Log Directory

```powershell
mkdir C:\ProgramData\i2plan\logs
```

#### 4. Start Service

```powershell
Start-Service i2plan
Get-Service i2plan

# View logs
Get-Content C:\ProgramData\i2plan\logs\service.log -Tail 50 -Wait
```

---

### FreeBSD (rc.d)

#### 1. Create rc.d Script

Create `/usr/local/etc/rc.d/i2plan`:

```bash
#!/bin/sh
#
# PROVIDE: i2plan
# REQUIRE: NETWORKING i2p
# KEYWORD: shutdown

. /etc/rc.subr

name="i2plan"
rcvar="i2plan_enable"

command="/usr/local/bin/i2plan"
command_args="start"
pidfile="/var/run/${name}.pid"

load_rc_config $name
run_rc_command "$1"
```

#### 2. Make Executable

```bash
chmod +x /usr/local/etc/rc.d/i2plan
```

#### 3. Enable and Start

```bash
sysrc i2plan_enable="YES"
service i2plan start
service i2plan status
```

---

### OpenBSD (rc.d)

#### 1. Create rc.d Script

Create `/etc/rc.d/i2plan`:

```bash
#!/bin/ksh
#
# i2plan mesh VPN daemon

daemon="/usr/local/bin/i2plan"
daemon_flags="start"

. /etc/rc.d/rc.subr

rc_cmd $1
```

#### 2. Make Executable

```bash
chmod +x /etc/rc.d/i2plan
```

#### 3. Enable and Start

```bash
rcctl enable i2plan
rcctl start i2plan
rcctl check i2plan

# View logs
tail -f /var/log/daemon
```

---

## Docker Deployment

### Simple Docker Setup

#### 1. Dockerfile

Create `Dockerfile`:

```dockerfile
FROM golang:1.24-alpine AS builder

WORKDIR /build
COPY . .
RUN go build -o i2plan ./cmd/i2plan

FROM alpine:latest

RUN apk --no-cache add ca-certificates iptables wireguard-tools

WORKDIR /app
COPY --from=builder /build/i2plan .

EXPOSE 8080
ENTRYPOINT ["./i2plan", "start"]
```

#### 2. Build Image

```bash
docker build -t i2plan:latest .
```

#### 3. Run Container

```bash
docker run -d \
  --name i2plan \
  --cap-add=NET_ADMIN \
  -e I2PLAN_NODE_NAME=docker-node \
  -e I2PLAN_SAM_ADDRESS=host.docker.internal:7656 \
  -v i2plan-data:/root/.i2plan \
  -p 8080:8080 \
  i2plan:latest
```

### Docker Compose with I2P

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  i2p:
    image: geti2p/i2p
    container_name: i2p-router
    ports:
      - "7657:7657"  # Router console
    volumes:
      - i2p-data:/var/lib/i2p
    restart: unless-stopped

  i2plan:
    build: .
    container_name: i2plan
    depends_on:
      - i2p
    cap_add:
      - NET_ADMIN
    environment:
      - I2PLAN_NODE_NAME=docker-mesh-node
      - I2PLAN_SAM_ADDRESS=i2p:7656
    volumes:
      - i2plan-data:/root/.i2plan
    ports:
      - "8080:8080"
    restart: unless-stopped

volumes:
  i2p-data:
  i2plan-data:
```

Start:

```bash
docker-compose up -d

# View logs
docker-compose logs -f i2plan

# Access web UI
open http://localhost:8080
```

---

## Security Hardening

### File Permissions

**Linux/BSD/macOS:**

```bash
# Configuration directory
chmod 700 ~/.i2plan
chmod 600 ~/.i2plan/config.toml

# I2P keys (CRITICAL!)
chmod 600 ~/.i2plan/keys/*.i2p.private
chmod 644 ~/.i2plan/keys/*.i2p.public

# Logs
chmod 755 ~/.i2plan/logs
chmod 640 ~/.i2plan/logs/*.log
```

**Windows (PowerShell as Administrator):**

```powershell
# Restrict to current user only
$path = "$env:APPDATA\i2plan"
icacls $path /inheritance:r /grant:r "${env:USERNAME}:(OI)(CI)F"

# Extra protection for keys
icacls "$path\keys" /inheritance:r /grant:r "${env:USERNAME}:(OI)(CI)F"
```

### I2P Router Security

1. **Change router console password**:
   - Visit http://127.0.0.1:7657
   - Go to: Configure → Router Console → Set password

2. **Limit SAM bridge to localhost**:
   - In I2P config, ensure SAM only listens on 127.0.0.1
   - Never expose port 7656 to the network

3. **Keep I2P updated**:
   ```bash
   # Check for updates in router console
   # Or reinstall latest version periodically
   ```

### Firewall Configuration

**Linux (iptables):**

```bash
# Allow I2P but don't need any inbound rules
# I2P handles NAT traversal
# Only allow local access to web UI
sudo iptables -A INPUT -p tcp --dport 8080 -s 127.0.0.1 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 8080 -j DROP
```

**macOS:**

```bash
# Use built-in firewall
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --add /usr/local/bin/i2plan
```

**Windows:**

```powershell
# Block web UI from network (allow only localhost)
New-NetFirewallRule -DisplayName "i2plan Web UI" -Direction Inbound -LocalPort 8080 -Protocol TCP -Action Block -RemoteAddress Any
New-NetFirewallRule -DisplayName "i2plan Web UI Localhost" -Direction Inbound -LocalPort 8080 -Protocol TCP -Action Allow -RemoteAddress 127.0.0.1
```

### Regular Security Practices

1. **Keep software updated**:
   ```bash
   # Update Go
   # Update I2P router
   # Rebuild i2plan from latest source
   cd wireguard && git pull && go build -o i2plan cmd/i2plan/main.go
   ```

2. **Rotate I2P keys periodically** (advanced):
   ```bash
   # Backup old keys
   cp -r ~/.i2plan/keys ~/.i2plan/keys.backup
   
   # Generate new identity (will get new I2P address)
   ./i2plan identity generate
   
   # Requires rejoining mesh with new invite
   ```

3. **Monitor logs for suspicious activity**:
   ```bash
   # Look for failed auth attempts, unusual errors
   grep -i "error\|fail" ~/.i2plan/logs/i2plan.log
   ```

---

## Monitoring and Logging

### Health Checks

```bash
# Via CLI
./i2plan rpc status

# Via HTTP (curl)
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

### Log Rotation

**Linux (systemd/journald):**

```bash
# Automatic with journald
# Control size in /etc/systemd/journald.conf:
SystemMaxUse=100M
```

**Linux (logrotate for file logs):**

Create `/etc/logrotate.d/i2plan`:

```
/var/log/i2plan/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    create 0640 i2plan adm
}
```

**macOS:**

```bash
# Use newsyslog
# Add to /etc/newsyslog.conf:
/usr/local/var/log/i2plan.log  644  7  *  @T00  J
```

**Windows:**

NSSM handles log rotation if configured (see service setup above).

### Prometheus Metrics

If you have Prometheus, scrape metrics:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'i2plan'
    static_configs:
      - targets: ['localhost:8080']
```

View metrics:
```bash
curl http://localhost:8080/metrics
```

---

## Maintenance

### Backup

**Essential files to backup:**

```bash
# I2P keys (CRITICAL - can't rejoin mesh without these)
~/.i2plan/keys/*.i2p.private

# Configuration
~/.i2plan/config.toml

# Identity store (peer information)
~/.i2plan/identity.db
```

**Backup script:**

```bash
#!/bin/bash
BACKUP_DIR=~/i2plan-backup-$(date +%Y%m%d)
mkdir -p $BACKUP_DIR
cp -r ~/.i2plan/keys $BACKUP_DIR/
cp ~/.i2plan/config.toml $BACKUP_DIR/
cp ~/.i2plan/identity.db $BACKUP_DIR/
tar czf $BACKUP_DIR.tar.gz $BACKUP_DIR
echo "Backup created: $BACKUP_DIR.tar.gz"
```

### Updates

```bash
# Pull latest code
cd wireguard
git pull

# Rebuild
go build -o i2plan cmd/i2plan/main.go

# Stop service
sudo systemctl stop i2plan  # Linux
# or: sudo launchctl stop net.i2p.i2plan  # macOS
# or: Stop-Service i2plan  # Windows

# Replace binary
sudo cp i2plan /usr/local/bin/

# Start service
sudo systemctl start i2plan  # Linux
# or: sudo launchctl start net.i2p.i2plan  # macOS
# or: Start-Service i2plan  # Windows
```

### Troubleshooting

See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for detailed debugging steps.

**Quick diagnostics:**

```bash
# Check i2plan status
./i2plan rpc status

# Check I2P router
curl -I http://127.0.0.1:7657

# Check SAM bridge
nc -zv 127.0.0.1 7656

# View recent logs
tail -n 100 ~/.i2plan/logs/i2plan.log

# Check WireGuard interface
ip link show wg0  # Linux/BSD
ifconfig wg0      # macOS
```

---

## Getting Help

- **Documentation**: https://github.com/go-i2p/wireguard
- **Issues**: https://github.com/go-i2p/wireguard/issues
- **I2P Forum**: https://i2pforum.net/
- **Matrix**: #i2p-dev:matrix.org

Remember: i2plan operates on a **trusted peer model**. Only invite people you trust to your mesh!
