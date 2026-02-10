package main

import (
	"fmt"
	"html"
	"os"
	"path"
	"regexp"
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
		document := fun.Document
		exampleIndex := strings.Index(document, "Example:")
		if exampleIndex != -1 {
			// Example 代码块不应该替换<和>
			doc := document[:exampleIndex]
			doc = html.EscapeString(doc)
			document = doc + document[exampleIndex:]
		} else {
			document = html.EscapeString(document)
		}

		// 简略的描述，去除\r，替换\n，删除Example:后面的内容，转义|，截取150个字符
		simpleDocument := document
		simpleDocument = strings.ReplaceAll(simpleDocument, "\r", "")
		simpleDocument = strings.ReplaceAll(simpleDocument, "\n", " ")
		exampleIndex = strings.Index(simpleDocument, "Example:")
		if exampleIndex != -1 {
			simpleDocument = simpleDocument[:exampleIndex]
		}
		ellipsisRunes := []rune(simpleDocument)
		if len(ellipsisRunes) > 150 {
			simpleDocument = fmt.Sprintf("%s...", string(ellipsisRunes[:150]))
			simpleDocument = strings.ReplaceAll(simpleDocument, "|", "\\|")
		}

		// exampleIndex = strings.Index(document, "Example:")
		// if exampleIndex != -1 {
		// 	document = strings.ReplaceAll(document[:exampleIndex], "\n", "\n\n") + document[exampleIndex:]
		// }
		lowerMethodName := strings.ToLower(fun.MethodName)
		file.WriteString(fmt.Sprintf("| [%s.%s](#%s) |%s|\n",
			html.EscapeString(fun.LibName),
			html.EscapeString(fun.MethodName),
			html.EscapeString(lowerMethodName),
			html.EscapeString(simpleDocument),
		))
		buf := strings.Builder{}
		buf.WriteString(fmt.Sprintf("### %s\n\n", html.EscapeString(fun.MethodName)))
		buf.WriteString(fmt.Sprintf("#### 详细描述\n%s\n\n", document))
		buf.WriteString(fmt.Sprintf("#### 定义\n\n`%s`\n\n", html.EscapeString(fun.Decl)))
		if len(fun.Params) > 0 {
			buf.WriteString("#### 参数\n")
			buf.WriteString("|参数名|参数类型|参数解释|\n")
			buf.WriteString("|:-----------|:---------- |:-----------|\n")
			for _, param := range fun.Params {
				buf.WriteString(fmt.Sprintf("| %s | `%s` |   |\n", html.EscapeString(param.Name), html.EscapeString(param.Type)))
			}
			buf.WriteString("\n")
		}
		if len(fun.Results) > 0 {
			buf.WriteString("#### 返回值\n")
			buf.WriteString("|返回值(顺序)|返回值类型|返回值解释|\n")
			buf.WriteString("|:-----------|:---------- |:-----------|\n")
			for _, result := range fun.Results {
				buf.WriteString(fmt.Sprintf("| %s | `%s` |   |\n", html.EscapeString(result.Name), html.EscapeString(result.Type)))
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
		if p.Example != "" {
			buf.WriteString(fmt.Sprintf("<TabItem value=\"%s-2\" label=\"示例\">\n", fun.MethodName))
			buf.WriteString(fmt.Sprintf("\n\n%s\n\n", p.Example))
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

func main() {
	if len(os.Args) < 2 {
		return
	}
	basepath := os.Args[1]
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
}
