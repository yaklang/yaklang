package loop_code_security_audit

import "fmt"

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
	// Tag 分类标签（如 injection, access_control, memory 等）
	Tag string
	// LangRe 适用的语言（空或"any"表示所有语言）
	// 可选值: "any", "c|cpp|go|rust", "c|cpp", "any"
	LangRe string
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
	var sb fmt.Stringer
	_ = sb
	result := ""
	for _, h := range c.SinkHints {
		result += fmt.Sprintf("- **%s**：%s\n  典型示例（参考，非固定关键词）：%v\n", h.Name, h.Description, h.Examples)
	}
	return result
}

// DefaultVulnCategories 默认的漏洞扫描类别。
// SinkHints 只描述"这类 Sink 长什么样"，不硬编码 grep 关键词。
// AI 在阶段A根据已知技术栈自主选择实际搜索词。
// 使用精选的 8 个核心类别，覆盖最常见的漏洞类型。
var DefaultVulnCategories = []VulnCategory{
	{
		ID:   "sql_injection",
		Name: "SQL注入(CWE-89)",
		Tag:  "injection",
		SinkHints: []SinkHint{
			{
				Name:        "直接执行SQL字符串的函数",
				Description: "将SQL字符串直接发送给数据库执行的函数。重点关注参数是否通过字符串拼接构造。",
				Examples:    []string{"PHP: mysqli_query, ->query, ->execute", "Java: Statement.execute, createNativeQuery", "Python: cursor.execute", "Go: db.Exec, db.Query, db.Raw", "Node: knex.raw, sequelize.query"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "sinks_searched", Description: "已搜索所有SQL执行Sink"},
			{ID: "parameterized_checked", Description: "已确认命中Sink是否使用参数化查询"},
		},
		Instruction: `## 当前任务：SQL注入扫描(CWE-89)
你只负责搜索 **SQL注入** 漏洞。使用 output-mode="files_with_matches" 搜索SQL执行Sink。
**是漏洞**：用户可控输入直接拼接进SQL字符串。**不是漏洞**：使用占位符(?, :name)。`,
	},
	{
		ID:   "cmd_injection",
		Name: "命令注入(CWE-78)",
		Tag:  "injection",
		SinkHints: []SinkHint{
			{
				Name:        "系统命令执行函数",
				Description: "调用操作系统shell或外部进程的函数。若命令字符串包含用户输入则危险。",
				Examples:    []string{"PHP: system, exec, shell_exec", "Java: Runtime.exec, ProcessBuilder", "Python: os.system, subprocess.run", "Go: exec.Command", "Node: child_process.exec"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "sinks_searched", Description: "已搜索所有命令执行Sink"},
		},
		Instruction: `## 当前任务：命令注入扫描(CWE-78)
你只负责搜索 **命令注入** 漏洞。使用 output-mode="files_with_matches" 搜索命令执行函数。
**是漏洞**：用户可控输入通过字符串拼接构造命令。**不是漏洞**：参数数组形式。`,
	},
	{
		ID:   "path_traversal",
		Name: "路径遍历(CWE-22)",
		Tag:  "access_control",
		SinkHints: []SinkHint{
			{
				Name:        "文件读写函数",
				Description: "直接操作文件系统的函数。若路径参数来自用户输入且无目录边界校验，可读取/写入任意文件。",
				Examples:    []string{"PHP: include, file_get_contents", "Java: new File, Files.readAllBytes", "Python: open, os.path.join", "Go: os.Open, os.ReadFile", "Node: fs.readFile"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "sinks_searched", Description: "已搜索所有文件操作Sink"},
		},
		Instruction: `## 当前任务：路径遍历扫描(CWE-22)
你只负责搜索 **路径遍历** 漏洞。使用 output-mode="files_with_matches" 搜索文件操作函数。
**是漏洞**：用户可控输入构造文件路径，且无目录边界校验。**不是漏洞**：路径限制在白名单目录内。`,
	},
	{
		ID:   "xss_injection",
		Name: "XSS(CWE-79)",
		Tag:  "injection",
		SinkHints: []SinkHint{
			{
				Name:        "响应输出函数",
				Description: "将用户数据直接写入HTTP响应体的函数。若无HTML编码，触发XSS。",
				Examples:    []string{"PHP: echo, print", "Java: response.getWriter().write", "Node: res.send, innerHTML"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "sinks_searched", Description: "已搜索所有输出函数"},
		},
		Instruction: `## 当前任务：XSS扫描(CWE-79)
你只负责搜索 **XSS** 漏洞。使用 output-mode="files_with_matches" 搜索输出函数。
**是漏洞**：用户输入未经HTML编码直接输出。**不是漏洞**：输出前经过编码。`,
	},
	{
		ID:   "code_execution",
		Name: "代码执行(CWE-94)",
		Tag:  "injection",
		SinkHints: []SinkHint{
			{
				Name:        "动态代码执行函数",
				Description: "将字符串作为代码执行的函数。若参数来自用户输入，可直接RCE。",
				Examples:    []string{"PHP: eval, assert", "Python: eval, exec", "JavaScript: eval, new Function", "Ruby: eval, instance_eval"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "sinks_searched", Description: "已搜索所有eval/exec类Sink"},
		},
		Instruction: `## 当前任务：代码执行扫描(CWE-94)
你只负责搜索 **代码执行** 漏洞。使用 output-mode="files_with_matches" 搜索eval/exec函数。
**是漏洞**：用户可控字符串传入代码执行函数。**不是漏洞**：eval内容完全硬编码。`,
	},
	{
		ID:   "auth_bypass",
		Name: "认证绕过(CWE-287)",
		Tag:  "access_control",
		SinkHints: []SinkHint{
			{
				Name:        "认证逻辑",
				Description: "认证函数中的密码比较逻辑。关注是否存在类型混淆或弱比较。",
				Examples:    []string{"jwt.decode, jwt.verify, password_verify, bcrypt.Compare"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "auth_logic_checked", Description: "已检查认证判断是否可绕过"},
		},
		Instruction: `## 当前任务：认证绕过扫描(CWE-287)
你只负责搜索 **认证绕过** 漏洞。使用 output-mode="files_with_matches" 搜索认证逻辑。
**是漏洞**：JWT alg=none、签名验证跳过、密钥硬编码。**不是漏洞**：使用bcrypt比较、JWT验证签名。`,
	},
	{
		ID:   "deserialization",
		Name: "反序列化(CWE-502)",
		Tag:  "integrity",
		SinkHints: []SinkHint{
			{
				Name:        "反序列化函数",
				Description: "将外部数据还原为对象的函数。若数据来自用户可控输入，可触发任意代码执行。",
				Examples:    []string{"PHP: unserialize", "Java: ObjectInputStream.readObject", "Python: pickle.loads, yaml.load"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "sinks_searched", Description: "已搜索所有反序列化Sink"},
		},
		Instruction: `## 当前任务：反序列化扫描(CWE-502)
你只负责搜索 **反序列化** 漏洞。使用 output-mode="files_with_matches" 搜索反序列化函数。
**是漏洞**：反序列化数据来自用户可控输入。**不是漏洞**：数据来自可信数据库。`,
	},
	{
		ID:   "ssrf",
		Name: "SSRF(CWE-918)",
		Tag:  "injection",
		SinkHints: []SinkHint{
			{
				Name:        "HTTP请求函数",
				Description: "发起HTTP请求的函数。若目标URL来自用户输入且无内网地址过滤，可触发SSRF。",
				Examples:    []string{"PHP: curl_exec, file_get_contents", "Java: HttpURLConnection, OkHttpClient", "Python: requests.get, urllib.request.urlopen", "Go: http.Get, http.Do"},
			},
		},
		Checklist: []ChecklistItem{
			{ID: "sinks_searched", Description: "已搜索所有HTTP请求函数"},
		},
		Instruction: `## 当前任务：SSRF扫描(CWE-918)
你只负责搜索 **SSRF** 漏洞。使用 output-mode="files_with_matches" 搜索HTTP请求函数。
**是漏洞**：用户可控URL/IP传入HTTP请求函数，未限制内网地址。**不是漏洞**：URL白名单验证。`,
	},
}
