/**
 * i2plan Web UI JavaScript
 * Vanilla JS implementation for the management interface.
 * @fileoverview Main application JavaScript with CSRF protection and API helpers.
 */

'use strict';

// CSRF token management - cached token for POST/PUT/DELETE requests
let csrfToken = null;

/**
 * Ensures a valid CSRF token is available, fetching one if necessary.
 * @returns {Promise<string>} The CSRF token.
 * @throws {Error} If token fetch fails.
 */
async function ensureCSRFToken() {
    if (csrfToken) {
        return csrfToken;
    }
    const response = await fetch('/api/csrf-token');
    if (!response.ok) {
        throw new Error('Failed to fetch CSRF token');
    }
    const data = await response.json();
    csrfToken = data.token;
    return csrfToken;
}

/**
 * Escapes HTML entities to prevent XSS when inserting user data into DOM.
 * @param {string} str - The string to escape.
 * @returns {string} HTML-escaped string.
 */
function escapeHtml(str) {
    if (str == null) return '';
    const div = document.createElement('div');
    div.textContent = String(str);
    return div.innerHTML;
}

/**
 * Displays a toast-style notification message (non-blocking alternative to alert).
 * @param {string} message - The message to display.
 * @param {'error'|'success'|'info'} type - The type of message.
 */
function showNotification(message, type = 'info') {
    // Use existing notification element or create one
    let container = document.getElementById('notification-container');
    if (!container) {
        container = document.createElement('div');
        container.id = 'notification-container';
        container.setAttribute('role', 'alert');
        container.setAttribute('aria-live', 'polite');
        container.style.cssText = 'position:fixed;top:1rem;right:1rem;z-index:1000;max-width:400px;';
        document.body.appendChild(container);
    }
    
    const notification = document.createElement('div');
    notification.className = `alert alert-${type}`;
    notification.textContent = message;
    notification.style.marginBottom = '0.5rem';
    container.appendChild(notification);
    
    // Auto-remove after 5 seconds
    setTimeout(() => {
        notification.remove();
    }, 5000);
}

/**
 * Makes a GET request to an API endpoint.
 * @param {string} endpoint - The API endpoint URL.
 * @returns {Promise<Object>} Parsed JSON response.
 * @throws {Error} If request fails or response is not OK.
 */
async function apiGet(endpoint) {
    const response = await fetch(endpoint);
    if (!response.ok) {
        // Safely parse error response, handling non-JSON responses
        let errorMessage = `Request failed with status ${response.status}`;
        try {
            const contentType = response.headers.get('content-type');
            if (contentType && contentType.includes('application/json')) {
                const error = await response.json();
                errorMessage = error.error || errorMessage;
            }
        } catch (e) {
            // Ignore parse errors, use default message
        }
        throw new Error(errorMessage);
    }
    return response.json();
}

/**
 * Makes a POST request to an API endpoint with CSRF protection.
 * Automatically retries once if CSRF token is invalid/expired.
 * @param {string} endpoint - The API endpoint URL.
 * @param {Object} data - The request body data.
 * @returns {Promise<Object>} Parsed JSON response.
 * @throws {Error} If request fails after retry.
 */
async function apiPost(endpoint, data) {
    const token = await ensureCSRFToken();
    
    const response = await fetch(endpoint, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'X-CSRF-Token': token
        },
        body: JSON.stringify(data)
    });
    
    if (!response.ok) {
        // If CSRF fails (403), refresh token and retry once
        if (response.status === 403) {
            csrfToken = null;
            const newToken = await ensureCSRFToken();
            const retryResponse = await fetch(endpoint, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-CSRF-Token': newToken
                },
                body: JSON.stringify(data)
            });
            if (!retryResponse.ok) {
                const error = await retryResponse.json().catch(() => ({}));
                throw new Error(error.error || 'Request failed after CSRF retry');
            }
            return retryResponse.json();
        }
        const error = await response.json().catch(() => ({}));
        throw new Error(error.error || `Request failed with status ${response.status}`);
    }
    return response.json();
}

/**
 * Refreshes the dashboard status display.
 * Fetches current status, peers, and routes from the API.
 */
async function refreshStatus() {
    try {
        const [status, peers, routes] = await Promise.all([
            apiGet('/api/status'),
            apiGet('/api/peers'),
            apiGet('/api/routes')
        ]);
        
        const peerCount = document.getElementById('peer-count');
        const routeCount = document.getElementById('route-count');
        
        if (peerCount) peerCount.textContent = peers.total || 0;
        if (routeCount) routeCount.textContent = routes.total || 0;
        
        showNotification('Status refreshed', 'success');
    } catch (err) {
        console.error('Failed to refresh status:', err);
        showNotification('Failed to refresh: ' + err.message, 'error');
    }
}

