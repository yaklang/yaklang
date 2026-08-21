package model

import (
	"fmt"
	"sort"
	"strings"
)

// LanguageProfile 按语言族划分的审计画像。
// 不同语言的高价值洞面不同：原生内存语言侧重内存/解析器；托管语言侧重网络入口与鉴权/注入。
type LanguageProfile string

const (
	ProfileMemoryNative   LanguageProfile = "memory_native"   // C/C++/Zig/部分 Rust unsafe
	ProfileManagedNetwork LanguageProfile = "managed_network" // Java/PHP/Python/Node/C#/Ruby…
	ProfileSystemsGo      LanguageProfile = "systems_go"      // Go：网络/并发为主，unsafe 次之
	ProfileMixed          LanguageProfile = "mixed"           // 多语言 monorepo
	ProfileUnknown        LanguageProfile = "unknown"
)

// DetectLanguageProfile 根据 Phase1 TechStack 文本推断审计画像。
func DetectLanguageProfile(techStack string) LanguageProfile {
	s := strings.ToLower(techStack)
	if strings.TrimSpace(s) == "" {
		return ProfileUnknown
	}

	has := func(keys ...string) bool {
		for _, k := range keys {
			if strings.Contains(s, k) {
				return true
			}
		}
		return false
	}

	mem := has("c/c++", "c++", "cplusplus", " zig", "zig,", "assembly", "asm", ".c ", " c ", "cmake", "makefile") ||
		(has(" rust", "rust,", "rustc") && has("unsafe", "ffi", "no_std", "embedded")) ||
		has("nuttx", "freebsd kernel", "linux kernel")
	// Pure C/C++ signals without being buried in "Objective-C" false positive too often
	if has("objective-c") {
		mem = false
	}
	if has(", c,", " c,", "c language", "pure c", "ansi c", "c99", "c11", "c23") ||
		has("c/c++", "cpp", "cxx") {
		mem = true
	}

	managed := has("java", "kotlin", "scala", "php", "python", "typescript", "javascript", "nodejs", "node.js",
		"csharp", "c#", ".net", "ruby", "laravel", "spring", "django", "flask", "express", "nestjs", "rails")
	goLang := has("golang", " go,", "go ", "go-", "/go", "go module", "gomod") ||
		strings.Contains(s, "go)") || strings.HasPrefix(s, "go") || strings.Contains(s, " go/")

	count := 0
	if mem {
		count++
	}
	if managed {
		count++
	}
	if goLang {
		count++
	}
	if count >= 2 {
		return ProfileMixed
	}
	if mem {
		return ProfileMemoryNative
	}
	if goLang {
		return ProfileSystemsGo
	}
	if managed {
		return ProfileManagedNetwork
	}
	// Heuristic: extension-ish stacks
	if has(".c", ".cpp", ".h", ".hpp", "redis", "nginx", "clickhouse") && !managed {
		return ProfileMemoryNative
	}
	return ProfileUnknown
}

// FocusPlan 描述某技术栈下类别的主次顺序与提示文案。
type FocusPlan struct {
	Profile   LanguageProfile
	Primary   []string // 必须深挖
	Secondary []string // 覆盖但可相对快扫
	Tertiary  []string // 低相关，可浅扫或仅在有明确信号时投入
	Guidance  string
}

