package model

import (
	"fmt"
	"strings"
)

// SinkHint 描述一类 Sink 的语义特征和跨语言示例。
// 不硬编码具体的 grep 关键词——AI 在阶段A看到技术栈后自行决定搜什么。
type SinkHint struct {
	// Name 组名，如 "SQL执行函数"
	Name string
	// Description 该组 Sink 的漏洞含义
	Description string
	// Examples 各语言的典型 Sink 示例（仅供 AI 参考，不直接用于 grep）
	Examples []string
}

// ChecklistItem 检查清单项，用于追踪专题覆盖率
type ChecklistItem struct {
	ID          string
	Description string
}

// VulnCategory 描述单个漏洞类别的扫描配置
type VulnCategory struct {
	// ID 类别唯一标识，与 Finding.Category 对应
	ID string
	// Name 中文名称
	Name string
	// SinkHints Sink 语义提示列表。
	// 每项描述一类 Sink 的漏洞模式，并附典型示例供 AI 参考。
	// AI 根据 Phase1 侦察到的实际技术栈，自主选择合适的 grep 关键词。
	SinkHints []SinkHint
	// Checklist 覆盖率清单
	Checklist []ChecklistItem
	// Instruction 该类别专属的扫描指南
	Instruction string
}

// RenderSinkHints 将 SinkHints 渲染为阶段A可读的提示文本。
// 供 ReactiveData 模板直接嵌入，引导 AI 自主推导关键词。
func (c *VulnCategory) RenderSinkHints() string {
	if len(c.SinkHints) == 0 {
		return ""
	}
	var result strings.Builder
	for _, h := range c.SinkHints {
		result.WriteString(fmt.Sprintf("- **%s**：%s\n  典型示例（参考，非固定关键词）：%v\n", h.Name, h.Description, h.Examples))
	}
	return result.String()
}

