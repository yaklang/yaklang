// Package webdoc 把 yaklang 导出库(yakdoc.ScriptLib)渲染为文档站(Docusaurus)使用的
// Markdown / MDX 文本。本包只依赖 yakdoc 数据类型与标准库，不引入 yak 引擎，因此可被
// 单元测试、边界测试与整文档不变量测试直接覆盖，保证 Markdown 构建健壮、不在文档站崩溃。
// 关键词: web 文档渲染, RenderLibMarkdown, 可测渲染, Markdown 健壮性
package webdoc

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
)

// YakDocParsed 用于存储解析后的注释字段
type YakDocParsed struct {
	Description     string
	LongDescription string
	Params          map[string]string // 参数名 -> 描述
	Returns         []string          // 按顺序存储返回值描述
	Example         string
}

// instanceValueRaw 返回实例的原始展示值(未做任何转义),特殊实例做语义补丁。
// 转义统一交给调用方的 escapeTableCell,避免二次转义。
func instanceValueRaw(ins *yakdoc.LibInstance) string {
	v := ins.ValueStr
	if ins.LibName == "os" && ins.InstanceName == "Args" {
		v = "Command line arguments"
	}
	return v
}

// stripFencedCodeBlocks 去除 ```...``` 围栏代码块（含围栏行），
// 用于生成函数索引表格里的单行摘要，避免代码块漏进表格破坏渲染。
func stripFencedCodeBlocks(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	inFence := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// indexOfExampleMarker 返回第一个示例标记的位置（覆盖 ASCII/全角冒号与中英文标记），
// 找不到返回 -1。
func indexOfExampleMarker(s string) int {
	markers := []string{"Example:", "example:", "Example：", "example：", "示例:", "示例："}
	idx := -1
	for _, m := range markers {
		if i := strings.Index(s, m); i != -1 && (idx == -1 || i < idx) {
			idx = i
		}
	}
	return idx
}

// cutAtExampleMarker 在第一个示例标记处截断，用于生成摘要时丢弃示例段落。
func cutAtExampleMarker(s string) string {
	if idx := indexOfExampleMarker(s); idx != -1 {
		return s[:idx]
	}
	return s
}

// bareURLSchemeRe 匹配 URL 协议头 "http://" / "https://"。
// 关键词: 裸URL autolink, MDX 构建崩溃, 协议分隔符转义
var bareURLSchemeRe = regexp.MustCompile(`https?://`)

// neutralizeBareURLAutolinks 把正文/表格里的"裸 URL"协议分隔符 "://" 替换为实体
// "&#58;//"，阻断 gfm autolink。否则形如 "http://127.0.0.1:8080"） 的文本会被
// gfm autolink 把后续的引号/全角标点一并并入 URL，生成 http://127.0.0.1:8080"）
// 这种非法 URL，导致文档站 MDX 构建崩溃。
//
// 转义后浏览器仍会把实体还原显示为 http:// ，只是该 URL 不再被识别为可点击链接。
// 唯一需要保留可点击的是 markdown 链接 "[text](http://...)"，因此跳过紧跟在 "](" 之后的 URL。
// 关键词: neutralizeBareURLAutolinks, 阻断 autolink, 跳过 markdown 链接
func neutralizeBareURLAutolinks(s string) string {
	locs := bareURLSchemeRe.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return s
	}
	var b strings.Builder
	prev := 0
	for _, loc := range locs {
		start, end := loc[0], loc[1]
		b.WriteString(s[prev:start])
		if start >= 2 && s[start-2:start] == "](" {
			b.WriteString(s[start:end])
		} else {
			b.WriteString(strings.Replace(s[start:end], "://", "&#58;//", 1))
		}
		prev = end
	}
	b.WriteString(s[prev:])
	return b.String()
}

// summarizeDocument 从原始文档生成单行摘要：去代码块与示例段、归一化空白、截断 150 runes、
// 仅 HTML 转义一次并转义表格分隔符。保留供向后兼容与潜在用途。
// 关键词: 文档摘要, 避免二次转义
func summarizeDocument(raw string) string {
	s := stripFencedCodeBlocks(raw)
	s = cutAtExampleMarker(s)
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) > 150 {
		s = string(runes[:150]) + "..."
	}
	s = html.EscapeString(s)
	s = neutralizeBareURLAutolinks(s)
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
}

// escapeProseKeepCode 对正文按行 HTML 转义，但保留 ```...``` 围栏代码块原样，
// 避免代码块内的 < 与 & 被转义成 &lt; / &amp; 字面量。
func escapeProseKeepCode(s string) string {
	lines := strings.Split(s, "\n")
	inFence := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		lines[i] = neutralizeBareURLAutolinks(html.EscapeString(line))
	}
	return strings.Join(lines, "\n")
}

