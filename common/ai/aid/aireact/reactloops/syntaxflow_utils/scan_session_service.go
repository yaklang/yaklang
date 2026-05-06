package syntaxflow_utils

import (
	"context"
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// ScanSession groups scan-session helpers previously embedded in loop_syntaxflow_scan.
type ScanSessionService struct{}

// LoadSessionResult loads task plus risk sample rows (same semantics as [LoadScanSessionResult]).
func (ScanSessionService) LoadSessionResult(db *gorm.DB, taskID string, riskSampleLimit int) (*ScanSessionResult, error) {
	return LoadScanSessionResult(db, taskID, riskSampleLimit)
}

// BuildScanJSONForLocalPath builds minimal code-scan JSON for a local tree (see [BuildCodeScanJSONForLocalPath]).
func (ScanSessionService) BuildScanJSONForLocalPath(ctx context.Context, path string) (string, error) {
	_ = ctx
	return BuildCodeScanJSONForLocalPath(path)
}

// LoadProgramsFromScanJSON parses JSON and loads SSA programs (see [LoadProgramsFromCodeScanJSON]).
func (ScanSessionService) LoadProgramsFromScanJSON(ctx context.Context, jsonRaw []byte) (*ssaconfig.Config, []*ssaapi.Program, error) {
	return LoadProgramsFromCodeScanJSON(ctx, jsonRaw)
}

// StartScan runs [StartSyntaxFlowScanBackground] for an already compiled project + config.
func (ScanSessionService) StartScan(ctx context.Context, cfg *ssaconfig.Config, progs []*ssaapi.Program) (taskID string, err error) {
	return StartSyntaxFlowScanBackground(ctx, cfg, progs)
}

// StartScanWithRuleFile appends rule file content to the scan ([StartSyntaxFlowScanBackgroundWithRuleFile]).
func (ScanSessionService) StartScanWithRuleFile(ctx context.Context, cfg *ssaconfig.Config, progs []*ssaapi.Program, rulePath string) (taskID string, err error) {
	return StartSyntaxFlowScanBackgroundWithRuleFile(ctx, cfg, progs, rulePath)
}

// CompareScanTasks returns a short textual diff of two SyntaxFlow tasks (status + risk totals).
func (ScanSessionService) CompareScanTasks(db *gorm.DB, a, b string) (string, error) {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return "", utils.Error("CompareScanTasks: empty task id")
	}
	ra, err := LoadScanSessionResult(db, a, 5)
	if err != nil {
		return "", err
	}
	rb, err := LoadScanSessionResult(db, b, 5)
	if err != nil {
		return "", err
	}
	sa, sb := "(nil)", "(nil)"
	var raRisks, rbRisks int
	if ra != nil && ra.ScanTask != nil {
		sa = ra.ScanTask.Status
		raRisks = int(ra.TotalRisks)
	}
	if rb != nil && rb.ScanTask != nil {
		sb = rb.ScanTask.Status
		rbRisks = int(rb.TotalRisks)
	}
	return fmt.Sprintf("task_a=%s status=%s risks=%d | task_b=%s status=%s risks=%d",
		a, sa, raRisks, b, sb, rbRisks), nil
}
