package loop_report_generating

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Bound inline Markdown rendering independently of the complete saved artifact
// and GEN_REPORT parsing. Either limit can trigger a preview; the notice counts
// toward both limits. These are initial display budgets, not file size limits.
const (
	maxReportDisplayBytes = 64 * 1024
	maxReportDisplayLines = 1000
	maxReportTitleBytes   = 256
)

func reportMarkdownLines(content string) int {
	if content == "" {
		return 0
	}
	lines := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") {
		lines++
	}
	return lines
}

func reportDisplayMarkdown(content string) string {
	lines := reportMarkdownLines(content)
	if len(content) <= maxReportDisplayBytes && lines <= maxReportDisplayLines {
		return content
	}
	notice := fmt.Sprintf(
		"> 内容较长（%d 字节，%d 行），以下仅展示开头预览。通过下方「查看文献」或文件系统查看完整报告。\n\n---\n\n",
		len(content), lines,
	)
	byteBudget := maxReportDisplayBytes - len(notice)
	lineBudget := maxReportDisplayLines - reportMarkdownLines(notice)
	end, displayedLines := 0, 0
	fenceStart, fenceLength := -1, 0
	var fenceMarker byte
	for end < len(content) && displayedLines < lineBudget {
		next := len(content)
		if newline := strings.IndexByte(content[end:], '\n'); newline >= 0 {
			next = end + newline + 1
		}
		if next > byteBudget {
			break // Never slice UTF-8, a long line, or a Markdown table row.
		}
		marker, length, rest := reportLineFence(content[end:next])
		if fenceStart < 0 && length >= 3 {
			fenceStart, fenceMarker, fenceLength = end, marker, length
		} else if fenceStart >= 0 && marker == fenceMarker && length >= fenceLength && rest == "" {
			fenceStart = -1
		}
		end = next
		displayedLines++
	}
	// Omit an unfinished fenced block instead of rendering truncated code or
	// Mermaid. Do not join the report's head and tail or fabricate a closing fence.
	if fenceStart >= 0 {
		end = fenceStart
	}
	preview := strings.TrimRight(content[:end], "\r\n")
	if strings.TrimSpace(preview) == "" {
		return fmt.Sprintf(
			"> 内容较长（%d 字节，%d 行），开头段落或代码块超过聊天展示上限。通过下方「查看文献」或文件系统查看完整报告。\n",
			len(content), lines,
		)
	}
	return notice + preview + "\n"
}

// Recognize top-level CommonMark fences (up to three leading spaces). A closing
// fence must use the opening marker, be at least as long, and have no info text.
func reportLineFence(line string) (marker byte, length int, rest string) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return 0, 0, ""
	}
	trimmed = strings.TrimRight(trimmed, "\r\n \t")
	if len(trimmed) < 3 || (trimmed[0] != '`' && trimmed[0] != '~') {
		return 0, 0, ""
	}
	marker = trimmed[0]
	for length < len(trimmed) && trimmed[length] == marker {
		length++
	}
	rest = strings.TrimSpace(trimmed[length:])
	if marker == '`' && strings.ContainsRune(rest, '`') {
		return 0, 0, "" // Backtick opening fences cannot contain backticks in the info string.
	}
	return marker, length, rest
}

func reportDisplayTitle(title string) string {
	if len(title) <= maxReportTitleBytes {
		return title
	}
	end := maxReportTitleBytes - len("…")
	for end > 0 && !utf8.RuneStart(title[end]) {
		end--
	}
	return title[:end] + "…"
}
