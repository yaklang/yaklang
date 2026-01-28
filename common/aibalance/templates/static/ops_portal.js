// ==================== OPS Portal JavaScript ====================

// State
let userInfo = null;
let availableModels = [];
let myApiKeys = [];

// ==================== Authentication Error Handler ====================

// Check if response indicates authentication error
function isAuthError(data) {
    if (!data) return false;
    // Check for permission denied errors
    if (data.error === 'Permission denied' || 
        data.error === 'Unauthorized' ||
        data.error === 'OPS user access required' ||
        data.reason === 'insufficient permissions' ||
        data.error === 'Authentication required') {
        return true;
    }
    return false;
}

// Clear authentication cookies and redirect to login
function handleAuthError() {
    console.warn('Authentication expired or invalid, redirecting to login...');
    // Clear ops_session cookie
    document.cookie = 'ops_session=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT;';
    // Show a brief message before redirecting
    alert('Session expired, please login again.');
    // Redirect to login page
    window.location.href = '/ops/login';
}

// Wrapper for fetch that handles authentication errors
async function authFetch(url, options = {}) {
    const response = await fetch(url, options);
    
    // Check for 401/403 status codes
    if (response.status === 401 || response.status === 403) {
        // Try to parse response to check error type
        try {
            const data = await response.clone().json();
            if (isAuthError(data)) {
                handleAuthError();
                return null;
            }
        } catch (e) {
            // If can't parse JSON, still redirect for 401
            if (response.status === 401) {
                handleAuthError();
                return null;
            }
        }
    }
    
    return response;
}

