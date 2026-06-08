// ==================== OPS Portal JavaScript ====================

// State
let userInfo = null;
let availableModels = [];
let myApiKeys = [];

// ==================== i18n（国际化：默认中文，可切换英文）====================
// 关键词: OPS portal i18n, 默认中文 default zh, 可切换英文 toggle en, localStorage 记忆
// 设计：
//   - I18N 字典持有 zh / en 两套文案；t(key, params) 取当前语言文案，支持 ${name} 占位替换；
//   - 静态 HTML 通过 data-i18n / data-i18n-html / data-i18n-placeholder / data-i18n-title 注解，
//     applyI18n() 统一回填；动态渲染处直接调用 t()；
//   - 默认 zh，选择记忆到 localStorage('ops_portal_lang')。
const OPS_LANG_KEY = 'ops_portal_lang';
let currentLang = 'zh';

const I18N = {
    zh: {
        'header.badge': '运营门户',
        'header.welcome': '欢迎，',
        'header.homepage': '主页',
        'header.logout': '退出登录',
        'lang.toggleToEn': 'English',
        'lang.toggleToZh': '中文',

        'stats.apiKeys': '已创建 API Key',
        'stats.defaultLimit': '默认流量额度',
        'stats.status': '账户状态',

        'tab.createKey': '创建 API Key',
        'tab.myKeys': '我的 API Key',
        'tab.apiUsage': 'API 使用指南',
        'tab.myInfo': '我的信息',
        'tab.settings': '设置',

        'common.selectModels': '选择模型',
        'common.selectAll': '全选',
        'common.clear': '清空',
        'common.loadingModels': '正在加载模型...',
        'common.noModels': '暂无可用模型',
        'common.loadModelsFailed': '加载模型失败',
        'common.globAdvanced': 'Glob 模式匹配（高级）',
        'common.selectedLabel': '已选：',
        'common.modelsUnit': ' 个模型',
        'common.noneSelected': '未选择',
        'preview.empty': '<strong>已选：</strong> <span style="color: #888;">未选择</span>',
        'common.moreSuffix': ' 个',
        'common.refresh': '刷新',
        'common.cancel': '取消',
        'common.active': '正常',
        'common.inactive': '已禁用',
        'common.unlimited': '不限制',
        'common.networkError': '网络错误',
        'common.copy': '复制',

        'create.allowedModels': '允许的模型',
        'create.globHint': '逗号分隔的 glob 模式。使用 <code>*</code> 作为通配符。例如：<code>memfit-*</code> 匹配所有 memfit 模型。',
        'create.globPh': '例如：memfit-*,qwen*,gpt-4*',
        'create.userBinding': '绑定用户信息（可选）',
        'create.usernamePh': '用户名（可重复）',
        'create.remarkPh': '备注（自由文本）',
        'create.metainfoPh': 'metainfo（JSON 文本，用于 OAuth/外部系统绑定，可选）',
        'create.tokenSettings': 'Token 设置',
        'create.recommended': '（推荐）',
        'create.unlimitedToken': '不限制 Token',
        'tokenDesc.unlimited': 'API Key 将不受 Token 限制',
        'tokenDesc.custom': '在下方设置自定义 Token 额度',
        'create.tokenBillingHint': '推荐的计费维度，按加权倍率（输入/输出/缓存）聚合计算。',
        'unit.mTokens': 'M tokens',
        'unit.kTokens': 'K tokens',
        'unit.tokens': 'tokens',
        'create.generate': '生成 API Key',
        'create.newKeyLabel': '你的新 API Key：',
        'create.saveNotice': '请妥善保存此 Key，它只会显示一次！',
        'create.copyClipboard': '复制到剪贴板',

        'myKeys.title': '我的 API Key',
        'myKeys.loading': '正在加载 API Key',
        'myKeys.thApiKey': 'API Key',
        'myKeys.thUserRemark': '用户名 / 备注',
        'myKeys.thModels': '模型',
        'myKeys.thTokenUsed': 'Token 计费用量',
        'myKeys.thTokenLimit': 'Token 额度 *',
        'myKeys.thCreatedAt': '创建时间',
        'myKeys.thActions': '操作',
        'myKeys.titleUserRemark': '绑定用户名（可重复）/ 备注',
        'myKeys.titleTokenUsed': 'Token 计费用量（按倍率加权）',
        'myKeys.titleTokenLimit': 'Token 额度（唯一计费维度）',
        'myKeys.empty': '你还没有创建任何 API Key。',
        'myKeys.createFirst': '创建你的第一个 API Key',
        'myKeys.edit': '编辑',
        'myKeys.delete': '删除',
        'myKeys.loadFailed': '加载 Key 失败',
        'myKeys.thStatus': '状态',
        'myKeys.searchPh': '搜索用户名 / 备注 / Key',
        'myKeys.statusAll': '全部状态',
        'myKeys.statusActive': '仅启用',
        'myKeys.statusInactive': '仅禁用',
        'myKeys.search': '搜索',
        'myKeys.clearFilter': '清除',
        'myKeys.enable': '启用',
        'myKeys.disable': '禁用',

        'pag.total': '共 ${total} 个 Key，第 ${page}/${pages} 页',
        'pag.first': '首页',
        'pag.prev': '上一页',
        'pag.next': '下一页',
        'pag.last': '末页',
        'pag.gotoPrefix': '跳转到',
        'pag.go': '跳转',
        'pag.perPage': '${n}/页',

        'usage.title': 'API 使用指南',
        'usage.s1.h': '1. 创建带流量限制的 API Key',
        'usage.s1.p': '创建带指定流量额度的 API Key（例如 50 MB = 52428800 字节）：',
        'usage.s1.code': 'cURL - 创建 50MB 限额 Key',
        'usage.s2.h': '2. 创建不限流量的 API Key',
        'usage.s2.p': '创建一个不受流量限制的 API Key：',
        'usage.s2.code': 'cURL - 创建不限额 Key',
        'usage.s3.h': '3. 使用 Glob 模式创建 API Key',
        'usage.s3.p': '使用 glob 模式匹配多个模型（例如 <code>gpt-*</code> 匹配所有 GPT 模型）：',
        'usage.s3.code': 'cURL - 使用 Glob 模式创建 Key',
        'usage.s4.h': '4. 列出与搜索我的 API Key',
        'usage.s4.p': '获取由你的运营账户创建的全部 API Key，支持分页、关键字搜索与状态筛选。你只能看到自己创建的 Key，看不到其他运营用户的 Key。',
        'usage.s4.code': 'cURL - 列出 Key',
        'usage.s4.filter.p': '按关键字搜索（匹配绑定用户名 / 备注 / api_key），并按状态筛选。下面的示例查找用户名、备注或 Key 中包含 "alice" 的已启用 Key：',
        'usage.s4.filter.code': 'cURL - 搜索与筛选 Key',
        'usage.s4.queryParams.h': '查询参数（列出与搜索）',
        'usage.s4.queryParams.q': '宽匹配关键字搜索（可选）。在 <code>username</code>、<code>remark</code>、<code>api_key</code> 上做模糊匹配。特殊字符会被转义（防 SQL 注入）。',
        'usage.s4.queryParams.username': '按绑定用户名筛选（可选，模糊匹配）。',
        'usage.s4.queryParams.active': '按状态筛选（可选）。<code>true</code>/<code>1</code> = 仅启用，<code>false</code>/<code>0</code> = 仅禁用，不传 = 全部。',
        'usage.s4.queryParams.page': '页码（可选，默认 1）。',
        'usage.s4.queryParams.pageSize': '每页条数（可选，默认 20，最大 100）。',
        'usage.s4.respFields.h': '响应字段说明（每个 Key）',
        'usage.s4.respFields.idKey': 'API Key 字符串及其数字记录 ID。',
        'usage.s4.respFields.active': '是否启用。被禁用（<code>false</code>）的 Key 在每次请求时都会被拒绝。',
        'usage.s4.respFields.allowedModels': '该 Key 允许调用的模型名 / glob 模式列表。',
        'usage.s4.respFields.binding': '绑定用户信息：业务用户名、自由文本备注、JSON 格式 metainfo（如 OAuth / 外部绑定）。',
        'usage.s4.respFields.token': 'Token 计费用量与 Token 限额（推荐使用的计费维度）。',
        'usage.s4.respFields.traffic': '字节流量用量与限额（旧版维度）。',
        'usage.s4.respFields.counts': '该 Key 的总请求数 / 成功数 / 失败数。',
        'usage.s4.respFields.io': '累计输入 / 输出字节数与联网搜索调用次数。',
        'usage.s4.respFields.meta': '最近使用时间（从未使用则不返回）、创建时间，以及创建该 Key 的运营账户名。',
        'usage.s5.h': '5. 更新 API Key 设置',
        'usage.s5.p': '更新已有的 API Key。只有你显式传入的字段会被修改，未传入的字段保持原值。可更新模型、限额、绑定用户信息与状态。',
        'usage.s5.code': 'cURL - 更新 Key',
        'usage.s6.h': '6. 启用 / 禁用 API Key',
        'usage.s6.p': '无需删除即可启用或禁用某个 Key，只需提交 <code>active</code> 字段。禁用立即生效：该 Key 会被移出生效集合，所有使用它的请求都会被拒绝。再次提交 <code>active: true</code> 即可恢复启用。',
        'usage.s6.code': 'cURL - 禁用 Key',
        'usage.s7.h': '7. 删除 API Key',
        'usage.s7.p': '删除你创建的某个 API Key：',
        'usage.s7.code': 'cURL - 删除 Key',
        'usage.s8.h': '8. 使用创建好的 API Key',
        'usage.s8.p': '在兼容 OpenAI 的接口中使用生成的 API Key：',
        'usage.s8.code': 'cURL - 对话补全',
        'usage.params.h': '请求参数说明',
        'usage.params.allowedModels': '模型名或 glob 模式数组（必填）。例如：<code>["gpt-4"]</code>、<code>["gpt-*", "claude-*"]</code>',
        'usage.params.trafficLimit': '流量额度，单位字节（可选）。例如：52428800（50MB）、104857600（100MB）、1073741824（1GB）',
        'usage.params.unlimited': '设为 <code>true</code> 表示不限流量（可选，默认 false）',
        'usage.params.tokenLimit': 'Token 限额（可选，推荐使用的计费维度）。<code>0</code> 或不传表示不限制 Token；设 <code>token_unlimited: true</code> 可显式禁用 Token 限额。',
        'usage.params.binding': '绑定用户信息（可选）。业务用户名、自由文本备注，以及用于外部 / OAuth 绑定的 JSON metainfo。',
        'usage.params.active': '启用 / 禁用状态，仅更新时有效（可选）。<code>true</code> = 启用，<code>false</code> = 禁用且立即不可用。不传则保持当前状态不变。',
        'usage.auth.h': '认证请求头',
        'usage.auth.opsKey': '用于管理类 API 调用（创建/更新/删除 Key）的运营 Key',
        'usage.auth.bearer': '用于 AI 服务调用（对话补全等）的 API Key',

        'info.loading': '正在加载用户信息',
        'info.userId': '用户 ID',
        'info.username': '用户名',
        'info.role': '角色',
        'info.status': '状态',
        'info.opsKey': '运营 Key',
        'info.defaultLimit': '默认额度',
        'info.apiKeysCount': '已创建 API Key',
        'info.createdAt': '创建时间',
        'info.resetOpsKey': '重置运营 Key',

        'settings.changePassword': '修改密码',
        'settings.currentPassword': '当前密码',
        'settings.newPassword': '新密码',
        'settings.minChars': '至少 8 位字符',
        'settings.confirmPassword': '确认新密码',
        'settings.submit': '修改密码',

        'edit.title': '编辑 API Key',
        'edit.apiKey': 'API Key',
        'edit.globHint': '逗号分隔的 glob 模式。使用 <code>*</code> 作为通配符。',
        'edit.currentUsagePrefix': '当前用量：',
        'edit.currentUsageSuffix': ' tokens',
        'edit.resetToken': '重置 Token',
        'edit.save': '保存修改',
        'edit.statusLabel': '状态',
        'edit.activeTitle': '启用',
        'edit.activeDescOn': 'API Key 已启用，可正常使用',
        'edit.activeDescOff': 'API Key 已禁用，将无法使用',
        'trafficDesc.unlimited': 'API Key 将不受流量限制',
        'trafficDesc.custom': '在下方设置自定义流量额度',

        // 模板：计费换算 / RMB / 创建结果 / 各类提示
        'calc.tokens': '换算：${n} tokens（${f}）',
        'calc.bytes': '换算：${n} bytes（${f}）',
        'calc.rmb': '约合 ${rmb} RMB（1 RMB = 10M 计费 Token）',
        'create.successAlert': 'API Key 已创建！流量：${traffic}，Token：${token}',
        'auth.sessionExpired': '会话已过期，请重新登录。',

        'toast.keyDeleted': 'API Key 删除成功',
        'toast.keyDeleteFailed': '删除 API Key 失败',
        'toast.keyEnabled': 'API Key 已启用',
        'toast.keyDisabled': 'API Key 已禁用',
        'toast.keyStatusFailed': '更新 API Key 状态失败',
        'toast.keyNotFound': '未找到该 API Key',
        'toast.selectModel': '请至少选择一个模型或输入一个 glob 模式',
        'toast.validTraffic': '请输入有效的流量额度，或启用不限流量',
        'toast.validToken': '请输入有效的 Token 额度，或启用不限 Token',
        'toast.keyUpdated': 'API Key 更新成功',
        'toast.keyUpdateFailed': '更新 API Key 失败',
        'toast.trafficReset': '流量已重置',
        'toast.trafficResetFailed': '重置流量失败',
        'toast.tokenReset': 'Token 用量已重置',
        'toast.tokenResetFailed': '重置 Token 失败',
        'toast.curlCopied': 'cURL 命令已复制到剪贴板！',
        'toast.keyCopied': 'API Key 已复制到剪贴板！',
        'toast.copyFailed': '复制到剪贴板失败',
        'confirm.deleteKey': '确定要删除此 API Key 吗？此操作不可撤销。',
        'confirm.disableKey': '确定要禁用此 API Key 吗？禁用后该 Key 将立即无法使用。',
        'confirm.enableKey': '确定要启用此 API Key 吗？',
        'confirm.resetTraffic': '确定要重置此 API Key 的流量计数吗？',
        'confirm.resetToken': '确定要重置此 API Key 的 Token 用量吗？',
        'confirm.resetOpsKey': '确定要重置你的运营 Key 吗？使用旧 Key 的应用都需要更新。',
        'alert.createKeyFailed': '创建 API Key 失败',
        'alert.networkRetry': '网络错误，请重试。',
        'alert.passwordMismatch': '两次输入的新密码不一致',
        'alert.passwordTooShort': '新密码至少需要 8 位字符',
        'alert.passwordChanged': '密码修改成功！',
        'alert.passwordChangeFailed': '修改密码失败',
        'opsKey.resetSuccess': '运营 Key 重置成功！\n\n新 Key：',
        'opsKey.resetFailed': '重置运营 Key 失败：',
        'opsKey.unknownError': '未知错误',
    },
    en: {
        'header.badge': 'OPS Portal',
        'header.welcome': 'Welcome, ',
        'header.homepage': 'Homepage',
        'header.logout': 'Logout',
        'lang.toggleToEn': 'English',
        'lang.toggleToZh': '中文',

        'stats.apiKeys': 'API Keys Created',
        'stats.defaultLimit': 'Default Traffic Limit',
        'stats.status': 'Account Status',

        'tab.createKey': 'Create API Key',
        'tab.myKeys': 'My API Keys',
        'tab.apiUsage': 'API Usage',
        'tab.myInfo': 'My Info',
        'tab.settings': 'Settings',

        'common.selectModels': 'Select Models',
        'common.selectAll': 'Select All',
        'common.clear': 'Clear',
        'common.loadingModels': 'Loading models...',
        'common.noModels': 'No models available',
        'common.loadModelsFailed': 'Failed to load models',
        'common.globAdvanced': 'Glob Patterns (Advanced)',
        'common.selectedLabel': 'Selected: ',
        'common.modelsUnit': ' models',
        'common.noneSelected': 'None selected',
        'preview.empty': '<strong>Selected: </strong> <span style="color: #888;">None selected</span>',
        'common.moreSuffix': ' more',
        'common.refresh': 'Refresh',
        'common.cancel': 'Cancel',
        'common.active': 'Active',
        'common.inactive': 'Inactive',
        'common.unlimited': 'Unlimited',
        'common.networkError': 'Network error',
        'common.copy': 'Copy',

        'create.allowedModels': 'Allowed Models',
        'create.globHint': 'Comma-separated glob patterns. Use <code>*</code> as wildcard. Example: <code>memfit-*</code> matches all memfit models.',
        'create.globPh': 'e.g., memfit-*,qwen*,gpt-4*',
        'create.userBinding': 'User Binding (optional)',
        'create.usernamePh': 'Username (repeatable)',
        'create.remarkPh': 'Remark (free text)',
        'create.metainfoPh': 'metainfo (JSON text, for OAuth/external binding, optional)',
        'create.tokenSettings': 'Token Settings',
        'create.recommended': '(Recommended)',
        'create.unlimitedToken': 'Unlimited Token',
        'tokenDesc.unlimited': 'API Key will have no token restrictions',
        'tokenDesc.custom': 'Set a custom token limit below',
        'create.tokenBillingHint': 'Recommended billing dimension. Aggregated by weighted multipliers (input/output/cache).',
        'unit.mTokens': 'M tokens',
        'unit.kTokens': 'K tokens',
        'unit.tokens': 'tokens',
        'create.generate': 'Generate API Key',
        'create.newKeyLabel': 'Your new API Key:',
        'create.saveNotice': 'Please save this key securely. It will only be shown once!',
        'create.copyClipboard': 'Copy to Clipboard',

        'myKeys.title': 'My API Keys',
        'myKeys.loading': 'Loading API Keys',
        'myKeys.thApiKey': 'API Key',
        'myKeys.thUserRemark': 'User / Remark',
        'myKeys.thModels': 'Models',
        'myKeys.thTokenUsed': 'Token Used (Billing)',
        'myKeys.thTokenLimit': 'Token Limit *',
        'myKeys.thCreatedAt': 'Created At',
        'myKeys.thActions': 'Actions',
        'myKeys.titleUserRemark': 'Bound username (repeatable) / remark',
        'myKeys.titleTokenUsed': 'Token billing usage (weighted by multipliers)',
        'myKeys.titleTokenLimit': 'Token limit (the only billing dimension)',
        'myKeys.empty': "You haven't created any API keys yet.",
        'myKeys.createFirst': 'Create Your First API Key',
        'myKeys.edit': 'Edit',
        'myKeys.delete': 'Delete',
        'myKeys.loadFailed': 'Failed to load keys',
        'myKeys.thStatus': 'Status',
        'myKeys.searchPh': 'Search username / remark / key',
        'myKeys.statusAll': 'All status',
        'myKeys.statusActive': 'Active only',
        'myKeys.statusInactive': 'Inactive only',
        'myKeys.search': 'Search',
        'myKeys.clearFilter': 'Clear',
        'myKeys.enable': 'Enable',
        'myKeys.disable': 'Disable',

        'pag.total': 'Total ${total} keys, Page ${page}/${pages}',
        'pag.first': 'First',
        'pag.prev': 'Prev',
        'pag.next': 'Next',
        'pag.last': 'Last',
        'pag.gotoPrefix': 'Go to',
        'pag.go': 'Go',
        'pag.perPage': '${n}/page',

        'usage.title': 'API Usage Guide',
        'usage.s1.h': '1. Create API Key with Traffic Limit',
        'usage.s1.p': 'Create an API key with a specific traffic limit (e.g., 50 MB = 52428800 bytes):',
        'usage.s1.code': 'cURL - Create Key with 50MB Limit',
        'usage.s2.h': '2. Create API Key with Unlimited Traffic',
        'usage.s2.p': 'Create an API key without traffic restrictions:',
        'usage.s2.code': 'cURL - Create Unlimited Key',
        'usage.s3.h': '3. Create API Key with Glob Patterns',
        'usage.s3.p': 'Use glob patterns to match multiple models (e.g., <code>gpt-*</code> matches all GPT models):',
        'usage.s3.code': 'cURL - Create Key with Glob Patterns',
        'usage.s4.h': '4. List & Search My API Keys',
        'usage.s4.p': 'Get all API keys created by your OPS account. Supports pagination, keyword search and status filtering. You can only ever see keys created by your own OPS account.',
        'usage.s4.code': 'cURL - List Keys',
        'usage.s4.filter.p': 'Search by keyword (matches bound username / remark / api_key) and filter by status. The example below finds active keys whose username, remark or key contains "alice":',
        'usage.s4.filter.code': 'cURL - Search & Filter Keys',
        'usage.s4.queryParams.h': 'Query Parameters (List & Search)',
        'usage.s4.queryParams.q': 'Broad keyword search (optional). Partial match over <code>username</code>, <code>remark</code> and <code>api_key</code>. Special characters are escaped (SQL-injection safe).',
        'usage.s4.queryParams.username': 'Filter by bound username (optional, partial match).',
        'usage.s4.queryParams.active': 'Filter by status (optional). <code>true</code>/<code>1</code> = enabled only, <code>false</code>/<code>0</code> = disabled only, omit = all.',
        'usage.s4.queryParams.page': 'Page number (optional, default 1).',
        'usage.s4.queryParams.pageSize': 'Items per page (optional, default 20, max 100).',
        'usage.s4.respFields.h': 'Response Fields (per key)',
        'usage.s4.respFields.idKey': 'The API key string and its numeric record ID.',
        'usage.s4.respFields.active': 'Whether the key is enabled. A disabled key (<code>false</code>) is rejected on every request.',
        'usage.s4.respFields.allowedModels': 'List of model names / glob patterns this key may call.',
        'usage.s4.respFields.binding': 'Bound user info: business username, free-text remark, and JSON metainfo (e.g. OAuth / external binding).',
        'usage.s4.respFields.token': 'Token billing usage and the token limit (the recommended billing dimension).',
        'usage.s4.respFields.traffic': 'Byte-traffic usage and limit (legacy dimension).',
        'usage.s4.respFields.counts': 'Total / successful / failed request counts for this key.',
        'usage.s4.respFields.io': 'Aggregated input / output bytes and web-search invocation count.',
        'usage.s4.respFields.meta': 'Last-used time (omitted if never used), creation time, and the OPS account name that created the key.',
        'usage.s5.h': '5. Update API Key Settings',
        'usage.s5.p': 'Update an existing API key. Only the fields you include are changed; omitted fields keep their current values. You can update models, limits, bound user info and status.',
        'usage.s5.code': 'cURL - Update Key',
        'usage.s6.h': '6. Enable / Disable an API Key',
        'usage.s6.p': 'Enable or disable a key without deleting it by sending only the <code>active</code> field. Disabling takes effect immediately: a disabled key is removed from the active set and every request using it is rejected. Re-enable it the same way with <code>active: true</code>.',
        'usage.s6.code': 'cURL - Disable Key',
        'usage.s7.h': '7. Delete API Key',
        'usage.s7.p': 'Delete an API key you created:',
        'usage.s7.code': 'cURL - Delete Key',
        'usage.s8.h': '8. Use the Created API Key',
        'usage.s8.p': 'Use the generated API key with OpenAI-compatible endpoints:',
        'usage.s8.code': 'cURL - Chat Completion',
        'usage.params.h': 'Request Parameters Reference',
        'usage.params.allowedModels': 'Array of model names or glob patterns (required). Examples: <code>["gpt-4"]</code>, <code>["gpt-*", "claude-*"]</code>',
        'usage.params.trafficLimit': 'Traffic limit in bytes (optional). Examples: 52428800 (50MB), 104857600 (100MB), 1073741824 (1GB)',
        'usage.params.unlimited': 'Set to <code>true</code> for unlimited traffic (optional, default: false)',
        'usage.params.tokenLimit': 'Token limit (optional, recommended billing dimension). <code>0</code> or omitted means no token limit; set <code>token_unlimited: true</code> to explicitly disable it.',
        'usage.params.binding': 'Bound user info (optional). Business username, free-text remark, and JSON metainfo for external/OAuth binding.',
        'usage.params.active': 'Enable/disable status, update only (optional). <code>true</code> = enabled, <code>false</code> = disabled and immediately unusable. Omit to keep the current status unchanged.',
        'usage.auth.h': 'Authentication Headers',
        'usage.auth.opsKey': 'Your OPS Key for management API calls (create/update/delete keys)',
        'usage.auth.bearer': 'API Key for AI service calls (chat completions, etc.)',

        'info.loading': 'Loading user info',
        'info.userId': 'User ID',
        'info.username': 'Username',
        'info.role': 'Role',
        'info.status': 'Status',
        'info.opsKey': 'OPS Key',
        'info.defaultLimit': 'Default Limit',
        'info.apiKeysCount': 'API Keys Created',
        'info.createdAt': 'Created At',
        'info.resetOpsKey': 'Reset OPS Key',

        'settings.changePassword': 'Change Password',
        'settings.currentPassword': 'Current Password',
        'settings.newPassword': 'New Password',
        'settings.minChars': 'Minimum 8 characters',
        'settings.confirmPassword': 'Confirm New Password',
        'settings.submit': 'Change Password',

        'edit.title': 'Edit API Key',
        'edit.apiKey': 'API Key',
        'edit.globHint': 'Comma-separated glob patterns. Use <code>*</code> as wildcard.',
        'edit.currentUsagePrefix': 'Current usage: ',
        'edit.currentUsageSuffix': ' tokens',
        'edit.resetToken': 'Reset Token',
        'edit.save': 'Save Changes',
        'edit.statusLabel': 'Status',
        'edit.activeTitle': 'Enabled',
        'edit.activeDescOn': 'API Key is active and usable',
        'edit.activeDescOff': 'API Key is disabled and cannot be used',
        'trafficDesc.unlimited': 'API Key will have no traffic restrictions',
        'trafficDesc.custom': 'Set a custom traffic limit below',

        'calc.tokens': 'Calculated: ${n} tokens (${f})',
        'calc.bytes': 'Calculated: ${n} bytes (${f})',
        'calc.rmb': '\u2248 ${rmb} RMB (1 RMB = 10M billing tokens)',
        'create.successAlert': 'API Key created! Traffic: ${traffic}, Token: ${token}',
        'auth.sessionExpired': 'Session expired, please login again.',

        'toast.keyDeleted': 'API Key deleted successfully',
        'toast.keyDeleteFailed': 'Failed to delete API key',
        'toast.keyEnabled': 'API Key enabled',
        'toast.keyDisabled': 'API Key disabled',
        'toast.keyStatusFailed': 'Failed to update API key status',
        'toast.keyNotFound': 'API Key not found',
        'toast.selectModel': 'Please select at least one model or enter a glob pattern',
        'toast.validTraffic': 'Please enter a valid traffic limit or enable unlimited traffic',
        'toast.validToken': 'Please enter a valid token limit or enable unlimited token',
        'toast.keyUpdated': 'API Key updated successfully',
        'toast.keyUpdateFailed': 'Failed to update API key',
        'toast.trafficReset': 'Traffic reset successfully',
        'toast.trafficResetFailed': 'Failed to reset traffic',
        'toast.tokenReset': 'Token usage reset successfully',
        'toast.tokenResetFailed': 'Failed to reset token',
        'toast.curlCopied': 'cURL command copied to clipboard!',
        'toast.keyCopied': 'API Key copied to clipboard!',
        'toast.copyFailed': 'Failed to copy to clipboard',
        'confirm.deleteKey': 'Are you sure you want to delete this API key? This action cannot be undone.',
        'confirm.disableKey': 'Are you sure you want to disable this API key? It will become unusable immediately.',
        'confirm.enableKey': 'Are you sure you want to enable this API key?',
        'confirm.resetTraffic': 'Are you sure you want to reset the traffic counter for this API key?',
        'confirm.resetToken': 'Are you sure you want to reset the token usage for this API key?',
        'confirm.resetOpsKey': 'Are you sure you want to reset your OPS Key? You will need to update any applications using the current key.',
        'alert.createKeyFailed': 'Failed to create API key',
        'alert.networkRetry': 'Network error. Please try again.',
        'alert.passwordMismatch': 'New passwords do not match',
        'alert.passwordTooShort': 'New password must be at least 8 characters',
        'alert.passwordChanged': 'Password changed successfully!',
        'alert.passwordChangeFailed': 'Failed to change password',
        'opsKey.resetSuccess': 'OPS Key reset successfully!\n\nNew Key: ',
        'opsKey.resetFailed': 'Failed to reset OPS key: ',
        'opsKey.unknownError': 'Unknown error',
    },
};

