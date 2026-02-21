        // 全局变量
        let resizeTimer;
        let currentContextMenuProviderId = null; // For provider actions
        let contextApiIdForEdit = null;          // For API key model editing
        let contextModelsForEdit = null;         // For API key model editing
        let isHealthCheckInProgress = false;
        let isProviderConfigValidated = false; // For add provider form validation
        
        // 全局数据存储
        let portalData = null;
        
        // API Keys 分页状态
        let apiKeysPage = 1;
        let apiKeysPageSize = 20;
        let apiKeysPagination = null;
        let apiKeysData = [];
        
        // ==================== Authentication Error Handler ====================
        
        // Check if response indicates authentication error
        function isAuthError(data) {
            if (!data) return false;
            // Check for permission denied errors
            if (data.error === 'Permission denied' || 
                data.error === 'Unauthorized' ||
                data.reason === 'insufficient permissions' ||
                data.error === 'Admin access required' ||
                data.error === 'Authentication required') {
                return true;
            }
            return false;
        }
        
        // Clear authentication cookies and redirect to login
        function handleAuthError() {
            console.warn('Authentication expired or invalid, redirecting to login...');
            // Clear admin_session cookie
            document.cookie = 'admin_session=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT;';
            // Show a brief message before redirecting
            alert('Session expired, please login again.');
            // Redirect to login page
            window.location.href = '/portal/login';
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
        
        // 模型选择相关
        let portalAvailableModels = [];
        let portalSelectedModels = new Set();
        let editModalSelectedModels = new Set();

        // ==================== Portal Data Loader ====================
        
        const PortalDataLoader = {
            // 加载所有页面数据
            loadData: async function() {
                try {
                    const response = await authFetch('/portal/api/data');
                    if (!response) return null; // Auth error handled
                    if (!response.ok) {
                        throw new Error(`HTTP error! status: ${response.status}`);
                    }
                    portalData = await response.json();
                    // Check for auth error in response body
                    if (checkAuthInResponse(portalData)) return null;
                    console.log('Portal data loaded:', portalData);
                    return portalData;
                } catch (error) {
                    console.error('Failed to load portal data:', error);
                    throw error;
                }
            },
            
            // 渲染统计卡片
            renderStats: function(data) {
                document.getElementById('current-time').textContent = data.current_time;
                document.getElementById('stat-total-providers').textContent = data.total_providers;
                document.getElementById('stat-healthy-providers').textContent = data.healthy_providers;
                document.getElementById('stat-total-requests').textContent = data.total_requests;
                document.getElementById('stat-success-rate').textContent = data.success_rate.toFixed(2) + '%';
                document.getElementById('stat-total-traffic').textContent = data.total_traffic_str;
                document.getElementById('stat-concurrent-requests').textContent = data.concurrent_requests || 0;
                document.getElementById('stat-web-search-count').textContent = data.web_search_count || 0;
                document.getElementById('stat-amap-count').textContent = data.amap_count || 0;
            },
            
            // 渲染供应商表格
            renderProviders: function(data) {
                const tbody = document.getElementById('provider-table-body');
                if (!tbody) return;
                
                tbody.innerHTML = '';
                
                if (!data.providers || data.providers.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="10" class="text-center">No providers found</td></tr>';
                    return;
                }
                
                data.providers.forEach(p => {
                    const row = document.createElement('tr');
                    row.dataset.id = p.id;
                    row.dataset.status = p.health_status_class;
                    
                    let healthBadge = '';
                    let latencyDisplay = '';
                    if (p.health_status_class === 'healthy') {
                        healthBadge = '<span class="health-badge healthy">健康</span>';
                        latencyDisplay = `<span class="health-latency">${p.last_latency}ms</span>`;
                    } else if (p.health_status_class === 'unhealthy') {
                        healthBadge = '<span class="health-badge unhealthy">异常</span>';
                        latencyDisplay = `<span class="health-latency">${p.last_latency > 0 ? p.last_latency + 'ms' : '-'}</span>`;
                    } else {
                        healthBadge = '<span class="health-badge unhealthy">未知</span>';
                        latencyDisplay = '<span class="health-latency">-</span>';
                    }
                    
                    row.innerHTML = `
                        <td class="checkbox-column">
                            <input type="checkbox" class="provider-checkbox">
                        </td>
                        <td class="text-center">${p.id}</td>
                        <td class="health-cell">
                            <div class="health-status">
                                <button class="refresh-btn" onclick="checkSingleProvider('${p.id}')" title="刷新健康状态">
                                    <svg viewBox="0 0 24 24">
                                        <path d="M17.65 6.35C16.2 4.9 14.21 4 12 4c-4.42 0-7.99 3.58-7.99 8s3.57 8 7.99 8c3.73 0 6.84-2.55 7.73-6h-2.08c-.82 2.33-3.04 4-5.65 4-3.31 0-6-2.69-6-6s2.69-6 6-6c1.66 0 3.14.69 4.22 1.78L13 11h7V4l-2.35 2.35z"/>
                                    </svg>
                                </button>
                                <div class="health-info">
                                    ${healthBadge}
                                    ${latencyDisplay}
                                </div>
                            </div>
                        </td>
                        <td class="copyable text-center" data-full-text="${this.escapeHtml(p.wrapper_name)}">${this.escapeHtml(p.wrapper_name)}</td>
                        <td class="copyable" data-full-text="${this.escapeHtml(p.model_name)}">${this.escapeHtml(p.model_name)}</td>
                        <td class="copyable text-center" data-full-text="${this.escapeHtml(p.type_name)}">${this.escapeHtml(p.type_name)}</td>
                        <td class="copyable" data-full-text="${this.escapeHtml(p.domain_or_url)}">${this.escapeHtml(p.domain_or_url)}</td>
                        <td class="copyable api-key-cell" data-full-text="${this.escapeHtml(p.api_key)}">
                            <div class="api-key-container">
                                <span class="api-key-display">${p.api_key ? '•••••••' : ''}</span>
                                <button class="btn btn-sm btn-copy" onclick="copyToClipboard('${this.escapeHtml(p.api_key)}')" title="复制 API Key">
                                    <svg viewBox="0 0 24 24" width="14" height="14">
                                        <path fill="currentColor" d="M16 1H4c-1.1 0-2 .9-2 2v14h2V3h12V1zm3 4H8c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h11c1.1 0 2-.9 2-2V7c0-1.1-.9-2-2-2zm0 16H8V7h11v14z"/>
                                    </svg>
                                </button>
                            </div>
                        </td>
                        <td>${p.total_requests}</td>
                        <td>
                            <div class="provider-actions">
                                <button class="btn btn-sm btn-quick-add" onclick="quickAddProvider('${p.id}')" title="快速添加">
                                    <svg viewBox="0 0 24 24" width="14" height="14">
                                        <path fill="currentColor" d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z"/>
                                    </svg>
                                </button>
                                <button class="delete-btn" onclick="deleteProvider('${p.id}')" title="删除提供者">
                                    <svg viewBox="0 0 24 24">
                                        <path d="M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/>
                                    </svg>
                                </button>
                            </div>
                        </td>
                    `;
                    
                    tbody.appendChild(row);
                });
                
                // Re-apply current filter
                const activeFilter = document.querySelector('.filter-buttons .filter-btn.active');
                if (activeFilter) {
                    filterProviders(activeFilter.dataset.filter);
                }
                
                // 重新绑定复选框事件监听器（因为复选框是动态生成的）
                this.bindProviderCheckboxEvents();
            },
            
            // 绑定 Provider 复选框事件和右键菜单事件
            bindProviderCheckboxEvents: function() {
                // 绑定复选框事件
                document.querySelectorAll('.provider-checkbox').forEach(checkbox => {
                    // 移除旧的监听器（如果有的话）
                    checkbox.removeEventListener('change', updateDeleteSelectedButton);
                    // 添加新的监听器
                    checkbox.addEventListener('change', updateDeleteSelectedButton);
                });
                
                // 绑定右键菜单事件到 Provider 行
                document.querySelectorAll('#provider-table-body tr[data-id]').forEach(row => {
                    row.removeEventListener('contextmenu', showContextMenu);
                    row.addEventListener('contextmenu', showContextMenu);
                });
            },
            
            // 渲染 API 密钥表格
            renderAPIKeys: function(data) {
                const tbody = document.getElementById('api-table-body');
                if (!tbody) return;
                
                tbody.innerHTML = '';
                
                if (!data.api_keys || data.api_keys.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="12" class="text-center">No API keys found</td></tr>';
                    return;
                }
                
                data.api_keys.forEach(key => {
                    const row = document.createElement('tr');
                    row.dataset.apiId = key.id;
                    row.dataset.apiStatus = key.active ? 'active' : 'inactive';
                    row.dataset.trafficLimit = key.traffic_limit;
                    row.dataset.trafficUsed = key.traffic_used;
                    row.dataset.trafficEnabled = key.traffic_limit_enable;
                    
                    let statusBadge = key.active 
                        ? '<span class="health-badge healthy" style="font-size:12px;">激活</span>'
                        : '<span class="health-badge unhealthy" style="font-size:12px;">禁用</span>';
                    
                    let trafficLimitCell = '';
                    if (key.traffic_limit_enable) {
                        const percent = key.traffic_percent;
                        let barColor = '#4caf50';
                        if (percent > 90) barColor = '#f44336';
                        else if (percent > 70) barColor = '#ff9800';
                        
                        trafficLimitCell = `
                            <div class="traffic-limit-info" title="已用/限额: ${key.traffic_used_formatted}/${key.traffic_limit_formatted} (${percent.toFixed(1)}%)">
                                <div class="traffic-progress" style="width: 80px; height: 8px; background: #e0e0e0; border-radius: 4px; overflow: hidden;">
                                    <div style="width: ${Math.min(percent, 100)}%; height: 100%; background: ${barColor};"></div>
                                </div>
                                <small>${key.traffic_used_formatted}/${key.traffic_limit_formatted}</small>
                            </div>
                        `;
                    } else {
                        trafficLimitCell = '<span style="color: #999;">未限制</span>';
                    }
                    
                    let actionButtons = key.active
                        ? `<button class="btn btn-sm btn-danger" onclick="toggleAPIKeyStatus('${key.id}', false)" title="禁用" style="padding:2px 4px;font-size:11px;">禁用</button>`
                        : `<button class="btn btn-sm" onclick="toggleAPIKeyStatus('${key.id}', true)" title="激活" style="padding:2px 4px;font-size:11px;">激活</button>`;
                    
                    // Get creator display name
                    const creatorName = key.created_by_ops_name || (key.created_by_ops_id ? 'OPS#' + key.created_by_ops_id : 'Admin');
                    
                    row.innerHTML = `
                        <td class="checkbox-column">
                            <input type="checkbox" class="api-checkbox">
                        </td>
                        <td class="text-center">${key.id}</td>
                        <td class="text-center">${statusBadge}</td>
                        <td class="copyable api-key-cell" data-full-text="${this.escapeHtml(key.key)}">${this.escapeHtml(key.display_key)}</td>
                        <td class="copyable editable-allowed-models" data-api-id="${key.id}" data-current-models="${this.escapeHtml(key.allowed_models)}" data-full-text="${this.escapeHtml(key.allowed_models)}" title="右键点击修改允许的模型">${this.escapeHtml(key.allowed_models)}</td>
                        <td class="text-center">${key.usage_count}</td>
                        <td class="text-center">${key.web_search_count || 0}</td>
                        <td class="text-center">
                            <span class="health-badge healthy">${key.success_count}</span>
                            <span class="health-badge unhealthy">${key.failure_count}</span>
                        </td>
                        <td class="text-center">
                            <div class="traffic-data">
                                <span title="输入流量">↓ ${key.input_bytes_formatted}</span>
                                <span title="输出流量">↑ ${key.output_bytes_formatted}</span>
                            </div>
                        </td>
                        <td class="text-center">${trafficLimitCell}</td>
                        <td class="text-center">${this.escapeHtml(creatorName)}</td>
                        <td>${key.last_used_at || '-'}</td>
                        <td class="text-center">
                            <div style="display: flex; gap: 2px; justify-content: center; flex-wrap: wrap;">
                                ${actionButtons}
                                <button class="btn btn-sm" onclick="showTrafficLimitDialog(${key.id}, ${key.traffic_limit}, ${key.traffic_used}, ${key.traffic_limit_enable})" title="流量设置" style="padding:2px 4px;font-size:11px;">流量</button>
                                <button class="btn btn-sm btn-danger" onclick="deleteAPIKey(${key.id})" title="删除" style="padding:2px 4px;font-size:11px;">删除</button>
                            </div>
                        </td>
                    `;
                    
                    tbody.appendChild(row);
                });
                
                // 重新绑定 API 复选框事件监听器（因为复选框是动态生成的）
                this.bindAPICheckboxEvents();
            },
            
            // 绑定 API Key 复选框事件和右键菜单事件
            bindAPICheckboxEvents: function() {
                // 绑定复选框事件
                document.querySelectorAll('.api-checkbox').forEach(checkbox => {
                    // 移除旧的监听器（如果有的话）
                    checkbox.removeEventListener('change', updateDeleteSelectedAPIButton);
                    // 添加新的监听器
                    checkbox.addEventListener('change', updateDeleteSelectedAPIButton);
                });
                
                // 绑定右键菜单事件到 "允许模型" 单元格
                document.querySelectorAll('#api-table tbody td.editable-allowed-models').forEach(cell => {
                    cell.removeEventListener('contextmenu', showContextMenu);
                    cell.addEventListener('contextmenu', showContextMenu);
                });
                
                // 绑定点击事件到 API Key 单元格以便复制
                document.querySelectorAll('#api-table tbody td.api-key-cell').forEach(cell => {
                    cell.style.cursor = 'pointer';
                    cell.title = '点击复制完整 API Key';
                    cell.addEventListener('click', function() {
                        const fullKey = this.getAttribute('data-full-text');
                        if (fullKey) {
                            copyToClipboard(fullKey);
                            showToast('API Key 已复制到剪贴板', 'success');
                        }
                    });
                });
            },
            
            // 渲染模型信息表格
            renderModels: function(data) {
                const tbody = document.getElementById('models-table-body');
                if (!tbody) return;
                
                tbody.innerHTML = '';
                
                if (!data.model_metas || data.model_metas.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="6" class="text-center">No models found</td></tr>';
                    return;
                }
                
                data.model_metas.forEach(model => {
                    const row = document.createElement('tr');
                    row.dataset.modelName = model.name;
                    row.dataset.trafficMultiplier = model.traffic_multiplier.toFixed(2);
                    
                    let badgeColor = '#2196f3';
                    if (model.traffic_multiplier > 1.5) badgeColor = '#ff9800';
                    else if (model.traffic_multiplier > 1.0) badgeColor = '#4caf50';
                    
                    row.innerHTML = `
                        <td class="copyable" data-full-text="${this.escapeHtml(model.name)}">${this.escapeHtml(model.name)}</td>
                        <td class="text-center">${model.provider_count}</td>
                        <td class="text-center">
                            <span class="traffic-multiplier-badge" style="background: ${badgeColor}; color: white; padding: 2px 8px; border-radius: 10px; font-size: 12px;" title="流量消耗将乘以此倍数">
                                x${model.traffic_multiplier.toFixed(2)}
                            </span>
                        </td>
                        <td class="copyable" data-full-text="${this.escapeHtml(model.description || '')}">${model.description || '-'}</td>
                        <td class="copyable" data-full-text="${this.escapeHtml(model.tags || '')}">${model.tags || '-'}</td>
                        <td class="text-center">
                            <button class="btn btn-sm" onclick="openEditModelModal('${this.escapeHtml(model.name)}', '${this.escapeHtml(model.description || '')}', '${this.escapeHtml(model.tags || '')}', ${model.traffic_multiplier})" title="编辑模型信息">
                                编辑
                            </button>
                            <button class="btn btn-sm" style="background-color: #4caf50; margin-left: 5px;" onclick="showCurlCommand('${this.escapeHtml(model.name)}')" title="查看 curl 命令">
                                curl
                            </button>
                        </td>
                    `;
                    
                    tbody.appendChild(row);
                });
            },
            
            // 渲染 TOTP 数据
            renderTOTP: function(data) {
                const secretEl = document.getElementById('totp-secret');
                const wrappedEl = document.getElementById('totp-wrapped');
                const codeEl = document.getElementById('totp-code');
                
                if (secretEl) secretEl.textContent = data.totp_secret || '--';
                if (wrappedEl) wrappedEl.textContent = data.totp_wrapped || '--';
                if (codeEl) codeEl.textContent = data.totp_code || '--';
            },
            
            // 填充模型选择组件
            populateModelSelect: function(data) {
                const modelList = document.getElementById('portalModelList');
                if (!modelList) return;
                
                // 从 providers 中提取唯一的模型名
                const modelSet = new Set();
                if (data.providers) {
                    data.providers.forEach(p => {
                        const name = p.wrapper_name || p.model_name;
                        if (name) modelSet.add(name);
                    });
                }
                
                // 排序并存储
                portalAvailableModels = Array.from(modelSet).sort();
                portalSelectedModels.clear();
                
                // 渲染模型列表
                portalRenderModelList();
                
                // 设置 glob input 监听
                const globInput = document.getElementById('portalGlobPatterns');
                if (globInput) {
                    globInput.addEventListener('input', portalUpdateSelectedPreview);
                }
            },
            
            // HTML 转义
            escapeHtml: function(str) {
                if (!str) return '';
                return String(str)
                    .replace(/&/g, '&amp;')
                    .replace(/</g, '&lt;')
                    .replace(/>/g, '&gt;')
                    .replace(/"/g, '&quot;')
                    .replace(/'/g, '&#39;');
            },
            
            // 隐藏 loading overlay
            hideLoading: function() {
                const overlay = document.getElementById('loading-overlay');
                if (overlay) {
                    overlay.style.opacity = '0';
                    setTimeout(() => {
                        overlay.style.display = 'none';
                    }, 300);
                }
            },
            
            // 显示 loading overlay
            showLoading: function() {
                const overlay = document.getElementById('loading-overlay');
                if (overlay) {
                    overlay.style.display = 'flex';
                    overlay.style.opacity = '1';
                }
            },
            
            // 初始化并渲染所有数据
            init: async function() {
                try {
                    const data = await this.loadData();
                    
                    // Auth error was handled, data is null
                    if (!data) {
                        this.hideLoading();
                        return;
                    }
                    
                    this.renderStats(data);
                    this.renderProviders(data);
                    // Use paginated API for API keys instead of bulk loading
                    loadAPIKeysPaginated(1, apiKeysPageSize);
                    this.renderModels(data);
                    this.renderTOTP(data);
                    this.populateModelSelect(data);
                    
                    this.hideLoading();
                    
                    console.log('Portal data rendering complete');
                } catch (error) {
                    console.error('Failed to initialize portal:', error);
                    this.hideLoading();
                    showToast('Failed to load portal data: ' + error.message, 'error');
                }
            },
            
            // 刷新数据
            refresh: async function() {
                try {
                    this.showLoading();
                    await this.init();
                } catch (error) {
                    console.error('Failed to refresh portal data:', error);
                }
            }
        };
        
        // 页面加载完成后初始化
        document.addEventListener('DOMContentLoaded', function() {
            PortalDataLoader.init();
        });
        
        // 全局刷新函数
        window.refreshPortalData = function() {
            PortalDataLoader.refresh();
        };

        // ==================== API Keys Pagination ====================
        
        // Load API keys with pagination
        async function loadAPIKeysPaginated(page = 1, pageSize = 20) {
            try {
                const url = `/portal/api/api-keys?page=${page}&pageSize=${pageSize}&sortBy=created_at&sortOrder=desc`;
                const response = await authFetch(url);
                if (!response) return; // Auth error handled
                
                const data = await response.json();
                if (checkAuthInResponse(data)) return;
                
                if (data.success) {
                    apiKeysData = data.data || [];
                    apiKeysPagination = data.pagination || null;
                    apiKeysPage = page;
                    apiKeysPageSize = pageSize;
                    
                    // Render the table with paginated data
                    renderAPIKeysTablePaginated(apiKeysData);
                    renderAPIKeysPagination();
                } else {
                    console.error('Failed to load API keys:', data.message);
                    showToast('Failed to load API keys: ' + (data.message || 'Unknown error'), 'error');
                }
            } catch (error) {
                console.error('Error loading API keys:', error);
                showToast('Error loading API keys', 'error');
            }
        }
        
        // Render API keys table with paginated data
        function renderAPIKeysTablePaginated(keys) {
            const tbody = document.getElementById('api-table-body');
            if (!tbody) return;
            
            tbody.innerHTML = '';
            
            if (!keys || keys.length === 0) {
                tbody.innerHTML = '<tr><td colspan="12" class="text-center">No API keys found</td></tr>';
                return;
            }
            
            keys.forEach(key => {
                const row = document.createElement('tr');
                row.dataset.apiId = key.id;
                row.dataset.apiStatus = key.active ? 'active' : 'inactive';
                row.dataset.trafficLimit = key.traffic_limit;
                row.dataset.trafficUsed = key.traffic_used;
                row.dataset.trafficEnabled = key.traffic_limit_enable;
                
                let statusBadge = key.active 
                    ? '<span class="health-badge healthy" style="font-size:12px;">激活</span>'
                    : '<span class="health-badge unhealthy" style="font-size:12px;">禁用</span>';
                
                // Format traffic data
                const inputFormatted = formatBytes(key.input_bytes || 0);
                const outputFormatted = formatBytes(key.output_bytes || 0);
                
                let trafficLimitCell = '';
                if (key.traffic_limit_enable) {
                    const percent = key.traffic_limit > 0 ? (key.traffic_used / key.traffic_limit * 100) : 0;
                    let barColor = '#4caf50';
                    if (percent > 90) barColor = '#f44336';
                    else if (percent > 70) barColor = '#ff9800';
                    
                    const usedFormatted = formatBytes(key.traffic_used || 0);
                    const limitFormatted = formatBytes(key.traffic_limit || 0);
                    
                    trafficLimitCell = `
                        <div class="traffic-limit-info" title="已用/限额: ${usedFormatted}/${limitFormatted} (${percent.toFixed(1)}%)">
                            <div class="traffic-progress" style="width: 80px; height: 8px; background: #e0e0e0; border-radius: 4px; overflow: hidden;">
                                <div style="width: ${Math.min(percent, 100)}%; height: 100%; background: ${barColor};"></div>
                            </div>
                            <small>${usedFormatted}/${limitFormatted}</small>
                        </div>
                    `;
                } else {
                    trafficLimitCell = '<span style="color: #999;">未限制</span>';
                }
                
                let actionButtons = key.active
                    ? `<button class="btn btn-sm btn-danger" onclick="toggleAPIKeyStatus('${key.id}', false)" title="禁用" style="padding:2px 4px;font-size:11px;">禁用</button>`
                    : `<button class="btn btn-sm" onclick="toggleAPIKeyStatus('${key.id}', true)" title="激活" style="padding:2px 4px;font-size:11px;">激活</button>`;
                
                // Get creator display name
                const creatorName = key.created_by_ops_name || (key.created_by_ops_id ? 'OPS#' + key.created_by_ops_id : 'Admin');
                
                row.innerHTML = `
                    <td class="checkbox-column">
                        <input type="checkbox" class="api-checkbox">
                    </td>
                    <td class="text-center">${key.id}</td>
                    <td class="text-center">${statusBadge}</td>
                    <td class="copyable api-key-cell" data-full-text="${escapeHtml(key.api_key)}">${escapeHtml(key.display_key)}</td>
                    <td class="copyable editable-allowed-models" data-api-id="${key.id}" data-current-models="${escapeHtml(key.allowed_models)}" data-full-text="${escapeHtml(key.allowed_models)}" title="右键点击修改允许的模型">${escapeHtml(key.allowed_models)}</td>
                    <td class="text-center">${key.usage_count || 0}</td>
                    <td class="text-center">${key.web_search_count || 0}</td>
                    <td class="text-center">
                        <span class="health-badge healthy">${key.success_count || 0}</span>
                        <span class="health-badge unhealthy">${key.failure_count || 0}</span>
                    </td>
                    <td class="text-center">
                        <div class="traffic-data">
                            <span title="输入流量">↓ ${inputFormatted}</span>
                            <span title="输出流量">↑ ${outputFormatted}</span>
                        </div>
                    </td>
                    <td class="text-center">${trafficLimitCell}</td>
                    <td class="text-center">${escapeHtml(creatorName)}</td>
                    <td>${key.last_used_time || key.created_at || '-'}</td>
                    <td class="text-center">
                        <div style="display: flex; gap: 2px; justify-content: center; flex-wrap: wrap;">
                            ${actionButtons}
                            <button class="btn btn-sm" onclick="showTrafficLimitDialog(${key.id}, ${key.traffic_limit || 0}, ${key.traffic_used || 0}, ${key.traffic_limit_enable})" title="流量设置" style="padding:2px 4px;font-size:11px;">流量</button>
                            <button class="btn btn-sm btn-danger" onclick="deleteAPIKey(${key.id})" title="删除" style="padding:2px 4px;font-size:11px;">删除</button>
                        </div>
                    </td>
                `;
                
                tbody.appendChild(row);
            });
            
            // Rebind events
            bindAPIKeyEvents();
        }
        
        // Bind API key events (checkboxes, context menu, copy)
        function bindAPIKeyEvents() {
            // Bind checkbox events
            document.querySelectorAll('.api-checkbox').forEach(checkbox => {
                checkbox.removeEventListener('change', updateDeleteSelectedAPIButton);
                checkbox.addEventListener('change', updateDeleteSelectedAPIButton);
            });
            
            // Bind context menu to editable-allowed-models cells
            document.querySelectorAll('#api-table tbody td.editable-allowed-models').forEach(cell => {
                cell.removeEventListener('contextmenu', showContextMenu);
                cell.addEventListener('contextmenu', showContextMenu);
            });
            
            // Bind click to copy API key
            document.querySelectorAll('#api-table tbody td.api-key-cell').forEach(cell => {
                cell.style.cursor = 'pointer';
                cell.title = '点击复制完整 API Key';
                cell.addEventListener('click', function() {
                    const fullKey = this.getAttribute('data-full-text');
                    if (fullKey) {
                        navigator.clipboard.writeText(fullKey).then(() => {
                            showToast('API Key copied to clipboard', 'success');
                        }).catch(err => {
                            console.error('Failed to copy:', err);
                        });
                    }
                });
            });
        }
        
        // Render API keys pagination controls
        function renderAPIKeysPagination() {
            let container = document.getElementById('api-keys-pagination');
            if (!container) {
                // Create pagination container if not exists
                // Find the table container that wraps #api-table
                const apiTable = document.getElementById('api-table');
                const tableContainer = apiTable ? apiTable.closest('.table-container') : null;
                if (tableContainer) {
                    container = document.createElement('div');
                    container.id = 'api-keys-pagination';
                    container.className = 'pagination-container';
                    container.style.cssText = 'margin-top: 15px; display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 10px;';
                    tableContainer.after(container);
                }
            }
            
            if (!container || !apiKeysPagination) return;
            
            const { page, pageSize, total, totalPages } = apiKeysPagination;
            const startItem = (page - 1) * pageSize + 1;
            const endItem = Math.min(page * pageSize, total);
            
            container.innerHTML = `
                <div style="color: #666; font-size: 14px;">
                    显示 ${startItem}-${endItem} 条，共 ${total} 条
                </div>
                <div style="display: flex; align-items: center; gap: 10px;">
                    <select onchange="changeAPIKeysPageSize(this.value)" style="padding: 5px 10px; border: 1px solid #ddd; border-radius: 4px;">
                        <option value="10" ${pageSize == 10 ? 'selected' : ''}>10 条/页</option>
                        <option value="20" ${pageSize == 20 ? 'selected' : ''}>20 条/页</option>
                        <option value="50" ${pageSize == 50 ? 'selected' : ''}>50 条/页</option>
                        <option value="100" ${pageSize == 100 ? 'selected' : ''}>100 条/页</option>
                    </select>
                    <div style="display: flex; gap: 5px;">
                        <button class="btn btn-sm" onclick="changeAPIKeysPage(1)" ${page <= 1 ? 'disabled' : ''}>首页</button>
                        <button class="btn btn-sm" onclick="changeAPIKeysPage(${page - 1})" ${page <= 1 ? 'disabled' : ''}>上一页</button>
                        <span style="padding: 5px 10px; color: #666;">第 ${page} / ${totalPages} 页</span>
                        <button class="btn btn-sm" onclick="changeAPIKeysPage(${page + 1})" ${page >= totalPages ? 'disabled' : ''}>下一页</button>
                        <button class="btn btn-sm" onclick="changeAPIKeysPage(${totalPages})" ${page >= totalPages ? 'disabled' : ''}>末页</button>
                    </div>
                    <div style="display: flex; align-items: center; gap: 5px;">
                        <span style="color: #666;">跳转</span>
                        <input type="number" id="api-keys-page-input" min="1" max="${totalPages}" value="${page}" 
                            style="width: 60px; padding: 5px; border: 1px solid #ddd; border-radius: 4px; text-align: center;"
                            onkeypress="if(event.key==='Enter')jumpToAPIKeysPage()">
                        <span style="color: #666;">页</span>
                        <button class="btn btn-sm" onclick="jumpToAPIKeysPage()">Go</button>
                    </div>
                </div>
            `;
        }
        
        // Change API keys page
        function changeAPIKeysPage(page) {
            if (page < 1) page = 1;
            if (apiKeysPagination && page > apiKeysPagination.totalPages) page = apiKeysPagination.totalPages;
            loadAPIKeysPaginated(page, apiKeysPageSize);
        }
        
        // Change API keys page size
        function changeAPIKeysPageSize(size) {
            apiKeysPageSize = parseInt(size) || 20;
            loadAPIKeysPaginated(1, apiKeysPageSize);
        }
        
        // Jump to specific page
        function jumpToAPIKeysPage() {
            const input = document.getElementById('api-keys-page-input');
            if (input) {
                const page = parseInt(input.value) || 1;
                changeAPIKeysPage(page);
            }
        }
        
        // Helper: format bytes to human readable
        function formatBytes(bytes) {
            if (bytes === 0) return '0 B';
            const k = 1024;
            const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
            const i = Math.floor(Math.log(bytes) / Math.log(k));
            return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
        }
        
        // Helper: escape HTML
        function escapeHtml(str) {
            if (str === null || str === undefined) return '';
            return String(str)
                .replace(/&/g, '&amp;')
                .replace(/</g, '&lt;')
                .replace(/>/g, '&gt;')
                .replace(/"/g, '&quot;')
                .replace(/'/g, '&#039;');
        }
        
        // Export pagination functions
        window.loadAPIKeysPaginated = loadAPIKeysPaginated;
        window.changeAPIKeysPage = changeAPIKeysPage;
        window.changeAPIKeysPageSize = changeAPIKeysPageSize;
        window.jumpToAPIKeysPage = jumpToAPIKeysPage;

        // 初始化 Toast 容器
        if (!document.getElementById('toast-container')) {
            const toastContainer = document.createElement('div');
            toastContainer.id = 'toast-container';
            document.body.appendChild(toastContainer);
        }

        // 标签页切换功能
        function switchTab(tabId) {
            // 更新标签页状态
            document.querySelectorAll('.tab').forEach(tab => {
                tab.classList.remove('active');
                if (tab.getAttribute('data-tab') === tabId) {
                    tab.classList.add('active');
                }
            });

            // 更新内容显示
            document.querySelectorAll('.tab-content').forEach(content => {
                content.classList.remove('active');
            });
            document.getElementById(tabId).classList.add('active');
            
            // Store the active tab ID in localStorage
            localStorage.setItem('activeTabId', tabId);
            console.log(`Switched to tab: ${tabId}, saved to localStorage.`); // Debug log

            // 如果是添加接口标签，显示添加表单
            if (tabId === 'add') {
                showAddProviderForm();
            }
            
            // 切换到运营用户标签时自动刷新数据
            if (tabId === 'ops-users') {
                refreshOpsUsers();
            }
            
            // 切换到操作日志标签时自动刷新数据
            if (tabId === 'ops-logs') {
                refreshOpsLogs();
            }
            
            // 切换到 Web Search 标签时自动刷新数据和全局配置
            if (tabId === 'web-search') {
                refreshWebSearchKeys();
            }
            // 切换到 Amap 标签时自动刷新数据和全局配置
            if (tabId === 'amap') {
                refreshAmapKeys();
                loadAmapConfig();
            }
        }

        // 添加接口表单
        function showAddProviderForm() {
            const addContent = document.getElementById('add');
            if (!addContent) return;

            addContent.innerHTML = `
                <div class="add-provider-form">
                    <div class="form-info">
                        <h3>添加新的AI提供者</h3>
                        <p>您可以在此添加新的AI提供者接口。系统将会为每个API密钥创建一个提供者实例。</p>
                        <div class="tips">
                            <p><strong>提示：</strong></p>
                            <ul>
                                <li>提供者名称：显示给用户的名称，例如 "GPT-4-1106-preview"</li>
                                <li>模型名称：实际调用的模型名称，例如 "gpt-4-1106-preview"</li>
                                <li>类型：提供者类型，如 chat、completion、embedding 等</li>
                                <li>域名/URL：API服务的域名或完整URL，例如 "api.openai.com"</li>
                                <li>API密钥：可输入多个API密钥，每行一个。<strong>验证时将使用第一个密钥。</strong></li>
                            </ul>
                        </div>
                    </div>
                    <form id="addProviderForm" onsubmit="submitAddProvider(event)">
                        <div class="form-row">
                            <div class="form-group">
                                <label for="wrapperName">提供者名称 *</label>
                                <input type="text" id="wrapperName" name="wrapperName" class="form-control autocomplete" 
                                       required placeholder="例如：GPT-4-1106-preview" 
                                       data-autocomplete-type="wrapper_names" list="wrapper-names-list">
                                <datalist id="wrapper-names-list"></datalist>
                            </div>
                            <div class="form-group">
                                <label for="modelName">模型名称 *</label>
                                <input type="text" id="modelName" name="modelName" class="form-control autocomplete" 
                                       required placeholder="例如：gpt-4-1106-preview" 
                                       data-autocomplete-type="model_names" list="model-names-list">
                                <datalist id="model-names-list"></datalist>
                            </div>
                        </div>
                        <div class="form-row">
                            <div class="form-group">
                                <label for="typeName">类型 *</label>
                                <select id="typeName" name="typeName" class="form-control" required>
                                    <option value="">-- 请选择类型 --</option>
                                    <!-- 类型选项将通过JavaScript动态填充 -->
                                </select>
                            </div>
                            <div class="form-group">
                                <label for="providerMode">模式 *</label>
                                <select id="providerMode" name="providerMode" class="form-control" required>
                                    <option value="chat" selected>Chat (对话)</option>
                                    <option value="embedding">Embedding (向量化)</option>
                                </select>
                                <small class="form-text text-muted">选择 Provider 的工作模式</small>
                            </div>
                        </div>
                        <div class="form-row">
                            <div class="form-group">
                                <label for="domainOrURL">域名/URL</label> <!-- 移除 * -->
                                <input type="text" id="domainOrURL" name="domainOrURL" class="form-control autocomplete" 
                                       placeholder="例如：api.openai.com" 
                                       list="domain-urls-list">
                                <datalist id="domain-urls-list"></datalist>
                                <small id="domainOrURL-hint" class="form-text text-muted" style="display: none; color: orange !important;">留空将使用默认直连 URL</small> <!-- 新增提示信息 -->
                            </div>
                        </div>
                        <div class="form-group">
                            <label for="apiKeys">API密钥 * (多个密钥请按行分割)</label>
                            <textarea id="apiKeys" name="apiKeys" class="form-control" rows="4" required placeholder="每行输入一个API密钥，例如：
sk-1234567890abcdef1234567890abcdef
sk-abcdef1234567890abcdef1234567890"></textarea>
                            <small class="form-text text-muted">每行一个API密钥，系统将为每个密钥创建一个提供者实例</small>
                        </div>
                        <div class="form-group">
                            <div class="checkbox">
                                <label>
                                    <input type="checkbox" id="noHTTPS" name="noHTTPS"> 不使用HTTPS (适用于本地或内网服务)
                                </label>
                            </div>
                        </div>
                        <div class="form-group"> <!-- Removed inline flex style -->
                            <button type="button" id="validateConfigBtn" class="btn" style="display: block; width: 100%; margin-bottom: 10px; background-color: #4285f4; color: white; min-width: 120px; height: 40px; font-size: 14px; font-weight: 500; border-radius: 4px; border: none; transition: all 0.3s ease; box-shadow: 0 2px 5px rgba(0,0,0,0.1); padding: 0 15px;">验证配置</button>
                            <button type="submit" id="submitAddProviderBtn" class="btn" disabled style="display: block; width: 100%; background-color: #bdbdbd; color: white; cursor: not-allowed; min-width: 120px; height: 40px; font-size: 14px; font-weight: 500; border-radius: 4px; border: none; transition: all 0.3s ease; box-shadow: 0 1px 3px rgba(0,0,0,0.1); padding: 0 15px;">添加提供者</button>
                        </div>
                        <div id="validationResult" class="validation-message"></div>
                    </form>
                </div>
            `;
            
            // 加载自动补全数据并填充表单
            fillAutoCompleteForm();

            // Add event listeners to form inputs to reset validation status
            const formInputs = ['wrapperName', 'modelName', 'domainOrURL', 'apiKeys'];
            formInputs.forEach(id => {
                const inputElement = document.getElementById(id);
                if (inputElement) {
                    inputElement.addEventListener('input', resetValidationStatus);
                }
            });
            const selectElement = document.getElementById('typeName');
            if (selectElement) {
                selectElement.addEventListener('change', resetValidationStatus);
            }
            const checkboxElement = document.getElementById('noHTTPS');
            if (checkboxElement) {
                checkboxElement.addEventListener('change', resetValidationStatus);
            }

            // Add event listener for the validate button
            const validateBtn = document.getElementById('validateConfigBtn');
            if (validateBtn) {
                validateBtn.addEventListener('click', validateProviderConfiguration);
            }
        }

        // 全局变量存储自动补全数据
        let autoCompleteData = {
            wrapper_names: [],
            model_names: [],
            model_types: [],
            domain_or_urls: [] // 添加 domain_or_urls
        };

        // 加载自动补全数据
        async function loadAutoCompleteData() {
            try {
                const response = await fetch('/portal/autocomplete');
                if (!response.ok) {
                    throw new Error('无法获取自动补全数据');
                }

                const data = await response.json();
                console.log("Received autocomplete data from backend:", data); // Debug log

                // 存储数据到全局变量
                autoCompleteData.wrapper_names = data.wrapper_names || [];
                autoCompleteData.model_names = data.model_names || [];
                autoCompleteData.model_types = data.model_types || [];
                autoCompleteData.domain_or_urls = data.domain_or_urls || []; // 获取 domain_or_urls
                console.log("Processed domain_or_urls:", autoCompleteData.domain_or_urls); // Debug log

                // 填充当前打开的表单（如果有）
                if (document.querySelector('.tab.active[data-tab="add"]')) {
                    fillAutoCompleteForm();
                }
            } catch (error) {
                console.error('加载自动补全数据失败:', error);
            }

            // 新增：填充 Domain/URL 选项
            const domainUrlsList = document.getElementById('domain-urls-list');
            if (domainUrlsList) {
                domainUrlsList.innerHTML = ''; // 清空现有选项
                console.log("Populating domain-urls-list with:", autoCompleteData.domain_or_urls); // Debug log
                autoCompleteData.domain_or_urls.forEach(url => {
                    const option = document.createElement('option');
                    option.value = url;
                    domainUrlsList.appendChild(option);
                });
                console.log("Finished populating domain-urls-list. Current innerHTML:", domainUrlsList.innerHTML); // Debug log
            } else {
                console.error("Could not find datalist element with ID 'domain-urls-list'"); // Debug log
            }

            // 填充类型选择框 - 从后端获取所有支持的 AI 类型
            const typeNameSelect = document.getElementById('typeName');
            if (typeNameSelect) {
                // 保留第一个空选项
                typeNameSelect.innerHTML = '';
                
                // 添加默认提示选项
                const defaultOption = document.createElement('option');
                defaultOption.value = '';
                defaultOption.textContent = '-- 请选择类型 --';
                typeNameSelect.appendChild(defaultOption);
                
                // 添加从服务器获取的类型选项
                if (autoCompleteData.model_types && autoCompleteData.model_types.length > 0) {
                    autoCompleteData.model_types.forEach(type => {
                        const option = document.createElement('option');
                        option.value = type;
                        option.textContent = type;
                        typeNameSelect.appendChild(option);
                    });
                    
                    // 默认选择 openai（如果存在）
                    if (autoCompleteData.model_types.includes('openai')) {
                        typeNameSelect.value = 'openai';
                    }
                } else {
                    // 后端未返回数据时，添加一些常见类型作为默认选项
                    const defaultTypes = ['openai', 'siliconflow', 'tongyi', 'moonshot', 'chatglm', 'deepseek', 'gemini', 'ollama'];
                    defaultTypes.forEach(type => {
                        const option = document.createElement('option');
                        option.value = type;
                        option.textContent = type;
                        typeNameSelect.appendChild(option);
                    });
                    // 默认选择 openai
                    typeNameSelect.value = 'openai';
                }
            }
            
            // 添加输入事件处理器
            const domainInput = document.getElementById('domainOrURL');
            const providerModeSelect = document.getElementById('providerMode');
            if (domainInput) {
                // 根据选择的类型预填充常见域名和联动模式
                document.getElementById('typeName').addEventListener('change', function() {
                    const selectedType = this.value.toLowerCase();
                    let suggestedDomain = '';
                    
                    // 根据类型提供默认域名建议
                    const domainSuggestions = {
                        'openai': 'api.openai.com',
                        'siliconflow': 'api.siliconflow.cn',
                        'tongyi': '', // 通义不需要域名
                        'moonshot': 'api.moonshot.cn',
                        'deepseek': 'api.deepseek.com',
                        'gemini': '', // Gemini 使用 Google API
                        'ollama': 'localhost:11434',
                        'chatglm': 'open.bigmodel.cn'
                    };
                    
                    if (domainSuggestions[selectedType] !== undefined) {
                        suggestedDomain = domainSuggestions[selectedType];
                    }
                    
                    // 如果域名输入框为空，则填充默认值
                    if (!domainInput.value.trim() && suggestedDomain) {
                        domainInput.value = suggestedDomain;
                    }
                    
                    // 类型和模式联动：大多数类型使用 chat 模式
                    // 如果类型名中包含 embedding 则自动选择 embedding 模式
                    if (providerModeSelect) {
                        if (selectedType.includes('embedding')) {
                            providerModeSelect.value = 'embedding';
                        } else {
                            providerModeSelect.value = 'chat';
                        }
                    }
                });
            }
            
            // 添加实时表单验证
            setupFormValidation();
        }
        
        // 设置表单验证
        function setupFormValidation() {
            const form = document.getElementById('addProviderForm');
            if (!form) return;
            
            const inputs = form.querySelectorAll('input[required], select[required], textarea[required]');
            
            inputs.forEach(input => {
                // 初始状态移除验证类
                input.classList.remove('is-valid', 'is-invalid');
                
                // 添加事件监听器
                input.addEventListener('input', function() { validateInput.call(this); resetValidationStatus(); }); // Also reset validation
                input.addEventListener('blur', function() { validateInput.call(this); }); // Don't reset on blur unless value changes (handled by input)
                
                if (input.tagName === 'SELECT') {
                    input.addEventListener('change', function() { validateInput.call(this); resetValidationStatus(); }); // Also reset validation
                }
            });
            
            // 如果已经有值，立即验证
            inputs.forEach(input => {
                if (input.value.trim()) {
                    validateInput.call(input);
                }
            });

            // Checkbox validation reset
            const noHTTPSCheckbox = document.getElementById('noHTTPS');
            if (noHTTPSCheckbox) {
                noHTTPSCheckbox.addEventListener('change', resetValidationStatus);
            }
        }
        
        // 验证单个输入项
        function validateInput() {
            if (this.hasAttribute('required')) {
                const value = this.value.trim();
                
                if (value === '') {
                    this.classList.remove('is-valid');
                    this.classList.add('is-invalid');
                } else {
                    this.classList.remove('is-invalid');
                    this.classList.add('is-valid');
                }
            }
            
            // 特殊验证逻辑
            if (this.id === 'apiKeys') {
                const keys = this.value.split('\n')
                    .map(key => key.trim())
                    .filter(key => key.length > 0);
                
                if (keys.length === 0) {
                    this.classList.remove('is-valid');
                    this.classList.add('is-invalid');
                } else {
                    this.classList.remove('is-invalid');
                    this.classList.add('is-valid');
                }
            }

            // 特殊处理 domainOrURL 字段
            if (this.id === 'domainOrURL') {
                const hintElement = document.getElementById('domainOrURL-hint');
                if (this.value.trim() === '') {
                    this.classList.remove('is-valid', 'is-invalid'); // 为空时移除验证状态
                    if (hintElement) hintElement.style.display = 'block'; // 显示提示
                } else {
                    this.classList.remove('is-invalid'); // 非空时移除无效状态
                    this.classList.add('is-valid');    // 非空时标记为有效
                    if (hintElement) hintElement.style.display = 'none';  // 隐藏提示
                }
            }
        }

        // 提交添加接口表单
        async function submitAddProvider(event) {
            event.preventDefault();
            const form = document.getElementById('addProviderForm');
            
            // 收集表单数据
            const wrapperName = document.getElementById('wrapperName').value.trim();
            const modelName = document.getElementById('modelName').value.trim();
            const typeName = document.getElementById('typeName').value.trim();
            const providerMode = document.getElementById('providerMode').value.trim();
            const domainOrURL = document.getElementById('domainOrURL').value.trim();
            const apiKeys = document.getElementById('apiKeys').value;
            const noHTTPS = document.getElementById('noHTTPS').checked;
            
            // 日志输出表单数据（方便调试）
            console.log('Submitting data:', { // Use common/log - Debug log
                wrapper_name: wrapperName,
                model_name: modelName,
                model_type: typeName,
                provider_mode: providerMode,
                domain_or_url: domainOrURL, // Now can be empty
                api_keys: apiKeys,
                no_https: noHTTPS ? 'on' : ''
            });
            
            // 验证必填字段 (移除对 domainOrURL 的检查)
            if (!wrapperName || !modelName || !typeName || !providerMode || !apiKeys) {
                showToast('请填写所有带 * 的必填字段', 'error');
                return;
            }
            
            // 解析API密钥
            const apiKeysList = apiKeys.split('\n')
                .map(key => key.trim())
                .filter(key => key.length > 0);
            
            if (apiKeysList.length === 0) {
                showToast('请至少提供一个有效的API密钥', 'error');
                return;
            }

            // 显示进度提示
            showToast('正在添加提供者...', 'info');
            
            try {
                // 创建URL编码的表单数据
                const params = new URLSearchParams();
                params.append('wrapper_name', wrapperName);
                params.append('model_name', modelName);
                params.append('model_type', typeName);
                params.append('provider_mode', providerMode);
                params.append('domain_or_url', domainOrURL);
                params.append('api_keys', apiKeys);
                if (noHTTPS) {
                    params.append('no_https', 'on');
                }
                
                // 发送请求
                const response = await fetch('/portal/add-providers', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/x-www-form-urlencoded'
                    },
                    body: params
                });
                
                if (!response.ok) {
                    const errorText = await response.text();
                    throw new Error(`服务器返回错误(${response.status}): ${errorText}`);
                }
                
                showToast('成功添加提供者', 'success');
                setTimeout(() => window.location.reload(), 1000);
            } catch (error) {
                showToast('添加失败: ' + error.message, 'error');
                console.error('添加提供者失败:', error);
            }
        }

        // 多选功能
        function toggleSelectAll() {
            const selectAllCheckbox = document.getElementById('select-all-header'); // Use the header checkbox ID
            if (!selectAllCheckbox) return;
            // Select only visible checkboxes
            const checkboxes = document.querySelectorAll('#provider-table-body tr:not([style*="display: none"]) .provider-checkbox');
            checkboxes.forEach(checkbox => {
                checkbox.checked = selectAllCheckbox.checked;
            });
            updateDeleteSelectedButton();
        }

        function updateDeleteSelectedButton() {
            const deleteButton = document.getElementById('delete-selected');
            if (!deleteButton) return;
            // Count only visible checked checkboxes
            const selectedCount = document.querySelectorAll('#provider-table-body tr:not([style*="display: none"]) .provider-checkbox:checked').length;
            deleteButton.disabled = selectedCount === 0;
        }

        // 初始化API表格
        function initializeAPITable() {
            const apiTable = document.getElementById('api-table');
            if (!apiTable) return;
            
            const headers = apiTable.querySelectorAll('th');
            headers.forEach((header, index) => {
                if (index === 0) return;
                header.style.cursor = 'pointer';
                header.addEventListener('click', () => sortAPITable(index));
            });
            
            // 添加事件监听器到API密钥表的复选框
            document.querySelectorAll('.api-checkbox').forEach(checkbox => {
                checkbox.addEventListener('change', updateDeleteSelectedAPIButton);
            });
        }
        
        // API表格排序功能
        function sortAPITable(columnIndex) {
            const table = document.getElementById('api-table');
            const tbody = table.querySelector('tbody');
            const rows = Array.from(tbody.querySelectorAll('tr'));
            
            const isNumeric = (value) => !isNaN(parseFloat(value)) && isFinite(value);
            
            rows.sort((a, b) => {
                let aValue = a.cells[columnIndex].textContent.trim();
                let bValue = b.cells[columnIndex].textContent.trim();
                
                // 处理数字列
                if (isNumeric(aValue) && isNumeric(bValue)) {
                    return parseFloat(aValue) - parseFloat(bValue);
                }
                
                // 处理日期列
                if (columnIndex === 3 || columnIndex === 4) {
                    // 如果是"-"，则视为最旧
                    if (aValue === "-") return 1;
                    if (bValue === "-") return -1;
                    
                    const aDate = new Date(aValue);
                    const bDate = new Date(bValue);
                    return bDate - aDate; // 默认按日期倒序
                }
                
                // 处理状态列
                if (columnIndex === 5) {
                    const aActive = a.cells[columnIndex].querySelector('.health-badge').classList.contains('healthy');
                    const bActive = b.cells[columnIndex].querySelector('.health-badge').classList.contains('healthy');
                    return bActive - aActive;
                }
                
                // 处理文本列
                return aValue.localeCompare(bValue, 'zh-CN');
            });
            
            // 重新插入排序后的行
            rows.forEach(row => tbody.appendChild(row));
        }

        // ============ Memory Diagnostic Functions ============
        function showMemoryDialog() {
            document.getElementById('memory-dialog').style.display = 'flex';
            fetchMemoryStats();
        }

        function closeMemoryDialog() {
            document.getElementById('memory-dialog').style.display = 'none';
        }

        function fetchMemoryStats() {
            document.getElementById('memory-stats-content').innerHTML = '<p>加载中...</p>';
            fetch('/portal/api/memory-stats')
                .then(response => response.json())
                .then(data => {
                    if (data.success) {
                        const m = data.memory;
                        document.getElementById('memory-stats-content').innerHTML = `
                            <table style="width: 100%; border-collapse: collapse;">
                                <tr style="background: #f5f5f5;"><th style="padding: 8px; text-align: left; border: 1px solid #ddd;">指标</th><th style="padding: 8px; text-align: right; border: 1px solid #ddd;">值</th></tr>
                                <tr><td style="padding: 8px; border: 1px solid #ddd;">当前分配 (Alloc)</td><td style="padding: 8px; text-align: right; border: 1px solid #ddd; font-weight: bold; color: ${m.alloc_mb > 500 ? '#e53935' : '#43a047'};">${m.alloc_mb} MB</td></tr>
                                <tr><td style="padding: 8px; border: 1px solid #ddd;">堆使用 (HeapInuse)</td><td style="padding: 8px; text-align: right; border: 1px solid #ddd;">${m.heap_inuse_mb} MB</td></tr>
                                <tr><td style="padding: 8px; border: 1px solid #ddd;">堆空闲 (HeapIdle)</td><td style="padding: 8px; text-align: right; border: 1px solid #ddd;">${m.heap_idle_mb} MB</td></tr>
                                <tr><td style="padding: 8px; border: 1px solid #ddd;">系统内存 (Sys)</td><td style="padding: 8px; text-align: right; border: 1px solid #ddd;">${m.sys_mb} MB</td></tr>
                                <tr><td style="padding: 8px; border: 1px solid #ddd;">堆对象数</td><td style="padding: 8px; text-align: right; border: 1px solid #ddd;">${m.heap_objects.toLocaleString()}</td></tr>
                                <tr><td style="padding: 8px; border: 1px solid #ddd;">Goroutines</td><td style="padding: 8px; text-align: right; border: 1px solid #ddd; font-weight: bold; color: ${m.goroutines > 100 ? '#e53935' : '#43a047'};">${m.goroutines}</td></tr>
                                <tr><td style="padding: 8px; border: 1px solid #ddd;">GC 次数</td><td style="padding: 8px; text-align: right; border: 1px solid #ddd;">${m.num_gc}</td></tr>
                            </table>
                        `;
                        // Update the card display
                        document.getElementById('memory-display').textContent = m.alloc_mb + ' MB';
                    } else {
                        document.getElementById('memory-stats-content').innerHTML = '<p style="color: red;">获取失败</p>';
                    }
                })
                .catch(err => {
                    document.getElementById('memory-stats-content').innerHTML = '<p style="color: red;">请求错误: ' + err + '</p>';
                });
        }

        function forceGC() {
            const gcResult = document.getElementById('gc-result');
            gcResult.style.display = 'block';
            gcResult.style.backgroundColor = '#fff3e0';
            gcResult.innerHTML = '🔄 正在执行 GC...';
            
            fetch('/portal/api/force-gc', { method: 'POST' })
                .then(response => response.json())
                .then(data => {
                    if (data.success) {
                        gcResult.style.backgroundColor = '#e8f5e9';
                        gcResult.innerHTML = `
                            ✅ GC 完成!<br>
                            GC 前: ${data.before_mb} MB<br>
                            GC 后: ${data.after_mb} MB<br>
                            <strong>释放: ${data.freed_mb} MB</strong>
                        `;
                        // Refresh stats
                        fetchMemoryStats();
                    } else {
                        gcResult.style.backgroundColor = '#ffebee';
                        gcResult.innerHTML = '❌ GC 失败';
                    }
                })
                .catch(err => {
                    gcResult.style.backgroundColor = '#ffebee';
                    gcResult.innerHTML = '❌ 请求错误: ' + err;
                });
        }

        // Store last goroutine dump data for copy functions
        let lastGoroutineDumpData = null;

        function fetchGoroutineDump() {
            const dumpResult = document.getElementById('goroutine-dump-result');
            dumpResult.style.display = 'block';
            dumpResult.innerHTML = '<div style="padding: 10px; background: #fff3e0; border-radius: 5px;">🔄 正在获取 Goroutine Dump...</div>';
            
            fetch('/portal/api/goroutine-dump')
                .then(response => response.json())
                .then(data => {
                    if (data.success) {
                        lastGoroutineDumpData = data; // Store for copy functions
                        
                        // Build summary text for one-click copy
                        let summaryText = `=== Goroutine Summary ===\n`;
                        summaryText += `Total Goroutines: ${data.total}\n`;
                        summaryText += `Unique Stacks: ${data.unique_stacks}\n\n`;
                        summaryText += `=== Top Goroutines (by count) ===\n`;
                        if (data.top_goroutines) {
                            data.top_goroutines.forEach((g, i) => {
                                summaryText += `\n[${i+1}] Count: ${g.count} - ${g.signature}\n`;
                                if (g.stack_trace) {
                                    summaryText += `Stack:\n${g.stack_trace}\n`;
                                }
                            });
                        }
                        
                        let html = `
                            <div style="padding: 10px; background: #e8f5e9; border-radius: 5px; margin-bottom: 10px; display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 10px;">
                                <div>
                                    <strong>✅ 总 Goroutines: ${data.total}</strong> | 唯一堆栈: ${data.unique_stacks}
                                </div>
                                <div style="display: flex; gap: 8px; flex-wrap: wrap;">
                                    <button class="btn" onclick="copyGoroutineSummary()" style="background-color: #4caf50; padding: 6px 12px; font-size: 12px;">📋 复制摘要</button>
                                    <button class="btn" onclick="copyFullGoroutineDump()" style="background-color: #2196f3; padding: 6px 12px; font-size: 12px;">📄 复制完整Dump</button>
                                </div>
                            </div>
                            <div style="background: #f5f5f5; padding: 10px; border-radius: 5px;">
                                <h4 style="margin-top: 0;">Top Goroutines (按数量排序):</h4>
                                <div style="overflow-x: auto; width: 100%;">
                                    <table style="width: 100%; border-collapse: collapse; table-layout: auto; min-width: 100%;">
                                        <tr style="background: #e0e0e0;">
                                            <th style="padding: 8px; text-align: center; border: 1px solid #ccc; width: 60px;">数量</th>
                                            <th style="padding: 8px; text-align: left; border: 1px solid #ccc; width: 200px;">函数签名</th>
                                            <th style="padding: 8px; text-align: left; border: 1px solid #ccc;">调用栈 (点击行复制)</th>
                                        </tr>`;
                        
                        if (data.top_goroutines) {
                            data.top_goroutines.forEach((g, i) => {
                                const bgColor = g.count > 100 ? '#ffebee' : (g.count > 10 ? '#fff3e0' : '#ffffff');
                                const countColor = g.count > 100 ? '#d32f2f' : (g.count > 10 ? '#f57c00' : '#333');
                                const stackPreview = g.stack_trace ? escapeHtml(g.stack_trace) : '<em style="color: #999;">无栈信息</em>';
                                
                                html += `
                                    <tr style="background: ${bgColor}; cursor: pointer;" onclick="copyGoroutineRow(${i})" title="点击复制此行完整信息">
                                        <td style="padding: 8px; border: 1px solid #ccc; color: ${countColor}; font-weight: bold; text-align: center; width: 60px;">${g.count}</td>
                                        <td style="padding: 8px; border: 1px solid #ccc; font-family: monospace; font-size: 12px; word-break: break-word; white-space: normal; width: 200px; vertical-align: top;">
                                            <strong>${escapeHtml(g.signature)}</strong>
                                        </td>
                                        <td style="padding: 8px; border: 1px solid #ccc; font-family: monospace; font-size: 11px; white-space: pre-wrap; word-break: break-word; vertical-align: top; background: #fafafa;">${stackPreview}</td>
                                    </tr>`;
                            });
                        }
                        
                        html += `</table>
                                </div>
                            </div>
                            <details style="margin-top: 10px;">
                                <summary style="cursor: pointer; padding: 10px; background: #e3f2fd; border-radius: 5px; display: flex; justify-content: space-between; align-items: center;">
                                    <span>查看完整 Dump (点击展开)</span>
                                </summary>
                                <div style="position: relative;">
                                    <button class="btn" onclick="copyFullGoroutineDump()" style="position: absolute; top: 10px; right: 10px; background-color: #4caf50; padding: 4px 10px; font-size: 11px; z-index: 10;">📋 复制</button>
                                    <pre id="full-dump-pre" style="background: #263238; color: #aed581; padding: 10px; padding-top: 40px; border-radius: 5px; overflow-x: auto; font-size: 11px; max-height: 400px; overflow-y: auto;">${escapeHtml(data.full_dump)}</pre>
                                </div>
                            </details>`;
                        
                        dumpResult.innerHTML = html;
                    } else {
                        dumpResult.innerHTML = '<div style="padding: 10px; background: #ffebee; border-radius: 5px;">❌ 获取失败</div>';
                    }
                })
                .catch(err => {
                    dumpResult.innerHTML = '<div style="padding: 10px; background: #ffebee; border-radius: 5px;">❌ 请求错误: ' + err + '</div>';
                });
        }

        function copyGoroutineSummary() {
            if (!lastGoroutineDumpData) {
                alert('没有可复制的数据，请先获取 Goroutine Dump');
                return;
            }
            
            let summaryText = `=== Goroutine Summary ===\n`;
            summaryText += `Total Goroutines: ${lastGoroutineDumpData.total}\n`;
            summaryText += `Unique Stacks: ${lastGoroutineDumpData.unique_stacks}\n\n`;
            summaryText += `=== Top Goroutines (by count) ===\n`;
            
            if (lastGoroutineDumpData.top_goroutines) {
                lastGoroutineDumpData.top_goroutines.forEach((g, i) => {
                    summaryText += `\n[${i+1}] Count: ${g.count} - ${g.signature}\n`;
                    if (g.stack_trace) {
                        summaryText += `Stack:\n${g.stack_trace}\n`;
                    }
                });
            }
            
            copyToClipboard(summaryText, '摘要已复制到剪贴板');
        }

        function copyFullGoroutineDump() {
            if (!lastGoroutineDumpData || !lastGoroutineDumpData.full_dump) {
                alert('没有可复制的数据，请先获取 Goroutine Dump');
                return;
            }
            copyToClipboard(lastGoroutineDumpData.full_dump, '完整 Dump 已复制到剪贴板');
        }

        function copyGoroutineRow(index) {
            if (!lastGoroutineDumpData || !lastGoroutineDumpData.top_goroutines || !lastGoroutineDumpData.top_goroutines[index]) {
                alert('没有可复制的数据');
                return;
            }
            
            const g = lastGoroutineDumpData.top_goroutines[index];
            let rowText = `=== Goroutine #${index + 1} ===\n`;
            rowText += `Count: ${g.count}\n`;
            rowText += `Signature: ${g.signature}\n`;
            if (g.stack_trace) {
                rowText += `\nStack Trace:\n${g.stack_trace}\n`;
            }
            if (g.sample_stack) {
                rowText += `\nFull Sample Stack:\n${g.sample_stack}\n`;
            }
            
            copyToClipboard(rowText, `Goroutine #${index + 1} 信息已复制到剪贴板`);
        }

        function copyToClipboard(text, successMessage) {
            if (navigator.clipboard && navigator.clipboard.writeText) {
                navigator.clipboard.writeText(text).then(() => {
                    showCopyToast(successMessage || '已复制到剪贴板');
                }).catch(err => {
                    fallbackCopyToClipboard(text, successMessage);
                });
            } else {
                fallbackCopyToClipboard(text, successMessage);
            }
        }

        function fallbackCopyToClipboard(text, successMessage) {
            const textarea = document.createElement('textarea');
            textarea.value = text;
            textarea.style.position = 'fixed';
            textarea.style.left = '-9999px';
            document.body.appendChild(textarea);
            textarea.select();
            try {
                document.execCommand('copy');
                showCopyToast(successMessage || '已复制到剪贴板');
            } catch (err) {
                alert('复制失败: ' + err);
            }
            document.body.removeChild(textarea);
        }

        function showCopyToast(message) {
            // Remove existing toast if any
            const existingToast = document.getElementById('copy-toast');
            if (existingToast) {
                existingToast.remove();
            }
            
            const toast = document.createElement('div');
            toast.id = 'copy-toast';
            toast.style.cssText = 'position: fixed; bottom: 20px; left: 50%; transform: translateX(-50%); background: #323232; color: white; padding: 12px 24px; border-radius: 4px; z-index: 10000; box-shadow: 0 2px 10px rgba(0,0,0,0.3); font-size: 14px;';
            toast.textContent = message;
            document.body.appendChild(toast);
            
            setTimeout(() => {
                toast.style.opacity = '0';
                toast.style.transition = 'opacity 0.3s';
                setTimeout(() => toast.remove(), 300);
            }, 2000);
        }

        // Auto-update memory display on page load
        setTimeout(function() {
            fetch('/portal/api/memory-stats')
                .then(response => response.json())
                .then(data => {
                    if (data.success) {
                        document.getElementById('memory-display').textContent = data.memory.alloc_mb + ' MB';
                    }
                })
                .catch(() => {});
        }, 500);
        // ============ End Memory Diagnostic Functions ============
        
        // 页面加载完成后的初始化
        document.addEventListener('DOMContentLoaded', function() {
            // 初始化表格
            initializeTable();
            
            // 初始化API表格
            initializeAPITable();
            
            // 初始化右键菜单
            initializeContextMenu();
            
            // --- BEGIN: Tab Initialization Logic ---
            // Restore last active tab or default to 'all'
            const savedTabId = localStorage.getItem('activeTabId');
            const defaultTabId = 'all';
            const initialTabId = savedTabId || defaultTabId;
            console.log(`Initializing tabs. Saved tab: ${savedTabId}, Initial tab: ${initialTabId}`); // Debug log

            // Initialize tabs based on saved state or default
            document.querySelectorAll('.tab').forEach(tab => {
                const currentTabId = tab.getAttribute('data-tab');
                tab.addEventListener('click', function(e) {
                    e.preventDefault();
                    switchTab(currentTabId); // Use currentTabId from closure
                });
                // Set initial active state
                if (currentTabId === initialTabId) {
                     tab.classList.add('active');
                } else {
                     tab.classList.remove('active');
                }
            });

            document.querySelectorAll('.tab-content').forEach(content => {
                if (content.id === initialTabId) {
                    content.classList.add('active');
                } else {
                    content.classList.remove('active');
                }
            });

            // If the initial tab is 'add', make sure the form is shown
            if (initialTabId === 'add') {
                showAddProviderForm();
            }
            
            // 页面加载时根据当前 Tab 自动加载数据
            if (initialTabId === 'ops-users') {
                refreshOpsUsers();
            } else if (initialTabId === 'ops-logs') {
                refreshOpsLogs();
            } else if (initialTabId === 'web-search') {
                refreshWebSearchKeys();
            } else if (initialTabId === 'amap') {
                refreshAmapKeys();
                loadAmapConfig();
            }
            // --- END: Tab Initialization Logic ---

            // 添加全局事件监听器，在各种情况下隐藏tooltip
            document.addEventListener('mousedown', function(e) {
                const tooltip = document.getElementById('global-tooltip');
                if (tooltip && !tooltip.contains(e.target) && 
                    !e.target.classList.contains('copyable')) {
                    hideTooltip();
                }
            });
            
            // 滚动时隐藏tooltip
            window.addEventListener('scroll', hideTooltip);
            
            // 页面大小变化时隐藏tooltip
            window.addEventListener('resize', hideTooltip);
            
            // 页面离开时隐藏tooltip
            window.addEventListener('beforeunload', hideTooltip);

            // 初始化复制功能
            document.querySelectorAll('.copyable').forEach(cell => {
                const fullText = cell.getAttribute('data-full-text') || cell.textContent;
                
                // 点击复制
                cell.addEventListener('click', () => {
                    copyToClipboard(fullText);
                });

                // 添加移动设备长按支持
                let pressTimer;
                cell.addEventListener('touchstart', () => {
                    pressTimer = setTimeout(() => {
                        copyToClipboard(fullText);
                        showTooltip(cell, '已复制!');
                    }, 500);
                });
                
                cell.addEventListener('touchend', () => {
                    clearTimeout(pressTimer);
                });

                cell.addEventListener('mouseenter', (e) => {
                    showTooltip(cell, fullText);
                });

                cell.addEventListener('mouseleave', () => {
                    hideTooltip();
                });
            });

            document.querySelectorAll('.provider-checkbox').forEach(checkbox => {
                checkbox.addEventListener('change', updateDeleteSelectedButton);
            });
            
            // 在初始化时预加载自动补全数据
            loadAutoCompleteData();
            
            // 动态填充模型选择器
            populateAllowedModelsSelector();

            // --- BEGIN: Hide Loading Overlay Logic ---
            // Hide loading overlay after a short delay
            setTimeout(() => {
                const loadingOverlay = document.getElementById('loading-overlay');
                if (loadingOverlay) {
                    loadingOverlay.classList.add('hidden');
                    // Optional: Remove the overlay from DOM after transition ends
                    // loadingOverlay.addEventListener('transitionend', () => {
                    //     loadingOverlay.remove();
                    // });
                    console.log('Hiding loading overlay.'); // Debug log
                }
            }, 300); // 300ms delay
            // --- END: Hide Loading Overlay Logic ---

            // BEGIN: Add default filter on load
            // Ensure the 'all' tab content is active before filtering
            const allTabContent = document.getElementById('all');
            if (allTabContent && allTabContent.classList.contains('active')) {
                 filterProviders('healthy'); // Default filter to 'healthy' only if 'all' tab is active
            }
            // END: Add default filter on load

            // Update event listeners for checkboxes to call the modified update function
            document.querySelectorAll('.provider-checkbox').forEach(checkbox => {
                checkbox.addEventListener('change', updateDeleteSelectedButton);
            });

            // Add listener to the header checkbox as well
            const selectAllHeaderCheckbox = document.getElementById('select-all-header');
            if (selectAllHeaderCheckbox) {
                selectAllHeaderCheckbox.addEventListener('change', toggleSelectAll);
            }

            // Initialize API filter buttons and default filter
            const apiTabContent = document.getElementById('api');
            if (apiTabContent && apiTabContent.classList.contains('active')) {
                filterApiKeys('all'); // Default filter to 'all' if API tab is active initially
            }

            // Update event listeners for API checkboxes
            document.querySelectorAll('.api-checkbox').forEach(checkbox => {
                checkbox.addEventListener('change', updateDeleteSelectedAPIButton);
            });

            // Add listener to the API header checkbox
            const selectAllAPICheckbox = document.getElementById('select-all-api');
            if (selectAllAPICheckbox) {
                selectAllAPICheckbox.addEventListener('change', toggleSelectAllAPI);
            }
        });

        // 删除功能
        async function deleteProvider(providerId) {
            if (confirm('确定要删除这个提供者吗？')) {
                try {
                    const response = await fetch(`/portal/delete-provider/${providerId}`, {
                        method: 'DELETE'
                    });

                    if (!response.ok) {
                        throw new Error('删除失败');
                    }

                    showToast('提供者删除成功', 'success');
                    setTimeout(() => window.location.reload(), 1000);
                } catch (error) {
                    showToast('删除失败: ' + error.message, 'error');
                }
            }
        }

        // 工具函数
        function copyToClipboard(text) {
            navigator.clipboard.writeText(text).then(() => {
                showToast('已复制到剪贴板');
            }).catch(err => {
                console.error('复制失败:', err);
                showToast('复制失败');
            });
        }

        // 全局tooltip计时器
        let tooltipTimerId = null;
        
        // 直接函数，不使用任何间接方式
        function showTooltip(element, text) {
            // 强制清除已有tooltip
            const existingTooltip = document.getElementById('global-tooltip');
            if (existingTooltip) {
                if (existingTooltip.parentNode) {
                    existingTooltip.parentNode.removeChild(existingTooltip);
                }
            }
            
            // 清除所有可能的定时器
            if (tooltipTimerId) {
                clearTimeout(tooltipTimerId);
                tooltipTimerId = null;
            }
            
            // 创建新tooltip
            const tooltip = document.createElement('div');
            tooltip.className = 'tooltip';
            tooltip.id = 'global-tooltip';
            tooltip.textContent = text;
            document.body.appendChild(tooltip);
            
            // 定位
            const rect = element.getBoundingClientRect();
            const tooltipHeight = tooltip.offsetHeight;
            const tooltipWidth = tooltip.offsetWidth;
            
            let top = rect.top - tooltipHeight - 5;
            if (top < 10) {
                top = rect.bottom + 5;
            }
            
            let left = rect.left + (rect.width / 2) - (tooltipWidth / 2);
            left = Math.max(10, Math.min(left, window.innerWidth - tooltipWidth - 10));
            
            tooltip.style.top = `${top}px`;
            tooltip.style.left = `${left}px`;
            
            // 立即显示
            tooltip.style.opacity = '1';
            tooltip.style.visibility = 'visible';
            tooltip.classList.add('show');
            
            // 五秒后强制关闭
            tooltipTimerId = setTimeout(function() {
                // 直接移除元素，不使用任何中间函数
                const tooltipToRemove = document.getElementById('global-tooltip');
                if (tooltipToRemove && tooltipToRemove.parentNode) {
                    tooltipToRemove.parentNode.removeChild(tooltipToRemove);
                }
                tooltipTimerId = null;
            }, 5000);
        }
        
        function hideTooltip() {
            // 清除定时器
            if (tooltipTimerId) {
                clearTimeout(tooltipTimerId);
                tooltipTimerId = null;
            }
            
            // 直接移除元素
            const tooltip = document.getElementById('global-tooltip');
            if (tooltip && tooltip.parentNode) {
                tooltip.parentNode.removeChild(tooltip);
            }
        }

        function showToast(message, type = 'info', duration = 3000) {
            const container = document.getElementById('toast-container');
            const toast = document.createElement('div');
            toast.className = `toast ${type}`;
            
            let iconPath = '';
            switch(type) {
                case 'success':
                    iconPath = 'M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z';
                    break;
                case 'error':
                    iconPath = 'M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z';
                    break;
                case 'warning':
                    iconPath = 'M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z';
                    break;
                default:
                    iconPath = 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-6h2v6zm0-8h-2V7h2v2z';
            }
            
            toast.innerHTML = `
                <div class="toast-icon">
                    <svg viewBox="0 0 24 24" width="24" height="24">
                        <path d="${iconPath}"></path>
                    </svg>
    </div>
                <div class="toast-content">${message}</div>
                <div class="toast-close" onclick="this.parentElement.remove()">
                    <svg viewBox="0 0 24 24" width="16" height="16">
                        <path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"></path>
                    </svg>
                </div>
            `;
            
            container.appendChild(toast);
            
            // 显示动画
            setTimeout(() => {
                toast.classList.add('show');
            }, 10);
            
            // 自动关闭
            if (duration > 0) {
                setTimeout(() => {
                    toast.classList.remove('show');
                    setTimeout(() => {
                        toast.remove();
                    }, 300);
                }, duration);
            }
            
            return toast;
        }

        // 表格功能
        function initializeTable() {
            const table = document.querySelector('table');
            if (!table) return;
            
            const headers = table.querySelectorAll('th');
            headers.forEach((header, index) => {
                if (index === 0) return;
                header.style.cursor = 'pointer';
                header.addEventListener('click', () => sortTable(index));
            });
        }

        // 右键菜单功能
        function initializeContextMenu() {
            // Listener for provider rows (main table)
            document.querySelectorAll('#provider-table-body tr[data-id]').forEach(row => {
                row.addEventListener('contextmenu', showContextMenu);
            });

            // Listener for "Allowed Models" cells in API Keys table
            document.querySelectorAll('#api-table tbody td.editable-allowed-models').forEach(cell => {
                cell.addEventListener('contextmenu', showContextMenu);
            });

            // Global click to hide context menu
            document.addEventListener('click', (e) => {
                const menu = document.getElementById('context-menu');
                if (menu && !menu.contains(e.target) && !e.target.closest('td.editable-allowed-models') && !e.target.closest('#provider-table-body tr[data-id]')) {
                    hideContextMenu();
                }
            });
            window.addEventListener('scroll', hideContextMenu, true);
        }

        function showContextMenu(e) {
            e.preventDefault();
            const menu = document.getElementById('context-menu');
            if (!menu) return;

            console.log("[Debug] showContextMenu called"); // Log: Function called

            // Hide all items first
            menu.querySelectorAll('.context-menu-item').forEach(item => item.style.display = 'none');

            const currentTargetElement = e.currentTarget; // Element the listener was attached to
            let showMenu = false;

            if (currentTargetElement.classList.contains('editable-allowed-models')) {
                // Context is API Key "Allowed Models" cell
                contextApiIdForEdit = currentTargetElement.dataset.apiId;
                contextModelsForEdit = currentTargetElement.dataset.currentModels;
                console.log("[Debug] Context: editable-allowed-models. ID:", contextApiIdForEdit, "Models:", contextModelsForEdit); // Log: Context identified

                const editModelsItem = document.getElementById('context-menu-item-edit-models');
                if (editModelsItem) {
                    editModelsItem.style.display = 'flex';
                    showMenu = true;
                    console.log("[Debug] Displaying '修改允许模型' menu item."); // Log: Menu item displayed
                }
            } else if (currentTargetElement.tagName === 'TR' && currentTargetElement.dataset.id && currentTargetElement.closest('#provider-table-body')) {
                // Context is a Provider row
                currentContextMenuProviderId = currentTargetElement.dataset.id;
                console.log("[Debug] Context: provider-table-body TR. ID:", currentContextMenuProviderId); // Log: Provider context
                // Show provider-specific items (items other than the new edit-models item)
                menu.querySelectorAll('.context-menu-item:not(#context-menu-item-edit-models)').forEach(item => {
                    item.style.display = 'flex';
                });
                showMenu = true;
            }

            if (showMenu) {
                const x = e.clientX;
                const y = e.clientY;
                const menuWidth = menu.offsetWidth;
                const menuHeight = menu.offsetHeight;
                const viewportWidth = window.innerWidth;
                const viewportHeight = window.innerHeight;
                let menuX = x;
                let menuY = y;
                if (x + menuWidth > viewportWidth) menuX = viewportWidth - menuWidth - 5;
                if (y + menuHeight > viewportHeight) menuY = viewportHeight - menuHeight - 5;
                menuX = Math.max(5, menuX);
                menuY = Math.max(5, menuY);

                menu.style.left = `${menuX}px`;
                menu.style.top = `${menuY}px`;
                menu.classList.add('show');
            } else {
                hideContextMenu();
            }
        }

        function hideContextMenu() {
            const menu = document.getElementById('context-menu');
            if (menu) {
                menu.classList.remove('show');
            }
            currentContextMenuProviderId = null; // Reset provider context
            contextApiIdForEdit = null;          // Reset API model edit context
            contextModelsForEdit = null;         // Reset API model edit context
        }

        // 健康检查功能
        async function checkAllProvidersHealth() {
            if (isHealthCheckInProgress) return;
            isHealthCheckInProgress = true;

            const button = document.getElementById('check-all-health-btn');
            const originalText = button.innerHTML;
            const originalClass = button.className;
            
            // 添加检查中状态样式
            button.innerHTML = `
                <svg viewBox="0 0 24 24" class="rotating" style="width: 16px; height: 16px; margin-right: 6px;">
                    <path fill="currentColor" d="M17.65 6.35C16.2 4.9 14.21 4 12 4c-4.42 0-7.99 3.58-7.99 8s3.57 8 7.99 8c3.73 0 6.84-2.55 7.73-6h-2.08c-.82 2.33-3.04 4-5.65 4-3.31 0-6-2.69-6-6s2.69-6 6-6c1.66 0 3.14.69 4.22 1.78L13 11h7V4l-2.35 2.35z"/>
                </svg>
                检查中...
            `;
            button.classList.add('checking');
            button.disabled = true;

            try {
                const response = await fetch('/portal/check-all-health', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    }
                });

                if (!response.ok) {
                    throw new Error('健康检查失败');
                }

                showToast('健康检查完成', 'success');
                setTimeout(() => window.location.reload(), 1000);
            } catch (error) {
                showToast('健康检查失败: ' + error.message, 'error');
            } finally {
                button.innerHTML = originalText;
                button.className = originalClass;
                button.disabled = false;
                isHealthCheckInProgress = false;
            }
        }

        async function checkSingleProvider(providerId, event) {
            const refreshBtn = document.querySelector(`tr[data-id="${providerId}"] .refresh-btn`);
            if (refreshBtn && refreshBtn.disabled) return;
            
            // 找到当前行的健康状态和延迟显示元素
            const row = document.querySelector(`tr[data-id="${providerId}"]`);
            const healthInfoDiv = row ? row.querySelector('.health-info') : null;
            
            if (!healthInfoDiv) return;
            
            // 保存原始的健康信息HTML
            const originalHealthInfo = healthInfoDiv.innerHTML;
            
            // 替换为检查中状态
            healthInfoDiv.innerHTML = `
                <span class="health-badge checking">检查中</span>
                <span class="health-latency">-</span>
            `;
            
            if (refreshBtn) {
                refreshBtn.disabled = true;
                refreshBtn.classList.add('rotating');
            }

            try {
                const response = await fetch(`/portal/check-health/${providerId}`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    }
                });

                if (!response.ok) {
                    throw new Error('健康检查失败');
                }

                // 尝试获取响应详细信息
                const resultData = await response.json();
                
                // 如果有单个提供者的详细结果，更新UI而不刷新整个页面
                if (resultData && resultData.data && resultData.success) {
                    const providerData = resultData.data;
                    
                    const isHealthy = providerData.healthy;
                    const responseTime = providerData.responseTime || 0;
                    
                    // 根据结果更新健康状态显示
                    if (isHealthy) {
                        healthInfoDiv.innerHTML = `
                            <span class="health-badge healthy">健康</span>
                            <span class="health-latency">${responseTime}ms</span>
                        `;
                        // 更新行的数据状态属性
                        row.setAttribute('data-status', 'healthy');
                    } else {
                        healthInfoDiv.innerHTML = `
                            <span class="health-badge unhealthy">异常</span>
                            <span class="health-latency">${responseTime > 0 ? responseTime + 'ms' : '-'}</span>
                        `;
                        // 更新行的数据状态属性
                        row.setAttribute('data-status', 'unhealthy');
                    }
                    
                    // 显示成功提示但不刷新页面
                    showToast('健康检查完成', 'success');
                } else {
                    // 无法获得详细结果时，刷新整个页面
                    showToast('健康检查完成', 'success');
                    setTimeout(() => window.location.reload(), 1000);
                }
            } catch (error) {
                // 发生错误时恢复原始显示
                healthInfoDiv.innerHTML = originalHealthInfo;
                showToast('健康检查失败: ' + error.message, 'error');
            } finally {
                if (refreshBtn) {
                    refreshBtn.disabled = false;
                    refreshBtn.classList.remove('rotating');
                }
            }
        }

        // ==================== Portal 模型选择辅助函数 ====================
        
        function portalRenderModelList() {
            const modelList = document.getElementById('portalModelList');
            if (!modelList) return;
            
            if (portalAvailableModels.length === 0) {
                modelList.innerHTML = '<div style="padding: 20px; text-align: center; color: #888;">No models available</div>';
                return;
            }
            
            modelList.innerHTML = portalAvailableModels.map(model => `
                <div class="model-item ${portalSelectedModels.has(model) ? 'selected' : ''}" onclick="portalToggleModel('${model}')">
                    <input type="checkbox" ${portalSelectedModels.has(model) ? 'checked' : ''} onclick="event.stopPropagation(); portalToggleModel('${model}')">
                    <label>${model}</label>
                </div>
            `).join('');
            
            portalUpdateSelectedPreview();
        }
        
        function portalToggleModel(model) {
            if (portalSelectedModels.has(model)) {
                portalSelectedModels.delete(model);
            } else {
                portalSelectedModels.add(model);
            }
            portalRenderModelList();
        }
        
        function portalSelectAllModels() {
            portalAvailableModels.forEach(m => portalSelectedModels.add(m));
            portalRenderModelList();
        }
        
        function portalClearAllModels() {
            portalSelectedModels.clear();
            portalRenderModelList();
        }
        
        function portalUpdateSelectedPreview() {
            const preview = document.getElementById('portalSelectedPreview');
            if (!preview) return;
            
            const globInput = document.getElementById('portalGlobPatterns');
            const globPatterns = globInput ? globInput.value.trim() : '';
            
            let html = '<strong>已选择:</strong> ';
            
            const modelArray = Array.from(portalSelectedModels).sort();
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
                html += '<span style="color: #888;">未选择</span>';
            }
            
            preview.innerHTML = html;
        }
        
        function portalGetSelectedModels() {
            const modelArray = Array.from(portalSelectedModels);
            const globInput = document.getElementById('portalGlobPatterns');
            const globPatterns = globInput ? globInput.value.trim() : '';
            const globArray = globPatterns ? globPatterns.split(',').map(p => p.trim()).filter(p => p) : [];
            return [...modelArray, ...globArray].sort();
        }

        // 新增：确认并生成 API Key 的函数
        function confirmAndGenerateApiKey() {
            const selectedModels = portalGetSelectedModels();

            if (selectedModels.length === 0) {
                showToast('请至少选择一个允许的模型', 'warning');
                return;
            }

            if (confirm('确定要生成一个新的 API Key 吗？选定的模型将被关联。')) {
                generateNewApiKey(); // 调用原来的生成函数
            }
        }

        // 添加 API Key 生成功能 (现在由 confirmAndGenerateApiKey 调用)
        async function generateNewApiKey() {
            const selectedModels = portalGetSelectedModels();
            
            // 再次检查，虽然 confirmAndGenerateApiKey 已经检查过
            if (selectedModels.length === 0) {
                showToast('内部错误：未选择模型', 'error'); 
                return;
            }
            
            try {
                const response = await fetch('/portal/generate-api-key', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    // 将选中的模型包含在请求体中
                    body: JSON.stringify({ allowed_models: selectedModels })
                });

                if (!response.ok) {
                    const errorText = await response.text();
                    throw new Error(`生成 API Key 失败: ${errorText || response.status}`);
                }

                const data = await response.json();
                document.getElementById('apiKeyDisplay').value = data.apiKey; // 更新显示区域
                showToast('API Key 生成成功', 'success');
                
                // 显示成功弹窗，而不是直接刷新
                showApiKeySuccessModal(data.apiKey);
                // // 稍微延迟刷新，让用户看到生成的 Key
                // setTimeout(() => window.location.reload(), 1500);
            } catch (error) {
                showToast('生成 API Key 失败: ' + error.message, 'error');
                console.error("Error generating API key:", error); // 使用 common/log
            }
        }
        
        // API密钥表格功能
        function toggleSelectAllAPI() {
            const selectAllCheckbox = document.querySelector('#select-all-api');
            // Select only visible checkboxes
            const checkboxes = document.querySelectorAll('#api-table tbody tr:not([style*="display: none"]) .api-checkbox');
            checkboxes.forEach(checkbox => {
                checkbox.checked = selectAllCheckbox.checked;
            });
            updateDeleteSelectedAPIButton(); 
        }
        
        // BEGIN: Filter API Keys Function
        function filterApiKeys(status) {
            const tableBody = document.querySelector('#api-table tbody');
            if (!tableBody) return;
            const rows = tableBody.querySelectorAll('tr[data-api-status]');
            const buttons = document.querySelectorAll('.api-filter-buttons .filter-btn');

            // Update button active state
            buttons.forEach(btn => {
                if (btn.getAttribute('data-filter') === status) {
                    btn.classList.add('active');
                } else {
                    btn.classList.remove('active');
                }
            });

            // Filter rows
            rows.forEach(row => {
                const rowStatus = row.getAttribute('data-api-status');
                if (status === 'all' || rowStatus === status) {
                    row.style.display = ''; // Show row
                } else {
                    row.style.display = 'none'; // Hide row
                }
            });

            // Reset select-all checkbox when filtering changes
            const selectAllApiCheckbox = document.getElementById('select-all-api');
            if (selectAllApiCheckbox) selectAllApiCheckbox.checked = false;
            updateDeleteSelectedAPIButton(); // Update button state based on visible items
        }
        // END: Filter API Keys Function
        
        // Rename this function and update its logic
        function updateDeleteSelectedAPIButton() {
            const deleteButton = document.getElementById('delete-selected-api'); // This might be null if commented out
            const disableButton = document.getElementById('disable-selected-api');
            const enableButton = document.getElementById('enable-selected-api'); // Get the enable button
            // Count only visible checked checkboxes
            const selectedCount = document.querySelectorAll('#api-table tbody tr:not([style*="display: none"]) .api-checkbox:checked').length;
            
            const enable = selectedCount > 0; // Enable buttons if at least one item is selected

            if (deleteButton) {
                deleteButton.disabled = !enable;
            }
            if (disableButton) {
                disableButton.disabled = !enable;
            }
            if (enableButton) { // Check if enable button exists
                enableButton.disabled = !enable;
            }
            console.log(`API Action Buttons updated. Selected: ${selectedCount}, Buttons Enabled: ${enable}`); // Debug log
        }
        
        function confirmDeleteSelectedAPI() {
            const selectedIds = Array.from(document.querySelectorAll('.api-checkbox:checked'))
                .map(checkbox => checkbox.closest('tr').getAttribute('data-api-id'));
                
            if (selectedIds.length === 0) return;
            
            if (confirm(`确定要删除选中的 ${selectedIds.length} 个API密钥吗？`)) {
                deleteMultipleAPIKeys(selectedIds);
            }
        }
        
        async function deleteMultipleAPIKeys(apiIds) {
            try {
                const response = await fetch('/portal/delete-api-keys', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ ids: apiIds })
                });
                
                if (!response.ok) {
                    throw new Error('删除API密钥失败');
                }
                
                showToast(`成功删除 ${apiIds.length} 个API密钥`, 'success');
                // Refresh paginated data instead of full page reload
                setTimeout(() => loadAPIKeysPaginated(apiKeysPage, apiKeysPageSize), 500);
            } catch (error) {
                showToast('删除失败: ' + error.message, 'error');
            }
        }
        
        async function deleteAPIKey(apiKeyId) {
            if (confirm('确定要删除这个API密钥吗？')) {
                try {
                    const response = await fetch(`/portal/delete-api-key/${apiKeyId}`, {
                        method: 'DELETE'
                    });
                    
                    if (!response.ok) {
                        throw new Error('删除失败');
                    }
                    
                    showToast('API密钥删除成功', 'success');
                    // Refresh paginated data instead of full page reload
                    setTimeout(() => loadAPIKeysPaginated(apiKeysPage, apiKeysPageSize), 500);
                } catch (error) {
                    showToast('删除失败: ' + error.message, 'error');
                }
            }
        }
        
        async function toggleAPIKeyStatus(apiKeyId, activate) {
            try {
                const action = activate ? 'activate' : 'deactivate';
                const response = await fetch(`/portal/${action}-api-key/${apiKeyId}`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    }
                });
                
                if (!response.ok) {
                    throw new Error(`${activate ? '激活' : '禁用'}API密钥失败`);
                }
                
                showToast(`API密钥${activate ? '激活' : '禁用'}成功`, 'success');
                // Refresh paginated data instead of full page reload
                setTimeout(() => loadAPIKeysPaginated(apiKeysPage, apiKeysPageSize), 500);
            } catch (error) {
                showToast(`操作失败: ${error.message}`, 'error');
            }
        }

        // 右键菜单操作
        function checkSelectedProvider() {
            if (currentContextMenuProviderId) {
                checkSingleProvider(currentContextMenuProviderId);
            }
            hideContextMenu();
        }

        function deleteSelectedProvider() {
            if (currentContextMenuProviderId) {
                deleteProvider(currentContextMenuProviderId);
            }
            hideContextMenu();
        }

        // 窗口大小改变时重新初始化
        window.addEventListener('resize', () => {
            if (resizeTimer) clearTimeout(resizeTimer);
            resizeTimer = setTimeout(initializeTable, 250);
        });

        // 点击事件监听器
        document.addEventListener('click', (e) => {
            if (!e.target.closest('#context-menu')) {
                hideContextMenu();
            }
        });

        // 表格排序功能
        function sortTable(columnIndex) {
            const table = document.querySelector('table');
            const tbody = table.querySelector('tbody');
            const rows = Array.from(tbody.querySelectorAll('tr'));
            
            const isNumeric = (value) => !isNaN(parseFloat(value)) && isFinite(value);
            
            rows.sort((a, b) => {
                let aValue = a.cells[columnIndex].textContent.trim();
                let bValue = b.cells[columnIndex].textContent.trim();
                
                // 处理数字列
                if (isNumeric(aValue) && isNumeric(bValue)) {
                    return parseFloat(aValue) - parseFloat(bValue);
                }
                
                // 处理健康状态列
                if (columnIndex === 2) {
                    const aHealthy = a.cells[columnIndex].querySelector('.health-badge').classList.contains('healthy');
                    const bHealthy = b.cells[columnIndex].querySelector('.health-badge').classList.contains('healthy');
                    return bHealthy - aHealthy;
                }
                
                // 处理文本列
                return aValue.localeCompare(bValue, 'zh-CN');
            });
            
            // 重新插入排序后的行
            rows.forEach(row => tbody.appendChild(row));
        }

        // 添加滚动事件监听器
        window.addEventListener('scroll', () => {
            hideContextMenu(); // Ensure this is called, or use the capture phase listener
        });

        // 动态填充 API Key 的模型选择器
        function populateAllowedModelsSelector() {
            const selectElement = document.getElementById('allowedModelsSelect');
            if (!selectElement) return;
        
            // 从现有 provider 数据中提取唯一的 WrapperName
            const providerRows = document.querySelectorAll('#all tbody tr');
            const wrapperNames = new Set();
            providerRows.forEach(row => {
                const wrapperNameCell = row.cells[3]; // 第4列是提供者名称 (WrapperName)
                if (wrapperNameCell) {
                    const wrapperName = wrapperNameCell.getAttribute('data-full-text') || wrapperNameCell.textContent.trim();
                    if (wrapperName) {
                        wrapperNames.add(wrapperName);
                    }
                }
            });
        
            selectElement.innerHTML = ''; // 清空现有选项
            if (wrapperNames.size === 0) {
                 // 如果没有 provider，可以添加一个提示或者禁用选择器
                 const option = document.createElement('option');
                 option.textContent = '没有可用的模型提供者';
                 option.disabled = true;
                 selectElement.appendChild(option);
                 console.warn("No providers found to populate allowed models selector."); // 使用 common/log
                 return;
            }

            // 添加选项
            wrapperNames.forEach(name => {
                const option = document.createElement('option');
                option.value = name;
                option.textContent = name;
                selectElement.appendChild(option);
            });
        }

        // 新增：API Key 成功弹窗相关函数
        function showApiKeySuccessModal(apiKey) {
            document.getElementById('generatedApiKeyDisplay').value = apiKey;
            document.getElementById('apiKeySuccessModal').style.display = 'flex';
        }

        function closeApiKeyModal(reload = false) {
            document.getElementById('apiKeySuccessModal').style.display = 'none';
            if (reload) {
                // Refresh paginated API keys data instead of full page reload
                loadAPIKeysPaginated(1, apiKeysPageSize);
            }
        }

        function copyGeneratedApiKey() {
            const apiKeyInput = document.getElementById('generatedApiKeyDisplay');
            apiKeyInput.select();
            apiKeyInput.setSelectionRange(0, 99999); // For mobile devices
            try {
                navigator.clipboard.writeText(apiKeyInput.value);
                showToast('API Key 已复制到剪贴板', 'success');
            } catch (err) {
                showToast('复制失败，请手动复制', 'error');
                console.error('Failed to copy API key: ', err);
            }
        }

        // 新增：确认删除选中的提供者
        function confirmDeleteSelected() {
            const selectedCheckboxes = document.querySelectorAll('.provider-checkbox:checked');
            const selectedIds = Array.from(selectedCheckboxes)
                .map(checkbox => checkbox.closest('tr').getAttribute('data-id'));
                
            if (selectedIds.length === 0) {
                showToast('请先选择要删除的提供者', 'warning');
                return;
            }
            
            if (confirm(`确定要删除选中的 ${selectedIds.length} 个提供者吗？`)) {
                deleteMultipleProviders(selectedIds);
            }
        }
        
        // 新增：批量删除提供者
        async function deleteMultipleProviders(providerIds) {
            showToast('正在删除提供者...', 'info');
            try {
                const response = await fetch('/portal/delete-providers', { // Assuming this endpoint
                    method: 'POST', // Assuming POST method like API keys
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ ids: providerIds }) // Assuming JSON body with 'ids' array
                });
                
                if (!response.ok) {
                    const errorText = await response.text();
                    throw new Error(`删除提供者失败 (${response.status}): ${errorText}`);
                }
                
                showToast(`成功删除 ${providerIds.length} 个提供者`, 'success');
                // 清除全选状态
                document.getElementById('select-all').checked = false;
                // 禁用删除按钮
                document.getElementById('delete-selected').disabled = true;
                // 短暂延迟后刷新页面以显示最新列表
                setTimeout(() => window.location.reload(), 1000); 
            } catch (error) {
                showToast('删除失败: ' + error.message, 'error');
                console.error('Error deleting multiple providers:', error); // 使用 common/log
            }
        }

        // New function: Confirm disabling selected API keys
        function confirmDisableSelectedAPI() {
            const selectedIds = Array.from(document.querySelectorAll('.api-checkbox:checked'))
                .map(checkbox => checkbox.closest('tr').getAttribute('data-api-id'));
                
            if (selectedIds.length === 0) return;
            
            if (confirm(`确定要禁用选中的 ${selectedIds.length} 个API密钥吗？`)) {
                disableMultipleAPIKeys(selectedIds);
            }
        }

        // New function: Send request to disable multiple API keys
        async function disableMultipleAPIKeys(apiIds) {
            showToast('正在禁用API密钥...', 'info');
            try {
                const response = await fetch('/portal/batch-deactivate-api-keys', { // New backend endpoint, was /portal/disable-api-keys
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ ids: apiIds })
                });
                
                if (!response.ok) {
                    const errorText = await response.text();
                    throw new Error(`禁用API密钥失败 (${response.status}): ${errorText}`);
                }
                
                showToast(`成功禁用 ${apiIds.length} 个API密钥`, 'success');
                // Uncheck all checkboxes and disable buttons
                document.getElementById('select-all-api').checked = false;
                document.querySelectorAll('.api-checkbox:checked').forEach(cb => cb.checked = false);
                updateDeleteSelectedAPIButton(); // Update button states

                // Refresh paginated data instead of full page reload
                setTimeout(() => loadAPIKeysPaginated(apiKeysPage, apiKeysPageSize), 500);
            } catch (error) {
                showToast('禁用失败: ' + error.message, 'error');
                console.error('Error disabling multiple API keys:', error); // Use common/log
            }
        }

        // Note: deleteMultipleAPIKeys is already defined above, this duplicate definition is removed
        // to avoid overriding the paginated refresh behavior

        // New function: Confirm enabling selected API keys
        function confirmEnableSelectedAPI() {
            const selectedIds = Array.from(document.querySelectorAll('.api-checkbox:checked'))
                .map(checkbox => checkbox.closest('tr').getAttribute('data-api-id'));
                
            if (selectedIds.length === 0) return;
            
            if (confirm(`确定要启用选中的 ${selectedIds.length} 个API密钥吗？`)) {
                enableMultipleAPIKeys(selectedIds);
            }
        }

        // New function: Send request to enable multiple API keys
        async function enableMultipleAPIKeys(apiIds) {
            showToast('正在启用API密钥...', 'info');
            try {
                const response = await fetch('/portal/batch-activate-api-keys', { // New backend endpoint, was /portal/enable-api-keys
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ ids: apiIds })
                });
                
                if (!response.ok) {
                    const errorText = await response.text();
                    throw new Error(`启用API密钥失败: ${errorText}`);
                }
                
                showToast(`成功启用 ${apiIds.length} 个API密钥`, 'success');
                // Uncheck all checkboxes and disable buttons
                document.getElementById('select-all-api').checked = false;
                document.querySelectorAll('.api-checkbox:checked').forEach(cb => cb.checked = false);
                updateDeleteSelectedAPIButton(); // Update button states

                // Refresh paginated data instead of full page reload
                setTimeout(() => loadAPIKeysPaginated(apiKeysPage, apiKeysPageSize), 500);
            } catch (error) {
                showToast('启用失败: ' + error.message, 'error');
                console.error('Error enabling multiple API keys:', error); // Use common/log
            }
        } // <-- Added missing closing brace

        // BEGIN: Filter Providers Function
        function filterProviders(status) {
            const tableBody = document.getElementById('provider-table-body');
            if (!tableBody) return; // Add check if table body exists
            const rows = tableBody.querySelectorAll('tr[data-status]');
            const buttons = document.querySelectorAll('.filter-btn');

            // Update button active state
            buttons.forEach(btn => {
                if (btn.getAttribute('data-filter') === status) {
                    btn.classList.add('active');
                } else {
                    btn.classList.remove('active');
                }
            });

            // Filter rows
            rows.forEach(row => {
                const rowStatus = row.getAttribute('data-status');
                if (status === 'all' || rowStatus === status) {
                    row.style.display = ''; // Show row
                } else {
                    row.style.display = 'none'; // Hide row
                }
            });

            // Reset select-all checkbox when filtering changes
            const selectAllHeaderCheckbox = document.getElementById('select-all-header'); // Use the header checkbox ID
            if (selectAllHeaderCheckbox) selectAllHeaderCheckbox.checked = false;
            updateDeleteSelectedButton(); // Update delete button state based on visible items
        }
        // END: Filter Providers Function

        // 填充自动补全表单
        function fillAutoCompleteForm() {
            // 填充提供者名称选项
            const wrapperNamesList = document.getElementById('wrapper-names-list');
            if (wrapperNamesList) {
                wrapperNamesList.innerHTML = '';
                autoCompleteData.wrapper_names.forEach(name => {
                    const option = document.createElement('option');
                    option.value = name;
                    wrapperNamesList.appendChild(option);
                });
            }
            
            // 填充模型名称选项
            const modelNamesList = document.getElementById('model-names-list');
            if (modelNamesList) {
                modelNamesList.innerHTML = '';
                autoCompleteData.model_names.forEach(name => {
                    const option = document.createElement('option');
                    option.value = name;
                    modelNamesList.appendChild(option);
                });
            }

            // 新增：填充 Domain/URL 选项
            const domainUrlsList = document.getElementById('domain-urls-list');
            if (domainUrlsList) {
                domainUrlsList.innerHTML = ''; // 清空现有选项
                console.log("Populating domain-urls-list with:", autoCompleteData.domain_or_urls); // Debug log
                autoCompleteData.domain_or_urls.forEach(url => {
                    const option = document.createElement('option');
                    option.value = url;
                    domainUrlsList.appendChild(option);
                });
                console.log("Finished populating domain-urls-list. Current innerHTML:", domainUrlsList.innerHTML); // Debug log
            } else {
                console.error("Could not find datalist element with ID 'domain-urls-list'"); // Debug log
            }

            // 填充类型选择框 - 从后端获取所有支持的 AI 类型
            const typeNameSelect = document.getElementById('typeName');
            if (typeNameSelect) {
                // 保留第一个空选项
                typeNameSelect.innerHTML = '';
                
                // 添加默认提示选项
                const defaultOption = document.createElement('option');
                defaultOption.value = '';
                defaultOption.textContent = '-- 请选择类型 --';
                typeNameSelect.appendChild(defaultOption);
                
                // 添加从服务器获取的类型选项
                if (autoCompleteData.model_types && autoCompleteData.model_types.length > 0) {
                    autoCompleteData.model_types.forEach(type => {
                        const option = document.createElement('option');
                        option.value = type;
                        option.textContent = type;
                        typeNameSelect.appendChild(option);
                    });
                    
                    // 默认选择 openai（如果存在）
                    if (autoCompleteData.model_types.includes('openai')) {
                        typeNameSelect.value = 'openai';
                    }
                } else {
                    // 后端未返回数据时，添加一些常见类型作为默认选项
                    const defaultTypes = ['openai', 'siliconflow', 'tongyi', 'moonshot', 'chatglm', 'deepseek', 'gemini', 'ollama'];
                    defaultTypes.forEach(type => {
                        const option = document.createElement('option');
                        option.value = type;
                        option.textContent = type;
                        typeNameSelect.appendChild(option);
                    });
                    // 默认选择 openai
                    typeNameSelect.value = 'openai';
                }
            }
            
            // 添加输入事件处理器
            const domainInput = document.getElementById('domainOrURL');
            const providerModeSelect = document.getElementById('providerMode');
            if (domainInput) {
                // 根据选择的类型预填充常见域名和联动模式
                document.getElementById('typeName').addEventListener('change', function() {
                    const selectedType = this.value.toLowerCase();
                    let suggestedDomain = '';
                    
                    // 根据类型提供默认域名建议
                    const domainSuggestions = {
                        'openai': 'api.openai.com',
                        'siliconflow': 'api.siliconflow.cn',
                        'tongyi': '', // 通义不需要域名
                        'moonshot': 'api.moonshot.cn',
                        'deepseek': 'api.deepseek.com',
                        'gemini': '', // Gemini 使用 Google API
                        'ollama': 'localhost:11434',
                        'chatglm': 'open.bigmodel.cn'
                    };
                    
                    if (domainSuggestions[selectedType] !== undefined) {
                        suggestedDomain = domainSuggestions[selectedType];
                    }
                    
                    // 如果域名输入框为空，则填充默认值
                    if (!domainInput.value.trim() && suggestedDomain) {
                        domainInput.value = suggestedDomain;
                    }
                    
                    // 类型和模式联动：大多数类型使用 chat 模式
                    // 如果类型名中包含 embedding 则自动选择 embedding 模式
                    if (providerModeSelect) {
                        if (selectedType.includes('embedding')) {
                            providerModeSelect.value = 'embedding';
                        } else {
                            providerModeSelect.value = 'chat';
                        }
                    }
                });
            }
            
            // 添加实时表单验证
            setupFormValidation();
        }
        
        // 设置表单验证
        function resetValidationStatus() {
            const submitBtn = document.getElementById('submitAddProviderBtn');
            if (submitBtn) {
                submitBtn.disabled = true;
                submitBtn.style.backgroundColor = '#bdbdbd';
                submitBtn.style.cursor = 'not-allowed';
                // 保持按钮样式一致
                submitBtn.style.minWidth = '120px';
                submitBtn.style.height = '40px';
                submitBtn.style.fontSize = '14px';
                submitBtn.style.fontWeight = '500';
                submitBtn.style.borderRadius = '4px';
                submitBtn.style.border = 'none';
                submitBtn.style.transition = 'all 0.3s ease';
                submitBtn.style.boxShadow = '0 2px 5px rgba(0,0,0,0.1)';
                submitBtn.style.padding = '0 15px';
            }
            const validationResultDiv = document.getElementById('validationResult');
            if (validationResultDiv) {
                validationResultDiv.innerHTML = '';
                validationResultDiv.className = 'validation-message'; // Reset to default class
            }
            isProviderConfigValidated = false;
            console.log('Provider validation status reset.'); // Debug log
        }

        // Function to validate provider configuration
        async function validateProviderConfiguration() {
            const wrapperNameInput = document.getElementById('wrapperName');
            const modelNameInput = document.getElementById('modelName');
            const typeNameSelect = document.getElementById('typeName');
            const domainOrURLInput = document.getElementById('domainOrURL');
            const apiKeysTextarea = document.getElementById('apiKeys');
            const noHTTPSCheckbox = document.getElementById('noHTTPS');
            const validationResultDiv = document.getElementById('validationResult');
            const submitBtn = document.getElementById('submitAddProviderBtn');
            const validateBtn = document.getElementById('validateConfigBtn');

            // 更新验证按钮状态
            validateBtn.disabled = true;
            validateBtn.innerHTML = '验证中...';
            validateBtn.style.backgroundColor = '#bdbdbd';
            // 保持按钮样式一致
            validateBtn.style.minWidth = '120px';
            validateBtn.style.height = '40px';
            validateBtn.style.fontSize = '14px';
            validateBtn.style.fontWeight = '500';
            validateBtn.style.borderRadius = '4px';
            validateBtn.style.border = 'none';
            validateBtn.style.transition = 'all 0.3s ease';
            validateBtn.style.boxShadow = '0 1px 3px rgba(0,0,0,0.1)';
            validateBtn.style.padding = '0 15px';

            // Clear previous results and disable submit button
            validationResultDiv.innerHTML = '';
            validationResultDiv.className = 'validation-message';
            validationResultDiv.style.padding = '10px';
            validationResultDiv.style.borderRadius = '4px';
            
            submitBtn.disabled = true;
            submitBtn.style.backgroundColor = '#bdbdbd';
            submitBtn.style.cursor = 'not-allowed';
            // 保持按钮样式一致
            submitBtn.style.minWidth = '120px';
            submitBtn.style.height = '40px';
            submitBtn.style.fontSize = '14px';
            submitBtn.style.fontWeight = '500';
            submitBtn.style.borderRadius = '4px';
            submitBtn.style.border = 'none';
            submitBtn.style.transition = 'all 0.3s ease';
            submitBtn.style.boxShadow = '0 1px 3px rgba(0,0,0,0.1)';
            submitBtn.style.padding = '0 15px';
            
            isProviderConfigValidated = false;

            const wrapperName = wrapperNameInput.value.trim();
            const modelName = modelNameInput.value.trim();
            const typeName = typeNameSelect.value.trim();
            const domainOrURL = domainOrURLInput.value.trim();
            const firstApiKey = apiKeysTextarea.value.split('\n')[0].trim();
            const providerModeSelect = document.getElementById('providerMode');
            const providerMode = providerModeSelect ? providerModeSelect.value : 'chat';

            if (!wrapperName || !modelName || !typeName || !firstApiKey) {
                validationResultDiv.textContent = '请填写提供者名称、模型名称、类型和至少一个API密钥进行验证。';
                validationResultDiv.className = 'validation-message error';
                
                // 恢复验证按钮状态
                validateBtn.disabled = false;
                validateBtn.innerHTML = '验证配置';
                validateBtn.style.backgroundColor = '#4285f4';
                validateBtn.style.boxShadow = '0 2px 5px rgba(0,0,0,0.1)';
                return;
            }
            
            validationResultDiv.textContent = '正在验证配置...';
            validationResultDiv.className = 'validation-message info';

            try {
                const params = new URLSearchParams();
                params.append('wrapper_name', wrapperName);
                params.append('model_name', modelName);
                params.append('model_type', typeName);
                params.append('domain_or_url', domainOrURL);
                params.append('api_key_to_validate', firstApiKey);
                params.append('provider_mode', providerMode); // 添加 provider_mode
                if (noHTTPSCheckbox.checked) {
                    params.append('no_https', 'on');
                }

                const response = await fetch('/portal/validate-provider', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: params
                });

                const result = await response.json(); // Expecting JSON: { "success": true/false, "message": "..." }

                if (response.ok && result.success) {
                    let successMsg = `验证成功: ${result.message || 'Provider validated successfully'}`;
                    if (result.latency !== undefined && result.latency > 0) {
                        successMsg += ` (延迟: ${result.latency}ms)`;
                    }
                    validationResultDiv.textContent = successMsg;
                    validationResultDiv.className = 'validation-message success';
                    submitBtn.disabled = false;
                    submitBtn.style.backgroundColor = '#4caf50'; // 绿色，表示成功
                    submitBtn.style.cursor = 'pointer';
                    submitBtn.style.boxShadow = '0 2px 5px rgba(0,0,0,0.1)';
                    isProviderConfigValidated = true;
                } else {
                    validationResultDiv.textContent = `验证失败: ${result.message || '配置无效或服务器发生错误。'}`;
                    validationResultDiv.className = 'validation-message error';
                    submitBtn.disabled = true;
                    submitBtn.style.backgroundColor = '#bdbdbd';
                    submitBtn.style.cursor = 'not-allowed';
                    submitBtn.style.boxShadow = '0 1px 3px rgba(0,0,0,0.1)';
                    isProviderConfigValidated = false;
                }
            } catch (error) {
                validationResultDiv.textContent = '验证请求失败: ' + error.message;
                validationResultDiv.className = 'validation-message error';
                submitBtn.disabled = true;
                submitBtn.style.backgroundColor = '#bdbdbd';
                submitBtn.style.cursor = 'not-allowed';
                submitBtn.style.boxShadow = '0 1px 3px rgba(0,0,0,0.1)';
                isProviderConfigValidated = false;
                console.error('Validation request failed:', error); // Debug log
            } finally {
                // 恢复验证按钮状态
                validateBtn.disabled = false;
                validateBtn.innerHTML = '验证配置';
                validateBtn.style.backgroundColor = '#4285f4';
                validateBtn.style.boxShadow = '0 2px 5px rgba(0,0,0,0.1)';
            }
        }

        // 验证和提交表单
        async function validateAndSubmit(event) {
            event.preventDefault();
            
            // 获取表单数据
            const wrapperName = document.getElementById('wrapper_name').value.trim();
            const modelName = document.getElementById('model_name').value.trim();
            const modelType = document.getElementById('model_type').value.trim();
            const domainOrUrl = document.getElementById('domain_or_url').value.trim();
            const apiKeys = document.getElementById('api_keys').value.trim();
            const noHttps = document.getElementById('no_https').checked;

            // 验证必填字段
            if (!wrapperName || !modelName || !modelType || !apiKeys) {
                showToast('请填写所有必填字段', 'error');
                return false;
            }

            // 验证 API Keys 格式
            const apiKeysList = apiKeys.split('\n')
                .map(key => key.trim())
                .filter(key => key.length > 0);

            if (apiKeysList.length === 0) {
                showToast('请至少提供一个有效的 API Key', 'error');
                return false;
            }

            try {
                showToast('正在添加提供者...', 'info');

                const formData = new FormData();
                formData.append('wrapper_name', wrapperName);
                formData.append('model_name', modelName);
                formData.append('model_type', modelType);
                formData.append('domain_or_url', domainOrUrl);
                formData.append('api_keys', apiKeys);
                if (noHttps) {
                    formData.append('no_https', 'on');
                }

                const response = await fetch('/portal/add-providers', {
                    method: 'POST',
                    body: formData
                });

                if (!response.ok) {
                    throw new Error('添加失败');
                }

                showToast('提供者添加成功', 'success');
                setTimeout(() => window.location.reload(), 1000);
            } catch (error) {
                showToast('添加失败: ' + error.message, 'error');
                return false;
            }
        }

        // 验证提供者配置
        async function validateProvider() {
            const wrapperName = document.getElementById('wrapper_name').value.trim();
            const modelName = document.getElementById('model_name').value.trim();
            const modelType = document.getElementById('model_type').value.trim();
            const domainOrUrl = document.getElementById('domain_or_url').value.trim();
            const apiKeys = document.getElementById('api_keys').value.trim();
            const noHttps = document.getElementById('no_https').checked;

            // 验证必填字段
            if (!wrapperName || !modelName || !modelType || !apiKeys) {
                showToast('请填写所有必填字段', 'error');
                return;
            }

            const firstApiKey = apiKeys.split('\n')[0].trim();
            if (!firstApiKey) {
                showToast('请至少提供一个有效的 API Key', 'error');
                return;
            }

            try {
                showToast('正在验证配置...', 'info');

                const formData = new FormData();
                formData.append('wrapper_name', wrapperName);
                formData.append('model_name', modelName);
                formData.append('model_type', modelType);
                formData.append('domain_or_url', domainOrUrl);
                formData.append('api_key_to_validate', firstApiKey);
                if (noHttps) {
                    formData.append('no_https', 'on');
                }

                const response = await fetch('/portal/validate-provider', {
                    method: 'POST',
                    body: formData
                });

                const result = await response.json();
                if (result.success) {
                    showToast(result.message, 'success');
                } else {
                    showToast(result.message, 'error');
                }
            } catch (error) {
                showToast('验证失败: ' + error.message, 'error');
            }
        }

        // 快速添加供应商 - 修改后
        function quickAddProvider(providerId) {
            const row = document.querySelector(`tr[data-id="${providerId}"]`);
            if (!row) {
                 console.error(`Provider row not found for ID: ${providerId}`); // Debug log
                 hideContextMenu();
                 return;
            }
            console.log(`Quick adding based on provider ID: ${providerId}`); // Debug log

            // 从选中行提取数据 (优先使用 data-full-text)
            const wrapperName = row.cells[3].getAttribute('data-full-text') || row.cells[3].textContent.trim(); // Cell 4: Provider
            const modelName = row.cells[4].getAttribute('data-full-text') || row.cells[4].textContent.trim();   // Cell 5: Model
            const typeName = row.cells[5].getAttribute('data-full-text') || row.cells[5].textContent.trim();     // Cell 6: Type
            const domainOrURL = row.cells[6].getAttribute('data-full-text') || row.cells[6].textContent.trim(); // Cell 7: Domain
            const apiKey = row.cells[7].getAttribute('data-full-text'); // Cell 8: API Key (get full key)

            console.log(`Extracted data: Wrapper=${wrapperName}, Model=${modelName}, Type=${typeName}, Domain=${domainOrURL}, Key=...${apiKey ? apiKey.slice(-4) : ''}`); // Debug log

            // 切换到 'add' 标签页
            switchTab('add');

            // 检查表单是否准备就绪的函数
            const checkFormReady = (callback) => {
                const form = document.getElementById('addProviderForm');
                const wrapperInput = document.getElementById('wrapperName');
                const modelInput = document.getElementById('modelName');
                const typeSelect = document.getElementById('typeName');
                const domainInput = document.getElementById('domainOrURL');
                const apiKeysInput = document.getElementById('apiKeys');

                if (form && wrapperInput && modelInput && typeSelect && domainInput && apiKeysInput) {
                    console.log("Add provider form is ready."); // Debug log
                    callback(); // 表单元素存在，执行回调
                } else {
                    console.log("Add provider form not ready yet, waiting..."); // Debug log
                    // 稍等后再次检查
                    setTimeout(() => checkFormReady(callback), 50); // 每 50ms 检查一次
                }
            };

            // 等待表单加载完成后填充数据
            checkFormReady(() => {
                const wrapperInput = document.getElementById('wrapperName');
                const modelInput = document.getElementById('modelName');
                const typeSelect = document.getElementById('typeName');
                const domainInput = document.getElementById('domainOrURL');
                const apiKeysInput = document.getElementById('apiKeys');

                // 填充表单字段
                wrapperInput.value = wrapperName;
                modelInput.value = modelName;
                domainInput.value = domainOrURL;
                apiKeysInput.value = apiKey || ''; // 填充 API keys

                // 设置类型 - 直接使用原始类型值
                // 如果原始类型存在于选项中，则选择它；否则尝试匹配或保持默认
                let typeFound = false;
                for (let i = 0; i < typeSelect.options.length; i++) {
                    if (typeSelect.options[i].value === typeName) {
                        typeSelect.value = typeName;
                        typeFound = true;
                        break;
                    }
                }
                if (!typeFound && typeName) {
                    // 尝试小写匹配
                    const lowerTypeName = typeName.toLowerCase();
                    for (let i = 0; i < typeSelect.options.length; i++) {
                        if (typeSelect.options[i].value.toLowerCase() === lowerTypeName) {
                            typeSelect.value = typeSelect.options[i].value;
                            typeFound = true;
                            break;
                        }
                    }
                }
                console.log(`Set type to "${typeSelect.value}" (original: "${typeName}", found: ${typeFound})`);
                // 触发 change 事件以处理可能的依赖逻辑（如域名建议）
                typeSelect.dispatchEvent(new Event('change'));

                // 设置值后重新验证必填字段
                [wrapperInput, modelInput, typeSelect].forEach(input => validateInput.call(input));
                // 处理 domainOrURL 的验证状态 (可能需要 is-valid)
                validateInput.call(domainInput);
                 // 处理 apiKeys 的验证状态 (现在预填充了，设为 valid)
                 validateInput.call(apiKeysInput); // Use the standard validation function

                // 聚焦到 API Keys 输入框
                apiKeysInput.focus();

                // 在填充值后重置整体表单验证状态（按钮启用状态、消息）
                resetValidationStatus();
                console.log("Form populated and validation reset."); // Debug log
            });

            hideContextMenu(); // 立即关闭右键菜单
        }

        // 新增：编辑允许模型的函数 (使用新的模型选择组件)
        function editAllowedModels(id, currentAllowedModelsString) {
            console.log("[Debug] editAllowedModels called with ID:", id, "Models:", currentAllowedModelsString);
            
            document.getElementById('editApiKeyId').value = id;
            document.getElementById('editingApiKeyIdDisplay').textContent = id;
            
            // 获取所有可用的 WrapperName
            let availableWrapperNames = new Set();
            if (autoCompleteData && autoCompleteData.wrapper_names && autoCompleteData.wrapper_names.length > 0) {
                autoCompleteData.wrapper_names.forEach(name => availableWrapperNames.add(name));
            } else {
                const providerRows = document.querySelectorAll('#all #provider-table-body tr');
                providerRows.forEach(row => {
                    const wrapperNameCell = row.cells[3];
                    if (wrapperNameCell) {
                        const wrapperName = wrapperNameCell.getAttribute('data-full-text') || wrapperNameCell.textContent.trim();
                        if (wrapperName) {
                            availableWrapperNames.add(wrapperName);
                        }
                    }
                });
            }
            
            // 也添加 portalAvailableModels 中的模型
            if (portalAvailableModels && portalAvailableModels.length > 0) {
                portalAvailableModels.forEach(name => availableWrapperNames.add(name));
            }
            
            // 排序可用模型
            const sortedAvailableModels = Array.from(availableWrapperNames).sort();
            
            // 解析当前允许的模型
            const currentlyAllowedModelsArray = currentAllowedModelsString ? currentAllowedModelsString.split(',').map(m => m.trim()).filter(m => m) : [];
            
            // 分离正常模型和 glob 模式
            editModalSelectedModels.clear();
            const globPatterns = [];
            currentlyAllowedModelsArray.forEach(m => {
                if (m.includes('*')) {
                    globPatterns.push(m);
                } else {
                    editModalSelectedModels.add(m);
                }
            });
            
            // 设置 glob 模式
            const globInput = document.getElementById('editModalGlobPatterns');
            if (globInput) {
                globInput.value = globPatterns.join(',');
                globInput.addEventListener('input', editModalUpdateSelectedPreview);
            }
            
            // 渲染模型列表
            editModalRenderModelList(sortedAvailableModels);
            
            const modalElement = document.getElementById('editAllowedModelsModal');
            if (modalElement) {
                modalElement.style.display = 'flex';
            }
        }
        
        // Edit Modal 模型列表渲染
        function editModalRenderModelList(availableModels) {
            const modelList = document.getElementById('editModalModelList');
            if (!modelList) return;
            
            if (!availableModels || availableModels.length === 0) {
                modelList.innerHTML = '<div style="padding: 20px; text-align: center; color: #888;">No models available</div>';
                return;
            }
            
            // Store available models for this modal
            window.editModalAvailableModels = availableModels;
            
            modelList.innerHTML = availableModels.map(model => `
                <div class="model-item ${editModalSelectedModels.has(model) ? 'selected' : ''}" onclick="editModalToggleModel('${model}')">
                    <input type="checkbox" ${editModalSelectedModels.has(model) ? 'checked' : ''} onclick="event.stopPropagation(); editModalToggleModel('${model}')">
                    <label>${model}</label>
                </div>
            `).join('');
            
            editModalUpdateSelectedPreview();
        }
        
        function editModalToggleModel(model) {
            if (editModalSelectedModels.has(model)) {
                editModalSelectedModels.delete(model);
            } else {
                editModalSelectedModels.add(model);
            }
            if (window.editModalAvailableModels) {
                editModalRenderModelList(window.editModalAvailableModels);
            }
        }
        
        function editModalSelectAllModels() {
            if (window.editModalAvailableModels) {
                window.editModalAvailableModels.forEach(m => editModalSelectedModels.add(m));
                editModalRenderModelList(window.editModalAvailableModels);
            }
        }
        
        function editModalClearAllModels() {
            editModalSelectedModels.clear();
            if (window.editModalAvailableModels) {
                editModalRenderModelList(window.editModalAvailableModels);
            }
        }
        
        function editModalUpdateSelectedPreview() {
            const preview = document.getElementById('editModalSelectedPreview');
            if (!preview) return;
            
            const globInput = document.getElementById('editModalGlobPatterns');
            const globPatterns = globInput ? globInput.value.trim() : '';
            
            let html = '<strong>已选择:</strong> ';
            
            const modelArray = Array.from(editModalSelectedModels).sort();
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
                html += '<span style="color: #888;">未选择</span>';
            }
            
            preview.innerHTML = html;
        }
        
        function editModalGetSelectedModels() {
            const modelArray = Array.from(editModalSelectedModels);
            const globInput = document.getElementById('editModalGlobPatterns');
            const globPatterns = globInput ? globInput.value.trim() : '';
            const globArray = globPatterns ? globPatterns.split(',').map(p => p.trim()).filter(p => p) : [];
            return [...modelArray, ...globArray].sort();
        }

        // 新增：关闭编辑模态框的函数
        function closeEditAllowedModelsModal() {
            const modalElement = document.getElementById('editAllowedModelsModal');
            if (modalElement) {
                modalElement.style.display = 'none';
            }
            editModalSelectedModels.clear();
        }

        // 新增：保存编辑的模型
        function saveAllowedModels() {
            const id = document.getElementById('editApiKeyId').value;
            const selectedModels = editModalGetSelectedModels();
            
            if (selectedModels.length === 0) {
                showToast('请至少选择一个模型或输入一个 glob 模式', 'warning');
                return;
            }

            console.log("[Debug] saveAllowedModels called. ID:", id, "Selected Models:", selectedModels);

            fetch(`/portal/update-api-key-allowed-models/${id}`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    allowed_models: selectedModels
                })
            })
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    closeEditAllowedModelsModal();
                    showToast('允许的模型已更新', 'success');
                    setTimeout(() => window.location.reload(), 1500);
                } else {
                    showToast('错误: ' + (data.message || '更新失败'), 'error');
                    console.error('Error updating allowed models:', data.message);
                }
            })
            .catch(error => {
                console.error('Fetch Error:', error);
                showToast('更新允许的模型失败: ' + error.message, 'error');
            });
        }

        // 新增：触发从上下文菜单编辑允许的模型 (确保调用纯JS版本)
        function triggerEditAllowedModelsFromContextMenu() {
            console.log("[Debug] triggerEditAllowedModelsFromContextMenu called. ID:", contextApiIdForEdit, "Models:", contextModelsForEdit); // Log: Function called
            if (contextApiIdForEdit && contextModelsForEdit !== null) {
                editAllowedModels(contextApiIdForEdit, contextModelsForEdit);
            }
            hideContextMenu();
        }

        // New function: Copy similar provider keys
        function copySimilarProviderKeys(providerId) {
            console.log(`Copy similar provider keys for provider ID: ${providerId}`);
            
            if (!providerId) {
                showToast('未找到供应商ID', 'error');
                return;
            }

            // 获取当前供应商的信息
            const currentRow = document.querySelector(`tr[data-id="${providerId}"]`);
            if (!currentRow) {
                showToast('未找到当前供应商信息', 'error');
                return;
            }

            // 提取当前供应商的模型名、类型名和域名
            const cells = currentRow.cells;
            const currentWrapperName = cells[3].getAttribute('data-full-text') || cells[3].textContent.trim(); // 提供者
            const currentModelName = cells[4].getAttribute('data-full-text') || cells[4].textContent.trim();   // 模型
            const currentTypeName = cells[5].getAttribute('data-full-text') || cells[5].textContent.trim();    // 类型
            const currentDomainOrURL = cells[6].getAttribute('data-full-text') || cells[6].textContent.trim(); // 域名

            console.log(`Current provider: ${currentWrapperName}, Model: ${currentModelName}, Type: ${currentTypeName}, Domain: ${currentDomainOrURL}`);

            // 查找相同的供应商（模型+类型+域名相同）
            const allRows = document.querySelectorAll('#provider-table-body tr[data-id]');
            const similarProviders = [];

            allRows.forEach(row => {
                const rowCells = row.cells;
                const rowModelName = rowCells[4].getAttribute('data-full-text') || rowCells[4].textContent.trim();
                const rowTypeName = rowCells[5].getAttribute('data-full-text') || rowCells[5].textContent.trim();
                const rowDomainOrURL = rowCells[6].getAttribute('data-full-text') || rowCells[6].textContent.trim();
                
                // 判断是否为相同的供应商（模型+类型+域名都相同）
                if (rowModelName === currentModelName && 
                    rowTypeName === currentTypeName && 
                    rowDomainOrURL === currentDomainOrURL) {
                    
                    const providerId = row.getAttribute('data-id');
                    const wrapperName = rowCells[3].getAttribute('data-full-text') || rowCells[3].textContent.trim();
                    const apiKey = rowCells[7].getAttribute('data-full-text') || ''; // API Key
                    
                    similarProviders.push({
                        id: providerId,
                        wrapperName: wrapperName,
                        modelName: rowModelName,
                        typeName: rowTypeName,
                        domainOrURL: rowDomainOrURL,
                        apiKey: apiKey
                    });
                }
            });

            if (similarProviders.length === 0) {
                showToast('未找到相同配置的供应商', 'warning');
                return;
            }

            // 显示结果弹窗
            showSimilarKeysModal(similarProviders, currentModelName, currentTypeName, currentDomainOrURL);
            hideContextMenu();
        }

        // 显示相同供应商Keys的弹窗
        function showSimilarKeysModal(providers, modelName, typeName, domainOrURL) {
            const modal = document.getElementById('copySimilarKeysModal');
            const titleElement = document.getElementById('similarKeysModalTitle');
            const descElement = document.getElementById('similarKeysModalDesc');
            const infoElement = document.getElementById('similarProvidersInfo');
            const textareaElement = document.getElementById('similarKeysTextarea');
            const countElement = document.getElementById('similarKeysCount');

            // 设置标题和描述
            titleElement.textContent = `${modelName} 的同类供应商 API Keys`;
            descElement.textContent = '找到以下相同配置的供应商：';

            // 设置配置信息
            infoElement.innerHTML = `
                <strong>匹配条件：</strong><br>
                模型名称：${modelName}<br>
                类型：${typeName}<br>
                域名/URL：${domainOrURL || '(默认)'}
            `;

            // 收集API Keys
            const apiKeys = providers
                .map(p => p.apiKey)
                .filter(key => key && key.trim() !== '')
                .filter((key, index, arr) => arr.indexOf(key) === index); // 去重

            // 显示在textarea中
            textareaElement.value = apiKeys.join('\n');
            countElement.textContent = apiKeys.length;

            // 显示弹窗（使用 flex 以便居中）
            modal.style.display = 'flex';

            console.log(`Found ${providers.length} similar providers with ${apiKeys.length} unique API keys`);
        }

        // 关闭相同供应商Keys弹窗
        function closeCopySimilarKeysModal() {
            const modal = document.getElementById('copySimilarKeysModal');
            modal.style.display = 'none';
        }

        // 复制相同供应商Keys到剪贴板
        function copySimilarKeysToClipboard() {
            const textareaElement = document.getElementById('similarKeysTextarea');
            const content = textareaElement.value;

            if (!content.trim()) {
                showToast('没有可复制的内容', 'warning');
                return;
            }

            // 尝试使用现代剪贴板API
            if (navigator.clipboard && navigator.clipboard.writeText) {
                navigator.clipboard.writeText(content).then(() => {
                    showToast(`已复制 ${content.split('\n').filter(line => line.trim()).length} 个API Key到剪贴板`, 'success');
                }).catch(err => {
                    console.error('复制失败:', err);
                    fallbackCopyToClipboard(content);
                });
            } else {
                // 降级处理：使用传统方法
                fallbackCopyToClipboard(content);
            }
        }

        // 降级复制方法
        function fallbackCopyToClipboard(text) {
            const textareaElement = document.getElementById('similarKeysTextarea');
            textareaElement.select();
            textareaElement.setSelectionRange(0, 99999); // 适用于移动设备

            try {
                const successful = document.execCommand('copy');
                if (successful) {
                    showToast(`已复制 ${text.split('\n').filter(line => line.trim()).length} 个API Key到剪贴板`, 'success');
                } else {
                    showToast('复制失败，请手动选择并复制', 'error');
                }
            } catch (err) {
                console.error('复制失败:', err);
                showToast('复制失败，请手动选择并复制', 'error');
            }
        }
        // Show Curl Command Modal
        function showCurlCommand(modelName) {
            // 获取当前页面的域名和协议
            const baseUrl = window.location.origin;
            const chatApiUrl = baseUrl + '/v1/chat/completions';
            const metaApiUrl = baseUrl + '/v1/query-model-meta-info';
            
            // 构建调用模型的 curl 命令
            const curlChatCommand = `curl -X POST '${chatApiUrl}' \\
  -H 'Content-Type: application/json' \\
  -H 'Authorization: Bearer YOUR_API_KEY' \\
  -d '{
    "model": "${modelName}",
    "messages": [
        {"role": "user", "content": "Hello, how are you?"}
    ],
    "stream": false
}'`;

            // 构建查看模型元信息的 curl 命令（公开接口，无需认证）
            // 支持 name 参数进行前缀过滤
            const curlMetaCommand = `# 查询所有模型信息
curl '${metaApiUrl}'

# 查询指定模型（精确匹配前缀）
curl '${metaApiUrl}?name=${modelName}'`;

            // 创建并显示模态框
            let modal = document.getElementById('curlCommandModal');
            if (!modal) {
                modal = document.createElement('div');
                modal.id = 'curlCommandModal';
                modal.className = 'delete-confirmation-modal';
                modal.innerHTML = `
                    <div class="modal-content" style="width: 750px; max-width: 90%; max-height: 85vh; overflow-y: auto;">
                        <span class="close-modal" onclick="closeCurlModal()">&times;</span>
                        <h3 style="margin-top: 0; color: #2c3e50;">🔗 API 调用示例</h3>
                        
                        <!-- 模型元信息查询 -->
                        <div style="margin-bottom: 20px;">
                            <h4 style="color: #1565c0; margin-bottom: 8px;">📋 查看模型元信息（公开接口）</h4>
                            <p style="color: #666; margin-bottom: 8px; font-size: 13px;">验证模型信息是否正确对外开放：</p>
                            <pre id="curlMetaCommandText" style="background: #1e1e1e; color: #d4d4d4; padding: 12px; border-radius: 5px; overflow-x: auto; font-size: 13px; white-space: pre-wrap; word-break: break-all;"></pre>
                            <button class="btn btn-sm" onclick="copyCurlMetaCommand()" style="background-color: #4caf50; margin-top: 8px;">
                                📋 复制
                            </button>
                        </div>
                        
                        <!-- 调用模型接口 -->
                        <div style="margin-bottom: 15px;">
                            <h4 style="color: #1565c0; margin-bottom: 8px;">💬 调用模型接口</h4>
                            <p style="color: #666; margin-bottom: 8px; font-size: 13px;">使用以下 curl 命令调用 <strong id="curlModelName"></strong> 模型：</p>
                            <pre id="curlCommandText" style="background: #1e1e1e; color: #d4d4d4; padding: 12px; border-radius: 5px; overflow-x: auto; font-size: 13px; white-space: pre-wrap; word-break: break-all;"></pre>
                            <button class="btn btn-sm" onclick="copyCurlCommand()" style="background-color: #4caf50; margin-top: 8px;">
                                📋 复制
                            </button>
                        </div>
                        
                        <div style="padding: 12px; background: #e3f2fd; border-radius: 5px; border-left: 4px solid #2196f3;">
                            <strong style="color: #1565c0;">💡 提示：</strong>
                            <ul style="margin: 5px 0 0 0; padding-left: 20px; color: #444; font-size: 13px;">
                                <li>模型元信息接口是<strong>公开</strong>的，无需 API 密钥</li>
                                <li><code style="background: #fff3e0; padding: 2px 4px; border-radius: 2px;">name</code> 参数支持前缀匹配，如 <code>name=memfit-</code> 可查询所有 memfit- 开头的模型</li>
                                <li>调用模型时请将 <code style="background: #fff3e0; padding: 2px 4px; border-radius: 2px;">YOUR_API_KEY</code> 替换为您的实际 API 密钥</li>
                                <li>设置 <code style="background: #fff3e0; padding: 2px 4px; border-radius: 2px;">"stream": true</code> 可启用流式响应</li>
                                <li>对于免费模型（以 <code>-free</code> 结尾），可省略 Authorization 头</li>
                            </ul>
                        </div>
                        <div class="modal-actions">
                            <button class="btn" onclick="closeCurlModal()" style="background-color: #9e9e9e;">
                                关闭
                            </button>
                        </div>
                    </div>
                `;
                document.body.appendChild(modal);
            }
            
            document.getElementById('curlModelName').textContent = modelName;
            document.getElementById('curlCommandText').textContent = curlChatCommand;
            document.getElementById('curlMetaCommandText').textContent = curlMetaCommand;
            modal.style.display = 'flex';
        }

        function closeCurlModal() {
            const modal = document.getElementById('curlCommandModal');
            if (modal) {
                modal.style.display = 'none';
            }
        }

        function copyCurlCommand() {
            const curlText = document.getElementById('curlCommandText').textContent;
            navigator.clipboard.writeText(curlText).then(() => {
                showToast('模型调用命令已复制到剪贴板', 'success');
            }).catch(err => {
                console.error('复制失败:', err);
                showToast('复制失败，请手动选择并复制', 'error');
            });
        }

        function copyCurlMetaCommand() {
            const curlText = document.getElementById('curlMetaCommandText').textContent;
            navigator.clipboard.writeText(curlText).then(() => {
                showToast('元信息查询命令已复制到剪贴板', 'success');
            }).catch(err => {
                console.error('复制失败:', err);
                showToast('复制失败，请手动选择并复制', 'error');
            });
        }

        // Model Metadata Edit Logic
        function openEditModelModal(name, description, tags, trafficMultiplier) {
            document.getElementById('editModelName').value = name;
            document.getElementById('editModelDescription').value = description;
            document.getElementById('editModelTags').value = tags;
            
            // Set traffic multiplier with default value of 1.0
            const multiplierInput = document.getElementById('editModelTrafficMultiplier');
            if (multiplierInput) {
                multiplierInput.value = trafficMultiplier !== undefined ? trafficMultiplier : 1.0;
            }
            
            document.getElementById('editModelMetaModal').style.display = 'block';
        }

        function closeEditModelModal() {
            document.getElementById('editModelMetaModal').style.display = 'none';
        }

        function saveModelMeta() {
            const name = document.getElementById('editModelName').value;
            const description = document.getElementById('editModelDescription').value;
            const tags = document.getElementById('editModelTags').value;
            
            // Get traffic multiplier
            const multiplierInput = document.getElementById('editModelTrafficMultiplier');
            const trafficMultiplier = multiplierInput ? parseFloat(multiplierInput.value) || 1.0 : 1.0;

            fetch('/portal/update-model-meta', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    model_name: name,
                    description: description,
                    tags: tags,
                    traffic_multiplier: trafficMultiplier
                })
            })
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    showToast('保存成功', 'success');
                    closeEditModelModal();
                    setTimeout(() => window.location.reload(), 1000);
                } else {
                    showToast('保存失败: ' + data.message, 'error');
                }
            })
            .catch(error => {
                console.error('Error:', error);
                showToast('保存失败', 'error');
            });
        }

        // TOTP 相关函数
        function refreshTOTPSecret() {
            if (!confirm('确定要刷新 TOTP 密钥吗？这将使所有客户端需要重新获取密钥。')) {
                return;
            }
            
            fetch('/portal/refresh-totp', {
                method: 'POST',
                credentials: 'same-origin'
            })
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    showToast('TOTP 密钥刷新成功', 'success');
                    document.getElementById('totp-secret').textContent = data.new_secret;
                    document.getElementById('totp-wrapped').textContent = data.wrapped;
                    refreshTOTPCode();
                } else {
                    showToast('刷新失败: ' + data.message, 'error');
                }
            })
            .catch(error => {
                console.error('Error:', error);
                showToast('刷新失败', 'error');
            });
        }

        function refreshTOTPCode() {
            fetch('/portal/get-totp-code', {
                method: 'GET',
                credentials: 'same-origin'
            })
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    document.getElementById('totp-code').textContent = data.code;
                    showToast('验证码已刷新', 'success');
                } else {
                    showToast('刷新失败: ' + data.message, 'error');
                }
            })
            .catch(error => {
                console.error('Error:', error);
                showToast('刷新失败', 'error');
            });
        }

        function copyTOTPSecret() {
            const secret = document.getElementById('totp-secret').textContent.trim();
            navigator.clipboard.writeText(secret).then(() => {
                showToast('密钥已复制到剪贴板', 'success');
            }).catch(err => {
                console.error('复制失败:', err);
                showToast('复制失败', 'error');
            });
        }

        // 自动刷新 TOTP 验证码（每30秒）
        setInterval(function() {
            const totpTab = document.getElementById('totp');
            if (totpTab && totpTab.classList.contains('active')) {
                fetch('/portal/get-totp-code', {
                    method: 'GET',
                    credentials: 'same-origin'
                })
                .then(response => response.json())
                .then(data => {
                    if (data.success) {
                        document.getElementById('totp-code').textContent = data.code;
                    }
                })
                .catch(error => {
                    console.error('Auto refresh error:', error);
                });
            }
        }, 30000);

        // 自动刷新统计卡片（每3秒），保持并发数和搜索次数实时更新
        setInterval(async function() {
            try {
                const response = await authFetch('/portal/api/data');
                if (!response || !response.ok) return;
                const data = await response.json();
                if (checkAuthInResponse(data)) return;
                PortalDataLoader.renderStats(data);
            } catch (e) {
                // Silently ignore refresh errors to avoid spamming console
            }
        }, 3000);

        // ==================== API Key Delete Functions ====================

        // Delete single API key
        async function deleteAPIKey(apiKeyId) {
            if (!confirm('确定要删除这个 API Key 吗？此操作不可恢复。')) {
                return;
            }
            
            try {
                const response = await fetch(`/portal/delete-api-key/${apiKeyId}`, {
                    method: 'DELETE',
                    headers: {
                        'Content-Type': 'application/json'
                    }
                });
                
                const data = await response.json();
                
                if (!response.ok || !data.success) {
                    throw new Error(data.message || '删除API密钥失败');
                }
                
                showToast('API密钥删除成功', 'success');
                setTimeout(() => window.location.reload(), 1000);
            } catch (error) {
                showToast(`删除失败: ${error.message}`, 'error');
            }
        }

        // Delete selected API keys (batch)
        async function confirmDeleteSelectedAPIKeys() {
            const checkboxes = document.querySelectorAll('#api-table tbody .api-checkbox:checked');
            if (checkboxes.length === 0) {
                showToast('请先选择要删除的API密钥', 'warning');
                return;
            }
            
            if (!confirm(`确定要删除选中的 ${checkboxes.length} 个 API Key 吗？此操作不可恢复。`)) {
                return;
            }
            
            const ids = [];
            checkboxes.forEach(checkbox => {
                const row = checkbox.closest('tr');
                const id = row.getAttribute('data-api-id');
                if (id) {
                    ids.push(id);
                }
            });
            
            if (ids.length === 0) {
                showToast('未找到有效的API密钥ID', 'error');
                return;
            }
            
            try {
                const response = await fetch('/portal/delete-api-keys', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ ids: ids })
                });
                
                const data = await response.json();
                
                if (!response.ok || !data.success) {
                    throw new Error(data.message || '批量删除API密钥失败');
                }
                
                showToast(`成功删除 ${data.deletedCount} 个API密钥`, 'success');
                setTimeout(() => window.location.reload(), 1000);
            } catch (error) {
                showToast(`删除失败: ${error.message}`, 'error');
            }
        }

        // ==================== API Key Traffic Limit Functions ====================

        // Show traffic limit dialog
        function showTrafficLimitDialog(apiKeyId, currentLimit, currentUsed, enabled) {
            const limitMB = currentLimit > 0 ? (currentLimit / 1024 / 1024).toFixed(2) : 0;
            const usedMB = currentUsed > 0 ? (currentUsed / 1024 / 1024).toFixed(2) : 0;
            
            const html = `
                <div id="trafficLimitModal" class="delete-confirmation-modal" style="display: flex;">
                    <div class="modal-content">
                        <span class="close-modal" onclick="closeTrafficLimitModal()">&times;</span>
                        <h4>设置流量限制</h4>
                        <div class="form-group">
                            <label>API Key ID: ${apiKeyId}</label>
                        </div>
                        <div class="form-group">
                            <label>当前已使用: ${usedMB} MB</label>
                        </div>
                        <div class="form-group">
                            <label for="trafficLimitInput">流量限额 (MB):</label>
                            <input type="number" id="trafficLimitInput" class="form-control" value="${limitMB}" min="0" step="1">
                            <small class="form-text text-muted">设置为 0 表示不限制</small>
                        </div>
                        <div class="form-group">
                            <label>
                                <input type="checkbox" id="trafficLimitEnable" ${enabled ? 'checked' : ''}>
                                启用流量限制
                            </label>
                        </div>
                        <div class="modal-actions">
                            <button class="btn btn-primary" onclick="saveTrafficLimit(${apiKeyId})">保存</button>
                            <button class="btn" onclick="closeTrafficLimitModal()">取消</button>
                        </div>
                    </div>
                </div>
            `;
            
            document.body.insertAdjacentHTML('beforeend', html);
        }

        function closeTrafficLimitModal() {
            const modal = document.getElementById('trafficLimitModal');
            if (modal) {
                modal.remove();
            }
        }

        async function saveTrafficLimit(apiKeyId) {
            const limitMB = parseFloat(document.getElementById('trafficLimitInput').value) || 0;
            const limitBytes = Math.floor(limitMB * 1024 * 1024);
            const enabled = document.getElementById('trafficLimitEnable').checked;
            
            try {
                const response = await fetch(`/portal/api-key-traffic-limit/${apiKeyId}`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({
                        traffic_limit: limitBytes,
                        enable: enabled
                    })
                });
                
                const data = await response.json();
                
                if (!response.ok || !data.success) {
                    throw new Error(data.message || '保存流量限制失败');
                }
                
                showToast('流量限制设置成功', 'success');
                closeTrafficLimitModal();
                setTimeout(() => window.location.reload(), 1000);
            } catch (error) {
                showToast(`保存失败: ${error.message}`, 'error');
            }
        }

        async function resetAPIKeyTraffic(apiKeyId) {
            if (!confirm('确定要重置这个 API Key 的流量计数吗？')) {
                return;
            }
            
            try {
                const response = await fetch(`/portal/reset-api-key-traffic/${apiKeyId}`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    }
                });
                
                const data = await response.json();
                
                if (!response.ok || !data.success) {
                    throw new Error(data.message || '重置流量失败');
                }
                
                showToast('流量计数已重置', 'success');
                setTimeout(() => window.location.reload(), 1000);
            } catch (error) {
                showToast(`重置失败: ${error.message}`, 'error');
            }
        }

        // ==================== API Key Pagination Functions ====================

        let currentAPIKeyPage = 1;
        let currentAPIKeyPageSize = 20;
        let currentAPIKeySortBy = 'created_at';
        let currentAPIKeySortOrder = 'desc';

        async function loadAPIKeysPage(page, sortBy, sortOrder) {
            if (page !== undefined) currentAPIKeyPage = page;
            if (sortBy !== undefined) currentAPIKeySortBy = sortBy;
            if (sortOrder !== undefined) currentAPIKeySortOrder = sortOrder;
            
            try {
                const response = await fetch(
                    `/portal/api/api-keys?page=${currentAPIKeyPage}&pageSize=${currentAPIKeyPageSize}&sortBy=${currentAPIKeySortBy}&sortOrder=${currentAPIKeySortOrder}`,
                    {
                        method: 'GET',
                        credentials: 'same-origin'
                    }
                );
                
                const data = await response.json();
                
                if (!response.ok || !data.success) {
                    throw new Error(data.message || '加载API密钥失败');
                }
                
                renderAPIKeysTable(data.data, data.pagination);
            } catch (error) {
                showToast(`加载失败: ${error.message}`, 'error');
            }
        }

        function renderAPIKeysTable(keys, pagination) {
            const tbody = document.querySelector('#api-table tbody');
            if (!tbody) return;
            
            tbody.innerHTML = '';
            
            keys.forEach(key => {
                const row = document.createElement('tr');
                row.setAttribute('data-api-id', key.id);
                row.setAttribute('data-api-status', key.active ? 'active' : 'inactive');
                
                // Calculate traffic percentage
                let trafficPercent = 0;
                let trafficDisplay = '-';
                if (key.traffic_limit_enable && key.traffic_limit > 0) {
                    trafficPercent = (key.traffic_used / key.traffic_limit * 100).toFixed(1);
                    trafficDisplay = `${formatBytes(key.traffic_used)} / ${formatBytes(key.traffic_limit)} (${trafficPercent}%)`;
                } else if (key.traffic_used > 0) {
                    trafficDisplay = formatBytes(key.traffic_used);
                }
                
                row.innerHTML = `
                    <td class="checkbox-column">
                        <input type="checkbox" class="api-checkbox">
                    </td>
                    <td class="text-center">${key.id}</td>
                    <td class="text-center" style="align-items:center;gap:2px;padding:2px 4px;">
                        ${key.active 
                            ? '<span class="health-badge healthy" style="flex-shrink:0;font-size:12px;">激活</span><button class="btn btn-sm btn-danger" onclick="toggleAPIKeyStatus(\'' + key.id + '\', false)" title="禁用API密钥" style="margin-left:auto;padding:2px 4px;font-size:12px;">禁用</button>'
                            : '<span class="health-badge unhealthy" style="flex-shrink:0;font-size:12px;">禁用</span><button class="btn btn-sm" onclick="toggleAPIKeyStatus(\'' + key.id + '\', true)" title="激活API密钥" style="margin-left:auto;padding:2px 4px;font-size:12px;">激活</button>'
                        }
                    </td>
                    <td class="copyable api-key-cell" data-full-text="${key.api_key}">${key.display_key}</td>
                    <td class="copyable editable-allowed-models" data-api-id="${key.id}" data-current-models="${key.allowed_models}" data-full-text="${key.allowed_models}" title="右键点击修改允许的模型">${key.allowed_models}</td>
                    <td class="text-center">${key.usage_count}</td>
                    <td class="text-center">${key.web_search_count || 0}</td>
                    <td class="text-center">
                        <span class="health-badge healthy">${key.success_count}</span>
                        <span class="health-badge unhealthy">${key.failure_count}</span>
                    </td>
                    <td class="text-center">
                        <div class="traffic-data">
                            <span title="输入流量">↓ ${formatBytes(key.input_bytes)}</span>
                            <span title="输出流量">↑ ${formatBytes(key.output_bytes)}</span>
                        </div>
                    </td>
                    <td class="text-center">
                        <span class="traffic-info" title="${key.traffic_limit_enable ? '已启用流量限制' : '未启用流量限制'}">${trafficDisplay}</span>
                    </td>
                    <td class="text-center">${key.created_by_ops_name || 'Admin'}</td>
                    <td>${key.last_used_time || '-'}</td>
                    <td class="text-center">
                        <button class="btn btn-sm" onclick="showTrafficLimitDialog(${key.id}, ${key.traffic_limit}, ${key.traffic_used}, ${key.traffic_limit_enable})" title="设置流量限制" style="padding:2px 4px;font-size:12px;">流量</button>
                        <button class="btn btn-sm btn-danger" onclick="deleteAPIKey(${key.id})" title="删除API密钥" style="padding:2px 4px;font-size:12px;">删除</button>
                    </td>
                `;
                
                tbody.appendChild(row);
            });
            
            // Update pagination controls
            updateAPIKeyPagination(pagination);
        }

        function updateAPIKeyPagination(pagination) {
            let paginationContainer = document.getElementById('api-key-pagination');
            if (!paginationContainer) {
                // Create pagination container if not exists
                const apiTable = document.getElementById('api-table');
                if (apiTable && apiTable.parentElement) {
                    paginationContainer = document.createElement('div');
                    paginationContainer.id = 'api-key-pagination';
                    paginationContainer.className = 'pagination-controls';
                    paginationContainer.style.cssText = 'display: flex; justify-content: space-between; align-items: center; margin-top: 15px; padding: 10px; background: #f8f9fa; border-radius: 4px;';
                    apiTable.parentElement.appendChild(paginationContainer);
                }
            }
            
            if (!paginationContainer) return;
            
            const { page, pageSize, total, totalPages } = pagination;
            
            paginationContainer.innerHTML = `
                <div class="pagination-info">
                    共 ${total} 条记录，第 ${page}/${totalPages} 页
                </div>
                <div class="pagination-buttons" style="display: flex; gap: 5px; align-items: center;">
                    <button class="btn btn-sm" ${page <= 1 ? 'disabled' : ''} onclick="loadAPIKeysPage(1)">首页</button>
                    <button class="btn btn-sm" ${page <= 1 ? 'disabled' : ''} onclick="loadAPIKeysPage(${page - 1})">上一页</button>
                    <span style="margin: 0 10px;">
                        跳转到 <input type="number" id="apiKeyPageInput" min="1" max="${totalPages}" value="${page}" style="width: 50px; text-align: center;"> 页
                        <button class="btn btn-sm" onclick="loadAPIKeysPage(parseInt(document.getElementById('apiKeyPageInput').value))">Go</button>
                    </span>
                    <button class="btn btn-sm" ${page >= totalPages ? 'disabled' : ''} onclick="loadAPIKeysPage(${page + 1})">下一页</button>
                    <button class="btn btn-sm" ${page >= totalPages ? 'disabled' : ''} onclick="loadAPIKeysPage(${totalPages})">末页</button>
                    <select onchange="changeAPIKeyPageSize(this.value)" style="margin-left: 10px;">
                        <option value="10" ${pageSize === 10 ? 'selected' : ''}>10条/页</option>
                        <option value="20" ${pageSize === 20 ? 'selected' : ''}>20条/页</option>
                        <option value="50" ${pageSize === 50 ? 'selected' : ''}>50条/页</option>
                        <option value="100" ${pageSize === 100 ? 'selected' : ''}>100条/页</option>
                    </select>
                </div>
            `;
        }

        function changeAPIKeyPageSize(newSize) {
            currentAPIKeyPageSize = parseInt(newSize);
            currentAPIKeyPage = 1; // Reset to first page
            loadAPIKeysPage();
        }

        function sortAPIKeys(column) {
            if (currentAPIKeySortBy === column) {
                // Toggle sort order
                currentAPIKeySortOrder = currentAPIKeySortOrder === 'asc' ? 'desc' : 'asc';
            } else {
                currentAPIKeySortBy = column;
                currentAPIKeySortOrder = 'desc';
            }
            loadAPIKeysPage();
        }

        // Helper function to format bytes
        function formatBytes(bytes) {
            if (bytes === 0 || bytes === undefined || bytes === null) return '0 B';
            const k = 1024;
            const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
            const i = Math.floor(Math.log(bytes) / Math.log(k));
            return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
        }

        // ==================== 可拖拽调整列宽功能 ====================
        
        const ColumnResizer = {
            // 存储键前缀
            storageKeyPrefix: 'aibalance_table_columns_',
            
            // 当前正在拖拽的状态
            resizing: null,
            
            // 初始化所有表格的列宽调整功能
            init: function() {
                const tables = document.querySelectorAll('table');
                tables.forEach((table, index) => {
                    const tableId = table.id || `table-${index}`;
                    this.initTable(table, tableId);
                });
                
                // 添加提示元素
                this.createResizeHint();
                
                console.log('Column resizer initialized for all tables');
            },
            
            // 初始化单个表格
            initTable: function(table, tableId) {
                const thead = table.querySelector('thead');
                if (!thead) return;
                
                const headers = thead.querySelectorAll('th');
                if (headers.length === 0) return;
                
                // 添加可调整大小的类
                table.classList.add('resizable');
                
                // 为每个表头添加拖拽手柄
                headers.forEach((th, colIndex) => {
                    // 跳过复选框列（第一列通常很小）
                    if (th.classList.contains('checkbox-column')) return;
                    
                    th.classList.add('resizable');
                    
                    // 创建拖拽手柄
                    const handle = document.createElement('div');
                    handle.className = 'resize-handle';
                    handle.dataset.colIndex = colIndex;
                    handle.dataset.tableId = tableId;
                    th.appendChild(handle);
                    
                    // 绑定鼠标事件
                    handle.addEventListener('mousedown', (e) => this.startResize(e, th, colIndex, tableId, table));
                });
                
                // 从 localStorage 恢复列宽
                this.restoreColumnWidths(table, tableId);
            },
            
            // 创建提示元素
            createResizeHint: function() {
                if (document.querySelector('.column-resize-hint')) return;
                
                const hint = document.createElement('div');
                hint.className = 'column-resize-hint';
                hint.textContent = 'Drag to resize column. Double-click to auto-fit.';
                document.body.appendChild(hint);
            },
            
            // 开始拖拽
            startResize: function(e, th, colIndex, tableId, table) {
                e.preventDefault();
                e.stopPropagation();
                
                const startX = e.pageX;
                const startWidth = th.offsetWidth;
                
                this.resizing = {
                    th: th,
                    colIndex: colIndex,
                    tableId: tableId,
                    table: table,
                    startX: startX,
                    startWidth: startWidth
                };
                
                th.classList.add('resizing');
                document.body.classList.add('resizing-column');
                
                // 显示提示
                this.showHint(`Column width: ${startWidth}px`);
                
                // 绑定全局事件
                document.addEventListener('mousemove', this.handleMouseMove);
                document.addEventListener('mouseup', this.handleMouseUp);
            },
            
            // 处理鼠标移动
            handleMouseMove: function(e) {
                if (!ColumnResizer.resizing) return;
                
                const { th, startX, startWidth, table, colIndex } = ColumnResizer.resizing;
                const diff = e.pageX - startX;
                const newWidth = Math.max(40, startWidth + diff); // 最小宽度 40px
                
                th.style.width = newWidth + 'px';
                th.style.minWidth = newWidth + 'px';
                
                // 同步调整对应列的单元格宽度
                const rows = table.querySelectorAll('tbody tr');
                rows.forEach(row => {
                    const cell = row.children[colIndex];
                    if (cell) {
                        cell.style.width = newWidth + 'px';
                        cell.style.minWidth = newWidth + 'px';
                    }
                });
                
                ColumnResizer.showHint(`Column width: ${Math.round(newWidth)}px`);
            },
            
            // 处理鼠标释放
            handleMouseUp: function(e) {
                if (!ColumnResizer.resizing) return;
                
                const { th, tableId, table, colIndex } = ColumnResizer.resizing;
                
                th.classList.remove('resizing');
                document.body.classList.remove('resizing-column');
                
                // 保存列宽到 localStorage
                ColumnResizer.saveColumnWidths(table, tableId);
                
                // 隐藏提示
                ColumnResizer.hideHint();
                
                // 清理状态和事件
                ColumnResizer.resizing = null;
                document.removeEventListener('mousemove', ColumnResizer.handleMouseMove);
                document.removeEventListener('mouseup', ColumnResizer.handleMouseUp);
                
                console.log(`Column ${colIndex} width saved for table ${tableId}`);
            },
            
            // 保存列宽到 localStorage
            saveColumnWidths: function(table, tableId) {
                const headers = table.querySelectorAll('thead th');
                const widths = {};
                
                headers.forEach((th, index) => {
                    const width = th.style.width || th.offsetWidth + 'px';
                    widths[index] = parseInt(width);
                });
                
                try {
                    localStorage.setItem(this.storageKeyPrefix + tableId, JSON.stringify(widths));
                    console.log(`Saved column widths for ${tableId}:`, widths);
                } catch (e) {
                    console.warn('Failed to save column widths to localStorage:', e);
                }
            },
            
            // 从 localStorage 恢复列宽
            restoreColumnWidths: function(table, tableId) {
                try {
                    const saved = localStorage.getItem(this.storageKeyPrefix + tableId);
                    if (!saved) return;
                    
                    const widths = JSON.parse(saved);
                    const headers = table.querySelectorAll('thead th');
                    
                    headers.forEach((th, index) => {
                        if (widths[index]) {
                            const width = widths[index] + 'px';
                            th.style.width = width;
                            th.style.minWidth = width;
                        }
                    });
                    
                    // 同步 tbody 列宽
                    const rows = table.querySelectorAll('tbody tr');
                    rows.forEach(row => {
                        headers.forEach((th, index) => {
                            if (widths[index]) {
                                const cell = row.children[index];
                                if (cell) {
                                    const width = widths[index] + 'px';
                                    cell.style.width = width;
                                    cell.style.minWidth = width;
                                }
                            }
                        });
                    });
                    
                    console.log(`Restored column widths for ${tableId}:`, widths);
                } catch (e) {
                    console.warn('Failed to restore column widths from localStorage:', e);
                }
            },
            
            // 重置表格列宽
            resetColumnWidths: function(tableId) {
                try {
                    localStorage.removeItem(this.storageKeyPrefix + tableId);
                    
                    // 移除内联样式
                    const table = tableId ? document.getElementById(tableId) || document.querySelector(`table[data-table-id="${tableId}"]`) : null;
                    if (table) {
                        const headers = table.querySelectorAll('thead th');
                        headers.forEach(th => {
                            th.style.width = '';
                            th.style.minWidth = '';
                        });
                        
                        const cells = table.querySelectorAll('tbody td');
                        cells.forEach(td => {
                            td.style.width = '';
                            td.style.minWidth = '';
                        });
                    }
                    
                    showToast('Column widths reset to default', 'success');
                    console.log(`Reset column widths for ${tableId}`);
                } catch (e) {
                    console.warn('Failed to reset column widths:', e);
                }
            },
            
            // 重置所有表格列宽
            resetAllColumnWidths: function() {
                const keys = [];
                for (let i = 0; i < localStorage.length; i++) {
                    const key = localStorage.key(i);
                    if (key && key.startsWith(this.storageKeyPrefix)) {
                        keys.push(key);
                    }
                }
                
                keys.forEach(key => localStorage.removeItem(key));
                
                // 移除所有表格的内联样式
                document.querySelectorAll('table.resizable').forEach(table => {
                    const headers = table.querySelectorAll('thead th');
                    headers.forEach(th => {
                        th.style.width = '';
                        th.style.minWidth = '';
                    });
                    
                    const cells = table.querySelectorAll('tbody td');
                    cells.forEach(td => {
                        td.style.width = '';
                        td.style.minWidth = '';
                    });
                });
                
                showToast('All column widths reset to default', 'success');
                console.log('All column widths reset');
            },
            
            // 显示提示
            showHint: function(text) {
                const hint = document.querySelector('.column-resize-hint');
                if (hint) {
                    hint.textContent = text;
                    hint.classList.add('visible');
                }
            },
            
            // 隐藏提示
            hideHint: function() {
                const hint = document.querySelector('.column-resize-hint');
                if (hint) {
                    hint.classList.remove('visible');
                }
            }
        };
        
        // 添加重置按钮到表格
        function addResetColumnWidthsButton() {
            // 为 API 表格添加重置按钮
            const apiActionBar = document.querySelector('#api-table')?.closest('.table-container')?.previousElementSibling;
            if (apiActionBar && !apiActionBar.querySelector('.reset-column-widths-btn')) {
                const resetBtn = document.createElement('button');
                resetBtn.className = 'reset-column-widths-btn';
                resetBtn.textContent = 'Reset column widths';
                resetBtn.title = 'Reset column widths to default';
                resetBtn.onclick = () => ColumnResizer.resetColumnWidths('api-table');
                apiActionBar.querySelector('div:last-child')?.appendChild(resetBtn);
            }
        }
        
        // 页面加载完成后初始化列宽调整功能
        document.addEventListener('DOMContentLoaded', function() {
            // 延迟初始化，确保表格已渲染
            setTimeout(() => {
                ColumnResizer.init();
                addResetColumnWidthsButton();
            }, 100);
        });
        
        // 暴露全局函数
        window.resetColumnWidths = function(tableId) {
            ColumnResizer.resetColumnWidths(tableId);
        };
        
        window.resetAllColumnWidths = function() {
            ColumnResizer.resetAllColumnWidths();
        };
        
        // ==================== OPS 用户管理 ====================
        
        let opsUsersData = [];
        let opsUsersPage = 1;
        let opsUsersPageSize = 20;
        let opsUsersPagination = null;
        let opsUsersFilter = '';
        let opsLogsData = [];
        let opsLogsPage = 1;
        let opsLogsPageSize = 20;
        
        // 显示创建 OPS 用户弹窗
        function showCreateOpsUserModal() {
            document.getElementById('createOpsUserModal').style.display = 'flex';
            document.getElementById('newOpsUsername').value = '';
        }
        
        // 关闭创建 OPS 用户弹窗
        function closeCreateOpsUserModal() {
            document.getElementById('createOpsUserModal').style.display = 'none';
        }
        
        // 创建 OPS 用户
        async function createOpsUser() {
            const username = document.getElementById('newOpsUsername').value.trim();
            
            if (!username) {
                showToast('Please enter a username', 'error');
                return;
            }
            
            try {
                const response = await fetch('/portal/api/ops-users', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ username })
                });
                
                const data = await response.json();
                
                if (data.success) {
                    closeCreateOpsUserModal();
                    
                    // 显示凭据弹窗
                    document.getElementById('createdOpsUsername').value = data.username;
                    document.getElementById('createdOpsPassword').value = data.password;
                    document.getElementById('createdOpsKey').value = data.ops_key;
                    document.getElementById('opsUserCredentialsModal').style.display = 'flex';
                    
                    refreshOpsUsers();
                } else {
                    showToast(data.error || 'Failed to create user', 'error');
                }
            } catch (error) {
                console.error('Error creating OPS user:', error);
                showToast('Network error', 'error');
            }
        }
        
        // 关闭凭据弹窗
        function closeOpsUserCredentialsModal() {
            document.getElementById('opsUserCredentialsModal').style.display = 'none';
        }
        
        // 复制 OPS 凭据
        function copyOpsCredentials() {
            const username = document.getElementById('createdOpsUsername').value;
            const password = document.getElementById('createdOpsPassword').value;
            const opsKey = document.getElementById('createdOpsKey').value;
            
            const text = `Username: ${username}\nPassword: ${password}\nOPS Key: ${opsKey}`;
            copyToClipboard(text);
            showToast('Credentials copied to clipboard', 'success');
        }
        
        // 刷新 OPS 用户列表 (支持分页)
        async function refreshOpsUsers(page, pageSize, username) {
            if (page !== undefined) opsUsersPage = page;
            if (pageSize !== undefined) opsUsersPageSize = pageSize;
            if (username !== undefined) opsUsersFilter = username;
            
            const tbody = document.getElementById('ops-users-table-body');
            tbody.innerHTML = '<tr><td colspan="7" style="text-align: center; padding: 20px; color: #666;">加载中...</td></tr>';
            
            try {
                let url = `/portal/api/ops-users?page=${opsUsersPage}&page_size=${opsUsersPageSize}`;
                if (opsUsersFilter) {
                    url += `&username=${encodeURIComponent(opsUsersFilter)}`;
                }
                
                const response = await fetch(url);
                const data = await response.json();
                
                if (data.success) {
                    opsUsersData = data.users || [];
                    opsUsersPagination = data.pagination || null;
                    renderOpsUsersTable();
                } else {
                    tbody.innerHTML = '<tr><td colspan="7" style="text-align: center; padding: 20px; color: #dc3545;">加载失败: ' + (data.error || 'Unknown error') + '</td></tr>';
                }
            } catch (error) {
                console.error('Error loading OPS users:', error);
                tbody.innerHTML = '<tr><td colspan="7" style="text-align: center; padding: 20px; color: #dc3545;">网络错误</td></tr>';
            }
        }
        
        // 切换 OPS 用户页码
        function changeOpsUsersPage(page) {
            if (page < 1) page = 1;
            if (opsUsersPagination && page > opsUsersPagination.total_pages) {
                page = opsUsersPagination.total_pages;
            }
            refreshOpsUsers(page);
        }
        
        // 切换 OPS 用户每页数量
        function changeOpsUsersPageSize(newSize) {
            opsUsersPageSize = parseInt(newSize);
            opsUsersPage = 1; // 重置到第一页
            refreshOpsUsers();
        }
        
        // 搜索 OPS 用户
        function searchOpsUsers() {
            const input = document.getElementById('ops-users-search-input');
            if (input) {
                opsUsersFilter = input.value.trim();
                opsUsersPage = 1; // 重置到第一页
                refreshOpsUsers();
            }
        }
        
        // 渲染 OPS 用户表格
        function renderOpsUsersTable() {
            const tbody = document.getElementById('ops-users-table-body');
            
            if (!opsUsersData || opsUsersData.length === 0) {
                tbody.innerHTML = '<tr><td colspan="7" style="text-align: center; padding: 20px; color: #666;">暂无运营用户</td></tr>';
                renderOpsUsersPagination();
                return;
            }
            
            tbody.innerHTML = opsUsersData.map(user => `
                <tr>
                    <td>${user.id}</td>
                    <td>${user.username}</td>
                    <td><span style="background: #e3f2fd; color: #1565c0; padding: 2px 8px; border-radius: 4px; font-size: 12px;">${user.role.toUpperCase()}</span></td>
                    <td>
                        <span style="background: ${user.active ? '#d4edda' : '#f8d7da'}; color: ${user.active ? '#155724' : '#721c24'}; padding: 2px 8px; border-radius: 4px; font-size: 12px;">
                            ${user.active ? '激活' : '禁用'}
                        </span>
                    </td>
                    <td><code style="font-size: 11px;">${user.ops_key.substring(0, 20)}...</code></td>
                    <td>${user.created_at}</td>
                    <td>
                        <div style="display: flex; gap: 5px; flex-wrap: wrap;">
                            <button class="btn btn-sm" onclick="toggleOpsUserStatus(${user.id}, ${!user.active})" style="font-size: 11px; padding: 4px 8px;">
                                ${user.active ? '禁用' : '启用'}
                            </button>
                            <button class="btn btn-sm" onclick="resetOpsUserPassword(${user.id})" style="font-size: 11px; padding: 4px 8px;">重置密码</button>
                            <button class="btn btn-sm" onclick="resetOpsUserKey(${user.id})" style="font-size: 11px; padding: 4px 8px;">重置Key</button>
                            <button class="btn btn-sm btn-danger" onclick="deleteOpsUser(${user.id})" style="font-size: 11px; padding: 4px 8px;">删除</button>
                        </div>
                    </td>
                </tr>
            `).join('');
            
            // 渲染分页控件
            renderOpsUsersPagination();
        }
        
        // 渲染 OPS 用户分页控件
        function renderOpsUsersPagination() {
            let paginationContainer = document.getElementById('ops-users-pagination');
            if (!paginationContainer) {
                // 创建分页容器
                const table = document.getElementById('ops-users-table');
                if (table && table.parentElement) {
                    paginationContainer = document.createElement('div');
                    paginationContainer.id = 'ops-users-pagination';
                    paginationContainer.className = 'pagination-controls';
                    paginationContainer.style.cssText = 'display: flex; justify-content: space-between; align-items: center; margin-top: 15px; padding: 10px; background: #f8f9fa; border-radius: 4px;';
                    table.parentElement.appendChild(paginationContainer);
                }
            }
            
            if (!paginationContainer || !opsUsersPagination) return;
            
            const { page, page_size, total, total_pages } = opsUsersPagination;
            
            paginationContainer.innerHTML = `
                <div class="pagination-info">
                    共 ${total} 条记录，第 ${page}/${total_pages || 1} 页
                </div>
                <div class="pagination-buttons" style="display: flex; gap: 5px; align-items: center;">
                    <button class="btn btn-sm" ${page <= 1 ? 'disabled' : ''} onclick="changeOpsUsersPage(1)">首页</button>
                    <button class="btn btn-sm" ${page <= 1 ? 'disabled' : ''} onclick="changeOpsUsersPage(${page - 1})">上一页</button>
                    <span style="margin: 0 10px;">
                        跳转到 <input type="number" id="opsUsersPageInput" min="1" max="${total_pages}" value="${page}" style="width: 50px; text-align: center;"> 页
                        <button class="btn btn-sm" onclick="changeOpsUsersPage(parseInt(document.getElementById('opsUsersPageInput').value))">Go</button>
                    </span>
                    <button class="btn btn-sm" ${page >= total_pages ? 'disabled' : ''} onclick="changeOpsUsersPage(${page + 1})">下一页</button>
                    <button class="btn btn-sm" ${page >= total_pages ? 'disabled' : ''} onclick="changeOpsUsersPage(${total_pages})">末页</button>
                    <select onchange="changeOpsUsersPageSize(this.value)" style="margin-left: 10px;">
                        <option value="10" ${page_size === 10 ? 'selected' : ''}>10条/页</option>
                        <option value="20" ${page_size === 20 ? 'selected' : ''}>20条/页</option>
                        <option value="50" ${page_size === 50 ? 'selected' : ''}>50条/页</option>
                        <option value="100" ${page_size === 100 ? 'selected' : ''}>100条/页</option>
                    </select>
                </div>
            `;
        }
        
        // 切换 OPS 用户状态
        async function toggleOpsUserStatus(userId, active) {
            try {
                const response = await fetch(`/portal/api/ops-users/${userId}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ active })
                });
                
                const data = await response.json();
                
                if (data.success) {
                    showToast('User status updated', 'success');
                    refreshOpsUsers();
                } else {
                    showToast(data.error || 'Failed to update status', 'error');
                }
            } catch (error) {
                console.error('Error toggling user status:', error);
                showToast('Network error', 'error');
            }
        }
        
        // 重置 OPS 用户密码
        async function resetOpsUserPassword(userId) {
            if (!confirm('Are you sure you want to reset this user\'s password?')) return;
            
            try {
                const response = await fetch(`/portal/api/ops-users/${userId}/reset-password`, {
                    method: 'POST'
                });
                
                const data = await response.json();
                
                if (data.success) {
                    alert('Password reset successfully!\n\nNew Password: ' + data.new_password);
                    refreshOpsUsers();
                } else {
                    showToast(data.error || 'Failed to reset password', 'error');
                }
            } catch (error) {
                console.error('Error resetting password:', error);
                showToast('Network error', 'error');
            }
        }
        
        // 重置 OPS 用户 Key
        async function resetOpsUserKey(userId) {
            if (!confirm('Are you sure you want to reset this user\'s OPS Key?')) return;
            
            try {
                const response = await fetch(`/portal/api/ops-users/${userId}/reset-key`, {
                    method: 'POST'
                });
                
                const data = await response.json();
                
                if (data.success) {
                    alert('OPS Key reset successfully!\n\nNew OPS Key: ' + data.new_ops_key);
                    refreshOpsUsers();
                } else {
                    showToast(data.error || 'Failed to reset OPS Key', 'error');
                }
            } catch (error) {
                console.error('Error resetting OPS Key:', error);
                showToast('Network error', 'error');
            }
        }
        
        // 删除 OPS 用户
        async function deleteOpsUser(userId) {
            if (!confirm('Are you sure you want to delete this user? This action cannot be undone.')) return;
            
            try {
                const response = await fetch(`/portal/api/ops-users/${userId}`, {
                    method: 'DELETE'
                });
                
                const data = await response.json();
                
                if (data.success) {
                    showToast('User deleted successfully', 'success');
                    refreshOpsUsers();
                } else {
                    showToast(data.error || 'Failed to delete user', 'error');
                }
            } catch (error) {
                console.error('Error deleting user:', error);
                showToast('Network error', 'error');
            }
        }
        
        // ==================== OPS 操作日志 ====================
        
        // 刷新 OPS 日志
        async function refreshOpsLogs() {
            const tbody = document.getElementById('ops-logs-table-body');
            tbody.innerHTML = '<tr><td colspan="8" style="text-align: center; padding: 20px; color: #666;">加载中...</td></tr>';
            
            try {
                const operatorName = document.getElementById('ops-log-filter-operator')?.value || '';
                const action = document.getElementById('ops-log-filter-action')?.value || '';
                
                let url = `/portal/api/ops-logs?page=${opsLogsPage}&page_size=${opsLogsPageSize}`;
                if (operatorName) url += `&operator_name=${encodeURIComponent(operatorName)}`;
                if (action) url += `&action=${encodeURIComponent(action)}`;
                
                const response = await fetch(url);
                const data = await response.json();
                
                if (data.success) {
                    opsLogsData = data.logs || [];
                    renderOpsLogsTable();
                    renderOpsLogsPagination(data.total, data.page, data.page_size);
                } else {
                    tbody.innerHTML = '<tr><td colspan="8" style="text-align: center; padding: 20px; color: #dc3545;">加载失败: ' + (data.error || 'Unknown error') + '</td></tr>';
                }
            } catch (error) {
                console.error('Error loading OPS logs:', error);
                tbody.innerHTML = '<tr><td colspan="8" style="text-align: center; padding: 20px; color: #dc3545;">网络错误</td></tr>';
            }
        }
        
        // 筛选 OPS 日志
        function filterOpsLogs() {
            opsLogsPage = 1;
            refreshOpsLogs();
        }
        
        // 清除 OPS 日志筛选
        function clearOpsLogsFilter() {
            const operatorInput = document.getElementById('ops-log-filter-operator');
            const actionSelect = document.getElementById('ops-log-filter-action');
            if (operatorInput) operatorInput.value = '';
            if (actionSelect) actionSelect.value = '';
            opsLogsPage = 1;
            refreshOpsLogs();
        }
        window.clearOpsLogsFilter = clearOpsLogsFilter;
        
        // 渲染 OPS 日志表格
        function renderOpsLogsTable() {
            const tbody = document.getElementById('ops-logs-table-body');
            
            if (!opsLogsData || opsLogsData.length === 0) {
                tbody.innerHTML = '<tr><td colspan="8" style="text-align: center; padding: 20px; color: #666;">暂无操作日志</td></tr>';
                return;
            }
            
            const actionLabels = {
                'create_api_key': '创建 API Key',
                'delete_api_key': '删除 API Key',
                'update_api_key': '更新 API Key',
                'reset_api_key_traffic': '重置流量',
                'reset_ops_key': '重置 OPS Key',
                'change_password': '修改密码'
            };
            
            tbody.innerHTML = opsLogsData.map(log => `
                <tr>
                    <td>${log.id}</td>
                    <td>${log.operator_name}</td>
                    <td><span style="background: #e3f2fd; color: #1565c0; padding: 2px 8px; border-radius: 4px; font-size: 12px;">${actionLabels[log.action] || log.action}</span></td>
                    <td>${log.target_type}</td>
                    <td>${log.target_id}</td>
                    <td style="max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title="${log.detail}">${log.detail || '-'}</td>
                    <td>${log.ip_address || '-'}</td>
                    <td>${log.created_at}</td>
                </tr>
            `).join('');
        }
        
        // 渲染 OPS 日志分页
        function renderOpsLogsPagination(total, page, pageSize) {
            const container = document.getElementById('ops-logs-pagination');
            const totalPages = Math.ceil(total / pageSize);
            
            container.innerHTML = `
                <div style="display: flex; justify-content: space-between; align-items: center; width: 100%; padding: 10px; background: #f8f9fa; border-radius: 4px;">
                    <div class="pagination-info">
                        共 ${total} 条记录，第 ${page}/${totalPages || 1} 页
                    </div>
                    <div class="pagination-buttons" style="display: flex; gap: 5px; align-items: center;">
                        <button class="btn btn-sm" ${page <= 1 ? 'disabled' : ''} onclick="opsLogsGoToPage(1)">首页</button>
                        <button class="btn btn-sm" ${page <= 1 ? 'disabled' : ''} onclick="opsLogsGoToPage(${page - 1})">上一页</button>
                        <span style="margin: 0 10px;">
                            跳转到 <input type="number" id="opsLogsPageInput" min="1" max="${totalPages}" value="${page}" style="width: 50px; text-align: center;"> 页
                            <button class="btn btn-sm" onclick="opsLogsGoToPage(parseInt(document.getElementById('opsLogsPageInput').value))">Go</button>
                        </span>
                        <button class="btn btn-sm" ${page >= totalPages ? 'disabled' : ''} onclick="opsLogsGoToPage(${page + 1})">下一页</button>
                        <button class="btn btn-sm" ${page >= totalPages ? 'disabled' : ''} onclick="opsLogsGoToPage(${totalPages})">末页</button>
                        <select onchange="changeOpsLogsPageSize(this.value)" style="margin-left: 10px;">
                            <option value="20" ${pageSize === 20 ? 'selected' : ''}>20条/页</option>
                            <option value="50" ${pageSize === 50 ? 'selected' : ''}>50条/页</option>
                            <option value="100" ${pageSize === 100 ? 'selected' : ''}>100条/页</option>
                        </select>
                    </div>
                </div>
            `;
        }
        
        // OPS 日志跳转页面
        window.opsLogsGoToPage = function(page) {
            opsLogsPage = page;
            refreshOpsLogs();
        };
        
        // 改变 OPS 日志每页数量
        window.changeOpsLogsPageSize = function(newSize) {
            opsLogsPageSize = parseInt(newSize);
            opsLogsPage = 1; // 重置到第一页
            refreshOpsLogs();
        };
        
        // 格式化字节
        function formatBytes(bytes) {
            if (bytes === 0) return '0 B';
            const k = 1024;
            const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
            const i = Math.floor(Math.log(bytes) / Math.log(k));
            return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
        }
        
        // Tab 切换时加载数据
        document.addEventListener('DOMContentLoaded', function() {
            const tabs = document.querySelectorAll('.tab');
            tabs.forEach(tab => {
                tab.addEventListener('click', function() {
                    const tabId = this.dataset.tab;
                    if (tabId === 'ops-users') {
                        refreshOpsUsers();
                    } else if (tabId === 'ops-logs') {
                        refreshOpsLogs();
                    } else if (tabId === 'web-search') {
                        refreshWebSearchKeys();
                    } else if (tabId === 'amap') {
                        refreshAmapKeys();
                    }
                });
            });
        });
        
        // ==================== Web Search Keys 管理 ====================
        
        let webSearchKeysData = [];
        
        async function loadWebSearchConfig() {
            try {
                const response = await fetch('/portal/api/web-search-config');
                if (!response.ok) throw new Error('Failed to fetch web search config');
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                
                if (data.success && data.config) {
                    const proxyInput = document.getElementById('ws-global-proxy');
                    if (proxyInput) {
                        proxyInput.value = data.config.proxy || '';
                    }
                    const freeUserCheckbox = document.getElementById('ws-allow-free-user');
                    if (freeUserCheckbox) {
                        freeUserCheckbox.checked = !!data.config.allow_free_user_web_search;
                    }
                }
            } catch (error) {
                console.error('Error fetching web search config:', error);
            }
        }
        
        async function saveWebSearchConfig() {
            const proxy = document.getElementById('ws-global-proxy')?.value.trim() || '';
            const allowFreeUser = document.getElementById('ws-allow-free-user')?.checked || false;
            
            try {
                const response = await fetch('/portal/api/web-search-config', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        proxy: proxy,
                        allow_free_user_web_search: allowFreeUser
                    })
                });
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                
                if (data.success) {
                    showToast('Web Search 配置已保存', 'success');
                } else {
                    showToast(data.error || data.message || '保存失败', 'error');
                }
            } catch (error) {
                showToast('保存失败: ' + error.message, 'error');
            }
        }
        
        async function refreshWebSearchKeys() {
            // Also load global config when refreshing
            loadWebSearchConfig();
            
            try {
                const response = await fetch('/portal/api/web-search-keys');
                if (!response.ok) throw new Error('Failed to fetch web search keys');
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                
                webSearchKeysData = data.keys || [];
                renderWebSearchKeysTable();
            } catch (error) {
                console.error('Error fetching web search keys:', error);
                const braveTbody = document.getElementById('ws-brave-tbody');
                const tavilyTbody = document.getElementById('ws-tavily-tbody');
                const chatglmTbody = document.getElementById('ws-chatglm-tbody');
                const bochaTbody = document.getElementById('ws-bocha-tbody');
                const unifuncsTbody = document.getElementById('ws-unifuncs-tbody');
                if (braveTbody) braveTbody.innerHTML = '<tr><td colspan="10" style="text-align: center; color: #e74c3c; padding: 20px;">加载失败</td></tr>';
                if (tavilyTbody) tavilyTbody.innerHTML = '<tr><td colspan="10" style="text-align: center; color: #e74c3c; padding: 20px;">加载失败</td></tr>';
                if (chatglmTbody) chatglmTbody.innerHTML = '<tr><td colspan="10" style="text-align: center; color: #e74c3c; padding: 20px;">加载失败</td></tr>';
                if (bochaTbody) bochaTbody.innerHTML = '<tr><td colspan="10" style="text-align: center; color: #e74c3c; padding: 20px;">加载失败</td></tr>';
                if (unifuncsTbody) unifuncsTbody.innerHTML = '<tr><td colspan="10" style="text-align: center; color: #e74c3c; padding: 20px;">加载失败</td></tr>';
            }
        }
        
        function renderWebSearchKeysTable() {
            const filter = document.getElementById('ws-type-filter')?.value || '';
            const braveKeys = webSearchKeysData.filter(k => k.searcher_type === 'brave');
            const tavilyKeys = webSearchKeysData.filter(k => k.searcher_type === 'tavily');
            const chatglmKeys = webSearchKeysData.filter(k => k.searcher_type === 'chatglm');
            const bochaKeys = webSearchKeysData.filter(k => k.searcher_type === 'bocha');
            const unifuncsKeys = webSearchKeysData.filter(k => k.searcher_type === 'unifuncs');
            
            const showBrave = !filter || filter === 'brave';
            const showTavily = !filter || filter === 'tavily';
            const showChatglm = !filter || filter === 'chatglm';
            const showBocha = !filter || filter === 'bocha';
            const showUnifuncs = !filter || filter === 'unifuncs';
            
            renderWebSearchTypeTable('ws-brave-tbody', showBrave ? braveKeys : []);
            renderWebSearchTypeTable('ws-tavily-tbody', showTavily ? tavilyKeys : []);
            renderWebSearchTypeTable('ws-chatglm-tbody', showChatglm ? chatglmKeys : []);
            renderWebSearchTypeTable('ws-bocha-tbody', showBocha ? bochaKeys : []);
            renderWebSearchTypeTable('ws-unifuncs-tbody', showUnifuncs ? unifuncsKeys : []);
            
            // Show/hide table sections based on filter
            const braveSection = document.getElementById('ws-brave-table');
            const tavilySection = document.getElementById('ws-tavily-table');
            const chatglmSection = document.getElementById('ws-chatglm-table');
            const bochaSection = document.getElementById('ws-bocha-table');
            const unifuncsSection = document.getElementById('ws-unifuncs-table');
            if (braveSection) braveSection.closest('.table-container').style.display = showBrave ? '' : 'none';
            if (tavilySection) tavilySection.closest('.table-container').style.display = showTavily ? '' : 'none';
            if (chatglmSection) chatglmSection.closest('.table-container').style.display = showChatglm ? '' : 'none';
            if (bochaSection) bochaSection.closest('.table-container').style.display = showBocha ? '' : 'none';
            if (unifuncsSection) unifuncsSection.closest('.table-container').style.display = showUnifuncs ? '' : 'none';
            
            // Also show/hide the section headers
            if (braveSection) braveSection.closest('.table-container').previousElementSibling.style.display = showBrave ? '' : 'none';
            if (tavilySection) tavilySection.closest('.table-container').previousElementSibling.style.display = showTavily ? '' : 'none';
            if (chatglmSection) chatglmSection.closest('.table-container').previousElementSibling.style.display = showChatglm ? '' : 'none';
            if (bochaSection) bochaSection.closest('.table-container').previousElementSibling.style.display = showBocha ? '' : 'none';
            if (unifuncsSection) unifuncsSection.closest('.table-container').previousElementSibling.style.display = showUnifuncs ? '' : 'none';
        }
        
        function renderWebSearchTypeTable(tbodyId, keys) {
            const tbody = document.getElementById(tbodyId);
            if (!tbody) return;
            
            if (keys.length === 0) {
                tbody.innerHTML = '<tr><td colspan="10" style="text-align: center; color: #888; padding: 20px;">暂无数据</td></tr>';
                return;
            }
            
            tbody.innerHTML = keys.map(k => {
                const safeId = parseInt(k.id, 10) || 0;
                const safeApiKey = escapeHtml(k.api_key);
                const safeBaseUrl = escapeHtml(k.base_url);
                const safeLastUsedTime = escapeHtml(k.last_used_time);
                const safeSuccessCount = parseInt(k.success_count, 10) || 0;
                const safeFailureCount = parseInt(k.failure_count, 10) || 0;
                const safeTotalRequests = parseInt(k.total_requests, 10) || 0;
                const safeLastLatency = parseInt(k.last_latency, 10) || 0;
                return `
                <tr>
                    <td>${safeId}</td>
                    <td style="font-family: monospace; font-size: 12px;" title="${safeApiKey}">${safeApiKey}</td>
                    <td style="max-width: 150px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title="${safeBaseUrl || '默认'}">${safeBaseUrl || '<span style="color:#999">默认</span>'}</td>
                    <td>
                        <span style="background: ${k.active ? '#e8f5e9' : '#fce4ec'}; color: ${k.active ? '#2e7d32' : '#c62828'}; padding: 2px 8px; border-radius: 4px; font-size: 12px;">
                            ${k.active ? '启用' : '停用'}
                        </span>
                    </td>
                    <td>
                        <span style="background: ${k.is_healthy ? '#e8f5e9' : '#fff3e0'}; color: ${k.is_healthy ? '#2e7d32' : '#e65100'}; padding: 2px 8px; border-radius: 4px; font-size: 12px;">
                            ${k.is_healthy ? '健康' : '异常'}
                        </span>
                    </td>
                    <td><span style="color: #2e7d32">${safeSuccessCount}</span> / <span style="color: #c62828">${safeFailureCount}</span></td>
                    <td>${safeTotalRequests}</td>
                    <td>${safeLastLatency > 0 ? safeLastLatency + 'ms' : '-'}</td>
                    <td style="font-size: 12px;">${safeLastUsedTime && safeLastUsedTime !== '0001-01-01 00:00:00' ? safeLastUsedTime : '-'}</td>
                    <td>
                        <div style="display: flex; gap: 4px; flex-wrap: wrap;">
                            <button class="btn btn-sm" id="ws-test-btn-${safeId}" onclick="testWebSearchKey(${safeId})" style="background: #9c27b0; font-size: 11px; padding: 2px 6px;">测试</button>
                            ${k.active 
                                ? `<button class="btn btn-sm" onclick="toggleWebSearchKeyStatus(${safeId}, false)" style="background: #ff9800; font-size: 11px; padding: 2px 6px;">停用</button>`
                                : `<button class="btn btn-sm" onclick="toggleWebSearchKeyStatus(${safeId}, true)" style="background: #4caf50; font-size: 11px; padding: 2px 6px;">启用</button>`
                            }
                            ${!k.is_healthy 
                                ? `<button class="btn btn-sm" onclick="resetWebSearchKeyHealth(${safeId})" style="background: #2196f3; font-size: 11px; padding: 2px 6px;">重置健康</button>`
                                : ''
                            }
                            <button class="btn btn-sm" onclick="deleteWebSearchKey(${safeId})" style="background: #f44336; font-size: 11px; padding: 2px 6px;">删除</button>
                        </div>
                    </td>
                </tr>
                `;
            }).join('');
        }
        
        function showAddWebSearchKeyModal() {
            document.getElementById('addWebSearchKeyModal').style.display = 'flex';
            document.getElementById('ws-new-type').value = 'brave';
            document.getElementById('ws-new-api-keys').value = '';
            document.getElementById('ws-new-base-url').value = '';
            document.getElementById('ws-new-proxy').value = '';
        }
        
        function closeAddWebSearchKeyModal() {
            document.getElementById('addWebSearchKeyModal').style.display = 'none';
        }
        
        async function submitAddWebSearchKey() {
            const searcherType = document.getElementById('ws-new-type').value;
            const apiKeys = document.getElementById('ws-new-api-keys').value.trim();
            const baseUrl = document.getElementById('ws-new-base-url').value.trim();
            const proxy = document.getElementById('ws-new-proxy').value.trim();
            
            if (!apiKeys) {
                showToast('请输入至少一个 API Key', 'error');
                return;
            }
            
            try {
                const response = await fetch('/portal/api/web-search-keys', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        searcher_type: searcherType,
                        api_keys: apiKeys,
                        base_url: baseUrl,
                        proxy: proxy
                    })
                });
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                
                if (data.success) {
                    const msg = data.added > 1 
                        ? `批量添加成功，共添加 ${data.added} 个 Key` 
                        : 'Web Search API Key 添加成功';
                    showToast(msg, 'success');
                    closeAddWebSearchKeyModal();
                    refreshWebSearchKeys();
                } else {
                    showToast(data.error || data.message || '添加失败', 'error');
                }
            } catch (error) {
                showToast('添加失败: ' + error.message, 'error');
            }
        }
        
        async function toggleWebSearchKeyStatus(id, activate) {
            const action = activate ? 'activate' : 'deactivate';
            const url = `/portal/${action}-web-search-key/${id}`;
            try {
                const response = await fetch(url, { method: 'POST' });
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                
                if (data.success) {
                    showToast(`Web Search Key ${activate ? '启用' : '停用'}成功`, 'success');
                    refreshWebSearchKeys();
                } else {
                    showToast(data.message || '操作失败', 'error');
                }
            } catch (error) {
                showToast('操作失败: ' + error.message, 'error');
            }
        }
        
        async function resetWebSearchKeyHealth(id) {
            try {
                const response = await fetch(`/portal/reset-web-search-key-health/${id}`, { method: 'POST' });
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                
                if (data.success) {
                    showToast('健康状态已重置', 'success');
                    refreshWebSearchKeys();
                } else {
                    showToast(data.message || '操作失败', 'error');
                }
            } catch (error) {
                showToast('操作失败: ' + error.message, 'error');
            }
        }
        
        async function deleteWebSearchKey(id) {
            if (!confirm('确定要删除此 Web Search API Key 吗？')) return;
            
            try {
                const response = await fetch(`/portal/api/web-search-keys/${id}`, { method: 'DELETE' });
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                
                if (data.success) {
                    showToast('Web Search API Key 已删除', 'success');
                    refreshWebSearchKeys();
                } else {
                    showToast(data.message || '删除失败', 'error');
                }
            } catch (error) {
                showToast('删除失败: ' + error.message, 'error');
            }
        }
        
        async function testWebSearchKey(id) {
            const btn = document.getElementById(`ws-test-btn-${id}`);
            if (btn) {
                btn.disabled = true;
                btn.textContent = '测试中...';
                btn.style.background = '#757575';
            }
            
            try {
                const response = await fetch(`/portal/test-web-search-key/${id}`, { method: 'POST' });
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                
                if (data.success) {
                    showToast(`测试通过! 返回 ${data.result_count} 条结果, 延迟 ${data.latency_ms}ms`, 'success');
                    if (btn) { btn.style.background = '#4caf50'; btn.textContent = '通过'; }
                } else {
                    showToast(`测试失败: ${data.message}`, 'error');
                    if (btn) { btn.style.background = '#f44336'; btn.textContent = '失败'; }
                }
                // Refresh to update stats
                setTimeout(() => refreshWebSearchKeys(), 1500);
            } catch (error) {
                showToast('测试失败: ' + error.message, 'error');
                if (btn) { btn.style.background = '#f44336'; btn.textContent = '失败'; }
                setTimeout(() => refreshWebSearchKeys(), 1500);
            }
        }
        
        // ==================== 暴露删除和操作相关的全局函数 ====================
        // Provider 相关
        window.confirmDeleteSelected = confirmDeleteSelected;
        window.deleteProvider = deleteProvider;
        window.deleteMultipleProviders = deleteMultipleProviders;
        window.checkSingleProvider = checkSingleProvider;
        window.checkAllProvidersHealth = checkAllProvidersHealth;
        window.checkSelectedProvider = checkSelectedProvider;
        window.selectAllProviders = selectAllProviders;
        window.updateDeleteSelectedButton = updateDeleteSelectedButton;
        
        // API Key 相关
        window.confirmDeleteSelectedAPI = confirmDeleteSelectedAPI;
        window.confirmDeleteSelectedAPIKeys = confirmDeleteSelectedAPI; // 别名，兼容 HTML 中的调用
        window.deleteAPIKey = deleteAPIKey;
        window.deleteMultipleAPIKeys = deleteMultipleAPIKeys;
        window.toggleAPIKeyStatus = toggleAPIKeyStatus;
        window.confirmDisableSelectedAPI = confirmDisableSelectedAPI;
        window.confirmEnableSelectedAPI = confirmEnableSelectedAPI;
        window.disableMultipleAPIKeys = disableMultipleAPIKeys;
        window.enableMultipleAPIKeys = enableMultipleAPIKeys;
        window.toggleSelectAllAPI = toggleSelectAllAPI;
        window.selectAllAPIKeys = selectAllAPIKeys;
        window.updateDeleteSelectedAPIButton = updateDeleteSelectedAPIButton;
        
        // 流量限制相关
        window.showTrafficLimitDialog = showTrafficLimitDialog;
        window.showTrafficLimitModal = showTrafficLimitDialog; // 别名
        window.closeTrafficLimitModal = closeTrafficLimitModal;
        window.saveTrafficLimit = saveTrafficLimit;
        window.closeTrafficLimitDialog = closeTrafficLimitModal; // 别名
        window.resetApiKeyTraffic = resetApiKeyTraffic;
        
        // 内存和系统监控相关
        window.showMemoryDialog = showMemoryDialog;
        window.closeMemoryDialog = closeMemoryDialog;
        window.fetchMemoryStats = fetchMemoryStats;
        window.forceGC = forceGC;
        window.fetchGoroutineDump = fetchGoroutineDump;
        
        // 筛选相关
        window.filterProviders = filterProviders;
        window.filterApiKeys = filterApiKeys;
        
        // 其他操作函数
        window.showToast = showToast;
        window.hideContextMenu = hideContextMenu;
        window.copyToClipboard = copyToClipboard;
        window.generateNewApiKey = generateNewApiKey;
        window.confirmAndGenerateApiKey = confirmAndGenerateApiKey;
        window.showApiKeySuccessModal = showApiKeySuccessModal;
        window.closeApiKeySuccessModal = closeApiKeySuccessModal;
        window.copyGeneratedApiKey = copyGeneratedApiKey;
        
        // 模型相关
        window.openEditModelModal = openEditModelModal;
        window.showCurlCommand = showCurlCommand;
        if (typeof closeEditModelModal === 'function') window.closeEditModelModal = closeEditModelModal;
        if (typeof saveModelMetadata === 'function') window.saveModelMetadata = saveModelMetadata;
        if (typeof closeCurlModal === 'function') window.closeCurlModal = closeCurlModal;
        if (typeof copyCurlCommand === 'function') window.copyCurlCommand = copyCurlCommand;
        
        // 右键菜单相关（Provider）
        window.quickAddProvider = quickAddProvider;
        window.copySimilarProviderKeys = copySimilarProviderKeys;
        window.deleteSelectedProvider = deleteSelectedProvider;
        window.showContextMenu = showContextMenu;
        window.initializeContextMenu = initializeContextMenu;
        
        // 同类供应商 Keys 弹窗相关
        if (typeof showSimilarKeysModal === 'function') window.showSimilarKeysModal = showSimilarKeysModal;
        if (typeof closeCopySimilarKeysModal === 'function') window.closeCopySimilarKeysModal = closeCopySimilarKeysModal;
        if (typeof copySimilarKeysToClipboard === 'function') window.copySimilarKeysToClipboard = copySimilarKeysToClipboard;
        
        // 右键菜单相关（API Key）
        window.triggerEditAllowedModelsFromContextMenu = triggerEditAllowedModelsFromContextMenu;
        if (typeof showEditAllowedModelsModal === 'function') window.showEditAllowedModelsModal = showEditAllowedModelsModal;
        if (typeof closeEditAllowedModelsModal === 'function') window.closeEditAllowedModelsModal = closeEditAllowedModelsModal;
        if (typeof saveEditedAllowedModels === 'function') window.saveEditedAllowedModels = saveEditedAllowedModels;
        
        // Tab 切换
        if (typeof openTab === 'function') window.openTab = openTab;
        if (typeof switchTab === 'function') window.switchTab = switchTab;
        
        // 关闭 API Key 成功模态框
        if (typeof closeApiKeyModal === 'function') window.closeApiKeyModal = closeApiKeyModal;
        
        // OPS 用户管理相关
        window.showCreateOpsUserModal = showCreateOpsUserModal;
        window.closeCreateOpsUserModal = closeCreateOpsUserModal;
        window.createOpsUser = createOpsUser;
        window.closeOpsUserCredentialsModal = closeOpsUserCredentialsModal;
        window.copyOpsCredentials = copyOpsCredentials;
        window.refreshOpsUsers = refreshOpsUsers;
        window.deleteOpsUser = deleteOpsUser;
        window.toggleOpsUserStatus = toggleOpsUserStatus;
        window.resetOpsUserPassword = resetOpsUserPassword;
        window.resetOpsUserKey = resetOpsUserKey;
        
        // OPS 日志相关
        window.refreshOpsLogs = refreshOpsLogs;
        window.filterOpsLogs = filterOpsLogs;
        
        // Web Search Keys 相关
        window.refreshWebSearchKeys = refreshWebSearchKeys;
        window.showAddWebSearchKeyModal = showAddWebSearchKeyModal;
        window.closeAddWebSearchKeyModal = closeAddWebSearchKeyModal;
        window.submitAddWebSearchKey = submitAddWebSearchKey;
        window.toggleWebSearchKeyStatus = toggleWebSearchKeyStatus;
        window.resetWebSearchKeyHealth = resetWebSearchKeyHealth;
        window.deleteWebSearchKey = deleteWebSearchKey;
        window.testWebSearchKey = testWebSearchKey;
        window.saveWebSearchConfig = saveWebSearchConfig;
        window.loadWebSearchConfig = loadWebSearchConfig;

        // ========== Amap Key Management Functions ==========

        async function refreshAmapKeys() {
            try {
                const response = await fetch('/portal/api/amap-keys');
                if (!response.ok) throw new Error('Failed to fetch amap keys');
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                if (!data.success) {
                    showToast('加载高德密钥失败', 'error');
                    return;
                }
                renderAmapKeysTable(data.keys || []);
            } catch (error) {
                console.error('Error refreshing amap keys:', error);
                showToast('加载高德密钥失败', 'error');
            }
        }

        // escapeHtmlForAmap escapes HTML special characters to prevent XSS
        function escapeHtmlForAmap(str) {
            if (str === null || str === undefined) return '';
            var s = String(str);
            var div = document.createElement('div');
            div.appendChild(document.createTextNode(s));
            return div.innerHTML;
        }

        function renderAmapKeysTable(keys) {
            const tbody = document.getElementById('amap-keys-tbody');
            if (!tbody) return;
            // Clear existing content safely
            while (tbody.firstChild) {
                tbody.removeChild(tbody.firstChild);
            }
            if (keys.length === 0) {
                var emptyRow = document.createElement('tr');
                var emptyCell = document.createElement('td');
                emptyCell.setAttribute('colspan', '11');
                emptyCell.style.cssText = 'text-align: center; color: #888; padding: 20px;';
                emptyCell.textContent = '暂无高德地图 API 密钥';
                emptyRow.appendChild(emptyCell);
                tbody.appendChild(emptyRow);
                return;
            }
            keys.forEach(function(k) {
                var tr = document.createElement('tr');

                // ID cell (integer, safe)
                var tdId = document.createElement('td');
                tdId.textContent = k.id;
                tr.appendChild(tdId);

                // API Key cell (use textContent to prevent XSS)
                var tdKey = document.createElement('td');
                tdKey.style.cssText = 'font-family: monospace; font-size: 12px;';
                tdKey.textContent = k.api_key || '';
                tr.appendChild(tdKey);

                // Active status
                var tdActive = document.createElement('td');
                var spanActive = document.createElement('span');
                spanActive.className = k.active ? 'status-active' : 'status-inactive';
                spanActive.style.cssText = 'padding: 2px 8px; border-radius: 3px; font-size: 12px;';
                spanActive.textContent = k.active ? '启用' : '停用';
                tdActive.appendChild(spanActive);
                tr.appendChild(tdActive);

                // Health status
                var tdHealth = document.createElement('td');
                var spanHealth = document.createElement('span');
                spanHealth.className = k.is_healthy ? 'status-active' : 'status-inactive';
                spanHealth.style.cssText = 'padding: 2px 8px; border-radius: 3px; font-size: 12px;';
                spanHealth.textContent = k.is_healthy ? '健康' : '异常';
                tdHealth.appendChild(spanHealth);
                tr.appendChild(tdHealth);

                // Last check time
                var tdCheckTime = document.createElement('td');
                tdCheckTime.style.cssText = 'font-size: 12px;';
                tdCheckTime.textContent = k.health_check_time || '-';
                tr.appendChild(tdCheckTime);

                // Last check error (use textContent for safety)
                var tdError = document.createElement('td');
                tdError.style.cssText = 'font-size: 11px; max-width: 150px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;';
                tdError.title = k.last_check_error || '';
                tdError.textContent = k.last_check_error || '-';
                tr.appendChild(tdError);

                // Success/Fail count (integers, safe)
                var tdCounts = document.createElement('td');
                tdCounts.textContent = k.success_count + '/' + k.failure_count;
                tr.appendChild(tdCounts);

                // Total requests
                var tdTotal = document.createElement('td');
                tdTotal.textContent = k.total_requests;
                tr.appendChild(tdTotal);

                // Latency
                var tdLatency = document.createElement('td');
                tdLatency.textContent = k.last_latency;
                tr.appendChild(tdLatency);

                // Last used time
                var tdUsed = document.createElement('td');
                tdUsed.style.cssText = 'font-size: 12px;';
                tdUsed.textContent = k.last_used_time || '-';
                tr.appendChild(tdUsed);

                // Actions (buttons use integer ID only, safe from injection)
                var tdActions = document.createElement('td');
                var btnStyle = 'padding: 2px 8px; font-size: 11px; margin: 1px;';
                var keyId = parseInt(k.id, 10); // ensure integer

                var btnToggle = document.createElement('button');
                btnToggle.className = 'btn';
                btnToggle.style.cssText = btnStyle;
                btnToggle.textContent = k.active ? '停用' : '启用';
                btnToggle.addEventListener('click', function() { toggleAmapKey(keyId); });
                tdActions.appendChild(btnToggle);

                var btnTest = document.createElement('button');
                btnTest.className = 'btn';
                btnTest.style.cssText = btnStyle + ' background: #3498db; color: white;';
                btnTest.textContent = '测试';
                btnTest.addEventListener('click', function() { testAmapKey(keyId); });
                tdActions.appendChild(btnTest);

                var btnReset = document.createElement('button');
                btnReset.className = 'btn';
                btnReset.style.cssText = btnStyle + ' background: #f39c12; color: white;';
                btnReset.textContent = '重置';
                btnReset.addEventListener('click', function() { resetAmapKeyHealth(keyId); });
                tdActions.appendChild(btnReset);

                var btnDelete = document.createElement('button');
                btnDelete.className = 'btn';
                btnDelete.style.cssText = btnStyle + ' background: #e74c3c; color: white;';
                btnDelete.textContent = '删除';
                btnDelete.addEventListener('click', function() { deleteAmapKey(keyId); });
                tdActions.appendChild(btnDelete);

                tr.appendChild(tdActions);
                tbody.appendChild(tr);
            });
        }

        function showAddAmapKeyModal() {
            document.getElementById('addAmapKeyModal').style.display = 'flex';
            document.getElementById('amap-new-keys').value = '';
        }

        function closeAddAmapKeyModal() {
            document.getElementById('addAmapKeyModal').style.display = 'none';
        }

        async function submitAddAmapKey() {
            const keysText = document.getElementById('amap-new-keys').value.trim();
            if (!keysText) {
                showToast('请输入至少一个 API 密钥', 'error');
                return;
            }
            try {
                const response = await fetch('/portal/api/amap-keys', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ api_keys: keysText })
                });
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                if (data.success) {
                    const msg = '成功添加 ' + data.added_count + '/' + data.total + ' 个密钥';
                    showToast(msg, 'success');
                    closeAddAmapKeyModal();
                    refreshAmapKeys();
                } else {
                    showToast(data.error || data.message || '添加密钥失败', 'error');
                }
            } catch (error) {
                console.error('Error adding amap keys:', error);
                showToast('添加密钥失败', 'error');
            }
        }

        async function toggleAmapKey(id) {
            try {
                const response = await fetch('/portal/toggle-amap-key/keys/' + id, {
                    method: 'POST'
                });
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                if (data.success) {
                    showToast('密钥状态已更新', 'success');
                    refreshAmapKeys();
                } else {
                    showToast(data.message || '切换状态失败', 'error');
                }
            } catch (error) {
                console.error('Error toggling amap key:', error);
                showToast('切换状态失败', 'error');
            }
        }

        async function testAmapKey(id) {
            showToast('正在测试密钥 #' + id + '...', 'info');
            try {
                const response = await fetch('/portal/test-amap-key/keys/' + id, {
                    method: 'POST'
                });
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                if (data.success) {
                    showToast('测试通过! 延迟: ' + data.latency_ms + 'ms', 'success');
                } else {
                    showToast('测试失败: ' + (data.message || '未知错误'), 'error');
                }
                setTimeout(function() { refreshAmapKeys(); }, 500);
            } catch (error) {
                console.error('Error testing amap key:', error);
                showToast('测试密钥失败', 'error');
            }
        }

        async function resetAmapKeyHealth(id) {
            try {
                const response = await fetch('/portal/reset-amap-key-health/keys/' + id, {
                    method: 'POST'
                });
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                if (data.success) {
                    showToast('健康状态已重置', 'success');
                    refreshAmapKeys();
                } else {
                    showToast(data.message || '重置失败', 'error');
                }
            } catch (error) {
                console.error('Error resetting amap key health:', error);
                showToast('重置健康状态失败', 'error');
            }
        }

        async function deleteAmapKey(id) {
            if (!confirm('确定删除此高德 API 密钥?')) return;
            try {
                const response = await fetch('/portal/api/amap-keys/keys/' + id, {
                    method: 'DELETE'
                });
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                if (data.success) {
                    showToast('密钥已删除', 'success');
                    refreshAmapKeys();
                } else {
                    showToast(data.message || '删除失败', 'error');
                }
            } catch (error) {
                console.error('Error deleting amap key:', error);
                showToast('删除密钥失败', 'error');
            }
        }

        async function checkAllAmapKeys() {
            showToast('正在检查所有高德密钥...', 'info');
            try {
                const response = await fetch('/portal/api/amap-keys/check-all', {
                    method: 'POST'
                });
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                if (data.success) {
                    showToast('检查完成: ' + data.healthy_count + '/' + data.total + ' 个健康', 'success');
                    refreshAmapKeys();
                } else {
                    showToast(data.message || '检查失败', 'error');
                }
            } catch (error) {
                console.error('Error checking all amap keys:', error);
                showToast('检查密钥失败', 'error');
            }
        }

        async function loadAmapConfig() {
            try {
                const response = await fetch('/portal/api/amap-config');
                if (!response.ok) throw new Error('Failed to fetch amap config');
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                if (data.success) {
                    const checkbox = document.getElementById('amap-allow-free-user');
                    if (checkbox) {
                        checkbox.checked = data.allow_free_user_amap || false;
                    }
                }
            } catch (error) {
                console.error('Error loading amap config:', error);
            }
        }

        async function saveAmapConfig() {
            const allowFreeUser = document.getElementById('amap-allow-free-user').checked;
            try {
                const response = await fetch('/portal/api/amap-config', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ allow_free_user_amap: allowFreeUser })
                });
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                if (data.success) {
                    showToast('配置已保存', 'success');
                } else {
                    showToast(data.message || '保存配置失败', 'error');
                }
            } catch (error) {
                console.error('Error saving amap config:', error);
                showToast('保存配置失败', 'error');
            }
        }

        // Amap Key Management exports
        window.refreshAmapKeys = refreshAmapKeys;
        window.showAddAmapKeyModal = showAddAmapKeyModal;
        window.closeAddAmapKeyModal = closeAddAmapKeyModal;
        window.submitAddAmapKey = submitAddAmapKey;
        window.toggleAmapKey = toggleAmapKey;
        window.testAmapKey = testAmapKey;
        window.resetAmapKeyHealth = resetAmapKeyHealth;
        window.deleteAmapKey = deleteAmapKey;
        window.checkAllAmapKeys = checkAllAmapKeys;
        window.loadAmapConfig = loadAmapConfig;
        window.saveAmapConfig = saveAmapConfig;
