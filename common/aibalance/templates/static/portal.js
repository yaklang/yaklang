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
        // 当前 API Key 列表按用户名过滤值（空=不过滤）
        // 关键词: apiKeyUsernameFilterValue, API Key 列表 username 过滤状态
        let apiKeyUsernameFilterValue = '';
        
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

        // ==================== Session Auto-Refresh ====================
        // 后端 session 有效期为 30 分钟。只要 portal 页面开着，
        // 这里就每 10 分钟自动调一次 /portal/api/session/refresh，把
        // ExpiresAt 顺延 30 分钟，避免因长时间挂在页面上而被强制登出。
        // 设计上不复用 authFetch，因为它会在 401/403 时直接跳转登录，
        // 而我们希望续期失败时让上层业务请求自然触发跳转，这里只静默重试。
        // 关键词: session auto refresh keep alive 自动续期 token
        const SESSION_REFRESH_INTERVAL_MS = 10 * 60 * 1000;
        let __sessionRefreshTimer = null;

        async function refreshAdminSessionOnce() {
            try {
                const resp = await fetch('/portal/api/session/refresh', {
                    method: 'POST',
                    credentials: 'same-origin',
                });
                if (resp.status === 401 || resp.status === 403) {
                    handleAuthError();
                    return false;
                }
                if (!resp.ok) {
                    console.warn('session refresh non-ok status:', resp.status);
                    return false;
                }
                const data = await resp.json().catch(() => null);
                if (data && data.expires_at) {
                    console.debug('session refreshed, new expires_at:', data.expires_at);
                }
                return true;
            } catch (e) {
                console.warn('session refresh error:', e);
                return false;
            }
        }

        function startSessionAutoRefresh() {
            if (__sessionRefreshTimer) return;
            // 进入页面立刻续一次，覆盖 "session 还剩不多就刷新页面" 的场景，
            // 避免下一次定时还没到就过期了。
            refreshAdminSessionOnce();
            __sessionRefreshTimer = setInterval(refreshAdminSessionOnce, SESSION_REFRESH_INTERVAL_MS);
            // 暴露给手动调试。
            window.__sessionRefreshTimer = __sessionRefreshTimer;
        }

        window.refreshAdminSessionOnce = refreshAdminSessionOnce;
        window.startSessionAutoRefresh = startSessionAutoRefresh;
        
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
                var todayDauEl = document.getElementById('stat-today-dau');
                if (todayDauEl) {
                    todayDauEl.textContent = (data.today_dau || 0).toLocaleString();
                }
                if (typeof renderDiskCard === 'function') {
                    renderDiskCard(data.disk_info || {});
                }
                if (typeof renderStorageCard === 'function') {
                    renderStorageCard(data.storage_info || {});
                }
                if (typeof renderDauCacheTab === 'function') {
                    renderDauCacheTab(data);
                }
            },
            
            // 渲染供应商表格
            renderProviders: function(data) {
                const tbody = document.getElementById('provider-table-body');
                if (!tbody) return;
                
                tbody.innerHTML = '';
                
                if (!data.providers || data.providers.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="11" class="text-center">No providers found</td></tr>';
                    return;
                }
                
                data.providers.forEach(p => {
                    const row = document.createElement('tr');
                    row.dataset.id = p.id;
                    row.dataset.status = p.health_status_class;
                    row.dataset.activeCacheControl = p.active_cache_control ? '1' : '0';
                    
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
                    
                    // Active Cache Control 徽章: 打开时显示绿色 CC, 关闭时留空
                    // 关键词: renderProviders active_cache_control 徽章列, 主动 cache_control 注入开关可视化
                    const ccBadge = p.active_cache_control
                        ? '<span class="health-badge healthy" title="Active Cache Control 已开启: 自动给最末 system 注入 cache_control:ephemeral">CC</span>'
                        : '<span class="health-badge" style="color:#999;background:transparent;border:1px dashed #ccc;" title="Active Cache Control 关闭: 走 tongyi+白名单 legacy 路径或 strip">-</span>';
                    
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
                                <button class="btn btn-sm btn-copy" onclick="copyToClipboard('${escapeJsInHtmlAttr(p.api_key)}')" title="复制 API Key">
                                    <svg viewBox="0 0 24 24" width="14" height="14">
                                        <path fill="currentColor" d="M16 1H4c-1.1 0-2 .9-2 2v14h2V3h12V1zm3 4H8c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h11c1.1 0 2-.9 2-2V7c0-1.1-.9-2-2-2zm0 16H8V7h11v14z"/>
                                    </svg>
                                </button>
                            </div>
                        </td>
                        <td>${p.total_requests}</td>
                        <td class="text-center">${ccBadge}</td>
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
                    row.dataset.tokenLimit = key.token_limit || 0;
                    row.dataset.tokenUsed = key.token_used || 0;
                    row.dataset.tokenEnabled = !!key.token_limit_enable;
                    
                    let statusBadge = key.active 
                        ? '<span class="health-badge healthy" style="font-size:12px;">激活</span>'
                        : '<span class="health-badge unhealthy" style="font-size:12px;">禁用</span>';
                    
                    // 字节流量限额列已停用：统一改用 Token 维度计费/限额
                    // 关键词: API Key Token 用量列 + Token 限额列渲染, 字节流量限额停用
                    const tokenUsedRaw = Number(key.token_used) || 0;
                    const tokenLimitRaw = Number(key.token_limit) || 0;
                    const tokenUsedDisplay = formatTokenCount(tokenUsedRaw);
                    let tokenLimitCell = '';
                    if (key.token_limit_enable && tokenLimitRaw > 0) {
                        const tPercent = (tokenUsedRaw / tokenLimitRaw) * 100;
                        let tBarColor = '#4caf50';
                        if (tPercent > 90) tBarColor = '#f44336';
                        else if (tPercent > 70) tBarColor = '#ff9800';
                        tokenLimitCell = `
                            <div class="traffic-limit-info" title="Token 用量: ${tokenUsedRaw}/${tokenLimitRaw} (${tPercent.toFixed(1)}%)">
                                <div class="traffic-progress" style="width: 80px; height: 8px; background: #e0e0e0; border-radius: 4px; overflow: hidden;">
                                    <div style="width: ${Math.min(tPercent, 100)}%; height: 100%; background: ${tBarColor};"></div>
                                </div>
                                <small>${tokenUsedDisplay}/${formatTokenCount(tokenLimitRaw)}</small>
                            </div>
                        `;
                    } else {
                        tokenLimitCell = '<span style="color: #999;">未限制</span>';
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
                        <td class="copyable editable-allowed-models" data-api-id="${key.id}" data-current-models="${this.escapeHtml(key.allowed_models)}" data-full-text="${this.escapeHtml(key.allowed_models)}" title="右键点击修改允许的模型">${renderAllowedModelsCellContent(key.allowed_models)}</td>
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
                        <td class="text-center" title="Token 计费用量">${tokenUsedDisplay}</td>
                        <td class="text-center">${tokenLimitCell}</td>
                        <td class="text-center">${this.escapeHtml(creatorName)}</td>
                        <td>${key.last_used_at || '-'}</td>
                        <td class="text-center">
                            <div style="display: flex; gap: 2px; justify-content: center; flex-wrap: wrap;">
                                ${actionButtons}
                                <button class="btn btn-sm" onclick="showTokenLimitDialog(${key.id}, ${tokenLimitRaw}, ${tokenUsedRaw}, ${!!key.token_limit_enable})" title="Token 限额设置" style="padding:2px 4px;font-size:11px;background:#1976d2;color:#fff;">Token★</button>
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
            
            // 渲染对外模型(Wrapper)信息表格：仅描述/标签，不含传统字节倍数与 Token 计费倍率
            // 关键词: renderModels wrapper 元数据, 仅描述标签, 传统倍数列已移除
            renderModels: function(data) {
                const tbody = document.getElementById('models-table-body');
                if (!tbody) return;

                tbody.innerHTML = '';

                if (!data.model_metas || data.model_metas.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="5" class="text-center">No models found</td></tr>';
                    return;
                }

                const self = this;

                data.model_metas.forEach(model => {
                    const row = document.createElement('tr');
                    row.dataset.modelName = model.name;

                    row.innerHTML =
                        '<td class="copyable" data-full-text="' + self.escapeHtml(model.name) + '">' + self.escapeHtml(model.name) + '</td>' +
                        '<td class="text-center">' + model.provider_count + '</td>' +
                        '<td class="copyable" data-full-text="' + self.escapeHtml(model.description || '') + '">' + (model.description || '-') + '</td>' +
                        '<td class="copyable" data-full-text="' + self.escapeHtml(model.tags || '') + '">' + (model.tags || '-') + '</td>' +
                        '<td class="text-center">' +
                        '<button class="btn btn-sm" onclick="openEditModelModal(\'' + escapeJsInHtmlAttr(model.name) + '\', \'' + escapeJsInHtmlAttr(model.description || '') + '\', \'' + escapeJsInHtmlAttr(model.tags || '') + '\')" title="编辑描述/标签">编辑</button>' +
                        '<button class="btn btn-sm" style="background-color: #4caf50; margin-left: 5px;" onclick="showCurlCommand(\'' + escapeJsInHtmlAttr(model.name) + '\')" title="查看 curl 命令">curl</button>' +
                        '</td>';

                    tbody.appendChild(row);
                });
            },

            // 渲染实际模型(内部转发名)计费倍率表：计费的真正主体
            // 关键词: renderActualModels, 实际模型计费倍率, 生效倍率, 勾选批量
            renderActualModels: function(data) {
                const tbody = document.getElementById('actual-models-table-body');
                if (!tbody) return;

                tbody.innerHTML = '';
                const selectAll = document.getElementById('actual-models-select-all');
                if (selectAll) selectAll.checked = false;

                const models = data.actual_models || [];
                if (models.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="7" class="text-center">No actual models found</td></tr>';
                    return;
                }

                const fmtEff = (v) => (typeof v === 'number') ? v.toFixed(2) : '-';
                const self = this;

                models.forEach(m => {
                    const row = document.createElement('tr');
                    const iEsc = self.escapeHtml(m.internal_model_name);
                    // iJs 专供 onclick 内联字符串实参使用（实际模型名可能来自上游模型列表，
                    // 含引号会闭合处理器导致 XSS），HTML 属性/正文展示仍用 iEsc。
                    const iJs = escapeJsInHtmlAttr(m.internal_model_name);
                    const wrappers = (m.wrappers || []).join(', ');

                    const statusBadge = m.has_multiplier
                        ? '<span style="background:#fff3e0; color:#ef6c00; padding:1px 6px; border-radius:8px;">已设</span>'
                        : '<span style="background:#eee; color:#888; padding:1px 6px; border-radius:8px;">继承默认</span>';

                    // 免费模型：倍率失效，加权计费 Token 恒为 0，单独用绿色徽标提示
                    const effBadges = m.is_free
                        ? '<span style="background:#e8f5e9; color:#2e7d32; padding:1px 8px; border-radius:8px; font-weight:600;">免费 (不计费)</span>'
                        : ('<span style="background:#e3f2fd; color:#1565c0; padding:1px 6px; border-radius:8px;">入 ' + fmtEff(m.effective_input) + '</span> ' +
                           '<span style="background:#e8f5e9; color:#2e7d32; padding:1px 6px; border-radius:8px;">出 ' + fmtEff(m.effective_output) + '</span> ' +
                           '<span style="background:#fff3e0; color:#ef6c00; padding:1px 6px; border-radius:8px;">建 ' + fmtEff(m.effective_cache_create) + '</span> ' +
                           '<span style="background:#fce4ec; color:#c2185b; padding:1px 6px; border-radius:8px;">命 ' + fmtEff(m.effective_cache_hit) + '</span>');

                    const editArgs = "'" + iJs + "'," +
                        (m.config_input || 0) + ',' + (m.config_output || 0) + ',' +
                        (m.config_cache_create || 0) + ',' + (m.config_cache_hit || 0) + ',' +
                        (m.is_free ? 'true' : 'false');
                    const clearBtn = m.has_multiplier
                        ? '<button class="btn btn-sm" style="background-color:#f44336; color:#fff; margin-left:5px;" onclick="clearModelMultiplierDirect(\'' + iJs + '\')" title="清除该实际模型倍率，回落全局默认">清除</button>'
                        : '';

                    row.innerHTML =
                        '<td class="text-center"><input type="checkbox" class="actual-model-check" value="' + iEsc + '"></td>' +
                        '<td class="copyable" data-full-text="' + iEsc + '" style="font-family:monospace;">' + iEsc + '</td>' +
                        '<td class="copyable" data-full-text="' + self.escapeHtml(wrappers) + '">' + (self.escapeHtml(wrappers) || '-') + '</td>' +
                        '<td class="text-center">' + (m.provider_count || 0) + '</td>' +
                        '<td class="text-center">' + statusBadge + '</td>' +
                        '<td class="text-center" style="font-family:monospace;">' + effBadges + '</td>' +
                        '<td class="text-center">' +
                        '<button class="btn btn-sm" onclick="openModelMultiplierModal(' + editArgs + ')" title="编辑该实际模型计费倍率">编辑</button>' +
                        clearBtn +
                        '</td>';

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
                    this.renderActualModels(data);
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
            // 启动 session 自动续期：只要页面开着就每 10 分钟续一次。
            // 关键词: session keep alive, 自动续期定时器启动
            startSessionAutoRefresh();
        });
        
        // 全局刷新函数
        window.refreshPortalData = function() {
            PortalDataLoader.refresh();
        };

        // ==================== API Keys Pagination ====================
        
        // Load API keys with pagination
        async function loadAPIKeysPaginated(page = 1, pageSize = 20) {
            try {
                let url = `/portal/api/api-keys?page=${page}&pageSize=${pageSize}&sortBy=created_at&sortOrder=desc`;
                if (apiKeyUsernameFilterValue) {
                    url += `&username=${encodeURIComponent(apiKeyUsernameFilterValue)}`;
                }
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

        // 按用户名过滤 API Key 列表（精确匹配，用户名可重复）
        // 关键词: applyApiKeyUsernameFilter, clearApiKeyUsernameFilter
        function applyApiKeyUsernameFilter() {
            const el = document.getElementById('apiKeyUsernameFilter');
            apiKeyUsernameFilterValue = el ? el.value.trim() : '';
            loadAPIKeysPaginated(1, apiKeysPageSize);
        }
        function clearApiKeyUsernameFilter() {
            const el = document.getElementById('apiKeyUsernameFilter');
            if (el) el.value = '';
            apiKeyUsernameFilterValue = '';
            loadAPIKeysPaginated(1, apiKeysPageSize);
        }

        // ==================== API Key 绑定用户信息编辑 ====================
        // 关键词: openApiKeyMetaModal saveApiKeyMeta closeApiKeyMetaModal, Username Remark MetaInfo
        // 通过 id 从 apiKeysData 查找当前行，避免把含引号的文本内联进 onclick 属性导致 HTML 破坏。
        function openApiKeyMetaModal(apiKeyId) {
            const row = (apiKeysData || []).find(k => String(k.id) === String(apiKeyId)) || {};
            const username = row.username || '';
            const remark = row.remark || '';
            const metainfo = row.metainfo || '';
            const existing = document.getElementById('apiKeyMetaModal');
            if (existing) existing.remove();
            const html = `
                <div id="apiKeyMetaModal" class="delete-confirmation-modal" style="display: flex;">
                    <div class="modal-content" style="width: 520px; max-width: 92vw;">
                        <span class="close-modal" onclick="closeApiKeyMetaModal()">&times;</span>
                        <h4>编辑绑定用户信息 <small style="color:#6a1b9a;font-weight:normal;">（API Key ID: ${apiKeyId}）</small></h4>
                        <div class="form-group">
                            <label for="metaUsernameInput">用户名（可重复）:</label>
                            <input type="text" id="metaUsernameInput" class="form-control" value="${escapeHtml(username || '')}" placeholder="用于按用户聚合，可重复">
                        </div>
                        <div class="form-group">
                            <label for="metaRemarkInput">备注:</label>
                            <input type="text" id="metaRemarkInput" class="form-control" value="${escapeHtml(remark || '')}" placeholder="自由文本备注">
                        </div>
                        <div class="form-group">
                            <label for="metaMetaInfoInput">metainfo（JSON 文本，OAuth 等外部系统绑定信息）:</label>
                            <textarea id="metaMetaInfoInput" class="form-control" rows="4" style="font-family: monospace; font-size: 12px;" placeholder='{"oauth_provider":"...","sub":"..."}'>${escapeHtml(metainfo || '')}</textarea>
                        </div>
                        <div class="modal-actions">
                            <span style="flex:1;"></span>
                            <button class="btn" onclick="closeApiKeyMetaModal()">取消</button>
                            <button class="btn btn-primary" onclick="saveApiKeyMeta(${apiKeyId})">保存</button>
                        </div>
                    </div>
                </div>
            `;
            document.body.insertAdjacentHTML('beforeend', html);
        }
        function closeApiKeyMetaModal() {
            const modal = document.getElementById('apiKeyMetaModal');
            if (modal) modal.remove();
        }
        async function saveApiKeyMeta(apiKeyId) {
            const username = (document.getElementById('metaUsernameInput') || {}).value || '';
            const remark = (document.getElementById('metaRemarkInput') || {}).value || '';
            const metainfo = (document.getElementById('metaMetaInfoInput') || {}).value || '';
            try {
                const response = await fetch(`/portal/api-key-meta/${apiKeyId}`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ username: username.trim(), remark: remark, metainfo: metainfo })
                });
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                if (!response.ok || !data.success) {
                    throw new Error(data.message || '保存绑定信息失败');
                }
                showToast('绑定信息已保存', 'success');
                closeApiKeyMetaModal();
                loadAPIKeysPaginated(apiKeysPage, apiKeysPageSize);
            } catch (error) {
                showToast('保存绑定信息失败: ' + error.message, 'error');
                console.error('Error saving api key meta:', error);
            }
        }

        // Render API keys table with paginated data
        function renderAPIKeysTablePaginated(keys) {
            const tbody = document.getElementById('api-table-body');
            if (!tbody) return;
            
            tbody.innerHTML = '';
            
            if (!keys || keys.length === 0) {
                // 关键词: API Key 列表 colspan 修复, 移除字节流量列后 15 -> 14
                tbody.innerHTML = '<tr><td colspan="14" class="text-center">No API keys found</td></tr>';
                return;
            }
            
            keys.forEach(key => {
                const row = document.createElement('tr');
                row.dataset.apiId = key.id;
                row.dataset.apiStatus = key.active ? 'active' : 'inactive';
                row.dataset.tokenLimit = key.token_limit || 0;
                row.dataset.tokenUsed = key.token_used || 0;
                row.dataset.tokenEnabled = !!key.token_limit_enable;
                
                let statusBadge = key.active 
                    ? '<span class="health-badge healthy" style="font-size:12px;">激活</span>'
                    : '<span class="health-badge unhealthy" style="font-size:12px;">禁用</span>';
                
                // 字节流量列已彻底移除：统一改用 Token 维度计费/限额，不再展示字节收发量
                // 关键词: paginated 渲染 Token 用量列, Token 限额列, 字节流量列已移除
                const tokenUsedRaw = Number(key.token_used) || 0;
                const tokenLimitRaw = Number(key.token_limit) || 0;
                const tokenUsedDisplay = formatTokenCount(tokenUsedRaw);
                let tokenLimitCell = '';
                if (key.token_limit_enable && tokenLimitRaw > 0) {
                    const tPercent = (tokenUsedRaw / tokenLimitRaw) * 100;
                    let tBarColor = '#4caf50';
                    if (tPercent > 90) tBarColor = '#f44336';
                    else if (tPercent > 70) tBarColor = '#ff9800';
                    tokenLimitCell = `
                        <div class="traffic-limit-info" title="Token 用量: ${tokenUsedRaw}/${tokenLimitRaw} (${tPercent.toFixed(1)}%)">
                            <div class="traffic-progress" style="width: 80px; height: 8px; background: #e0e0e0; border-radius: 4px; overflow: hidden;">
                                <div style="width: ${Math.min(tPercent, 100)}%; height: 100%; background: ${tBarColor};"></div>
                            </div>
                            <small>${tokenUsedDisplay}/${formatTokenCount(tokenLimitRaw)}</small>
                        </div>
                    `;
                } else {
                    tokenLimitCell = '<span style="color: #999;">未限制</span>';
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
                    <td class="text-center" title="${escapeHtml(key.remark || '')}">
                        <span>${key.username ? escapeHtml(key.username) : '<span style=\'color:#bbb;\'>-</span>'}</span>
                        ${key.remark ? `<small style="display:block;color:#888;max-width:140px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">${escapeHtml(key.remark)}</small>` : ''}
                    </td>
                    <td class="copyable editable-allowed-models" data-api-id="${key.id}" data-current-models="${escapeHtml(key.allowed_models)}" data-full-text="${escapeHtml(key.allowed_models)}" title="右键点击修改允许的模型">${renderAllowedModelsCellContent(key.allowed_models)}</td>
                    <td class="text-center">${key.usage_count || 0}</td>
                    <td class="text-center">${key.web_search_count || 0}</td>
                    <td class="text-center">
                        <span class="health-badge healthy">${key.success_count || 0}</span>
                        <span class="health-badge unhealthy">${key.failure_count || 0}</span>
                    </td>
                    <td class="text-center" title="Token 计费用量">${tokenUsedDisplay}</td>
                    <td class="text-center">${tokenLimitCell}</td>
                    <td class="text-center">${escapeHtml(creatorName)}</td>
                    <td>${key.last_used_time || key.created_at || '-'}</td>
                    <td class="text-center">
                        <div style="display: flex; gap: 2px; justify-content: center; flex-wrap: wrap;">
                            ${actionButtons}
                            <button class="btn btn-sm" onclick="showTokenLimitDialog(${key.id}, ${tokenLimitRaw}, ${tokenUsedRaw}, ${!!key.token_limit_enable})" title="Token 限额设置" style="padding:2px 4px;font-size:11px;background:#1976d2;color:#fff;">Token★</button>
                            <button class="btn btn-sm" onclick="openApiKeyMetaModal(${key.id})" title="编辑绑定用户信息（用户名/备注/metainfo）" style="padding:2px 4px;font-size:11px;background:#6a1b9a;color:#fff;">绑定</button>
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

        // Helper: format raw Token count to human readable (K/M/B/T tokens).
        // 关键词: formatTokenCount, Token 数千分位/M 单位展示, 与 portal Token 列对齐
        function formatTokenCount(n) {
            const num = Number(n) || 0;
            if (num === 0) return '0';
            const abs = Math.abs(num);
            if (abs < 1000) return String(num);
            if (abs < 1_000_000) return (num / 1000).toFixed(num % 1000 === 0 ? 0 : 1) + 'K';
            if (abs < 1_000_000_000) return (num / 1_000_000).toFixed(num % 1_000_000 === 0 ? 0 : 2) + 'M';
            return (num / 1_000_000_000).toFixed(2) + 'B';
        }

        // 计费 Token 与 RMB 换算：1 RMB = 10M 计费 Token。
        // updateRMBHint 读取某个「M Token」数值输入框，实时把换算后的 RMB 写入提示元素。
        // 关键词: updateRMBHint, 1 RMB=10M 计费 Token, 换算文案
        var BILLING_TOKEN_M_PER_RMB = 10; // 10M 计费 Token / RMB
        function formatRMBFromTokenM(mTokens) {
            const m = Number(mTokens) || 0;
            if (m <= 0) return '不限制 / 不计费';
            const rmb = m / BILLING_TOKEN_M_PER_RMB;
            const rmbStr = (rmb % 1 === 0) ? rmb.toFixed(0) : rmb.toFixed(2);
            return '约合 ' + rmbStr + ' RMB（1 RMB = 10M 计费 Token）';
        }
        function updateRMBHint(inputId, hintId) {
            const input = document.getElementById(inputId);
            const hint = document.getElementById(hintId);
            if (!input || !hint) return;
            const m = parseInt(input.value);
            hint.textContent = formatRMBFromTokenM(isNaN(m) ? 0 : m);
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

        // escapeJsInHtmlAttr 生成可安全放入 onclick="fn('<value>')" 这类双引号属性内
        // 单引号 JS 字符串实参的文本。注意：浏览器会先对属性值做 HTML 实体解码，再把结果
        // 当成 JS 源码解析，所以单独的 escapeHtml 不足以防御（&#39; 会被解码回 '，仍可闭合
        // 字符串注入代码）。这里必须两层转义：先做 JS 字符串字面量转义（防闭合单引号/插代码），
        // 再做 HTML 属性转义（防闭合双引号属性 / 保证解码后还原成预期 JS 源）。
        // 关键词: escapeJsInHtmlAttr, onclick 内联实参 XSS 防护, 双层转义
        function escapeJsInHtmlAttr(value) {
            var s = (value === null || value === undefined) ? '' : String(value);
            // 1) JS 单引号字符串字面量转义
            s = s.replace(/\\/g, '\\\\')
                 .replace(/'/g, "\\'")
                 .replace(/\r/g, '\\r')
                 .replace(/\n/g, '\\n')
                 .replace(/</g, '\\x3C')
                 .replace(/>/g, '\\x3E');
            // 2) HTML 双引号属性转义（& 与 " 必须编码；解码后还原为合法 JS 源）
            s = s.replace(/&/g, '&amp;').replace(/"/g, '&quot;');
            return s;
        }

        // Render compact allowed-models cell content (used by API key list).
        // Keeps the cell narrow even with dozens of allowed models, while
        // exposing the full list via tooltip + count badge. Right-click menu
        // (editable-allowed-models) and full text copy (data-full-text) on
        // the parent <td> still work because the tag/structure is preserved
        // by the caller.
        function renderAllowedModelsCellContent(allowedModelsString) {
            const raw = (allowedModelsString || '').toString();
            const items = raw.split(',').map(s => s.trim()).filter(s => s);
            if (items.length === 0) {
                return '<span class="allowed-models-empty" title="未配置任何允许模型">未授权</span>';
            }
            const previewCount = 2;
            const visible = items.slice(0, previewCount);
            const hidden = items.slice(previewCount);
            const badges = visible.map(name => {
                let cls = 'allowed-model-chip';
                if (name.includes('*')) cls += ' chip-glob';
                else if (name.endsWith('-free')) cls += ' chip-free';
                else cls += ' chip-paid';
                return '<span class="' + cls + '" title="' + escapeHtml(name) + '">' + escapeHtml(name) + '</span>';
            }).join('');
            const moreBadge = hidden.length > 0
                ? '<span class="allowed-model-chip chip-more" title="' + escapeHtml(hidden.join(', ')) + '">+' + hidden.length + '</span>'
                : '';
            const countBadge = '<span class="allowed-model-count" title="共 ' + items.length + ' 个允许模型，右键修改">' + items.length + '</span>';
            return '<div class="allowed-models-wrap">' + countBadge + badges + moreBadge + '</div>';
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
        // 关键词: switchTab, sidebar 菜单项激活, 兼容旧 .tab 顶部 tab
        function switchTab(tabId) {
            // 更新顶部隐藏的 .tab (向后兼容旧选择器), 以及新左侧 .menu-item
            document.querySelectorAll('.tab').forEach(tab => {
                tab.classList.remove('active');
                if (tab.getAttribute('data-tab') === tabId) {
                    tab.classList.add('active');
                }
            });
            document.querySelectorAll('.menu-item').forEach(mi => {
                mi.classList.remove('active');
                if (mi.getAttribute('data-tab') === tabId) {
                    mi.classList.add('active');
                }
            });

            // 更新内容显示
            document.querySelectorAll('.tab-content').forEach(content => {
                content.classList.remove('active');
            });
            const target = document.getElementById(tabId);
            if (target) {
                target.classList.add('active');
            }

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
            if (tabId === 'rate-limit') {
                loadRateLimitConfig();
                loadRateLimitStatus();
                startRateLimitModelStatsAutoRefresh();
                // 恢复上次选中的限流子 tab, 默认"频率与速率"
                let savedSub = 'rl-sub-rate';
                try { savedSub = localStorage.getItem('rateLimitSubTab') || 'rl-sub-rate'; } catch (e) {}
                if (!document.getElementById(savedSub)) savedSub = 'rl-sub-rate';
                switchRateLimitSubTab(savedSub);
            } else {
                stopRateLimitModelStatsAutoRefresh();
            }

            if (tabId === 'dau-cache') {
                if (typeof refreshDauCacheTab === 'function') {
                    refreshDauCacheTab();
                }
            }
            // Mirror tab 切换时刷新规则列表
            // 关键词: switchTab mirror tab 初始化, MirrorMgmt.refresh
            // 注意: 必须用 window.MirrorMgmt 访问, 不能用裸名 MirrorMgmt;
            // MirrorMgmt 是文件末尾 const 声明的, 在 TDZ 阶段裸名访问 (即使 typeof)
            // 会抛 ReferenceError: Cannot access 'MirrorMgmt' before initialization.
            if (tabId === 'mirror') {
                if (window.MirrorMgmt && typeof window.MirrorMgmt.refresh === 'function') {
                    window.MirrorMgmt.refresh();
                }
            }
            // 镜像数据 tab 切换时自动加载最近记录
            // 关键词: switchTab mirror-records 初始化, MirrorRecords.load
            if (tabId === 'mirror-records') {
                if (window.MirrorRecords && typeof window.MirrorRecords.load === 'function') {
                    window.MirrorRecords.load();
                }
            }
        }

        // 限流配置二级 tab 切换
        // 关键词: rate-limit 子 tab 切换, switchRateLimitSubTab, rl-subpane
        function switchRateLimitSubTab(paneId) {
            document.querySelectorAll('.rl-subtab').forEach(function (t) {
                t.classList.toggle('active', t.getAttribute('data-rlsub') === paneId);
            });
            document.querySelectorAll('.rl-subpane').forEach(function (p) {
                p.classList.toggle('active', p.id === paneId);
            });
            try { localStorage.setItem('rateLimitSubTab', paneId); } catch (e) {}
        }

        // 事件委托绑定限流子 tab 点击, 避免依赖 DOMContentLoaded 加载顺序
        // 关键词: rate-limit 子 tab 点击绑定, data-rlsub
        document.addEventListener('click', function (e) {
            const tab = e.target.closest ? e.target.closest('.rl-subtab') : null;
            if (!tab) return;
            const paneId = tab.getAttribute('data-rlsub');
            if (paneId) switchRateLimitSubTab(paneId);
        });

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
                                <label for="optionalAllowReason">思考配置</label>
                                <select id="optionalAllowReason" name="optionalAllowReason" class="form-control">
                                    <option value="" selected>默认 (跟随客户端请求)</option>
                                    <option value="true">启用思考</option>
                                    <option value="false">禁用思考</option>
                                </select>
                                <small class="form-text text-muted">控制模型是否使用深度思考模式</small>
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
                        <div class="form-group">
                            <div class="checkbox">
                                <label>
                                    <input type="checkbox" id="activeCacheControl" name="activeCacheControl"> 启用 Active Cache Control (主动给最末 system 注入 cache_control:ephemeral, 推荐 dashscope/anthropic 等支持 ephemeral 缓存的 provider 打开)
                                </label>
                            </div>
                            <small class="form-text text-muted">打开后, 客户端无 cache_control 时由 aibalance 自动给最末 system 消息注入 baseline ephemeral 标记; 客户端自带 cache_control 时 pass-through 不改写。Tongyi 白名单 model 即使关闭也保留旧行为。</small>
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
            domain_or_urls: [],
            domain_suggestions: {}
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
                autoCompleteData.domain_or_urls = data.domain_or_urls || [];
                autoCompleteData.domain_suggestions = data.domain_suggestions || {};
                console.log("Processed domain_or_urls:", autoCompleteData.domain_or_urls);

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
                    
                    // 优先使用后端返回的域名建议，硬编码作为 fallback
                    const fallbackDomainSuggestions = {
                        'openai': 'api.openai.com',
                        'siliconflow': 'api.siliconflow.cn',
                        'tongyi': 'dashscope.aliyuncs.com',
                        'moonshot': 'api.moonshot.cn',
                        'deepseek': 'api.deepseek.com',
                        'gemini': 'generativelanguage.googleapis.com',
                        'ollama': '127.0.0.1:11434',
                        'chatglm': 'open.bigmodel.cn',
                        'volcengine': 'ark.cn-beijing.volces.com',
                        'openrouter': 'openrouter.ai',
                        'comate': 'comate.baidu.com'
                    };
                    
                    if (autoCompleteData.domain_suggestions && autoCompleteData.domain_suggestions[selectedType] !== undefined) {
                        suggestedDomain = autoCompleteData.domain_suggestions[selectedType];
                    } else if (fallbackDomainSuggestions[selectedType] !== undefined) {
                        suggestedDomain = fallbackDomainSuggestions[selectedType];
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
            const activeCacheControlInput = document.getElementById('activeCacheControl');
            const activeCacheControl = activeCacheControlInput ? activeCacheControlInput.checked : false;
            const optionalAllowReason = document.getElementById('optionalAllowReason').value;
            
            // 日志输出表单数据（方便调试）
            console.log('Submitting data:', {
                wrapper_name: wrapperName,
                model_name: modelName,
                model_type: typeName,
                provider_mode: providerMode,
                domain_or_url: domainOrURL,
                api_keys: apiKeys,
                no_https: noHTTPS ? 'on' : '',
                active_cache_control: activeCacheControl ? 'on' : '',
                optional_allow_reason: optionalAllowReason
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
                if (activeCacheControl) {
                    params.append('active_cache_control', 'on');
                }
                if (optionalAllowReason) {
                    params.append('optional_allow_reason', optionalAllowReason);
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
            // 新左侧 sidebar menu-item 点击也走 switchTab
            // 关键词: sidebar menu-item 点击绑定, switchTab 入口
            document.querySelectorAll('.menu-item').forEach(mi => {
                const currentTabId = mi.getAttribute('data-tab');
                mi.addEventListener('click', function(e) {
                    e.preventDefault();
                    switchTab(currentTabId);
                });
                if (currentTabId === initialTabId) {
                    mi.classList.add('active');
                } else {
                    mi.classList.remove('active');
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
            } else if (initialTabId === 'rate-limit') {
                loadRateLimitConfig();
                loadRateLimitStatus();
                startRateLimitModelStatsAutoRefresh();
            } else if (initialTabId === 'mirror') {
                // mirror tab 在 page refresh 后保持的场景, 也需要主动拉数据
                // 关键词: DOMContentLoaded initial mirror auto refresh, window.MirrorMgmt
                // 必须用 window.MirrorMgmt, 不能 typeof MirrorMgmt (TDZ 抛错).
                if (window.MirrorMgmt && typeof window.MirrorMgmt.refresh === 'function') {
                    window.MirrorMgmt.refresh();
                }
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
            
            // 仅图标与关闭按钮是固定模板（无外部数据），可安全用 innerHTML；
            // message 可能来自外部可控数据（如客户端 IP/错误信息），必须经 textContent 写入，
            // 严禁拼接进 innerHTML，避免 XSS 打穿管理后台。
            // 关键词: showToast XSS 防护, message 走 textContent
            toast.innerHTML = `
                <div class="toast-icon">
                    <svg viewBox="0 0 24 24" width="24" height="24">
                        <path d="${iconPath}"></path>
                    </svg>
    </div>
                <div class="toast-content"></div>
                <div class="toast-close" onclick="this.parentElement.remove()">
                    <svg viewBox="0 0 24 24" width="16" height="16">
                        <path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"></path>
                    </svg>
                </div>
            `;
            const toastContentEl = toast.querySelector('.toast-content');
            if (toastContentEl) toastContentEl.textContent = (message == null ? '' : String(message));
            
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
        // ==================== Tool Calls Capability Probe ====================
        // 关键词: aibalance probe tool calls, capability matrix manual trigger
        let isToolCallsProbeInProgress = false;
        async function probeAllToolCalls() {
            if (isToolCallsProbeInProgress) return;
            isToolCallsProbeInProgress = true;
            const button = document.getElementById('probe-all-tool-calls-btn');
            const originalHTML = button ? button.innerHTML : '';
            if (button) {
                button.innerHTML = `
                    <svg viewBox="0 0 24 24" class="rotating" style="width: 16px; height: 16px; margin-right: 6px;">
                        <path fill="currentColor" d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15l-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z"/>
                    </svg>
                    探测中...
                `;
                button.disabled = true;
            }
            try {
                const response = await fetch('/portal/probe-tool-calls-all', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                });
                if (!response.ok) {
                    throw new Error('probe request failed: HTTP ' + response.status);
                }
                const data = await response.json();
                if (!data.success) {
                    throw new Error(data.message || 'probe failed');
                }
                const results = Array.isArray(data.data) ? data.data : [];
                let native = 0, react = 0, skipped = 0, failed = 0;
                results.forEach(item => {
                    if (item.skipped) { skipped += 1; return; }
                    if (item.error) { failed += 1; return; }
                    if (item.round1_mode === 'native' && item.round2_mode === 'native') native += 1;
                    else react += 1;
                });
                showToast(`工具调用能力探测完成: native=${native} react=${react} skipped=${skipped} failed=${failed}`, 'success');
            } catch (e) {
                showToast('工具调用能力探测失败: ' + e.message, 'error');
            } finally {
                if (button) {
                    button.innerHTML = originalHTML;
                    button.disabled = false;
                }
                isToolCallsProbeInProgress = false;
            }
        }

        async function probeSingleToolCalls(providerId) {
            try {
                const response = await fetch(`/portal/probe-tool-calls/${providerId}`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                });
                if (!response.ok) {
                    throw new Error('probe request failed: HTTP ' + response.status);
                }
                const data = await response.json();
                if (!data.success) {
                    throw new Error(data.message || 'probe failed');
                }
                const d = data.data || {};
                showToast(`Provider ${d.wrapper_name || providerId} 探测完成: round1=${d.round1_mode} round2=${d.round2_mode}${d.error ? ' err=' + d.error : ''}`, 'success');
            } catch (e) {
                showToast('工具调用能力探测失败: ' + e.message, 'error');
            }
        }

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
            
            // 模型名通过索引回查（portalToggleModelByIndex），不内联进 onclick 字符串，
            // 彻底规避模型名含引号导致的属性/处理器注入；展示文本仍走 escapeHtml。
            // 关键词: portalRenderModelList XSS 防护, 索引法 onclick
            modelList.innerHTML = portalAvailableModels.map((model, idx) => `
                <div class="model-item ${portalSelectedModels.has(model) ? 'selected' : ''}" onclick="portalToggleModelByIndex(${idx})">
                    <input type="checkbox" ${portalSelectedModels.has(model) ? 'checked' : ''} onclick="event.stopPropagation(); portalToggleModelByIndex(${idx})">
                    <label>${escapeHtml(model)}</label>
                </div>
            `).join('');
            
            portalUpdateSelectedPreview();
        }

        // portalToggleModelByIndex 用列表索引回查模型名后再切换选中，避免内联模型名进 onclick。
        function portalToggleModelByIndex(idx) {
            const model = portalAvailableModels[idx];
            if (model != null) portalToggleModel(model);
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
            
            // 读取可选的绑定用户信息（用户名/备注/metainfo）
            // 关键词: generateNewApiKey username remark metainfo 携带
            const unameEl = document.getElementById('apiKeyUsernameInput');
            const remarkEl = document.getElementById('apiKeyRemarkInput');
            const metaEl = document.getElementById('apiKeyMetaInfoInput');
            const username = unameEl ? unameEl.value.trim() : '';
            const remark = remarkEl ? remarkEl.value : '';
            const metainfo = metaEl ? metaEl.value : '';

            try {
                const response = await fetch('/portal/generate-api-key', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    // 将选中的模型与绑定信息包含在请求体中
                    body: JSON.stringify({ allowed_models: selectedModels, username: username, remark: remark, metainfo: metainfo })
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
                    
                    // 优先使用后端返回的域名建议，硬编码作为 fallback
                    const fallbackDomainSuggestions = {
                        'openai': 'api.openai.com',
                        'siliconflow': 'api.siliconflow.cn',
                        'tongyi': 'dashscope.aliyuncs.com',
                        'moonshot': 'api.moonshot.cn',
                        'deepseek': 'api.deepseek.com',
                        'gemini': 'generativelanguage.googleapis.com',
                        'ollama': '127.0.0.1:11434',
                        'chatglm': 'open.bigmodel.cn',
                        'volcengine': 'ark.cn-beijing.volces.com',
                        'openrouter': 'openrouter.ai',
                        'comate': 'comate.baidu.com'
                    };
                    
                    if (autoCompleteData.domain_suggestions && autoCompleteData.domain_suggestions[selectedType] !== undefined) {
                        suggestedDomain = autoCompleteData.domain_suggestions[selectedType];
                    } else if (fallbackDomainSuggestions[selectedType] !== undefined) {
                        suggestedDomain = fallbackDomainSuggestions[selectedType];
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
                params.append('provider_mode', providerMode);
                const optAllowReason = document.getElementById('optionalAllowReason');
                if (optAllowReason && optAllowReason.value) {
                    params.append('optional_allow_reason', optAllowReason.value);
                }
                if (noHTTPSCheckbox.checked) {
                    params.append('no_https', 'on');
                }
                const activeCCInput = document.getElementById('activeCacheControl');
                if (activeCCInput && activeCCInput.checked) {
                    params.append('active_cache_control', 'on');
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
            const activeCC = row.dataset.activeCacheControl === '1'; // Active Cache Control flag, 见 renderProviders 写入

            console.log(`Extracted data: Wrapper=${wrapperName}, Model=${modelName}, Type=${typeName}, Domain=${domainOrURL}, Key=...${apiKey ? apiKey.slice(-4) : ''}, ActiveCC=${activeCC}`); // Debug log

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

                // 同步 Active Cache Control 复选框, 让 quickAdd 拷贝原 provider 的设置
                // 关键词: quickAddProvider activeCacheControl 同步, 主动 cache_control 注入开关
                const activeCCInput = document.getElementById('activeCacheControl');
                if (activeCCInput) {
                    activeCCInput.checked = activeCC;
                }

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
            
            // 同 portalRenderModelList：模型名按索引回查，避免内联进 onclick 造成注入。
            // 关键词: editModalRenderModelList XSS 防护, 索引法 onclick
            modelList.innerHTML = availableModels.map((model, idx) => `
                <div class="model-item ${editModalSelectedModels.has(model) ? 'selected' : ''}" onclick="editModalToggleModelByIndex(${idx})">
                    <input type="checkbox" ${editModalSelectedModels.has(model) ? 'checked' : ''} onclick="event.stopPropagation(); editModalToggleModelByIndex(${idx})">
                    <label>${escapeHtml(model)}</label>
                </div>
            `).join('');
            
            editModalUpdateSelectedPreview();
        }

        // editModalToggleModelByIndex 用索引从当前模型列表回查模型名后切换选中。
        function editModalToggleModelByIndex(idx) {
            const list = window.editModalAvailableModels || [];
            const model = list[idx];
            if (model != null) editModalToggleModel(model);
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
        // wrapper 级仅编辑 描述/标签；传统倍数(字节流量)已彻底移除，Token 计费倍率在「实际模型计费倍率」表设置
        // 关键词: openEditModelModal wrapper 描述标签, 传统倍数字段已移除
        function openEditModelModal(name, description, tags) {
            document.getElementById('editModelName').value = name;
            document.getElementById('editModelDescription').value = description;
            document.getElementById('editModelTags').value = tags;

            // 修复定位 bug: .delete-confirmation-modal 依赖 flex 居中。
            // 关键词: editModelMetaModal display flex 居中
            document.getElementById('editModelMetaModal').style.display = 'flex';
        }

        function closeEditModelModal() {
            document.getElementById('editModelMetaModal').style.display = 'none';
        }

        function saveModelMeta() {
            const name = document.getElementById('editModelName').value;
            const description = document.getElementById('editModelDescription').value;
            const tags = document.getElementById('editModelTags').value;

            // wrapper 级仅提交描述/标签；不再提交传统字节倍数（后端缺省时保持原值不变）。
            // Token 计费倍率在「实际模型计费倍率」表设置。
            // 关键词: saveModelMeta wrapper 元数据提交, 不含 traffic_multiplier, 不含 token 倍率
            fetch('/portal/update-model-meta', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    model_name: name,
                    description: description,
                    tags: tags
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

        // ==================== 实际模型计费倍率 + 批量应用 ====================
        // 关键词: 实际模型倍率, 按内部转发名计费, 按模式批量, 勾选批量

        function readMulValue(id) {
            const el = document.getElementById(id);
            if (!el) return 0;
            const v = parseFloat(el.value);
            return (isNaN(v) || v < 0) ? 0 : v;
        }

        // ---- 单个实际模型倍率编辑 ----
        // 关键词: openModelMultiplierModal, 实际模型倍率编辑
        function openModelMultiplierModal(internal, cfgIn, cfgOut, cfgCc, cfgCh, isFree) {
            document.getElementById('modelMultiplierInternal').value = internal;
            const setv = (id, v) => {
                const el = document.getElementById(id);
                if (el) el.value = (typeof v === 'number' && v > 0) ? v : 0;
            };
            setv('modelMultiplierInput', cfgIn);
            setv('modelMultiplierOutput', cfgOut);
            setv('modelMultiplierCacheCreate', cfgCc);
            setv('modelMultiplierCacheHit', cfgCh);
            const freeEl = document.getElementById('modelMultiplierIsFree');
            if (freeEl) freeEl.checked = !!isFree;
            document.getElementById('modelMultiplierModal').style.display = 'flex';
        }

        function closeModelMultiplierModal() {
            document.getElementById('modelMultiplierModal').style.display = 'none';
        }

        function saveModelMultiplier() {
            const internal = document.getElementById('modelMultiplierInternal').value;
            const freeEl = document.getElementById('modelMultiplierIsFree');
            fetch('/portal/update-model-multiplier', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    internal_model_name: internal,
                    input_token_multiplier: readMulValue('modelMultiplierInput'),
                    output_token_multiplier: readMulValue('modelMultiplierOutput'),
                    cache_creation_multiplier: readMulValue('modelMultiplierCacheCreate'),
                    cache_hit_multiplier: readMulValue('modelMultiplierCacheHit'),
                    is_free: freeEl ? !!freeEl.checked : false
                })
            })
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    showToast('倍率已保存', 'success');
                    closeModelMultiplierModal();
                    setTimeout(() => window.location.reload(), 800);
                } else {
                    showToast('保存失败: ' + data.message, 'error');
                }
            })
            .catch(error => {
                console.error('Error:', error);
                showToast('保存失败', 'error');
            });
        }

        function clearModelMultiplier() {
            const internal = document.getElementById('modelMultiplierInternal').value;
            clearModelMultiplierDirect(internal);
        }

        // 直接清除某实际模型倍率（表行的「清除」按钮调用）
        function clearModelMultiplierDirect(internal) {
            if (!confirm('确定清除该实际模型的计费倍率？清除后将回落到全局默认 / 系统常量。')) {
                return;
            }
            fetch('/portal/delete-model-multiplier', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ internal_model_name: internal })
            })
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    showToast('倍率已清除', 'success');
                    if (document.getElementById('modelMultiplierModal').style.display === 'flex') {
                        closeModelMultiplierModal();
                    }
                    setTimeout(() => window.location.reload(), 800);
                } else {
                    showToast('清除失败: ' + data.message, 'error');
                }
            })
            .catch(error => {
                console.error('Error:', error);
                showToast('清除失败', 'error');
            });
        }

        // ---- 勾选辅助 ----
        // 关键词: toggleAllActualModels, getSelectedInternalModels, 勾选
        function toggleAllActualModels(checkbox) {
            document.querySelectorAll('.actual-model-check').forEach(cb => {
                cb.checked = checkbox.checked;
            });
        }

        function getSelectedInternalModels() {
            const names = [];
            document.querySelectorAll('.actual-model-check:checked').forEach(cb => {
                if (cb.value) names.push(cb.value);
            });
            return names;
        }

        // ---- 全局默认倍率 ----
        // 关键词: openGlobalDefaultMultiplierModal, 全局默认倍率
        function openGlobalDefaultMultiplierModal() {
            const g = (typeof portalData === 'object' && portalData && portalData.global_default_multiplier) || {};
            const setv = (id, v) => {
                const el = document.getElementById(id);
                if (el) el.value = (typeof v === 'number' && v > 0) ? v : 0;
            };
            setv('globalDefaultInput', g.input_token_multiplier);
            setv('globalDefaultOutput', g.output_token_multiplier);
            setv('globalDefaultCacheCreate', g.cache_creation_multiplier);
            setv('globalDefaultCacheHit', g.cache_hit_multiplier);
            document.getElementById('globalDefaultMultiplierModal').style.display = 'flex';
        }

        function closeGlobalDefaultMultiplierModal() {
            document.getElementById('globalDefaultMultiplierModal').style.display = 'none';
        }

        function saveGlobalDefaultMultiplier() {
            fetch('/portal/set-global-default-multiplier', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    input_token_multiplier: readMulValue('globalDefaultInput'),
                    output_token_multiplier: readMulValue('globalDefaultOutput'),
                    cache_creation_multiplier: readMulValue('globalDefaultCacheCreate'),
                    cache_hit_multiplier: readMulValue('globalDefaultCacheHit')
                })
            })
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    showToast('全局默认倍率已保存', 'success');
                    closeGlobalDefaultMultiplierModal();
                    setTimeout(() => window.location.reload(), 800);
                } else {
                    showToast('保存失败: ' + data.message, 'error');
                }
            })
            .catch(error => {
                console.error('Error:', error);
                showToast('保存失败', 'error');
            });
        }

        // ---- 按模式批量应用 ----
        // 关键词: openPatternMultiplierModal, applyPatternMultiplier, 按模式批量
        function openPatternMultiplierModal() {
            document.getElementById('patternMultiplierModal').style.display = 'flex';
        }

        function closePatternMultiplierModal() {
            document.getElementById('patternMultiplierModal').style.display = 'none';
        }

        function applyPatternMultiplier() {
            const pattern = (document.getElementById('patternMultiplierPattern').value || '').trim();
            if (!pattern) {
                showToast('请输入名称模式', 'error');
                return;
            }
            if (!confirm('将把该组倍率应用到所有匹配 "' + pattern + '" 的实际模型，覆盖它们当前的设置。确定继续？')) {
                return;
            }
            fetch('/portal/apply-model-multiplier-by-pattern', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    pattern: pattern,
                    input_token_multiplier: readMulValue('patternMultiplierInput'),
                    output_token_multiplier: readMulValue('patternMultiplierOutput'),
                    cache_creation_multiplier: readMulValue('patternMultiplierCacheCreate'),
                    cache_hit_multiplier: readMulValue('patternMultiplierCacheHit')
                })
            })
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    showToast('已应用到 ' + (data.applied || 0) + ' 个实际模型', 'success');
                    closePatternMultiplierModal();
                    setTimeout(() => window.location.reload(), 1000);
                } else {
                    showToast('应用失败: ' + data.message, 'error');
                }
            })
            .catch(error => {
                console.error('Error:', error);
                showToast('应用失败', 'error');
            });
        }

        // ---- 应用到勾选 ----
        // 关键词: openSelectedMultiplierModal, applySelectedMultiplier, 勾选批量
        function openSelectedMultiplierModal() {
            const names = getSelectedInternalModels();
            if (names.length === 0) {
                showToast('请先勾选至少一个实际模型', 'error');
                return;
            }
            const countEl = document.getElementById('selectedMultiplierCount');
            if (countEl) countEl.textContent = names.length;
            document.getElementById('selectedMultiplierModal').style.display = 'flex';
        }

        function closeSelectedMultiplierModal() {
            document.getElementById('selectedMultiplierModal').style.display = 'none';
        }

        function applySelectedMultiplier() {
            const names = getSelectedInternalModels();
            if (names.length === 0) {
                showToast('没有勾选任何实际模型', 'error');
                return;
            }
            fetch('/portal/apply-model-multiplier-to-models', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    internal_model_names: names,
                    input_token_multiplier: readMulValue('selectedMultiplierInput'),
                    output_token_multiplier: readMulValue('selectedMultiplierOutput'),
                    cache_creation_multiplier: readMulValue('selectedMultiplierCacheCreate'),
                    cache_hit_multiplier: readMulValue('selectedMultiplierCacheHit')
                })
            })
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    showToast('已应用到 ' + (data.applied || 0) + ' 个实际模型', 'success');
                    closeSelectedMultiplierModal();
                    setTimeout(() => window.location.reload(), 1000);
                } else {
                    showToast('应用失败: ' + data.message, 'error');
                }
            })
            .catch(error => {
                console.error('Error:', error);
                showToast('应用失败', 'error');
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

        // ==================== API Key Token Limit (recommended) ====================
        //
        // 关键词: showTokenLimitDialog saveTokenLimit resetAPIKeyToken,
        //        API Key Token 维度限额 UI, 推荐使用替代字节限额
        //
        // UI 设计：
        //   1. 限额输入单位为 M tokens（百万）；保存时转换为 raw token；
        //   2. 顶部明确说明「推荐使用 Token 限额，替代流量限制」；
        //   3. 支持启用/禁用开关；
        //   4. 同 modal 提供「重置 Token 用量」入口（沿用 traffic 风格）。

        function showTokenLimitDialog(apiKeyId, currentLimit, currentUsed, enabled) {
            // 关闭已存在 modal 避免重复（防止快速点击多次）
            const existing = document.getElementById('tokenLimitModal');
            if (existing) existing.remove();

            const usedRaw = Number(currentUsed) || 0;
            const limitRaw = Number(currentLimit) || 0;
            // 默认以 M 为单位展示；如果限额非 M 的整数倍则显示带小数
            const limitM = limitRaw > 0 ? (limitRaw / 1_000_000) : 0;
            const usedDisplay = formatTokenCount(usedRaw);
            const usedM = (usedRaw / 1_000_000).toFixed(usedRaw === 0 ? 0 : 3);

            const html = `
                <div id="tokenLimitModal" class="delete-confirmation-modal" style="display: flex;">
                    <div class="modal-content" style="width: 480px; max-width: 90vw;">
                        <span class="close-modal" onclick="closeTokenLimitModal()">&times;</span>
                        <h4>Token 限额设置 <small style="color:#1976d2;font-weight:normal;">（推荐使用，替代字节流量限制）</small></h4>
                        <div class="form-group">
                            <label>API Key ID: ${apiKeyId}</label>
                        </div>
                        <div class="form-group">
                            <label>当前已用 Token: <strong>${usedRaw}</strong> (${usedDisplay} ≈ ${usedM} M)</label>
                        </div>
                        <div class="form-group">
                            <label for="tokenLimitMInput">Token 限额（单位：M tokens）:</label>
                            <input type="number" id="tokenLimitMInput" class="form-control" value="${limitM}" min="0" step="0.1" oninput="updateRMBHint('tokenLimitMInput','tokenLimitRmbHint')">
                            <small id="tokenLimitRmbHint" style="color:#2e7d32; font-size:12px; display:block; margin-top:4px;"></small>
                            <small class="form-text text-muted">
                                设置为 0 表示不限制。Token 维度按上游 SSE 末帧 usage 经四维倍率
                                加权后累加 (input/output/cache_creation/cache_hit)，更贴近真实计费。
                            </small>
                        </div>
                        <div class="form-group">
                            <label>
                                <input type="checkbox" id="tokenLimitEnableInput" ${enabled ? 'checked' : ''}>
                                启用 Token 限额
                            </label>
                        </div>
                        <div class="modal-actions">
                            <button class="btn" onclick="resetAPIKeyToken(${apiKeyId})" style="background:#ff9800;color:#fff;">重置 Token 用量</button>
                            <span style="flex:1;"></span>
                            <button class="btn" onclick="closeTokenLimitModal()">取消</button>
                            <button class="btn btn-primary" onclick="saveTokenLimit(${apiKeyId})">保存</button>
                        </div>
                    </div>
                </div>
            `;
            document.body.insertAdjacentHTML('beforeend', html);
            // 初始化 1 RMB=10M 计费 Token 换算提示
            updateRMBHint('tokenLimitMInput', 'tokenLimitRmbHint');
        }

        function closeTokenLimitModal() {
            const modal = document.getElementById('tokenLimitModal');
            if (modal) modal.remove();
        }

        async function saveTokenLimit(apiKeyId) {
            const limitMRaw = parseFloat(document.getElementById('tokenLimitMInput').value);
            const limitM = isFinite(limitMRaw) && limitMRaw > 0 ? limitMRaw : 0;
            // Math.round 用于避免浮点累积误差（例如 1.1 * 1e6 = 1100000.0000001）
            const limitRaw = Math.round(limitM * 1_000_000);
            const enabled = document.getElementById('tokenLimitEnableInput').checked;

            try {
                const response = await fetch(`/portal/api-key-token-limit/${apiKeyId}`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ token_limit: limitRaw, enable: enabled })
                });
                const data = await response.json();
                if (!response.ok || !data.success) {
                    throw new Error(data.message || '保存 Token 限额失败');
                }
                showToast('Token 限额已保存', 'success');
                closeTokenLimitModal();
                setTimeout(() => window.location.reload(), 800);
            } catch (error) {
                showToast(`保存失败: ${error.message}`, 'error');
            }
        }

        async function resetAPIKeyToken(apiKeyId) {
            if (!confirm('确定要重置这个 API Key 的 Token 用量计数吗？')) {
                return;
            }
            try {
                const response = await fetch(`/portal/reset-api-key-token/${apiKeyId}`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' }
                });
                const data = await response.json();
                if (!response.ok || !data.success) {
                    throw new Error(data.message || '重置 Token 用量失败');
                }
                showToast('Token 用量已重置', 'success');
                closeTokenLimitModal();
                setTimeout(() => window.location.reload(), 800);
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
                    <td class="copyable editable-allowed-models" data-api-id="${key.id}" data-current-models="${escapeHtml(key.allowed_models)}" data-full-text="${escapeHtml(key.allowed_models)}" title="右键点击修改允许的模型">${renderAllowedModelsCellContent(key.allowed_models)}</td>
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

                    // 防御：历史 bug 曾把"允许模型"列拖到几千 px，并被
                    // localStorage 持久化下来。这里在恢复时按 CSS 上限钳制，
                    // 避免每次刷新都把 #api-table 撑爆。
                    const apiTableColumnCaps = {
                        4: 320,  // 允许模型 (0-based index, 第 5 列)
                    };
                    const isApiTable = tableId === 'api-table';

                    headers.forEach((th, index) => {
                        if (widths[index]) {
                            let w = widths[index];
                            if (isApiTable && apiTableColumnCaps[index] && w > apiTableColumnCaps[index]) {
                                w = apiTableColumnCaps[index];
                            }
                            const width = w + 'px';
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
                                    let w = widths[index];
                                    if (isApiTable && apiTableColumnCaps[index] && w > apiTableColumnCaps[index]) {
                                        w = apiTableColumnCaps[index];
                                    }
                                    const width = w + 'px';
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
        
        // 显示可复制的敏感结果弹窗（如重置后的新密码 / 新 OPS Key）
        // 关键词: showSecretResult 可复制结果弹窗, 替代 alert 不可复制问题, 自动选中便于复制
        function showSecretResult(title, label, value) {
            document.getElementById('secretResultTitle').textContent = title;
            document.getElementById('secretResultLabel').textContent = label + ':';
            const input = document.getElementById('secretResultValue');
            input.value = value;
            document.getElementById('secretResultModal').style.display = 'flex';
            // 自动聚焦并选中，方便直接 Ctrl/Cmd+C 复制
            setTimeout(function() { input.focus(); input.select(); }, 50);
        }

        // 关闭敏感结果弹窗
        function closeSecretResultModal() {
            document.getElementById('secretResultModal').style.display = 'none';
        }

        // 复制敏感结果弹窗中的值
        function copySecretResult() {
            const value = document.getElementById('secretResultValue').value;
            copyToClipboard(value);
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
                    <td>${escapeHtml(user.username)}</td>
                    <td><span style="background: #e3f2fd; color: #1565c0; padding: 2px 8px; border-radius: 4px; font-size: 12px;">${escapeHtml(String(user.role || '').toUpperCase())}</span></td>
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
                    showSecretResult('密码重置成功', '新密码', data.new_password);
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
                    showSecretResult('OPS Key 重置成功', '新 OPS Key', data.new_ops_key);
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
            
            // 日志各字段（操作者名/目标/详情/IP）可能含用户可控内容（如绑定用户名、X-Forwarded-For），
            // 一律 escapeHtml 后再渲染，title 属性同样转义，避免存储型 XSS 打穿后台。
            // 关键词: renderOpsLogsTable XSS 防护, detail/ip 转义
            tbody.innerHTML = opsLogsData.map(log => `
                <tr>
                    <td>${escapeHtml(log.id)}</td>
                    <td>${escapeHtml(log.operator_name)}</td>
                    <td><span style="background: #e3f2fd; color: #1565c0; padding: 2px 8px; border-radius: 4px; font-size: 12px;">${escapeHtml(actionLabels[log.action] || log.action)}</span></td>
                    <td>${escapeHtml(log.target_type)}</td>
                    <td>${escapeHtml(log.target_id)}</td>
                    <td style="max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title="${escapeHtml(log.detail || '')}">${escapeHtml(log.detail) || '-'}</td>
                    <td>${escapeHtml(log.ip_address) || '-'}</td>
                    <td>${escapeHtml(log.created_at)}</td>
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
                const qwenTbody = document.getElementById('ws-qwen-tbody');
                if (braveTbody) braveTbody.innerHTML = '<tr><td colspan="10" style="text-align: center; color: #e74c3c; padding: 20px;">加载失败</td></tr>';
                if (tavilyTbody) tavilyTbody.innerHTML = '<tr><td colspan="10" style="text-align: center; color: #e74c3c; padding: 20px;">加载失败</td></tr>';
                if (chatglmTbody) chatglmTbody.innerHTML = '<tr><td colspan="10" style="text-align: center; color: #e74c3c; padding: 20px;">加载失败</td></tr>';
                if (bochaTbody) bochaTbody.innerHTML = '<tr><td colspan="10" style="text-align: center; color: #e74c3c; padding: 20px;">加载失败</td></tr>';
                if (unifuncsTbody) unifuncsTbody.innerHTML = '<tr><td colspan="10" style="text-align: center; color: #e74c3c; padding: 20px;">加载失败</td></tr>';
                if (qwenTbody) qwenTbody.innerHTML = '<tr><td colspan="10" style="text-align: center; color: #e74c3c; padding: 20px;">加载失败</td></tr>';
            }
        }
        
        function renderWebSearchKeysTable() {
            const filter = document.getElementById('ws-type-filter')?.value || '';
            const braveKeys = webSearchKeysData.filter(k => k.searcher_type === 'brave');
            const tavilyKeys = webSearchKeysData.filter(k => k.searcher_type === 'tavily');
            const chatglmKeys = webSearchKeysData.filter(k => k.searcher_type === 'chatglm');
            const bochaKeys = webSearchKeysData.filter(k => k.searcher_type === 'bocha');
            const unifuncsKeys = webSearchKeysData.filter(k => k.searcher_type === 'unifuncs');
            const qwenKeys = webSearchKeysData.filter(k => k.searcher_type === 'qwen');
            
            const showBrave = !filter || filter === 'brave';
            const showTavily = !filter || filter === 'tavily';
            const showChatglm = !filter || filter === 'chatglm';
            const showBocha = !filter || filter === 'bocha';
            const showUnifuncs = !filter || filter === 'unifuncs';
            const showQwen = !filter || filter === 'qwen';
            
            renderWebSearchTypeTable('ws-brave-tbody', showBrave ? braveKeys : []);
            renderWebSearchTypeTable('ws-tavily-tbody', showTavily ? tavilyKeys : []);
            renderWebSearchTypeTable('ws-chatglm-tbody', showChatglm ? chatglmKeys : []);
            renderWebSearchTypeTable('ws-bocha-tbody', showBocha ? bochaKeys : []);
            renderWebSearchTypeTable('ws-unifuncs-tbody', showUnifuncs ? unifuncsKeys : []);
            renderWebSearchTypeTable('ws-qwen-tbody', showQwen ? qwenKeys : []);
            
            // Show/hide table sections based on filter
            const braveSection = document.getElementById('ws-brave-table');
            const tavilySection = document.getElementById('ws-tavily-table');
            const chatglmSection = document.getElementById('ws-chatglm-table');
            const bochaSection = document.getElementById('ws-bocha-table');
            const unifuncsSection = document.getElementById('ws-unifuncs-table');
            const qwenSection = document.getElementById('ws-qwen-table');
            if (braveSection) braveSection.closest('.table-container').style.display = showBrave ? '' : 'none';
            if (tavilySection) tavilySection.closest('.table-container').style.display = showTavily ? '' : 'none';
            if (chatglmSection) chatglmSection.closest('.table-container').style.display = showChatglm ? '' : 'none';
            if (bochaSection) bochaSection.closest('.table-container').style.display = showBocha ? '' : 'none';
            if (unifuncsSection) unifuncsSection.closest('.table-container').style.display = showUnifuncs ? '' : 'none';
            if (qwenSection) qwenSection.closest('.table-container').style.display = showQwen ? '' : 'none';
            
            // Also show/hide the section headers
            if (braveSection) braveSection.closest('.table-container').previousElementSibling.style.display = showBrave ? '' : 'none';
            if (tavilySection) tavilySection.closest('.table-container').previousElementSibling.style.display = showTavily ? '' : 'none';
            if (chatglmSection) chatglmSection.closest('.table-container').previousElementSibling.style.display = showChatglm ? '' : 'none';
            if (bochaSection) bochaSection.closest('.table-container').previousElementSibling.style.display = showBocha ? '' : 'none';
            if (unifuncsSection) unifuncsSection.closest('.table-container').previousElementSibling.style.display = showUnifuncs ? '' : 'none';
            if (qwenSection) qwenSection.closest('.table-container').previousElementSibling.style.display = showQwen ? '' : 'none';
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
        //
        // 历史教训: 这块原本是大段 `window.X = X;` 直接赋值. 任意一个标识符未声明
        // (例如曾经的 selectAllProviders / selectAllAPIKeys 占位名) 都会抛
        // ReferenceError, 把整段顶层脚本执行掐断, 后果是:
        //   - 后续所有 expose 不再绑定
        //   - MirrorMgmt 等模块都不会被装配到 window
        //   - HTML 内联 onclick="MirrorMgmt.xxx()" 全部 ReferenceError
        //
        // 解决方案: 用 __exposeFn(name, () => X) 形式. 由于 getter 是 lazy 求值,
        // 内部出现 ReferenceError 只会被自身 try-catch 兜底, 不影响其他行.
        // 同时 console.warn 出来便于排查到底哪个函数还没实现.
        //
        // 关键词: portal.js __exposeFn defensive global binding, ReferenceError safe,
        // top-level script abort 防御, window inline onclick 绑定保护
        function __exposeFn(name, getter) {
            try {
                const v = getter();
                if (typeof v !== 'undefined') {
                    window[name] = v;
                }
            } catch (e) {
                console.warn('[expose] skip ' + name + ': ' + ((e && e.message) || e));
            }
        }

        // Provider 相关
        __exposeFn('confirmDeleteSelected', () => confirmDeleteSelected);
        __exposeFn('deleteProvider', () => deleteProvider);
        __exposeFn('deleteMultipleProviders', () => deleteMultipleProviders);
        __exposeFn('checkSingleProvider', () => checkSingleProvider);
        __exposeFn('checkAllProvidersHealth', () => checkAllProvidersHealth);
        __exposeFn('checkSelectedProvider', () => checkSelectedProvider);
        __exposeFn('selectAllProviders', () => selectAllProviders);
        __exposeFn('probeAllToolCalls', () => probeAllToolCalls);
        __exposeFn('probeSingleToolCalls', () => probeSingleToolCalls);
        __exposeFn('updateDeleteSelectedButton', () => updateDeleteSelectedButton);

        // API Key 相关
        __exposeFn('confirmDeleteSelectedAPI', () => confirmDeleteSelectedAPI);
        __exposeFn('confirmDeleteSelectedAPIKeys', () => confirmDeleteSelectedAPI); // 别名，兼容 HTML 中的调用
        __exposeFn('deleteAPIKey', () => deleteAPIKey);
        __exposeFn('deleteMultipleAPIKeys', () => deleteMultipleAPIKeys);
        __exposeFn('toggleAPIKeyStatus', () => toggleAPIKeyStatus);
        __exposeFn('confirmDisableSelectedAPI', () => confirmDisableSelectedAPI);
        __exposeFn('confirmEnableSelectedAPI', () => confirmEnableSelectedAPI);
        __exposeFn('disableMultipleAPIKeys', () => disableMultipleAPIKeys);
        __exposeFn('enableMultipleAPIKeys', () => enableMultipleAPIKeys);
        __exposeFn('toggleSelectAllAPI', () => toggleSelectAllAPI);
        __exposeFn('selectAllAPIKeys', () => selectAllAPIKeys);
        __exposeFn('updateDeleteSelectedAPIButton', () => updateDeleteSelectedAPIButton);
        // API Key 绑定用户信息编辑 + 用户名过滤
        __exposeFn('openApiKeyMetaModal', () => openApiKeyMetaModal);
        __exposeFn('closeApiKeyMetaModal', () => closeApiKeyMetaModal);
        __exposeFn('saveApiKeyMeta', () => saveApiKeyMeta);
        __exposeFn('applyApiKeyUsernameFilter', () => applyApiKeyUsernameFilter);
        __exposeFn('clearApiKeyUsernameFilter', () => clearApiKeyUsernameFilter);

        // 流量限制相关
        __exposeFn('showTrafficLimitDialog', () => showTrafficLimitDialog);
        __exposeFn('showTrafficLimitModal', () => showTrafficLimitDialog); // 别名
        __exposeFn('closeTrafficLimitModal', () => closeTrafficLimitModal);
        __exposeFn('saveTrafficLimit', () => saveTrafficLimit);
        __exposeFn('closeTrafficLimitDialog', () => closeTrafficLimitModal); // 别名
        // 修正：之前赋值的是不存在的标识符 resetApiKeyTraffic（小写 pi），
        // 函数实际定义为 resetAPIKeyTraffic（大写 API）。统一指向真实函数。
        // 关键词: resetAPIKeyTraffic 名称大小写修复
        __exposeFn('resetApiKeyTraffic', () => resetAPIKeyTraffic);
        __exposeFn('resetAPIKeyTraffic', () => resetAPIKeyTraffic);

        // Token 限额相关（推荐使用，替代字节限额）
        // 关键词: window.showTokenLimitDialog window.saveTokenLimit window.resetAPIKeyToken
        __exposeFn('showTokenLimitDialog', () => showTokenLimitDialog);
        __exposeFn('closeTokenLimitModal', () => closeTokenLimitModal);
        __exposeFn('saveTokenLimit', () => saveTokenLimit);
        __exposeFn('resetAPIKeyToken', () => resetAPIKeyToken);
        __exposeFn('formatTokenCount', () => formatTokenCount);
        // 1 RMB=10M 计费 Token 实时换算提示
        __exposeFn('updateRMBHint', () => updateRMBHint);
        // 模型级免费 Token 覆盖行的金额限制换算提示
        __exposeFn('updateFreeTokenRowRMB', () => updateFreeTokenRowRMB);

        // 内存和系统监控相关
        __exposeFn('showMemoryDialog', () => showMemoryDialog);
        __exposeFn('closeMemoryDialog', () => closeMemoryDialog);
        __exposeFn('fetchMemoryStats', () => fetchMemoryStats);
        __exposeFn('forceGC', () => forceGC);
        __exposeFn('fetchGoroutineDump', () => fetchGoroutineDump);

        // 筛选相关
        __exposeFn('filterProviders', () => filterProviders);
        __exposeFn('filterApiKeys', () => filterApiKeys);

        // 其他操作函数
        __exposeFn('showToast', () => showToast);
        __exposeFn('hideContextMenu', () => hideContextMenu);
        __exposeFn('copyToClipboard', () => copyToClipboard);
        __exposeFn('generateNewApiKey', () => generateNewApiKey);
        __exposeFn('confirmAndGenerateApiKey', () => confirmAndGenerateApiKey);
        __exposeFn('showApiKeySuccessModal', () => showApiKeySuccessModal);
        __exposeFn('closeApiKeySuccessModal', () => closeApiKeySuccessModal);
        __exposeFn('copyGeneratedApiKey', () => copyGeneratedApiKey);

        // 模型相关
        __exposeFn('openEditModelModal', () => openEditModelModal);
        __exposeFn('showCurlCommand', () => showCurlCommand);
        __exposeFn('closeEditModelModal', () => closeEditModelModal);
        __exposeFn('saveModelMetadata', () => saveModelMetadata);
        __exposeFn('closeCurlModal', () => closeCurlModal);
        __exposeFn('copyCurlCommand', () => copyCurlCommand);

        // 右键菜单相关（Provider）
        __exposeFn('quickAddProvider', () => quickAddProvider);
        __exposeFn('copySimilarProviderKeys', () => copySimilarProviderKeys);
        __exposeFn('deleteSelectedProvider', () => deleteSelectedProvider);
        __exposeFn('showContextMenu', () => showContextMenu);
        __exposeFn('initializeContextMenu', () => initializeContextMenu);

        // 同类供应商 Keys 弹窗相关
        __exposeFn('showSimilarKeysModal', () => showSimilarKeysModal);
        __exposeFn('closeCopySimilarKeysModal', () => closeCopySimilarKeysModal);
        __exposeFn('copySimilarKeysToClipboard', () => copySimilarKeysToClipboard);

        // 右键菜单相关（API Key）
        __exposeFn('triggerEditAllowedModelsFromContextMenu', () => triggerEditAllowedModelsFromContextMenu);
        __exposeFn('showEditAllowedModelsModal', () => showEditAllowedModelsModal);
        __exposeFn('closeEditAllowedModelsModal', () => closeEditAllowedModelsModal);
        __exposeFn('saveEditedAllowedModels', () => saveEditedAllowedModels);

        // Tab 切换
        __exposeFn('openTab', () => openTab);
        __exposeFn('switchTab', () => switchTab);

        // 关闭 API Key 成功模态框
        __exposeFn('closeApiKeyModal', () => closeApiKeyModal);

        // OPS 用户管理相关
        __exposeFn('showCreateOpsUserModal', () => showCreateOpsUserModal);
        __exposeFn('closeCreateOpsUserModal', () => closeCreateOpsUserModal);
        __exposeFn('createOpsUser', () => createOpsUser);
        __exposeFn('closeOpsUserCredentialsModal', () => closeOpsUserCredentialsModal);
        __exposeFn('copyOpsCredentials', () => copyOpsCredentials);
        __exposeFn('refreshOpsUsers', () => refreshOpsUsers);
        __exposeFn('deleteOpsUser', () => deleteOpsUser);
        __exposeFn('toggleOpsUserStatus', () => toggleOpsUserStatus);
        __exposeFn('resetOpsUserPassword', () => resetOpsUserPassword);
        __exposeFn('resetOpsUserKey', () => resetOpsUserKey);

        // OPS 日志相关
        __exposeFn('refreshOpsLogs', () => refreshOpsLogs);
        __exposeFn('filterOpsLogs', () => filterOpsLogs);

        // Web Search Keys 相关
        __exposeFn('refreshWebSearchKeys', () => refreshWebSearchKeys);
        __exposeFn('showAddWebSearchKeyModal', () => showAddWebSearchKeyModal);
        __exposeFn('closeAddWebSearchKeyModal', () => closeAddWebSearchKeyModal);
        __exposeFn('submitAddWebSearchKey', () => submitAddWebSearchKey);
        __exposeFn('toggleWebSearchKeyStatus', () => toggleWebSearchKeyStatus);
        __exposeFn('resetWebSearchKeyHealth', () => resetWebSearchKeyHealth);
        __exposeFn('deleteWebSearchKey', () => deleteWebSearchKey);
        __exposeFn('testWebSearchKey', () => testWebSearchKey);
        __exposeFn('saveWebSearchConfig', () => saveWebSearchConfig);
        __exposeFn('loadWebSearchConfig', () => loadWebSearchConfig);

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

        // ==================== Rate Limit Config ====================

        // 自定义 429 文案：后端 custom_429_kind_defaults（各 limit_kind 的默认文案/中文名/触发原因）缓存
        // 关键词: custom429KindDefaults, custom_429_kind_defaults 缓存
        let custom429KindDefaults = [];

        // 渲染「自定义 429 文案」各 limit_kind 编辑器：展示中文名/触发原因/默认文案，并提供可编辑覆盖框
        // 关键词: renderCustom429Kinds, 每个 kind 可编辑 + 编辑时可见默认文案
        function renderCustom429Kinds(defaults, overrides) {
            const container = document.getElementById('rl-custom429-kinds-list');
            if (!container) return;
            overrides = overrides || {};
            if (!Array.isArray(defaults) || defaults.length === 0) {
                container.innerHTML = '<small style="color:#999;">无可配置的限流类型</small>';
                return;
            }
            container.innerHTML = defaults.map(function (meta) {
                const kind = meta.kind || '';
                const labelZh = meta.label_zh || kind;
                const type = meta.type || '';
                const def = meta.default_message || '';
                const desc = meta.description || '';
                const dynamicBadge = meta.dynamic
                    ? '<span style="background:#ff9800;color:#fff;padding:1px 6px;border-radius:8px;font-size:11px;margin-left:6px;" title="动态类型：实际返回会在默认文案后追加运行时数值（如排队位置 / 已用量）">动态</span>'
                    : '';
                const ov = overrides[kind] != null ? overrides[kind] : '';
                return ''
                    + '<div class="form-group" style="margin-bottom:0;border:1px solid #eee;border-radius:6px;padding:10px;background:#fafafa;">'
                    +   '<div style="display:flex;align-items:center;justify-content:space-between;gap:8px;flex-wrap:wrap;">'
                    +     '<label style="margin:0;font-weight:600;">' + escapeHtml(labelZh)
                    +       ' <small style="color:#888;font-weight:normal;">(' + escapeHtml(kind) + (type ? ' &middot; ' + escapeHtml(type) : '') + ')</small>'
                    +       dynamicBadge
                    +     '</label>'
                    +     '<button type="button" class="btn btn-sm" style="font-size:11px;padding:2px 8px;" onclick="applyCustom429Default(\'' + escapeHtml(kind) + '\')" title="将默认文案填入下方编辑框">套用默认</button>'
                    +   '</div>'
                    +   (desc ? '<div style="color:#999;font-size:12px;margin:4px 0;">触发原因：' + escapeHtml(desc) + '</div>' : '')
                    +   '<div style="color:#555;font-size:12px;margin:4px 0;"><span style="color:#1976d2;">默认文案：</span>' + escapeHtml(def) + '</div>'
                    +   '<textarea id="rl-custom429-kind-' + escapeHtml(kind) + '" class="form-control" rows="2" style="font-size:13px;" placeholder="' + escapeHtml(def) + '">' + escapeHtml(ov) + '</textarea>'
                    + '</div>';
            }).join('');
        }

        // 套用默认：将某个 limit_kind 的默认文案填入对应编辑框（方便在默认基础上微调）
        // 关键词: applyCustom429Default, 套用默认文案
        function applyCustom429Default(kind) {
            const meta = (custom429KindDefaults || []).find(function (m) { return m.kind === kind; });
            if (!meta) return;
            const el = document.getElementById('rl-custom429-kind-' + kind);
            if (el) el.value = meta.default_message || '';
        }

        async function loadRateLimitConfig() {
            try {
                const response = await fetch('/portal/api/rate-limit-config');
                if (!response.ok) throw new Error('Failed to fetch rate limit config');
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                if (data.success && data.config) {
                    const cfg = data.config;
                    const rpmInput = document.getElementById('rl-default-rpm');
                    if (rpmInput) rpmInput.value = cfg.default_rpm || 600;
                    const delayInput = document.getElementById('rl-free-user-delay');
                    if (delayInput) delayInput.value = cfg.free_user_delay_sec || 0;
                    const delayMaxInput = document.getElementById('rl-free-user-delay-max');
                    if (delayMaxInput) delayMaxInput.value = cfg.free_user_delay_max_sec || 0;
                    const freeOutputTPSInput = document.getElementById('rl-free-output-tps');
                    if (freeOutputTPSInput) freeOutputTPSInput.value = cfg.free_user_output_tps || 0;
                    renderModelRPMOverrides(
                        cfg.model_rpm_overrides || {},
                        cfg.model_delay_overrides || {},
                        cfg.model_output_tps_overrides || {}
                    );

                    // 免费用户 Token 日限额：全局 + 模型级覆盖
                    // 关键词: loadRateLimitConfig free_user_token_limit_m
                    const tokLimitInput = document.getElementById('rl-free-token-limit-m-input');
                    if (tokLimitInput) tokLimitInput.value = (cfg.free_user_token_limit_m == null ? 1200 : cfg.free_user_token_limit_m);
                    renderFreeTokenModelOverrides(cfg.free_user_token_model_overrides || {});

                    // 付费用户全局日 Token 总额度（第二道硬门），0=不限制
                    // 关键词: loadRateLimitConfig paid_user_token_limit_m
                    const paidTokLimitInput = document.getElementById('rl-paid-token-limit-m-input');
                    if (paidTokLimitInput) paidTokLimitInput.value = (cfg.paid_user_token_limit_m == null ? 0 : cfg.paid_user_token_limit_m);

                    // 刷新 1 RMB=10M 计费 Token 换算提示
                    updateRMBHint('rl-free-token-limit-m-input', 'rl-free-token-limit-rmb');
                    updateRMBHint('rl-paid-token-limit-m-input', 'rl-paid-token-limit-rmb');

                    // 软限额阈值 / 软限额 TPS
                    // 关键词: loadRateLimitConfig free_user_token_soft_limit_m
                    const softLimitMInput = document.getElementById('rl-free-token-soft-limit-m');
                    if (softLimitMInput) softLimitMInput.value = cfg.free_user_token_soft_limit_m || 0;
                    const softLimitTPSInput = document.getElementById('rl-free-soft-limit-tps');
                    if (softLimitTPSInput) softLimitTPSInput.value = cfg.free_user_soft_limit_tps || 0;

                    // memfit-* 客户端版本控流配置
                    // 关键词: loadRateLimitConfig memfit_version_gate_enabled, memfit_version_min_build_time
                    const gateEl = document.getElementById('rl-memfit-version-gate-enabled');
                    if (gateEl) gateEl.checked = !!cfg.memfit_version_gate_enabled;
                    const minBtEl = document.getElementById('rl-memfit-version-min-build-time');
                    if (minBtEl) minBtEl.value = cfg.memfit_version_min_build_time || '';

                    // 自定义 429/错误文案配置：按后端默认列表动态渲染各 kind 编辑器，并回填已保存覆盖
                    // 关键词: loadRateLimitConfig custom_429_enabled, custom_429_notice, custom_429_kind_defaults
                    const c429EnabledEl = document.getElementById('rl-custom429-enabled');
                    if (c429EnabledEl) c429EnabledEl.checked = !!cfg.custom_429_enabled;
                    const c429NoticeEl = document.getElementById('rl-custom429-notice');
                    if (c429NoticeEl) c429NoticeEl.value = cfg.custom_429_notice || '';
                    custom429KindDefaults = Array.isArray(cfg.custom_429_kind_defaults) ? cfg.custom_429_kind_defaults : [];
                    renderCustom429Kinds(custom429KindDefaults, cfg.custom_429_kind_overrides || {});

                    // 轻量降级规则
                    // 关键词: loadRateLimitConfig model_downgrade_rules
                    renderModelDowngradeRules(cfg.model_downgrade_rules || []);

                    // 单 IP 免费模型每日用量限额
                    // 关键词: loadRateLimitConfig free_user_ip_limit
                    const ipEnabledEl = document.getElementById('rl-free-ip-limit-enabled');
                    if (ipEnabledEl) ipEnabledEl.checked = !!cfg.free_user_ip_limit_enable;
                    const ipReqLimitEl = document.getElementById('rl-free-ip-daily-request-limit');
                    if (ipReqLimitEl) ipReqLimitEl.value = (cfg.free_user_ip_daily_request_limit == null ? 0 : cfg.free_user_ip_daily_request_limit);
                    const ipTokLimitEl = document.getElementById('rl-free-ip-daily-token-limit-m');
                    if (ipTokLimitEl) ipTokLimitEl.value = (cfg.free_user_ip_daily_token_limit_m == null ? 0 : cfg.free_user_ip_daily_token_limit_m);

                    // 刷新免费额度相关「金额限制」换算提示（单 IP 每日 Token 上限 / 软限额阈值）
                    // 关键词: loadRateLimitConfig 金额限制 RMB 提示, 单 IP Token 上限, 软限额阈值
                    updateRMBHint('rl-free-ip-daily-token-limit-m', 'rl-free-ip-daily-token-limit-rmb');
                    updateRMBHint('rl-free-token-soft-limit-m', 'rl-free-token-soft-limit-rmb');

                    // 一键限流 IP 默认参数（RPM / 输出 TPS）
                    // 关键词: loadRateLimitConfig throttled_ip_default_rpm/tps
                    const thrRpmEl = document.getElementById('rl-throttled-ip-default-rpm');
                    if (thrRpmEl) thrRpmEl.value = (cfg.throttled_ip_default_rpm == null ? 3 : cfg.throttled_ip_default_rpm);
                    const thrTpsEl = document.getElementById('rl-throttled-ip-default-tps');
                    if (thrTpsEl) thrTpsEl.value = (cfg.throttled_ip_default_tps == null ? 15 : cfg.throttled_ip_default_tps);
                }
            } catch (error) {
                console.error('Error loading rate limit config:', error);
            }
        }

        async function saveRateLimitConfig() {
            const defaultRPM = parseInt(document.getElementById('rl-default-rpm').value) || 600;
            const freeDelay = parseInt(document.getElementById('rl-free-user-delay').value) || 0;
            const freeDelayMaxRaw = document.getElementById('rl-free-user-delay-max').value;
            let freeDelayMax = parseInt(freeDelayMaxRaw);
            if (isNaN(freeDelayMax) || freeDelayMax < 0) freeDelayMax = 0;
            const freeOutputTPSRaw = document.getElementById('rl-free-output-tps').value;
            let freeOutputTPS = parseInt(freeOutputTPSRaw);
            if (isNaN(freeOutputTPS) || freeOutputTPS < 0) freeOutputTPS = 0;
            const collected = collectModelRPMOverrides();

            // 免费 Token 限额相关字段
            // 关键词: saveRateLimitConfig free_user_token_limit_m, model overrides
            const tokLimitInputEl = document.getElementById('rl-free-token-limit-m-input');
            let freeTokenLimitM = parseInt(tokLimitInputEl ? tokLimitInputEl.value : '');
            if (isNaN(freeTokenLimitM) || freeTokenLimitM < 0) freeTokenLimitM = 1200;
            const freeTokenOverrides = collectFreeTokenModelOverrides();

            // 付费用户全局日 Token 总额度（第二道硬门），0=不限制
            // 关键词: saveRateLimitConfig paid_user_token_limit_m
            const paidTokLimitInputEl = document.getElementById('rl-paid-token-limit-m-input');
            let paidTokenLimitM = parseInt(paidTokLimitInputEl ? paidTokLimitInputEl.value : '');
            if (isNaN(paidTokenLimitM) || paidTokenLimitM < 0) paidTokenLimitM = 0;

            // 软限额相关字段
            // 关键词: saveRateLimitConfig free_user_token_soft_limit_m
            const softLimitMRaw = document.getElementById('rl-free-token-soft-limit-m').value;
            let softLimitM = parseInt(softLimitMRaw);
            if (isNaN(softLimitM) || softLimitM < 0) softLimitM = 0;
            const softLimitTPSRaw = document.getElementById('rl-free-soft-limit-tps').value;
            let softLimitTPS = parseInt(softLimitTPSRaw);
            if (isNaN(softLimitTPS) || softLimitTPS < 0) softLimitTPS = 0;

            // memfit-* 客户端版本控流配置
            // 关键词: saveRateLimitConfig memfit_version_gate_enabled, memfit_version_min_build_time
            const gateEl = document.getElementById('rl-memfit-version-gate-enabled');
            const minBtEl = document.getElementById('rl-memfit-version-min-build-time');
            const memfitGateEnabled = !!(gateEl && gateEl.checked);
            const memfitMinBuildTime = minBtEl ? (minBtEl.value || '').trim() : '';

            // 自定义 429/错误文案配置
            // 关键词: saveRateLimitConfig custom_429_enabled, custom_429_notice, custom_429_kind_overrides
            const c429EnabledEl = document.getElementById('rl-custom429-enabled');
            const custom429Enabled = !!(c429EnabledEl && c429EnabledEl.checked);
            const c429NoticeEl = document.getElementById('rl-custom429-notice');
            const custom429Notice = c429NoticeEl ? (c429NoticeEl.value || '').trim() : '';
            const custom429KindOverrides = {};
            (custom429KindDefaults || []).forEach(function (meta) {
                const el = document.getElementById('rl-custom429-kind-' + meta.kind);
                if (el) {
                    const v = (el.value || '').trim();
                    if (v !== '') custom429KindOverrides[meta.kind] = v;
                }
            });

            // 轻量降级规则（空列表表示显式关闭降级）
            // 关键词: saveRateLimitConfig model_downgrade_rules
            const modelDowngradeRules = collectModelDowngradeRules();

            // 单 IP 免费模型每日用量限额
            // 关键词: saveRateLimitConfig free_user_ip_limit
            const ipEnabledEl = document.getElementById('rl-free-ip-limit-enabled');
            const freeIPLimitEnable = !!(ipEnabledEl && ipEnabledEl.checked);
            const ipReqLimitRaw = document.getElementById('rl-free-ip-daily-request-limit');
            let freeIPDailyRequestLimit = parseInt(ipReqLimitRaw ? ipReqLimitRaw.value : '');
            if (isNaN(freeIPDailyRequestLimit) || freeIPDailyRequestLimit < 0) freeIPDailyRequestLimit = 0;
            const ipTokLimitRaw = document.getElementById('rl-free-ip-daily-token-limit-m');
            let freeIPDailyTokenLimitM = parseInt(ipTokLimitRaw ? ipTokLimitRaw.value : '');
            if (isNaN(freeIPDailyTokenLimitM) || freeIPDailyTokenLimitM < 0) freeIPDailyTokenLimitM = 0;

            // 一键限流 IP 默认参数（RPM / 输出 TPS），<=0 由后端按 3/15 兜底
            // 关键词: saveRateLimitConfig throttled_ip_default_rpm/tps
            const thrRpmRaw = document.getElementById('rl-throttled-ip-default-rpm');
            let throttledIPDefaultRPM = parseInt(thrRpmRaw ? thrRpmRaw.value : '');
            if (isNaN(throttledIPDefaultRPM) || throttledIPDefaultRPM < 0) throttledIPDefaultRPM = 0;
            const thrTpsRaw = document.getElementById('rl-throttled-ip-default-tps');
            let throttledIPDefaultTPS = parseInt(thrTpsRaw ? thrTpsRaw.value : '');
            if (isNaN(throttledIPDefaultTPS) || throttledIPDefaultTPS < 0) throttledIPDefaultTPS = 0;

            try {
                const response = await fetch('/portal/api/rate-limit-config', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        default_rpm: defaultRPM,
                        free_user_delay_sec: freeDelay,
                        free_user_delay_max_sec: freeDelayMax,
                        model_rpm_overrides: collected.rpm,
                        model_delay_overrides: collected.delay,
                        free_user_token_limit_m: freeTokenLimitM,
                        free_user_token_model_overrides: freeTokenOverrides,
                        paid_user_token_limit_m: paidTokenLimitM,
                        free_user_output_tps: freeOutputTPS,
                        model_output_tps_overrides: collected.tps,
                        free_user_token_soft_limit_m: softLimitM,
                        free_user_soft_limit_tps: softLimitTPS,
                        memfit_version_gate_enabled: memfitGateEnabled,
                        memfit_version_min_build_time: memfitMinBuildTime,
                        custom_429_enabled: custom429Enabled,
                        custom_429_notice: custom429Notice,
                        custom_429_kind_overrides: custom429KindOverrides,
                        model_downgrade_rules: modelDowngradeRules,
                        free_user_ip_limit_enable: freeIPLimitEnable,
                        free_user_ip_daily_request_limit: freeIPDailyRequestLimit,
                        free_user_ip_daily_token_limit_m: freeIPDailyTokenLimitM,
                        throttled_ip_default_rpm: throttledIPDefaultRPM,
                        throttled_ip_default_tps: throttledIPDefaultTPS
                    })
                });
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                if (data.success) {
                    showToast('限流配置已保存', 'success');
                    loadRateLimitStatus();
                } else {
                    showToast(data.message || '保存失败', 'error');
                }
            } catch (error) {
                console.error('Error saving rate limit config:', error);
                showToast('保存限流配置失败', 'error');
            }
        }

        async function loadRateLimitStatus() {
            try {
                const response = await fetch('/portal/api/rate-limit-status');
                if (!response.ok) throw new Error('Failed to fetch rate limit status');
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                if (data.success) {
                    const queueEl = document.getElementById('rl-queue-count');
                    if (queueEl) queueEl.textContent = data.queue_count || 0;
                    const rpmEl = document.getElementById('rl-effective-rpm');
                    if (rpmEl) rpmEl.textContent = data.default_rpm || '--';

                    // 免费用户 Token 用量快照
                    // 关键词: loadRateLimitStatus free_user_token_usage 实时显示
                    const usage = data.free_user_token_usage || {};
                    const global = usage.global || {};
                    const usedMText = (typeof global.used_m === 'number') ? global.used_m.toFixed(2) : '--';
                    const limitMText = (typeof global.limit_m === 'number') ? String(global.limit_m) : '--';
                    const topUsedEl = document.getElementById('rl-free-token-used-m');
                    if (topUsedEl) topUsedEl.textContent = usedMText;
                    const topLimitEl = document.getElementById('rl-free-token-limit-m');
                    if (topLimitEl) topLimitEl.textContent = limitMText;
                    const resetEl = document.getElementById('rl-free-token-reset-date');
                    if (resetEl) resetEl.textContent = usage.reset_date || '--';
                    const blockUsedEl = document.getElementById('rl-free-token-global-used');
                    if (blockUsedEl) blockUsedEl.textContent = usedMText;
                    const blockLimitEl = document.getElementById('rl-free-token-global-limit');
                    if (blockLimitEl) blockLimitEl.textContent = limitMText;

                    // 付费用户全局日 Token 总额度快照（第二道硬门）
                    // 关键词: loadRateLimitStatus paid_user_token_usage 实时显示
                    const paidUsage = data.paid_user_token_usage || {};
                    const paidUsedMText = (typeof paidUsage.used_m === 'number') ? paidUsage.used_m.toFixed(2) : '--';
                    const paidLimitMText = (typeof paidUsage.limit_m === 'number')
                        ? (paidUsage.limit_m > 0 ? String(paidUsage.limit_m) : '不限制')
                        : '--';
                    const paidUsedEl = document.getElementById('rl-paid-token-global-used');
                    if (paidUsedEl) paidUsedEl.textContent = paidUsedMText;
                    const paidLimitEl = document.getElementById('rl-paid-token-global-limit');
                    if (paidLimitEl) paidLimitEl.textContent = paidLimitMText;

                    // 单 IP 免费模型用量快照（多少人在用 + Top IP 榜）
                    // 关键词: loadRateLimitStatus free_ip_usage 渲染
                    renderFreeIPUsage(data.free_ip_usage || {});
                }
            } catch (error) {
                console.error('Error loading rate limit status:', error);
            }
            // Clicking "刷新状态" should refresh hot-model stats as well.
            loadRateLimitModelStats();
            // 同步刷新客户端版本统计（memfit 版本控流）
            // 关键词: loadRateLimitStatus 关联刷新 loadClientVersionStats
            loadClientVersionStats();
        }

        // renderFreeIPUsage 渲染「今日免费 IP 用量」面板：多少 IP 在用 + Top 榜（仅 >10M）+ 一键限流。
        // 关键词: renderFreeIPUsage, 单 IP 免费用量, 防盗刷面板, 一键限流
        function renderFreeIPUsage(usage) {
            const countEl = document.getElementById('rl-free-ip-distinct-count');
            if (countEl) countEl.textContent = (typeof usage.distinct_ip_count === 'number') ? usage.distinct_ip_count : '--';
            const dateEl = document.getElementById('rl-free-ip-reset-date');
            if (dateEl) dateEl.textContent = usage.reset_date || '--';

            // 已限流 IP 列表（独立于今日用量榜，可随时解除）
            renderThrottledIPList(Array.isArray(usage.throttled_ips) ? usage.throttled_ips : []);

            const tbody = document.getElementById('rl-free-ip-usage-tbody');
            if (!tbody) return;
            const top = Array.isArray(usage.top) ? usage.top : [];
            if (top.length === 0) {
                tbody.innerHTML = '<tr><td colspan="5" style="padding: 12px; text-align: center; color: #999;">今日暂无加权 Token 超过 10M 的免费 IP</td></tr>';
                bindFreeIPActionButtons();
                return;
            }
            tbody.innerHTML = top.map(it => {
                const ip = escapeHtml(it.ip || '');
                const req = Number(it.request_count) || 0;
                const usedMNum = (typeof it.used_m === 'number') ? it.used_m : 0;
                const usedM = usedMNum.toFixed(3);
                // 加权 Token 折算 RMB：1 RMB = 10M 计费 Token（BILLING_TOKEN_M_PER_RMB）。
                const rmb = '¥' + (usedMNum / BILLING_TOKEN_M_PER_RMB).toFixed(2);
                const throttled = !!it.throttled;
                const btn = throttled
                    ? '<button class="rl-ip-unthrottle-btn btn" data-ip="' + ip + '" style="height:26px; font-size:12px; background:#c62828; color:#fff;">解除</button>'
                    : '<button class="rl-ip-throttle-btn btn" data-ip="' + ip + '" style="height:26px; font-size:12px; background:#ef6c00; color:#fff;">限流</button>';
                const tag = throttled ? ' <span style="color:#c62828; font-size:11px;">(已限流)</span>' : '';
                // 该 IP 用得最多的 TOP3 模型（小字子行）。
                // 数量(M) 用原始 Token（used_m，含不计费模型）；金额(¥) 用加权 Token（weighted_m）折算，
                // 不计费模型 weighted_m=0 -> ¥0.00（计数量、不算钱）。
                // 关键词: renderFreeIPUsage top_models, per-IP TOP3 模型子行, 数量 vs 金额
                const models = Array.isArray(it.top_models) ? it.top_models : [];
                let modelsRow = '';
                if (models.length > 0) {
                    const chips = models.map(function (m, mi) {
                        const name = escapeHtml(m.model || '');
                        const mm = (typeof m.used_m === 'number') ? m.used_m : 0;
                        const mReq = Number(m.request_count) || 0;
                        // 金额按加权 Token 折算；旧数据无 weighted_m 时回退 0（避免把数量误当金额）。
                        const wM = (typeof m.weighted_m === 'number') ? m.weighted_m : 0;
                        const mRmb = '¥' + (wM / BILLING_TOKEN_M_PER_RMB).toFixed(2);
                        const free = wM <= 0;
                        const rank = 'TOP' + (mi + 1);
                        return '<span style="display:inline-block; margin:2px 6px 2px 0; padding:1px 6px; background:#f1f8ff; border:1px solid #cfe3ff; border-radius:10px; color:#37474f;">'
                            + '<span style="color:#90a4ae;">' + rank + ' </span>'
                            + '<code style="color:#1565c0;">' + name + '</code> · ' + mm.toFixed(2) + 'M · ' + mReq + '次 · '
                            + '<span style="color:' + (free ? '#2e7d32' : '#1565c0') + ';">' + mRmb + (free ? ' 不计费' : '') + '</span>'
                            + '</span>';
                    }).join('');
                    modelsRow = '<tr><td colspan="5" style="padding: 0 10px 8px 22px; border-bottom: 1px solid #e1f5fe; font-size: 11px; color:#789;">'
                        + '<span style="color:#90a4ae;">TOP 模型(数量): </span>' + chips
                        + '</td></tr>';
                }
                return '<tr>'
                    + '<td style="padding: 6px 10px; border-bottom: 1px solid #e1f5fe;"><code>' + ip + '</code>' + tag + '</td>'
                    + '<td style="padding: 6px 10px; border-bottom: 1px solid #e1f5fe; text-align: right;">' + req + '</td>'
                    + '<td style="padding: 6px 10px; border-bottom: 1px solid #e1f5fe; text-align: right;">' + usedM + '</td>'
                    + '<td style="padding: 6px 10px; border-bottom: 1px solid #e1f5fe; text-align: right; color:#1565c0;">' + rmb + '</td>'
                    + '<td style="padding: 6px 10px; border-bottom: 1px solid #e1f5fe; text-align: center;">' + btn + '</td>'
                    + '</tr>'
                    + modelsRow;
            }).join('');
            bindFreeIPActionButtons();
        }

        // renderThrottledIPList 渲染「已限流 IP」列表（带 RPM/TPS 与解除按钮）；空列表时隐藏整块。
        // 关键词: renderThrottledIPList, 已限流 IP 列表, 解除限流
        function renderThrottledIPList(list) {
            const wrap = document.getElementById('rl-throttled-ip-wrap');
            const listEl = document.getElementById('rl-throttled-ip-list');
            const countEl = document.getElementById('rl-throttled-ip-count');
            if (!wrap || !listEl) return;
            if (!list.length) {
                wrap.style.display = 'none';
                listEl.innerHTML = '';
                if (countEl) countEl.textContent = '0';
                return;
            }
            wrap.style.display = 'block';
            if (countEl) countEl.textContent = String(list.length);
            listEl.innerHTML = list.map(it => {
                const ip = escapeHtml(it.ip || '');
                const rpm = Number(it.rpm) || 0;
                const tps = Number(it.tps) || 0;
                const reason = it.reason ? (' · ' + escapeHtml(it.reason)) : '';
                return '<div style="display:flex; align-items:center; gap:10px; font-size:12px; background:#ffebee; border:1px solid #ffcdd2; border-radius:4px; padding:4px 8px;">'
                    + '<code style="flex:0 0 auto;">' + ip + '</code>'
                    + '<span style="color:#555;">RPM ' + rpm + ' · TPS ' + tps + reason + '</span>'
                    + '<button class="rl-ip-unthrottle-btn btn" data-ip="' + ip + '" style="margin-left:auto; height:24px; font-size:11px; background:#c62828; color:#fff;">解除</button>'
                    + '</div>';
            }).join('');
            bindFreeIPActionButtons();
        }

        // bindFreeIPActionButtons 给「限流 / 解除」按钮绑定点击事件（用 onclick 赋值，幂等可重复调用）。
        // 关键词: bindFreeIPActionButtons, 一键限流按钮绑定
        function bindFreeIPActionButtons() {
            document.querySelectorAll('.rl-ip-throttle-btn').forEach(function (b) {
                b.onclick = function () { throttleIP(this.getAttribute('data-ip')); };
            });
            document.querySelectorAll('.rl-ip-unthrottle-btn').forEach(function (b) {
                b.onclick = function () { unthrottleIP(this.getAttribute('data-ip')); };
            });
        }

        // throttleIP 一键限流某 IP：后端按配置默认 RPM/TPS 套用。
        // 关键词: throttleIP, 一键限流 IP 请求
        async function throttleIP(ip) {
            if (!ip) return;
            if (!confirm('确定要限流 IP ' + ip + ' 吗？\n该 IP 的请求频率(RPM)与输出速率(TPS)将被压到配置的默认值，且持久保留直到手动解除。')) return;
            try {
                const response = await fetch('/portal/api/throttle-ip', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ ip: ip })
                });
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                if (data.success) {
                    showToast('已限流 IP ' + ip + '（RPM ' + data.rpm + ' / TPS ' + data.tps + '）', 'success');
                    loadRateLimitStatus();
                } else {
                    showToast(data.error || '限流失败', 'error');
                }
            } catch (e) {
                console.error('throttleIP failed:', e);
                showToast('限流请求失败', 'error');
            }
        }

        // unthrottleIP 解除某 IP 的限流。
        // 关键词: unthrottleIP, 解除限流请求
        async function unthrottleIP(ip) {
            if (!ip) return;
            if (!confirm('确定要解除对 IP ' + ip + ' 的限流吗？')) return;
            try {
                const response = await fetch('/portal/api/unthrottle-ip', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ ip: ip })
                });
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                if (data.success) {
                    showToast('已解除 IP ' + ip + ' 的限流', 'success');
                    loadRateLimitStatus();
                } else {
                    showToast(data.error || '解除失败', 'error');
                }
            } catch (e) {
                console.error('unthrottleIP failed:', e);
                showToast('解除限流请求失败', 'error');
            }
        }

        // ==================== Memfit Client Version Stats ====================
        // 关键词: loadClientVersionStats memfit 客户端版本 Top20 渲染
        function escapeHtml(s) {
            if (s == null) return '';
            return String(s)
                .replace(/&/g, '&amp;')
                .replace(/</g, '&lt;')
                .replace(/>/g, '&gt;')
                .replace(/"/g, '&quot;')
                .replace(/'/g, '&#39;');
        }

        async function loadClientVersionStats() {
            const tbody = document.getElementById('rl-client-version-table-body');
            if (!tbody) return;
            try {
                const response = await fetch('/portal/api/client-version-stats?limit=20');
                if (!response.ok) throw new Error('Failed to fetch client version stats');
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                if (!data.success) {
                    tbody.innerHTML = '<tr><td colspan="5" style="padding: 12px; text-align: center; color: #c62828;">加载失败</td></tr>';
                    return;
                }
                const totalEl = document.getElementById('rl-client-version-total');
                if (totalEl) totalEl.textContent = '共 ' + (data.total || 0) + ' 条';
                const items = Array.isArray(data.items) ? data.items : [];
                if (items.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="5" style="padding: 12px; text-align: center; color: #999;">暂无 memfit-* 客户端版本上报记录</td></tr>';
                    return;
                }
                tbody.innerHTML = items.map(it => {
                    const ver = escapeHtml(it.version || '');
                    const bt = escapeHtml(it.build_time || '');
                    const fs = escapeHtml(it.first_seen_text || '');
                    const ls = escapeHtml(it.last_seen_text || '');
                    const cnt = Number(it.request_count) || 0;
                    return '<tr>'
                        + '<td style="padding: 6px 10px; border-bottom: 1px solid #f0f0f0;"><code>' + ver + '</code></td>'
                        + '<td style="padding: 6px 10px; border-bottom: 1px solid #f0f0f0; font-family: monospace; color: #555;">' + (bt || '-') + '</td>'
                        + '<td style="padding: 6px 10px; border-bottom: 1px solid #f0f0f0;">' + fs + '</td>'
                        + '<td style="padding: 6px 10px; border-bottom: 1px solid #f0f0f0;">' + ls + '</td>'
                        + '<td style="padding: 6px 10px; border-bottom: 1px solid #f0f0f0; text-align: right;">' + cnt + '</td>'
                        + '</tr>';
                }).join('');
            } catch (error) {
                console.error('Error loading client version stats:', error);
                tbody.innerHTML = '<tr><td colspan="5" style="padding: 12px; text-align: center; color: #c62828;">加载失败</td></tr>';
            }
        }

        // clearClientVersionStats 二次确认后调用后端清空接口, 成功后刷新表格.
        // 关键词: clearClientVersionStats, portal 清空客户端版本记录前端
        async function clearClientVersionStats() {
            if (!confirm('确认清空所有客户端版本记录？此操作不可恢复（数据会被硬删除）。')) return;
            try {
                const response = await fetch('/portal/api/client-version-stats/clear', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'}
                });
                if (!response.ok) {
                    alert('清空失败: HTTP ' + response.status);
                    return;
                }
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                if (!data.success) {
                    alert('清空失败: ' + (data.error || '未知错误'));
                    return;
                }
                alert('已清空 ' + (data.removed || 0) + ' 条客户端版本记录');
                loadClientVersionStats();
            } catch (error) {
                console.error('clearClientVersionStats failed:', error);
                alert('清空失败: ' + error);
            }
        }

        // ===== Hot-model RPM stats (cross-apiKey aggregated, recent 60s) =====
        // NOTE: use `var` and a shared threshold helper instead of a
        // top-level `const` to avoid any TDZ risk (observed in production
        // when other init paths reach these functions before the declaration
        // line is executed, e.g. via cached page state).
        var rateLimitModelStatsTimer = null;
        function getRateLimitModelMinRPM() { return 3; }

        async function loadRateLimitModelStats() {
            const tbody = document.getElementById('rl-model-stats-tbody');
            if (!tbody) return;
            const minRPM = getRateLimitModelMinRPM();
            try {
                const response = await fetch('/portal/api/rate-limit-model-stats?min_rpm=' + minRPM);
                if (!response.ok) throw new Error('Failed to fetch model RPM stats');
                const data = await response.json();
                if (isAuthError(data)) { handleAuthError(); return; }
                if (!data.success) {
                    tbody.innerHTML = '<tr><td colspan="4" style="padding: 12px; text-align: center; color: #c62828;">加载失败</td></tr>';
                    return;
                }
                const windowEl = document.getElementById('rl-model-window');
                if (windowEl && data.window_seconds) windowEl.textContent = data.window_seconds;
                const minEl = document.getElementById('rl-model-min-rpm');
                if (minEl && (data.min_rpm || data.min_rpm === 0)) minEl.textContent = data.min_rpm;

                const models = Array.isArray(data.models) ? data.models : [];
                if (models.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="4" style="padding: 12px; text-align: center; color: #999;">当前没有模型 RPM ≥ ' + (data.min_rpm || minRPM) + '</td></tr>';
                } else {
                    tbody.innerHTML = models.map(m => {
                        const effective = Number(m.effective_rpm) || 0;
                        const rpm = Number(m.rpm) || 0;
                        let ratioText = '--';
                        let color = '#555';
                        if (effective > 0) {
                            const ratio = rpm / effective;
                            ratioText = (ratio * 100).toFixed(1) + '%';
                            if (ratio >= 0.9) color = '#c62828';
                            else if (ratio >= 0.6) color = '#ef6c00';
                            else color = '#2e7d32';
                        }
                        const modelName = String(m.model == null ? '' : m.model)
                            .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
                        return '<tr>' +
                            '<td style="padding: 8px 10px; border-bottom: 1px solid #f0e4c0; font-family: monospace;">' + modelName + '</td>' +
                            '<td style="padding: 8px 10px; border-bottom: 1px solid #f0e4c0; text-align: right; font-family: monospace;"><strong>' + rpm + '</strong></td>' +
                            '<td style="padding: 8px 10px; border-bottom: 1px solid #f0e4c0; text-align: right; font-family: monospace;">' + (effective > 0 ? effective : '--') + '</td>' +
                            '<td style="padding: 8px 10px; border-bottom: 1px solid #f0e4c0; text-align: right; font-family: monospace; color: ' + color + ';">' + ratioText + '</td>' +
                            '</tr>';
                    }).join('');
                }

                const updatedEl = document.getElementById('rl-model-updated-at');
                if (updatedEl) {
                    const now = new Date();
                    const pad = n => String(n).padStart(2, '0');
                    updatedEl.textContent = '更新于 ' + pad(now.getHours()) + ':' + pad(now.getMinutes()) + ':' + pad(now.getSeconds());
                }
            } catch (error) {
                console.error('Error loading model RPM stats:', error);
                if (tbody) {
                    tbody.innerHTML = '<tr><td colspan="4" style="padding: 12px; text-align: center; color: #c62828;">加载异常: ' + (error.message || 'unknown') + '</td></tr>';
                }
            }
        }

        // Start / stop the 10s auto-refresh based on whether the rate-limit
        // tab is the active tab. We also stop when the page is hidden so
        // background tabs don't waste bandwidth.
        function startRateLimitModelStatsAutoRefresh() {
            stopRateLimitModelStatsAutoRefresh();
            loadRateLimitModelStats();
            rateLimitModelStatsTimer = setInterval(() => {
                if (document.hidden) return;
                loadRateLimitModelStats();
            }, 10000);
        }

        function stopRateLimitModelStatsAutoRefresh() {
            if (rateLimitModelStatsTimer) {
                clearInterval(rateLimitModelStatsTimer);
                rateLimitModelStatsTimer = null;
            }
        }

        // 关键词: renderModelRPMOverrides RPM + 延迟区间 + TPS, 老数据兼容
        function renderModelRPMOverrides(rpmOverrides, delayOverrides, tpsOverrides) {
            const container = document.getElementById('rl-model-overrides-list');
            if (!container) return;
            container.innerHTML = '';
            rpmOverrides = rpmOverrides || {};
            delayOverrides = delayOverrides || {};
            tpsOverrides = tpsOverrides || {};
            const modelSet = new Set([
                ...Object.keys(rpmOverrides),
                ...Object.keys(delayOverrides),
                ...Object.keys(tpsOverrides)
            ]);
            if (modelSet.size === 0) {
                container.innerHTML = '<p style="color: #999; font-size: 13px;">暂无模型级覆盖配置。</p>';
                return;
            }
            Array.from(modelSet).sort().forEach((model, idx) => {
                const rpm = rpmOverrides[model];
                // 兼容老数据：delayOverrides[m] 可能是数字或 {min,max} 对象。
                let delayMin = '';
                let delayMax = '';
                const raw = delayOverrides[model];
                if (raw !== undefined && raw !== null) {
                    if (typeof raw === 'number') {
                        delayMin = raw;
                        delayMax = 0;
                    } else if (typeof raw === 'object') {
                        if (raw.min !== undefined && raw.min !== null) delayMin = raw.min;
                        if (raw.max !== undefined && raw.max !== null) delayMax = raw.max;
                    }
                }
                const tps = tpsOverrides[model];
                appendModelRPMRow(container, model, rpm, delayMin, delayMax, tps, idx);
            });
        }

        // 关键词: appendModelRPMRow 5 列布局, 模型/RPM/延迟Min/延迟Max/TPS
        function appendModelRPMRow(container, model, rpm, delayMin, delayMax, tps, idx) {
            const row = document.createElement('div');
            row.style.cssText = 'display: flex; gap: 8px; align-items: center; margin-bottom: 8px; flex-wrap: wrap;';
            row.className = 'rl-model-row';
            const rpmVal = (rpm === undefined || rpm === null || rpm === '') ? '' : rpm;
            const delayMinVal = (delayMin === undefined || delayMin === null || delayMin === '') ? '' : delayMin;
            const delayMaxVal = (delayMax === undefined || delayMax === null || delayMax === '') ? '' : delayMax;
            const tpsVal = (tps === undefined || tps === null || tps === '') ? '' : tps;
            row.innerHTML = `
                <input type="text" class="form-control rl-model-name" value="${model || ''}" placeholder="模型名称（对外）" style="flex: 1; min-width: 180px; font-family: monospace; font-size: 13px; padding: 6px 10px;">
                <input type="number" class="form-control rl-model-rpm" value="${rpmVal}" placeholder="RPM" min="1" title="模型 RPM 上限（留空使用全局默认）" style="width: 90px; font-family: monospace; font-size: 13px; padding: 6px 10px;">
                <input type="number" class="form-control rl-model-delay-min" value="${delayMinVal}" placeholder="延迟Min" min="0" title="延迟最小值（秒）；留空使用全局默认" style="width: 100px; font-family: monospace; font-size: 13px; padding: 6px 10px;">
                <input type="number" class="form-control rl-model-delay-max" value="${delayMaxVal}" placeholder="延迟Max" min="0" title="延迟最大值（秒）；留空或 0 时按老语义 N~2N（N=Min）" style="width: 100px; font-family: monospace; font-size: 13px; padding: 6px 10px;">
                <input type="number" class="form-control rl-model-tps" value="${tpsVal}" placeholder="TPS" min="0" title="输出 TPS 限速（token/s）；留空或 0 表示不限速" style="width: 90px; font-family: monospace; font-size: 13px; padding: 6px 10px;">
                <button class="btn btn-danger" onclick="this.parentElement.remove()" style="height: 32px; font-size: 12px; padding: 4px 10px;">删除</button>
            `;
            container.appendChild(row);
        }

        function addModelRPMOverride() {
            const container = document.getElementById('rl-model-overrides-list');
            if (!container) return;
            const placeholder = container.querySelector('p');
            if (placeholder) placeholder.remove();
            appendModelRPMRow(container, '', '', '', '', '', container.children.length);
        }

        // 关键词: collectModelRPMOverrides 收集 RPM + 延迟区间 + TPS
        function collectModelRPMOverrides() {
            const rpm = {};
            const delay = {};
            const tps = {};
            document.querySelectorAll('.rl-model-row').forEach(row => {
                const name = row.querySelector('.rl-model-name').value.trim();
                if (!name) return;
                const rpmRaw = row.querySelector('.rl-model-rpm').value;
                const delayMinRaw = row.querySelector('.rl-model-delay-min').value;
                const delayMaxRaw = row.querySelector('.rl-model-delay-max').value;
                const tpsRaw = row.querySelector('.rl-model-tps').value;
                const rpmVal = parseInt(rpmRaw);
                if (rpmRaw !== '' && !isNaN(rpmVal) && rpmVal > 0) {
                    rpm[name] = rpmVal;
                }
                const dmin = parseInt(delayMinRaw);
                const dmax = parseInt(delayMaxRaw);
                const hasMin = (delayMinRaw !== '' && !isNaN(dmin) && dmin >= 0);
                const hasMax = (delayMaxRaw !== '' && !isNaN(dmax) && dmax >= 0);
                if (hasMin || hasMax) {
                    delay[name] = {
                        min: hasMin ? dmin : 0,
                        max: hasMax ? dmax : 0
                    };
                }
                const tpsVal = parseInt(tpsRaw);
                if (tpsRaw !== '' && !isNaN(tpsVal) && tpsVal > 0) {
                    tps[name] = tpsVal;
                }
            });
            return { rpm: rpm, delay: delay, tps: tps };
        }

        // ==================== Lightweight model downgrade rules ====================
        // 关键词: 模型用途降级规则, tier/from/to, X-Yak-AI-Model-Usage-Type 保护用量

        function renderModelDowngradeRules(rules) {
            const container = document.getElementById('rl-downgrade-rules-list');
            if (!container) return;
            container.innerHTML = '';
            rules = Array.isArray(rules) ? rules : [];
            if (rules.length === 0) {
                container.innerHTML = '<p style="color: #999; font-size: 13px;">暂无降级规则（保存后表示关闭降级）。</p>';
                return;
            }
            rules.forEach((rule, idx) => {
                const r = rule || {};
                appendModelDowngradeRow(container, r.tier || '', r.from || '', r.to || '', idx);
            });
        }

        // tier 用途类型：空字符串表示「任意」，其余对齐 consts.Tier*。
        // 关键词: appendModelDowngradeRow tier 下拉, from/to 模型
        function appendModelDowngradeRow(container, tier, from, to, idx) {
            const row = document.createElement('div');
            row.style.cssText = 'display: flex; gap: 8px; align-items: center; margin-bottom: 8px; flex-wrap: wrap;';
            row.className = 'rl-downgrade-row';
            const tierOptions = [
                { v: '', label: '任意' },
                { v: 'lightweight', label: '快速 lightweight' },
                { v: 'intelligent', label: '高质 intelligent' },
                { v: 'vision', label: '视觉 vision' }
            ];
            const optsHtml = tierOptions.map(function (o) {
                const sel = (o.v === (tier || '')) ? ' selected' : '';
                return '<option value="' + o.v + '"' + sel + '>' + o.label + '</option>';
            }).join('');
            const fromVal = (from || '').replace(/"/g, '&quot;');
            const toVal = (to || '').replace(/"/g, '&quot;');
            row.innerHTML =
                '<select class="form-control rl-downgrade-tier" title="客户端上报的模型用途类型；任意表示不限 tier" style="width: 160px; font-size: 13px; padding: 6px 10px;">' + optsHtml + '</select>' +
                '<input type="text" class="form-control rl-downgrade-from" value="' + fromVal + '" placeholder="源模型（对外，如 memfit-standard-free）" style="flex: 1; min-width: 200px; font-family: monospace; font-size: 13px; padding: 6px 10px;">' +
                '<span style="color: #888; font-size: 13px;">→</span>' +
                '<input type="text" class="form-control rl-downgrade-to" value="' + toVal + '" placeholder="目标模型（如 memfit-light-free）" style="flex: 1; min-width: 180px; font-family: monospace; font-size: 13px; padding: 6px 10px;">' +
                '<button class="btn btn-danger" onclick="this.parentElement.remove()" style="height: 32px; font-size: 12px; padding: 4px 10px;">删除</button>';
            container.appendChild(row);
        }

        function addModelDowngradeRule() {
            const container = document.getElementById('rl-downgrade-rules-list');
            if (!container) return;
            const placeholder = container.querySelector('p');
            if (placeholder) placeholder.remove();
            appendModelDowngradeRow(container, '', '', '', container.children.length);
        }

        // collectModelDowngradeRules 收集降级规则数组；from/to 任一为空的行被丢弃。
        // 关键词: collectModelDowngradeRules tier/from/to 数组
        function collectModelDowngradeRules() {
            const out = [];
            document.querySelectorAll('.rl-downgrade-row').forEach(row => {
                const tier = (row.querySelector('.rl-downgrade-tier').value || '').trim();
                const from = (row.querySelector('.rl-downgrade-from').value || '').trim();
                const to = (row.querySelector('.rl-downgrade-to').value || '').trim();
                if (!from || !to) return;
                out.push({ tier: tier, from: from, to: to });
            });
            return out;
        }

        // ==================== Free user Token quota model overrides ====================
        // 关键词: 免费用户 Token 限额 模型覆盖, exempt 复选框

        function renderFreeTokenModelOverrides(overrides) {
            const container = document.getElementById('rl-free-token-model-overrides-list');
            if (!container) return;
            container.innerHTML = '';
            overrides = overrides || {};
            const keys = Object.keys(overrides).sort();
            if (keys.length === 0) {
                container.innerHTML = '<p style="color: #999; font-size: 13px;">暂无模型级覆盖配置。</p>';
                return;
            }
            keys.forEach((model, idx) => {
                const ov = overrides[model] || {};
                appendFreeTokenModelOverrideRow(container, model, ov.limit_m || 0, !!ov.exempt, idx);
            });
        }

        function appendFreeTokenModelOverrideRow(container, model, limitM, exempt, idx) {
            const row = document.createElement('div');
            row.style.cssText = 'display: flex; gap: 10px; align-items: center; margin-bottom: 8px;';
            row.className = 'rl-free-token-row';
            const limitVal = (limitM === undefined || limitM === null || limitM === '') ? '' : limitM;
            row.innerHTML =
                '<input type="text" class="form-control rl-free-token-name" value="' + (model || '').replace(/"/g, '&quot;') + '" placeholder="模型名称（对外，例如 memfit-light-free）" style="flex: 1; font-family: monospace; font-size: 13px; padding: 6px 10px;">' +
                '<input type="number" class="form-control rl-free-token-limit-m" value="' + limitVal + '" placeholder="限额(M)" min="0" title="该模型独立桶限额（M Token）；留空或 0 = 与全局共享池合并" oninput="updateFreeTokenRowRMB(this)" style="width: 130px; font-family: monospace; font-size: 13px; padding: 6px 10px;">' +
                '<small class="rl-free-token-rmb" title="金额限制：1 RMB = 10M 计费 Token" style="font-size: 11px; color: #1565c0; white-space: nowrap; min-width: 64px;"></small>' +
                '<label style="display: flex; align-items: center; gap: 4px; font-size: 12px; color: #555; padding: 0 6px; white-space: nowrap;">' +
                '<input type="checkbox" class="rl-free-token-exempt"' + (exempt ? ' checked' : '') + ' title="勾选表示该模型完全豁免计费（不进入任何桶）"> 不计费' +
                '</label>' +
                '<button class="btn btn-danger" onclick="this.parentElement.remove()" style="height: 32px; font-size: 12px; padding: 4px 10px;">删除</button>';
            container.appendChild(row);
            // 初始化该行「金额限制」换算提示
            updateFreeTokenRowRMB(row.querySelector('.rl-free-token-limit-m'));
        }

        // updateFreeTokenRowRMB 把某个模型级覆盖行的「限额(M)」换算成 RMB 写到同行提示里。
        // 1 RMB = 10M 计费 Token（BILLING_TOKEN_M_PER_RMB）。0 / 空 = 不显示金额。
        // 关键词: updateFreeTokenRowRMB, 模型级覆盖金额限制
        function updateFreeTokenRowRMB(inputEl) {
            if (!inputEl) return;
            const row = inputEl.closest ? inputEl.closest('.rl-free-token-row') : null;
            if (!row) return;
            const span = row.querySelector('.rl-free-token-rmb');
            if (!span) return;
            const m = parseInt(inputEl.value);
            const mm = isNaN(m) ? 0 : m;
            span.textContent = mm > 0 ? ('≈ ¥' + (mm / BILLING_TOKEN_M_PER_RMB).toFixed(2)) : '';
        }

        function addFreeTokenModelOverride() {
            const container = document.getElementById('rl-free-token-model-overrides-list');
            if (!container) return;
            const placeholder = container.querySelector('p');
            if (placeholder) placeholder.remove();
            appendFreeTokenModelOverrideRow(container, '', '', false, container.children.length);
        }

        function collectFreeTokenModelOverrides() {
            const out = {};
            document.querySelectorAll('.rl-free-token-row').forEach(row => {
                const name = row.querySelector('.rl-free-token-name').value.trim();
                if (!name) return;
                const limitRaw = row.querySelector('.rl-free-token-limit-m').value;
                const exempt = row.querySelector('.rl-free-token-exempt').checked;
                let limitM = parseInt(limitRaw);
                if (isNaN(limitM) || limitM < 0) limitM = 0;
                out[name] = { limit_m: limitM, exempt: !!exempt };
            });
            return out;
        }

        // Rate Limit exports
        window.loadRateLimitConfig = loadRateLimitConfig;
        window.saveRateLimitConfig = saveRateLimitConfig;
        window.applyCustom429Default = applyCustom429Default;
        window.loadRateLimitStatus = loadRateLimitStatus;
        window.loadRateLimitModelStats = loadRateLimitModelStats;
        window.startRateLimitModelStatsAutoRefresh = startRateLimitModelStatsAutoRefresh;
        window.stopRateLimitModelStatsAutoRefresh = stopRateLimitModelStatsAutoRefresh;
        // memfit-* 客户端版本统计导出
        // 关键词: window.loadClientVersionStats memfit 客户端版本控流
        window.loadClientVersionStats = loadClientVersionStats;
        // 关键词: window.clearClientVersionStats 清空客户端版本记录
        window.clearClientVersionStats = clearClientVersionStats;
        window.addModelRPMOverride = addModelRPMOverride;
        window.addFreeTokenModelOverride = addFreeTokenModelOverride;
        // 模型用途降级规则导出
        // 关键词: window.addModelDowngradeRule 轻量降级规则
        window.addModelDowngradeRule = addModelDowngradeRule;

        // ==================== DAU & Cache Stats ====================
        // 关键词: DAU 与缓存 tab 渲染, 纯 SVG 折线, 无外部库依赖

        function dauCacheFormatNumber(n) {
            if (n === null || n === undefined || isNaN(n)) return '0';
            return Number(n).toLocaleString();
        }

        function dauCacheFormatRatio(r) {
            if (r === null || r === undefined || isNaN(r)) return '0.00%';
            return (r * 100).toFixed(2) + '%';
        }

        function dauCacheEscapeHtml(s) {
            if (s === null || s === undefined) return '';
            return String(s)
                .replace(/&/g, '&amp;')
                .replace(/</g, '&lt;')
                .replace(/>/g, '&gt;')
                .replace(/"/g, '&quot;')
                .replace(/'/g, '&#39;');
        }

        // drawLineChart 在指定 SVG 节点里绘制多条折线。
        // series: [{label, color, points:[number,...]}]，所有 series 必须等长。
        // labels: x 轴对应的字符串（一般为 date），与 points 等长。
        // 关键词: drawLineChart, 纯 SVG 折线, 自适应坐标
        function drawLineChart(svgId, series, labels, options) {
            const svg = document.getElementById(svgId);
            if (!svg) return;
            options = options || {};
            const formatY = options.formatY || dauCacheFormatNumber;

            // 清空旧内容
            while (svg.firstChild) svg.removeChild(svg.firstChild);

            const vbParts = (svg.getAttribute('viewBox') || '0 0 1200 240').split(/\s+/).map(Number);
            const W = vbParts[2] || 1200;
            const H = vbParts[3] || 240;
            const padL = 60, padR = 140, padT = 20, padB = 30;
            const innerW = W - padL - padR;
            const innerH = H - padT - padB;

            const cleanSeries = (series || []).filter(s => s && s.points && s.points.length > 0);
            if (cleanSeries.length === 0 || (labels || []).length === 0) {
                const t = document.createElementNS('http://www.w3.org/2000/svg', 'text');
                t.setAttribute('x', W / 2);
                t.setAttribute('y', H / 2);
                t.setAttribute('text-anchor', 'middle');
                t.setAttribute('fill', '#999');
                t.setAttribute('font-size', '14');
                t.textContent = '暂无数据';
                svg.appendChild(t);
                return;
            }

            const n = labels.length;
            let maxY = 0;
            cleanSeries.forEach(s => {
                s.points.forEach(v => {
                    const num = Number(v) || 0;
                    if (num > maxY) maxY = num;
                });
            });
            if (maxY <= 0) maxY = 1;
            // 留 10% 顶部空间
            const yMax = maxY * 1.1;

            // axis frame
            const ns = 'http://www.w3.org/2000/svg';
            const frame = document.createElementNS(ns, 'rect');
            frame.setAttribute('x', padL);
            frame.setAttribute('y', padT);
            frame.setAttribute('width', innerW);
            frame.setAttribute('height', innerH);
            frame.setAttribute('fill', 'none');
            frame.setAttribute('stroke', '#ddd');
            frame.setAttribute('stroke-width', '1');
            svg.appendChild(frame);

            // y grid + labels (5 段)
            for (let i = 0; i <= 4; i++) {
                const yVal = yMax * (1 - i / 4);
                const yPos = padT + (innerH * i / 4);
                const grid = document.createElementNS(ns, 'line');
                grid.setAttribute('x1', padL);
                grid.setAttribute('x2', padL + innerW);
                grid.setAttribute('y1', yPos);
                grid.setAttribute('y2', yPos);
                grid.setAttribute('stroke', '#eee');
                grid.setAttribute('stroke-width', '1');
                svg.appendChild(grid);

                const lbl = document.createElementNS(ns, 'text');
                lbl.setAttribute('x', padL - 6);
                lbl.setAttribute('y', yPos + 4);
                lbl.setAttribute('text-anchor', 'end');
                lbl.setAttribute('fill', '#666');
                lbl.setAttribute('font-size', '11');
                lbl.textContent = formatY(yVal);
                svg.appendChild(lbl);
            }

            // x labels: 首/中/尾 三个
            const xIdxs = n === 1 ? [0] : [0, Math.floor((n - 1) / 2), n - 1];
            xIdxs.forEach(idx => {
                const xPos = n === 1 ? padL + innerW / 2 : padL + (innerW * idx / (n - 1));
                const t = document.createElementNS(ns, 'text');
                t.setAttribute('x', xPos);
                t.setAttribute('y', padT + innerH + 18);
                t.setAttribute('text-anchor', 'middle');
                t.setAttribute('fill', '#666');
                t.setAttribute('font-size', '11');
                t.textContent = labels[idx] || '';
                svg.appendChild(t);
            });

            const xPosOf = (idx) => n === 1 ? padL + innerW / 2 : padL + (innerW * idx / (n - 1));
            const yPosOf = (val) => padT + innerH * (1 - (Number(val) || 0) / yMax);

            // 折线 + 图例
            cleanSeries.forEach((s, sIdx) => {
                const color = s.color || '#4285f4';
                const points = s.points;
                let d = '';
                for (let i = 0; i < points.length; i++) {
                    const x = xPosOf(i);
                    const y = yPosOf(points[i]);
                    d += (i === 0 ? 'M' : 'L') + x.toFixed(2) + ',' + y.toFixed(2) + ' ';
                }
                const path = document.createElementNS(ns, 'path');
                path.setAttribute('d', d.trim());
                path.setAttribute('fill', 'none');
                path.setAttribute('stroke', color);
                path.setAttribute('stroke-width', '1.6');
                path.setAttribute('stroke-linejoin', 'round');
                svg.appendChild(path);

                const legendY = padT + 14 + sIdx * 18;
                const swatch = document.createElementNS(ns, 'rect');
                swatch.setAttribute('x', padL + innerW + 14);
                swatch.setAttribute('y', legendY - 8);
                swatch.setAttribute('width', 12);
                swatch.setAttribute('height', 12);
                swatch.setAttribute('fill', color);
                svg.appendChild(swatch);

                const legendText = document.createElementNS(ns, 'text');
                legendText.setAttribute('x', padL + innerW + 32);
                legendText.setAttribute('y', legendY + 2);
                legendText.setAttribute('fill', '#333');
                legendText.setAttribute('font-size', '11');
                legendText.textContent = s.label || ('series ' + (sIdx + 1));
                svg.appendChild(legendText);
            });
        }

        // ============ 模型堆叠图辅助函数 ============
        // 关键词: dau-cache 模型堆叠, pivotModelTrend, drawStackedAreaChart

        // MODEL_PRIORITY_ORDER 是堆叠/图例的优先级关键字, 命中前缀的模型先排.
        // 顺序: standard -> basic -> light -> max, 其余按总量降序.
        // 关键词: MODEL_PRIORITY_ORDER, 模型堆叠优先级 standard basic light max
        var MODEL_PRIORITY_ORDER = ['standard', 'basic', 'light', 'max'];

        // STACK_COLORS 是堆叠/多线图的默认色板, 与 sidebar 配色协调.
        // 关键词: STACK_COLORS 模型堆叠色板
        var STACK_COLORS = [
            '#1565c0', '#558b2f', '#ef6c00', '#c2185b', '#4527a0', '#00695c',
            '#f9a825', '#6a1b9a', '#00838f', '#283593', '#bf360c', '#37474f',
            '#827717', '#ad1457', '#ffb300'
        ];

        // pickColor 按索引循环取色, 「其他」固定灰色.
        function pickColor(idx, isOther) {
            if (isOther) return '#9e9e9e';
            return STACK_COLORS[idx % STACK_COLORS.length];
        }

        // modelPriorityRank 返回模型名的优先级 (越小越靠前). 未命中返回 999.
        // 用小写包含匹配, 兼容 memfit-standard-free / memfit-basic-free 等命名.
        // 关键词: modelPriorityRank, 模型排序 standard basic light max
        function modelPriorityRank(name) {
            var lower = String(name || '').toLowerCase();
            for (var i = 0; i < MODEL_PRIORITY_ORDER.length; i++) {
                if (lower.indexOf(MODEL_PRIORITY_ORDER[i]) >= 0) return i;
            }
            return 999;
        }

        // pivotModelTrend 把后端扁平行 [{date, model, <metric>}, ...] 透视成:
        //   { labels: [date,...]                   // 全 days 日期轴 (升序)
        //     series: [{label, color, points:[number,...], total}, ...]  // 每模型一条
        //   }
        // 处理:
        //   - 自建 days 长度的日期轴, 缺失日期补 0
        //   - 模型排序: 先按 MODEL_PRIORITY_ORDER 命中, 然后按 total 降序
        //   - 超过 topN 的模型并入 "其他"
        // 关键词: pivotModelTrend, 透视聚合 + 优先级排序 + Top-N 其他
        function pivotModelTrend(rows, metricKey, options) {
            options = options || {};
            var days = options.days || 180;
            var topN = options.topN || 8;
            var endDate = options.endDate ? new Date(options.endDate) : new Date();

            // 1. 构造连续日期轴 (从 days-1 天前到 endDate)
            var labels = [];
            var dateIdx = {};
            for (var i = days - 1; i >= 0; i--) {
                var d = new Date(endDate.getFullYear(), endDate.getMonth(), endDate.getDate() - i);
                var key = d.getFullYear() + '-' +
                    String(d.getMonth() + 1).padStart(2, '0') + '-' +
                    String(d.getDate()).padStart(2, '0');
                dateIdx[key] = labels.length;
                labels.push(key);
            }

            // 2. 透视: model -> points[days]
            var modelMap = {}; // name -> {points:Array(days).fill(0), total:0}
            (rows || []).forEach(function (r) {
                if (!r) return;
                var name = r.model || '(unknown)';
                var date = r.date;
                var idx = dateIdx[date];
                if (idx === undefined) return;
                var v = Number(r[metricKey]) || 0;
                if (!modelMap[name]) {
                    modelMap[name] = { points: new Array(days).fill(0), total: 0 };
                }
                modelMap[name].points[idx] += v;
                modelMap[name].total += v;
            });

            // 3. 排序: 优先级在前, 同优先级按 total 降序, 同 total 按名字字典序
            var allModels = Object.keys(modelMap).map(function (name) {
                return {
                    name: name,
                    total: modelMap[name].total,
                    points: modelMap[name].points,
                    rank: modelPriorityRank(name)
                };
            }).filter(function (m) { return m.total > 0; }); // 跳过完全 0 的模型
            allModels.sort(function (a, b) {
                if (a.rank !== b.rank) return a.rank - b.rank;
                if (b.total !== a.total) return b.total - a.total;
                return a.name.localeCompare(b.name);
            });

            // 4. Top-N 截断 + "其他" 聚合
            var series = [];
            var keep = allModels.slice(0, topN);
            var rest = allModels.slice(topN);
            keep.forEach(function (m, idx) {
                series.push({
                    label: m.name,
                    color: pickColor(idx, false),
                    points: m.points,
                    total: m.total
                });
            });
            if (rest.length > 0) {
                var merged = new Array(days).fill(0);
                var mergedTotal = 0;
                rest.forEach(function (m) {
                    for (var i = 0; i < days; i++) merged[i] += m.points[i] || 0;
                    mergedTotal += m.total;
                });
                series.push({
                    label: '其他 (' + rest.length + ' 个模型)',
                    color: pickColor(0, true),
                    points: merged,
                    total: mergedTotal
                });
            }

            return { labels: labels, series: series };
        }

        // drawStackedAreaChart 在 SVG 上绘制线性堆叠面积图.
        // series 顺序 = 堆叠从底向上的顺序; 同时绘制顶部"总和"线方便读总量.
        // 关键词: drawStackedAreaChart, 线性堆叠面积图, 纯 SVG
        function drawStackedAreaChart(svgId, series, labels, options) {
            var svg = document.getElementById(svgId);
            if (!svg) return;
            options = options || {};
            var formatY = options.formatY || dauCacheFormatNumber;

            while (svg.firstChild) svg.removeChild(svg.firstChild);

            var vb = (svg.getAttribute('viewBox') || '0 0 1400 240').split(/\s+/).map(Number);
            var W = vb[2] || 1400, H = vb[3] || 240;
            var padL = 60, padR = 220, padT = 18, padB = 30;
            var innerW = W - padL - padR;
            var innerH = H - padT - padB;
            var ns = 'http://www.w3.org/2000/svg';

            var clean = (series || []).filter(function (s) { return s && s.points && s.points.length > 0; });
            var n = (labels || []).length;
            if (clean.length === 0 || n === 0) {
                var t = document.createElementNS(ns, 'text');
                t.setAttribute('x', W / 2);
                t.setAttribute('y', H / 2);
                t.setAttribute('text-anchor', 'middle');
                t.setAttribute('fill', '#999');
                t.setAttribute('font-size', '14');
                t.textContent = '暂无数据';
                svg.appendChild(t);
                return;
            }

            // 计算每日堆叠总和 (= y 轴最大值参考)
            var stackTotals = new Array(n).fill(0);
            clean.forEach(function (s) {
                for (var i = 0; i < n; i++) stackTotals[i] += (Number(s.points[i]) || 0);
            });
            var maxY = 0;
            for (var i = 0; i < n; i++) { if (stackTotals[i] > maxY) maxY = stackTotals[i]; }
            if (maxY <= 0) maxY = 1;
            var yMax = maxY * 1.1;

            // 坐标帧 + 网格 + Y 标签 (5 段)
            var frame = document.createElementNS(ns, 'rect');
            frame.setAttribute('x', padL); frame.setAttribute('y', padT);
            frame.setAttribute('width', innerW); frame.setAttribute('height', innerH);
            frame.setAttribute('fill', 'none'); frame.setAttribute('stroke', '#ddd');
            svg.appendChild(frame);
            for (var g = 0; g <= 4; g++) {
                var yVal = yMax * (1 - g / 4);
                var yPos = padT + innerH * g / 4;
                var grid = document.createElementNS(ns, 'line');
                grid.setAttribute('x1', padL); grid.setAttribute('x2', padL + innerW);
                grid.setAttribute('y1', yPos); grid.setAttribute('y2', yPos);
                grid.setAttribute('stroke', '#eee'); grid.setAttribute('stroke-width', '1');
                svg.appendChild(grid);
                var lbl = document.createElementNS(ns, 'text');
                lbl.setAttribute('x', padL - 6); lbl.setAttribute('y', yPos + 4);
                lbl.setAttribute('text-anchor', 'end');
                lbl.setAttribute('fill', '#666'); lbl.setAttribute('font-size', '11');
                lbl.textContent = formatY(yVal);
                svg.appendChild(lbl);
            }
            // X 标签: 首/四分位/中/三分位/尾 5 个 (长窗口下更易读)
            var xIdxs = n === 1 ? [0] : [0, Math.floor((n - 1) / 4), Math.floor((n - 1) / 2), Math.floor((n - 1) * 3 / 4), n - 1];
            xIdxs.forEach(function (idx) {
                var xPos = n === 1 ? padL + innerW / 2 : padL + innerW * idx / (n - 1);
                var tt = document.createElementNS(ns, 'text');
                tt.setAttribute('x', xPos); tt.setAttribute('y', padT + innerH + 18);
                tt.setAttribute('text-anchor', 'middle');
                tt.setAttribute('fill', '#666'); tt.setAttribute('font-size', '11');
                tt.textContent = labels[idx] || '';
                svg.appendChild(tt);
            });

            var xPosOf = function (i) { return n === 1 ? padL + innerW / 2 : padL + innerW * i / (n - 1); };
            var yPosOf = function (v) { return padT + innerH * (1 - (Number(v) || 0) / yMax); };

            // 自底向上累加, 每层用 polygon 填充 (lower-bound 上一层 cumulative).
            var lower = new Array(n).fill(0);
            clean.forEach(function (s, sIdx) {
                var color = s.color || pickColor(sIdx, false);
                var upper = new Array(n);
                for (var i = 0; i < n; i++) upper[i] = lower[i] + (Number(s.points[i]) || 0);

                var pts = [];
                for (var i2 = 0; i2 < n; i2++) {
                    pts.push(xPosOf(i2).toFixed(2) + ',' + yPosOf(upper[i2]).toFixed(2));
                }
                for (var j = n - 1; j >= 0; j--) {
                    pts.push(xPosOf(j).toFixed(2) + ',' + yPosOf(lower[j]).toFixed(2));
                }
                var poly = document.createElementNS(ns, 'polygon');
                poly.setAttribute('points', pts.join(' '));
                poly.setAttribute('fill', color);
                poly.setAttribute('fill-opacity', '0.85');
                poly.setAttribute('stroke', color);
                poly.setAttribute('stroke-width', '0.6');
                poly.setAttribute('stroke-opacity', '0.9');
                svg.appendChild(poly);

                // 图例: 右侧栏, 显示总量
                var legendY = padT + 14 + sIdx * 18;
                if (legendY < padT + innerH - 4) {
                    var swatch = document.createElementNS(ns, 'rect');
                    swatch.setAttribute('x', padL + innerW + 14);
                    swatch.setAttribute('y', legendY - 8);
                    swatch.setAttribute('width', 12); swatch.setAttribute('height', 12);
                    swatch.setAttribute('fill', color);
                    svg.appendChild(swatch);
                    var lt = document.createElementNS(ns, 'text');
                    lt.setAttribute('x', padL + innerW + 32);
                    lt.setAttribute('y', legendY + 2);
                    lt.setAttribute('fill', '#333');
                    lt.setAttribute('font-size', '11');
                    var totalText = s.total != null ? ' (' + dauCacheFormatNumber(s.total) + ')' : '';
                    lt.textContent = (s.label || ('series ' + (sIdx + 1))) + totalText;
                    svg.appendChild(lt);
                }

                lower = upper;
            });

            // 顶部"总和"虚线 (堆叠最高 = 当日总量)
            var topPath = '';
            for (var k = 0; k < n; k++) {
                topPath += (k === 0 ? 'M' : 'L') + xPosOf(k).toFixed(2) + ',' + yPosOf(stackTotals[k]).toFixed(2) + ' ';
            }
            var top = document.createElementNS(ns, 'path');
            top.setAttribute('d', topPath.trim());
            top.setAttribute('fill', 'none');
            top.setAttribute('stroke', '#212121');
            top.setAttribute('stroke-width', '1');
            top.setAttribute('stroke-dasharray', '4,3');
            top.setAttribute('opacity', '0.55');
            svg.appendChild(top);
        }

        // formatBytesHuman 把字节数格式化成 KB/MB/GB/TB.
        // 关键词: formatBytesHuman, dau-cache 磁盘 KPI
        function formatBytesHuman(n) {
            n = Number(n) || 0;
            if (n < 1024) return n + ' B';
            var units = ['KB', 'MB', 'GB', 'TB', 'PB'];
            var u = -1;
            do { n /= 1024; u++; } while (n >= 1024 && u < units.length - 1);
            return n.toFixed(n >= 10 ? 0 : 1) + ' ' + units[u];
        }

        // renderDiskCard 渲染顶部信息条的「磁盘可用」KPI 卡。
        // 主数字 = 可用空间; 副行 = 已用百分比 + 总量; 按已用百分比染色; path 进 title.
        // 关键词: renderDiskCard, 顶部磁盘 KPI 渲染, disk_info
        function renderDiskCard(disk) {
            disk = disk || {};
            const card = document.getElementById('disk-card');
            const freeEl = document.getElementById('disk-free-display');
            const subEl = document.getElementById('disk-sub');
            if (!card || !freeEl || !subEl) return;
            if (disk.available) {
                const usedPct = Number(disk.used_percent) || 0;
                freeEl.textContent = formatBytesHuman(disk.free || 0);
                subEl.textContent = '已用 ' + usedPct.toFixed(1) + '% / 总 ' + formatBytesHuman(disk.total || 0);
                card.title = '路径: ' + (disk.path || '-') +
                    '\n总: ' + formatBytesHuman(disk.total || 0) +
                    '\n可用: ' + formatBytesHuman(disk.free || 0) +
                    '\n已用: ' + formatBytesHuman(disk.used || 0) + ' (' + usedPct.toFixed(2) + '%)';
                // 按已用百分比染色 (仅染主数字, 卡片底色保持与其他卡一致)
                let fg = '#2c3e50';
                if (usedPct >= 90) fg = '#c62828';
                else if (usedPct >= 75) fg = '#ef6c00';
                freeEl.style.color = fg;
            } else {
                freeEl.textContent = 'N/A';
                freeEl.style.color = '#999';
                subEl.textContent = disk.path ? disk.path : '暂不可用';
                card.title = '磁盘信息暂不可用';
            }
        }

        // renderStorageCard 渲染顶部信息条的「存储采集数据」KPI 卡。
        // 主数字 = 已落盘条数; 副行 = 占用大小; 未启用/未装配时显示「未启用」。
        // 关键词: renderStorageCard, 顶部存储 KPI 渲染, storage_info
        function renderStorageCard(storage) {
            storage = storage || {};
            const card = document.getElementById('storage-card');
            const recEl = document.getElementById('storage-records-display');
            const subEl = document.getElementById('storage-sub');
            if (!card || !recEl || !subEl) return;
            if (storage.available) {
                const records = Number(storage.records) || 0;
                const bytes = Number(storage.bytes) || 0;
                recEl.textContent = records.toLocaleString() + ' 条';
                recEl.style.color = '#1565c0';
                subEl.textContent = '占用 ' + formatBytesHuman(bytes);
                card.title = '已采集落盘 ' + records.toLocaleString() + ' 条 / 占用 ' + formatBytesHuman(bytes);
            } else {
                recEl.textContent = '未启用';
                recEl.style.color = '#999';
                subEl.textContent = '在「流量镜像」中开启落盘';
                card.title = '数据落盘未启用';
            }
        }

        // renderDauCacheTab 把后端 portal data 一次性渲染到 dau-cache tab 的所有节点。
        // 关键词: renderDauCacheTab, KPI 数字 + 三张折线 + 拆分表
        function renderDauCacheTab(data) {
            if (!data) return;
            const setText = (id, text) => {
                const el = document.getElementById(id);
                if (el) el.textContent = text;
            };

            const todayDate = data.today_date || '';
            setText('dc-date', todayDate);

            const breakdown = data.today_dau_breakdown || {api_key:0, free_trace:0, free_ip:0, total:0};
            setText('dc-dau-total', dauCacheFormatNumber(breakdown.total || data.today_dau || 0));
            setText('dc-dau-apikey', dauCacheFormatNumber(breakdown.api_key || 0));
            setText('dc-dau-trace', dauCacheFormatNumber(breakdown.free_trace || 0));
            setText('dc-dau-ip', dauCacheFormatNumber(breakdown.free_ip || 0));

            const summaries = data.daily_summary_60_days || [];
            const todaySummary = summaries.find(s => s.date === todayDate) || {};
            const todayReqs = todaySummary.total_requests || 0;
            const totalDauNum = breakdown.total || data.today_dau || 0;
            setText('dc-req-total', dauCacheFormatNumber(todayReqs));
            const avg = totalDauNum > 0 ? (todayReqs / totalDauNum) : 0;
            setText('dc-avg-per-user', avg.toFixed(2));

            const cacheStats = data.today_cache_stats || {};
            setText('dc-cache-ratio', dauCacheFormatRatio(cacheStats.hit_ratio || 0));

            // 60 天日活折线
            const dauList = data.dau_60_days || [];
            const dauLabels = dauList.map(d => d.date);
            drawLineChart('dc-chart-dau', [
                {label: 'API Key', color: '#558b2f', points: dauList.map(d => d.api_key || 0)},
                {label: 'Free Trace', color: '#ef6c00', points: dauList.map(d => d.free_trace || 0)},
                {label: 'Free IP', color: '#c2185b', points: dauList.map(d => d.free_ip || 0)},
                {label: 'Total', color: '#1565c0', points: dauList.map(d => d.total || 0)},
            ], dauLabels);

            // 60 天单用户平均请求折线（需 join summary + dau）
            const dauByDate = {};
            dauList.forEach(d => { dauByDate[d.date] = d.total || 0; });
            const summaryLabels = summaries.map(s => s.date);
            const avgPoints = summaries.map(s => {
                const tot = dauByDate[s.date] || 0;
                return tot > 0 ? ((s.total_requests || 0) / tot) : 0;
            });
            drawLineChart('dc-chart-avg', [
                {label: 'requests / user', color: '#00695c', points: avgPoints},
            ], summaryLabels, {formatY: v => Number(v).toFixed(2)});

            // ============ 180 天按模型堆叠图 ============
            // 关键词: dau-cache 180 天堆叠图渲染, model_trend_180_days
            const modelRows = data.model_trend_180_days || [];
            const tokenStack = pivotModelTrend(modelRows, 'total_tokens', {days: 180, topN: 8, endDate: todayDate});
            drawStackedAreaChart('dc-chart-token-stack', tokenStack.series, tokenStack.labels);

            const reqStack = pivotModelTrend(modelRows, 'request_count', {days: 180, topN: 8, endDate: todayDate});
            drawStackedAreaChart('dc-chart-req-stack', reqStack.series, reqStack.labels);

            const promptStack = pivotModelTrend(modelRows, 'prompt_tokens', {days: 180, topN: 8, endDate: todayDate});
            drawStackedAreaChart('dc-chart-prompt-stack', promptStack.series, promptStack.labels);

            // 60 天缓存命中比例: 平均线 + 每模型命中比例多线 (取近 60 天窗口的同一份 modelRows 透视)
            // 关键词: dau-cache 缓存命中多线, 平均 + per-model hit ratio
            const trend = data.cache_trend_60_days || [];
            const trendLabels = trend.map(t => t.date);
            const promptPivot60 = pivotModelTrend(modelRows, 'prompt_tokens', {days: 60, topN: 6, endDate: todayDate});
            const cachedPivot60 = pivotModelTrend(modelRows, 'cached_tokens', {days: 60, topN: 6, endDate: todayDate});
            // 用同样的模型集合 (以 prompt 为准) 求 cached/prompt 比例点
            const cachedByModel = {};
            cachedPivot60.series.forEach(s => { cachedByModel[s.label] = s.points; });
            const cacheSeries = [
                {label: 'avg hit ratio', color: '#212121', points: trend.map(t => t.hit_ratio || 0)},
            ];
            promptPivot60.series.forEach((s, i) => {
                const cp = cachedByModel[s.label] || [];
                const ratios = s.points.map((p, idx) => {
                    const c = Number(cp[idx]) || 0;
                    return p > 0 ? (c / p) : 0;
                });
                cacheSeries.push({label: s.label, color: pickColor(i, s.label.indexOf('其他') === 0), points: ratios});
            });
            drawLineChart('dc-chart-cache', cacheSeries, promptPivot60.labels, {
                formatY: v => (v * 100).toFixed(2) + '%'
            });

            // 今日拆分表
            const tbody = document.getElementById('dc-breakdown-body');
            if (tbody) {
                const rows = data.today_cache_breakdown || [];
                if (rows.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="9" style="padding: 12px; text-align: center; color: #999;">今日尚无 usage 数据</td></tr>';
                } else {
                    tbody.innerHTML = rows.map(r => `
                        <tr>
                            <td style="padding: 6px 10px; border-bottom: 1px solid #f0f0f0;">${dauCacheEscapeHtml(r.wrapper_name)}</td>
                            <td style="padding: 6px 10px; border-bottom: 1px solid #f0f0f0;">${dauCacheEscapeHtml(r.model_name)}</td>
                            <td style="padding: 6px 10px; border-bottom: 1px solid #f0f0f0;">${dauCacheEscapeHtml(r.provider_type_name)}</td>
                            <td style="padding: 6px 10px; border-bottom: 1px solid #f0f0f0;">${dauCacheEscapeHtml(r.provider_domain)}</td>
                            <td style="padding: 6px 10px; border-bottom: 1px solid #f0f0f0; font-family: monospace;">${dauCacheEscapeHtml(r.api_key_shrink)}</td>
                            <td style="padding: 6px 10px; border-bottom: 1px solid #f0f0f0; text-align: right;">${dauCacheFormatNumber(r.request_count)}</td>
                            <td style="padding: 6px 10px; border-bottom: 1px solid #f0f0f0; text-align: right;">${dauCacheFormatNumber(r.prompt_tokens)}</td>
                            <td style="padding: 6px 10px; border-bottom: 1px solid #f0f0f0; text-align: right;">${dauCacheFormatNumber(r.cached_tokens)}</td>
                            <td style="padding: 6px 10px; border-bottom: 1px solid #f0f0f0; text-align: right;">${dauCacheFormatRatio(r.hit_ratio)}</td>
                        </tr>
                    `).join('');
                }
            }
        }

        // refreshDauCacheTab 主动重新拉一次 portal data 并仅刷新 dau-cache 视图。
        // 关键词: refreshDauCacheTab, tab 切到日活与缓存时主动拉新
        async function refreshDauCacheTab() {
            try {
                const response = await authFetch('/portal/api/data');
                if (!response) return;
                const data = await response.json();
                if (checkAuthInResponse(data)) return;
                portalData = data;
                renderDauCacheTab(data);
                const todayDauEl = document.getElementById('stat-today-dau');
                if (todayDauEl) {
                    todayDauEl.textContent = (data.today_dau || 0).toLocaleString();
                }
            } catch (e) {
                console.error('refreshDauCacheTab failed:', e);
            }
        }

        // ==================== 图表点击放大 (lightbox) ====================
        // 把被点图表盒子内的 SVG 克隆进放大弹窗, 设为 100% 充满, 标题取同格的标题.
        // 克隆而非移动: 原图保持不动, 关闭弹窗直接清空克隆即可.
        // 关键词: enlargeChart 图表放大, closeChartZoom, chart-zoom-modal lightbox
        function enlargeChart(box) {
            if (!box) return;
            const svg = box.querySelector('svg');
            const modal = document.getElementById('chart-zoom-modal');
            const body = document.getElementById('chart-zoom-body');
            const titleEl = document.getElementById('chart-zoom-title');
            if (!svg || !modal || !body) return;
            // 标题: 同 cell 内的 .dc-chart-title 文本
            let title = '图表';
            const cell = box.closest('.dc-chart-cell');
            if (cell) {
                const t = cell.querySelector('.dc-chart-title');
                if (t) title = t.textContent.trim();
            }
            if (titleEl) titleEl.textContent = title;
            // 克隆并放大: 去掉固定 height, 由 CSS 充满弹窗
            const clone = svg.cloneNode(true);
            clone.setAttribute('width', '100%');
            clone.setAttribute('height', '100%');
            body.innerHTML = '';
            body.appendChild(clone);
            modal.classList.add('open');
        }

        function closeChartZoom(evt) {
            // 点遮罩或关闭按钮才关; 点内容区 (inner) 已 stopPropagation
            const modal = document.getElementById('chart-zoom-modal');
            if (!modal) return;
            modal.classList.remove('open');
            const body = document.getElementById('chart-zoom-body');
            if (body) body.innerHTML = '';
        }

        // 事件委托: 点击任意 .dc-chart-box 触发放大 (图表会被重绘, 委托更稳妥)
        document.addEventListener('click', function (e) {
            const box = e.target.closest && e.target.closest('.dc-chart-box');
            if (box) enlargeChart(box);
        });
        // Esc 关闭放大弹窗
        document.addEventListener('keydown', function (e) {
            if (e.key === 'Escape') closeChartZoom();
        });

        window.enlargeChart = enlargeChart;
        window.closeChartZoom = closeChartZoom;
        window.drawLineChart = drawLineChart;
        window.drawStackedAreaChart = drawStackedAreaChart;
        window.pivotModelTrend = pivotModelTrend;
        window.formatBytesHuman = formatBytesHuman;
        window.renderDiskCard = renderDiskCard;
        window.renderStorageCard = renderStorageCard;
        window.renderDauCacheTab = renderDauCacheTab;
        window.refreshDauCacheTab = refreshDauCacheTab;

        // ==================== Mirror Rules 管理 ====================
        // 关键词: MirrorMgmt UI, mirror rules portal frontend, AiMirrorRule CRUD,
        //         window scope assignment, IIFE 立即写 window 防 TDZ
        //
        // 设计:
        //  - 单例对象, 状态 (cache / edit) 都挂在 MirrorMgmt 上, 避免全局变量污染.
        //  - fetch 调用都加 same-origin, 错误统一 toast.
        //  - 试运行结果直接渲染到弹窗内的结果区, 不另开浮层.
        //  - 直接 window.MirrorMgmt = (...)() 不使用 const, 这样:
        //      a) HTML inline onclick="MirrorMgmt..." 立刻能解析到 (走 window)
        //      b) 其他代码 typeof MirrorMgmt 不会因 const TDZ 抛错
        //      c) 这块 IIFE 出错或前面 top-level 中断时, window.MirrorMgmt 仍为 undefined
        //         而不是处于 "已声明未初始化" 的死状态
        window.MirrorMgmt = (function() {
            const condLabels = {
                'always':                '每次请求',
                'action_eq':             '@action 等于',
                'any_toolcall':          '任意 tool_calls',
                'action_call_tool_eq':   '@action+工具名',
            };
            // fallbackDefaultScript: 当 /portal/api/mirror-rules/_meta 拉取失败时的最后一道兜底.
            // 真实的默认脚本由后端 DefaultMirrorScript() 提供 (含 if YAK_MAIN 自测块).
            // 关键词: mirror fallback default script, _meta 拉取失败兜底
            const fallbackDefaultScript = [
                '// aibalance mirror callback (fallback template).',
                '// 关键词: aibalance mirror callback, handle(data) entry',
                'func handle(data) {',
                '    log.info(sprint("mirror got req: model=%v action=%v dur=%vms",',
                '        data.model, data.action, data["duration_ms"]))',
                '}',
                '',
                'if YAK_MAIN {',
                '    handle({"req_id": "local-test", "model": "demo", "action": ""})',
                '}',
                ''
            ].join('\n');

            const state = {
                cache: [],
                meta: null,       // { default_script, data_spec, condition_types }
                metaLoading: null // in-flight promise, 防止并发重复拉取
            };

            // ensureMeta 把 _meta 接口结果缓存到 state.meta, 并按需懒加载.
            // 关键词: ensureMeta, mirror meta 懒加载缓存, default_script data_spec
            async function ensureMeta() {
                if (state.meta) return state.meta;
                if (state.metaLoading) return state.metaLoading;
                state.metaLoading = (async () => {
                    try {
                        const resp = await fetch('/portal/api/mirror-rules/_meta', {credentials: 'same-origin'});
                        if (!resp.ok) throw new Error('HTTP ' + resp.status);
                        const j = await resp.json();
                        state.meta = {
                            default_script: j.default_script || fallbackDefaultScript,
                            data_spec:      Array.isArray(j.data_spec) ? j.data_spec : [],
                            condition_types: Array.isArray(j.condition_types) ? j.condition_types : [],
                        };
                    } catch (e) {
                        console.warn('mirror: load meta failed, use fallback', e);
                        state.meta = {
                            default_script: fallbackDefaultScript,
                            data_spec: [],
                            condition_types: [],
                        };
                    } finally {
                        state.metaLoading = null;
                    }
                    return state.meta;
                })();
                return state.metaLoading;
            }

            function getDefaultScript() {
                return (state.meta && state.meta.default_script) || fallbackDefaultScript;
            }

            // renderDataSpec 把后端给的 spec 渲染到弹窗右侧的帮助面板.
            // 关键词: renderDataSpec, mirror data spec 字段表渲染
            function renderDataSpec() {
                const host = document.getElementById('mirrorDataSpecHost');
                if (!host) return;
                const spec = (state.meta && state.meta.data_spec) || [];
                if (!spec.length) {
                    host.innerHTML = '<div style="color:#888; padding:12px; font-size:12px;">未获取到字段说明.</div>';
                    return;
                }
                const html = spec.map(f => {
                    const name = escapeHtml(f.name || '');
                    const type = escapeHtml(f.type || '');
                    const desc = escapeHtml(f.description || '');
                    const example = escapeHtml(f.example || '');
                    return `<div class="mirror-spec-entry">
                        <div class="mirror-spec-entry-head">
                            <span class="mirror-spec-name">${name}</span>
                            <span class="mirror-spec-type">${type}</span>
                        </div>
                        <div class="mirror-spec-desc">${desc}</div>
                        ${example ? `<div class="mirror-spec-example">${example}</div>` : ''}
                    </div>`;
                }).join('');
                host.innerHTML = html;
            }

            async function refresh() {
                try {
                    const resp = await fetch('/portal/api/mirror-rules', {credentials: 'same-origin'});
                    if (!resp.ok) {
                        throw new Error('HTTP ' + resp.status);
                    }
                    const j = await resp.json();
                    state.cache = j.rules || [];
                    renderTable(state.cache);
                    renderKpi(state.cache);
                } catch (e) {
                    console.error('mirror: refresh failed', e);
                    showToast('加载镜像规则失败: ' + (e.message || e), 'error');
                }
                // 顺带刷新落盘设置与实时用量。
                loadStorageConfig();
            }

            // GiB 与字节互转辅助 (1 GiB = 1<<30)。
            const GIB = 1024 * 1024 * 1024;
            function fmtBytes(n) {
                n = Number(n) || 0;
                if (n >= GIB) return (n / GIB).toFixed(2) + ' GiB';
                if (n >= 1024 * 1024) return (n / (1024 * 1024)).toFixed(2) + ' MiB';
                if (n >= 1024) return (n / 1024).toFixed(2) + ' KiB';
                return n + ' B';
            }

            // loadStorageConfig 读取落盘配置 + 实时计数, 填充到 mirror tab 的「数据落盘设置」卡。
            // 关键词: loadStorageConfig, 落盘配置读取
            async function loadStorageConfig() {
                try {
                    const resp = await fetch('/portal/api/mirror-storage-config', {credentials: 'same-origin'});
                    if (!resp.ok) return;
                    const j = await resp.json();
                    if (!j || !j.success) return;
                    const en = document.getElementById('mirror-storage-enabled');
                    if (en) en.checked = !!j.enabled;
                    const maxEl = document.getElementById('mirror-storage-max-gib');
                    if (maxEl) maxEl.value = ((Number(j.max_bytes) || 0) / GIB).toFixed(2);
                    const recEl = document.getElementById('mirror-storage-reclaim-gib');
                    if (recEl) recEl.value = ((Number(j.reclaim_bytes) || 0) / GIB).toFixed(2);
                    const secEl = document.getElementById('mirror-storage-check-sec');
                    if (secEl) secEl.value = Number(j.check_interval_sec) || 60;
                    const recordsEl = document.getElementById('mirror-storage-records');
                    if (recordsEl) recordsEl.textContent = (Number(j.records) || 0).toLocaleString();
                    const bytesEl = document.getElementById('mirror-storage-bytes');
                    if (bytesEl) bytesEl.textContent = fmtBytes(j.bytes);
                } catch (e) {
                    console.error('mirror: loadStorageConfig failed', e);
                }
            }

            // saveStorageConfig 保存落盘配置 (GiB 转字节后提交)。
            // 关键词: saveStorageConfig, 落盘配置保存
            async function saveStorageConfig() {
                const en = document.getElementById('mirror-storage-enabled');
                const maxEl = document.getElementById('mirror-storage-max-gib');
                const recEl = document.getElementById('mirror-storage-reclaim-gib');
                const secEl = document.getElementById('mirror-storage-check-sec');
                const maxGib = parseFloat(maxEl && maxEl.value) || 0;
                const recGib = parseFloat(recEl && recEl.value) || 0;
                const sec = parseInt(secEl && secEl.value, 10) || 0;
                const body = {
                    enabled: !!(en && en.checked),
                    max_bytes: Math.round(maxGib * GIB),
                    reclaim_bytes: Math.round(recGib * GIB),
                    check_interval_sec: sec,
                };
                try {
                    const resp = await fetch('/portal/api/mirror-storage-config', {
                        method: 'POST',
                        headers: {'Content-Type': 'application/json'},
                        credentials: 'same-origin',
                        body: JSON.stringify(body),
                    });
                    const j = await resp.json();
                    if (isAuthError(j)) { handleAuthError(); return; }
                    if (j && j.success) {
                        showToast('落盘设置已保存', 'success');
                        loadStorageConfig();
                    } else {
                        showToast((j && (j.message || j.error)) || '保存失败', 'error');
                    }
                } catch (e) {
                    console.error('mirror: saveStorageConfig failed', e);
                    showToast('保存落盘设置失败', 'error');
                }
            }

            function renderKpi(rules) {
                let enabled = 0, total = 0, success = 0, failDrop = 0;
                rules.forEach(r => {
                    if (r.enabled) enabled++;
                    total += r.total_triggered || 0;
                    success += r.total_success || 0;
                    failDrop += (r.total_failed || 0) + (r.total_dropped || 0);
                });
                const set = (id, v) => { const el = document.getElementById(id); if (el) el.textContent = v.toLocaleString(); };
                set('mirror-enabled-count', enabled);
                set('mirror-total-triggered', total);
                set('mirror-total-success', success);
                set('mirror-total-fail-drop', failDrop);
            }

            function renderTable(rules) {
                const tbody = document.getElementById('mirror-rules-tbody');
                if (!tbody) return;
                if (!rules.length) {
                    tbody.innerHTML = '<tr><td colspan="9" style="padding: 14px; text-align: center; color: #999;">尚未配置任何镜像规则.</td></tr>';
                    return;
                }
                const rows = rules.map(r => {
                    const condText = (condLabels[r.condition_type] || r.condition_type) +
                        (r.action_name ? ' [' + escapeHtml(r.action_name) + ']' : '') +
                        (r.tool_name ? ' / tool=' + escapeHtml(r.tool_name) : '');
                    const stats = `${r.total_triggered || 0} / ${r.total_success || 0} / ${r.total_failed || 0} / ${r.total_dropped || 0}`;
                    const queueState = `${r.queue_length || 0} / ${r.queue_capacity || r.queue_size || 0}`;
                    const last = r.last_triggered_at || '-';
                    const enabledHtml = r.enabled
                        ? '<span class="mirror-status-badge enabled">已启用</span>'
                        : '<span class="mirror-status-badge disabled">已禁用</span>';
                    return `
                        <tr>
                            <td>${r.id}</td>
                            <td>${escapeHtml(r.name || '')}</td>
                            <td>${enabledHtml}</td>
                            <td>${condText}</td>
                            <td>${r.concurrency}</td>
                            <td><span class="mirror-stats-mini">${queueState}</span></td>
                            <td><span class="mirror-stats-mini">${stats}</span></td>
                            <td>${escapeHtml(last)}</td>
                            <td>
                                <button class="btn btn-sm" onclick="MirrorMgmt.openEditModal(${r.id})">编辑</button>
                                <button class="btn btn-sm" onclick="MirrorMgmt.toggle(${r.id}, ${!r.enabled})">${r.enabled ? '停用' : '启用'}</button>
                                <button class="btn btn-sm" onclick="MirrorMgmt.viewLogs(${r.id})">日志</button>
                                <button class="btn btn-sm btn-danger" onclick="MirrorMgmt.del(${r.id})">删除</button>
                            </td>
                        </tr>
                    `;
                }).join('');
                tbody.innerHTML = rows;
            }

            function escapeHtml(s) {
                return String(s == null ? '' : s)
                    .replace(/&/g, '&amp;')
                    .replace(/</g, '&lt;')
                    .replace(/>/g, '&gt;')
                    .replace(/"/g, '&quot;')
                    .replace(/'/g, '&#39;');
            }

            function showToast(msg, type) {
                if (typeof window.showToast === 'function') {
                    window.showToast(msg, type || 'info');
                } else {
                    console.log('[toast]', msg);
                }
            }

            async function openCreateModal() {
                await ensureMeta();
                fillModal({
                    id: 0,
                    name: '',
                    enabled: true,
                    condition_type: 'always',
                    action_name: '',
                    tool_name: '',
                    callback_script: getDefaultScript(),
                    concurrency: 4,
                    queue_size: 1024,
                    timeout_ms: 30000,
                });
                document.getElementById('mirrorRuleModalTitle').textContent = '新增镜像规则';
                document.getElementById('mirrorRuleModal').style.display = 'flex';
                renderDataSpec();
            }

            async function openEditModal(id) {
                const r = state.cache.find(x => x.id === id);
                if (!r) {
                    showToast('未找到规则 id=' + id, 'error');
                    return;
                }
                await ensureMeta();
                fillModal(r);
                document.getElementById('mirrorRuleModalTitle').textContent = '编辑镜像规则 #' + id;
                document.getElementById('mirrorRuleModal').style.display = 'flex';
                renderDataSpec();
            }

            // resetToDefaultScript 让用户一键把脚本恢复成后端定义的默认模板.
            // 关键词: resetToDefaultScript, mirror 脚本恢复默认
            async function resetToDefaultScript() {
                await ensureMeta();
                if (!confirm('确定要把当前脚本恢复成默认模板吗? 此操作会覆盖现有内容.')) return;
                document.getElementById('mirrorRuleScript').value = getDefaultScript();
            }

            function fillModal(r) {
                document.getElementById('mirrorRuleId').value = r.id || 0;
                document.getElementById('mirrorRuleName').value = r.name || '';
                document.getElementById('mirrorRuleEnabled').checked = !!r.enabled;
                document.getElementById('mirrorRuleCondition').value = r.condition_type || 'always';
                document.getElementById('mirrorRuleAction').value = r.action_name || '';
                document.getElementById('mirrorRuleTool').value = r.tool_name || '';
                document.getElementById('mirrorRuleScript').value = r.callback_script || getDefaultScript();
                document.getElementById('mirrorRuleConcurrency').value = r.concurrency || 4;
                document.getElementById('mirrorRuleQueueSize').value = r.queue_size || 1024;
                document.getElementById('mirrorRuleTimeoutMs').value = r.timeout_ms || 30000;
                const resultEl = document.getElementById('mirrorRuleTestResult');
                if (resultEl) { resultEl.style.display = 'none'; resultEl.innerHTML = ''; resultEl.className = 'mirror-test-result'; }
                onConditionChange();
            }

            function closeModal() {
                document.getElementById('mirrorRuleModal').style.display = 'none';
            }

            // onConditionChange 根据条件类型切换可见字段, 同时根据语义动态更新
            // Action 字段的 (必填/可选) 标签 + placeholder + 帮助文案.
            //
            // 语义:
            //   action_eq            => Action 名称 *必填* (规则核心)
            //   action_call_tool_eq  => Action 名称 *可选过滤器*, 留空匹配三种 call-tool 类
            //                          (call-tool / directly_call_tool / require_tool)
            //
            // 关键词: mirror onConditionChange, Action 名称 必填/可选 切换,
            //        action_call_tool_eq 可选过滤器 UI 提示
            function onConditionChange() {
                const cond = document.getElementById('mirrorRuleCondition').value;
                const showAction = (cond === 'action_eq' || cond === 'action_call_tool_eq');
                const showTool   = (cond === 'action_call_tool_eq');
                document.querySelectorAll('.mirror-cond-action').forEach(el => el.style.display = showAction ? '' : 'none');
                document.querySelectorAll('.mirror-cond-tool').forEach(el => el.style.display = showTool ? '' : 'none');

                const actionInput = document.getElementById('mirrorRuleAction');
                const actionHint  = document.getElementById('mirrorRuleActionLabelHint');
                const actionHelp  = document.getElementById('mirrorRuleActionHelp');
                if (!actionInput || !actionHint || !actionHelp) return;

                if (cond === 'action_eq') {
                    actionHint.textContent = '* 必填';
                    actionHint.className = 'mirror-label-hint required';
                    actionInput.placeholder = '例如: directly_answer / call-tool / require_tool';
                    actionHelp.innerHTML = '必填: 完全匹配响应中解析出的 <code>@action</code> 字段.';
                } else if (cond === 'action_call_tool_eq') {
                    actionHint.textContent = '(可选过滤器)';
                    actionHint.className = 'mirror-label-hint optional';
                    actionInput.placeholder = '留空 = 三种 call-tool 类全匹配; 填了 = 只匹配该 action';
                    actionHelp.innerHTML = '可选: 留空时, <code>call-tool</code> / <code>directly_call_tool</code> / <code>require_tool</code> 三种 action 都会被通配; 填了则只精确匹配该 action.';
                } else {
                    actionHint.textContent = '';
                    actionHint.className = 'mirror-label-hint';
                    actionInput.placeholder = '';
                    actionHelp.innerHTML = '';
                }
            }

            function collectFormPayload() {
                return {
                    name: document.getElementById('mirrorRuleName').value.trim(),
                    enabled: document.getElementById('mirrorRuleEnabled').checked,
                    condition_type: document.getElementById('mirrorRuleCondition').value,
                    action_name: document.getElementById('mirrorRuleAction').value.trim(),
                    tool_name: document.getElementById('mirrorRuleTool').value.trim(),
                    callback_script: document.getElementById('mirrorRuleScript').value,
                    concurrency: parseInt(document.getElementById('mirrorRuleConcurrency').value || '4', 10),
                    queue_size: parseInt(document.getElementById('mirrorRuleQueueSize').value || '1024', 10),
                    timeout_ms: parseInt(document.getElementById('mirrorRuleTimeoutMs').value || '30000', 10),
                };
            }

            async function save() {
                const id = parseInt(document.getElementById('mirrorRuleId').value || '0', 10);
                const payload = collectFormPayload();
                if (!payload.name) {
                    showToast('名称必填', 'error');
                    return;
                }
                if (!payload.callback_script.trim()) {
                    showToast('回调脚本必填', 'error');
                    return;
                }
                try {
                    let resp;
                    if (id > 0) {
                        resp = await fetch('/portal/api/mirror-rules/' + id, {
                            method: 'PUT',
                            credentials: 'same-origin',
                            headers: {'Content-Type': 'application/json'},
                            body: JSON.stringify(payload),
                        });
                    } else {
                        resp = await fetch('/portal/api/mirror-rules', {
                            method: 'POST',
                            credentials: 'same-origin',
                            headers: {'Content-Type': 'application/json'},
                            body: JSON.stringify(payload),
                        });
                    }
                    const j = await resp.json();
                    if (!resp.ok || j.error) {
                        throw new Error(j.error || ('HTTP ' + resp.status));
                    }
                    showToast(id > 0 ? '已更新' : '已创建', 'success');
                    closeModal();
                    refresh();
                } catch (e) {
                    showToast('保存失败: ' + (e.message || e), 'error');
                }
            }

            async function toggle(id, enabled) {
                try {
                    const resp = await fetch('/portal/api/mirror-rules/' + id + '/toggle', {
                        method: 'POST',
                        credentials: 'same-origin',
                        headers: {'Content-Type': 'application/json'},
                        body: JSON.stringify({enabled: enabled}),
                    });
                    const j = await resp.json();
                    if (!resp.ok || j.error) {
                        throw new Error(j.error || ('HTTP ' + resp.status));
                    }
                    showToast(enabled ? '已启用' : '已禁用', 'success');
                    refresh();
                } catch (e) {
                    showToast('切换失败: ' + (e.message || e), 'error');
                }
            }

            async function del(id) {
                if (!confirm('确定要删除该镜像规则 (id=' + id + ')?')) return;
                try {
                    const resp = await fetch('/portal/api/mirror-rules/' + id, {
                        method: 'DELETE',
                        credentials: 'same-origin',
                    });
                    const j = await resp.json();
                    if (!resp.ok || j.error) {
                        throw new Error(j.error || ('HTTP ' + resp.status));
                    }
                    showToast('已删除', 'success');
                    refresh();
                } catch (e) {
                    showToast('删除失败: ' + (e.message || e), 'error');
                }
            }

            // fmtSaveBytes 把字节数格式化为人类可读单位 (B/KiB/MiB).
            function fmtSaveBytes(n) {
                n = Number(n) || 0;
                if (n >= 1024 * 1024) return (n / (1024 * 1024)).toFixed(2) + ' MiB';
                if (n >= 1024) return (n / 1024).toFixed(2) + ' KiB';
                return n + ' B';
            }

            // renderTestSaveBlock 渲染试运行里 save() 的调用反馈块:
            //   - 没调 save(): 中性提示。
            //   - 调了但落盘未启用: 黄色提示 (生产也不会落盘, 引导去开启)。
            //   - 调了且已启用: 绿色提示 (生产会落盘, 试运行本身不写)。
            // 关键词: renderTestSaveBlock, save 试运行反馈
            function renderTestSaveBlock(j) {
                const calls = Number(j.save_calls) || 0;
                const bytes = Number(j.save_bytes) || 0;
                const enabled = !!j.save_enabled;
                if (calls === 0) {
                    return '<div class="mirror-test-save" style="border-left-color:#64748b;">'
                        + '<div style="color:#94a3b8;">save(): 本次脚本未调用 save()，不会落盘归档。</div>'
                        + '</div>';
                }
                const head = enabled
                    ? '<span style="color:#34d399; font-weight:600;">save() 已调用 ' + calls + ' 次</span> <span style="color:#cbd5e1;">将写入 ' + escapeHtml(fmtSaveBytes(bytes)) + '</span> <span style="color:#94a3b8;">(试运行不实际写盘)</span>'
                    : '<span style="color:#fbbf24; font-weight:600;">save() 已调用 ' + calls + ' 次</span> <span style="color:#cbd5e1;">将写入 ' + escapeHtml(fmtSaveBytes(bytes)) + '</span>';
                const hint = enabled
                    ? '<div style="color:#94a3b8; font-size:11px; margin-top:2px;">落盘已启用：生产环境命中此规则时会把内容写入归档。</div>'
                    : '<div style="color:#fbbf24; font-size:11px; margin-top:2px;">注意：落盘当前<strong>未启用</strong>，生产环境也不会真正写入。请到「流量镜像 → 数据落盘设置」勾选「启用 save() 落盘」。</div>';
                let preview = '';
                if (j.save_preview) {
                    preview = '<div style="color:#94a3b8; font-size:11px; margin-top:6px;">// save() 首次写入内容预览:</div>'
                        + '<pre class="mirror-test-save-preview">' + escapeHtml(j.save_preview) + '</pre>';
                }
                const border = enabled ? '#34d399' : '#fbbf24';
                return '<div class="mirror-test-save" style="border-left-color:' + border + ';">'
                    + '<div>' + head + '</div>' + hint + preview
                    + '</div>';
            }

            // testCurrent 调用后端 /test 接口同步跑一次脚本, 把结果按 success/fail
            // 分别用绿色/红色头条 + JSON body 渲染. 关键词: mirror testCurrent, 试运行结果美化
            async function testCurrent() {
                const id = parseInt(document.getElementById('mirrorRuleId').value || '0', 10);
                const payload = collectFormPayload();
                const url = '/portal/api/mirror-rules/' + (id > 0 ? id : '0') + '/test';
                const resultEl = document.getElementById('mirrorRuleTestResult');
                if (resultEl) {
                    resultEl.style.display = '';
                    resultEl.className = 'mirror-test-result';
                    resultEl.innerHTML = '<div class="mirror-test-result-head"><span style="color:#fbbf24;">Running...</span></div>';
                }
                try {
                    const resp = await fetch(url, {
                        method: 'POST',
                        credentials: 'same-origin',
                        headers: {'Content-Type': 'application/json'},
                        body: JSON.stringify({
                            script: payload.callback_script,
                            snapshot: {}
                        }),
                    });
                    const j = await resp.json();
                    if (!resultEl) return;
                    const executed = !!j.executed;
                    const dur = (j.duration_ms == null ? '-' : j.duration_ms) + ' ms';
                    const tag = executed
                        ? '<span class="mirror-test-status-ok">SUCCESS</span>'
                        : '<span class="mirror-test-status-fail">FAILED</span>';
                    const errLine = j.error
                        ? '<div style="color:#f87171; margin-bottom:6px;">error: ' + escapeHtml(j.error) + '</div>'
                        : '';
                    const snapText = j.snapshot ? JSON.stringify(j.snapshot, null, 2) : '';
                    // save() 调用反馈: 让用户知道 save 调没调、会写多少、生产是否会真落盘。
                    // 关键词: testCurrent save 反馈展示
                    const saveBlock = renderTestSaveBlock(j);
                    resultEl.className = 'mirror-test-result' + (executed ? '' : '');
                    resultEl.innerHTML = `
                        <div class="mirror-test-result-head">
                            ${tag}<span style="color:#cbd5e1;">duration=${escapeHtml(String(dur))}</span>
                        </div>
                        ${errLine}
                        ${saveBlock}
                        <div style="color:#94a3b8; font-size:11px; margin-top:6px;">// sample snapshot passed to handle(data):</div>
                        <div>${escapeHtml(snapText)}</div>
                    `;
                } catch (e) {
                    if (resultEl) {
                        resultEl.className = 'mirror-test-result error';
                        resultEl.innerHTML = '<div class="mirror-test-result-head"><span class="mirror-test-status-fail">FAILED</span></div>' +
                            '<div>试运行失败: ' + escapeHtml(e.message || String(e)) + '</div>';
                    }
                }
            }

            async function viewLogs(id) {
                const r = state.cache.find(x => x.id === id);
                document.getElementById('mirrorLogsRuleName').textContent = r ? r.name : ('#' + id);
                const list = document.getElementById('mirrorLogsList');
                if (list) list.innerHTML = '<div style="color:#999; padding: 12px;">Loading...</div>';
                document.getElementById('mirrorLogsModal').style.display = 'flex';
                try {
                    const resp = await fetch('/portal/api/mirror-rules/' + id + '/logs', {credentials: 'same-origin'});
                    const j = await resp.json();
                    if (!resp.ok || j.error) {
                        throw new Error(j.error || ('HTTP ' + resp.status));
                    }
                    const logs = j.logs || [];
                    if (!logs.length) {
                        list.innerHTML = '<div style="color:#999; padding: 12px;">尚无调用记录.</div>';
                        return;
                    }
                    list.innerHTML = logs.map(l => {
                        const cls = l.success ? 'success' : 'failure';
                        const tag = l.success ? '<span style="color:#2e7d32; font-weight:600;">SUCCESS</span>' : '<span style="color:#c62828; font-weight:600;">FAILED</span>';
                        // save() 反馈: 这次调了几次 / 落盘几次 / 字节数, 让生产环境也能确认。
                        // 关键词: viewLogs save_calls 展示
                        const calls = Number(l.save_calls) || 0;
                        const persisted = Number(l.save_persisted) || 0;
                        const sbytes = Number(l.save_bytes) || 0;
                        let saveLine = '';
                        if (calls > 0) {
                            const ok = persisted > 0;
                            const color = ok ? '#2e7d32' : '#ef6c00';
                            const fmt = (sbytes >= 1024 * 1024) ? (sbytes / (1024 * 1024)).toFixed(2) + ' MiB'
                                : (sbytes >= 1024) ? (sbytes / 1024).toFixed(2) + ' KiB' : sbytes + ' B';
                            saveLine = '<div style="color:' + color + ';">save: 调用 ' + calls + ' 次 / 落盘 ' + persisted + ' 次 / ' + escapeHtml(fmt)
                                + (ok ? '' : '（未落盘，可能落盘未启用）') + '</div>';
                        }
                        return `<div class="mirror-log-entry ${cls}">
                            <div>${escapeHtml(l.timestamp || '')} | req_id=${escapeHtml(l.req_id || '')} | dur=${l.duration_ms || 0}ms | ${tag}</div>
                            ${l.error_message ? '<div style="color:#c62828;">' + escapeHtml(l.error_message) + '</div>' : ''}
                            ${saveLine}
                            ${l.stdout ? '<div style="color:#555;">stdout: ' + escapeHtml(l.stdout) + '</div>' : ''}
                        </div>`;
                    }).join('');
                } catch (e) {
                    if (list) list.innerHTML = '<div style="color:#c62828; padding: 12px;">加载日志失败: ' + escapeHtml(e.message || String(e)) + '</div>';
                }
            }

            function closeLogs() {
                document.getElementById('mirrorLogsModal').style.display = 'none';
            }

            return {
                refresh: refresh,
                openCreateModal: openCreateModal,
                openEditModal: openEditModal,
                closeModal: closeModal,
                onConditionChange: onConditionChange,
                save: save,
                toggle: toggle,
                del: del,
                testCurrent: testCurrent,
                resetToDefaultScript: resetToDefaultScript,
                viewLogs: viewLogs,
                closeLogs: closeLogs,
                saveStorageConfig: saveStorageConfig,
            };
        })();

        // MirrorRecords: 「镜像数据」页面逻辑, 加载最近落盘记录并人性化展示.
        // 关键词: MirrorRecords, 最近落盘记录查看, 人性化字段 + 原始 JSON 折叠
        const MirrorRecords = (function () {
            let seq = 0;

            // tsHuman 把毫秒时间戳转成本地可读时间.
            function tsHuman(ms) {
                const n = Number(ms) || 0;
                if (n <= 0) return '-';
                try { return new Date(n).toLocaleString(); } catch (e) { return String(n); }
            }

            // pickUserMessage 从 request_messages 里取最后一条 user 文本片段.
            function pickUserMessage(rec) {
                const msgs = rec && rec.request_messages;
                if (!Array.isArray(msgs) || !msgs.length) return '';
                for (let i = msgs.length - 1; i >= 0; i--) {
                    const m = msgs[i] || {};
                    const role = (m.role || '').toLowerCase();
                    if (role === 'user' && m.content) return String(m.content);
                }
                const last = msgs[msgs.length - 1] || {};
                return last.content ? String(last.content) : '';
            }

            function clip(s, n) {
                s = (s == null) ? '' : String(s);
                if (s.length <= n) return s;
                return s.slice(0, n) + '…';
            }

            // asText 把任意值转纯文本: 字符串原样, 其它 JSON 缩进序列化。
            function asText(v) {
                if (v == null) return '';
                if (typeof v === 'string') return v;
                try { return JSON.stringify(v, null, 2); } catch (e) { return String(v); }
            }

            // buildRequestText 把 request_messages 渲染成可读纯文本 (按 role 分段)。
            // 关键词: buildRequestText, 请求纯文本对照
            function buildRequestText(rec) {
                const msgs = rec && rec.request_messages;
                if (!Array.isArray(msgs) || !msgs.length) return '(no request_messages)';
                return msgs.map(function (m) {
                    m = m || {};
                    const role = m.role || 'unknown';
                    let block = '===== ' + role + ' =====\n' + asText(m.content);
                    if (Array.isArray(m.tool_calls) && m.tool_calls.length) {
                        block += '\n[tool_calls]\n' + asText(m.tool_calls);
                    }
                    return block;
                }).join('\n\n');
            }

            // buildResponseText 把响应渲染成可读纯文本 (reasoning + answer + tool_calls)。
            // 关键词: buildResponseText, 响应纯文本对照
            function buildResponseText(rec) {
                const parts = [];
                if (rec.response_reason) {
                    parts.push('===== reasoning =====\n' + asText(rec.response_reason));
                }
                parts.push('===== answer =====\n' + asText(rec.response_text));
                if (Array.isArray(rec.tool_calls) && rec.tool_calls.length) {
                    parts.push('===== tool_calls =====\n' + asText(rec.tool_calls));
                }
                return parts.join('\n\n');
            }

            // copyEl 复制某个元素的纯文本内容到剪贴板, 并在按钮上给出短暂反馈。
            // 关键词: copyEl, 一键复制, navigator.clipboard 带 textarea 兜底
            function copyEl(id, btn) {
                const el = document.getElementById(id);
                if (!el) return;
                const text = el.textContent || '';
                const feedback = function (ok) {
                    if (!btn) return;
                    const orig = btn.getAttribute('data-label') || btn.textContent;
                    btn.setAttribute('data-label', orig);
                    btn.textContent = ok ? '已复制' : '复制失败';
                    setTimeout(function () { btn.textContent = orig; }, 1500);
                };
                const fallback = function () {
                    try {
                        const ta = document.createElement('textarea');
                        ta.value = text;
                        ta.style.position = 'fixed';
                        ta.style.opacity = '0';
                        document.body.appendChild(ta);
                        ta.focus();
                        ta.select();
                        const ok = document.execCommand('copy');
                        document.body.removeChild(ta);
                        feedback(ok);
                    } catch (e) { feedback(false); }
                };
                if (navigator.clipboard && navigator.clipboard.writeText) {
                    navigator.clipboard.writeText(text).then(function () { feedback(true); }, fallback);
                } else {
                    fallback();
                }
            }

            // toggleEl 通用展开/收起: 切换目标元素显隐并在触发器上切换文案。
            // 关键词: toggleEl, 通用展开收起
            function toggleEl(id, el, showLabel, hideLabel) {
                const target = document.getElementById(id);
                if (!target) return;
                const show = target.style.display === 'none' || target.style.display === '';
                target.style.display = show ? 'flex' : 'none';
                if (el && showLabel && hideLabel) el.textContent = show ? hideLabel : showLabel;
            }

            // renderRecord 把一条记录渲染为一张卡: 人性化关键字段 + 可展开原始 JSON.
            function renderRecord(rec, idx) {
                const id = 'mirror-rec-raw-' + (seq++) ;
                const model = escapeHtml(rec.model || rec.type_name || '(未知模型)');
                const action = rec.action ? escapeHtml(rec.action) : '';
                const ts = escapeHtml(tsHuman(rec.timestamp_ms));
                const free = rec.is_free_model ? '免费' : '计费';
                const stream = rec.stream ? '流式' : '非流式';
                const dur = (Number(rec.duration_ms) || 0);
                const inB = (Number(rec.input_bytes) || 0);
                const outB = (Number(rec.output_bytes) || 0);
                const toolCalls = Array.isArray(rec.tool_calls) ? rec.tool_calls.length : 0;
                const userMsg = escapeHtml(clip(pickUserMessage(rec), 200));
                const respText = escapeHtml(clip(rec.response_text, 200));

                let usageStr = '';
                if (rec.usage && typeof rec.usage === 'object') {
                    const pt = rec.usage.prompt_tokens || rec.usage.PromptTokens || 0;
                    const ct = rec.usage.completion_tokens || rec.usage.CompletionTokens || 0;
                    const tt = rec.usage.total_tokens || rec.usage.TotalTokens || 0;
                    if (pt || ct || tt) usageStr = 'tokens ' + pt + '/' + ct + ' (合计 ' + tt + ')';
                }

                let raw = '';
                try { raw = JSON.stringify(rec, null, 2); } catch (e) { raw = String(rec); }

                // 请求/响应对照: 纯文本代码块, 各带一键复制, 便于核对真实内容。
                // 关键词: renderRecord 请求/响应对照, 纯文本 + 复制
                const s = (seq++);
                const cmpId = 'mirror-rec-cmp-' + s;
                const reqId = 'mirror-rec-req-' + s;
                const respId = 'mirror-rec-resp-' + s;
                const reqFull = escapeHtml(buildRequestText(rec));
                const respFull = escapeHtml(buildResponseText(rec));
                const compareBlock = '<div id="' + cmpId + '" class="mirror-rec-compare" style="display:none;">'
                    + '<div class="mirror-rec-col">'
                    +   '<div class="mirror-rec-col-head"><span>Request</span>'
                    +     '<button class="mirror-rec-copy-btn" onclick="MirrorRecords.copyEl(\'' + reqId + '\', this)">复制</button></div>'
                    +   '<pre id="' + reqId + '" class="mirror-rec-text">' + reqFull + '</pre>'
                    + '</div>'
                    + '<div class="mirror-rec-col">'
                    +   '<div class="mirror-rec-col-head"><span>Response</span>'
                    +     '<button class="mirror-rec-copy-btn" onclick="MirrorRecords.copyEl(\'' + respId + '\', this)">复制</button></div>'
                    +   '<pre id="' + respId + '" class="mirror-rec-text">' + respFull + '</pre>'
                    + '</div>'
                    + '</div>';

                const chips = [];
                chips.push('<span class="mirror-rec-chip">' + free + '</span>');
                chips.push('<span class="mirror-rec-chip">' + stream + '</span>');
                if (action) chips.push('<span class="mirror-rec-chip" style="background:#fff3e0; border-color:#ffcc80;">action: ' + action + '</span>');
                if (toolCalls > 0) chips.push('<span class="mirror-rec-chip">tool_calls: ' + toolCalls + '</span>');
                chips.push('<span class="mirror-rec-chip">耗时 ' + dur + 'ms</span>');
                chips.push('<span class="mirror-rec-chip">收/发 ' + formatBytesHuman(inB) + ' / ' + formatBytesHuman(outB) + '</span>');
                if (usageStr) chips.push('<span class="mirror-rec-chip">' + escapeHtml(usageStr) + '</span>');

                return '<div class="mirror-rec-card">'
                    + '<div class="mirror-rec-head">'
                    + '<span class="mirror-rec-idx">#' + (idx + 1) + '</span>'
                    + '<code class="mirror-rec-model">' + model + '</code>'
                    + '<span class="mirror-rec-ts">' + ts + '</span>'
                    + '</div>'
                    + '<div class="mirror-rec-chips">' + chips.join('') + '</div>'
                    + (userMsg ? '<div class="mirror-rec-line"><span class="mirror-rec-label">请求:</span> ' + userMsg + '</div>' : '')
                    + (respText ? '<div class="mirror-rec-line"><span class="mirror-rec-label">响应:</span> ' + respText + '</div>' : '')
                    + '<div class="mirror-rec-toggle-bar">'
                    +   '<span class="mirror-rec-toggle" onclick="MirrorRecords.toggleEl(\'' + cmpId + '\', this, \'展开 请求/响应对照\', \'收起 请求/响应对照\')">展开 请求/响应对照</span>'
                    +   '<span class="mirror-rec-toggle" onclick="MirrorRecords.toggleRaw(\'' + id + '\', this)">展开原始 JSON</span>'
                    + '</div>'
                    + compareBlock
                    + '<pre id="' + id + '" class="mirror-rec-raw" style="display:none;">' + escapeHtml(raw) + '</pre>'
                    + '</div>';
            }

            function toggleRaw(id, el) {
                const pre = document.getElementById(id);
                if (!pre) return;
                const show = pre.style.display === 'none';
                pre.style.display = show ? 'block' : 'none';
                if (el) el.textContent = show ? '收起原始 JSON' : '展开原始 JSON';
            }

            async function load() {
                const statusEl = document.getElementById('mirror-records-status');
                const listEl = document.getElementById('mirror-records-list');
                const countEl = document.getElementById('mirror-records-count');
                const n = (countEl && parseInt(countEl.value, 10)) || 20;
                if (statusEl) statusEl.textContent = '加载中…';
                if (listEl) listEl.innerHTML = '';
                try {
                    const resp = await fetch('/portal/api/mirror-records/recent?n=' + n, {credentials: 'same-origin'});
                    const j = await resp.json();
                    if (isAuthError(j)) { handleAuthError(); return; }
                    if (!j || !j.success) {
                        if (statusEl) statusEl.textContent = (j && (j.message || j.error)) || '加载失败';
                        return;
                    }
                    const records = Array.isArray(j.records) ? j.records : [];
                    if (!records.length) {
                        if (statusEl) statusEl.textContent = '暂无落盘记录 (可能未启用落盘, 或还没有命中的镜像规则调用 save()).';
                        return;
                    }
                    if (statusEl) statusEl.textContent = '共 ' + records.length + ' 条 (最新在前)';
                    if (listEl) listEl.innerHTML = records.map(renderRecord).join('');
                } catch (e) {
                    console.error('mirror-records: load failed', e);
                    if (statusEl) statusEl.textContent = '加载失败: ' + (e.message || e);
                }
            }

            return { load: load, toggleRaw: toggleRaw, toggleEl: toggleEl, copyEl: copyEl };
        })();
        window.MirrorRecords = MirrorRecords;
