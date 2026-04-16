package syntaxflow

import (
	"fmt"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func escapeProgramsLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// QuerySyntaxFlowScanTasksByProgramsContains queries scan tasks whose programs field contains programHint.
func QuerySyntaxFlowScanTasksByProgramsContains(programHint string, scanOnly bool, limit int) ([]*schema.SyntaxFlowScanTask, error) {
	db := consts.GetGormSSAProjectDataBase()
	if db == nil {
		return nil, utils.Errorf("ssa project database is nil")
	}
	hint := strings.TrimSpace(programHint)
	if hint == "" {
		return nil, utils.Errorf("empty program hint")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	like := "%" + escapeProgramsLike(hint) + "%"
	q := db.Model(&schema.SyntaxFlowScanTask{}).
		Where("programs LIKE ?", like).
		Order("updated_at DESC").
		Limit(limit)
	if scanOnly {
		q = q.Where("kind = ?", schema.SFResultKindScan)
	}

	var tasks []*schema.SyntaxFlowScanTask
	if err := q.Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

// SyntaxFlowProjectScanCheckResult is a high-level result for project scan check.
type SyntaxFlowProjectScanCheckResult struct {
	ProgramHint    string                       `json:"program_hint"`
	ScanOnly       bool                         `json:"scan_only"`
	Limit          int                          `json:"limit"`
	LatestTaskID   string                       `json:"latest_task_id"`
	Tasks          []*schema.SyntaxFlowScanTask `json:"tasks"`
	ReportMarkdown string                       `json:"report_markdown"`
}

// RunSyntaxFlowProjectScanCheck performs one query and returns both report and latest task id.
func RunSyntaxFlowProjectScanCheck(programHint string, scanOnly bool, limit int) (*SyntaxFlowProjectScanCheckResult, error) {
	hint := strings.TrimSpace(programHint)
	if hint == "" {
		return nil, utils.Errorf("empty program hint")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	tasks, err := QuerySyntaxFlowScanTasksByProgramsContains(hint, scanOnly, limit)
	if err != nil {
		return nil, err
	}

	latestTaskID := ""
	if len(tasks) > 0 {
		latestTaskID = strings.TrimSpace(tasks[0].TaskId)
	}
	report := buildScanProjectCheckReportMarkdown(hint, scanOnly, tasks)

	return &SyntaxFlowProjectScanCheckResult{
		ProgramHint:    hint,
		ScanOnly:       scanOnly,
		Limit:          limit,
		LatestTaskID:   latestTaskID,
		Tasks:          tasks,
		ReportMarkdown: report,
	}, nil
}

// SyntaxFlowScanProjectCheckReport returns markdown report for project scan check.
func SyntaxFlowScanProjectCheckReport(programHint string, scanOnly bool, limit int) (string, error) {
	res, err := RunSyntaxFlowProjectScanCheck(programHint, scanOnly, limit)
	if err != nil {
		return "", err
	}
	return res.ReportMarkdown, nil
}

func buildScanProjectCheckReportMarkdown(programHint string, scanOnly bool, tasks []*schema.SyntaxFlowScanTask) string {
	var b strings.Builder
	b.WriteString("# SyntaxFlow Project Scan Check\n\n")
	b.WriteString(fmt.Sprintf("- **programs LIKE**: `%s`\n", strings.TrimSpace(programHint)))
	b.WriteString(fmt.Sprintf("- **scan_only (kind=scan)**: %v\n", scanOnly))
	b.WriteString(fmt.Sprintf("- **matched rows**: %d\n\n", len(tasks)))
	if len(tasks) == 0 {
		b.WriteString("No matched scan task found. Please verify project/repo slug in `syntax_flow_scan_tasks.programs` or run a global latest-scan check.\n")
		return b.String()
	}

	b.WriteString("Rows are ordered by **updated_at DESC**, first row is usually the latest scan for this project.\n\n")
	b.WriteString("| task_id | kind | status | programs | updated_at | risks |\n")
	b.WriteString("|---------|------|--------|----------|------------|-------|\n")
	for _, t := range tasks {
		up := ""
		if !t.UpdatedAt.IsZero() {
			up = t.UpdatedAt.Format(time.RFC3339)
		}
		pro := strings.ReplaceAll(strings.TrimSpace(t.Programs), "|", "/")
		if len(pro) > 80 {
			pro = pro[:77] + "..."
		}
		b.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s | %d |\n",
			t.TaskId, t.Kind, t.Status, pro, up, t.RiskCount))
	}
	b.WriteString("\n**Suggestion**: use `scan_id=<task_id>` from the first row, or provide a specific task_id explicitly.\n")
	return b.String()
}

// PickLatestSyntaxFlowScanTaskIDByProgramsContains picks latest scan task id matching programs contains hint.
func PickLatestSyntaxFlowScanTaskIDByProgramsContains(programHint string) (string, error) {
	db := consts.GetGormSSAProjectDataBase()
	if db == nil {
		return "", utils.Errorf("ssa project database is nil")
	}
	hint := strings.TrimSpace(programHint)
	if hint == "" {
		return "", utils.Errorf("empty program hint")
	}
	like := "%" + escapeProgramsLike(hint) + "%"

	var t schema.SyntaxFlowScanTask
	q := db.Model(&schema.SyntaxFlowScanTask{}).
		Where("kind = ?", schema.SFResultKindScan).
		Where("programs LIKE ?", like).
		Order("updated_at DESC")
	if err := q.First(&t).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			var t2 schema.SyntaxFlowScanTask
			q2 := db.Model(&schema.SyntaxFlowScanTask{}).
				Where("programs LIKE ?", like).
				Order("updated_at DESC")
			if err2 := q2.First(&t2).Error; err2 != nil {
				return "", err2
			}
			return strings.TrimSpace(t2.TaskId), nil
		}
		return "", err
	}
	return strings.TrimSpace(t.TaskId), nil
}

// PickLatestSyntaxFlowScanTaskID picks latest syntaxflow scan task id globally.
func PickLatestSyntaxFlowScanTaskID() (string, error) {
	db := consts.GetGormSSAProjectDataBase()
	if db == nil {
		return "", utils.Errorf("ssa project database is nil")
	}
	var t schema.SyntaxFlowScanTask
	q := db.Model(&schema.SyntaxFlowScanTask{}).
		Where("kind = ?", schema.SFResultKindScan).
		Order("updated_at DESC")
	if err := q.First(&t).Error; err != nil {
		var t2 schema.SyntaxFlowScanTask
		if err2 := db.Model(&schema.SyntaxFlowScanTask{}).Order("updated_at DESC").First(&t2).Error; err2 != nil {
			return "", err
		}
		return strings.TrimSpace(t2.TaskId), nil
	}
	return strings.TrimSpace(t.TaskId), nil
}
