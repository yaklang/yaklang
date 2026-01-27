// ==================== OPS Portal JavaScript ====================

// State
let userInfo = null;
let availableModels = [];
let myApiKeys = [];

// ==================== Initialize ====================

document.addEventListener('DOMContentLoaded', function() {
    initTabs();
    loadUserInfo();
    loadModels();
    initForms();
    updateApiEndpoint();
});

// ==================== Tab Management ====================

function initTabs() {
    const tabs = document.querySelectorAll('.tab');
    tabs.forEach(tab => {
        tab.addEventListener('click', function() {
            const tabId = this.dataset.tab;
            switchToTab(tabId);
        });
    });
}

function switchToTab(tabId) {
    const tabs = document.querySelectorAll('.tab');
    
    // Update tab active state
    tabs.forEach(t => t.classList.remove('active'));
    const activeTab = document.querySelector(`.tab[data-tab="${tabId}"]`);
    if (activeTab) {
        activeTab.classList.add('active');
    }
    
    // Update content
    document.querySelectorAll('.tab-content').forEach(content => {
        content.classList.remove('active');
    });
    const tabContent = document.getElementById(tabId);
    if (tabContent) {
        tabContent.classList.add('active');
    }
    
    // Load data for specific tabs
    if (tabId === 'my-keys') {
        loadMyApiKeys();
    }
}

// ==================== User Info ====================

async function loadUserInfo() {
    try {
        const response = await fetch('/ops/my-info');
        const data = await response.json();
        
        if (data.success) {
            userInfo = data;
            updateUI();
            updateCurlExample();
        } else {
            console.error('Failed to load user info:', data.error);
            if (data.error === 'OPS user access required') {
                // Redirect to login
                window.location.href = '/ops/login';
            }
        }
    } catch (error) {
        console.error('Error loading user info:', error);
    }
}

function updateUI() {
    if (!userInfo) return;
    
    // Header
    document.getElementById('username-display').textContent = userInfo.username;
    
    // Stats
    document.getElementById('stat-api-keys').textContent = userInfo.api_keys_count || 0;
    document.getElementById('stat-default-limit').textContent = formatBytes(userInfo.default_limit);
    
    const statusEl = document.getElementById('stat-status');
    statusEl.textContent = userInfo.active ? 'Active' : 'Inactive';
    statusEl.style.color = userInfo.active ? '#28a745' : '#dc3545';
    
    // Default limit hint
    document.getElementById('default-limit-hint').textContent = formatBytes(userInfo.default_limit);
    
    // My Info tab
    document.getElementById('my-info-loading').classList.add('hidden');
    document.getElementById('my-info-content').classList.remove('hidden');
    
    document.getElementById('info-user-id').textContent = userInfo.user_id;
    document.getElementById('info-username').textContent = userInfo.username;
    document.getElementById('info-role').textContent = userInfo.role.toUpperCase();
    
    const infoStatus = document.getElementById('info-status');
    infoStatus.textContent = userInfo.active ? 'Active' : 'Inactive';
    infoStatus.style.color = userInfo.active ? '#28a745' : '#dc3545';
    
    document.getElementById('info-ops-key').textContent = userInfo.ops_key;
    document.getElementById('info-default-limit').textContent = formatBytes(userInfo.default_limit);
    document.getElementById('info-api-keys-count').textContent = userInfo.api_keys_count || 0;
    document.getElementById('info-created-at').textContent = userInfo.created_at;
}

// ==================== Models ====================

async function loadModels() {
    try {
        const response = await fetch('/v1/models');
        const data = await response.json();
        
        const select = document.getElementById('allowed-models');
        select.innerHTML = '';
        
        if (data.data && data.data.length > 0) {
            availableModels = data.data.map(m => m.id);
            data.data.forEach(model => {
                const option = document.createElement('option');
                option.value = model.id;
                option.textContent = model.id;
                select.appendChild(option);
            });
        } else {
            select.innerHTML = '<option value="">No models available</option>';
        }
    } catch (error) {
        console.error('Error loading models:', error);
        document.getElementById('allowed-models').innerHTML = '<option value="">Failed to load models</option>';
    }
}

// ==================== My API Keys ====================

