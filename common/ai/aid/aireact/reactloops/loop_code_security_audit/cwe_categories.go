package loop_code_security_audit

// CWECategoryLibrary 完整的 CWE 漏洞类别库。
// 覆盖 OWASP Top 10 2021 + 常见内存安全/逻辑漏洞类别。
// AI 在 Phase 1 探索项目后，根据技术栈裁剪不适用的类别。
var CWECategoryLibrary = []VulnCategory{
	// ═══════════════════════════════════════════════════════════════════
	// A03:2021 - Injection（注入类）
	// ═══════════════════════════════════════════════════════════════════

	{
		ID:     "cwe_89",
		Name:   "SQL注入(CWE-89)",
		Tag:    "injection",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "直接执行SQL字符串的函数",
				Description: "将SQL字符串直接发送给数据库执行的函数。重点关注参数是否通过字符串拼接构造。",
				Examples:    []string{"PHP: mysqli_query, ->query, ->execute", "Java: Statement.execute, createNativeQuery", "Python: cursor.execute", "Go: db.Exec, db.Query, db.Raw", "Node: knex.raw, sequelize.query"},
			},
			{
				Name:        "ORM Raw/原生查询方法",
				Description: "ORM框架中允许嵌入原始SQL片段的方法，容易将用户输入直接拼进SQL。",
				Examples:    []string{"Laravel: whereRaw, selectRaw, orderByRaw, DB::statement", "Django: .raw(), RawSQL", "GORM: .Where(\"...\"+input), .Raw()", "Hibernate: createNativeQuery"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "sources_enumerated", Description: "已枚举所有HTTP参数获取点"},
			{ID: "sinks_searched", Description: "已搜索所有SQL执行Sink"},
			{ID: "parameterized_checked", Description: "已确认命中Sink是否使用参数化查询"},
			{ID: "dataflow_traced", Description: "已追踪source→sink数据流"},
		},
		Instruction: `## 当前任务：SQL注入扫描(CWE-89)

你只负责搜索 **SQL注入** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**，只获取文件列表，避免遗漏。

根据技术栈自主决定grep关键词：
- SQL执行Sink：Exec、Query、Raw、Where、execute、prepare等
- 用户输入来源：$_GET/$_POST/getParameter/c.Query/request.form/req.query等

### 漏洞判断标准
**是漏洞**：用户可控输入直接拼接进SQL字符串，中间无参数化。
**不是漏洞**：使用占位符(?, :name)、ORM的find/where方法(非Raw)、intval()/(int)强转。

### data_flow格式
HTTP入口 → 处理函数[文件:行] → db.Query("... " + param)[文件:行]`,
	},

	{
		ID:     "cwe_78",
		Name:   "OS命令注入(CWE-78)",
		Tag:    "injection",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "系统命令执行函数",
				Description: "调用操作系统shell或外部进程的函数。若命令字符串包含用户输入则危险。",
				Examples:    []string{"PHP: system, exec, shell_exec, passthru, popen, proc_open", "Java: Runtime.exec, ProcessBuilder", "Python: os.system, subprocess.run/call/Popen, os.popen", "Go: exec.Command, exec.CommandContext", "Node: child_process.exec, execSync, spawn"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "sources_enumerated", Description: "已枚举所有用户输入来源"},
			{ID: "sinks_searched", Description: "已搜索所有命令执行Sink"},
			{ID: "string_concat_checked", Description: "已确认命令参数构造方式（拼接 vs 数组）"},
		},
		Instruction: `## 当前任务：OS命令注入扫描(CWE-78)

你只负责搜索 **命令注入** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

根据技术栈选择对应语言的命令执行函数关键词进行grep。

### 漏洞判断标准
**是漏洞**：用户可控输入通过字符串拼接构造命令（如 "bash -c " + userInput）。
**不是漏洞**：命令和参数完全硬编码、参数数组形式、严格白名单验证。

### 重点关注
bash -c 字符串形式、管道符、分号注入(a; whoami)、反引号注入。`,
	},

	{
		ID:     "cwe_79",
		Name:   "跨站脚本XSS(CWE-79)",
		Tag:    "injection",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "直接输出用户数据的响应函数",
				Description: "将用户数据直接写入HTTP响应体的函数。若无HTML编码，触发XSS。",
				Examples:    []string{"PHP: echo, print, <?=", "Java: response.getWriter().write, PrintWriter.print", "Node: res.send, innerHTML, document.write", "Python: flask.make_response, django.HttpResponse"},
			},
			{
				Name:        "前端模板渲染",
				Description: "前端框架中直接渲染用户输入的函数。",
				Examples:    []string{"React: dangerouslySetInnerHTML", "Vue: v-html", "Angular: [innerHTML], bypassSecurityTrustHtml", "jQuery: .html(), .append()"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "output_sinks_found", Description: "已找到所有直接输出用户数据的位置"},
			{ID: "html_encoding_checked", Description: "已检查输出前是否有HTML编码"},
		},
		Instruction: `## 当前任务：XSS扫描(CWE-79)

你只负责搜索 **跨站脚本(XSS)** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索响应输出函数和前端模板渲染关键词。

### 漏洞判断标准
**是漏洞**：用户输入未经htmlspecialchars/encodeURIComponent/h()直接输出到HTML响应。
**不是漏洞**：输出前经过HTML编码、使用安全的模板引擎默认转义。`,
	},

	{
		ID:     "cwe_94",
		Name:   "代码注入/代码执行(CWE-94)",
		Tag:    "injection",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "动态代码执行函数",
				Description: "将字符串作为代码执行的函数。若参数来自用户输入，可直接RCE。",
				Examples:    []string{"PHP: eval, assert(PHP7以下), call_user_func, create_function(PHP7以下)", "Python: eval, exec, compile, __import__", "JavaScript: eval, new Function, vm.runInThisContext", "Ruby: eval, instance_eval, class_eval"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "eval_sinks_found", Description: "已找到所有eval/exec类Sink"},
			{ID: "input_source_checked", Description: "已追踪代码执行参数来源"},
		},
		Instruction: `## 当前任务：代码注入扫描(CWE-94)

你只负责搜索 **任意代码执行** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索eval/exec类函数关键词。

### 漏洞判断标准
**是漏洞**：用户可控字符串传入代码执行函数。
**不是漏洞**：eval内容完全硬编码、PHP8中assert()不执行代码、严格白名单。`,
	},

	{
		ID:     "cwe_95",
		Name:   "表达式注入(CWE-95)",
		Tag:    "injection",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "表达式引擎求值函数",
				Description: "将字符串作为表达式求值的函数。若表达式来自用户输入，可导致RCE。",
				Examples:    []string{"Python: eval() with math expressions, numexpr", "Java: SpEL, OGNL, MVEL, JEXL", "Node: mathjs eval(), vm.runInContext()", "Go: govaluate, cel-go"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "expression_sinks_found", Description: "已找到所有表达式求值Sink"},
			{ID: "input_sanitization_checked", Description: "已检查输入是否有表达式语法限制"},
		},
		Instruction: `## 当前任务：表达式注入扫描(CWE-95)

你只负责搜索 **表达式注入** 漏洞（SSTI、SpEL注入、OGNL注入等）。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索表达式引擎相关函数和模板渲染函数。

### 漏洞判断标准
**是漏洞**：用户输入被作为表达式或模板字符串传入引擎。
**不是漏洞**：表达式完全硬编码、使用安全的沙箱环境。`,
	},

	{
		ID:     "cwe_918",
		Name:   "服务端请求伪造SSRF(CWE-918)",
		Tag:    "injection",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "发起HTTP请求的函数",
				Description: "若目标URL来自用户输入且无内网地址过滤，可触发SSRF访问内网服务。",
				Examples:    []string{"PHP: curl_exec, file_get_contents(http...), fsockopen", "Java: HttpURLConnection, new URL, OkHttpClient, RestTemplate", "Python: requests.get/post, urllib.request.urlopen", "Go: http.Get, http.Post, http.Do", "Node: http.request, axios, fetch"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "http_clients_found", Description: "已枚举所有HTTP请求发起位置"},
			{ID: "ssrf_filter_checked", Description: "已检查是否有内网IP过滤"},
		},
		Instruction: `## 当前任务：SSRF扫描(CWE-918)

你只负责搜索 **服务端请求伪造(SSRF)** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索HTTP客户端函数和URL构造逻辑。

### 漏洞判断标准
**是漏洞**：用户可控URL/IP传入HTTP请求函数，未限制访问内网地址(10.x, 192.168.x, 127.0.0.1)。
**不是漏洞**：URL白名单验证、禁止内网地址、使用代理网关隔离。`,
	},

	{
		ID:     "cwe_611",
		Name:   "XML外部实体注入XXE(CWE-611)",
		Tag:    "injection",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "XML解析器",
				Description: "若解析器未禁用外部实体(XXE)，解析用户提交的XML可读取任意文件或触发SSRF。",
				Examples:    []string{"Java: DocumentBuilderFactory, SAXParserFactory, XMLReader, XMLInputFactory", "PHP: simplexml_load_string, DOMDocument, xml_parse", "Python: etree.parse, minidom.parseString, ElementTree.parse"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "xml_parsers_found", Description: "已枚举所有XML解析器位置"},
			{ID: "external_entity_disabled", Description: "已检查是否禁用外部实体"},
		},
		Instruction: `## 当前任务：XXE扫描(CWE-611)

你只负责搜索 **XML外部实体注入(XXE)** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索XML解析器相关函数。

### 漏洞判断标准
**是漏洞**：XML解析器未禁用外部实体。
**不是漏洞**：设置了FEATURE_SECURE_PROCESSING、使用SafeLoader。`,
	},

	// ═══════════════════════════════════════════════════════════════════
	// A01:2021 - Broken Access Control（访问控制缺陷）
	// ═══════════════════════════════════════════════════════════════════

	{
		ID:     "cwe_22",
		Name:   "路径遍历(CWE-22)",
		Tag:    "access_control",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "文件读写/包含函数",
				Description: "直接操作文件系统的函数。若路径参数来自用户输入且无目录边界校验，可读取/写入任意文件。",
				Examples:    []string{"PHP: include, require, file_get_contents, file_put_contents, fopen, readfile, unlink", "Java: new File, FileInputStream, Files.readAllBytes, Paths.get", "Python: open, os.path.join, pathlib.Path", "Go: os.Open, os.ReadFile, ioutil.ReadFile", "Node: fs.readFile, fs.writeFile, path.join"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "sources_enumerated", Description: "已枚举文件路径相关的用户输入点"},
			{ID: "sinks_searched", Description: "已搜索所有文件操作Sink"},
			{ID: "path_sanitization_checked", Description: "已检查路径是否经过Clean/basename/realpath处理"},
			{ID: "directory_boundary_checked", Description: "已检查是否有目录边界校验"},
		},
		Instruction: `## 当前任务：路径遍历扫描(CWE-22)

你只负责搜索 **路径遍历** 和 **不安全文件操作** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

根据技术栈选择对应的文件操作函数关键词grep。

### 漏洞判断标准
**是漏洞**：用户可控输入构造文件路径，且无目录边界校验。
**不是漏洞**：路径限制在白名单目录内(Clean + HasPrefix)、basename()处理、路径完全由代码控制。

### 重点关注
../ 遍历、URL编码绕过 %2e%2e%2f、PHP include/require动态文件名（文件包含）。`,
	},

	{
		ID:     "cwe_287",
		Name:   "认证绕过(CWE-287)",
		Tag:    "access_control",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "JWT验证逻辑",
				Description: "JWT token的解码/验证处。关注alg=none攻击、签名验证是否跳过、密钥是否硬编码。",
				Examples:    []string{"jwt.decode, jwt.verify, JWT.decode, HS256, RS256, alg.*none"},
			},
			{
				Name:        "密码验证逻辑",
				Description: "认证函数中密码比较逻辑。关注是否存在类型混淆或弱比较。",
				Examples:    []string{"strcmp, password_verify, bcrypt.Compare, ==.*password, equal.*secret"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "jwt_checked", Description: "已检查JWT验证逻辑"},
			{ID: "auth_logic_checked", Description: "已检查认证判断是否可绕过"},
		},
		Instruction: `## 当前任务：认证绕过扫描(CWE-287)

你只负责搜索 **认证绕过** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索JWT相关库、认证逻辑函数、密码比较函数。

### 漏洞判断标准
**是漏洞**：JWT alg=none、签名验证跳过、密钥硬编码、strcmp类型混淆(PHP)、==弱比较密码。
**不是漏洞**：使用bcrypt/argon2哈希比较、JWT使用RS256且验证签名。`,
	},

	{
		ID:     "cwe_862",
		Name:   "授权缺失/越权(CWE-862)",
		Tag:    "access_control",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "权限/角色判断逻辑",
				Description: "代码中判断用户角色或权限的if条件。关注是否存在可绕过条件。",
				Examples:    []string{"if.*admin, if.*role, if.*permission, if.*isAuth, is_authorized, check_permission"},
			},
			{
				Name:        "按ID查询资源(IDOR)",
				Description: "通过ID直接查询资源而不验证归属的代码。关注ID是否来自用户输入且未附加user_id条件。",
				Examples:    []string{"findById, getById, SELECT.*WHERE.*id =, .find(id)"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "auth_logic_checked", Description: "已检查认证判断是否可绕过"},
			{ID: "resource_ownership_checked", Description: "已检查资源查询是否带user_id归属校验(IDOR)"},
		},
		Instruction: `## 当前任务：授权缺失/越权扫描(CWE-862)

你只负责搜索 **授权缺失** 和 **越权访问(IDOR)** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索权限检查关键词、资源ID查询模式。

### 漏洞判断标准
**是漏洞**：资源ID来自用户输入，数据库查询未附加user_id条件。
**不是漏洞**：查询带user_id条件、使用ORM的scope/where约束。`,
	},

	{
		ID:     "cwe_798",
		Name:   "硬编码凭证(CWE-798)",
		Tag:    "access_control",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "硬编码密钥/密码",
				Description: "代码中直接写入的密码、API密钥、数据库连接串等敏感信息。",
				Examples:    []string{"password = \"...\", apiKey = \"...\", secret = \"...\", CONNECTION_STRING, DSN, token = \"...\""},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "hardcoded_secrets_found", Description: "已搜索所有硬编码凭证"},
			{ID: "env_var_checked", Description: "已确认是否应改为环境变量"},
		},
		Instruction: `## 当前任务：硬编码凭证扫描(CWE-798)

你只负责搜索 **硬编码凭证** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索password、secret、apiKey、token、credentials、dsn、connection_string等关键词。

### 漏洞判断标准
**是漏洞**：密码、API密钥、数据库连接串直接写在代码中。
**不是漏洞**：从环境变量/配置文件/密钥管理服务读取、测试用mock数据。`,
	},

	// ═══════════════════════════════════════════════════════════════════
	// A08:2021 - Software and Data Integrity Failures（软件和数据完整性失败）
	// ═══════════════════════════════════════════════════════════════════

	{
		ID:     "cwe_502",
		Name:   "不安全反序列化(CWE-502)",
		Tag:    "integrity",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "反序列化函数",
				Description: "将外部数据还原为对象的函数。若数据来自用户可控输入，可触发任意代码执行(Gadget Chain)。",
				Examples:    []string{"PHP: unserialize", "Java: ObjectInputStream.readObject, XMLDecoder, XStream, Jackson enableDefaultTyping", "Python: pickle.loads, yaml.load(非SafeLoader), marshal.loads", "Node: node-serialize unserialize"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "deser_sinks_found", Description: "已找到所有反序列化Sink"},
			{ID: "data_source_checked", Description: "已追踪反序列化数据来源是否用户可控"},
		},
		Instruction: `## 当前任务：不安全反序列化扫描(CWE-502)

你只负责搜索 **不安全反序列化** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索反序列化函数，追踪数据来源是否用户可控。

### 漏洞判断标准
**是漏洞**：反序列化数据来自用户可控输入(Cookie、请求体、base64参数)。
**不是漏洞**：完全来自可信数据库、Java设置了ObjectInputFilter白名单、Python使用SafeLoader。`,
	},

	// ═══════════════════════════════════════════════════════════════════
	// A02:2021 - Cryptographic Failures（加密失败）
	// ═══════════════════════════════════════════════════════════════════

	{
		ID:     "cwe_327",
		Name:   "使用不安全加密算法(CWE-327)",
		Tag:    "crypto",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "弱加密算法",
				Description: "使用已知不安全的加密算法，如DES、RC4、MD5用于密码哈希等。",
				Examples:    []string{"DES, 3DES, RC4, Blowfish, MD5(密码), SHA1(密码), ECB模式"},
			},
			{
				Name:        "弱哈希算法",
				Description: "使用弱哈希算法存储密码或验证完整性。",
				Examples:    []string{"md5(), sha1(), MD5.Sum, SHA1.Sum, hashlib.md5, hashlib.sha1"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "weak_crypto_found", Description: "已找到所有弱加密算法使用"},
			{ID: "password_hashing_checked", Description: "已检查密码哈希是否使用bcrypt/argon2"},
		},
		Instruction: `## 当前任务：不安全加密算法扫描(CWE-327)

你只负责搜索 **不安全加密算法** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索DES、RC4、MD5、SHA1、ECB等弱加密关键词。

### 漏洞判断标准
**是漏洞**：密码使用MD5/SHA1哈希、使用DES/RC4加密、使用ECB模式。
**不是漏洞**：密码使用bcrypt/argon2/scrypt、加密使用AES-256-GCM、哈希用于非安全用途(如缓存key)。`,
	},

	{
		ID:     "cwe_321",
		Name:   "硬编码加密密钥(CWE-321)",
		Tag:    "crypto",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "硬编码密钥",
				Description: "加密/签名函数使用的密钥直接写在代码中。",
				Examples:    []string{"AES_KEY = \"...\", HMAC_SECRET = \"...\", jwt.sign(payload, \"hardcoded_secret\")"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "hardcoded_keys_found", Description: "已找到所有硬编码加密密钥"},
		},
		Instruction: `## 当前任务：硬编码加密密钥扫描(CWE-321)

你只负责搜索 **硬编码加密密钥** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索KEY、SECRET、HMAC、AES、RSA等关键词。

### 漏洞判断标准
**是漏洞**：加密密钥直接写在代码中。
**不是漏洞**：从环境变量/密钥管理服务读取、使用密钥派生函数。`,
	},

	// ═══════════════════════════════════════════════════════════════════
	// 内存安全类（C/C++/Go/Rust）
	// ═══════════════════════════════════════════════════════════════════

	{
		ID:     "cwe_120",
		Name:   "缓冲区溢出(CWE-120)",
		Tag:    "memory",
		LangRe: "c|cpp|go|rust",
		SinkHints: []SinkHint{
			{
				Name:        "不安全的字符串/内存操作",
				Description: "未检查边界的操作函数，可能导致缓冲区溢出。",
				Examples:    []string{"C: strcpy, strcat, sprintf, gets, memcpy(无长度检查)", "Go: copy(无边界检查), unsafe.Pointer操作"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "unsafe_ops_found", Description: "已找到所有不安全的字符串/内存操作"},
			{ID: "boundary_checked", Description: "已检查是否有边界校验"},
		},
		Instruction: `## 当前任务：缓冲区溢出扫描(CWE-120)

你只负责搜索 **缓冲区溢出** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索strcpy、strcat、sprintf、gets、memcpy等不安全函数。

### 漏洞判断标准
**是漏洞**：使用不安全函数且无边界检查。
**不是漏洞**：使用strncpy/snprintf/strlcpy、有明确的长度校验。`,
	},

	{
		ID:     "cwe_190",
		Name:   "整数溢出(CWE-190)",
		Tag:    "memory",
		LangRe: "c|cpp|go|rust",
		SinkHints: []SinkHint{
			{
				Name:        "整数运算",
				Description: "整数运算可能导致溢出，特别是在内存分配、数组索引计算时。",
				Examples:    []string{"malloc(n * size), int加减乘除, uint溢出"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "integer_ops_found", Description: "已找到所有可能导致溢出的整数运算"},
		},
		Instruction: `## 当前任务：整数溢出扫描(CWE-190)

你只负责搜索 **整数溢出** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索malloc、calloc、数组分配、整数运算等。

### 漏洞判断标准
**是漏洞**：整数运算结果用于内存分配或数组索引，无溢出检查。
**不是漏洞**：使用安全的整数运算库、有溢出检查。`,
	},

	{
		ID:     "cwe_416",
		Name:   "释放后重用UAF(CWE-416)",
		Tag:    "memory",
		LangRe: "c|cpp",
		SinkHints: []SinkHint{
			{
				Name:        "指针操作",
				Description: "释放内存后继续使用指针，可能导致代码执行。",
				Examples:    []string{"free()后继续使用指针, delete后继续使用对象"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "uaf_patterns_found", Description: "已找到所有可能的UAF模式"},
		},
		Instruction: `## 当前任务：UAF扫描(CWE-416)

你只负责搜索 **释放后重用(UAF)** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索free、delete、释放后使用指针的模式。

### 漏洞判断标准
**是漏洞**：free/delete后继续使用指针。
**不是漏洞**：释放后置NULL、使用智能指针。`,
	},

	{
		ID:     "cwe_476",
		Name:   "空指针解引用(CWE-476)",
		Tag:    "memory",
		LangRe: "c|cpp|go|java",
		SinkHints: []SinkHint{
			{
				Name:        "指针/引用解引用",
				Description: "未检查nil/null的指针/引用解引用，可能导致崩溃。",
				Examples:    []string{"C/C++: ptr->member, *ptr", "Go: ptr.Field, ptr.Method()", "Java: obj.method()"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "null_checks_found", Description: "已找到所有可能的空指针解引用"},
		},
		Instruction: `## 当前任务：空指针解引用扫描(CWE-476)

你只负责搜索 **空指针解引用** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索可能返回nil/null的函数调用后的解引用操作。

### 漏洞判断标准
**是漏洞**：未检查nil/null直接解引用。
**不是漏洞**：有nil检查、使用Optional/Result类型。`,
	},

	// ═══════════════════════════════════════════════════════════════════
	// A04:2021 - Insecure Design（不安全设计）
	// ═══════════════════════════════════════════════════════════════════

	{
		ID:     "cwe_306",
		Name:   "关键功能缺少认证(CWE-306)",
		Tag:    "design",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "敏感操作路由",
				Description: "管理接口、敏感操作缺少认证检查。",
				Examples:    []string{"/admin, /api/internal, /debug, /metrics, /health(含敏感信息)"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "unprotected_routes_found", Description: "已找到所有缺少认证的敏感路由"},
		},
		Instruction: `## 当前任务：关键功能缺少认证扫描(CWE-306)

你只负责搜索 **关键功能缺少认证** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索admin、internal、debug、metrics等敏感路由，检查是否有认证中间件。

### 漏洞判断标准
**是漏洞**：敏感路由无认证中间件保护。
**不是漏洞**：有认证中间件、使用IP白名单。`,
	},

	{
		ID:     "cwe_307",
		Name:   "认证机制过度宽松(CWE-307)",
		Tag:    "design",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "登录/认证逻辑",
				Description: "登录接口无速率限制、无账号锁定机制，可被暴力破解。",
				Examples:    []string{"login, signin, authenticate, /api/login"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "rate_limiting_checked", Description: "已检查登录接口是否有速率限制"},
			{ID: "account_lockout_checked", Description: "已检查是否有账号锁定机制"},
		},
		Instruction: `## 当前任务：认证机制过度宽松扫描(CWE-307)

你只负责搜索 **认证机制过度宽松** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索登录接口，检查是否有速率限制、账号锁定、验证码等防护。

### 漏洞判断标准
**是漏洞**：登录接口无速率限制、无账号锁定。
**不是漏洞**：有rate limiter、有账号锁定、有验证码。`,
	},

	// ═══════════════════════════════════════════════════════════════════
	// A05:2021 - Security Misconfiguration（安全配置错误）
	// ═══════════════════════════════════════════════════════════════════

	{
		ID:     "cwe_16",
		Name:   "安全配置错误(CWE-16)",
		Tag:    "config",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "不安全的默认配置",
				Description: "使用不安全的默认配置，如debug模式开启、CORS全放开等。",
				Examples:    []string{"DEBUG=true, CORS: *, Access-Control-Allow-Origin: *, X-Powered-By暴露"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "debug_mode_checked", Description: "已检查是否开启debug模式"},
			{ID: "cors_checked", Description: "已检查CORS配置"},
		},
		Instruction: `## 当前任务：安全配置错误扫描(CWE-16)

你只负责搜索 **安全配置错误** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索DEBUG、CORS、Access-Control-Allow-Origin等配置。

### 漏洞判断标准
**是漏洞**：生产环境开启DEBUG、CORS全放开、暴露服务器版本信息。
**不是漏洞**：CORS限制特定域名、生产环境关闭DEBUG。`,
	},

	// ═══════════════════════════════════════════════════════════════════
	// A06:2021 - Vulnerable and Outdated Components（易受攻击和过时的组件）
	// ═══════════════════════════════════════════════════════════════════

	{
		ID:     "cwe_1104",
		Name:   "使用易受攻击的第三方组件(CWE-1104)",
		Tag:    "dependency",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "依赖配置文件",
				Description: "项目依赖的第三方库版本，可能包含已知漏洞。",
				Examples:    []string{"package.json, requirements.txt, go.mod, pom.xml, Gemfile, Cargo.toml"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "deps_scanned", Description: "已扫描依赖文件"},
		},
		Instruction: `## 当前任务：第三方组件漏洞扫描(CWE-1104)

你只负责搜索 **使用易受攻击的第三方组件** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

读取package.json、requirements.txt、go.mod、pom.xml等依赖文件。

### 漏洞判断标准
**是漏洞**：依赖了已知有漏洞的版本。
**不是漏洞**：依赖版本已更新、有安全补丁。`,
	},

	// ═══════════════════════════════════════════════════════════════════
	// A07:2021 - Identification and Authentication Failures（身份识别和认证失败）
	// ═══════════════════════════════════════════════════════════════════

	{
		ID:     "cwe_384",
		Name:   "会话固定(CWE-384)",
		Tag:    "auth",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "会话管理",
				Description: "登录后未重新生成session ID，可被会话固定攻击。",
				Examples:    []string{"session.id不变, cookie未更新, JWT未刷新"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "session_fixation_checked", Description: "已检查登录后是否重新生成session"},
		},
		Instruction: `## 当前任务：会话固定扫描(CWE-384)

你只负责搜索 **会话固定** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索session管理、登录逻辑。

### 漏洞判断标准
**是漏洞**：登录后未重新生成session ID。
**不是漏洞**：登录后regenerate session、使用一次性token。`,
	},

	{
		ID:     "cwe_613",
		Name:   "会话过期不足(CWE-613)",
		Tag:    "auth",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "会话超时配置",
				Description: "会话/token超时时间过长或无超时，增加会话劫持风险。",
				Examples:    []string{"session timeout, token expiry, maxAge, expires_in"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "session_timeout_checked", Description: "已检查会话超时配置"},
		},
		Instruction: `## 当前任务：会话过期不足扫描(CWE-613)

你只负责搜索 **会话过期不足** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索session timeout、token expiry等配置。

### 漏洞判断标准
**是漏洞**：会话/token超时时间过长(>24h)或无超时。
**不是漏洞**：合理的超时时间、有refresh token机制。`,
	},

	// ═══════════════════════════════════════════════════════════════════
	// A09:2021 - Security Logging and Monitoring Failures（安全日志和监控失败）
	// ═══════════════════════════════════════════════════════════════════

	{
		ID:     "cwe_778",
		Name:   "日志记录不足(CWE-778)",
		Tag:    "logging",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "安全事件日志",
				Description: "敏感操作（登录失败、权限拒绝等）未记录日志。",
				Examples:    []string{"login failed, access denied, authentication error未记录"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "security_logging_checked", Description: "已检查安全事件是否记录日志"},
		},
		Instruction: `## 当前任务：日志记录不足扫描(CWE-778)

你只负责搜索 **日志记录不足** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索认证失败、权限拒绝等安全事件的处理逻辑。

### 漏洞判断标准
**是漏洞**：安全事件（登录失败、权限拒绝）未记录日志。
**不是漏洞**：有完整的安全审计日志。`,
	},

	// ═══════════════════════════════════════════════════════════════════
	// 其他常见漏洞
	// ═══════════════════════════════════════════════════════════════════

	{
		ID:     "cwe_352",
		Name:   "跨站请求伪造CSRF(CWE-352)",
		Tag:    "web",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "状态变更操作",
				Description: "修改数据的HTTP接口缺少CSRF防护。",
				Examples:    []string{"POST/PUT/DELETE接口, 表单提交, API调用"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "csrf_token_checked", Description: "已检查是否有CSRF token"},
			{ID: "same_site_checked", Description: "已检查Cookie是否有SameSite属性"},
		},
		Instruction: `## 当前任务：CSRF扫描(CWE-352)

你只负责搜索 **跨站请求伪造(CSRF)** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索POST/PUT/DELETE接口，检查是否有CSRF防护。

### 漏洞判断标准
**是漏洞**：状态变更接口无CSRF token、Cookie无SameSite属性。
**不是漏洞**：有CSRF token、使用SameSite=Strict/Lax、使用JSON API(无Cookie认证)。`,
	},

	{
		ID:     "cwe_434",
		Name:   "无限制文件上传(CWE-434)",
		Tag:    "web",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "文件上传处理",
				Description: "文件上传接口未限制文件类型、大小，可上传恶意文件。",
				Examples:    []string{"multipart upload, file upload, form-data, multer, express-fileupload"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "file_type_checked", Description: "已检查是否限制文件类型"},
			{ID: "file_size_checked", Description: "已检查是否限制文件大小"},
		},
		Instruction: `## 当前任务：无限制文件上传扫描(CWE-434)

你只负责搜索 **无限制文件上传** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索文件上传处理逻辑。

### 漏洞判断标准
**是漏洞**：未限制文件类型、未检查文件内容、未限制文件大小。
**不是漏洞**：白名单限制类型、检查magic bytes、限制大小。`,
	},

	{
		ID:     "cwe_915",
		Name:   "批量赋值(CWE-915)",
		Tag:    "web",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "对象属性批量赋值",
				Description: "将用户输入直接绑定到对象属性，可能修改不应被用户控制的字段。",
				Examples:    []string{"Go: c.ShouldBindJSON, json.Unmarshal到struct", "Python: **kwargs, setattr", "Java: @ModelAttribute, BeanUtils.copyProperties", "Node: Object.assign, req.body直接使用"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "mass_assignment_checked", Description: "已检查是否有批量赋值保护"},
		},
		Instruction: `## 当前任务：批量赋值扫描(CWE-915)

你只负责搜索 **批量赋值** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索对象绑定、反序列化到业务对象的逻辑。

### 漏洞判断标准
**是漏洞**：用户输入直接绑定到对象所有属性（如role、is_admin等敏感字段可被篡改）。
**不是漏洞**：使用白名单绑定、使用DTO/VO隔离。`,
	},

	{
		ID:     "cwe_532",
		Name:   "敏感信息写入日志(CWE-532)",
		Tag:    "logging",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "日志输出",
				Description: "日志中包含密码、token、敏感数据。",
				Examples:    []string{"log.info(password), log.debug(token), console.log(secret), logger.error(request)"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "sensitive_logging_checked", Description: "已检查日志中是否包含敏感数据"},
		},
		Instruction: `## 当前任务：敏感信息写入日志扫描(CWE-532)

你只负责搜索 **敏感信息写入日志** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索日志输出函数，检查是否包含敏感数据。

### 漏洞判断标准
**是漏洞**：日志中包含密码、token、密钥、身份证号等敏感信息。
**不是漏洞**：日志中敏感数据已脱敏、只记录操作结果不记录敏感输入。`,
	},

	{
		ID:     "cwe_601",
		Name:   "URL重定向(CWE-601)",
		Tag:    "web",
		LangRe: "any",
		SinkHints: []SinkHint{
			{
				Name:        "重定向函数",
				Description: "URL重定向参数来自用户输入，可构造钓鱼链接。",
				Examples:    []string{"redirect, Location header, window.location, res.redirect, 302响应"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "redirect_target_checked", Description: "已检查重定向目标是否白名单校验"},
		},
		Instruction: `## 当前任务：URL重定向扫描(CWE-601)

你只负责搜索 **URL重定向** 漏洞。

### 搜索策略
**务必使用 output-mode="files_with_matches" 模式进行阶段A搜索**。

搜索redirect、Location header等重定向逻辑。

### 漏洞判断标准
**是漏洞**：重定向目标来自用户输入且无白名单校验。
**不是漏洞**：白名单校验重定向目标、只允许相对路径重定向。`,
	},
}

