package loop_fast_context

import (
	"fmt"
	"strings"
)

// LocationHit is one structured code location returned to the caller.
// Line ranges are 1-based inclusive; EndLine 0 means unknown / whole file.
type LocationHit struct {
	Path       string `json:"path"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	Reason     string `json:"reason,omitempty"`
	Confidence string `json:"confidence,omitempty"` // high | medium | low
}

// ExplorationReport is the fixed deliverable surfaced to callers and the frontend.
type ExplorationReport struct {
	Query       string        `json:"query"`
	Summary     string        `json:"summary"` // <= 50 words, human-readable
	Locations   []LocationHit `json:"locations"`
	SearchStats SearchStats   `json:"search_stats"`
}

// SearchStats gives users a compact view of what happened inside the subagent.
type SearchStats struct {
	Rounds      int `json:"rounds"`
	ToolCalls   int `json:"tool_calls"`
	UniqueFiles int `json:"unique_files"`
}

// FormatUserMarkdown renders a human-readable card for the frontend.
func (r *ExplorationReport) FormatUserMarkdown() string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("## FastContext 探索结果\n\n")
	if q := strings.TrimSpace(r.Query); q != "" {
		b.WriteString("**查询**：")
		b.WriteString(q)
		b.WriteString("\n\n")
	}
	if s := strings.TrimSpace(r.Summary); s != "" {
		b.WriteString("**结论**：")
		b.WriteString(s)
		b.WriteString("\n\n")
	}
	b.WriteString(fmt.Sprintf("**统计**：%d 轮搜索 · %d 次工具调用 · %d 个候选文件\n\n",
		r.SearchStats.Rounds, r.SearchStats.ToolCalls, r.SearchStats.UniqueFiles))

	if len(r.Locations) == 0 {
		b.WriteString("未定位到相关代码位置。\n")
		return b.String()
	}

	b.WriteString("### 代码位置\n\n")
	b.WriteString("| 文件 | 行号 | 说明 |\n")
	b.WriteString("|------|------|------|\n")
	for _, loc := range r.Locations {
		lineRange := formatLineRange(loc.StartLine, loc.EndLine)
		reason := strings.TrimSpace(loc.Reason)
		if reason == "" {
			reason = "—"
		}
		reason = strings.ReplaceAll(reason, "|", "\\|")
		b.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", loc.Path, lineRange, reason))
	}
	return b.String()
}

func formatLineRange(start, end int) string {
	if start <= 0 {
		return "—"
	}
	if end <= 0 || end == start {
		return fmt.Sprintf("%d", start)
	}
	if end < start {
		end = start
	}
	return fmt.Sprintf("%d-%d", start, end)
}