/**
 * Normalizes a peer state string to a valid CSS class suffix.
 * @param {string} state - The raw state string from API.
 * @returns {string} Lowercase, sanitized class name.
 */
function normalizeStateClass(state) {
    if (!state) return 'unknown';
    // Only allow alphanumeric and hyphens for CSS class safety
    return String(state).toLowerCase().replace(/[^a-z0-9-]/g, '');
}

/**
 * Refreshes the peers table with current data from the API.
 * Uses HTML escaping to prevent XSS.
 */
async function refreshPeers() {
    try {
        const data = await apiGet('/api/peers');
        const tbody = document.getElementById('peers-tbody');
        if (!tbody) return;
        
        if (!data.peers || data.peers.length === 0) {
            tbody.innerHTML = '<tr class="peer-row empty-row"><td colspan="4" class="muted">No peers connected</td></tr>';
            return;
        }
        
        tbody.innerHTML = data.peers.map(peer => `
            <tr class="peer-row" data-node-id="${escapeHtml(peer.node_id)}">
                <td class="mono">${escapeHtml(truncate(peer.node_id, 20))}</td>
                <td class="mono">${escapeHtml(peer.tunnel_ip)}</td>
                <td><span class="status-indicator status-badge status-${normalizeStateClass(peer.state)}">${escapeHtml(peer.state)}</span></td>
                <td>${escapeHtml(peer.last_seen)}</td>
            </tr>
        `).join('');
        
        showNotification('Peers refreshed', 'success');
    } catch (err) {
        console.error('Failed to refresh peers:', err);
        showNotification('Failed to refresh peers: ' + err.message, 'error');
    }
}

/**
 * Refreshes the routes table with current data from the API.
 * Uses HTML escaping to prevent XSS.
 */
async function refreshRoutes() {
    try {
        const data = await apiGet('/api/routes');
        const tbody = document.getElementById('routes-tbody');
        if (!tbody) return;
        
        if (!data.routes || data.routes.length === 0) {
            tbody.innerHTML = '<tr class="route-row empty-row"><td colspan="5" class="muted">No routes available</td></tr>';
            return;
        }
        
        tbody.innerHTML = data.routes.map(route => `
            <tr class="route-row" data-tunnel-ip="${escapeHtml(route.tunnel_ip)}">
                <td class="mono">${escapeHtml(route.tunnel_ip)}</td>
                <td class="mono">${escapeHtml(truncate(route.node_id, 20))}</td>
                <td class="mono">${route.via_node_id ? escapeHtml(truncate(route.via_node_id, 16)) : '<em>direct</em>'}</td>
                <td>${escapeHtml(String(route.hop_count))}</td>
                <td>${escapeHtml(route.last_seen)}</td>
            </tr>
        `).join('');
        
        showNotification('Routes refreshed', 'success');
    } catch (err) {
        console.error('Failed to refresh routes:', err);
        showNotification('Failed to refresh routes: ' + err.message, 'error');
    }
}

/**
 * DOMContentLoaded handler - initializes forms and pre-fetches CSRF token.
 */