// GetDefaultCategories 返回默认的扫描类别（完整CWE库）。
func GetDefaultCategories() []VulnCategory {
	return CWECategoryLibrary
}

// GetCategoriesByTag 按标签过滤类别。
func GetCategoriesByTag(tag string) []VulnCategory {
	var result []VulnCategory
	for _, c := range CWECategoryLibrary {
		if c.Tag == tag {
			result = append(result, c)
		}
	}
	return result
}

// GetCategoriesByLang 按语言过滤类别（LangRe为空或匹配的类别）。
func GetCategoriesByLang(lang string) []VulnCategory {
	var result []VulnCategory
	for _, c := range CWECategoryLibrary {
		if c.LangRe == "" || c.LangRe == "any" {
			result = append(result, c)
			continue
		}
		// 简单匹配：检查lang是否包含在LangRe中
		if containsLang(c.LangRe, lang) {
			result = append(result, c)
		}
	}
	return result
}

func containsLang(langRe, lang string) bool {
	// 简单实现：检查lang是否在langRe中
	// 实际应该用正则匹配
	switch langRe {
	case "c|cpp|go|rust":
		return lang == "c" || lang == "cpp" || lang == "go" || lang == "rust" || lang == "golang"
	case "c|cpp":
		return lang == "c" || lang == "cpp"
	default:
		return true
	}
}
