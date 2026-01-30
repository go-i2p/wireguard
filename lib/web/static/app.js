// i2plan Web UI JavaScript

// CSRF token management
let csrfToken = null;

async function ensureCSRFToken() {
    if (csrfToken) {
        return csrfToken;
    }
    try {
        const response = await fetch('/api/csrf-token');
        if (!response.ok) {
            throw new Error('Failed to fetch CSRF token');
        }
        const data = await response.json();
        csrfToken = data.token;
        return csrfToken;
    } catch (err) {
        console.error('CSRF token fetch failed:', err);
        throw err;
    }
}

// API helpers
async function apiGet(endpoint) {
    const response = await fetch(endpoint);
    if (!response.ok) {
        const error = await response.json();
        throw new Error(error.error || 'Request failed');
    }
    return response.json();
}

async function apiPost(endpoint, data) {
    // Ensure we have a CSRF token before making POST requests
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
        // If CSRF fails (403), try to refresh the token and retry once
        if (response.status === 403) {
            csrfToken = null; // Clear cached token
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
                const error = await retryResponse.json();
                throw new Error(error.error || 'Request failed');
            }
            return retryResponse.json();
        }
        const error = await response.json();
        throw new Error(error.error || 'Request failed');
    }
    return response.json();
}

// Dashboard
async function refreshStatus() {
    try {
        const status = await apiGet('/api/status');
        const peers = await apiGet('/api/peers');
        const routes = await apiGet('/api/routes');
        
        const peerCount = document.getElementById('peer-count');
        const routeCount = document.getElementById('route-count');
        
        if (peerCount) peerCount.textContent = peers.total || 0;
        if (routeCount) routeCount.textContent = routes.total || 0;
        
        console.log('Status refreshed:', status);
    } catch (err) {
        console.error('Failed to refresh status:', err);
        alert('Failed to refresh: ' + err.message);
    }
}

// Peers
async function refreshPeers() {
    try {
        const data = await apiGet('/api/peers');
        const tbody = document.getElementById('peers-table');
        if (!tbody) return;
        
        tbody.innerHTML = data.peers.map(peer => `
            <tr>
                <td class="mono">${truncate(peer.node_id, 20)}</td>
                <td class="mono">${peer.tunnel_ip}</td>
                <td><span class="status-badge status-${peer.state}">${peer.state}</span></td>
                <td>${peer.last_seen}</td>
            </tr>
        `).join('');
    } catch (err) {
        console.error('Failed to refresh peers:', err);
        alert('Failed to refresh: ' + err.message);
    }
}

// Routes
async function refreshRoutes() {
    try {
        const data = await apiGet('/api/routes');
        const tbody = document.getElementById('routes-table');
        if (!tbody) return;
        
        tbody.innerHTML = data.routes.map(route => `
            <tr>
                <td class="mono">${route.tunnel_ip}</td>
                <td class="mono">${truncate(route.node_id, 20)}</td>
                <td class="mono">${route.via_node_id ? truncate(route.via_node_id, 16) : '<em>direct</em>'}</td>
                <td>${route.hop_count}</td>
                <td>${route.last_seen}</td>
            </tr>
        `).join('');
    } catch (err) {
        console.error('Failed to refresh routes:', err);
        alert('Failed to refresh: ' + err.message);
    }
}

// Invites
document.addEventListener('DOMContentLoaded', function() {
    // Pre-fetch CSRF token on page load for better UX
    ensureCSRFToken().catch(err => {
        console.warn('Failed to pre-fetch CSRF token:', err);
    });
    
    // Create invite form
    const createForm = document.getElementById('create-invite-form');
    if (createForm) {
        createForm.addEventListener('submit', async function(e) {
            e.preventDefault();
            
            const expiry = document.getElementById('expiry').value;
            const maxUses = parseInt(document.getElementById('max_uses').value) || 1;
            
            try {
                const result = await apiPost('/api/invite/create', {
                    expiry: expiry,
                    max_uses: maxUses
                });
                
                const inviteResult = document.getElementById('invite-result');
                const inviteCode = document.getElementById('invite-code');
                
                inviteCode.value = result.invite_code;
                inviteResult.classList.remove('hidden');
            } catch (err) {
                alert('Failed to create invite: ' + err.message);
            }
        });
    }
    
    // Accept invite form
    const acceptForm = document.getElementById('accept-invite-form');
    if (acceptForm) {
        acceptForm.addEventListener('submit', async function(e) {
            e.preventDefault();
            
            const inviteCode = document.getElementById('invite_code').value.trim();
            if (!inviteCode) {
                alert('Please enter an invite code');
                return;
            }
            
            const resultDiv = document.getElementById('accept-result');
            const successDiv = document.getElementById('accept-success');
            const errorDiv = document.getElementById('accept-error');
            
            try {
                const result = await apiPost('/api/invite/accept', {
                    invite_code: inviteCode
                });
                
                successDiv.textContent = `Connected! Tunnel IP: ${result.tunnel_ip}`;
                successDiv.classList.remove('hidden');
                errorDiv.classList.add('hidden');
                resultDiv.classList.remove('hidden');
                
                // Clear the input
                document.getElementById('invite_code').value = '';
            } catch (err) {
                errorDiv.textContent = 'Failed: ' + err.message;
                errorDiv.classList.remove('hidden');
                successDiv.classList.add('hidden');
                resultDiv.classList.remove('hidden');
            }
        });
    }
});

// Copy invite code to clipboard
function copyInviteCode() {
    const inviteCode = document.getElementById('invite-code');
    inviteCode.select();
    document.execCommand('copy');
    
    // Visual feedback
    const btn = event.target;
    const originalText = btn.textContent;
    btn.textContent = 'Copied!';
    setTimeout(() => btn.textContent = originalText, 2000);
}

// Utility functions
function truncate(str, len) {
    if (!str) return '';
    if (str.length <= len) return str;
    return str.slice(0, len - 3) + '...';
}

// Auto-refresh every 30 seconds on dashboard
if (window.location.pathname === '/') {
    setInterval(refreshStatus, 30000);
}