// t 取当前语言文案；支持 ${name} 占位替换；缺失时回退到 zh，再回退到 key 本身。
// 关键词: ops i18n t(), 占位替换, 回退
function t(key, params) {
    const table = I18N[currentLang] || I18N.zh;
    let s = table[key];
    if (s === undefined) s = (I18N.zh[key] !== undefined ? I18N.zh[key] : key);
    if (params) {
        s = s.replace(/\$\{(\w+)\}/g, function (_, name) {
            return (params[name] !== undefined && params[name] !== null) ? String(params[name]) : '';
        });
    }
    return s;
}

// applyI18n 回填所有带 data-i18n* 注解的静态元素，并刷新页头切换按钮与 <html lang>。
// 关键词: ops applyI18n, data-i18n 回填
function applyI18n() {
    document.querySelectorAll('[data-i18n]').forEach(function (el) {
        el.textContent = t(el.getAttribute('data-i18n'));
    });
    document.querySelectorAll('[data-i18n-html]').forEach(function (el) {
        el.innerHTML = t(el.getAttribute('data-i18n-html'));
    });
    document.querySelectorAll('[data-i18n-placeholder]').forEach(function (el) {
        el.setAttribute('placeholder', t(el.getAttribute('data-i18n-placeholder')));
    });
    document.querySelectorAll('[data-i18n-title]').forEach(function (el) {
        el.setAttribute('title', t(el.getAttribute('data-i18n-title')));
    });
    document.documentElement.lang = (currentLang === 'zh') ? 'zh-CN' : 'en';
    const lt = document.getElementById('lang-toggle');
    if (lt) lt.textContent = (currentLang === 'zh') ? t('lang.toggleToEn') : t('lang.toggleToZh');
}