async function loadMyApiKeys() {
    const loading = document.getElementById('my-keys-loading');
    const content = document.getElementById('my-keys-content');
    const tbody = document.getElementById('my-keys-tbody');
    const empty = document.getElementById('my-keys-empty');
    
    loading.classList.remove('hidden');
    content.classList.add('hidden');
    
    try {
        const response = await fetch('/ops/api/my-keys');
        const data = await response.json();
        
        loading.classList.add('hidden');
        content.classList.remove('hidden');
        
        if (data.success) {
            myApiKeys = data.keys || [];
            
            if (myApiKeys.length === 0) {
                tbody.innerHTML = '';
                empty.classList.remove('hidden');
            } else {
                empty.classList.add('hidden');
                renderApiKeysTable();
            }
        } else {
            tbody.innerHTML = `<tr><td colspan="6" style="text-align: center; color: #dc3545;">Error: ${data.error || 'Failed to load keys'}</td></tr>`;
        }
    } catch (error) {
        console.error('Error loading API keys:', error);
        loading.classList.add('hidden');
        content.classList.remove('hidden');
        tbody.innerHTML = '<tr><td colspan="6" style="text-align: center; color: #dc3545;">Network error</td></tr>';
    }
}

function renderApiKeysTable() {
    const tbody = document.getElementById('my-keys-tbody');
    
    tbody.innerHTML = myApiKeys.map(key => {
        const trafficPercent = key.traffic_limit > 0 
            ? Math.min(100, (key.traffic_used / key.traffic_limit) * 100).toFixed(1)
            : 0;
        const trafficColor = trafficPercent > 80 ? '#dc3545' : trafficPercent > 50 ? '#ffc107' : '#28a745';
        
        return `
            <tr>
                <td><code>${key.api_key.substring(0, 20)}...</code></td>
                <td>${(key.allowed_models || []).slice(0, 3).join(', ')}${(key.allowed_models || []).length > 3 ? '...' : ''}</td>
                <td style="color: ${trafficColor}">${formatBytes(key.traffic_used)}</td>
                <td>${formatBytes(key.traffic_limit)}</td>
                <td>${key.created_at || '--'}</td>
                <td>
                    <button class="btn btn-sm btn-danger" onclick="deleteApiKey('${key.api_key}')">Delete</button>
                </td>
            </tr>
        `;
    }).join('');
}

async function deleteApiKey(apiKey) {
    if (!confirm('Are you sure you want to delete this API key? This action cannot be undone.')) {
        return;
    }
    
    try {
        const response = await fetch('/ops/api/delete-api-key', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ api_key: apiKey })
        });
        
        const data = await response.json();
        
        if (data.success) {
            showToast('API Key deleted successfully', 'success');
            loadMyApiKeys();
            loadUserInfo(); // Refresh stats
        } else {
            showToast(data.error || 'Failed to delete API key', 'error');
        }
    } catch (error) {
        console.error('Error deleting API key:', error);
        showToast('Network error', 'error');
    }
}

// ==================== Form Handlers ====================

function initForms() {
    // Create API Key form
    document.getElementById('create-key-form').addEventListener('submit', handleCreateApiKey);
    
    // Change password form
    document.getElementById('change-password-form').addEventListener('submit', handleChangePassword);
}

async function handleCreateApiKey(e) {
    e.preventDefault();
    
    const select = document.getElementById('allowed-models');
    const selectedModels = Array.from(select.selectedOptions).map(opt => opt.value);
    const trafficLimit = document.getElementById('traffic-limit').value;
    
    if (selectedModels.length === 0) {
        showAlert('create-key-alert', 'Please select at least one model', 'error');
        return;
    }
    
    try {
        const response = await fetch('/ops/api/create-api-key', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                allowed_models: selectedModels,
                traffic_limit: trafficLimit ? parseInt(trafficLimit) : 0
            })
        });
        
        const data = await response.json();
        
        if (data.success) {
            showAlert('create-key-alert', 'API Key created successfully!', 'success');
            document.getElementById('generated-key').textContent = data.api_key;
            document.getElementById('api-key-result').classList.add('show');
            
            // Refresh user info to update stats
            loadUserInfo();
        } else {
            showAlert('create-key-alert', data.error || 'Failed to create API key', 'error');
        }
    } catch (error) {
        console.error('Error creating API key:', error);
        showAlert('create-key-alert', 'Network error. Please try again.', 'error');
    }
}

async function handleChangePassword(e) {
    e.preventDefault();
    
    const oldPassword = document.getElementById('old-password').value;
    const newPassword = document.getElementById('new-password').value;
    const confirmPassword = document.getElementById('confirm-password').value;
    
    if (newPassword !== confirmPassword) {
        showAlert('settings-alert', 'New passwords do not match', 'error');
        return;
    }
    
    if (newPassword.length < 8) {
        showAlert('settings-alert', 'New password must be at least 8 characters', 'error');
        return;
    }
    
    try {
        const response = await fetch('/ops/change-password', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                old_password: oldPassword,
                new_password: newPassword
            })
        });
        
        const data = await response.json();
        
        if (data.success) {
            showAlert('settings-alert', 'Password changed successfully!', 'success');
            document.getElementById('change-password-form').reset();
        } else {
            showAlert('settings-alert', data.error || 'Failed to change password', 'error');
        }
    } catch (error) {
        console.error('Error changing password:', error);
        showAlert('settings-alert', 'Network error. Please try again.', 'error');
    }
}

