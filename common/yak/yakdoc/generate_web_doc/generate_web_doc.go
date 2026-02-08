package main

import (
	"fmt"
	"html"
	"os"
	"path"
	"sort"
	"strings"
	"regexp"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

// YakDocParsed 用于存储解析后的注释字段
type YakDocParsed struct {
	Description     string
	LongDescription string
	Category	    string			  // 分类
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

// 辅助函数：解析注释中的参数和返回值描述
func parseCommentDetails(doc string) *YakDocParsed {
	parsed := &YakDocParsed{
		Params: make(map[string]string),
	}
	lines := strings.Split(doc, "\n")

	var currentTag string
	var longDescLines []string
	var exampleLines []string

	// 正则匹配: - name(type): description
	listRegex := regexp.MustCompile(`^\s*-\s*([\w.]+)\s*(?:\(.*\))?\s*[:：]\s*(.*)$`)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lowerLine := strings.ToLower(trimmed)

		// 检测标签切换
		if strings.HasPrefix(lowerLine, "description:") {
			currentTag = "description"
			parsed.Description = strings.TrimSpace(strings.TrimPrefix(trimmed, "description:"))
			continue
		} else if strings.HasPrefix(lowerLine, "long_description:") {
			currentTag = "long_description"
			firstLine := strings.TrimSpace(strings.TrimPrefix(trimmed, "long_description:"))
			if firstLine != "" {
				longDescLines = append(longDescLines, firstLine)
			}
			continue
		} else if strings.HasPrefix(lowerLine, "category:") {
			currentTag = "category"
			parsed.Category = strings.TrimSpace(strings.TrimPrefix(trimmed, "category:"))
			continue
		}else if strings.HasPrefix(lowerLine, "parameters:") {
			currentTag = "parameters"
			continue
		} else if strings.HasPrefix(lowerLine, "returns:") {
			currentTag = "returns"
			continue
		} else if strings.HasPrefix(lowerLine, "example:") {
			currentTag = "example"
			continue
		}

		// 处理内容
		switch currentTag {
			case "long_description":
				// 只有非空行才加入，避免产生过多的空格
				if trimmed != "" {
					// 去除换行符的关键：直接存入 trimmed 后的文本
					longDescLines = append(longDescLines, trimmed)
				}
			case "parameters":
				matches := listRegex.FindStringSubmatch(line)
				if len(matches) > 2 {
					parsed.Params[matches[1]] = matches[2]
				}
			case "returns":
				matches := listRegex.FindStringSubmatch(line)
				if len(matches) > 2 {
					parsed.Returns = append(parsed.Returns, matches[2])
				} else if trimmed != "" && strings.HasPrefix(trimmed, "-") {
					parsed.Returns = append(parsed.Returns, strings.TrimPrefix(trimmed, "-"))
				}
			case "example":
				// Example 块通常需要保留换行符以维持代码格式，所以这里不去除换行
				exampleLines = append(exampleLines, line)
			}
	}

	// 合并 LongDescription：用空格连接各行，并处理可能出现的连续空格
	rawLongDesc := strings.Join(longDescLines, " ")
	parsed.LongDescription = strings.Join(strings.Fields(rawLongDesc), "")
	
	parsed.Example = strings.Join(exampleLines, "\n")
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

	groups := lo.GroupBy(funcList, func(item FuncInfo) string {
		if item.Parsed.Category != "" {
			return item.Parsed.Category
		}
		return "未分类函数"
	})
	existingCategories := lo.Keys(groups)

	file.WriteString("## 函数索引\n\n")

	for _, cat := range existingCategories {
		funcs := groups[cat]
		// 组内按方法名排序
		sort.SliceStable(funcs, func(i, j int) bool {
			return funcs[i].Decl.MethodName < funcs[j].Decl.MethodName
		})

		file.WriteString(fmt.Sprintf("### %s\n\n", cat))
		file.WriteString("|函数名|函数描述/介绍|\n")
		file.WriteString("|:------|:--------|\n")
		for _, f := range funcs {
			lowerMethodName := strings.ToLower(f.Decl.MethodName)
			file.WriteString(fmt.Sprintf("| [%s.%s](#%s) | %s |\n",
				html.EscapeString(f.Decl.LibName),
				html.EscapeString(f.Decl.MethodName),
				html.EscapeString(lowerMethodName),
				html.EscapeString(f.Parsed.Description),
			))
		}
		file.WriteString("\n")
	}

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

	bufList := make([]strings.Builder, 0, len(funcList))

	for _, cat := range existingCategories  {
		funcs := groups[cat]
		buf1 := strings.Builder{}
		buf1.WriteString(fmt.Sprintf("## %s\n\n", cat))
		bufList = append(bufList, buf1)

		for _, f := range funcs {
			fun := f.Decl
			p := f.Parsed
			buf := strings.Builder{}

			buf.WriteString(fmt.Sprintf("### %s\n\n", fun.MethodName))
			buf.WriteString(fmt.Sprintf("- 描述 %s\n\n", p.Description))
			if p.LongDescription != "" {
				buf.WriteString(fmt.Sprintf("- 详细描述 %s\n\n", p.LongDescription))
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