// initI18n 启动时读取 localStorage 语言（默认 zh）并应用。
// 关键词: ops initI18n, 默认中文, localStorage 记忆
function initI18n() {
    let saved = 'zh';
    try { saved = localStorage.getItem(OPS_LANG_KEY) || 'zh'; } catch (e) { saved = 'zh'; }
    currentLang = (saved === 'en') ? 'en' : 'zh';
    applyI18n();
}

// setOpsLang 切换语言：记忆选择、回填静态文案、并刷新动态视图（统计/信息/表格/分页/预览/换算）。
// 关键词: ops setOpsLang, toggleOpsLang, 切换后刷新动态视图
function setOpsLang(lang) {
    currentLang = (lang === 'en') ? 'en' : 'zh';
    try { localStorage.setItem(OPS_LANG_KEY, currentLang); } catch (e) { /* ignore */ }
    applyI18n();
    refreshDynamicI18n();
}

function toggleOpsLang() {
    setOpsLang(currentLang === 'zh' ? 'en' : 'zh');
}

// syncToggleDescs 依据各 toggle 当前勾选状态回填「不限制/自定义」描述文案。
// applyI18n 会把 data-i18n 描述统一回填为「不限制」版本，这里再按实际状态纠正，
// 避免用户在「自定义额度」状态下切换语言时描述被错误重置。
// 关键词: ops syncToggleDescs, toggle 描述按状态纠正
function syncToggleDescs() {
    const pairs = [
        ['unlimited-token', 'token-desc', 'tokenDesc'],
        ['edit-token-unlimited', 'edit-token-desc', 'tokenDesc'],
        ['unlimited-traffic', 'traffic-desc', 'trafficDesc'],
        ['edit-unlimited-traffic', 'edit-traffic-desc', 'trafficDesc'],
    ];
    pairs.forEach(function (p) {
        const cb = document.getElementById(p[0]);
        const desc = document.getElementById(p[1]);
        if (cb && desc) {
            desc.textContent = cb.checked ? t(p[2] + '.unlimited') : t(p[2] + '.custom');
        }
    });
}