// BuildFocusPlan 按技术栈生成类别优先级。未列出的默认类别归入 Secondary。
func BuildFocusPlan(techStack string) FocusPlan {
	profile := DetectLanguageProfile(techStack)
	plan := FocusPlan{Profile: profile}

	switch profile {
	case ProfileMemoryNative:
		plan.Primary = []string{"memory_safety", "resource_exhaustion", "race_condition", "path_traversal", "cmd_injection"}
		plan.Secondary = []string{"auth_bypass", "xxe_ssrf", "header_injection", "code_execution"}
		plan.Tertiary = []string{"sql_injection", "xss_injection", "deserialization", "expression_injection"}
		plan.Guidance = `【语言画像：原生内存（C/C++/类 C）】
审计重点应放在**内存与解析器正确性**，而不是 Web 注入清单打卡：
- 主攻：缓冲区/UAF/整数溢出欠分配、无界嵌套与分配 DoS、TOCTOU、路径/命令拼接
- 次要：若存在网络协议/管理接口，再看鉴权、SSRF、CRLF
- 低优先：经典 SQLi/XSS/SpEL（除非项目内嵌 HTTP 管理面或脚本引擎）
阶段A/B 把时间主要花在编解码、分配、生命周期与并发共享状态上。`

	case ProfileSystemsGo:
		plan.Primary = []string{"auth_bypass", "xxe_ssrf", "path_traversal", "cmd_injection", "race_condition", "resource_exhaustion", "header_injection"}
		plan.Secondary = []string{"sql_injection", "deserialization", "code_execution", "memory_safety"}
		plan.Tertiary = []string{"xss_injection", "expression_injection"}
		plan.Guidance = `【语言画像：Go】
审计重点以**网络入口、鉴权与并发**为主：
- 主攻：鉴权/越权、SSRF（含 http.Get 绕过加固 Dial）、路径穿越、命令注入、竞态、帧/缓冲无界分配、CRLF
- 次要：SQL、反序列化、脚本/插件执行；仅在出现 unsafe/cgo 时升级 memory_safety
- 低优先：浏览器 XSS、Java SpEL（Go 项目通常不涉及）
关注 net/http、反向代理、迁移/webhook、权限中间件与 sync 临界区。`

	case ProfileManagedNetwork:
		plan.Primary = []string{"sql_injection", "auth_bypass", "xxe_ssrf", "deserialization", "expression_injection", "xss_injection", "cmd_injection", "path_traversal", "code_execution"}
		plan.Secondary = []string{"header_injection", "race_condition", "resource_exhaustion"}
		plan.Tertiary = []string{"memory_safety"}
		plan.Guidance = `【语言画像：托管语言网络应用（Java/PHP/Python/Node/C#…）】
审计重点以**网络层入口 → 鉴权/注入/反序列化**为主：
- 主攻：SQLi、鉴权越权/IAM、SSRF/XXE、反序列化、表达式注入（SpEL/OGNL/EL）、XSS/SSTI、命令/路径、动态代码执行
- 次要：CRLF、竞态、解析器 DoS
- 低优先：手工内存破坏（除非 JNI/native 扩展）；不要把时间耗在 C 式 malloc 审查上
按 HTTP/RPC 入口与框架中间件做 source→sink，而不是全仓库内存语义搜索。`

	case ProfileMixed:
		plan.Primary = []string{"auth_bypass", "xxe_ssrf", "memory_safety", "resource_exhaustion", "sql_injection", "deserialization", "path_traversal", "cmd_injection"}
		plan.Secondary = []string{"expression_injection", "xss_injection", "race_condition", "header_injection", "code_execution"}
		plan.Guidance = `【语言画像：多语言混合仓库】
分别对待子系统：native/核心库按内存与解析器审；HTTP/API/管理面按网络注入与鉴权审。
不要用单一 Web 清单或单一内存清单覆盖整个 monorepo。`

	default:
		plan.Primary = nil // 保持默认全量顺序
		plan.Guidance = `【语言画像：未识别】
无法从技术栈可靠判断语言族时：默认类别全选；同时兼顾网络注入与内存/解析器两类信号，避免只扫 Web 或只扫内存。`
	}
	return plan
}

// OrderCategoriesByFocus 按语言画像重排类别：Primary → Secondary → Tertiary → 其余。
// 不删除类别（侧重≠省略）；规划阶段仍可按用户意图裁剪。
func OrderCategoriesByFocus(techStack string, cats []VulnCategory) []VulnCategory {
	if len(cats) == 0 {
		return cats
	}
	plan := BuildFocusPlan(techStack)
	if plan.Profile == ProfileUnknown && len(plan.Primary) == 0 {
		return cats
	}

	rank := map[string]int{}
	setRank := func(ids []string, base int) {
		for i, id := range ids {
			if _, ok := rank[id]; !ok {
				rank[id] = base + i
			}
		}
	}
	setRank(plan.Primary, 0)
	setRank(plan.Secondary, 100)
	setRank(plan.Tertiary, 200)

	out := append([]VulnCategory(nil), cats...)
	// stable sort by rank; unknown ids sit in the middle band (150)
	sort.SliceStable(out, func(i, j int) bool {
		ri, okI := rank[out[i].ID]
		rj, okJ := rank[out[j].ID]
		if !okI {
			ri = 150
		}
		if !okJ {
			rj = 150
		}
		return ri < rj
	})
	return out
}

