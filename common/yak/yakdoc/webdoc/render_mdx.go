package webdoc

import (
	"fmt"
	"html"
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
)

// RenderLibMDX 把一个库渲染为 MDX(带 Tabs 的富交互页),目前仅 ai 库使用。
// 复用与 Markdown 路径相同的解析/示例/锚点逻辑;描述使用解析后的结构化字段(天然去重)。
// 注意:MDX 含 JSX(<Tabs>),不适用 CheckMarkdownInvariants,其健壮性由文档站真实构建保证。
// 关键词: RenderLibMDX, Tabs, ai 库
func RenderLibMDX(lib *yakdoc.ScriptLib, description string, formatExample func(string) string) string {
	if formatExample == nil {
		formatExample = func(s string) string { return s }
	}
	var b strings.Builder

	b.WriteString("---\n")
	b.WriteString("sidebar_label: " + lib.Name + "\n")
	b.WriteString("slug: /api/" + lib.Name + "\n")
	b.WriteString("title: " + lib.Name + "\n")
	b.WriteString("description: " + description + "\n")
	b.WriteString("---\n")

	b.WriteString("import Tabs from '@theme/Tabs';\n")
	b.WriteString("import TabItem from '@theme/TabItem';\n")
	b.WriteString("import CodeBlock from '@theme/CodeBlock';\n\n")

	b.WriteString(":::info\n" + description + "\n:::\n\n")

	funcList := sortedFuncs(lib)
	anchors := assignAnchors(funcList)

	b.WriteString("## 函数索引\n\n")
	b.WriteString("|函数名|函数描述/介绍|\n")
	b.WriteString("|:------|:--------|\n")
	for _, fun := range funcList {
		parsed := parseCommentDetails(fun.Document)
		b.WriteString(fmt.Sprintf("| [%s.%s](#%s) | %s |\n",
			html.EscapeString(fun.LibName),
			html.EscapeString(fun.MethodName),
			anchors[fun],
			escapeTableCell(parsed.Description),
		))
	}
	b.WriteString("\n\n")

	if len(lib.Instances) > 0 {
		b.WriteString("## 实例索引\n\n")
		b.WriteString("|实例名|类型|说明|\n")
		b.WriteString("|:------|:------|:------|\n")
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

	b.WriteString("## API 详情\n\n")
	for _, fun := range funcList {
		p := parseCommentDetails(fun.Document)

		b.WriteString(fmt.Sprintf("### %s {#%s}\n\n", html.EscapeString(fun.MethodName), anchors[fun]))
		if desc := strings.TrimSpace(p.Description); desc != "" {
			b.WriteString(fmt.Sprintf("- 描述: %s\n\n", escapeTableCell(desc)))
		}
		if p.LongDescription != "" {
			b.WriteString(fmt.Sprintf("- 详细描述: %s\n\n", escapeTableCell(p.LongDescription)))
		}

		b.WriteString("\n<Tabs>\n")
		b.WriteString(fmt.Sprintf("<TabItem value=\"%s-1\" label=\"定义\" default>\n\n", fun.MethodName))
		// 围栏内原样输出签名,不做 HTML 转义(否则 < & 会显示成实体字面量)。
		b.WriteString(fmt.Sprintf("```go\n%s\n```\n\n", fun.Decl))

		if len(fun.Params) > 0 {
			b.WriteString("**参数配置信息**\n")
			b.WriteString("\n|参数名|参数类型|参数解释|\n")
			b.WriteString("|:-----------|:---------- |:-----------|\n")
			for _, param := range fun.Params {
				b.WriteString(fmt.Sprintf("| %s | `%s` | %s |\n",
					html.EscapeString(param.Name), param.Type, escapeTableCell(p.Params[param.Name])))
			}
			b.WriteString("\n")
		}
		if len(fun.Results) > 0 {
			b.WriteString("**返回值**\n")
			b.WriteString("\n|返回值(顺序)|返回值类型|返回值解释|\n")
			b.WriteString("|:-----------|:---------- |:-----------|\n")
			for i, result := range fun.Results {
				explanation := ""
				if i < len(p.Returns) {
					explanation = p.Returns[i]
				}
				b.WriteString(fmt.Sprintf("| %s | `%s` | %s |\n",
					html.EscapeString(result.Name), result.Type, escapeTableCell(explanation)))
			}
			b.WriteString("\n")
		}
		b.WriteString("</TabItem>\n")
		if exampleCode := extractExampleCode(fun.Document); exampleCode != "" {
			b.WriteString(fmt.Sprintf("<TabItem value=\"%s-2\" label=\"示例\">\n", fun.MethodName))
			b.WriteString(fmt.Sprintf("\n\n%s\n\n", fenceExampleYak(formatExample(exampleCode))))
			b.WriteString("</TabItem>\n")
		}
		b.WriteString("</Tabs>\n\n")
		b.WriteString("\n---\n\n")
	}

	return b.String()
}