// refreshDynamicI18n 重渲染由 JS 生成、含文案的动态片段，使语言切换即时生效。
function refreshDynamicI18n() {
    try { if (userInfo) updateUI(); } catch (e) { /* ignore */ }
    try { renderModelList(); } catch (e) { /* ignore */ }
    try { updateSelectedPreview(); } catch (e) { /* ignore */ }
    try { syncToggleDescs(); } catch (e) { /* ignore */ }
    try { if (typeof updateEditSelectedPreview === 'function') updateEditSelectedPreview(); } catch (e) { /* ignore */ }
    try {
        const content = document.getElementById('my-keys-content');
        if (content && !content.classList.contains('hidden') && Array.isArray(myApiKeys) && myApiKeys.length > 0) {
            renderApiKeysTable();
        }
    } catch (e) { /* ignore */ }
    try { calculateTokenBytes(); } catch (e) { /* ignore */ }
    try { if (document.getElementById('edit-token-limit-group')) calculateEditTokenBytes(); } catch (e) { /* ignore */ }
}

// HTML 转义助手：渲染用户名/备注/metainfo 等用户可控文本时防止 XSS。
// 关键词: ops_portal escapeHtml, Username Remark MetaInfo 安全渲染
function escapeHtml(str) {
    if (str === null || str === undefined) return '';
    return String(str)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;');
}

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
    alert(t('auth.sessionExpired'));
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

// ==================== Session Auto-Refresh ====================
// 后端 OPS session 有效期为 30 分钟。只要 OPS portal 页面开着，
// 这里就每 10 分钟自动调一次 /ops/api/session/refresh，把
// ExpiresAt 顺延 30 分钟，避免因长时间挂在页面上而被强制登出。
// 关键词: ops session auto refresh keep alive 自动续期 token
const SESSION_REFRESH_INTERVAL_MS = 10 * 60 * 1000;
let __sessionRefreshTimer = null;

async function refreshOpsSessionOnce() {
    try {
        const resp = await fetch('/ops/api/session/refresh', {
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
            console.debug('ops session refreshed, new expires_at:', data.expires_at);
        }
        return true;
    } catch (e) {
        console.warn('ops session refresh error:', e);
        return false;
    }
}

function startSessionAutoRefresh() {
    if (__sessionRefreshTimer) return;
    refreshOpsSessionOnce();
    __sessionRefreshTimer = setInterval(refreshOpsSessionOnce, SESSION_REFRESH_INTERVAL_MS);
}

// ==================== Initialize ====================

document.addEventListener('DOMContentLoaded', function() {
    // 先应用语言（默认中文，记忆到 localStorage），再加载数据，保证首屏即为目标语言。
    initI18n();
    initTabs();
    loadUserInfo();
    loadModels();
    initForms();
    updateApiEndpoint();
    // 启动 OPS session 自动续期：只要页面开着就每 10 分钟续一次。
    startSessionAutoRefresh();
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
    statusEl.textContent = userInfo.active ? t('common.active') : t('common.inactive');
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
    infoStatus.textContent = userInfo.active ? t('common.active') : t('common.inactive');
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
            modelList.innerHTML = `<div style="padding: 20px; text-align: center; color: #888;">${t('common.noModels')}</div>`;
        }
    } catch (error) {
        console.error('Error loading models:', error);
        modelList.innerHTML = `<div style="padding: 20px; text-align: center; color: #dc3545;">${t('common.loadModelsFailed')}</div>`;
    }
}

