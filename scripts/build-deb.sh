#!/bin/bash
# Build Debian .deb package for i2plan
set -e

VERSION="${VERSION:-1.0.0}"
ARCH="amd64"
PKG_NAME="i2plan_${VERSION}_${ARCH}"

echo "Building Debian package: ${PKG_NAME}.deb"

# Create package directory structure
mkdir -p "${PKG_NAME}/DEBIAN"
mkdir -p "${PKG_NAME}/usr/bin"
mkdir -p "${PKG_NAME}/usr/share/doc/i2plan"
mkdir -p "${PKG_NAME}/usr/share/man/man1"
mkdir -p "${PKG_NAME}/lib/systemd/system"

# Copy binary
cp i2plan "${PKG_NAME}/usr/bin/"
chmod 755 "${PKG_NAME}/usr/bin/i2plan"

# Copy documentation
cp README.md "${PKG_NAME}/usr/share/doc/i2plan/"
cp LICENSE "${PKG_NAME}/usr/share/doc/i2plan/copyright"

# Copy systemd service
cp installer/linux/i2plan.service "${PKG_NAME}/lib/systemd/system/"

# Create control file
cat > "${PKG_NAME}/DEBIAN/control" <<EOF
Package: i2plan
Version: ${VERSION}
Section: net
Priority: optional
Architecture: ${ARCH}
Maintainer: go-i2p <noreply@github.com>
Description: WireGuard mesh VPN over I2P
 i2plan enables decentralized private networks using WireGuard
 over the I2P anonymizing network with gossip-based peer discovery and
 invite-based authentication.
Depends: i2prouter (>= 2.3.0) | i2pd (>= 2.44.0)
Recommends: wireguard-tools
EOF

# Create postinst script
cat > "${PKG_NAME}/DEBIAN/postinst" <<'EOF'
#!/bin/bash
set -e

# Create data directory
mkdir -p /var/lib/i2plan
chmod 755 /var/lib/i2plan

# Set capabilities on binary (requires setcap)
if command -v setcap &> /dev/null; then
    setcap cap_net_admin=+ep /usr/bin/i2plan || true
fi

# Reload systemd
if command -v systemctl &> /dev/null; then
    systemctl daemon-reload || true
fi

echo "i2plan installed successfully!"
echo "Start with: sudo systemctl start i2plan"
echo "Or run directly: i2plan"
EOF
chmod 755 "${PKG_NAME}/DEBIAN/postinst"

# Create prerm script
cat > "${PKG_NAME}/DEBIAN/prerm" <<'EOF'
#!/bin/bash
set -e

# Stop service if running
if command -v systemctl &> /dev/null; then
    systemctl stop i2plan || true
fi
EOF
chmod 755 "${PKG_NAME}/DEBIAN/prerm"

# Create postrm script
cat > "${PKG_NAME}/DEBIAN/postrm" <<'EOF'
#!/bin/bash
set -e

# Clean up on purge
if [ "$1" = "purge" ]; then
    rm -rf /var/lib/i2plan
    rm -rf /etc/i2plan
fi

# Reload systemd
if command -v systemctl &> /dev/null; then
    systemctl daemon-reload || true
fi
EOF
chmod 755 "${PKG_NAME}/DEBIAN/postrm"

# Build package
dpkg-deb --build "${PKG_NAME}"

# Validate package
if command -v lintian &> /dev/null; then
    lintian --no-tag-display-limit "${PKG_NAME}.deb" || true
fi

dpkg-deb --info "${PKG_NAME}.deb"
echo "Debian package built: ${PKG_NAME}.deb"