document.addEventListener('DOMContentLoaded', function() {
    // Pre-fetch CSRF token on page load for better UX
    ensureCSRFToken().catch(function(err) {
        console.warn('Failed to pre-fetch CSRF token:', err);
    });
    
    // Create invite form handler
    const createForm = document.getElementById('create-invite-form');
    if (createForm) {
        createForm.addEventListener('submit', async function(e) {
            e.preventDefault();
            
            const expiryEl = document.getElementById('expiry');
            const maxUsesEl = document.getElementById('max_uses');
            const expiry = expiryEl ? expiryEl.value : '24h';
            const maxUses = maxUsesEl ? (parseInt(maxUsesEl.value, 10) || 1) : 1;
            
            try {
                const result = await apiPost('/api/invite/create', {
                    expiry: expiry,
                    max_uses: maxUses
                });
                
                const inviteResult = document.getElementById('invite-result');
                const inviteCode = document.getElementById('invite-code');
                
                if (inviteCode) inviteCode.value = result.invite_code || '';
                if (inviteResult) inviteResult.classList.remove('hidden');
                
                showNotification('Invite created successfully', 'success');
            } catch (err) {
                showNotification('Failed to create invite: ' + err.message, 'error');
            }
        });
    }
    
    // Accept invite form handler
    const acceptForm = document.getElementById('accept-invite-form');
    if (acceptForm) {
        acceptForm.addEventListener('submit', async function(e) {
            e.preventDefault();
            
            const inviteCodeEl = document.getElementById('invite_code');
            const inviteCode = inviteCodeEl ? inviteCodeEl.value.trim() : '';
            if (!inviteCode) {
                showNotification('Please enter an invite code', 'error');
                return;
            }
            
            const resultDiv = document.getElementById('accept-result');
            const successDiv = document.getElementById('accept-success');
            const errorDiv = document.getElementById('accept-error');
            
            try {
                const result = await apiPost('/api/invite/accept', {
                    invite_code: inviteCode
                });
                
                if (successDiv) {
                    successDiv.textContent = 'Connected! Tunnel IP: ' + (result.tunnel_ip || 'unknown');
                    successDiv.classList.remove('hidden');
                }
                if (errorDiv) errorDiv.classList.add('hidden');
                if (resultDiv) resultDiv.classList.remove('hidden');
                
                // Clear the input
                if (inviteCodeEl) inviteCodeEl.value = '';
                
                showNotification('Invite accepted successfully', 'success');
            } catch (err) {
                if (errorDiv) {
                    errorDiv.textContent = 'Failed: ' + err.message;
                    errorDiv.classList.remove('hidden');
                }
                if (successDiv) successDiv.classList.add('hidden');
                if (resultDiv) resultDiv.classList.remove('hidden');
                
                showNotification('Failed to accept invite: ' + err.message, 'error');
            }
        });
    }
});

/**
 * Copies the generated invite code to clipboard.
 * Uses modern Clipboard API with fallback for older browsers.
 * @param {Event} event - The click event.
 */
function copyInviteCode(event) {
    const inviteCode = document.getElementById('invite-code');
    if (!inviteCode) return;
    
    const text = inviteCode.value;
    const btn = event ? event.target : null;
    
    // Modern Clipboard API
    if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(function() {
            if (btn) showCopyFeedback(btn);
            showNotification('Copied to clipboard', 'success');
        }).catch(function(err) {
            console.error('Clipboard write failed:', err);
            fallbackCopy(inviteCode, btn);
        });
    } else {
        fallbackCopy(inviteCode, btn);
    }
}

/**
 * Fallback copy method for browsers without Clipboard API.
 * @param {HTMLTextAreaElement} textArea - The textarea element to copy from.
 * @param {HTMLElement|null} btn - The button element for feedback.
 */
function fallbackCopy(textArea, btn) {
    textArea.select();
    textArea.setSelectionRange(0, 99999); // For mobile
    try {
        document.execCommand('copy');
        if (btn) showCopyFeedback(btn);
        showNotification('Copied to clipboard', 'success');
    } catch (err) {
        console.error('Fallback copy failed:', err);
        showNotification('Failed to copy - please copy manually', 'error');
    }
}

/**
 * Shows visual feedback on the copy button.
 * @param {HTMLElement} btn - The button element.
 */
function showCopyFeedback(btn) {
    const originalText = btn.textContent;
    btn.textContent = 'Copied!';
    btn.disabled = true;
    setTimeout(function() {
        btn.textContent = originalText;
        btn.disabled = false;
    }, 2000);
}

/**
 * Truncates a string to the specified length, adding ellipsis if needed.
 * @param {string} str - The string to truncate.
 * @param {number} len - Maximum length including ellipsis.
 * @returns {string} Truncated string.
 */
function truncate(str, len) {
    if (str == null) return '';
    str = String(str);
    if (str.length <= len) return str;
    if (len <= 3) return str.slice(0, len);
    return str.slice(0, len - 3) + '...';
}

// Auto-refresh interval reference for cleanup
let autoRefreshInterval = null;

/**
 * Sets up auto-refresh for the dashboard page.
 * Refreshes status every 30 seconds.
 */
function setupAutoRefresh() {
    if (window.location.pathname === '/' || window.location.pathname === '') {
        // Clear any existing interval
        if (autoRefreshInterval) {
            clearInterval(autoRefreshInterval);
        }
        autoRefreshInterval = setInterval(refreshStatus, 30000);
    }
}

// Initialize auto-refresh on page load
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', setupAutoRefresh);
} else {
    setupAutoRefresh();
}

// QR Code functionality

/**
 * Displays the QR code for the generated invite.
 * @param {Event} event - The click event.
 */
