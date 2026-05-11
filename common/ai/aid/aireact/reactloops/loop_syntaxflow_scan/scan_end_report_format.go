// Human-readable scan end summaries for logs, pipeline vars, and user-facing markdown tables.
package loop_syntaxflow_scan

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
)

// SkipQueryProductHint 为对外说明中「skip」的共性原因（不暴露引擎内部名）。
const SkipQueryProductHint = "当某条规则与目标语言/工程形态不一致时，引擎会跳过该规则（不视为执行失败）。"

// FormatSyntaxFlowScanEndReport is a one-line scan-end summary for pipeline / logs.
func FormatSyntaxFlowScanEndReport(st *schema.SyntaxFlowScanTask) string {
	if st == nil {
		return ""
	}
	return fmt.Sprintf(
		"【扫描终态】 task_id=%s status=%s reason=%q programs=%s kind=%s\n"+
			"【规则/Query】 rules_count=%d total_query=%d success=%d failed=%d skip=%d\n"+
			"【Risk 分级】 total=%d critical=%d high=%d warn=%d low=%d info=%d",
		st.TaskId, st.Status, st.Reason, st.Programs, string(st.Kind),
		st.RulesCount, st.TotalQuery, st.SuccessQuery, st.FailedQuery, st.SkipQuery,
		st.RiskCount, st.CriticalCount, st.HighCount, st.WarningCount, st.LowCount, st.InfoCount,
	)
}

// FormatSyntaxFlowScanEndReportMarkdownTable 扫描结束用户向表格（主对话与 P4 输入），避免以内部 key 作主结构。
func FormatSyntaxFlowScanEndReportMarkdownTable(st *schema.SyntaxFlowScanTask) string {
	if st == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("## 扫描任务行终态（汇总）\n\n")
	b.WriteString("| 字段 | 值 |\n| --- | --- |\n")
	fmt.Fprintf(&b, "| task_id | `%s` |\n", st.TaskId)
	fmt.Fprintf(&b, "| status | `%s` |\n", st.Status)
	fmt.Fprintf(&b, "| reason | %s |\n", escapeScanTableCell(st.Reason))
	fmt.Fprintf(&b, "| programs | %s |\n", escapeScanTableCell(st.Programs))
	fmt.Fprintf(&b, "| kind | `%s` |\n", string(st.Kind))
	b.WriteString("\n### Query 与规则批次\n\n")
	b.WriteString("| 指标 | 数值 |\n| --- | ---: |\n")
	fmt.Fprintf(&b, "| rules_count（批次数/规则配置相关） | %d |\n", st.RulesCount)
	fmt.Fprintf(&b, "| total_query | %d |\n", st.TotalQuery)
	fmt.Fprintf(&b, "| success | %d |\n", st.SuccessQuery)
	fmt.Fprintf(&b, "| failed | %d |\n", st.FailedQuery)
	fmt.Fprintf(&b, "| skip | %d |\n", st.SkipQuery)
	b.WriteString("\n**skip 说明**: " + SkipQueryProductHint + "\n\n")
	b.WriteString("### 风险分级汇总\n\n")
	b.WriteString("| 级别 | 条数 |\n| --- | ---: |\n")
	fmt.Fprintf(&b, "| total | %d |\n", st.RiskCount)
	fmt.Fprintf(&b, "| critical | %d |\n", st.CriticalCount)
	fmt.Fprintf(&b, "| high | %d |\n", st.HighCount)
	fmt.Fprintf(&b, "| warning | %d |\n", st.WarningCount)
	fmt.Fprintf(&b, "| low | %d |\n", st.LowCount)
	fmt.Fprintf(&b, "| info | %d |\n", st.InfoCount)
	return b.String()
}

func escapeScanTableCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "¦")
	return s
}
