# QR Code Features for Invite Sharing

This document describes the new QR code generation and scanning features implemented for both the TUI and Web UI interfaces.

## Overview

QR codes provide a convenient way to share invite codes between devices, especially when transferring from desktop to mobile or between different machines.

## TUI (Terminal UI) Features

### QR Code Generation

When creating an invite in the TUI (`i2plan tui`):

1. Press `n` to generate a new invite
2. The invite code will be displayed along with a terminal-rendered QR code
3. The QR code uses ASCII characters to display in the terminal
4. Scan the QR code with a mobile device to quickly transfer the invite

### QR Code Scanning

The TUI provides a scan mode for extracting invite codes from QR images:

1. Press `s` to enter scan mode
2. Since terminal webcam access is not available, you can:
   - Use a mobile QR scanner app to read the code
   - Use command-line tools like `zbarimg` to extract data from a saved QR image:
     ```bash
     zbarimg qr-code.png
     ```
   - Paste the extracted `i2plan://` URL into the text field

3. Press Enter to accept the invite

**Key Bindings:**
- `n` - Generate new invite (with QR code display)
- `a` - Accept invite code (paste manually)
- `s` - Scan QR code (instructions provided)

## Web UI Features

### QR Code Generation

When creating an invite in the Web UI:

1. Navigate to the Invites page
2. Configure expiry and max uses
3. Click "Generate Invite"
4. Click "Show QR Code" to display a PNG QR code
5. The QR code can be scanned with any mobile device or QR code reader

**API Endpoint:** `POST /api/invite/qr`
- Request: `{"invite_code": "i2plan://..."}`
- Response: PNG image (256x256 pixels, medium error correction)

### Camera Scanning

The Web UI includes webcam-based QR code scanning:

1. Navigate to the Invites page
2. Click "Scan QR Code"
3. Grant camera permissions when prompted
4. Position the QR code in front of your webcam
5. The code will be automatically detected and populated in the invite field
6. Click "Accept Invite" to complete the process

**Features:**
- Uses WebRTC getUserMedia API for camera access
- Powered by jsQR library for client-side QR decoding
- Validates that scanned codes start with `i2plan://`
- Automatically stops scanning after successful detection
- Works on desktop and mobile browsers (with camera permissions)

## Security Considerations

Following the security guidelines in the README:

- **No sensitive data logged**: Only metadata about QR operations is logged
- **Invite codes are public identifiers**: Safe to display in QR codes
- **CSRF protection**: The QR generation endpoint requires CSRF tokens
- **Camera permissions**: Users must explicitly grant camera access

## Technical Implementation

### Dependencies

**TUI:**
- `github.com/skip2/go-qrcode` - QR code generation (server-side)
- `github.com/mdp/qrterminal/v3` - Terminal QR rendering

**Web UI:**
- `github.com/skip2/go-qrcode` - QR code generation (server-side)
- `jsQR` (CDN) - Client-side QR code scanning from webcam

### Code Structure

**TUI (`lib/tui/invites.go`):**
- Added `InvitesModeScan` mode
- Added `qrCode` field to `InvitesModel`
- Implemented `generateQRCode()` for terminal rendering
- Added `handleScanModeKey()` for scan mode input
- Added `viewScan()` for scan mode UI

**Web UI:**
- `lib/web/handlers.go`: Added `handleAPIInviteQR()` endpoint
- `lib/web/templates/invites.html`: Added QR display and scanner UI
- `lib/web/static/app.js`: Added `showQRCode()`, `startQRScan()`, `stopQRScan()`, `scanQRCode()` functions
- `lib/web/templates/base.html`: Added jsQR library CDN reference

## Testing

All existing tests continue to pass:

```bash
# Test TUI
go test ./lib/tui -v

# Test Web UI
go test ./lib/web -v

# Build full binary
go build ./cmd/i2plan
```

## Usage Examples

### TUI Example

```bash
# Start the TUI
./i2plan tui

# In the Invites tab:
# 1. Press 'n' to create an invite
# 2. QR code appears in the terminal
# 3. Scan with mobile device

# To scan a QR code:
# 1. Press 's' to enter scan mode
# 2. Use external tool: zbarimg saved-qr.png
# 3. Paste the i2plan:// URL
# 4. Press Enter
```

### Web UI Example

```bash
# Start the node
./i2plan

# In another terminal, start the web UI
./i2plan web

# Navigate to http://localhost:8080/invites

# To create and share:
# 1. Click "Generate Invite"
# 2. Click "Show QR Code"
# 3. Scan with mobile device

# To scan:
# 1. Click "Scan QR Code"
# 2. Allow camera access
# 3. Position QR code in view
# 4. Code auto-populates
# 5. Click "Accept Invite"
```

## Browser Compatibility

**QR Scanning (Camera):**
- Chrome/Edge: ✅ Full support
- Firefox: ✅ Full support
- Safari: ✅ Full support (iOS 11+)
- Mobile browsers: ✅ Works on modern mobile browsers

**QR Display:**
- All modern browsers support PNG image display

## Limitations

1. **TUI Camera Access**: Terminal applications cannot access webcam devices directly. Users must use external tools for QR scanning.

2. **HTTPS Requirement**: WebRTC camera access requires HTTPS in production environments (exception: localhost for development).

3. **Camera Permissions**: Users must grant camera permissions for scanning to work.

4. **jsQR Dependency**: Web UI scanning requires jsQR library from CDN. If offline, scanning won't work (but QR display still functions).

## Future Enhancements

Potential improvements for future versions:

- Configurable QR code size in Web UI
- Save QR code as image file in TUI
- NFC support for mobile-to-mobile transfers
- Batch invite QR code generation
- QR code expiry indicators
- Animated instructions for camera positioning
