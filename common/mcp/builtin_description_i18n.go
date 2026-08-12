package mcp

import (
	"github.com/yaklang/yaklang/common/schema"
)

// builtinToolDescriptionI18n is the unified bilingual UI description table for
// builtin MCP tools (same shape as schema nodeIdMapper / AIOutputI18n).
//
// Wire rules:
//   - mcp.Tool.Description (WithDescription) remains English for AI / MCP tools/list
//   - UI DescriptionI18n Zh/En come from this table at export time
var builtinToolDescriptionI18n = map[string]*schema.I18n{
	// global_hotpatch
	"get_global_hotpatch_config":      schema.NewI18n("获取当前全局热加载配置，包括启用状态、版本和生效模板", "Get current global hotpatch config (enabled, version, active template)"),
	"enable_global_hotpatch":          schema.NewI18n("启用全局热加载模板；对新的 MITM 请求与 WebFuzzer 任务生效", "Enable global hotpatch for new MITM / WebFuzzer tasks"),
	"disable_global_hotpatch":         schema.NewI18n("关闭全局热加载；新的 MITM 与 WebFuzzer 不再执行全局层", "Disable global hotpatch for new MITM / WebFuzzer tasks"),
	"reset_global_hotpatch_config":    schema.NewI18n("重置全局热加载为默认关闭状态，并清空生效模板", "Reset global hotpatch to disabled and clear active templates"),
	"create_global_hotpatch_template": schema.NewI18n("在配置库中创建全局热加载模板；脚本需定义 beforeRequest/afterRequest，再用 enable_global_hotpatch 激活", "Create a global hotpatch template (beforeRequest/afterRequest), then enable it"),
	"query_hotpatch_template_list":    schema.NewI18n("列出可用热加载模板名称，可按类型过滤（fuzzer/mitm/httpflow-analyze/global）", "List HotPatchTemplate names, optionally filtered by type"),

	// core scan / network
	"brute":                schema.NewI18n("根据参数发起爆破（暴力破解）任务", "Start a brute-force attack with the given parameters"),
	"hybrid_scan":          schema.NewI18n("对多个目标执行 Yak 脚本混合扫描", "Run Yak scripts across multiple targets (hybrid scan)"),
	"port_scan":            schema.NewI18n("对目标进行端口扫描", "Scan ports on targets"),
	"query_ports":          schema.NewI18n("按条件查询端口扫描结果", "Query port-scan results with filters"),
	"delete_ports":         schema.NewI18n("按条件删除端口扫描结果", "Delete port-scan results with filters"),
	"subdomain_collection": schema.NewI18n("收集指定域名的子域名", "Collect subdomains for a target domain"),
	"web_crawler":          schema.NewI18n("网站爬虫，用于爬取站点页面", "Crawl websites"),

	// http / fuzzer
	"http_fuzzer":                 schema.NewI18n("按参数发送 HTTP 数据包，可直接使用 fuzztag", "Send HTTP packets; fuzztag supported"),
	"create_web_fuzzer_tab":       schema.NewI18n("在 Yakit 中创建单个 Web Fuzzer 标签页；多步复现请用 create_web_fuzzer_tabs", "Create one Web Fuzzer tab; use create_web_fuzzer_tabs for multi-step flows"),
	"create_web_fuzzer_tabs":      schema.NewI18n("批量创建多个 Web Fuzzer 标签页并一次推送到 Yakit，适合漏洞复现多步骤展示", "Create multiple Web Fuzzer tabs in one push for multi-step repro"),
	"query_web_fuzzer_tabs":       schema.NewI18n("查询当前用户可见的 Web Fuzzer 标签页/分组状态与可用颜色，修改前应先调用", "List visible Web Fuzzer tabs/groups and colors; call before mutations"),
	"update_web_fuzzer_tab":       schema.NewI18n("部分更新已有 Web Fuzzer 标签页；可用 targetGroupId 移入分组，或 \"0\" 取消分组", "Partially update a Web Fuzzer tab; targetGroupId moves/ungroups"),
	"delete_web_fuzzer_tabs":      schema.NewI18n("按 pageId 删除 Web Fuzzer 标签页；空分组会自动清理", "Delete Web Fuzzer tabs by pageId; empty groups are cleaned up"),
	"manage_web_fuzzer_tab_group": schema.NewI18n("创建/更新/删除 Web Fuzzer 标签页分组；先 query_web_fuzzer_tabs，再选不冲突的颜色", "Create/update/delete Web Fuzzer tab groups"),

	// httpflow
	"query_http_flow":       schema.NewI18n("按条件查询 HTTP 流量记录", "Query HTTP flows with filters"),
	"set_tag_for_http_flow": schema.NewI18n("为 HTTP 流量设置标签", "Set tags on an HTTP flow"),
	"delete_http_flow":      schema.NewI18n("按条件删除 HTTP 流量记录", "Delete HTTP flows with filters"),

	// codec
	"render_fuzztag":       schema.NewI18n("渲染 fuzztag（HTTP Fuzzer DSL），例如 {{int(1-10)}} 展开为 1..10", "Render fuzztag DSL, e.g. {{int(1-10)}} → 1..10"),
	"codec_method_details": schema.NewI18n("获取 Codec 方法详情（名称、说明、参数），配合 exec_codec 使用", "Get codec method details; use with exec_codec"),
	"exec_codec":           schema.NewI18n("执行多步编解码 Codec 工作流", "Run a multi-step codec workflow"),

	// cve / risk
	"query_cve":        schema.NewI18n("按条件查询 CVE", "Query CVEs with filters"),
	"query_risks":      schema.NewI18n("分页查询漏洞/风险记录（扫描、插件、MITM、OOB 等）", "Page vulnerability/risk records"),
	"query_risk":       schema.NewI18n("按 id 或 hash 获取单条风险详情（含完整请求响应）", "Get one risk by id or hash"),
	"delete_risk":      schema.NewI18n("按 id/hash/过滤条件删除风险，或批量清理", "Delete risks by id/hash/filter or bulk cleanup"),
	"query_new_risks":  schema.NewI18n("按 afterId 增量拉取新风险，用于自动化轮询", "Poll incremental risks with id > afterId"),
	"set_tag_for_risk": schema.NewI18n("替换风险标签（id 或 hash）", "Replace tags on a risk"),

	// fingerprint
	"query_fingerprint":  schema.NewI18n("分页查询 HTTP 服务指纹规则（端口扫描/爬虫使用）", "Page HTTP fingerprint rules"),
	"create_fingerprint": schema.NewI18n("创建自定义指纹规则（需 ruleName 与 matchExpression）", "Create a custom fingerprint rule"),
	"update_fingerprint": schema.NewI18n("按 id 或 ruleName 更新指纹规则字段", "Update fingerprint by id or ruleName"),
	"delete_fingerprint": schema.NewI18n("按过滤条件删除指纹规则", "Delete fingerprint rules by filter"),

	// mitm
	"get_mitm_filter":           schema.NewI18n("读取 MITM 抓包过滤（主机、URI、方法、MIME 等）", "Read MITM capture filter"),
	"set_mitm_filter":           schema.NewI18n("更新 MITM 抓包过滤规则", "Update MITM capture filter"),
	"query_mitm_replacer_rules": schema.NewI18n("搜索已保存的 MITM 替换规则库（不一定当前生效）", "Search saved MITM replacer rules"),
	"get_current_rules":         schema.NewI18n("获取当前实时拦截生效的 MITM 替换规则", "Get active MITM replacer rules"),
	"set_current_rules":         schema.NewI18n("立即替换运行中代理的 MITM 替换规则；建议先 get_current_rules", "Replace active MITM replacers on the running proxy"),
	"download_mitm_cert":        schema.NewI18n("下载 MITM 根证书 PEM，安装到客户端信任库后再代理到 start_mitm_v2", "Download MITM root CA PEM"),
	"start_mitm_v2":             schema.NewI18n("后台启动 MITM v2 HTTPS 代理（status:started）", "Start MITM v2 HTTPS proxy in background"),

	// project database
	"get_current_database_context":    schema.NewI18n("获取当前 MCP 数据库上下文（Yakit Home、工程库路径与元数据）", "Get current MCP database context"),
	"list_project_databases":          schema.NewI18n("列出 Yakit 配置库中的工程数据库", "List project databases from Yakit profile"),
	"switch_current_project_database": schema.NewI18n("按工程 id 切换当前工程数据库（影响当前 MCP/Yak 进程）", "Switch current project database by id"),
	"create_project_database":         schema.NewI18n("创建新的工程数据库，可选切换为当前工程", "Create a project database; optionally switch to it"),

	// payload
	"list_all_payload_dictionary_details": schema.NewI18n("列出全部字典详情（文件/数据库/文件夹）", "List all payload dictionary details"),
	"query_payload":                       schema.NewI18n("按条件查询 Payload", "Query payloads with filters"),
	"save_payload":                        schema.NewI18n("将 Payload 保存到数据库", "Save payload(s) to database"),
	"create_payload_folder":               schema.NewI18n("创建 Payload 文件夹", "Create a payload folder"),
	"delete_payload":                      schema.NewI18n("按组或文件夹删除 Payload", "Delete payload by group or folder"),
	"rename_payload_group":                schema.NewI18n("重命名 Payload 字典（组名）", "Rename a payload group"),
	"rename_payload_folder":               schema.NewI18n("重命名 Payload 文件夹", "Rename a payload folder"),
	"update_one_payload":                  schema.NewI18n("更新单条 Payload", "Update one payload"),
	"update_payload_file_content":         schema.NewI18n("更新文件类型 Payload 的全部内容", "Update full content of a file-type payload"),

	// reverse
	"generate_reverse_shell_command": schema.NewI18n("生成反弹 Shell 命令", "Generate a reverse shell command"),
	"get_bridge_log_server":          schema.NewI18n("读取已保存的 Yak Bridge DNSLog/反连服务配置", "Read persisted Yak Bridge DNSLog/reverse config"),
	"set_bridge_log_server":          schema.NewI18n("持久化 Yak Bridge DNSLog/反连服务配置", "Persist Yak Bridge DNSLog/reverse config"),
	"get_global_reverse_server":      schema.NewI18n("获取 Yak 全局反连地址信息", "Get Yak global reverse addresses"),
	"require_dnslog_domain":          schema.NewI18n("申请 DNSLog 子域名与 token，用于 OOB 检测", "Request DNSLog domain and token for OOB"),
	"query_dnslog_by_token":          schema.NewI18n("按 token 查询 DNSLog 命中事件", "Query DNSLog hits by token"),
	"require_random_port_token":      schema.NewI18n("经 Yak Bridge 申请随机高端口反连 token", "Request random high-port reverse token via Bridge"),
	"query_random_port_trigger":      schema.NewI18n("查询随机端口反连是否命中", "Query TCP reverse hit for a random-port token"),
	"start_facades":                  schema.NewI18n("后台启动本地 Facades（嵌入式 DNSLog/RMI/HTTP）", "Start local Facades (DNSLog/RMI/HTTP) in background"),

	// system
	"get_system_proxy": schema.NewI18n("获取系统代理设置", "Get system proxy"),
	"set_system_proxy": schema.NewI18n("设置系统代理", "Set system proxy"),
	"notify":           schema.NewI18n("向用户发送通知消息", "Send a notification to the user"),

	// ssa / syntaxflow
	"ssa_compile":                schema.NewI18n("将源码工程编译为 SSA 中间表示", "Compile a project into SSA IR"),
	"ssa_query":                  schema.NewI18n("在已编译的 SSA 程序上执行 SyntaxFlow 数据流查询", "Run a SyntaxFlow query on a compiled SSA program"),
	"query_syntaxflow_rule":      schema.NewI18n("分页查询 SyntaxFlow 规则", "Page SyntaxFlow rules"),
	"create_syntaxflow_rule":     schema.NewI18n("创建自定义 SyntaxFlow 规则", "Create a custom SyntaxFlow rule"),
	"update_syntaxflow_rule":     schema.NewI18n("按 ruleName 更新规则字段", "Update a SyntaxFlow rule by ruleName"),
	"query_syntaxflow_result":    schema.NewI18n("分页查询 SyntaxFlow 命中结果", "Page SyntaxFlow hit results"),
	"query_syntaxflow_scan_task": schema.NewI18n("分页查询 SyntaxFlow 扫描任务状态与进度", "Page SyntaxFlow scan tasks"),
	"syntaxflow_scan":            schema.NewI18n("后台启动/暂停/恢复 SSA SyntaxFlow 批量扫描", "Start/pause/resume SSA SyntaxFlow batch scan"),

	// yso
	"get_all_yso_gadget_options":          schema.NewI18n("列出 YSO Java 反序列化 gadget 链名称", "List YSO Java deserialization gadget names"),
	"get_all_yso_class_options":           schema.NewI18n("列出指定 gadget 链可用的 payload 类", "List payload classes for a gadget chain"),
	"get_all_yso_class_generater_options": schema.NewI18n("列出 gadget+class 所需的生成器参数", "List generator options for gadget+class"),
	"generate_yso_bytes":                  schema.NewI18n("生成 Java 序列化利用字节（响应中为 base64）", "Generate Java serialized exploit bytes (base64)"),
	"yso_dump":                            schema.NewI18n("解析并检查 generate_yso_bytes 输出的序列化结构", "Inspect serialization structure from generate_yso_bytes"),

	// yak document / script
	"yakdoc_get_all_library_names": schema.NewI18n("YakDocument：获取全部标准库名称", "YakDocument: list standard library names"),
	"yakdoc_library_details":       schema.NewI18n("YakDocument：获取标准库详情（函数与变量名）", "YakDocument: library details (functions/variables)"),
	"yakdoc_function_details":      schema.NewI18n("YakDocument：获取标准函数详情（说明与参数）", "YakDocument: function details"),
	"yakdoc_variable_details":      schema.NewI18n("YakDocument：获取标准变量详情", "YakDocument: variable details"),
	"static_analyze_yak_script":    schema.NewI18n("用 yaklang 引擎静态分析插件源码并返回可读问题列表", "Static-analyze Yak plugin source and return issues"),
	"query_yak_script":             schema.NewI18n("按条件查询 Yak 脚本/插件", "Query Yak scripts with filters"),
	"exec_yak_script":              schema.NewI18n("按源码或脚本名执行 Yak 脚本", "Execute a Yak script by code or name"),
	"save_yak_script":              schema.NewI18n("创建或更新本地 Yakit 插件（保存前会静态分析）", "Create or update a local Yakit plugin"),
	"create_yak_script_group":      schema.NewI18n("创建 Yak 脚本分组", "Create a Yak script group"),
	"list_yak_script_group":        schema.NewI18n("列出 Yak 脚本分组信息", "List Yak script groups"),
	"query_yak_script_group":       schema.NewI18n("按过滤后的脚本查询所属分组名", "Query group names for filtered Yak scripts"),
	"set_group_for_yak_script":     schema.NewI18n("批量为脚本添加/移除分组", "Add/remove groups for Yak scripts"),
	"rename_yak_script_group":      schema.NewI18n("重命名 Yak 脚本分组", "Rename a Yak script group"),
	"delete_yak_script_group":      schema.NewI18n("删除 Yak 脚本分组（请反复确认）", "Delete a Yak script group (confirm carefully)"),
	"query_online_yak_script":      schema.NewI18n("按条件查询线上 Yak 脚本", "Query online Yak scripts"),
	"download_online_yak_script":   schema.NewI18n("按条件将线上 Yak 脚本下载到本地", "Download online Yak scripts to local"),

	// dynamic
	"dynamic_add_tool": schema.NewI18n("根据 Yak 脚本内容动态添加工具", "Dynamically add a tool from Yak script content"),
}

// BuiltinToolDescriptionI18n returns the unified UI i18n entry for a builtin tool, or nil.
func BuiltinToolDescriptionI18n(name string) *schema.I18n {
	return builtinToolDescriptionI18n[name]
}

// ResolveBuiltinToolDescriptionI18n builds schema.I18n for UI export.
// Table Zh/En win when present; otherwise En falls back to the AI Description.
func ResolveBuiltinToolDescriptionI18n(name, description string) *schema.I18n {
	entry := BuiltinToolDescriptionI18n(name)
	zh := ""
	en := description
	if entry != nil {
		zh = entry.Zh
		if entry.En != "" {
			en = entry.En
		}
	}
	return schema.NewI18n(zh, en)
}