// escapeTableCell 处理表格普通文本单元：去换行、HTML 转义一次、转义表格分隔符。
func escapeTableCell(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	s = html.EscapeString(s)
	s = neutralizeBareURLAutolinks(s)
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
}

// exampleFence 是 MANUAL_EXAMPLE_SPEC §2 规定的 14 个反引号围栏。
// 采用超长围栏保证示例内部出现三反引号/Markdown 片段时整段仍是同一代码块。
const exampleFence = "``````````````" // 14 backticks

// dedent 去除多行文本的公共前导空白（按字节，适配制表符或空格统一缩进的示例）。
func dedent(s string) string {
	lines := strings.Split(s, "\n")
	minIndent := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		indent := len(l) - len(strings.TrimLeft(l, " \t"))
		if minIndent == -1 || indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent <= 0 {
		return s
	}
	for i, l := range lines {
		if len(l) >= minIndent {
			lines[i] = l[minIndent:]
		}
	}
	return strings.Join(lines, "\n")
}

// extractExampleCode 从 doc 注释提取示例代码：去示例标记行、去已有 ``` 围栏、
// 去公共缩进与首尾空行，得到可直接执行/包裹的纯代码。
// 关键词: 示例提取, Example 段, 去围栏去缩进
func extractExampleCode(doc string) string {
	idx := indexOfExampleMarker(doc)
	if idx == -1 {
		return ""
	}
	rest := doc[idx:]
	if nl := strings.Index(rest, "\n"); nl != -1 {
		rest = rest[nl+1:]
	} else {
		return ""
	}
	kept := make([]string, 0)
	for _, line := range strings.Split(rest, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(dedent(strings.Join(kept, "\n")))
}

// fenceExampleYak 用 14 反引号 yak 围栏包裹示例代码（MANUAL_EXAMPLE_SPEC §2），
// 使 verify-manual-examples.py 能稳定抽取并执行。
func fenceExampleYak(code string) string {
	return exampleFence + "yak\n" + code + "\n" + exampleFence
}

// parseCommentDetails 解析注释中的描述、参数与返回值说明。
// 关键词: 注释解析, 参数/返回值说明
func parseCommentDetails(doc string) *YakDocParsed {
	parsed := &YakDocParsed{
		Params: make(map[string]string),
	}

	lines := strings.Split(doc, "\n")
	var cleanLines []string
	for _, line := range lines {
		cleanLines = append(cleanLines, strings.TrimSpace(line))
	}

	var currentSection string // "", "params", "returns", "example"
	var longDescLines []string
	var exampleLines []string

	listRegex := regexp.MustCompile(`^\s*-\s*([\w.]+)\s*(?:\(.*\))?\s*[:：]\s*(.*)$`)

	for i := 0; i < len(cleanLines); i++ {
		line := cleanLines[i]
		lowerLine := strings.ToLower(line)

		if strings.HasPrefix(line, "参数") {
			currentSection = "params"
			continue
		} else if strings.HasPrefix(line, "返回值") {
			currentSection = "returns"
			continue
		} else if strings.HasPrefix(lowerLine, "example") {
			currentSection = "example"
			continue
		}

		if line == "" && currentSection != "example" {
			continue
		}

		switch currentSection {
		case "":
			if parsed.Description == "" {
				parsed.Description = line
			} else {
				longDescLines = append(longDescLines, line)
			}
		case "params":
			matches := listRegex.FindStringSubmatch(line)
			if len(matches) > 2 {
				parsed.Params[matches[1]] = matches[2]
			}
		case "returns":
			matches := listRegex.FindStringSubmatch(line)
			if len(matches) > 2 {
				parsed.Returns = append(parsed.Returns, matches[2])
			} else if strings.HasPrefix(line, "-") {
				parsed.Returns = append(parsed.Returns, strings.TrimSpace(strings.TrimPrefix(line, "-")))
			}
		case "example":
			exampleLines = append(exampleLines, lines[i])
		}
	}

	if len(longDescLines) > 0 {
		rawLongDesc := strings.Join(longDescLines, " ")
		parsed.LongDescription = strings.Join(strings.Fields(rawLongDesc), "")
	}

	parsed.Example = strings.Join(exampleLines, "\n")
	return parsed
}

// firstStructuralMarker 返回原始 doc 中最早出现的"结构标记"字节偏移：示例标记、或行首
// 以 参数/返回值/Returns/Return Values 开头的小节标题。找不到返回 -1。
// 关键词: 描述去重, 结构标记定位
func firstStructuralMarker(doc string) int {
	best := -1
	consider := func(i int) {
		if i >= 0 && (best == -1 || i < best) {
			best = i
		}
	}
	consider(indexOfExampleMarker(doc))

	offset := 0
	for _, line := range strings.Split(doc, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "参数") || strings.HasPrefix(t, "返回值") ||
			strings.HasPrefix(t, "Returns") || strings.HasPrefix(t, "Return Values") {
			consider(offset)
			break
		}
		offset += len(line) + 1 // +1 还原被 Split 去掉的换行
	}
	return best
}

