package loop_ssa_api_discovery

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

// FileOpInput describes one file operation audit row to persist.
type FileOpInput struct {
	Stage      string
	Operation  string
	RelPath    string
	RuleID     string
	ToolName   string
	Outcome    string
	Summary    string
	Detail     map[string]any
	DurationMs int
}

func logFileOp(rt *Runtime, in FileOpInput) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return
	}
	detailJSON := ""
	if len(in.Detail) > 0 {
		if b, err := json.Marshal(in.Detail); err == nil {
			detailJSON = string(b)
		}
	}
	_ = rt.Repo.AppendFileOperation(&store.DiscoveryFileOperation{
		SessionID:     rt.Session.ID,
		PipelineStage: in.Stage,
		Operation:     in.Operation,
		RelPath:       in.RelPath,
		RuleID:        in.RuleID,
		ToolName:      in.ToolName,
		Outcome:       in.Outcome,
		Summary:       in.Summary,
		DetailJSON:    detailJSON,
		DurationMs:    in.DurationMs,
	})
}

func logFileOpsBatch(rt *Runtime, rows []FileOpInput) {
	for _, row := range rows {
		logFileOp(rt, row)
	}
}