async function showQRCode(event) {
    const inviteCode = document.getElementById('invite-code');
    if (!inviteCode || !inviteCode.value) {
        showNotification('No invite code to display', 'error');
        return;
    }

    const btn = event ? event.target : null;
    if (btn) btn.disabled = true;

    try {
        const token = await ensureCSRFToken();
        const response = await fetch('/api/invite/qr', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-CSRF-Token': token
            },
            body: JSON.stringify({
                invite_code: inviteCode.value
            })
        });

        if (!response.ok) {
            throw new Error('Failed to generate QR code');
        }

        // Get image blob and create object URL
        const blob = await response.blob();
        const url = URL.createObjectURL(blob);

        // Display the QR code
        const qrImage = document.getElementById('qr-code-image');
        const qrContainer = document.getElementById('qr-code-container');
        
        if (qrImage && qrContainer) {
            qrImage.src = url;
            qrContainer.classList.remove('hidden');
            showNotification('QR code generated', 'success');
        }
    } catch (err) {
        console.error('QR code generation failed:', err);
        showNotification('Failed to generate QR code: ' + err.message, 'error');
    } finally {
        if (btn) btn.disabled = false;
    }
}

// QR Scanner state
let qrStream = null;
let qrScanInterval = null;

/**
 * Starts the QR code scanner using the device camera.
 */
async function startQRScan() {
    const videoElement = document.getElementById('qr-video');
    const scannerContainer = document.getElementById('qr-scanner-container');
    const scanButton = document.getElementById('scan-qr-btn');

    if (!videoElement || !scannerContainer) {
        showNotification('Scanner not available', 'error');
        return;
    }

    try {
        // Request camera access
        qrStream = await navigator.mediaDevices.getUserMedia({
            video: { facingMode: 'environment' } // Prefer back camera on mobile
        });

        videoElement.srcObject = qrStream;
        videoElement.play();
        
        scannerContainer.classList.remove('hidden');
        if (scanButton) scanButton.disabled = true;

        // Start scanning
        qrScanInterval = setInterval(scanQRCode, 500);
        
        showNotification('Scanner started - position QR code in view', 'info');
    } catch (err) {
        console.error('Camera access failed:', err);
        showNotification('Camera access denied or unavailable', 'error');
    }
}

/**
 * Stops the QR code scanner and releases camera resources.
 */
function stopQRScan() {
    const videoElement = document.getElementById('qr-video');
    const scannerContainer = document.getElementById('qr-scanner-container');
    const scanButton = document.getElementById('scan-qr-btn');

    // Stop scan interval
    if (qrScanInterval) {
        clearInterval(qrScanInterval);
        qrScanInterval = null;
    }

    // Stop video stream
    if (qrStream) {
        qrStream.getTracks().forEach(track => track.stop());
        qrStream = null;
    }

    if (videoElement) {
        videoElement.srcObject = null;
    }

    if (scannerContainer) {
        scannerContainer.classList.add('hidden');
    }

    if (scanButton) {
        scanButton.disabled = false;
    }
}

/**
 * Scans a frame from the video for QR codes.
 * Uses a lightweight pure-JS QR decoder (jsQR library would be needed).
 * For simplicity, this implementation uses a basic approach.
 */
function scanQRCode() {
    const videoElement = document.getElementById('qr-video');
    const canvasElement = document.getElementById('qr-canvas');
    
    if (!videoElement || !canvasElement || videoElement.readyState !== videoElement.HAVE_ENOUGH_DATA) {
        return;
    }

    const canvas = canvasElement.getContext('2d');
    canvasElement.width = videoElement.videoWidth;
    canvasElement.height = videoElement.videoHeight;
    
    canvas.drawImage(videoElement, 0, 0, canvasElement.width, canvasElement.height);
    const imageData = canvas.getImageData(0, 0, canvasElement.width, canvasElement.height);

    // Try to decode QR code using jsQR (if available)
    // Note: This requires including jsQR library in the HTML
    if (typeof jsQR !== 'undefined') {
        const code = jsQR(imageData.data, imageData.width, imageData.height, {
            inversionAttempts: 'dontInvert',
        });

        if (code && code.data) {
            // Check if it's an i2plan invite
            if (code.data.startsWith('i2plan://')) {
                const inviteCodeInput = document.getElementById('invite_code');
                if (inviteCodeInput) {
                    inviteCodeInput.value = code.data;
                    stopQRScan();
                    showNotification('QR code scanned successfully!', 'success');
                }
            } else {
                showNotification('Not a valid i2plan invite code', 'error');
            }
        }
    } else {
        // jsQR not available - provide manual instruction
        console.warn('jsQR library not loaded - QR scanning unavailable');
        stopQRScan();
        showNotification('QR scanning requires jsQR library. Please paste the code manually.', 'error');
    }
}