// CategoryFocusTier 返回某类别在当前技术栈下的层级文案。
func CategoryFocusTier(techStack, categoryID string) string {
	plan := BuildFocusPlan(techStack)
	in := func(ids []string) bool {
		for _, id := range ids {
			if id == categoryID {
				return true
			}
		}
		return false
	}
	switch {
	case in(plan.Primary):
		return "primary"
	case in(plan.Tertiary):
		return "tertiary"
	case in(plan.Secondary):
		return "secondary"
	default:
		return "secondary"
	}
}

// LanguageFocusForCategory 注入到单类扫描 ReactiveData：告诉 AI 在本语言下如何审当前类。
func LanguageFocusForCategory(techStack, categoryID string) string {
	plan := BuildFocusPlan(techStack)
	tier := CategoryFocusTier(techStack, categoryID)
	var b strings.Builder
	b.WriteString(plan.Guidance)
	b.WriteString("\n\n")
	switch tier {
	case "primary":
		b.WriteString(fmt.Sprintf("当前类别 `%s` 对本技术栈为**主攻面**：阶段A 多轮搜索，阶段B 深挖数据流/边界条件，宁可多报。\n", categoryID))
	case "tertiary":
		b.WriteString(fmt.Sprintf("当前类别 `%s` 对本技术栈为**低优先面**：仅在出现明确 Sink/框架信号时投入；无信号可快速 complete，勿硬搜无关模式。\n", categoryID))
	default:
		b.WriteString(fmt.Sprintf("当前类别 `%s` 对本技术栈为**次要面**：标准覆盖即可，发现质量弱时优先保证主攻类时间。\n", categoryID))
	}

	// Per language + category micro hints
	switch plan.Profile {
	case ProfileMemoryNative:
		switch categoryID {
		case "memory_safety":
			b.WriteString("C/C++：盯 length 字段、memcpy/realloc、对象释放路径与协议 RESTORE/解码。\n")
		case "resource_exhaustion":
			b.WriteString("C/C++：盯嵌套深度、按声明长度分配、解压扩展比。\n")
		case "sql_injection", "xss_injection", "expression_injection":
			b.WriteString("若仓库无 HTTP/脚本管理面，不要为凑清单虚构 Web Sink。\n")
		}
	case ProfileManagedNetwork:
		switch categoryID {
		case "memory_safety":
			b.WriteString("托管语言：除非 JNI/native，否则不要把精力放在 malloc 语义上。\n")
		case "expression_injection":
			b.WriteString("Java/Spring：重点 Sort/SpEL/OGNL；PHP/Python 则看模板与动态求值交叉点。\n")
		case "auth_bypass":
			b.WriteString("盯过滤器链、IAM/session policy、IDOR 与默认无鉴权管理端。\n")
		}
	case ProfileSystemsGo:
		switch categoryID {
		case "xxe_ssrf":
			b.WriteString("Go：对比 hostmatcher 加固路径 vs 裸 http.Get/file://。\n")
		case "race_condition":
			b.WriteString("Go：共享 map/会话状态、check-then-act 权限路径。\n")
		case "memory_safety":
			b.WriteString("仅当存在 unsafe/cgo 时深挖，否则可浅扫。\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// FormatFocusPlanForPrompt 供 Phase2 规划提示词使用。
func FormatFocusPlanForPrompt(techStack string) string {
	plan := BuildFocusPlan(techStack)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("技术栈：%s\n画像：%s\n\n", strings.TrimSpace(techStack), plan.Profile))
	b.WriteString(plan.Guidance)
	b.WriteString("\n\n主攻类别（selected 中应靠前且不可随意删除）：\n")
	for _, id := range plan.Primary {
		b.WriteString("- " + id + "\n")
	}
	if len(plan.Secondary) > 0 {
		b.WriteString("\n次要类别（建议保留）：\n")
		for _, id := range plan.Secondary {
			b.WriteString("- " + id + "\n")
		}
	}
	if len(plan.Tertiary) > 0 {
		b.WriteString("\n低优先类别（无信号时可省略，须说明理由）：\n")
		for _, id := range plan.Tertiary {
			b.WriteString("- " + id + "\n")
		}
	}
	return b.String()
}
