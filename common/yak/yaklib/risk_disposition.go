package yaklib

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type RiskDispositionResult struct {
	DatabaseID     string
	Disposition    string
	UpdatedCount   int
	UnchangedCount int
	MissingIDs     []int64
}

func (r *RiskDispositionResult) Dump() string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf(
		"database_id=%s disposition=%s updated=%d unchanged=%d missing=%d missing_ids=%s\n",
		r.DatabaseID,
		r.Disposition,
		r.UpdatedCount,
		r.UnchangedCount,
		len(r.MissingIDs),
		formatInt64List(r.MissingIDs),
	)
}

func formatInt64List(values []int64) string {
	if len(values) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%d", value))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// TriageRiskInDatabase updates recoverable risk dispositions in the current
// project database. It never deletes risks or changes their evidence fields.
func TriageRiskInDatabase(
	ids []int64,
	disposition string,
	duplicateOf int64,
	reason string,
	opts ...DBHistoryOption,
) (*RiskDispositionResult, error) {
	c, err := historyConfig(0, opts)
	if err != nil {
		return nil, err
	}
	if c.methods != "" || c.status != "" || c.source != "" || c.packetLimit != 0 {
		return nil, fmt.Errorf("HTTP-specific options are not supported by risk disposition updates")
	}
	if c.projectID != "" && c.projectID != "current" {
		return nil, fmt.Errorf("risk disposition updates are limited to the current project database")
	}
	if c.riskSelectionScopeSet {
		allowed := make(map[int64]struct{}, len(c.riskSelectionAllowedIDs))
		for _, id := range c.riskSelectionAllowedIDs {
			allowed[id] = struct{}{}
		}
		for _, id := range ids {
			if _, ok := allowed[id]; !ok {
				return nil, fmt.Errorf("risk %d is outside the attached risk selection", id)
			}
		}
	}
	db, closeDB, err := historyProjectDatabase(c)
	if err != nil {
		return nil, err
	}
	defer closeDB()

	disposition = schema.NormalizeRiskDisposition(disposition)
	result, err := yakit.UpdateRiskDispositions(db, ids, disposition, duplicateOf, reason)
	if err != nil {
		return nil, err
	}
	return &RiskDispositionResult{
		DatabaseID:     "current",
		Disposition:    disposition,
		UpdatedCount:   result.UpdatedCount,
		UnchangedCount: result.UnchangedCount,
		MissingIDs:     result.MissingIDs,
	}, nil
}

func init() {
	RiskExports["TriageRiskInDatabase"] = TriageRiskInDatabase
}
