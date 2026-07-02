package yakgrpc

import (
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// 关键词: EXAMPLE 标记渲染, 动态反引号围栏, Markdown 展示
//
// 标准库文档注释里用 <|EXAMPLE_START|> ... <|EXAMPLE_END|> 包裹可运行示例
// (见 common/yak/yakdoc/webdoc/render_examples.go)。这些标记原样出现在
// 补全/悬浮/签名的文本里发给前端时，会直接显示成裸标记文本，Markdown 展示很难看。
//
// 这里在"发送给用户前的最后一刻"把每个 EXAMPLE 块替换成一段代码围栏：
//   - 用 N 个反引号作为围栏，N 取块内代码里最长连续反引号数 +1(且不小于 3)，
//     保证示例内部即便含有 ``` 也能被"完美包裹"而不破坏代码块；
//   - 标记行后的标题渲染为加粗小节标签；
//   - 去掉示例内部作者已写的 ``` 围栏与单独成行的 "..." 省略占位，避免重复围栏。

const (
	exampleStartMarkerToken = "<|EXAMPLE_START|>"
	exampleEndMarkerToken   = "<|EXAMPLE_END|>"
)

// stripLineCommentPrefix 去掉一行可能存在的前导 "//"，兼容文档仍保留注释前缀的情况。
func stripLineCommentPrefix(line string) string {
	t := strings.TrimSpace(line)
	t = strings.TrimPrefix(t, "//")
	return strings.TrimSpace(t)
}

// maxBacktickRun 返回字符串中最长的一段连续反引号的数量。
func maxBacktickRun(s string) int {
	maxRun, cur := 0, 0
	for _, r := range s {
		if r == '`' {
			cur++
			if cur > maxRun {
				maxRun = cur
			}
		} else {
			cur = 0
		}
	}
	return maxRun
}

// dedentLines 去除多行文本的公共前导空白(空格/制表符)。
func dedentLines(lines []string) []string {
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
		return lines
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if len(l) >= minIndent {
			out[i] = l[minIndent:]
		} else {
			out[i] = l
		}
	}
	return out
}

// cleanExampleCode 把 EXAMPLE 块内的原始行清洗成可直接被围栏包裹的纯代码：
// 去掉作者已写的 ``` 围栏行、去掉单独成行的 "..." 省略占位、去公共缩进与首尾空白。
func cleanExampleCode(lines []string) string {
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		t := stripLineCommentPrefix(line)
		if strings.HasPrefix(t, "```") {
			continue
		}
		if t == "..." || t == "…" {
			continue
		}
		kept = append(kept, strings.TrimPrefix(strings.TrimSpace(line), "// "))
	}
	kept = dedentLines(kept)
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

// fenceExampleCode 用 N 个反引号(N 依据代码内反引号情况动态计算)包裹示例代码。
func fenceExampleCode(code string) string {
	n := maxBacktickRun(code) + 1
	if n < 3 {
		n = 3
	}
	fence := strings.Repeat("`", n)
	return fence + "yak\n" + code + "\n" + fence
}

// RenderExampleMarkersForMarkdown 把文本中所有 <|EXAMPLE_START|>...<|EXAMPLE_END|>
// 块替换为带标题的 Markdown 代码围栏。文本中不含标记时原样返回。
// 该函数应在把文本发送给前端展示前的最后一刻调用。
func RenderExampleMarkersForMarkdown(text string) string {
	if !strings.Contains(text, exampleStartMarkerToken) {
		return text
	}

	lines := strings.Split(text, "\n")
	var out []string
	var block []string
	var title string
	inBlock := false
	exampleIndex := 0
	multiExample := strings.Count(text, exampleStartMarkerToken) > 1

	flush := func() {
		code := cleanExampleCode(block)
		block = nil
		if code == "" {
			title = ""
			return
		}
		exampleIndex++
		label := "示例"
		if title != "" {
			label = "示例：" + title
		} else if multiExample {
			label = "示例 " + strconv.Itoa(exampleIndex)
		}
		out = append(out, "**"+label+"**", "", fenceExampleCode(code), "")
		title = ""
	}

	for _, line := range lines {
		marker := stripLineCommentPrefix(line)
		if !inBlock {
			if idx := strings.Index(marker, exampleStartMarkerToken); idx >= 0 {
				inBlock = true
				title = strings.TrimSpace(marker[idx+len(exampleStartMarkerToken):])
				block = nil
				continue
			}
			out = append(out, line)
			continue
		}
		if strings.Contains(marker, exampleEndMarkerToken) {
			flush()
			inBlock = false
			continue
		}
		block = append(block, line)
	}
	// 容错：未闭合的块也收尾，避免把残留标记漏给前端
	if inBlock {
		flush()
	}

	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}

// applyExampleFenceToResponse 对返回给前端的建议逐条渲染 EXAMPLE 标记。
// Label(悬浮内容) 与 Description(补全/签名文档) 都可能内嵌标记，均需处理。
func applyExampleFenceToResponse(resp *ypb.YaklangLanguageSuggestionResponse) *ypb.YaklangLanguageSuggestionResponse {
	if resp == nil {
		return resp
	}
	for _, item := range resp.SuggestionMessage {
		if item == nil {
			continue
		}
		item.Label = RenderExampleMarkersForMarkdown(item.Label)
		item.Description = RenderExampleMarkersForMarkdown(item.Description)
	}
	return resp
}