function renderModelList() {
    const modelList = document.getElementById('model-list');
    
    // 模型名按索引回查（toggleModelByIndex），不内联进 onclick；展示文本与 id/for 走 escapeHtml。
    // 关键词: ops renderModelList XSS 防护, 索引法 onclick
    modelList.innerHTML = availableModels.map((model, idx) => `
        <div class="model-item ${selectedModels.has(model) ? 'selected' : ''}" onclick="toggleModelByIndex(${idx})">
            <input type="checkbox" id="model-${escapeHtml(model)}" ${selectedModels.has(model) ? 'checked' : ''} onclick="event.stopPropagation(); toggleModelByIndex(${idx})">
            <label for="model-${escapeHtml(model)}">${escapeHtml(model)}</label>
        </div>
    `).join('');
    
    updateSelectedPreview();
}

// toggleModelByIndex 用索引回查模型名后切换选中，避免内联模型名进 onclick。
function toggleModelByIndex(idx) {
    const model = availableModels[idx];
    if (model != null) toggleModel(model);
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
    
    let html = `<strong>${t('common.selectedLabel')}</strong> `;
    
    // Show selected models (sorted)
    const modelArray = Array.from(selectedModels).sort();
    if (modelArray.length > 0) {
        if (modelArray.length <= 5) {
            html += modelArray.map(m => `<span class="tag">${escapeHtml(m)}</span>`).join('');
        } else {
            html += modelArray.slice(0, 3).map(m => `<span class="tag">${escapeHtml(m)}</span>`).join('');
            html += `<span class="tag">+${modelArray.length - 3}${t('common.moreSuffix')}</span>`;
        }
    }
    
    // Show glob patterns
    if (globPatterns) {
        const patterns = globPatterns.split(',').map(p => p.trim()).filter(p => p);
        patterns.forEach(p => {
            html += `<span class="tag glob">${escapeHtml(p)}</span>`;
        });
    }
    
    if (modelArray.length === 0 && !globPatterns) {
        html += `<span style="color: #888;">${t('common.noneSelected')}</span>`;
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
// 当前过滤条件：关键字（用户名/备注/Key）与状态（''=全部, 'true'/'false'）。
// 关键词: OPS my-keys 过滤状态保持, q/active 查询参数
let myKeysSearch = '';
let myKeysStatus = '';

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
        // 拼接分页 + 过滤查询参数；过滤条件为空则不附加。
        let url = `/ops/api/my-keys?page=${myKeysPage}&page_size=${myKeysPageSize}`;
        if (myKeysSearch) url += `&q=${encodeURIComponent(myKeysSearch)}`;
        if (myKeysStatus === 'true' || myKeysStatus === 'false') url += `&active=${myKeysStatus}`;
        const response = await fetch(url);
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
            tbody.innerHTML = `<tr><td colspan="8" style="text-align: center; color: #dc3545;">${escapeHtml(data.error || t('myKeys.loadFailed'))}</td></tr>`;
        }
    } catch (error) {
        console.error('Error loading API keys:', error);
        loading.classList.add('hidden');
        content.classList.remove('hidden');
        tbody.innerHTML = `<tr><td colspan="8" style="text-align: center; color: #dc3545;">${t('common.networkError')}</td></tr>`;
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
        // 关键词: OPS my-keys 渲染 Token 计费用量/限额列, 字节流量停用, 计费视图
        const tokenUsedRaw = Number(key.token_used) || 0;
        const tokenLimitRaw = Number(key.token_limit) || 0;
        const tokenIsUnlimited = !key.token_limit_enable || tokenLimitRaw <= 0;
        let tokenColor = '#28a745';
        if (!tokenIsUnlimited && tokenLimitRaw > 0) {
            const tPercent = Math.min(100, (tokenUsedRaw / tokenLimitRaw) * 100);
            tokenColor = tPercent > 80 ? '#dc3545' : tPercent > 50 ? '#ffc107' : '#28a745';
        }
        const tokenUsedDisplay = formatTokenCount(tokenUsedRaw);
        const tokenLimitDisplay = tokenIsUnlimited
            ? `<span style="color: #28a745; font-weight: 500;">${t('common.unlimited')}</span>`
            : formatTokenCount(tokenLimitRaw);
        
        // Display models (sorted)
        const models = key.allowed_models || [];
        const modelsDisplay = models.length > 3 
            ? models.slice(0, 3).join(', ') + ` (+${models.length - 3})`
            : models.join(', ');

        // 绑定用户信息：用户名(可重复) + 备注
        const uname = key.username ? escapeHtml(key.username) : '<span style="color:#bbb;">-</span>';
        const remarkHtml = key.remark
            ? `<small style="display:block;color:#888;max-width:160px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">${escapeHtml(key.remark)}</small>`
            : '';

        // 状态徽标与启用/禁用切换：active=true 显示绿色徽标并提供"禁用"按钮；反之红色徽标 + "启用"按钮。
        // 关键词: OPS my-keys 状态徽标 enable/disable 按钮
        const isActive = key.active !== false;
        const statusBadge = isActive
            ? `<span style="display:inline-block;padding:2px 8px;border-radius:10px;background:#e6f4ea;color:#137333;font-size:12px;font-weight:500;">${t('common.active')}</span>`
            : `<span style="display:inline-block;padding:2px 8px;border-radius:10px;background:#fce8e6;color:#c5221f;font-size:12px;font-weight:500;">${t('common.inactive')}</span>`;
        const toggleBtn = isActive
            ? `<button class="btn btn-sm" onclick="toggleApiKeyActive('${key.api_key}', false)" style="background:#fbbc04;color:#3c4043;">${t('myKeys.disable')}</button>`
            : `<button class="btn btn-sm" onclick="toggleApiKeyActive('${key.api_key}', true)" style="background:#34a853;color:#fff;">${t('myKeys.enable')}</button>`;

        return `
            <tr>
                <td><code>${key.api_key.substring(0, 20)}...</code></td>
                <td title="${escapeHtml(key.remark || '')}">${uname}${remarkHtml}</td>
                <td title="${escapeHtml(models.join(', '))}">${escapeHtml(modelsDisplay) || '-'}</td>
                <td style="color: ${tokenColor}" title="${t('myKeys.titleTokenUsed')}">${tokenUsedDisplay}</td>
                <td title="${t('myKeys.titleTokenLimit')}">${tokenLimitDisplay}</td>
                <td>${statusBadge}</td>
                <td>${key.created_at || '--'}</td>
                <td>
                    <div style="display: flex; gap: 5px; flex-wrap: wrap;">
                        <button class="btn btn-sm" onclick="openEditKeyModal('${key.api_key}')" style="background: #4285f4;">${t('myKeys.edit')}</button>
                        ${toggleBtn}
                        <button class="btn btn-sm btn-danger" onclick="deleteApiKey('${key.api_key}')">${t('myKeys.delete')}</button>
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
            ${t('pag.total', { total: total, page: page, pages: total_pages || 1 })}
        </div>
        <div class="pagination-buttons" style="display: flex; gap: 5px; align-items: center;">
            <button class="btn btn-sm" ${page <= 1 ? 'disabled' : ''} onclick="changeMyKeysPage(1)">${t('pag.first')}</button>
            <button class="btn btn-sm" ${page <= 1 ? 'disabled' : ''} onclick="changeMyKeysPage(${page - 1})">${t('pag.prev')}</button>
            <span style="margin: 0 10px;">
                ${t('pag.gotoPrefix')} <input type="number" id="myKeysPageInput" min="1" max="${total_pages}" value="${page}" style="width: 50px; text-align: center;"> 
                <button class="btn btn-sm" onclick="changeMyKeysPage(parseInt(document.getElementById('myKeysPageInput').value))">${t('pag.go')}</button>
            </span>
            <button class="btn btn-sm" ${page >= total_pages ? 'disabled' : ''} onclick="changeMyKeysPage(${page + 1})">${t('pag.next')}</button>
            <button class="btn btn-sm" ${page >= total_pages ? 'disabled' : ''} onclick="changeMyKeysPage(${total_pages})">${t('pag.last')}</button>
            <select onchange="changeMyKeysPageSize(this.value)" style="margin-left: 10px;">
                <option value="10" ${page_size === 10 ? 'selected' : ''}>${t('pag.perPage', { n: 10 })}</option>
                <option value="20" ${page_size === 20 ? 'selected' : ''}>${t('pag.perPage', { n: 20 })}</option>
                <option value="50" ${page_size === 50 ? 'selected' : ''}>${t('pag.perPage', { n: 50 })}</option>
                <option value="100" ${page_size === 100 ? 'selected' : ''}>${t('pag.perPage', { n: 100 })}</option>
            </select>
        </div>
    `;
}

