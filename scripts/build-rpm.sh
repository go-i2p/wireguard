#!/bin/bash
# Build RPM package for i2plan
set -e

VERSION="${VERSION:-1.0.0}"
ARCH="x86_64"

echo "Building RPM package: i2plan-${VERSION}-1.${ARCH}.rpm"

# Create RPM build directory structure
mkdir -p rpmbuild/{BUILD,RPMS,SOURCES,SPECS,SRPMS}

# Copy binary to BUILD directory
mkdir -p rpmbuild/BUILD
cp i2plan rpmbuild/BUILD/

# Copy systemd service
cp installer/linux/i2plan.service rpmbuild/BUILD/

# Copy documentation
cp README.md rpmbuild/BUILD/

# Create SPEC file
cat > rpmbuild/SPECS/i2plan.spec <<EOF
Name:           i2plan
Version:        ${VERSION}
Release:        1%{?dist}
Summary:        WireGuard mesh VPN over I2P
License:        See LICENSE file
URL:            https://github.com/go-i2p/wireguard
BuildArch:      ${ARCH}

Requires:       i2pd >= 2.44.0

%description
i2plan enables decentralized private networks using WireGuard
over the I2P anonymizing network with gossip-based peer discovery and
invite-based authentication.

%install
mkdir -p %{buildroot}/usr/bin
mkdir -p %{buildroot}/lib/systemd/system
mkdir -p %{buildroot}/usr/share/doc/i2plan

install -m 755 %{_builddir}/i2plan %{buildroot}/usr/bin/i2plan
install -m 644 %{_builddir}/i2plan.service %{buildroot}/lib/systemd/system/
install -m 644 %{_builddir}/README.md %{buildroot}/usr/share/doc/i2plan/

%post
# Create data directory
mkdir -p /var/lib/i2plan
chmod 755 /var/lib/i2plan

# Set capabilities (requires libcap)
if command -v setcap &> /dev/null; then
    setcap cap_net_admin=+ep /usr/bin/i2plan || true
fi

# Reload systemd
if command -v systemctl &> /dev/null; then
    systemctl daemon-reload || true
fi

echo "i2plan installed successfully!"
echo "Start with: sudo systemctl start i2plan"

%preun
# Stop service if running
if command -v systemctl &> /dev/null; then
    systemctl stop i2plan || true
fi

%postun
# Clean up on uninstall
if [ \$1 -eq 0 ]; then
    rm -rf /var/lib/i2plan
    rm -rf /etc/i2plan
fi

# Reload systemd
if command -v systemctl &> /dev/null; then
    systemctl daemon-reload || true
fi

%files
/usr/bin/i2plan
/lib/systemd/system/i2plan.service
/usr/share/doc/i2plan/README.md

%changelog
* $(date "+%a %b %d %Y") GitHub Actions <noreply@github.com> - ${VERSION}-1
- Automated release build
EOF

# Build RPM
rpmbuild -bb --define "_topdir $(pwd)/rpmbuild" rpmbuild/SPECS/i2plan.spec

# Copy built RPM to current directory
cp rpmbuild/RPMS/${ARCH}/i2plan-${VERSION}-1.*.rpm .

echo "RPM package built: i2plan-${VERSION}-1.*.rpm"
ls -lh i2plan-*.rpm
