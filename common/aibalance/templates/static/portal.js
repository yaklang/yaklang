        // 全局变量
        let resizeTimer;
        let currentContextMenuProviderId = null; // For provider actions
        let contextApiIdForEdit = null;          // For API key model editing
        let contextModelsForEdit = null;         // For API key model editing
        let isHealthCheckInProgress = false;
        let isProviderConfigValidated = false; // For add provider form validation

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

            // 填充类型选择框
            const typeNameSelect = document.getElementById('typeName');
            if (typeNameSelect) {
                // 保留第一个空选项
                const firstOption = typeNameSelect.querySelector('option:first-child');
                typeNameSelect.innerHTML = '';
                if (firstOption) {
                    typeNameSelect.appendChild(firstOption);
                }
                
                // 添加从服务器获取的类型选项
                autoCompleteData.model_types.forEach(type => {
                    const option = document.createElement('option');
                    option.value = type;
                    option.textContent = type;
                    typeNameSelect.appendChild(option);
                });
                
                // 如果没有类型选项，添加默认选项
                if (typeNameSelect.options.length <= 1) {
                    // 后端未返回数据时，添加一些常见类型作为默认选项
                    const defaultTypes = [
                        'chat', 
                        'completion', 
                        'embedding'
                    ];
                    
                    defaultTypes.forEach(type => {
                        const option = document.createElement('option');
                        option.value = type;
                        option.textContent = type;
                        typeNameSelect.appendChild(option);
                    });
                }
            }
            
            // 添加输入事件处理器
            const domainInput = document.getElementById('domainOrURL');
            if (domainInput) {
                // 根据选择的类型预填充常见域名
                document.getElementById('typeName').addEventListener('change', function() {
                    const selectedType = this.value;
                    let suggestedDomain = '';
                    
                    // 根据类型提供默认域名建议
                    if (['chat', 'completion', 'embedding'].includes(selectedType.toLowerCase())) {
                        suggestedDomain = 'api.openai.com';
                    }
                    
                    // 如果域名输入框为空，则填充默认值
                    if (!domainInput.value.trim()) {
                        domainInput.value = suggestedDomain;
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

        function escapeHtml(text) {
            if (!text) return '';
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
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

        // 新增：确认并生成 API Key 的函数
        function confirmAndGenerateApiKey() {
            const allowedModelsSelect = document.getElementById('allowedModelsSelect');
            const selectedModels = Array.from(allowedModelsSelect.selectedOptions).map(option => option.value);

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
            const allowedModelsSelect = document.getElementById('allowedModelsSelect');
            const selectedModels = Array.from(allowedModelsSelect.selectedOptions).map(option => option.value);
            
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
                setTimeout(() => window.location.reload(), 1000);
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
                    setTimeout(() => window.location.reload(), 1000);
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
                setTimeout(() => window.location.reload(), 1000);
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
            document.getElementById('apiKeySuccessModal').style.display = 'block';
        }

        function closeApiKeyModal(reload = false) {
            document.getElementById('apiKeySuccessModal').style.display = 'none';
            if (reload) {
                window.location.reload();
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

                setTimeout(() => window.location.reload(), 1000); // Refresh page
            } catch (error) {
                showToast('禁用失败: ' + error.message, 'error');
                console.error('Error disabling multiple API keys:', error); // Use common/log
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
                setTimeout(() => window.location.reload(), 1000);
            } catch (error) {
                showToast('删除失败: ' + error.message, 'error');
            }
        }

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

                setTimeout(() => window.location.reload(), 1000); // Refresh page
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

            // 填充类型选择框
            const typeNameSelect = document.getElementById('typeName');
            if (typeNameSelect) {
                // 保留第一个空选项
                const firstOption = typeNameSelect.querySelector('option:first-child');
                typeNameSelect.innerHTML = '';
                if (firstOption) {
                    typeNameSelect.appendChild(firstOption);
                }
                
                // 添加从服务器获取的类型选项
                autoCompleteData.model_types.forEach(type => {
                    const option = document.createElement('option');
                    option.value = type;
                    option.textContent = type;
                    typeNameSelect.appendChild(option);
                });
                
                // 如果没有类型选项，添加默认选项
                if (typeNameSelect.options.length <= 1) {
                    // 后端未返回数据时，添加一些常见类型作为默认选项
                    const defaultTypes = [
                        'chat', 
                        'completion', 
                        'embedding'
                    ];
                    
                    defaultTypes.forEach(type => {
                        const option = document.createElement('option');
                        option.value = type;
                        option.textContent = type;
                        typeNameSelect.appendChild(option);
                    });
                }
            }
            
            // 添加输入事件处理器
            const domainInput = document.getElementById('domainOrURL');
            if (domainInput) {
                // 根据选择的类型预填充常见域名
                document.getElementById('typeName').addEventListener('change', function() {
                    const selectedType = this.value;
                    let suggestedDomain = '';
                    
                    // 根据类型提供默认域名建议
                    if (['chat', 'completion', 'embedding'].includes(selectedType.toLowerCase())) {
                        suggestedDomain = 'api.openai.com';
                    }
                    
                    // 如果域名输入框为空，则填充默认值
                    if (!domainInput.value.trim()) {
                        domainInput.value = suggestedDomain;
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
                    validationResultDiv.textContent = `验证成功: ${result.message || '配置有效。现在可以添加提供者。'}`;
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

                // 仔细设置 Select 的值
                let typeFound = false;
                for (let i = 0; i < typeSelect.options.length; i++) {
                    if (typeSelect.options[i].value === typeName) {
                        typeSelect.value = typeName;
                        typeFound = true;
                        break;
                    }
                }
                if (!typeFound) {
                     console.warn(`Type "${typeName}" not found in select options. Leaving type selection unchanged.`); // Debug log
                     typeSelect.value = ""; // 重置为默认提示选项
                } else {
                     console.log(`Successfully set type to "${typeName}"`); // Debug log
                     // 触发 change 事件以处理可能的依赖逻辑（如域名建议）
                     typeSelect.dispatchEvent(new Event('change'));
                }

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

        // 新增：编辑允许模型的函数 (纯 JavaScript 版本 - 使用 Select)
        function editAllowedModels(id, currentAllowedModelsString) {
            console.log("[Debug] editAllowedModels (Pure JS with Select) called with ID:", id, "Models:", currentAllowedModelsString);
            
            document.getElementById('editApiKeyId').value = id; // Hidden input to store the ID
            document.getElementById('editingApiKeyIdDisplay').textContent = id; // Span to display the ID
            
            const selectElement = document.getElementById('editAllowedModelsSelectModal');
            selectElement.innerHTML = ''; // Clear existing options

            // 1. 获取所有可用的 WrapperName
            // 优先使用 autoCompleteData.wrapper_names，如果它已被填充
            let availableWrapperNames = new Set();
            if (autoCompleteData && autoCompleteData.wrapper_names && autoCompleteData.wrapper_names.length > 0) {
                autoCompleteData.wrapper_names.forEach(name => availableWrapperNames.add(name));
            } else {
                // Fallback: 从API Key列表的当前显示模型中提取，或者从Provider列表提取
                // 这里我们简化，假设populateAllowedModelsSelector已经运行过，或者autoCompleteData会包含所需信息
                // 或者，更可靠地，再次扫描provider表
                const providerRows = document.querySelectorAll('#all #provider-table-body tr'); // Target provider table specifically
                providerRows.forEach(row => {
                    const wrapperNameCell = row.cells[3]; // 第4列是提供者名称 (WrapperName)
                    if (wrapperNameCell) {
                        const wrapperName = wrapperNameCell.getAttribute('data-full-text') || wrapperNameCell.textContent.trim();
                        if (wrapperName) {
                            availableWrapperNames.add(wrapperName);
                        }
                    }
                });
            }
            
            if (availableWrapperNames.size === 0) {
                const option = document.createElement('option');
                option.textContent = '没有可用的模型提供者';
                option.disabled = true;
                selectElement.appendChild(option);
                console.warn("[Debug] No available wrapper names to populate select modal.");
            } else {
                Array.from(availableWrapperNames).sort().forEach(name => { // Sort for better UX
                    const option = document.createElement('option');
                    option.value = name;
                    option.textContent = name;
                    selectElement.appendChild(option);
                });
            }

            // 2. 预选当前API Key允许的模型
            const currentlyAllowedModelsArray = currentAllowedModelsString ? currentAllowedModelsString.split(',').map(m => m.trim()).filter(m => m) : [];
            for (let i = 0; i < selectElement.options.length; i++) {
                if (currentlyAllowedModelsArray.includes(selectElement.options[i].value)) {
                    selectElement.options[i].selected = true;
                } else {
                    selectElement.options[i].selected = false; // Ensure others are not selected
                }
            }
            
            const modalElement = document.getElementById('editAllowedModelsModal');
            if (modalElement) {
                modalElement.style.display = 'block';
                console.log("[Debug] editAllowedModelsModal shown via style.display");
            } else {
                console.error("[Debug] editAllowedModelsModal element not found!");
            }
        }

        // 新增：关闭编辑模态框的函数
        function closeEditAllowedModelsModal() {
            const modalElement = document.getElementById('editAllowedModelsModal');
            if (modalElement) {
                modalElement.style.display = 'none';
                console.log("[Debug] editAllowedModelsModal hidden via style.display");
            }
        }

        // 新增：保存编辑的模型 (确保从新的 select 元素读取)
        function saveAllowedModels() {
            const id = document.getElementById('editApiKeyId').value;
            // const allowedModels = document.getElementById('editAllowedModelsTextarea').value; // 旧代码：使用 textarea
            const selectElement = document.getElementById('editAllowedModelsSelectModal');
            const selectedModels = Array.from(selectElement.selectedOptions).map(option => option.value);
            const allowedModelsString = selectedModels.join(',');


            console.log("[Debug] saveAllowedModels called. ID:", id, "Selected Models String:", allowedModelsString);

            fetch(`/portal/update-api-key-allowed-models/${id}`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    allowed_models: allowedModelsString
                })
            })
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    closeEditAllowedModelsModal(); // Close modal on success
                    showToast('允许的模型已更新', 'success'); // Show success toast
                    // 刷新页面以显示更新后的数据
                    setTimeout(() => window.location.reload(), 1500); // Delay reload slightly
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

            // 显示弹窗
            modal.style.display = 'block';

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
            modal.style.display = 'block';
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
        function openEditModelModal(name, description, tags) {
            document.getElementById('editModelName').value = name;
            document.getElementById('editModelDescription').value = description;
            document.getElementById('editModelTags').value = tags;
            document.getElementById('editModelMetaModal').style.display = 'block';
        }

        function closeEditModelModal() {
            document.getElementById('editModelMetaModal').style.display = 'none';
        }

        function saveModelMeta() {
            const name = document.getElementById('editModelName').value;
            const description = document.getElementById('editModelDescription').value;
            const tags = document.getElementById('editModelTags').value;

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
