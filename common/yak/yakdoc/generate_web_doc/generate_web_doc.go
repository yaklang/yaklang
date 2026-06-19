package main

import (
	"flag"
	"fmt"
	"html"
	"os"
	"path"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

// YakDocParsed 用于存储解析后的注释字段
type YakDocParsed struct {
	Description     string
	LongDescription string
	Params          map[string]string // 参数名 -> 描述
	Returns         []string          // 按顺序存储返回值描述
	Example         string
}

func specialPatchValueStr(ins *yakdoc.LibInstance) string {
	valueStr := ins.ValueStr
	if ins.LibName == "os" && ins.InstanceName == "Args" {
		valueStr = "Command line arguments"
	}
	return html.EscapeString(valueStr)
}

func CheckDocCodeBlockMatched() {
	helper := yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())
	failCount := 0
	checkFunc := func(f *yakdoc.FuncDecl) {
		if len(f.Document) == 0 {
			return
		}
		if count := strings.Count(f.Document, "```"); count%2 != 0 {
			failCount++
			fmt.Printf("%s.%s code block not matched\n", f.LibName, f.MethodName)
		}
	}

	for _, lib := range helper.Libs {
		for _, f := range lib.Functions {
			checkFunc(f)
		}
	}

	for _, f := range helper.Functions {
		checkFunc(f)
	}

	for _, lib := range helper.StructMethods {
		for _, f := range lib.Functions {
			checkFunc(f)
		}
	}

	if failCount > 0 {
		panic("code block check not passed")
	}
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

// renderDetailDoc 渲染"详细描述"正文：示例标记之前的 prose 做 HTML 转义并保留围栏代码块，
// 示例代码统一改为 14 反引号 yak 围栏输出（MANUAL_EXAMPLE_SPEC），既保证示例原样不被
// 转义，又能被 verify-manual-examples.py 抽取验证。
// 关键词: 详细描述渲染, 示例 14 反引号围栏, 正文转义
func renderDetailDoc(doc string) string {
	idx := indexOfExampleMarker(doc)
	if idx == -1 {
		return escapeProseKeepCode(doc)
	}
	prose := escapeProseKeepCode(doc[:idx])
	code := extractExampleCode(doc)
	if code == "" {
		return prose
	}
	return prose + "\nExample:\n\n" + fenceExampleYak(code) + "\n"
}

// summarizeDocument 从原始文档生成函数索引表格用的单行摘要：
// 去除代码块与示例段、归一化空白、截断 150 runes、仅 HTML 转义一次并转义表格分隔符。
// 关键词: 文档摘要, 避免二次转义, 避免代码块漏进表格
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
		lines[i] = html.EscapeString(line)
	}
	return strings.Join(lines, "\n")
}

