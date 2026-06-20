package webdoc

import (
	"html"
	"strconv"
	"strings"
)

// 关键词: 多 example 解析, EXAMPLE_START/END 标记, 示例分类
//
// 一个函数的 doc 注释可以包含多个示例，每个示例用如下标记包裹(标题可选)：
//
//	<|EXAMPLE_START|> 基础用法
//	... yak 代码 ...
//	<|EXAMPLE_END|>
//
// 标记行前面可带 "//"(若 doc 仍含注释前缀)；标题为 START 标记之后的剩余文本。
// 为向后兼容，若注释里没有任何 EXAMPLE_START 标记，则回退到旧的单 "Example:" 段。

const (
	exampleStartMarker = "<|EXAMPLE_START|>"
	exampleEndMarker   = "<|EXAMPLE_END|>"
)

// DocExample 表示从注释里解析出的一个示例：标题(可空) + 纯代码。
type DocExample struct {
	Title string
	Code  string
}

// cleanExampleBlock 把示例原始行清洗为可直接包裹的纯代码：去掉已有的 ``` 围栏行、
// 去掉单独成行的 "..." 省略占位(作者用来表示"此处省略代码"，并非合法 yak)、
// 去公共缩进、去首尾空白。
// 关键词: 示例清洗, 去围栏, 去省略号占位
func cleanExampleBlock(lines []string) string {
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			continue
		}
		if t == "..." || t == "…" {
			continue // 省略占位行，丢弃以免破坏语法
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(dedent(strings.Join(kept, "\n")))
}

// stripCommentPrefix 去掉一行可能存在的前导 "//"，用于在 doc 仍保留注释前缀时也能识别标记。
func stripCommentPrefix(line string) string {
	t := strings.TrimSpace(line)
	t = strings.TrimPrefix(t, "//")
	return strings.TrimSpace(t)
}

// extractExamples 解析注释中的全部示例。优先使用 EXAMPLE_START/END 标记(支持多个、带标题)；
// 若没有标记则回退到旧的单 Example: 段(无标题)。
// 关键词: extractExamples, 多示例, 标题
func extractExamples(doc string) []DocExample {
	lines := strings.Split(doc, "\n")
	var examples []DocExample
	var cur []string
	var curTitle string
	inBlock := false
	found := false

	flush := func() {
		if code := cleanExampleBlock(cur); code != "" {
			examples = append(examples, DocExample{Title: curTitle, Code: code})
		}
		cur = nil
		curTitle = ""
	}

	for _, line := range lines {
		marker := stripCommentPrefix(line)
		if !inBlock {
			if idx := strings.Index(marker, exampleStartMarker); idx >= 0 {
				found = true
				inBlock = true
				curTitle = strings.TrimSpace(marker[idx+len(exampleStartMarker):])
				cur = nil
			}
			continue
		}
		if strings.Contains(marker, exampleEndMarker) {
			flush()
			inBlock = false
			continue
		}
		cur = append(cur, line)
	}
	// 容错：未闭合的 block 也收尾，避免丢示例
	if inBlock {
		flush()
	}

	if found {
		return examples
	}
	// 回退：旧单 Example: 段
	if code := extractExampleCode(doc); code != "" {
		return []DocExample{{Title: "", Code: code}}
	}
	return nil
}

// renderExamples 把解析出的示例渲染为带标题的多段 14 反引号 yak 围栏。
// 无标题：单个示例用 "示例"，多个用 "示例 1/2/..."；有标题用 "示例：<标题>"。
// 关键词: renderExamples, 14 反引号围栏, 示例标题
func renderExamples(examples []DocExample, formatExample func(string) string) string {
	if len(examples) == 0 {
		return ""
	}
	if formatExample == nil {
		formatExample = func(s string) string { return s }
	}
	var b strings.Builder
	for i, ex := range examples {
		label := "示例"
		if ex.Title != "" {
			label = "示例：" + ex.Title
		} else if len(examples) > 1 {
			label = "示例 " + strconv.Itoa(i+1)
		}
		b.WriteString("**" + escapeInlineLabel(label) + "**\n\n")
		b.WriteString(fenceExampleYak(formatExample(ex.Code)) + "\n\n")
	}
	return b.String()
}

// escapeInlineLabel 转义加粗小节标签里的文本(标题来自注释，可能含 < & 等)，避免 MDX 把
// < 当作 JSX；同时中和裸 URL。
func escapeInlineLabel(s string) string {
	return neutralizeBareURLAutolinks(html.EscapeString(s))
}
