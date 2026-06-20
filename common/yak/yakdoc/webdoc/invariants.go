package webdoc

import (
	"fmt"
	"regexp"
	"strings"
)

// 关键词: CheckMarkdownInvariants, Markdown 健壮性守卫, 锚点完整性, 围栏配对, 表格列一致
//
// CheckMarkdownInvariants 对一段(由本包生成的)Markdown 做结构不变量校验，复刻并守住
// 那些会让文档站 MDX 构建崩溃或渲染错乱的失败模式。仅用于纯 Markdown(.md)，不适用 MDX。
// 校验项:
//   1. 代码围栏成对(``` 行数为偶数,无未闭合)。
//   2. 仅一个 H1;无超过 3 级的标题。
//   3. 显式 heading id 无重复;每个 (#id) 链接都有对应的 heading id(锚点完整)。
//   4. 同一张表格各行的列数一致(忽略行内代码与转义管道)。
//   5. 非代码处无未中和的裸 URL(避免 gfm autolink 崩溃)。
//   6. 非代码处无裸 '<'(避免被 MDX 当作 JSX)。

var (
	mdHeadingRe = regexp.MustCompile(`^(#{1,6})\s+`)
	mdIDRe      = regexp.MustCompile(`\{#([A-Za-z0-9_-]+)\}`)
	mdLinkRe    = regexp.MustCompile(`\]\(#([A-Za-z0-9_-]+)\)`)
	mdURLRe     = regexp.MustCompile(`https?://`)
)

type linkRef struct {
	id   string
	line int
}

// stripInlineCode 去掉一行里的行内代码 `...`(连同反引号),用于在不被代码内容干扰的前提下
// 统计表格列、检测裸 URL 与裸 '<'。注意:仅处理单反引号行内代码,围栏代码块在调用方按行跳过。
func stripInlineCode(line string) string {
	var b strings.Builder
	inCode := false
	for _, r := range line {
		if r == '`' {
			inCode = !inCode
			continue
		}
		if inCode {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// countTableColumns 统计去除行内代码后一行里的有效管道数(跳过被反斜杠转义的 \|)。
func countTableColumns(stripped string) int {
	cols := 0
	runes := []rune(stripped)
	for i, r := range runes {
		if r == '|' {
			if i > 0 && runes[i-1] == '\\' {
				continue
			}
			cols++
		}
	}
	return cols
}

// CheckMarkdownInvariants 校验 Markdown 结构不变量,违例时返回聚合后的错误(便于测试断言)。
func CheckMarkdownInvariants(md string) error {
	lines := strings.Split(md, "\n")
	var violations []string
	add := func(format string, args ...interface{}) {
		violations = append(violations, fmt.Sprintf(format, args...))
	}

	fenceToggles := 0
	exampleFenceLines := 0
	inFence := false
	h1 := 0
	explicitIDs := map[string]int{}
	var links []linkRef
	tableCols := -1

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			fenceToggles++
			if strings.HasPrefix(trimmed, exampleFence) {
				exampleFenceLines++
			}
			inFence = !inFence
			tableCols = -1
			continue
		}
		if inFence {
			continue
		}

		if m := mdHeadingRe.FindStringSubmatch(line); m != nil {
			level := len(m[1])
			if level == 1 {
				h1++
			}
			if level > 3 {
				add("line %d: heading level %d exceeds 3: %q", idx+1, level, trimmed)
			}
			for _, im := range mdIDRe.FindAllStringSubmatch(line, -1) {
				explicitIDs[im[1]]++
			}
		}

		stripped := stripInlineCode(line)

		for _, lm := range mdLinkRe.FindAllStringSubmatch(stripped, -1) {
			links = append(links, linkRef{id: lm[1], line: idx + 1})
		}

		if strings.Contains(stripped, "<") {
			add("line %d: raw '<' outside code (MDX/JSX risk): %q", idx+1, trimmed)
		}

		for _, loc := range mdURLRe.FindAllStringIndex(stripped, -1) {
			start := loc[0]
			if !(start >= 2 && stripped[start-2:start] == "](") {
				add("line %d: bare URL not neutralized (autolink crash risk): %q", idx+1, trimmed)
				break
			}
		}

		if strings.HasPrefix(trimmed, "|") {
			cols := countTableColumns(stripInlineCode(trimmed))
			if tableCols == -1 {
				tableCols = cols
			} else if cols != tableCols {
				add("line %d: table column mismatch (got %d want %d): %q", idx+1, cols, tableCols, trimmed)
			}
		} else {
			tableCols = -1
		}
	}

	if inFence || fenceToggles%2 != 0 {
		add("unbalanced code fence: %d fence lines", fenceToggles)
	}
	if exampleFenceLines%2 != 0 {
		add("unbalanced 14-backtick example fence: %d fence lines", exampleFenceLines)
	}
	if h1 != 1 {
		add("expected exactly 1 H1, got %d", h1)
	}
	for id, n := range explicitIDs {
		if n > 1 {
			add("duplicate heading id #%s (%d times)", id, n)
		}
	}
	for _, lt := range links {
		if explicitIDs[lt.id] == 0 {
			add("line %d: link target #%s has no matching heading id", lt.line, lt.id)
		}
	}

	if len(violations) > 0 {
		return fmt.Errorf("markdown invariants failed:\n  - %s", strings.Join(violations, "\n  - "))
	}
	return nil
}
