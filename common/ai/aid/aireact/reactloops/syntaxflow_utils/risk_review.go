package syntaxflow_utils

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// LoadRisk returns a single SSA risk by primary key.
func LoadRisk(db *gorm.DB, riskID int64) (*schema.SSARisk, error) {
	if db == nil {
		return nil, utils.Error("nil db")
	}
	return yakit.GetSSARiskByID(db, riskID)
}

// ListRiskDisposals returns disposal rows for the risk, optionally including inherited rows.
func ListRiskDisposals(db *gorm.DB, riskID int64, includeInherited bool) ([]schema.SSARiskDisposals, error) {
	if db == nil {
		return nil, utils.Error("nil db")
	}
	if includeInherited {
		return yakit.GetSSARiskDisposalsWithInheritance(db, riskID)
	}
	return yakit.GetSSARiskDisposalsOnly(db, riskID)
}

// RiskReloadText formats a risk row and disposals for AI feedback (concise).
// codeTruncate limits code_fragment length (e.g. 4000 or 12000 when get_full_code).
func RiskReloadText(risk *schema.SSARisk, disposals []schema.SSARiskDisposals, codeTruncate int) string {
	if codeTruncate <= 0 {
		codeTruncate = 4000
	}
	if risk == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("SSA Risk id=%d sev=%s program=%s rule=%s title=%s\n",
		risk.ID, risk.Severity, utils.ShrinkTextBlock(risk.ProgramName, 120),
		utils.ShrinkTextBlock(risk.FromRule, 80), utils.ShrinkTextBlock(risk.Title, 160)))
	sb.WriteString(fmt.Sprintf("runtime_id=%s result_id=%d latest_disposal=%s\n",
		risk.RuntimeId, risk.ResultID, risk.LatestDisposalStatus))
	if strings.TrimSpace(risk.CodeFragment) != "" {
		sb.WriteString("\n--- code_fragment (truncated) ---\n")
		sb.WriteString(utils.ShrinkTextBlock(risk.CodeFragment, codeTruncate))
		sb.WriteString("\n")
	}
	if strings.TrimSpace(risk.Details) != "" {
		sb.WriteString("\n--- details (truncated) ---\n")
		sb.WriteString(utils.ShrinkTextBlock(risk.Details, 2000))
		sb.WriteString("\n")
	}
	if len(disposals) > 0 {
		sb.WriteString("\n--- disposals (recent) ---\n")
		for i, d := range disposals {
			if i >= 8 {
				sb.WriteString(fmt.Sprintf("... and %d more\n", len(disposals)-8))
				break
			}
			sb.WriteString(fmt.Sprintf("- id=%d status=%s comment=%s task_id=%s\n",
				d.ID, d.Status, utils.ShrinkTextBlock(d.Comment, 200), d.TaskId))
		}
	}
	return sb.String()
}

// RiskToRuleSeed builds a JSON-able map for write_syntaxflow_rule.
func RiskToRuleSeed(risk *schema.SSARisk) map[string]any {
	if risk == nil {
		return nil
	}
	return map[string]any{
		"ssa_risk_id":     risk.ID,
		"program_name":    risk.ProgramName,
		"runtime_id":      risk.RuntimeId,
		"from_rule":       risk.FromRule,
		"severity":        risk.Severity,
		"title":           risk.Title,
		"risk_type":       risk.RiskType,
		"function_name":   risk.FunctionName,
		"code_source_url": risk.CodeSourceUrl,
		"code_range":      risk.CodeRange,
		"code_fragment":   risk.CodeFragment,
		"result_id":       risk.ResultID,
	}
}

// RiskToRuleSeedJSON returns compact JSON text for loop vars / feedback.
func RiskToRuleSeedJSON(risk *schema.SSARisk) string {
	m := RiskToRuleSeed(risk)
	if m == nil {
		return ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

// NormalizeDisposalStatus maps user/action strings to valid disposal status tokens.
func NormalizeDisposalStatus(s string) string {
	return string(schema.ValidSSARiskDisposalStatus(strings.TrimSpace(s)))
}