// Check response data for auth errors (for successful HTTP responses with error in body)
function checkAuthInResponse(data) {
    if (isAuthError(data)) {
        handleAuthError();
        return true;
    }
    return false;
}

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
        const response = await authFetch('/ops/my-info');
        if (!response) return; // Auth error handled
        const data = await response.json();
        
        // Check for auth error in response body
        if (checkAuthInResponse(data)) return;
        
        if (data.success) {
            userInfo = data;
            updateUI();
            updateCurlExample();
        } else {
            console.error('Failed to load user info:', data.error);
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
    
    // Default limit hint (may not exist in new UI)
    const defaultLimitHint = document.getElementById('default-limit-hint');
    if (defaultLimitHint) {
        defaultLimitHint.textContent = formatBytes(userInfo.default_limit);
    }
    
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

let selectedModels = new Set();

async function loadModels() {
    const modelList = document.getElementById('model-list');
    
    try {
        const response = await fetch('/v1/models');
        const data = await response.json();
        
        if (data.data && data.data.length > 0) {
            // Sort models alphabetically for consistent display
            availableModels = data.data.map(m => m.id).sort();
            renderModelList();
        } else {
            modelList.innerHTML = '<div style="padding: 20px; text-align: center; color: #888;">No models available</div>';
        }
    } catch (error) {
        console.error('Error loading models:', error);
        modelList.innerHTML = '<div style="padding: 20px; text-align: center; color: #dc3545;">Failed to load models</div>';
    }
}

function renderModelList() {
    const modelList = document.getElementById('model-list');
    
    modelList.innerHTML = availableModels.map(model => `
        <div class="model-item ${selectedModels.has(model) ? 'selected' : ''}" onclick="toggleModel('${model}')">
            <input type="checkbox" id="model-${model}" ${selectedModels.has(model) ? 'checked' : ''} onclick="event.stopPropagation(); toggleModel('${model}')">
            <label for="model-${model}">${model}</label>
        </div>
    `).join('');
    
    updateSelectedPreview();
}

function toggleModel(model) {
    if (selectedModels.has(model)) {
        selectedModels.delete(model);
    } else {
        selectedModels.add(model);
    }
    renderModelList();
}

function selectAllModels() {
    availableModels.forEach(m => selectedModels.add(m));
    renderModelList();
}

function clearAllModels() {
    selectedModels.clear();
    renderModelList();
}

function updateSelectedPreview() {
    const preview = document.getElementById('selected-preview');
    if (!preview) return;
    
    const globInput = document.getElementById('glob-patterns');
    const globPatterns = globInput ? globInput.value.trim() : '';
    
    let html = '<strong>Selected:</strong> ';
    
    // Show selected models (sorted)
    const modelArray = Array.from(selectedModels).sort();
    if (modelArray.length > 0) {
        if (modelArray.length <= 5) {
            html += modelArray.map(m => `<span class="tag">${m}</span>`).join('');
        } else {
            html += modelArray.slice(0, 3).map(m => `<span class="tag">${m}</span>`).join('');
            html += `<span class="tag">+${modelArray.length - 3} more</span>`;
        }
    }
    
    // Show glob patterns
    if (globPatterns) {
        const patterns = globPatterns.split(',').map(p => p.trim()).filter(p => p);
        patterns.forEach(p => {
            html += `<span class="tag glob">${p}</span>`;
        });
    }
    
    if (modelArray.length === 0 && !globPatterns) {
        html += '<span style="color: #888;">None selected</span>';
    }
    
    preview.innerHTML = html;
}

// Listen for glob input changes
document.addEventListener('DOMContentLoaded', function() {
    const globInput = document.getElementById('glob-patterns');
    if (globInput) {
        globInput.addEventListener('input', updateSelectedPreview);
    }
});

// ==================== My API Keys ====================

let myKeysPage = 1;
let myKeysPageSize = 20;
let myKeysPagination = null;

async function loadMyApiKeys(page, pageSize) {
    if (page !== undefined) myKeysPage = page;
    if (pageSize !== undefined) myKeysPageSize = pageSize;
    
    const loading = document.getElementById('my-keys-loading');
    const content = document.getElementById('my-keys-content');
    const tbody = document.getElementById('my-keys-tbody');
    const empty = document.getElementById('my-keys-empty');
    
    loading.classList.remove('hidden');
    content.classList.add('hidden');
    
    try {
        const response = await fetch(`/ops/api/my-keys?page=${myKeysPage}&page_size=${myKeysPageSize}`);
        const data = await response.json();
        
        loading.classList.add('hidden');
        content.classList.remove('hidden');
        
        if (data.success) {
            myApiKeys = data.keys || [];
            myKeysPagination = data.pagination || null;
            
            if (myApiKeys.length === 0 && myKeysPage === 1) {
                tbody.innerHTML = '';
                empty.classList.remove('hidden');
                renderMyKeysPagination();
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

// Change My Keys page
function changeMyKeysPage(page) {
    if (page < 1) page = 1;
    if (myKeysPagination && page > myKeysPagination.total_pages) {
        page = myKeysPagination.total_pages;
    }
    loadMyApiKeys(page);
}

// Change My Keys page size
function changeMyKeysPageSize(newSize) {
    myKeysPageSize = parseInt(newSize);
    myKeysPage = 1; // Reset to first page
    loadMyApiKeys();
}

function renderApiKeysTable() {
    const tbody = document.getElementById('my-keys-tbody');
    
    tbody.innerHTML = myApiKeys.map(key => {
        const isUnlimited = !key.traffic_limit_enable || !key.traffic_limit || key.traffic_limit <= 0;
        let trafficColor = '#28a745';
        let trafficUsedDisplay = formatBytes(key.traffic_used || 0);
        let trafficLimitDisplay = isUnlimited ? '<span style="color: #28a745; font-weight: 500;">Unlimited</span>' : formatBytes(key.traffic_limit);
        
        if (!isUnlimited && key.traffic_limit > 0) {
            const trafficPercent = Math.min(100, ((key.traffic_used || 0) / key.traffic_limit) * 100);
            trafficColor = trafficPercent > 80 ? '#dc3545' : trafficPercent > 50 ? '#ffc107' : '#28a745';
        }
        
        // Display models (sorted)
        const models = key.allowed_models || [];
        const modelsDisplay = models.length > 3 
            ? models.slice(0, 3).join(', ') + ` (+${models.length - 3})`
            : models.join(', ');
        
        return `
            <tr>
                <td><code>${key.api_key.substring(0, 20)}...</code></td>
                <td title="${models.join(', ')}">${modelsDisplay || '-'}</td>
                <td style="color: ${trafficColor}">${trafficUsedDisplay}</td>
                <td>${trafficLimitDisplay}</td>
                <td>${key.created_at || '--'}</td>
                <td>
                    <div style="display: flex; gap: 5px; flex-wrap: wrap;">
                        <button class="btn btn-sm" onclick="openEditKeyModal('${key.api_key}')" style="background: #4285f4;">Edit</button>
                        <button class="btn btn-sm btn-danger" onclick="deleteApiKey('${key.api_key}')">Delete</button>
                    </div>
                </td>
            </tr>
        `;
    }).join('');
    
    // Render pagination controls
    renderMyKeysPagination();
}

// Render My Keys pagination controls
function renderMyKeysPagination() {
    let paginationContainer = document.getElementById('my-keys-pagination');
    if (!paginationContainer) {
        // Create pagination container
        const table = document.getElementById('my-keys-table');
        if (table && table.parentElement) {
            paginationContainer = document.createElement('div');
            paginationContainer.id = 'my-keys-pagination';
            paginationContainer.className = 'pagination-controls';
            paginationContainer.style.cssText = 'display: flex; justify-content: space-between; align-items: center; margin-top: 15px; padding: 10px; background: #f8f9fa; border-radius: 4px;';
            table.parentElement.appendChild(paginationContainer);
        }
    }
    
    if (!paginationContainer || !myKeysPagination) {
        if (paginationContainer) {
            paginationContainer.innerHTML = '';
        }
        return;
    }
    
    const { page, page_size, total, total_pages } = myKeysPagination;
    
    paginationContainer.innerHTML = `
        <div class="pagination-info">
            Total ${total} keys, Page ${page}/${total_pages || 1}
        </div>
        <div class="pagination-buttons" style="display: flex; gap: 5px; align-items: center;">
            <button class="btn btn-sm" ${page <= 1 ? 'disabled' : ''} onclick="changeMyKeysPage(1)">First</button>
            <button class="btn btn-sm" ${page <= 1 ? 'disabled' : ''} onclick="changeMyKeysPage(${page - 1})">Prev</button>
            <span style="margin: 0 10px;">
                Go to <input type="number" id="myKeysPageInput" min="1" max="${total_pages}" value="${page}" style="width: 50px; text-align: center;"> 
                <button class="btn btn-sm" onclick="changeMyKeysPage(parseInt(document.getElementById('myKeysPageInput').value))">Go</button>
            </span>
            <button class="btn btn-sm" ${page >= total_pages ? 'disabled' : ''} onclick="changeMyKeysPage(${page + 1})">Next</button>
            <button class="btn btn-sm" ${page >= total_pages ? 'disabled' : ''} onclick="changeMyKeysPage(${total_pages})">Last</button>
            <select onchange="changeMyKeysPageSize(this.value)" style="margin-left: 10px;">
                <option value="10" ${page_size === 10 ? 'selected' : ''}>10/page</option>
                <option value="20" ${page_size === 20 ? 'selected' : ''}>20/page</option>
                <option value="50" ${page_size === 50 ? 'selected' : ''}>50/page</option>
                <option value="100" ${page_size === 100 ? 'selected' : ''}>100/page</option>
            </select>
        </div>
    `;
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

// ==================== Edit API Key ====================

let editSelectedModels = new Set();
let currentEditKey = null;

function openEditKeyModal(apiKey) {
    // Find the key data
    currentEditKey = myApiKeys.find(k => k.api_key === apiKey);
    if (!currentEditKey) {
        showToast('API Key not found', 'error');
        return;
    }
    
    // Populate modal
    document.getElementById('edit-key-api-key').value = apiKey;
    document.getElementById('edit-key-display').value = apiKey;
    
    // Load current allowed models
    editSelectedModels.clear();
    const currentModels = currentEditKey.allowed_models || [];
    
    // Separate regular models from glob patterns
    const regularModels = [];
    const globPatterns = [];
    currentModels.forEach(m => {
        if (m.includes('*')) {
            globPatterns.push(m);
        } else {
            regularModels.push(m);
            editSelectedModels.add(m);
        }
    });
    
    // Set glob patterns
    document.getElementById('edit-glob-patterns').value = globPatterns.join(',');
    
    // Render model list
    renderEditModelList();
    updateEditSelectedPreview();
    
    // Set traffic settings
    const isUnlimited = !currentEditKey.traffic_limit_enable || !currentEditKey.traffic_limit || currentEditKey.traffic_limit <= 0;
    const unlimitedCheckbox = document.getElementById('edit-unlimited-traffic');
    const trafficLimitGroup = document.getElementById('edit-traffic-limit-group');
    const unlimitedToggle = document.getElementById('edit-unlimited-toggle');
    const trafficDesc = document.getElementById('edit-traffic-desc');
    
    unlimitedCheckbox.checked = isUnlimited;
    if (isUnlimited) {
        trafficLimitGroup.style.display = 'none';
        unlimitedToggle.classList.add('active');
        trafficDesc.textContent = 'API Key will have no traffic restrictions';
    } else {
        trafficLimitGroup.style.display = 'block';
        unlimitedToggle.classList.remove('active');
        trafficDesc.textContent = 'Set a custom traffic limit below';
        
        // Convert bytes to value and unit
        const trafficBytes = currentEditKey.traffic_limit || 0;
        const valueInput = document.getElementById('edit-traffic-limit-value');
        const unitSelect = document.getElementById('edit-traffic-limit-unit');
        
        if (trafficBytes >= 1073741824 && trafficBytes % 1073741824 === 0) {
            valueInput.value = trafficBytes / 1073741824;
            unitSelect.value = '1073741824';
        } else if (trafficBytes >= 1048576 && trafficBytes % 1048576 === 0) {
            valueInput.value = trafficBytes / 1048576;
            unitSelect.value = '1048576';
        } else if (trafficBytes >= 1024 && trafficBytes % 1024 === 0) {
            valueInput.value = trafficBytes / 1024;
            unitSelect.value = '1024';
        } else {
            valueInput.value = trafficBytes;
            unitSelect.value = '1';
        }
        calculateEditTrafficBytes();
    }
    
    document.getElementById('edit-traffic-used').textContent = formatBytes(currentEditKey.traffic_used || 0);
    
    // Set up toggle event
    unlimitedCheckbox.onchange = function() {
        if (this.checked) {
            trafficLimitGroup.style.display = 'none';
            unlimitedToggle.classList.add('active');
            trafficDesc.textContent = 'API Key will have no traffic restrictions';
        } else {
            trafficLimitGroup.style.display = 'block';
            unlimitedToggle.classList.remove('active');
            trafficDesc.textContent = 'Set a custom traffic limit below';
            calculateEditTrafficBytes();
        }
    };
    
    // Set up traffic input event listeners
    const editValueInput = document.getElementById('edit-traffic-limit-value');
    const editUnitSelect = document.getElementById('edit-traffic-limit-unit');
    if (editValueInput) {
        editValueInput.addEventListener('input', calculateEditTrafficBytes);
    }
    if (editUnitSelect) {
        editUnitSelect.addEventListener('change', calculateEditTrafficBytes);
    }
    
    // Show modal
    document.getElementById('edit-key-modal').style.display = 'flex';
}

// Calculate edit traffic bytes
function calculateEditTrafficBytes() {
    const valueInput = document.getElementById('edit-traffic-limit-value');
    const unitSelect = document.getElementById('edit-traffic-limit-unit');
    const calculatedDisplay = document.getElementById('edit-calculated-bytes');
    
    if (!valueInput || !unitSelect) return 0;
    
    const value = parseFloat(valueInput.value) || 0;
    const multiplier = parseInt(unitSelect.value) || 1;
    const bytes = Math.floor(value * multiplier);
    
    if (calculatedDisplay) {
        const formatted = formatBytes(bytes);
        calculatedDisplay.textContent = `Calculated: ${bytes.toLocaleString()} bytes (${formatted})`;
    }
    
    return bytes;
}

function closeEditKeyModal() {
    document.getElementById('edit-key-modal').style.display = 'none';
    currentEditKey = null;
    editSelectedModels.clear();
}

function renderEditModelList() {
    const modelList = document.getElementById('edit-model-list');
    
    if (availableModels.length === 0) {
        modelList.innerHTML = '<div style="padding: 20px; text-align: center; color: #888;">No models available</div>';
        return;
    }
    
    modelList.innerHTML = availableModels.map(model => `
        <div class="model-item ${editSelectedModels.has(model) ? 'selected' : ''}" onclick="editToggleModel('${model}')">
            <input type="checkbox" ${editSelectedModels.has(model) ? 'checked' : ''} onclick="event.stopPropagation(); editToggleModel('${model}')">
            <label>${model}</label>
        </div>
    `).join('');
}

function editToggleModel(model) {
    if (editSelectedModels.has(model)) {
        editSelectedModels.delete(model);
    } else {
        editSelectedModels.add(model);
    }
    renderEditModelList();
    updateEditSelectedPreview();
}

function editSelectAllModels() {
    availableModels.forEach(m => editSelectedModels.add(m));
    renderEditModelList();
    updateEditSelectedPreview();
}

function editClearAllModels() {
    editSelectedModels.clear();
    renderEditModelList();
    updateEditSelectedPreview();
}

function updateEditSelectedPreview() {
    const preview = document.getElementById('edit-selected-preview');
    if (!preview) return;
    
    const globInput = document.getElementById('edit-glob-patterns');
    const globPatterns = globInput ? globInput.value.trim() : '';
    
    let html = '<strong>Selected:</strong> ';
    
    const modelArray = Array.from(editSelectedModels).sort();
    if (modelArray.length > 0) {
        if (modelArray.length <= 5) {
            html += modelArray.map(m => `<span class="tag">${m}</span>`).join('');
        } else {
            html += modelArray.slice(0, 3).map(m => `<span class="tag">${m}</span>`).join('');
            html += `<span class="tag">+${modelArray.length - 3} more</span>`;
        }
    }
    
    if (globPatterns) {
        const patterns = globPatterns.split(',').map(p => p.trim()).filter(p => p);
        patterns.forEach(p => {
            html += `<span class="tag glob">${p}</span>`;
        });
    }
    
    if (modelArray.length === 0 && !globPatterns) {
        html += '<span style="color: #888;">None selected</span>';
    }
    
    preview.innerHTML = html;
}

// Listen for edit glob input changes
document.addEventListener('DOMContentLoaded', function() {
    const editGlobInput = document.getElementById('edit-glob-patterns');
    if (editGlobInput) {
        editGlobInput.addEventListener('input', updateEditSelectedPreview);
    }
});

async function saveEditKey() {
    const apiKey = document.getElementById('edit-key-api-key').value;
    
    // Get selected models
    const modelArray = Array.from(editSelectedModels);
    
    // Get glob patterns
    const globInput = document.getElementById('edit-glob-patterns');
    const globPatterns = globInput ? globInput.value.trim() : '';
    const globArray = globPatterns ? globPatterns.split(',').map(p => p.trim()).filter(p => p) : [];
    
    // Combine and sort
    const allModels = [...modelArray, ...globArray].sort();
    
    if (allModels.length === 0) {
        showToast('Please select at least one model or enter a glob pattern', 'error');
        return;
    }
    
    // Get traffic settings
    const isUnlimited = document.getElementById('edit-unlimited-traffic').checked;
    const trafficLimit = calculateEditTrafficBytes();
    
    if (!isUnlimited && trafficLimit <= 0) {
        showToast('Please enter a valid traffic limit or enable unlimited traffic', 'error');
        return;
    }
    
    try {
        const requestBody = {
            api_key: apiKey,
            allowed_models: allModels,
            unlimited: isUnlimited
        };
        
        if (!isUnlimited && trafficLimit > 0) {
            requestBody.traffic_limit = trafficLimit;
        }
        
        const response = await fetch('/ops/api/update-api-key', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(requestBody)
        });
        
        const data = await response.json();
        
        if (data.success) {
            showToast('API Key updated successfully', 'success');
            closeEditKeyModal();
            loadMyApiKeys();
        } else {
            showToast(data.error || 'Failed to update API key', 'error');
        }
    } catch (error) {
        console.error('Error updating API key:', error);
        showToast('Network error', 'error');
    }
}

async function resetEditKeyTraffic() {
    const apiKey = document.getElementById('edit-key-api-key').value;
    
    if (!confirm('Are you sure you want to reset the traffic counter for this API key?')) {
        return;
    }
    
    try {
        const response = await fetch('/ops/api/reset-traffic', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ api_key: apiKey })
        });
        
        const data = await response.json();
        
        if (data.success) {
            showToast('Traffic reset successfully', 'success');
            document.getElementById('edit-traffic-used').textContent = '0 B';
            loadMyApiKeys();
        } else {
            showToast(data.error || 'Failed to reset traffic', 'error');
        }
    } catch (error) {
        console.error('Error resetting traffic:', error);
        showToast('Network error', 'error');
    }
}

// ==================== Form Handlers ====================

// Calculate traffic limit in bytes from value and unit
function calculateTrafficBytes() {
    const valueInput = document.getElementById('traffic-limit-value');
    const unitSelect = document.getElementById('traffic-limit-unit');
    const calculatedDisplay = document.getElementById('calculated-bytes');
    
    if (!valueInput || !unitSelect) return 0;
    
    const value = parseFloat(valueInput.value) || 0;
    const multiplier = parseInt(unitSelect.value) || 1;
    const bytes = Math.floor(value * multiplier);
    
    if (calculatedDisplay) {
        const formatted = formatBytes(bytes);
        calculatedDisplay.textContent = `Calculated: ${bytes.toLocaleString()} bytes (${formatted})`;
    }
    
    return bytes;
}

function initForms() {
    // Create API Key form
    document.getElementById('create-key-form').addEventListener('submit', handleCreateApiKey);
    
    // Change password form
    document.getElementById('change-password-form').addEventListener('submit', handleChangePassword);
    
    // Unlimited traffic toggle
    const unlimitedCheckbox = document.getElementById('unlimited-traffic');
    const trafficLimitGroup = document.getElementById('traffic-limit-group');
    const unlimitedToggle = document.getElementById('unlimited-toggle');
    const trafficDesc = document.getElementById('traffic-desc');
    
    if (unlimitedCheckbox && trafficLimitGroup) {
        unlimitedCheckbox.addEventListener('change', function() {
            if (this.checked) {
                trafficLimitGroup.style.display = 'none';
                unlimitedToggle.classList.add('active');
                trafficDesc.textContent = 'API Key will have no traffic restrictions';
            } else {
                trafficLimitGroup.style.display = 'block';
                unlimitedToggle.classList.remove('active');
                trafficDesc.textContent = 'Set a custom traffic limit below';
                calculateTrafficBytes();
            }
        });
    }
    
    // Traffic value and unit change handlers
    const trafficValueInput = document.getElementById('traffic-limit-value');
    const trafficUnitSelect = document.getElementById('traffic-limit-unit');
    
    if (trafficValueInput) {
        trafficValueInput.addEventListener('input', calculateTrafficBytes);
    }
    if (trafficUnitSelect) {
        trafficUnitSelect.addEventListener('change', calculateTrafficBytes);
    }
    
    // Glob patterns input listener
    const globInput = document.getElementById('glob-patterns');
    if (globInput) {
        globInput.addEventListener('input', updateSelectedPreview);
    }
}

async function handleCreateApiKey(e) {
    e.preventDefault();
    
    // Get selected models from checkboxes
    const modelArray = Array.from(selectedModels);
    
    // Get glob patterns
    const globInput = document.getElementById('glob-patterns');
    const globPatterns = globInput ? globInput.value.trim() : '';
    const globArray = globPatterns ? globPatterns.split(',').map(p => p.trim()).filter(p => p) : [];
    
    // Combine models and glob patterns
    const allModels = [...modelArray, ...globArray];
    
    const unlimitedCheckbox = document.getElementById('unlimited-traffic');
    const isUnlimited = unlimitedCheckbox ? unlimitedCheckbox.checked : true;
    
    // Calculate traffic limit using new method
    const trafficLimit = calculateTrafficBytes();
    
    if (allModels.length === 0) {
        showAlert('create-key-alert', 'Please select at least one model or enter a glob pattern', 'error');
        return;
    }
    
    // Validate traffic limit if not unlimited
    if (!isUnlimited && trafficLimit <= 0) {
        showAlert('create-key-alert', 'Please enter a valid traffic limit or enable unlimited traffic', 'error');
        return;
    }
    
    try {
        const requestBody = {
            allowed_models: allModels,
            unlimited: isUnlimited
        };
        
        if (!isUnlimited && trafficLimit > 0) {
            requestBody.traffic_limit = trafficLimit;
        }
        
        const response = await fetch('/ops/api/create-api-key', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(requestBody)
        });
        
        const data = await response.json();
        
        if (data.success) {
            const limitInfo = data.unlimited ? 'Unlimited' : formatBytes(data.traffic_limit);
            showAlert('create-key-alert', `API Key created successfully! Traffic: ${limitInfo}`, 'success');
            document.getElementById('generated-key').textContent = data.api_key;
            document.getElementById('api-key-result').classList.add('show');
            
            // Clear form (wrap in try-catch to not affect success message)
            try {
                selectedModels.clear();
                renderModelList();
                if (globInput) globInput.value = '';
                updateSelectedPreview();
            } catch (clearError) {
                console.error('Error clearing form:', clearError);
            }
            
            // Refresh user info to update stats (async, don't wait)
            loadUserInfo().catch(err => console.error('Error refreshing user info:', err));
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
    
    const opsKey = userInfo.ops_key;
    const endpoint = window.location.origin;
    
    // Update all dynamic OPS key placeholders
    document.querySelectorAll('.ops-key-dynamic').forEach(el => {
        el.textContent = opsKey;
    });
    
    // Update all dynamic endpoint placeholders
    document.querySelectorAll('.api-endpoint-dynamic').forEach(el => {
        el.textContent = endpoint;
    });
}

function copyCurlExample(elementId) {
    const endpoint = window.location.origin;
    const opsKey = userInfo ? userInfo.ops_key : 'YOUR_OPS_KEY';
    
    // Define curl commands for each example
    const curlCommands = {
        'curl-create-limited': `curl -X POST '${endpoint}/ops/api/create-api-key' \\
  -H 'Content-Type: application/json' \\
  -H 'X-Ops-Key: ${opsKey}' \\
  -d '{
    "allowed_models": ["gpt-4", "gpt-3.5-turbo"],
    "traffic_limit": 52428800
  }'`,
        'curl-create-unlimited': `curl -X POST '${endpoint}/ops/api/create-api-key' \\
  -H 'Content-Type: application/json' \\
  -H 'X-Ops-Key: ${opsKey}' \\
  -d '{
    "allowed_models": ["gpt-4", "gpt-3.5-turbo", "claude-*"],
    "unlimited": true
  }'`,
        'curl-create-glob': `curl -X POST '${endpoint}/ops/api/create-api-key' \\
  -H 'Content-Type: application/json' \\
  -H 'X-Ops-Key: ${opsKey}' \\
  -d '{
    "allowed_models": ["gpt-*", "claude-*", "memfit-*"],
    "traffic_limit": 104857600
  }'`,
        'curl-list-keys': `curl -X GET '${endpoint}/ops/api/my-keys' \\
  -H 'X-Ops-Key: ${opsKey}'`,
        'curl-update-key': `curl -X POST '${endpoint}/ops/api/update-api-key' \\
  -H 'Content-Type: application/json' \\
  -H 'X-Ops-Key: ${opsKey}' \\
  -d '{
    "api_key": "mf-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
    "allowed_models": ["gpt-4", "claude-*"],
    "traffic_limit": 209715200,
    "unlimited": false
  }'`,
        'curl-delete-key': `curl -X POST '${endpoint}/ops/api/delete-api-key' \\
  -H 'Content-Type: application/json' \\
  -H 'X-Ops-Key: ${opsKey}' \\
  -d '{
    "api_key": "mf-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  }'`,
        'curl-chat': `curl -X POST '${endpoint}/v1/chat/completions' \\
  -H 'Content-Type: application/json' \\
  -H 'Authorization: Bearer YOUR_GENERATED_API_KEY' \\
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'`
    };
    
    const curlCommand = curlCommands[elementId] || curlCommands['curl-create-limited'];
    
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
window.selectAllModels = selectAllModels;
window.clearAllModels = clearAllModels;
window.toggleModel = toggleModel;
// Edit API Key functions
window.openEditKeyModal = openEditKeyModal;
window.closeEditKeyModal = closeEditKeyModal;
window.editToggleModel = editToggleModel;
window.editSelectAllModels = editSelectAllModels;
window.editClearAllModels = editClearAllModels;
window.saveEditKey = saveEditKey;
window.resetEditKeyTraffic = resetEditKeyTraffic;
// My Keys pagination functions
window.changeMyKeysPage = changeMyKeysPage;
window.changeMyKeysPageSize = changeMyKeysPageSize;