// ==================== OPS Key Management ====================

async function resetOpsKey() {
    if (!confirm('Are you sure you want to reset your OPS Key? You will need to update any applications using the current key.')) {
        return;
    }
    
    try {
        const response = await fetch('/ops/reset-key', {
            method: 'POST'
        });
        
        const data = await response.json();
        
        if (data.success) {
            alert('OPS Key reset successfully!\n\nNew Key: ' + data.new_ops_key);
            loadUserInfo();
        } else {
            alert('Failed to reset OPS key: ' + (data.error || 'Unknown error'));
        }
    } catch (error) {
        console.error('Error resetting OPS key:', error);
        alert('Network error. Please try again.');
    }
}

// ==================== API Usage / Curl ====================

function updateApiEndpoint() {
    const endpoint = window.location.origin;
    const endpointEl = document.getElementById('api-endpoint');
    if (endpointEl) {
        endpointEl.textContent = endpoint;
    }
    
    // Update all placeholders
    document.querySelectorAll('.api-endpoint-placeholder').forEach(el => {
        el.textContent = endpoint;
    });
}

function updateCurlExample() {
    if (!userInfo) return;
    
    const opsKeyEl = document.getElementById('curl-ops-key');
    if (opsKeyEl) {
        opsKeyEl.textContent = userInfo.ops_key;
    }
}

function copyCurlExample() {
    const endpoint = window.location.origin;
    const opsKey = userInfo ? userInfo.ops_key : 'YOUR_OPS_KEY';
    
    const curlCommand = `curl -X POST '${endpoint}/ops/api/create-api-key' \\
  -H 'Content-Type: application/json' \\
  -H 'X-Ops-Key: ${opsKey}' \\
  -d '{
    "allowed_models": ["gpt-4", "gpt-3.5-turbo"],
    "traffic_limit": 52428800
  }'`;
    
    copyToClipboard(curlCommand).then(() => {
        showToast('cURL command copied to clipboard!', 'success');
    }).catch(err => {
        console.error('Failed to copy:', err);
        showToast('Failed to copy to clipboard', 'error');
    });
}

// ==================== Clipboard ====================

function copyApiKey() {
    const key = document.getElementById('generated-key').textContent;
    copyToClipboard(key).then(() => {
        showToast('API Key copied to clipboard!', 'success');
    }).catch(err => {
        console.error('Failed to copy:', err);
        showToast('Failed to copy to clipboard', 'error');
    });
}

function copyToClipboard(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
        return navigator.clipboard.writeText(text);
    }
    
    // Fallback for older browsers
    return new Promise((resolve, reject) => {
        const textarea = document.createElement('textarea');
        textarea.value = text;
        textarea.style.position = 'fixed';
        textarea.style.opacity = '0';
        document.body.appendChild(textarea);
        textarea.select();
        try {
            document.execCommand('copy');
            resolve();
        } catch (err) {
            reject(err);
        } finally {
            document.body.removeChild(textarea);
        }
    });
}

// ==================== UI Helpers ====================

function showAlert(elementId, message, type) {
    const alert = document.getElementById(elementId);
    alert.textContent = message;
    alert.className = 'alert alert-' + type;
    alert.classList.remove('hidden');
    
    // Auto hide after 5 seconds
    setTimeout(() => {
        alert.classList.add('hidden');
    }, 5000);
}

function showToast(message, type) {
    // Create toast element
    let toast = document.getElementById('ops-toast');
    if (!toast) {
        toast = document.createElement('div');
        toast.id = 'ops-toast';
        toast.style.cssText = `
            position: fixed;
            bottom: 20px;
            right: 20px;
            padding: 12px 24px;
            border-radius: 8px;
            color: white;
            font-weight: 500;
            z-index: 9999;
            opacity: 0;
            transition: opacity 0.3s ease;
        `;
        document.body.appendChild(toast);
    }
    
    toast.textContent = message;
    toast.style.backgroundColor = type === 'success' ? '#28a745' : type === 'error' ? '#dc3545' : '#333';
    toast.style.opacity = '1';
    
    setTimeout(() => {
        toast.style.opacity = '0';
    }, 3000);
}

function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

// ==================== Export to window ====================

window.resetOpsKey = resetOpsKey;
window.copyApiKey = copyApiKey;
window.copyCurlExample = copyCurlExample;
window.deleteApiKey = deleteApiKey;
window.loadMyApiKeys = loadMyApiKeys;
window.switchToTab = switchToTab;
window.showAlert = showAlert;
window.showToast = showToast;
window.formatBytes = formatBytes;