// leadingProse 取函数文档里"参数/返回值/示例"之前的描述正文，做 HTML 转义(保留代码块)、
// 折叠多余空行并去首尾空白。用于"详细描述"区块，避免把参数/返回值列表重复 dump。
// 关键词: leadingProse, 描述去重
func leadingProse(doc string) string {
	prose := doc
	if idx := firstStructuralMarker(doc); idx >= 0 {
		prose = doc[:idx]
	}
	return strings.TrimSpace(collapseBlankLines(escapeProseKeepCode(prose)))
}

// collapseBlankLines 把连续多个空行折叠为一个；围栏代码块内部原样保留(代码里的空行有意义)。
// 关键词: collapseBlankLines, 空行折叠
func collapseBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := false
	inFence := false
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			inFence = !inFence
			out = append(out, l)
			blank = false
			continue
		}
		if inFence {
			out = append(out, l)
			continue
		}
		isBlank := strings.TrimSpace(l) == ""
		if isBlank && blank {
			continue
		}
		out = append(out, l)
		blank = isBlank
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}

// isOptionFunc 判定一个函数是否为"配置选项"风格：恰好一个返回值且类型以 Option 结尾
// (如 PocConfigOption / AIEngineConfigOption)。该启发式在全库普遍成立。
// 关键词: 配置选项判定, 函数分组
func isOptionFunc(fun *yakdoc.FuncDecl) bool {
	return len(fun.Results) == 1 && strings.HasSuffix(fun.Results[0].Type, "Option")
}

// classifyFunctions 把函数分为"主要函数"与"配置选项"两组，组内保持入参顺序。
// 关键词: classifyFunctions, 主要函数, 配置选项
func classifyFunctions(funcs []*yakdoc.FuncDecl) (core, options []*yakdoc.FuncDecl) {
	for _, f := range funcs {
		if isOptionFunc(f) {
			options = append(options, f)
		} else {
			core = append(core, f)
		}
	}
	return
}

// sortedFuncs 把库的函数表转为按方法名稳定排序的切片。
func sortedFuncs(lib *yakdoc.ScriptLib) []*yakdoc.FuncDecl {
	funcList := lo.MapToSlice(lib.Functions, func(_ string, value *yakdoc.FuncDecl) *yakdoc.FuncDecl {
		return value
	})
	sort.SliceStable(funcList, func(i, j int) bool {
		return funcList[i].MethodName < funcList[j].MethodName
	})
	return funcList
}

// assignAnchors 给定最终展示顺序，为每个函数分配唯一锚点 id(默认方法名小写)。
// 大小写冲突或重名时追加 -2/-3 序号，保证同页 id 唯一且索引链接与标题一致。
// 关键词: 锚点分配, 唯一 id, 断锚修复
func assignAnchors(order []*yakdoc.FuncDecl) map[*yakdoc.FuncDecl]string {
	used := map[string]int{}
	res := make(map[*yakdoc.FuncDecl]string, len(order))
	for _, f := range order {
		base := strings.ToLower(f.MethodName)
		id := base
		if n, ok := used[base]; ok {
			id = fmt.Sprintf("%s-%d", base, n+1)
			used[base] = n + 1
		} else {
			used[base] = 1
		}
		res[f] = id
	}
	return res
}

