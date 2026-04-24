package loop_syntaxflow_scan

import (
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const DefaultRiskSampleLimit = 100

// ScanSessionResult aggregates a SyntaxFlow scan task and SSA risks for that runtime (task_id == runtime_id).
type ScanSessionResult struct {
	TaskID     string
	ScanTask   *schema.SyntaxFlowScanTask
	TotalRisks int
	Risks      []*schema.SSARisk
	Preface    string
}

// LoadScanSessionResult loads task row + risk count + up to riskSampleLimit risks for AI preface.
func LoadScanSessionResult(db *gorm.DB, taskID string, riskSampleLimit int) (*ScanSessionResult, error) {
	if db == nil {
		return nil, utils.Error("nil db")
	}
	if taskID == "" {
		return nil, utils.Error("empty task_id")
	}
	if riskSampleLimit <= 0 {
		riskSampleLimit = DefaultRiskSampleLimit
	}

	_, tasks, err := yakit.QuerySyntaxFlowScanTask(db, &ypb.QuerySyntaxFlowScanTaskRequest{
		Filter: &ypb.SyntaxFlowScanTaskFilter{
			TaskIds: []string{taskID},
		},
		Pagination: &ypb.Paging{Page: 1, Limit: 1, OrderBy: "id", Order: "desc"},
	})
	if err != nil || len(tasks) == 0 {
		return nil, utils.Errorf("syntaxflow scan task not found: %v", err)
	}
	st := tasks[0]

	riskFilter := &ypb.SSARisksFilter{
		RuntimeID: []string{taskID},
	}
	totalRisks, err := yakit.QuerySSARiskCount(db, riskFilter)
	if err != nil {
		totalRisks = 0
	}

	paging := &ypb.Paging{Page: 1, Limit: int64(riskSampleLimit), OrderBy: "id", Order: "desc"}
	_, risks, qerr := yakit.QuerySSARisk(db, riskFilter, paging)
	if qerr != nil {
		risks = nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("SyntaxFlowScanTask: task_id=%s programs=%s status=%s kind=%s risk_count=%d rules=%d\n",
		st.TaskId, st.Programs, st.Status, string(st.Kind), st.RiskCount, st.RulesCount))
	sb.WriteString(FormatScanTaskProgressLine(st))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("SSA risks for this runtime (total=%d), showing up to %d:\n", totalRisks, len(risks)))
	for i, rk := range risks {
		sb.WriteString(fmt.Sprintf("%d. risk id=%d sev=%s rule=%s title=%s\n",
			i+1, rk.ID, rk.Severity, utils.ShrinkTextBlock(rk.FromRule, 64), utils.ShrinkTextBlock(rk.Title, 100)))
	}

	return &ScanSessionResult{
		TaskID:     taskID,
		ScanTask:   st,
		TotalRisks: totalRisks,
		Risks:      risks,
		Preface:    sb.String(),
	}, nil
}

// FormatScanTaskProgressLine summarizes query progress from the task row.
func FormatScanTaskProgressLine(st *schema.SyntaxFlowScanTask) string {
	if st == nil {
		return ""
	}
	return fmt.Sprintf("Progress: total_query=%d success=%d failed=%d skip=%d (status=%s)\n",
		st.TotalQuery, st.SuccessQuery, st.FailedQuery, st.SkipQuery, st.Status)
}

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