async function deleteApiKey(apiKey) {
    if (!confirm(t('confirm.deleteKey'))) {
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
            showToast(t('toast.keyDeleted'), 'success');
            loadMyApiKeys();
            loadUserInfo(); // Refresh stats
        } else {
            showToast(data.error || t('toast.keyDeleteFailed'), 'error');
        }
    } catch (error) {
        console.error('Error deleting API key:', error);
        showToast(t('common.networkError'), 'error');
    }
}

// 启用/禁用 API Key：调用 update-api-key 仅提交 active 字段（后端用 *bool 区分未提供）。
// 禁用后该 Key 立即从内存可用集合移除，请求会被拒绝。
// 关键词: OPS toggleApiKeyActive 启用禁用, update-api-key active 字段
async function toggleApiKeyActive(apiKey, nextActive) {
    const confirmMsg = nextActive ? t('confirm.enableKey') : t('confirm.disableKey');
    if (!confirm(confirmMsg)) {
        return;
    }

    try {
        const response = await fetch('/ops/api/update-api-key', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ api_key: apiKey, active: nextActive })
        });

        const data = await response.json();

        if (data.success) {
            showToast(nextActive ? t('toast.keyEnabled') : t('toast.keyDisabled'), 'success');
            loadMyApiKeys();
        } else {
            showToast(data.error || t('toast.keyStatusFailed'), 'error');
        }
    } catch (error) {
        console.error('Error toggling API key status:', error);
        showToast(t('common.networkError'), 'error');
    }
}

// 应用 my-keys 过滤：读取搜索框与状态下拉，重置到第一页后重新加载。
// 关键词: OPS applyMyKeysFilter 应用过滤
function applyMyKeysFilter() {
    const searchEl = document.getElementById('my-keys-search');
    const statusEl = document.getElementById('my-keys-status-filter');
    myKeysSearch = searchEl ? searchEl.value.trim() : '';
    myKeysStatus = statusEl ? statusEl.value : '';
    loadMyApiKeys(1);
}

// 清除 my-keys 过滤条件并重新加载。
// 关键词: OPS clearMyKeysFilter 清除过滤
function clearMyKeysFilter() {
    const searchEl = document.getElementById('my-keys-search');
    const statusEl = document.getElementById('my-keys-status-filter');
    if (searchEl) searchEl.value = '';
    if (statusEl) statusEl.value = '';
    myKeysSearch = '';
    myKeysStatus = '';
    loadMyApiKeys(1);
}

// ==================== Edit API Key ====================

let editSelectedModels = new Set();
let currentEditKey = null;

function openEditKeyModal(apiKey) {
    // Find the key data
    currentEditKey = myApiKeys.find(k => k.api_key === apiKey);
    if (!currentEditKey) {
        showToast(t('toast.keyNotFound'), 'error');
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

    // 填充绑定用户信息（用户名/备注/metainfo）
    // 关键词: openEditKeyModal username remark metainfo 回填
    const editUsernameEl = document.getElementById('edit-username');
    const editRemarkEl = document.getElementById('edit-remark');
    const editMetaEl = document.getElementById('edit-metainfo');
    if (editUsernameEl) editUsernameEl.value = currentEditKey.username || '';
    if (editRemarkEl) editRemarkEl.value = currentEditKey.remark || '';
    if (editMetaEl) editMetaEl.value = currentEditKey.metainfo || '';
    
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
        trafficDesc.textContent = t('trafficDesc.unlimited');
    } else {
        trafficLimitGroup.style.display = 'block';
        unlimitedToggle.classList.remove('active');
        trafficDesc.textContent = t('trafficDesc.custom');
        
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
            trafficDesc.textContent = t('trafficDesc.unlimited');
        } else {
            trafficLimitGroup.style.display = 'block';
            unlimitedToggle.classList.remove('active');
            trafficDesc.textContent = t('trafficDesc.custom');
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

    // ==================== Token Settings Initialization ====================
    // 关键词: openEditKeyModal Token 维度初始化, 推荐 token over traffic
    const tokenUnlimitedCheckbox = document.getElementById('edit-token-unlimited');
    const tokenLimitGroup = document.getElementById('edit-token-limit-group');
    const tokenUnlimitedToggle = document.getElementById('edit-token-unlimited-toggle');
    const tokenDesc = document.getElementById('edit-token-desc');
    const tokenIsUnlimited = !currentEditKey.token_limit_enable || !currentEditKey.token_limit || currentEditKey.token_limit <= 0;

    if (tokenUnlimitedCheckbox) {
        tokenUnlimitedCheckbox.checked = tokenIsUnlimited;
        if (tokenIsUnlimited) {
            if (tokenLimitGroup) tokenLimitGroup.style.display = 'none';
            if (tokenUnlimitedToggle) tokenUnlimitedToggle.classList.add('active');
            if (tokenDesc) tokenDesc.textContent = t('tokenDesc.unlimited');
        } else {
            if (tokenLimitGroup) tokenLimitGroup.style.display = 'block';
            if (tokenUnlimitedToggle) tokenUnlimitedToggle.classList.remove('active');
            if (tokenDesc) tokenDesc.textContent = t('tokenDesc.custom');

            const tokenRaw = Number(currentEditKey.token_limit) || 0;
            const tokenValueInput = document.getElementById('edit-token-limit-value');
            const tokenUnitSelect = document.getElementById('edit-token-limit-unit');
            if (tokenValueInput && tokenUnitSelect) {
                if (tokenRaw >= 1_000_000 && tokenRaw % 1_000_000 === 0) {
                    tokenValueInput.value = tokenRaw / 1_000_000;
                    tokenUnitSelect.value = '1000000';
                } else if (tokenRaw >= 1000 && tokenRaw % 1000 === 0) {
                    tokenValueInput.value = tokenRaw / 1000;
                    tokenUnitSelect.value = '1000';
                } else {
                    tokenValueInput.value = tokenRaw;
                    tokenUnitSelect.value = '1';
                }
            }
            calculateEditTokenBytes();
        }

        tokenUnlimitedCheckbox.onchange = function() {
            if (this.checked) {
                if (tokenLimitGroup) tokenLimitGroup.style.display = 'none';
                if (tokenUnlimitedToggle) tokenUnlimitedToggle.classList.add('active');
                if (tokenDesc) tokenDesc.textContent = t('tokenDesc.unlimited');
            } else {
                if (tokenLimitGroup) tokenLimitGroup.style.display = 'block';
                if (tokenUnlimitedToggle) tokenUnlimitedToggle.classList.remove('active');
                if (tokenDesc) tokenDesc.textContent = t('tokenDesc.custom');
                calculateEditTokenBytes();
            }
        };

        const tokenValueInput2 = document.getElementById('edit-token-limit-value');
        const tokenUnitSelect2 = document.getElementById('edit-token-limit-unit');
        if (tokenValueInput2) tokenValueInput2.addEventListener('input', calculateEditTokenBytes);
        if (tokenUnitSelect2) tokenUnitSelect2.addEventListener('change', calculateEditTokenBytes);
    }
    const tokenUsedEl = document.getElementById('edit-token-used');
    if (tokenUsedEl) {
        tokenUsedEl.textContent = (Number(currentEditKey.token_used) || 0).toString();
    }

    // ==================== Active(启用/禁用) 初始化 ====================
    // 关键词: openEditKeyModal active 启用禁用回填, 描述随勾选切换
    const editActiveCheckbox = document.getElementById('edit-active');
    const editActiveDesc = document.getElementById('edit-active-desc');
    if (editActiveCheckbox) {
        const isActive = currentEditKey.active !== false;
        editActiveCheckbox.checked = isActive;
        if (editActiveDesc) {
            editActiveDesc.textContent = isActive ? t('edit.activeDescOn') : t('edit.activeDescOff');
        }
        editActiveCheckbox.onchange = function() {
            if (editActiveDesc) {
                editActiveDesc.textContent = this.checked ? t('edit.activeDescOn') : t('edit.activeDescOff');
            }
        };
    }

    // Show modal
    document.getElementById('edit-key-modal').style.display = 'flex';
}

// 计费 Token 与 RMB 换算：1 RMB = 10M 计费 Token（与管理端口径一致）。
// 关键词: OPS formatRMBFromTokens, 1 RMB=10M 计费 Token, 换算文案
const OPS_BILLING_TOKENS_PER_RMB = 10000000; // 10M 计费 Token / RMB
function formatRMBFromTokens(tokens) {
    const n = Number(tokens) || 0;
    const rmb = n / OPS_BILLING_TOKENS_PER_RMB;
    return (rmb % 1 === 0) ? rmb.toFixed(0) : rmb.toFixed(2);
}

// 关键词: calculateEditTokenBytes, OPS Token 限额输入实时换算, RMB 换算提示
function calculateEditTokenBytes() {
    const valueInput = document.getElementById('edit-token-limit-value');
    const unitSelect = document.getElementById('edit-token-limit-unit');
    const calculatedDisplay = document.getElementById('edit-token-calculated');
    const rmbDisplay = document.getElementById('edit-token-calculated-rmb');
    if (!valueInput || !unitSelect) return 0;
    const value = parseFloat(valueInput.value) || 0;
    const multiplier = parseInt(unitSelect.value) || 1;
    const tokens = Math.floor(value * multiplier);
    if (calculatedDisplay) {
        const formatted = formatTokenCount(tokens);
        calculatedDisplay.textContent = t('calc.tokens', { n: tokens.toLocaleString(), f: formatted });
    }
    if (rmbDisplay) {
        rmbDisplay.textContent = t('calc.rmb', { rmb: formatRMBFromTokens(tokens) });
    }
    return tokens;
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
        calculatedDisplay.textContent = t('calc.bytes', { n: bytes.toLocaleString(), f: formatted });
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
    
    // 模型名按索引回查（editToggleModelByIndex），不内联进 onclick；展示文本走 escapeHtml。
    // 关键词: ops renderEditModelList XSS 防护, 索引法 onclick
    modelList.innerHTML = availableModels.map((model, idx) => `
        <div class="model-item ${editSelectedModels.has(model) ? 'selected' : ''}" onclick="editToggleModelByIndex(${idx})">
            <input type="checkbox" ${editSelectedModels.has(model) ? 'checked' : ''} onclick="event.stopPropagation(); editToggleModelByIndex(${idx})">
            <label>${escapeHtml(model)}</label>
        </div>
    `).join('');
}

// editToggleModelByIndex 用索引回查模型名后切换选中，避免内联模型名进 onclick。
function editToggleModelByIndex(idx) {
    const model = availableModels[idx];
    if (model != null) editToggleModel(model);
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
    
    let html = `<strong>${t('common.selectedLabel')}</strong> `;
    
    const modelArray = Array.from(editSelectedModels).sort();
    if (modelArray.length > 0) {
        if (modelArray.length <= 5) {
            html += modelArray.map(m => `<span class="tag">${escapeHtml(m)}</span>`).join('');
        } else {
            html += modelArray.slice(0, 3).map(m => `<span class="tag">${escapeHtml(m)}</span>`).join('');
            html += `<span class="tag">+${modelArray.length - 3}${t('common.moreSuffix')}</span>`;
        }
    }
    
    if (globPatterns) {
        const patterns = globPatterns.split(',').map(p => p.trim()).filter(p => p);
        patterns.forEach(p => {
            html += `<span class="tag glob">${escapeHtml(p)}</span>`;
        });
    }
    
    if (modelArray.length === 0 && !globPatterns) {
        html += `<span style="color: #888;">${t('common.noneSelected')}</span>`;
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
        showToast(t('toast.selectModel'), 'error');
        return;
    }
    
    // Get traffic settings
    const isUnlimited = document.getElementById('edit-unlimited-traffic').checked;
    const trafficLimit = calculateEditTrafficBytes();
    
    if (!isUnlimited && trafficLimit <= 0) {
        showToast(t('toast.validTraffic'), 'error');
        return;
    }

    // 关键词: OPS saveEditKey Token 维度收集 + 校验
    const tokenUnlimitedEl = document.getElementById('edit-token-unlimited');
    const tokenIsUnlimited = tokenUnlimitedEl ? tokenUnlimitedEl.checked : true;
    const tokenLimit = tokenIsUnlimited ? 0 : calculateEditTokenBytes();
    if (!tokenIsUnlimited && tokenLimit <= 0) {
        showToast(t('toast.validToken'), 'error');
        return;
    }
    
    // 绑定用户信息（用户名/备注/metainfo）始终发送，后端用 *string 区分未提供与置空
    // 关键词: OPS saveEditKey username remark metainfo 携带
    const editUsernameEl = document.getElementById('edit-username');
    const editRemarkEl = document.getElementById('edit-remark');
    const editMetaEl = document.getElementById('edit-metainfo');

    // 启用/禁用状态：后端用 *bool 区分未提供，这里始终显式携带当前勾选值
    // 关键词: OPS saveEditKey active 启用禁用携带
    const editActiveEl = document.getElementById('edit-active');

    try {
        const requestBody = {
            api_key: apiKey,
            allowed_models: allModels,
            unlimited: isUnlimited,
            // Token 字段始终发送, 让后端基于 token_unlimited / token_limit 判定
            token_unlimited: tokenIsUnlimited,
            token_limit: tokenLimit,
            username: editUsernameEl ? editUsernameEl.value.trim() : '',
            remark: editRemarkEl ? editRemarkEl.value : '',
            metainfo: editMetaEl ? editMetaEl.value : '',
            active: editActiveEl ? editActiveEl.checked : true
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
            showToast(t('toast.keyUpdated'), 'success');
            closeEditKeyModal();
            loadMyApiKeys();
        } else {
            showToast(data.error || t('toast.keyUpdateFailed'), 'error');
        }
    } catch (error) {
        console.error('Error updating API key:', error);
        showToast(t('common.networkError'), 'error');
    }
}

async function resetEditKeyTraffic() {
    const apiKey = document.getElementById('edit-key-api-key').value;
    
    if (!confirm(t('confirm.resetTraffic'))) {
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
            showToast(t('toast.trafficReset'), 'success');
            document.getElementById('edit-traffic-used').textContent = '0 B';
            loadMyApiKeys();
        } else {
            showToast(data.error || t('toast.trafficResetFailed'), 'error');
        }
    } catch (error) {
        console.error('Error resetting traffic:', error);
        showToast(t('common.networkError'), 'error');
    }
}