// RenderLibMarkdown 把一个库渲染为增强版 Markdown：分组索引 + 分组详情、签名代码块、
// 加粗小节标签、去重描述、显式唯一锚点、函数间分隔线。formatExample 为示例代码格式化器，
// 传 nil 表示不格式化(保持注释原样)。
// 关键词: RenderLibMarkdown, 分组索引, 显式锚点, 签名代码块
func RenderLibMarkdown(lib *yakdoc.ScriptLib, formatExample func(string) string) string {
	if formatExample == nil {
		formatExample = func(s string) string { return s }
	}
	var b strings.Builder

	// 库 H1 用显式且不与函数锚点冲突的 id(library- 前缀),规避库名与同名函数抢 slug。
	b.WriteString(fmt.Sprintf("# %s {#library-%s}\n\n", html.EscapeString(lib.Name), lib.Name))

	funcList := sortedFuncs(lib)

	// 事实性概览行(非杜撰)。
	if len(funcList) > 0 || len(lib.Instances) > 0 {
		parts := make([]string, 0, 2)
		if len(funcList) > 0 {
			parts = append(parts, fmt.Sprintf("%d 个函数", len(funcList)))
		}
		if len(lib.Instances) > 0 {
			parts = append(parts, fmt.Sprintf("%d 个实例", len(lib.Instances)))
		}
		b.WriteString("> 共 " + strings.Join(parts, "、") + "\n\n")
	}

	// 实例区块
	if len(lib.Instances) > 0 {
		b.WriteString("## 实例\n\n")
		b.WriteString("|实例名|类型|说明|\n")
		b.WriteString("|:--|:--|:--|\n")
		keys := lo.Keys(lib.Instances)
		sort.Strings(keys)
		for _, key := range keys {
			ins := lib.Instances[key]
			b.WriteString(fmt.Sprintf("| %s | `%s` | %s |\n",
				escapeTableCell(ins.InstanceName),
				ins.Type,
				escapeTableCell(instanceValueRaw(ins)),
			))
		}
		b.WriteString("\n")
	}

	if len(funcList) == 0 {
		return b.String()
	}

	core, options := classifyFunctions(funcList)
	grouped := len(core) > 0 && len(options) > 0

	var order []*yakdoc.FuncDecl
	if grouped {
		order = append(order, core...)
		order = append(order, options...)
	} else {
		order = funcList
	}
	anchors := assignAnchors(order)

	// 函数索引
	b.WriteString("## 函数索引\n\n")
	writeIndex := func(funcs []*yakdoc.FuncDecl) {
		b.WriteString("|函数|说明|\n")
		b.WriteString("|:--|:--|\n")
		for _, fun := range funcs {
			parsed := parseCommentDetails(fun.Document)
			b.WriteString(fmt.Sprintf("| [%s.%s](#%s) | %s |\n",
				html.EscapeString(fun.LibName),
				html.EscapeString(fun.MethodName),
				anchors[fun],
				escapeTableCell(parsed.Description),
			))
		}
		b.WriteString("\n")
	}
	if grouped {
		b.WriteString("**主要函数**\n\n")
		writeIndex(core)
		b.WriteString("**配置选项**\n\n")
		writeIndex(options)
	} else {
		writeIndex(funcList)
	}

	// 函数详情
	b.WriteString("## 函数详情\n\n")
	writeDetails := func(funcs []*yakdoc.FuncDecl) {
		for _, fun := range funcs {
			b.WriteString(renderFuncDetail(fun, anchors[fun], formatExample))
		}
	}
	if grouped {
		b.WriteString("**主要函数**\n\n")
		writeDetails(core)
		b.WriteString("**配置选项**\n\n")
		writeDetails(options)
	} else {
		writeDetails(funcList)
	}

	return b.String()
}

// renderFuncDetail 渲染单个函数的详情块：标题(显式锚点) + 签名代码块 + 去重描述 +
// 参数表 + 返回值表 + 示例 + 分隔线。
// 关键词: renderFuncDetail, 签名代码块, 加粗小节
func renderFuncDetail(fun *yakdoc.FuncDecl, anchor string, formatExample func(string) string) string {
	parsed := parseCommentDetails(fun.Document)
	var b strings.Builder

	b.WriteString(fmt.Sprintf("### %s {#%s}\n\n", html.EscapeString(fun.MethodName), anchor))

	// 签名放进 go 代码块(围栏内原样,不转义,< & 安全)。
	b.WriteString("```go\n" + fun.Decl + "\n```\n\n")

	prose := leadingProse(fun.Document)
	if strings.TrimSpace(prose) == "" {
		prose = "暂无描述"
	}
	b.WriteString(prose + "\n\n")

	if len(fun.Params) > 0 {
		b.WriteString("**参数**\n\n")
		b.WriteString("|参数名|类型|说明|\n")
		b.WriteString("|:--|:--|:--|\n")
		for _, param := range fun.Params {
			b.WriteString(fmt.Sprintf("| %s | `%s` | %s |\n",
				html.EscapeString(param.Name), param.Type, escapeTableCell(parsed.Params[param.Name])))
		}
		b.WriteString("\n")
	}

	if len(fun.Results) > 0 {
		b.WriteString("**返回值**\n\n")
		b.WriteString("|序号|类型|说明|\n")
		b.WriteString("|:--|:--|:--|\n")
		for i, result := range fun.Results {
			explanation := ""
			if i < len(parsed.Returns) {
				explanation = parsed.Returns[i]
			}
			b.WriteString(fmt.Sprintf("| %s | `%s` | %s |\n",
				html.EscapeString(result.Name), result.Type, escapeTableCell(explanation)))
		}
		b.WriteString("\n")
	}

	if code := extractExampleCode(fun.Document); code != "" {
		b.WriteString("**示例**\n\n")
		b.WriteString(fenceExampleYak(formatExample(code)) + "\n\n")
	}

	b.WriteString("---\n\n")
	return b.String()
}
