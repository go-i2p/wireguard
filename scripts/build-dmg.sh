#!/bin/bash
# Build macOS DMG package for i2plan
set -e

VERSION="${VERSION:-1.0.0}"
ARCH="${ARCH:-amd64}"
DMG_NAME="i2plan-${VERSION}-darwin-${ARCH}.dmg"
APP_NAME="i2plan.app"

echo "Building macOS DMG: ${DMG_NAME}"

# Create app bundle structure
mkdir -p "${APP_NAME}/Contents/MacOS"
mkdir -p "${APP_NAME}/Contents/Resources"

# Copy binary
cp i2plan "${APP_NAME}/Contents/MacOS/"
chmod +x "${APP_NAME}/Contents/MacOS/i2plan"

# Create Info.plist
cat > "${APP_NAME}/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>i2plan</string>
    <key>CFBundleIdentifier</key>
    <string>net.geti2p.i2plan</string>
    <key>CFBundleName</key>
    <string>i2plan</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>${VERSION}</string>
    <key>CFBundleVersion</key>
    <string>${VERSION}</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.13</string>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
EOF

# Create README for DMG
cat > README.txt <<EOF
i2plan ${VERSION}

Installation:
1. Copy i2plan.app to /Applications/
2. Open Terminal
3. Grant network privileges:
   sudo /Applications/i2plan.app/Contents/MacOS/i2plan --version

Usage:
Run from Terminal:
  sudo /Applications/i2plan.app/Contents/MacOS/i2plan

Or install as launch daemon (see DEPLOYMENT.md)

Documentation: https://github.com/go-i2p/wireguard
EOF

# Create DMG using create-dmg
if ! command -v create-dmg &> /dev/null; then
    echo "Warning: create-dmg not found, using simple hdiutil method"
    
    # Create temporary directory
    mkdir -p dmg-temp
    cp -R "${APP_NAME}" dmg-temp/
    cp README.txt dmg-temp/
    
    # Create DMG
    hdiutil create -volname "i2plan ${VERSION}" \
        -srcfolder dmg-temp \
        -ov -format UDZO \
        "${DMG_NAME}"
    
    # Clean up
    rm -rf dmg-temp
else
    # Use create-dmg for better looking DMG
    create-dmg \
        --volname "i2plan ${VERSION}" \
        --window-pos 200 120 \
        --window-size 600 400 \
        --icon-size 100 \
        --app-drop-link 425 120 \
        --hide-extension "${APP_NAME}" \
        --no-internet-enable \
        "${DMG_NAME}" \
        "${APP_NAME}" || {
            # Fallback to hdiutil if create-dmg fails
            echo "create-dmg failed, using hdiutil..."
            mkdir -p dmg-temp
            cp -R "${APP_NAME}" dmg-temp/
            cp README.txt dmg-temp/
            hdiutil create -volname "i2plan ${VERSION}" \
                -srcfolder dmg-temp \
                -ov -format UDZO \
                "${DMG_NAME}"
            rm -rf dmg-temp
        }
fi

# Clean up
rm -rf "${APP_NAME}" README.txt

echo "DMG package built: ${DMG_NAME}"
ls -lh "${DMG_NAME}"