// escapeTableCell 处理表格普通文本单元：去换行、HTML 转义一次、转义表格分隔符。
func escapeTableCell(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	s = html.EscapeString(s)
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
	// 跳过示例标记所在行（如 "Example:" / "example："）
	if nl := strings.Index(rest, "\n"); nl != -1 {
		rest = rest[nl+1:]
	} else {
		return ""
	}
	// 去掉已有的三反引号围栏行，避免与外层 14 反引号围栏冲突
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

// formatYakCode 格式化yak代码，使用 YaklangCompileAndFormat 接口
// 如果格式化失败，则回退到原始代码
func formatYakCode(originalCode string) string {
	if originalCode == "" {
		return originalCode
	}

	code := strings.TrimSpace(originalCode)

	// 检查是否包含代码块标记
	hasCodeBlock := false
	codeBlockLang := ""
	codeContent := code

	if strings.HasPrefix(code, "```") {
		hasCodeBlock = true
		// 提取语言标识和代码内容
		lines := strings.Split(code, "\n")
		if len(lines) > 0 {
			firstLine := lines[0]
			codeBlockLang = strings.TrimPrefix(firstLine, "```")
			codeBlockLang = strings.TrimSpace(codeBlockLang)

			// 去除首尾的代码块标记
			if len(lines) > 2 {
				lastLineIdx := len(lines) - 1
				if strings.HasPrefix(strings.TrimSpace(lines[lastLineIdx]), "```") {
					codeContent = strings.Join(lines[1:lastLineIdx], "\n")
				} else {
					codeContent = strings.Join(lines[1:], "\n")
				}
			} else if len(lines) > 1 {
				codeContent = strings.Join(lines[1:], "\n")
			}
		}
	}

	// 尝试使用 YaklangCompileAndFormat 格式化代码
	engine := antlr4yak.New()
	formatted, err := engine.FormattedAndSyntaxChecking(codeContent)

	// 只有在格式化成功且返回非空结果时才使用格式化后的代码
	if err == nil && formatted != "" {
		codeContent = formatted
	} else {
		// 格式化失败，保持原始代码内容
		log.Debugf("failed to format yak code: %v, using original code", err)
	}

	// 如果原本有代码块标记，重新添加
	if hasCodeBlock {
		if codeBlockLang != "" {
			return fmt.Sprintf("```%s\n%s\n```", codeBlockLang, codeContent)
		}
		return fmt.Sprintf("```yak\n%s\n```", codeContent)
	}

	return codeContent
}

// 辅助函数：解析注释中的参数和返回值描述
func parseCommentDetails(doc string) *YakDocParsed {
	parsed := &YakDocParsed{
		Params: make(map[string]string),
	}

	// 按行分割并清理
	lines := strings.Split(doc, "\n")
	var cleanLines []string
	for _, line := range lines {
		// 这里的 doc 假设已经去掉了 "//" 前缀，如果没有去掉，需要先在此处 strings.TrimPrefix(line, "//")
		cleanLines = append(cleanLines, strings.TrimSpace(line))
	}

	var currentSection string // "", "long_desc", "params", "returns", "example"
	var longDescLines []string
	var exampleLines []string

	// 正则匹配: - name(type): description 或 - name: description
	listRegex := regexp.MustCompile(`^\s*-\s*([\w.]+)\s*(?:\(.*\))?\s*[:：]\s*(.*)$`)

	for i := 0; i < len(cleanLines); i++ {
		line := cleanLines[i]
		lowerLine := strings.ToLower(line)

		// 1. 检测状态切换 (关键字识别)
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

		// 2. 处理内容提取
		if line == "" && currentSection != "example" {
			continue // 忽略非代码块中的空行
		}

		switch currentSection {
		case "":
			// 初始状态逻辑：第一行是 Description，之后到“参数”之前的内容是 LongDescription
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
			// 保留原始格式（包括空格）
			exampleLines = append(exampleLines, lines[i])
		}
	}

	// 格式化输出
	// LongDescription 处理：合并行，根据原逻辑使用了 Fields 处理（注意：Fields 会压缩所有空格）
	if len(longDescLines) > 0 {
		rawLongDesc := strings.Join(longDescLines, " ")
		parsed.LongDescription = strings.Join(strings.Fields(rawLongDesc), "")
	}

	parsed.Example = strings.Join(exampleLines, "\n")

	// 格式化example代码
	if parsed.Example != "" {
		parsed.Example = formatYakCode(parsed.Example)
	}

	return parsed
}

func GenerateSingleFile(basepath string, lib *yakdoc.ScriptLib) {
	file, err := os.Create(path.Join(basepath, lib.Name+".md"))
	if err != nil {
		log.Errorf("create file error: %v", err)
	}
	defer file.Close()
	file.WriteString("# " + lib.Name + "\n\n")
	if len(lib.Instances) > 0 {
		file.WriteString("|实例名|实例描述|\n")
		file.WriteString("|:------|:--------|\n")
		keys := lo.Keys(lib.Instances)
		sort.Strings(keys)
		for _, key := range keys {
			ins := lib.Instances[key]
			file.WriteString(fmt.Sprintf("%s|(%s) %s|\n",
				html.EscapeString(ins.InstanceName),
				html.EscapeString(ins.Type),
				specialPatchValueStr(ins),
			))
		}
		file.WriteString("\n")
	}

	file.WriteString("|函数名|函数描述/介绍|\n")
	file.WriteString("|:------|:--------|\n")

	// 将Functions转成list
	funcList := lo.MapToSlice(lib.Functions, func(key string, value *yakdoc.FuncDecl) *yakdoc.FuncDecl {
		return value
	})
	sort.SliceStable(funcList, func(i, j int) bool {
		return funcList[i].MethodName < funcList[j].MethodName
	})
	bufList := make([]strings.Builder, 0, len(funcList))

	for _, fun := range funcList {
		// 解析注释里的参数/返回值解释，用于填充表格第三列
		parsed := parseCommentDetails(fun.Document)

		// 函数索引表格用的单行摘要：从原始文档生成，仅转义一次，去除代码块/示例，避免破表
		// 关键词: 函数索引摘要, 避免二次转义
		simpleDocument := summarizeDocument(fun.Document)

		// 详细描述正文：示例前转义 prose 并保留围栏代码块，示例后原样保留；空文档加占位
		detailDoc := renderDetailDoc(fun.Document)
		if strings.TrimSpace(detailDoc) == "" {
			detailDoc = "暂无描述"
		}

		lowerMethodName := strings.ToLower(fun.MethodName)
		file.WriteString(fmt.Sprintf("| [%s.%s](#%s) |%s|\n",
			html.EscapeString(fun.LibName),
			html.EscapeString(fun.MethodName),
			html.EscapeString(lowerMethodName),
			simpleDocument,
		))
		buf := strings.Builder{}
		buf.WriteString(fmt.Sprintf("### %s\n\n", html.EscapeString(fun.MethodName)))
		buf.WriteString(fmt.Sprintf("#### 详细描述\n%s\n\n", detailDoc))
		// 定义放在行内代码块里，不能 HTML 转义，否则 < 与 & 会显示成 &lt; / &amp;
		buf.WriteString(fmt.Sprintf("#### 定义\n\n`%s`\n\n", fun.Decl))
		if len(fun.Params) > 0 {
			buf.WriteString("#### 参数\n")
			buf.WriteString("|参数名|参数类型|参数解释|\n")
			buf.WriteString("|:-----------|:---------- |:-----------|\n")
			for _, param := range fun.Params {
				// 类型在行内代码块里，不转义；参数解释取自注释，无则留空
				buf.WriteString(fmt.Sprintf("| %s | `%s` | %s |\n", html.EscapeString(param.Name), param.Type, escapeTableCell(parsed.Params[param.Name])))
			}
			buf.WriteString("\n")
		}
		if len(fun.Results) > 0 {
			buf.WriteString("#### 返回值\n")
			buf.WriteString("|返回值(顺序)|返回值类型|返回值解释|\n")
			buf.WriteString("|:-----------|:---------- |:-----------|\n")
			for i, result := range fun.Results {
				explanation := ""
				if i < len(parsed.Returns) {
					explanation = parsed.Returns[i]
				}
				buf.WriteString(fmt.Sprintf("| %s | `%s` | %s |\n", html.EscapeString(result.Name), result.Type, escapeTableCell(explanation)))
			}
			buf.WriteString("\n")
		}
		buf.WriteString("\n")
		bufList = append(bufList, buf)
	}
	file.WriteString("\n\n")
	file.WriteString("## 函数定义\n")
	for _, buf := range bufList {
		file.WriteString(buf.String())
	}
}

func GenerateSingleFileMDX(basepath string, lib *yakdoc.ScriptLib, description string) {
	file, err := os.Create(path.Join(basepath, lib.Name+".mdx"))
	if err != nil {
		log.Errorf("create file error: %v", err)
	}
	defer file.Close()
	file.WriteString("---\n")
	file.WriteString("sidebar_label: " + lib.Name + "\n")
	file.WriteString("slug: /api/" + lib.Name + "\n")
	file.WriteString("title: " + lib.Name + "\n")
	file.WriteString("description: " + description + "\n")
	file.WriteString("---\n")

	file.WriteString("import Tabs from '@theme/Tabs';\n")
	file.WriteString("import TabItem from '@theme/TabItem';\n")
	file.WriteString("import CodeBlock from '@theme/CodeBlock';\n\n")

	file.WriteString(":::info\n" + description + "\n:::\n\n")

	// 预处理：解析所有函数并按分类分组
	type FuncInfo struct {
		Decl   *yakdoc.FuncDecl
		Parsed *YakDocParsed
	}

	// 将Functions转成list
	funcList := lo.MapToSlice(lib.Functions, func(key string, value *yakdoc.FuncDecl) FuncInfo {
		return FuncInfo{Decl: value, Parsed: parseCommentDetails(value.Document)}
	})
	// 全局排序：按方法名排序
	sort.SliceStable(funcList, func(i, j int) bool {
		return funcList[i].Decl.MethodName < funcList[j].Decl.MethodName
	})

	file.WriteString("## 函数索引\n\n")
	file.WriteString("|函数名|函数描述/介绍|\n")
	file.WriteString("|:------|:--------|\n")

	for _, f := range funcList {
		lowerMethodName := strings.ToLower(f.Decl.MethodName)
		file.WriteString(fmt.Sprintf("| [%s.%s](#%s) | %s |\n",
			html.EscapeString(f.Decl.LibName),
			html.EscapeString(f.Decl.MethodName),
			html.EscapeString(lowerMethodName),
			f.Parsed.Description,
		))
	}

	file.WriteString("\n\n")

	if len(lib.Instances) > 0 {
		file.WriteString("## 实例索引\n\n")
		file.WriteString("|实例名|实例描述|\n")
		file.WriteString("|:------|:--------|\n")
		keys := lo.Keys(lib.Instances)
		sort.Strings(keys)
		for _, key := range keys {
			ins := lib.Instances[key]
			file.WriteString(fmt.Sprintf("%s|(%s) %s|\n",
				html.EscapeString(ins.InstanceName),
				html.EscapeString(ins.Type),
				specialPatchValueStr(ins),
			))
		}
		file.WriteString("\n")
	}

	bufList := make([]strings.Builder, 0, len(funcList))
	file.WriteString("## API 详情\n\n")

	for _, f := range funcList {
		fun := f.Decl
		p := f.Parsed
		buf := strings.Builder{}

		buf.WriteString(fmt.Sprintf("### %s\n\n", fun.MethodName))
		buf.WriteString(fmt.Sprintf("- 描述: %s\n\n", p.Description))
		if p.LongDescription != "" {
			buf.WriteString(fmt.Sprintf("- 详细描述: %s\n\n", p.LongDescription))
		}
		buf.WriteString("\n<Tabs>\n")
		buf.WriteString(fmt.Sprintf("<TabItem value=\"%s-1\" label=\"定义\" default>\n\n", fun.MethodName))
		buf.WriteString(fmt.Sprintf("```go\n%s\n```\n\n", html.EscapeString(fun.Decl)))

		if len(fun.Params) > 0 {
			buf.WriteString("**参数配置信息**\n")
			buf.WriteString("\n|参数名|参数类型|参数解释|\n")
			buf.WriteString("|:-----------|:---------- |:-----------|\n")
			for _, param := range fun.Params {
				explanation := p.Params[param.Name]
				buf.WriteString(fmt.Sprintf("| %s | `%s` |  %s |\n", html.EscapeString(param.Name), html.EscapeString(param.Type), explanation))
			}
			buf.WriteString("\n")
		}
		if len(fun.Results) > 0 {
			buf.WriteString("**返回值**\n")
			buf.WriteString("\n|返回值(顺序)|返回值类型|返回值解释|\n")
			buf.WriteString("|:-----------|:---------- |:-----------|\n")
			for i, result := range fun.Results {
				explanation := ""
				if i < len(p.Returns) {
					explanation = p.Returns[i]
				}
				buf.WriteString(fmt.Sprintf("| %s | `%s` |  %s |\n", html.EscapeString(result.Name), html.EscapeString(result.Type), explanation))
			}
			buf.WriteString("\n")
		}
		buf.WriteString("</TabItem>\n")
		// 示例统一用 14 反引号 yak 围栏（MANUAL_EXAMPLE_SPEC），便于 verify-manual-examples.py 抽取验证
		if exampleCode := extractExampleCode(fun.Document); exampleCode != "" {
			buf.WriteString(fmt.Sprintf("<TabItem value=\"%s-2\" label=\"示例\">\n", fun.MethodName))
			buf.WriteString(fmt.Sprintf("\n\n%s\n\n", fenceExampleYak(exampleCode)))
			buf.WriteString("</TabItem>\n")
		}
		buf.WriteString("</Tabs>\n\n")
		buf.WriteString("\n---\n\n")
		bufList = append(bufList, buf)
	}

	file.WriteString("\n\n")
	for _, buf := range bufList {
		file.WriteString(buf.String())
	}
}

// FuncCoverage 记录单个导出函数的文档缺口（缺描述/缺示例/缺参数解释/缺返回解释）。
// 关键词: 文档覆盖率, 文档缺口
type FuncCoverage struct {
	Lib            string
	Method         string
	MissingDesc    bool     // 无描述（首行描述为空）
	MissingExample bool     // 无 Example 段
	ParamsNoExpl   []string // 缺解释的参数名
	ResultsNoExpl  int      // 缺解释的返回值个数
}

// HasGap 该函数是否存在任一文档缺口。
func (c *FuncCoverage) HasGap() bool {
	return c.MissingDesc || c.MissingExample || len(c.ParamsNoExpl) > 0 || c.ResultsNoExpl > 0
}

// CoverageReport 全量文档覆盖率统计结果。
type CoverageReport struct {
	Total    int             // 导出函数总数
	WithGap  int             // 存在缺口的函数数
	Gaps     []*FuncCoverage // 仅包含有缺口的函数明细（按库名+方法名排序）
	libCount map[string]int  // 每库缺口计数
}

// collectDocCoverage 遍历所有库的导出函数，统计文档缺口。该函数无副作用、可测试。
// 关键词: collectDocCoverage, 文档覆盖率统计
func collectDocCoverage(libs map[string]*yakdoc.ScriptLib) *CoverageReport {
	report := &CoverageReport{libCount: make(map[string]int)}
	// 库与函数都排序，保证输出稳定
	libNames := lo.Keys(libs)
	sort.Strings(libNames)

	for _, libName := range libNames {
		lib := libs[libName]
		methodNames := lo.Keys(lib.Functions)
		sort.Strings(methodNames)
		for _, name := range methodNames {
			fun := lib.Functions[name]
			report.Total++

			parsed := parseCommentDetails(fun.Document)
			cov := &FuncCoverage{Lib: fun.LibName, Method: fun.MethodName}
			cov.MissingDesc = strings.TrimSpace(parsed.Description) == ""
			cov.MissingExample = extractExampleCode(fun.Document) == ""
			for _, param := range fun.Params {
				if strings.TrimSpace(parsed.Params[param.Name]) == "" {
					cov.ParamsNoExpl = append(cov.ParamsNoExpl, param.Name)
				}
			}
			for i := range fun.Results {
				if i >= len(parsed.Returns) || strings.TrimSpace(parsed.Returns[i]) == "" {
					cov.ResultsNoExpl++
				}
			}

			if cov.HasGap() {
				report.WithGap++
				report.Gaps = append(report.Gaps, cov)
				report.libCount[fun.LibName]++
			}
		}
	}
	return report
}

// LogSummary 以英文 log 打印覆盖率汇总（非阻断）。逐项 Warn 缺口、末尾打印每库与总计。
func (r *CoverageReport) LogSummary() {
	for _, g := range r.Gaps {
		var missing []string
		if g.MissingDesc {
			missing = append(missing, "description")
		}
		if len(g.ParamsNoExpl) > 0 {
			missing = append(missing, fmt.Sprintf("param-explanation(%s)", strings.Join(g.ParamsNoExpl, ",")))
		}
		if g.ResultsNoExpl > 0 {
			missing = append(missing, fmt.Sprintf("return-explanation(%d)", g.ResultsNoExpl))
		}
		if g.MissingExample {
			missing = append(missing, "example")
		}
		log.Warnf("doc coverage gap: %s.%s missing %s", g.Lib, g.Method, strings.Join(missing, ", "))
	}

	libs := lo.Keys(r.libCount)
	sort.Strings(libs)
	for _, name := range libs {
		log.Warnf("doc coverage: lib %s has %d function(s) with gaps", name, r.libCount[name])
	}
	log.Infof("doc coverage summary: %d/%d functions have gaps (%d ok)", r.WithGap, r.Total, r.Total-r.WithGap)
}

// WriteMarkdown 把覆盖率明细写成 markdown 底单，用于驱动 backfill。该文件应写到 docs/api 之外。
func (r *CoverageReport) WriteMarkdown(p string) error {
	if dir := path.Dir(p); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	buf := strings.Builder{}
	buf.WriteString("# API Documentation Coverage Baseline\n\n")
	buf.WriteString(fmt.Sprintf("Total functions: %d; functions with gaps: %d; ok: %d\n\n", r.Total, r.WithGap, r.Total-r.WithGap))

	// 按库聚合
	byLib := make(map[string][]*FuncCoverage)
	for _, g := range r.Gaps {
		byLib[g.Lib] = append(byLib[g.Lib], g)
	}
	libs := lo.Keys(byLib)
	sort.Strings(libs)
	for _, lib := range libs {
		buf.WriteString(fmt.Sprintf("## %s (%d)\n\n", lib, len(byLib[lib])))
		buf.WriteString("|function|missing description|missing param explanation|missing return explanation|missing example|\n")
		buf.WriteString("|:--|:--|:--|:--|:--|\n")
		for _, g := range byLib[lib] {
			descMark := ""
			if g.MissingDesc {
				descMark = "yes"
			}
			paramMark := ""
			if len(g.ParamsNoExpl) > 0 {
				paramMark = strings.Join(g.ParamsNoExpl, ",")
			}
			retMark := ""
			if g.ResultsNoExpl > 0 {
				retMark = fmt.Sprintf("%d", g.ResultsNoExpl)
			}
			exMark := ""
			if g.MissingExample {
				exMark = "yes"
			}
			buf.WriteString(fmt.Sprintf("|%s|%s|%s|%s|%s|\n", g.Method, descMark, paramMark, retMark, exMark))
		}
		buf.WriteString("\n")
	}
	return os.WriteFile(p, []byte(buf.String()), 0o644)
}

func main() {
	// 关闭 GC 以规避 vendored ANTLR4 运行时（v4.0.0-20220911224424）的堆损坏 bug：
	// 该运行时偶发在 prediction-context 结构上写出野指针，GC 标记线程扫描堆时会触发
	// "fatal error: found bad pointer in Go heap" 导致生成器随机崩溃。本工具为短生命周期
	// 的一次性 CLI，关闭 GC 可稳定规避崩溃；根治需升级该 vendored ANTLR4 运行时。
	debug.SetGCPercent(-1)

	var (
		strict         bool
		coverageReport string
	)
	flag.BoolVar(&strict, "strict", false, "exit non-zero if any doc coverage gap is found (local use only, never enable in CI)")
	flag.StringVar(&coverageReport, "coverage-report", "", "write a markdown coverage baseline to this path (must be outside docs/api so it is not synced)")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		return
	}
	basepath := args[0]
	if !utils.IsDir(basepath) {
		if err := os.MkdirAll(basepath, 0o777); err != nil {
			log.Errorf("create dir error: %v", err)
			return
		}
	}

	CheckDocCodeBlockMatched()
	// 列表维护需要生成 MDX 的库及对应描述
	mdxLibs := map[string]string{
		"ai": "AI 模块提供了与多种大语言模型集成的能力，支持 OpenAI、ChatGLM、Moonshot 等主流 AI 服务。通过统一的接口调用不同的 AI 服务，支持对话、函数调用、流式输出等功能。",
		// 可以继续添加其他库名和描述
	}

	helper := yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())
	for _, lib := range helper.Libs {
		if desc, ok := mdxLibs[lib.Name]; ok {
			GenerateSingleFileMDX(basepath, lib, desc)
		} else {
			GenerateSingleFile(basepath, lib)
		}
	}

	// 文档覆盖率统计：非阻断，仅打印 warning 协助本地补全；CI 永不因此失败（除非显式 -strict）。
	report := collectDocCoverage(helper.Libs)
	report.LogSummary()
	if coverageReport != "" {
		if err := report.WriteMarkdown(coverageReport); err != nil {
			log.Errorf("write coverage report failed: %v", err)
		} else {
			log.Infof("coverage baseline written to %s", coverageReport)
		}
	}
	if strict && report.WithGap > 0 {
		log.Errorf("strict mode enabled and %d function(s) have documentation gaps", report.WithGap)
		os.Exit(1)
	}
}
