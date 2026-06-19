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

const reportProgramsColMax = 80

func likePatternForHint(hint string) string {
	s := strings.ReplaceAll(hint, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return "%" + s + "%"
}

func parseProgramHint(programHint string) (string, error) {
	hint := strings.TrimSpace(programHint)
	if hint == "" {
		return "", utils.Errorf("empty program hint")
	}
	return hint, nil
}

func clampScanCheckLimit(limit int) int {
	switch {
	case limit <= 0:
		return 20
	case limit > 200:
		return 200
	default:
		return limit
	}
}

// scanTasksByProjectHintQuery matches tasks by profile SSAProject.project_name (via task.project_id)
// or legacy rows with project_id=0 and programs LIKE. SSA DB and profile DB are separate files — no JOIN.
func scanTasksByProjectHintQuery(ssaDB *gorm.DB, like string) (*gorm.DB, error) {
	profileDB := consts.GetGormProfileDatabase()
	if profileDB == nil {
		return nil, utils.Errorf("profile database is nil")
	}
	var ids []uint
	if err := profileDB.Model(&schema.SSAProject{}).
		Where("project_name LIKE ?", like).
		Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	q := ssaDB.Model(&schema.SyntaxFlowScanTask{})
	if len(ids) > 0 {
		return q.Where("(project_id IN (?) AND project_id > 0) OR (project_id = 0 AND programs LIKE ?)", ids, like), nil
	}
	return q.Where("project_id = 0 AND programs LIKE ?", like), nil
}

// QuerySyntaxFlowScanTasksByProgramsContains queries scan tasks by project name (ssa_projects.project_name)
// when task.project_id > 0; falls back to legacy programs LIKE only when project_id = 0.
func QuerySyntaxFlowScanTasksByProgramsContains(programHint string, scanOnly bool, limit int) ([]*schema.SyntaxFlowScanTask, error) {
	db := consts.GetGormSSAProjectDataBase()
	if db == nil {
		return nil, utils.Errorf("ssa project database is nil")
	}
	hint, err := parseProgramHint(programHint)
	if err != nil {
		return nil, err
	}
	limit = clampScanCheckLimit(limit)

	q, err := scanTasksByProjectHintQuery(db, likePatternForHint(hint))
	if err != nil {
		return nil, err
	}
	q = q.Order("updated_at DESC").Limit(limit)
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

// RunSyntaxFlowProjectScanCheck 查询某个程序的 SyntaxFlow 扫描任务并汇总核对结果
// 导出名为 syntaxflow.RunSyntaxFlowProjectScanCheck
// 参数:
//   - programHint: 程序名提示（用于匹配扫描任务关联的程序）
//   - scanOnly: 是否仅统计扫描类型任务
//   - limit: 返回任务数量上限
//
// 返回值:
//   - 项目扫描核对结果（含最近任务 ID、任务列表、Markdown 报告等）
//   - 错误信息
//
// Example:
// ```
// // 查询某程序最近的扫描任务核对结果（示意性示例，需要已有扫描任务）
// result = syntaxflow.RunSyntaxFlowProjectScanCheck("my-program", false, 10)~
// dump(result)
// ```
func RunSyntaxFlowProjectScanCheck(programHint string, scanOnly bool, limit int) (*SyntaxFlowProjectScanCheckResult, error) {
	tasks, err := QuerySyntaxFlowScanTasksByProgramsContains(programHint, scanOnly, limit)
	if err != nil {
		return nil, err
	}
	hint := strings.TrimSpace(programHint)
	limit = clampScanCheckLimit(limit)

	latestTaskID := ""
	if len(tasks) > 0 {
		latestTaskID = strings.TrimSpace(tasks[0].TaskId)
	}

	return &SyntaxFlowProjectScanCheckResult{
		ProgramHint:    hint,
		ScanOnly:       scanOnly,
		Limit:          limit,
		LatestTaskID:   latestTaskID,
		Tasks:          tasks,
		ReportMarkdown: buildScanProjectCheckReportMarkdown(hint, scanOnly, tasks),
	}, nil
}

func buildScanProjectCheckReportMarkdown(programHint string, scanOnly bool, tasks []*schema.SyntaxFlowScanTask) string {
	var b strings.Builder
	b.WriteString("# SyntaxFlow Project Scan Check\n\n")
	b.WriteString(fmt.Sprintf("- **match hint**: `%s` (prefer `ssa_projects.project_name` via `project_id`; legacy rows with `project_id=0` use `programs` LIKE)\n", strings.TrimSpace(programHint)))
	b.WriteString(fmt.Sprintf("- **scan_only (kind=scan)**: %v\n", scanOnly))
	b.WriteString(fmt.Sprintf("- **matched rows**: %d\n\n", len(tasks)))
	if len(tasks) == 0 {
		b.WriteString("No matched scan task found. Please verify `ssa_projects.project_name` (and task.project_id link) or legacy `syntax_flow_scan_tasks.programs`, or run a global latest-scan check.\n")
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
		if len(pro) > reportProgramsColMax {
			pro = pro[:reportProgramsColMax-3] + "..."
		}
		b.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s | %d |\n",
			t.TaskId, t.Kind, t.Status, pro, up, t.RiskCount))
	}
	b.WriteString("\n**Suggestion**: use `scan_id=<task_id>` from the first row, or provide a specific task_id explicitly.\n")
	return b.String()
}
