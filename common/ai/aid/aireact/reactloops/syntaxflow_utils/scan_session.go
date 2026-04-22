package syntaxflow_utils

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