// 关键词: resetEditKeyToken, OPS 用户重置自己 Key 的 Token 用量
async function resetEditKeyToken() {
    const apiKey = document.getElementById('edit-key-api-key').value;
    if (!confirm(t('confirm.resetToken'))) {
        return;
    }
    try {
        const response = await fetch('/ops/api/reset-token', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ api_key: apiKey })
        });
        const data = await response.json();
        if (data.success) {
            showToast(t('toast.tokenReset'), 'success');
            const usedEl = document.getElementById('edit-token-used');
            if (usedEl) usedEl.textContent = '0';
            loadMyApiKeys();
        } else {
            showToast(data.error || t('toast.tokenResetFailed'), 'error');
        }
    } catch (error) {
        console.error('Error resetting token:', error);
        showToast(t('common.networkError'), 'error');
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
        calculatedDisplay.textContent = t('calc.bytes', { n: bytes.toLocaleString(), f: formatted });
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
                trafficDesc.textContent = t('trafficDesc.unlimited');
            } else {
                trafficLimitGroup.style.display = 'block';
                unlimitedToggle.classList.remove('active');
                trafficDesc.textContent = t('trafficDesc.custom');
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

    // ==================== Token toggles (Create form) ====================
    // 关键词: OPS Create API Key Token 限额 toggle, 推荐使用 Token over Traffic
    const tokenUnlimitedCheckbox = document.getElementById('unlimited-token');
    const tokenLimitGroup = document.getElementById('token-limit-group');
    const tokenUnlimitedToggle = document.getElementById('token-unlimited-toggle');
    const tokenDesc = document.getElementById('token-desc');
    if (tokenUnlimitedCheckbox && tokenLimitGroup) {
        tokenUnlimitedCheckbox.addEventListener('change', function() {
            if (this.checked) {
                tokenLimitGroup.style.display = 'none';
                if (tokenUnlimitedToggle) tokenUnlimitedToggle.classList.add('active');
                if (tokenDesc) tokenDesc.textContent = t('tokenDesc.unlimited');
            } else {
                tokenLimitGroup.style.display = 'block';
                if (tokenUnlimitedToggle) tokenUnlimitedToggle.classList.remove('active');
                if (tokenDesc) tokenDesc.textContent = t('tokenDesc.custom');
                calculateTokenBytes();
            }
        });
    }
    const tokenValueInput = document.getElementById('token-limit-value');
    const tokenUnitSelect = document.getElementById('token-limit-unit');
    if (tokenValueInput) tokenValueInput.addEventListener('input', calculateTokenBytes);
    if (tokenUnitSelect) tokenUnitSelect.addEventListener('change', calculateTokenBytes);
    
    // Glob patterns input listener
    const globInput = document.getElementById('glob-patterns');
    if (globInput) {
        globInput.addEventListener('input', updateSelectedPreview);
    }
}