// DefaultVulnCategories 默认漏洞扫描类别（Web/应用层 + 基础设施常见面）。
// SinkHints 只描述"这类 Sink 长什么样"，不硬编码 grep 关键词。
// AI 在阶段A根据已知技术栈自主选择实际搜索词。
var DefaultVulnCategories = []VulnCategory{
	{
		ID:   "sql_injection",
		Name: "SQL 注入",
		SinkHints: []SinkHint{
			{
				Name:        "直接执行 SQL 字符串的函数",
				Description: "将 SQL 字符串直接发送给数据库执行的函数。重点关注参数是否通过字符串拼接构造。",
				Examples:    []string{"PHP: mysqli_query, ->query, ->execute", "Java: Statement.execute, createNativeQuery", "Python: cursor.execute", "Go: db.Exec, db.Query, db.Raw", "Node: knex.raw, sequelize.query"},
			},
			{
				Name:        "ORM Raw/原生查询方法",
				Description: "ORM 框架中允许嵌入原始 SQL 片段的方法，容易将用户输入直接拼进 SQL。",
				Examples:    []string{"Laravel: whereRaw, selectRaw, orderByRaw, DB::statement", "Django: .raw(), RawSQL", "GORM: .Where(\"...\"+input), .Raw()", "Hibernate: createNativeQuery"},
			},
			{
				Name:        "用户输入来源（Source 端）",
				Description: "HTTP 请求参数获取函数，用于枚举哪些变量携带用户可控数据。",
				Examples:    []string{"PHP: $_GET, $_POST, $_REQUEST", "Java: getParameter, getHeader", "Go: c.Query, c.PostForm, r.URL.Query", "Python: request.form, request.args", "Node: req.query, req.body"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "sources_enumerated", Description: "已枚举所有 HTTP 参数获取点"},
			{ID: "sinks_searched", Description: "已搜索所有 SQL 执行 Sink"},
			{ID: "parameterized_checked", Description: "已确认命中 Sink 是否使用参数化查询"},
			{ID: "dataflow_traced", Description: "已追踪 source→sink 数据流"},
		},
		Instruction: `## 当前任务：SQL 注入扫描

你只负责搜索 **SQL 注入** 漏洞。

### 搜索策略

**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**，只获取文件列表，避免遗漏。

根据 Phase1 侦察到的技术栈，自主决定 grep 关键词。搜索关键词应简短精准：
- 搜 SQL 执行 Sink（例如 Go 项目搜 "Exec("、"Query("、"Raw("、"Where(" 等简短关键词，每个关键词单独搜索）
- 搜用户输入来源（$_GET/$_POST/getParameter/c.Query 等），了解哪些变量接收用户数据
- **不要用过于复杂的正则**（如同时匹配 Exec 和 fmt.Sprintf），跨行调用会导致漏匹配

### 漏洞判断标准

**是漏洞**：用户可控输入直接拼接进 SQL 字符串，中间无参数化。
**不是漏洞**：使用占位符（?, :name）、ORM 的 find/where 方法（非 Raw）、intval()/(int) 强转。

### data_flow 格式

HTTP入口 → 处理函数[文件:行] → db.Query("... " + param)[文件:行]`,
	},

	{
		ID:   "cmd_injection",
		Name: "命令注入",
		SinkHints: []SinkHint{
			{
				Name:        "系统命令执行函数",
				Description: "调用操作系统 shell 或外部进程的函数。若命令字符串包含用户输入则危险。",
				Examples:    []string{"PHP: system, exec, shell_exec, passthru, popen, proc_open", "Java: Runtime.exec, ProcessBuilder", "Python: os.system, subprocess.run/call/Popen, os.popen", "Go: exec.Command", "Node: child_process.exec, execSync, spawn"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "sources_enumerated", Description: "已枚举所有用户输入来源"},
			{ID: "sinks_searched", Description: "已搜索所有命令执行 Sink"},
			{ID: "string_concat_checked", Description: "已确认命令参数构造方式（拼接 vs 数组）"},
		},
		Instruction: `## 当前任务：命令注入扫描

你只负责搜索 **命令注入** 漏洞。

### 搜索策略

**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**，只获取文件列表，避免遗漏。

根据技术栈选择对应语言的命令执行函数关键词进行 grep，关键词应简短精准。
重点判断：命令字符串是通过拼接用户输入构造（危险），还是参数数组形式（安全）。

### 漏洞判断标准

**是漏洞**：用户可控输入通过字符串拼接构造命令（如 "bash -c " + userInput）。
**不是漏洞**：命令和参数完全硬编码、参数数组形式、严格白名单验证。

### 重点关注

bash -c 字符串形式、管道符、分号注入（a; whoami）、反引号注入。`,
	},

	{
		ID:   "path_traversal",
		Name: "路径遍历/文件操作",
		SinkHints: []SinkHint{
			{
				Name:        "文件读写/包含函数",
				Description: "直接操作文件系统的函数。若路径参数来自用户输入且无目录边界校验，可读取/写入任意文件。",
				Examples:    []string{"PHP: include, require, file_get_contents, file_put_contents, fopen, readfile, unlink", "Java: new File, FileInputStream, Files.readAllBytes, Paths.get", "Python: open, os.path.join, pathlib.Path", "Go: os.Open, os.ReadFile, ioutil.ReadFile", "Node: fs.readFile, fs.writeFile, path.join"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "sources_enumerated", Description: "已枚举文件路径相关的用户输入点"},
			{ID: "sinks_searched", Description: "已搜索所有文件操作 Sink"},
			{ID: "path_sanitization_checked", Description: "已检查路径是否经过 Clean/basename/realpath 处理"},
			{ID: "directory_boundary_checked", Description: "已检查是否有目录边界校验"},
		},
		Instruction: `## 当前任务：路径遍历/文件操作扫描

你只负责搜索 **路径遍历** 和 **不安全文件操作** 漏洞。

### 搜索策略

**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**，只获取文件列表，避免遗漏。

根据技术栈选择对应的文件操作函数关键词 grep，关键词应简短精准。重点关注文件路径的来源和净化方式。

### 漏洞判断标准

**是漏洞**：用户可控输入构造文件路径，且无目录边界校验。
**不是漏洞**：路径限制在白名单目录内（Clean + HasPrefix）、basename() 处理、路径完全由代码控制。

### 重点关注

../../../ 遍历、URL 编码绕过 %2e%2e%2f、PHP include/require 动态文件名（文件包含）。
路径展开/规范化中的长度计算错误（欠分配）若导致内存破坏，同时可记入或交叉关注 ` + "`memory_safety`" + `。`,
	},

	{
		ID:   "xxe_ssrf",
		Name: "XXE / SSRF",
		SinkHints: []SinkHint{
			{
				Name:        "XML 解析器",
				Description: "若解析器未禁用外部实体（XXE），解析用户提交的 XML 可读取任意文件或触发 SSRF。",
				Examples:    []string{"Java: DocumentBuilderFactory, SAXParserFactory, XMLReader, XMLInputFactory", "PHP: simplexml_load_string, DOMDocument, xml_parse", "Python: etree.parse, minidom.parseString, ElementTree.parse"},
			},
			{
				Name:        "发起 HTTP 请求的函数",
				Description: "若目标 URL 来自用户输入且无内网地址过滤，可触发 SSRF 访问内网服务。",
				Examples:    []string{"PHP: curl_exec, file_get_contents(http...), fsockopen", "Java: HttpURLConnection, new URL, OkHttpClient, RestTemplate", "Python: requests.get/post, urllib.request.urlopen", "Go: http.Get, http.Post, http.Do"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "xml_parsers_found", Description: "已枚举所有 XML 解析器位置"},
			{ID: "external_entity_disabled", Description: "已检查是否禁用外部实体"},
			{ID: "http_clients_found", Description: "已枚举所有 HTTP 请求发起位置"},
			{ID: "ssrf_filter_checked", Description: "已检查是否有内网 IP 过滤"},
		},
		Instruction: `## 当前任务：XXE / SSRF 扫描

你只负责搜索 **XXE** 和 **SSRF** 漏洞。

### 搜索策略

**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**，只获取文件列表，避免遗漏。

根据技术栈选择 XML 解析器和 HTTP 客户端的对应关键词进行 grep，关键词应简短精准。

### XXE 判断标准

XML 解析器未禁用外部实体（Java: setFeature 未设置 FEATURE_SECURE_PROCESSING；PHP: libxml_disable_entity_loader 未调用）。

### SSRF 判断标准

用户可控 URL/IP 传入 HTTP 请求函数，未限制访问内网地址（10.x, 192.168.x, 127.0.0.1, 169.254.169.254）。
重点：部分路径用了加固 DialContext，但其它路径仍用默认 http.Get / file:// 绕过；迁移/webhook/avatar/OAuth 回调等易漏点。`,
	},

	{
		ID:   "deserialization",
		Name: "不安全的反序列化",
		SinkHints: []SinkHint{
			{
				Name:        "反序列化函数",
				Description: "将外部数据还原为对象的函数。若数据来自用户可控输入，可触发任意代码执行（Gadget Chain）。",
				Examples:    []string{"PHP: unserialize", "Java: ObjectInputStream.readObject, XMLDecoder, XStream, Jackson enableDefaultTyping", "Python: pickle.loads, yaml.load（非 SafeLoader）, marshal.loads", "Node: node-serialize unserialize"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "deser_sinks_found", Description: "已找到所有反序列化 Sink"},
			{ID: "data_source_checked", Description: "已追踪反序列化数据来源是否用户可控"},
			{ID: "gadget_chain_checked", Description: "已检查依赖中是否有可用 Gadget"},
		},
		Instruction: `## 当前任务：不安全的反序列化扫描

你只负责搜索 **不安全的反序列化** 漏洞（对象重建 / Gadget Chain）。

C/C++ 协议载荷导致的内存破坏（UAF/OOB）归 ` + "`memory_safety`" + `，不要在本类用「反序列化」名义漏报或错报。

### 搜索策略

**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**，只获取文件列表，避免遗漏。

根据技术栈搜索反序列化函数，追踪数据来源是否用户可控。也关注 LDAP referral、JNDI、远程类加载等触发反序列化的间接路径。

### 漏洞判断标准

**是漏洞**：反序列化数据来自用户可控输入（Cookie、请求体、base64 参数、恶意 LDAP/JNDI）。
**不是漏洞**：完全来自可信数据库、Java 设置了 ObjectInputFilter 白名单、Python 使用 SafeLoader。`,
	},

	{
		ID:   "auth_bypass",
		Name: "认证绕过/越权",
		SinkHints: []SinkHint{
			{
				Name:        "JWT 验证逻辑",
				Description: "JWT token 的解码/验证处。关注 alg=none 攻击、签名验证是否跳过、密钥是否硬编码。",
				Examples:    []string{"jwt.decode, jwt.verify, JWT.decode, HS256, RS256, alg.*none"},
			},
			{
				Name:        "权限/角色判断逻辑",
				Description: "代码中判断用户角色或权限的 if 条件。关注是否存在可绕过条件。",
				Examples:    []string{"if.*admin, if.*role, if.*permission, if.*isAuth, strcmp(, ==.*password, token.*=="},
			},
			{
				Name:        "按 ID 查询资源（IDOR）",
				Description: "通过 ID 直接查询资源而不验证归属的代码。关注 ID 是否来自用户输入且未附加 user_id 条件。",
				Examples:    []string{"findById, getById, SELECT.*WHERE.*id =, .find(id)"},
			},
			{
				Name:        "IAM / Session Policy / ACL 策略校验",
				Description: "服务账号、STS、session policy、DenyOnly、resource ACL 等授权逻辑。关注「仅检查 Deny」却未强制 Allow、空策略被当成无限制、伪造 clientId/subject 越权操作他人资源。",
				Examples:    []string{"DenyOnly, sessionPolicy, isAllowed, CheckPermission, Authorize, ACL, clientId, subject, Impersonat, AssumeRole"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "jwt_checked", Description: "已检查 JWT 验证逻辑"},
			{ID: "auth_logic_checked", Description: "已检查认证判断是否可绕过"},
			{ID: "resource_ownership_checked", Description: "已检查资源查询是否带 user_id 归属校验（IDOR）"},
			{ID: "iam_policy_checked", Description: "已检查 IAM/session policy/ACL 是否可提权或越权"},
		},
		Instruction: `## 当前任务：认证绕过/越权扫描

你只负责搜索 **认证绕过**、**越权访问（IDOR）** 以及 **授权/策略逻辑缺陷（提权）**。

### 搜索策略

**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**，只获取文件列表，避免遗漏。

根据技术栈搜索 JWT、权限检查、资源 ID 查询、IAM/session policy/ACL 相关代码。

### 认证绕过判断标准

JWT alg=none、签名验证跳过、密钥硬编码、strcmp 类型混淆（PHP）、== 弱比较密码。
PHP 经典模式：Cookie/Session 篡改（如 security=low）、dvwaPage 中 disable_authentication 配置、客户端 JS 修改权限字段后提交。
缺少鉴权过滤器/中间件的管理接口（默认 NoAuth）也属于本类。

### IDOR 判断标准

资源 ID 来自用户输入，数据库查询未附加 user_id 条件。

### 授权/策略提权判断标准

受限服务账号/STS 可通过「仅 Deny 检查」或空策略继承父权限；伪造身份字段（clientId/subject）操作他人订阅/资源；SCIM/provisioning 路径可提升角色。`,
	},

	{
		ID:   "xss_injection",
		Name: "XSS/模板注入",
		SinkHints: []SinkHint{
			{
				Name:        "直接输出用户数据的响应函数",
				Description: "将用户数据直接写入 HTTP 响应体的函数。若无 HTML 编码，触发 XSS。",
				Examples:    []string{"PHP: echo, print, <?=", "Java: response.getWriter().write, PrintWriter.print", "Node: res.send, innerHTML, document.write"},
			},
			{
				Name:        "服务端模板引擎（SSTI）",
				Description: "若用户输入被当作模板字符串（而非模板变量）传入引擎，可触发 RCE。",
				Examples:    []string{"Python: render_template_string(user_input), Template(user_input).render()", "Java: Velocity.evaluate, FreeMarker Template", "Go: template.HTML(user_input), template.JS(user_input)"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "output_sinks_found", Description: "已找到所有直接输出用户数据的位置"},
			{ID: "html_encoding_checked", Description: "已检查输出前是否有 HTML 编码"},
			{ID: "template_string_checked", Description: "已检查模板引擎是否将用户输入作为模板字符串"},
		},
		Instruction: `## 当前任务：XSS/模板注入扫描

你只负责搜索 **XSS** 和 **SSTI（服务端模板注入）** 漏洞。

**注意**：SpEL / OGNL / EL / MVEL 等**表达式语言注入**由独立类别 ` + "`expression_injection`" + ` 负责；本类专注 HTML/JS 输出与模板字符串注入。

### 搜索策略

**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**，只获取文件列表，避免遗漏。

根据技术栈选择响应输出函数和模板引擎关键词进行 grep，关键词应简短精准。

### XSS 判断标准

用户输入未经 htmlspecialchars/encodeURIComponent/h() 直接输出到 HTML 响应。

### SSTI 判断标准

用户输入被作为**模板本身**（而非模板变量）传入引擎（如 Template(user_input).render()），可导致 RCE。`,
	},

	{
		ID:   "code_execution",
		Name: "代码执行",
		SinkHints: []SinkHint{
			{
				Name:        "动态代码执行函数",
				Description: "将字符串作为代码执行的函数。若参数来自用户输入，可直接 RCE。",
				Examples:    []string{"PHP: eval, assert（PHP7以下）, call_user_func, call_user_func_array, create_function（PHP7以下）", "Python: eval, exec, compile, __import__", "JavaScript: eval, new Function, vm.runInThisContext", "Ruby: eval, instance_eval, class_eval"},
			},
			{
				Name:        "嵌入式脚本/沙箱执行入口",
				Description: "Lua/JS/WASM 等脚本引擎入口。关注是否允许危险库、超时与内存限制、可逃逸到宿主进程。",
				Examples:    []string{"luaL_load, lua_pcall, ScriptEngine.eval, QuickJS, wasmtime, otto.Run, goja.RunString"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "eval_sinks_found", Description: "已找到所有 eval/exec 类 Sink"},
			{ID: "input_source_checked", Description: "已追踪代码执行参数来源"},
			{ID: "php_version_noted", Description: "已确认 PHP 版本（PHP8 assert 不执行代码）"},
			{ID: "sandbox_escape_checked", Description: "已检查脚本沙箱限制与危险 API 暴露"},
		},
		Instruction: `## 当前任务：代码执行扫描

你只负责搜索 **任意代码执行** 与 **脚本沙箱逃逸** 类漏洞。

内存破坏导致的 RCE（UAF/OOB）归 ` + "`memory_safety`" + `；表达式语言注入归 ` + "`expression_injection`" + `。

### 搜索策略

**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**，只获取文件列表，避免遗漏。

根据技术栈选择 eval/exec/脚本引擎关键词进行 grep，关键词应简短精准，关注参数是否来自用户输入。

### 漏洞判断标准

**是漏洞**：用户可控字符串传入代码执行函数；或脚本引擎暴露危险宿主 API / 无资源限制。
**不是漏洞**：eval 内容完全硬编码、PHP8 中 assert() 不执行代码、严格白名单。

### 重点关注

PHP call_user_func($func, $arg)（$func 用户可控时可调用 system/exec）；PHP preg_replace /e 修饰符（PHP7+ 已移除）；Redis/中间件 Lua 等脚本入口。`,
	},

	{
		ID:   "expression_injection",
		Name: "表达式语言注入",
		SinkHints: []SinkHint{
			{
				Name:        "表达式语言求值入口",
				Description: "将字符串解析为表达式并求值。若属性名/表达式片段来自用户输入，可导致信息泄露或 RCE。",
				Examples:    []string{"SpelExpressionParser, parseExpression, SpelPropertyComparator, Ognl.getValue, ValueExpression, ELProcessor, MVEL.eval, JEXL"},
			},
			{
				Name:        "动态排序/过滤/查询表达式",
				Description: "Sort、Comparator、Filter、Criteria 等把字段名拼进表达式引擎的路径。",
				Examples:    []string{"Sort.by, OrderBy, PropertyComparator, CriteriaBuilder, filterExpression, orderBy"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "el_sinks_found", Description: "已找到 SpEL/OGNL/EL 等求值 Sink"},
			{ID: "user_controlled_expr_checked", Description: "已确认表达式/属性名是否用户可控"},
			{ID: "sandbox_or_simple_eval_checked", Description: "已检查是否限制为 SimpleEvaluationContext / 白名单属性"},
		},
		Instruction: `## 当前任务：表达式语言注入扫描

你只负责搜索 **SpEL / OGNL / EL / MVEL / JEXL** 等表达式注入（CWE-917），与 HTML XSS、Velocity/FreeMarker SSTI 区分开。

### 搜索策略

**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**，只获取文件列表，避免遗漏。

按技术栈搜索表达式解析器、parseExpression、Ognl、ELProcessor，以及把用户 Sort/filter 字段名拼进表达式的路径。

### 漏洞判断标准

**是漏洞**：用户可控属性名/表达式进入 parseExpression 或 StandardEvaluationContext 求值。
**不是漏洞**：仅 SimpleEvaluationContext + 固定属性白名单；表达式完全硬编码。`,
	},

	{
		ID:   "memory_safety",
		Name: "内存安全/缓冲区错误",
		SinkHints: []SinkHint{
			{
				Name:        "不安全的内存分配与拷贝",
				Description: "长度来自协议/用户输入却未校验，导致堆/栈溢出、OOB、整数溢出后的欠分配。",
				Examples:    []string{"C/C++: malloc, realloc, memcpy, memmove, strcpy, sprintf, gets", "Rust unsafe: copy_nonoverlapping, transmute", "Go: unsafe.Pointer, SliceHeader"},
			},
			{
				Name:        "释放后使用 / 双重释放路径",
				Description: "free/delete 后仍读写；或同一对象被释放两次。常见于协议反序列化、对象池、引用计数错误。",
				Examples:    []string{"free(, delete , RefCount, shared_ptr, Use-After-Free, double free, RESTORE, unserialize path in C"},
			},
			{
				Name:        "长度字段与边界校验",
				Description: "解析 length/size/count 字段时未与缓冲区实际大小交叉验证，或 size*nmemb 溢出。",
				Examples:    []string{"ntohl, be32toh, readUint32, remaining, checkBounds, SIZE_MAX, checked_mul"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "alloc_sites_found", Description: "已枚举关键分配/拷贝站点"},
			{ID: "length_validation_checked", Description: "已检查 length/size 是否与缓冲区交叉验证"},
			{ID: "uaf_double_free_checked", Description: "已检查 free 后引用与双重释放路径"},
		},
		Instruction: `## 当前任务：内存安全扫描

你只负责搜索 **缓冲区溢出、OOB、UAF、double-free、整数溢出导致欠分配** 等内存安全问题（常见于 C/C++/Rust unsafe/Go unsafe，以及自研二进制协议解析）。

Java/Go 托管代码中的「业务反序列化 Gadget」归 ` + "`deserialization`" + `；本类关注**内存破坏与错误生命周期**。

### 搜索策略

**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**，只获取文件列表，避免遗漏。

优先搜索：协议编解码、自定义二进制格式、对象池、引用计数、memcpy/realloc、length 字段解析。

### 漏洞判断标准

**是漏洞**：攻击者可控长度/偏移未校验即拷贝；释放后仍使用；同一指针 double free；size 运算溢出后分配过小。
**不是漏洞**：有严格边界检查、checked 算术、所有权清晰且不可外部触发。`,
	},

	{
		ID:   "resource_exhaustion",
		Name: "资源耗尽/解析器 DoS",
		SinkHints: []SinkHint{
			{
				Name:        "无界递归/嵌套解析",
				Description: "JSON/XML/Protobuf/Redis 数组等允许任意嵌套深度，导致栈溢出或对象图爆炸。",
				Examples:    []string{"maxDepth, nest, recursion, Decoder, Aggregator, nested array, EntityExpansion"},
			},
			{
				Name:        "无界缓冲/分配",
				Description: "按声明长度或读到分隔符为止无限分配，可打满堆/直接内存。",
				Examples:    []string{"ByteBuf, DirectMemory, make([]byte, n), ReadAll, ioutil.ReadAll, MaxFrameSize, content-length"},
			},
			{
				Name:        "压缩炸弹 / 扩展炸弹",
				Description: "zip/gzip/xml entity 扩展比例无上限。",
				Examples:    []string{"gzip.NewReader, zip.NewReader, ZipSlip, billion laughs, Ratio"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "nesting_limit_checked", Description: "已检查嵌套深度是否有上限"},
			{ID: "alloc_limit_checked", Description: "已检查单帧/单请求分配上限"},
			{ID: "compression_ratio_checked", Description: "已检查解压扩展比限制"},
		},
		Instruction: `## 当前任务：资源耗尽 / 解析器 DoS 扫描

你只负责搜索 **无界嵌套、无界分配、压缩/扩展炸弹** 导致的可用性破坏（OOM / 直接内存耗尽 / 栈溢出）。

### 搜索策略

**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**，只获取文件列表，避免遗漏。

重点搜编解码器、帧聚合器、ReadAll、按 length 分配、gzip/zip 解压路径。

### 漏洞判断标准

**是漏洞**：攻击者可控嵌套深度或声明长度，缺少硬上限或超限仍继续分配。
**不是漏洞**：有明确 maxDepth/maxFrameSize/解压比限制并在超限时拒绝。`,
	},

	{
		ID:   "header_injection",
		Name: "CRLF/HTTP 头注入",
		SinkHints: []SinkHint{
			{
				Name:        "HTTP 头/状态行写入",
				Description: "将用户输入写入 Header/状态行/Trailer。若含 \\r\\n 可分裂响应或注入头。",
				Examples:    []string{"setHeader, addHeader, Header().Set, WriteHeader, Location:, Set-Cookie, \\r\\n, %0d%0a"},
			},
			{
				Name:        "日志/协议行拼接",
				Description: "SMTP/Redis/HTTP 明文协议行拼接用户输入，可注入额外命令/头。",
				Examples:    []string{"fmt.Fprintf(conn, , writeln, CRLF, \\r\\n\"+"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "header_writes_found", Description: "已枚举写 Header/状态行的位置"},
			{ID: "crlf_sanitization_checked", Description: "已检查是否过滤 \\r\\n / 非法字符"},
		},
		Instruction: `## 当前任务：CRLF / HTTP 头注入扫描

你只负责搜索 **HTTP 响应分裂、Header 注入、协议行 CRLF 注入**。

### 搜索策略

**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**，只获取文件列表，避免遗漏。

搜索 setHeader/addHeader、Location/Set-Cookie 赋值，以及把用户输入拼进含 \\r\\n 的协议行。

### 漏洞判断标准

**是漏洞**：用户输入进入头字段或协议行，未剥离 CR/LF。
**不是漏洞**：框架自动编码/拒绝非法头名与头值。`,
	},

	{
		ID:   "race_condition",
		Name: "竞态条件/TOCTOU",
		SinkHints: []SinkHint{
			{
				Name:        "检查后使用（TOCTOU）",
				Description: "先检查权限/存在性再操作，中间无原子性，可被并发请求绕过。",
				Examples:    []string{"exists then create, checkPermission then act, os.Stat then Open, SELECT then UPDATE"},
			},
			{
				Name:        "共享状态无锁",
				Description: "全局 map/计数器/连接池在并发下读写无同步，导致安全属性失效。",
				Examples:    []string{"sync.Mutex missing, concurrent map, atomic, Race, double-submit"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "tocou_paths_found", Description: "已找到检查后使用的敏感路径"},
			{ID: "locking_or_tx_checked", Description: "已检查是否有锁/事务/唯一约束"},
		},
		Instruction: `## 当前任务：竞态条件 / TOCTOU 扫描

你只负责搜索 **权限/状态检查与实际操作之间的竞态**，以及影响安全结论的无锁共享状态。

### 搜索策略

**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**，只获取文件列表，避免遗漏。

关注「先查后改」、一次性 token 消费、配额扣减、文件 create 与权限检查分离的路径。

### 漏洞判断标准

**是漏洞**：两个并发请求都能通过检查并完成一次本应互斥的敏感操作。
**不是漏洞**：有事务/行锁/原子 compare-and-swap/唯一索引保证。`,
	},
}