// 关键词: calculateTokenBytes, OPS Create API Key Token 限额输入实时换算, RMB 换算提示
function calculateTokenBytes() {
    const valueInput = document.getElementById('token-limit-value');
    const unitSelect = document.getElementById('token-limit-unit');
    const calculatedDisplay = document.getElementById('token-calculated');
    const rmbDisplay = document.getElementById('token-calculated-rmb');
    if (!valueInput || !unitSelect) return 0;
    const value = parseFloat(valueInput.value) || 0;
    const multiplier = parseInt(unitSelect.value) || 1;
    const tokens = Math.floor(value * multiplier);
    if (calculatedDisplay) {
        const formatted = (typeof formatTokenCount === 'function') ? formatTokenCount(tokens) : tokens.toString();
        calculatedDisplay.textContent = t('calc.tokens', { n: tokens.toLocaleString(), f: formatted });
    }
    if (rmbDisplay) {
        rmbDisplay.textContent = t('calc.rmb', { rmb: formatRMBFromTokens(tokens) });
    }
    return tokens;
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

    // 关键词: OPS handleCreateApiKey Token 限额参数收集
    const tokenUnlimitedCheckbox = document.getElementById('unlimited-token');
    const tokenIsUnlimited = tokenUnlimitedCheckbox ? tokenUnlimitedCheckbox.checked : true;
    const tokenLimit = tokenIsUnlimited ? 0 : calculateTokenBytes();
    
    if (allModels.length === 0) {
        showAlert('create-key-alert', t('toast.selectModel'), 'error');
        return;
    }
    
    // Validate traffic limit if not unlimited
    if (!isUnlimited && trafficLimit <= 0) {
        showAlert('create-key-alert', t('toast.validTraffic'), 'error');
        return;
    }
    if (!tokenIsUnlimited && tokenLimit <= 0) {
        showAlert('create-key-alert', t('toast.validToken'), 'error');
        return;
    }
    
    // 绑定用户信息（可选）：用户名(可重复)/备注/metainfo
    // 关键词: OPS handleCreateApiKey username remark metainfo 携带
    const createUsernameEl = document.getElementById('create-username');
    const createRemarkEl = document.getElementById('create-remark');
    const createMetaEl = document.getElementById('create-metainfo');

    try {
        const requestBody = {
            allowed_models: allModels,
            unlimited: isUnlimited,
            token_unlimited: tokenIsUnlimited,
            username: createUsernameEl ? createUsernameEl.value.trim() : '',
            remark: createRemarkEl ? createRemarkEl.value : '',
            metainfo: createMetaEl ? createMetaEl.value : ''
        };
        
        if (!isUnlimited && trafficLimit > 0) {
            requestBody.traffic_limit = trafficLimit;
        }
        if (!tokenIsUnlimited && tokenLimit > 0) {
            requestBody.token_limit = tokenLimit;
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
            const trafficInfo = data.unlimited ? t('common.unlimited') : formatBytes(data.traffic_limit);
            const tokenInfo = (data.token_limit_enable && data.token_limit > 0)
                ? formatTokenCount(data.token_limit)
                : t('common.unlimited');
            showAlert('create-key-alert', t('create.successAlert', { traffic: trafficInfo, token: tokenInfo }), 'success');
            document.getElementById('generated-key').textContent = data.api_key;
            document.getElementById('api-key-result').classList.add('show');
            
            // Clear form (wrap in try-catch to not affect success message)
            try {
                selectedModels.clear();
                renderModelList();
                if (globInput) globInput.value = '';
                updateSelectedPreview();
                if (createUsernameEl) createUsernameEl.value = '';
                if (createRemarkEl) createRemarkEl.value = '';
                if (createMetaEl) createMetaEl.value = '';
            } catch (clearError) {
                console.error('Error clearing form:', clearError);
            }
            
            // Refresh user info to update stats (async, don't wait)
            loadUserInfo().catch(err => console.error('Error refreshing user info:', err));
        } else {
            showAlert('create-key-alert', data.error || t('alert.createKeyFailed'), 'error');
        }
    } catch (error) {
        console.error('Error creating API key:', error);
        showAlert('create-key-alert', t('alert.networkRetry'), 'error');
    }
}

async function handleChangePassword(e) {
    e.preventDefault();
    
    const oldPassword = document.getElementById('old-password').value;
    const newPassword = document.getElementById('new-password').value;
    const confirmPassword = document.getElementById('confirm-password').value;
    
    if (newPassword !== confirmPassword) {
        showAlert('settings-alert', t('alert.passwordMismatch'), 'error');
        return;
    }
    
    if (newPassword.length < 8) {
        showAlert('settings-alert', t('alert.passwordTooShort'), 'error');
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
            showAlert('settings-alert', t('alert.passwordChanged'), 'success');
            document.getElementById('change-password-form').reset();
        } else {
            showAlert('settings-alert', data.error || t('alert.passwordChangeFailed'), 'error');
        }
    } catch (error) {
        console.error('Error changing password:', error);
        showAlert('settings-alert', t('alert.networkRetry'), 'error');
    }
}

// ==================== OPS Key Management ====================

async function resetOpsKey() {
    if (!confirm(t('confirm.resetOpsKey'))) {
        return;
    }
    
    try {
        const response = await fetch('/ops/reset-key', {
            method: 'POST'
        });
        
        const data = await response.json();
        
        if (data.success) {
            alert(t('opsKey.resetSuccess') + data.new_ops_key);
            loadUserInfo();
        } else {
            alert(t('opsKey.resetFailed') + (data.error || t('opsKey.unknownError')));
        }
    } catch (error) {
        console.error('Error resetting OPS key:', error);
        alert(t('alert.networkRetry'));
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
        'curl-list-keys': `curl -X GET '${endpoint}/ops/api/my-keys?page=1&page_size=20' \\
  -H 'X-Ops-Key: ${opsKey}'`,
        'curl-list-keys-filtered': `curl -G '${endpoint}/ops/api/my-keys' \\
  -H 'X-Ops-Key: ${opsKey}' \\
  --data-urlencode 'q=alice' \\
  --data-urlencode 'active=true' \\
  --data-urlencode 'page=1' \\
  --data-urlencode 'page_size=20'`,
        'curl-update-key': `curl -X POST '${endpoint}/ops/api/update-api-key' \\
  -H 'Content-Type: application/json' \\
  -H 'X-Ops-Key: ${opsKey}' \\
  -d '{
    "api_key": "mf-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
    "allowed_models": ["gpt-4", "claude-*"],
    "token_limit": 1000000,
    "traffic_limit": 209715200,
    "unlimited": false,
    "username": "alice",
    "remark": "team-a quota",
    "metainfo": "{\\"team\\":\\"a\\"}",
    "active": true
  }'`,
        'curl-toggle-key': `curl -X POST '${endpoint}/ops/api/update-api-key' \\
  -H 'Content-Type: application/json' \\
  -H 'X-Ops-Key: ${opsKey}' \\
  -d '{
    "api_key": "mf-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
    "active": false
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
        showToast(t('toast.curlCopied'), 'success');
    }).catch(err => {
        console.error('Failed to copy:', err);
        showToast(t('toast.copyFailed'), 'error');
    });
}

// ==================== Clipboard ====================

function copyApiKey() {
    const key = document.getElementById('generated-key').textContent;
    copyToClipboard(key).then(() => {
        showToast(t('toast.keyCopied'), 'success');
    }).catch(err => {
        console.error('Failed to copy:', err);
        showToast(t('toast.copyFailed'), 'error');
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

// 关键词: OPS formatTokenCount, K/M/B tokens 展示, 与 portal 端对齐
function formatTokenCount(n) {
    const num = Number(n) || 0;
    if (num === 0) return '0';
    const abs = Math.abs(num);
    if (abs < 1000) return String(num);
    if (abs < 1_000_000) return (num / 1000).toFixed(num % 1000 === 0 ? 0 : 1) + 'K';
    if (abs < 1_000_000_000) return (num / 1_000_000).toFixed(num % 1_000_000 === 0 ? 0 : 2) + 'M';
    return (num / 1_000_000_000).toFixed(2) + 'B';
}

// ==================== Export to window ====================

window.resetOpsKey = resetOpsKey;
window.copyApiKey = copyApiKey;
window.copyCurlExample = copyCurlExample;
window.deleteApiKey = deleteApiKey;
window.loadMyApiKeys = loadMyApiKeys;
// 关键词: OPS my-keys 启用禁用/过滤 window 导出
window.toggleApiKeyActive = toggleApiKeyActive;
window.applyMyKeysFilter = applyMyKeysFilter;
window.clearMyKeysFilter = clearMyKeysFilter;
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
// 关键词: OPS Token 限额相关 window 导出
window.resetEditKeyToken = resetEditKeyToken;
window.calculateEditTokenBytes = calculateEditTokenBytes;
window.calculateTokenBytes = calculateTokenBytes;
window.formatTokenCount = formatTokenCount;
// My Keys pagination functions
window.changeMyKeysPage = changeMyKeysPage;
window.changeMyKeysPageSize = changeMyKeysPageSize;
// 关键词: OPS i18n window 导出, 页头语言切换按钮 onclick 调用 toggleOpsLang
window.toggleOpsLang = toggleOpsLang;
window.setOpsLang = setOpsLang;
window.t = t;
