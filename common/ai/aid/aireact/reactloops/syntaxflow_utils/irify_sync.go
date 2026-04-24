package syntaxflow_utils

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"google.golang.org/protobuf/encoding/protojson"
)

// SyncSyntaxFlowLoopVarsFromIrifyTask copies irify_syntaxflow* attachments onto the loop when the
// corresponding loop var is empty. When session mode is "start" (from loop or attachment), it does
// not copy task_id from attachments. Call once at the beginning of P1 intake.
func SyncSyntaxFlowLoopVarsFromIrifyTask(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) {
	if loop == nil || task == nil {
		return
	}
	if strings.TrimSpace(loop.Get(LoopVarSyntaxFlowScanSessionMode)) == "" {
		if m := readIrifySessionModeFromTask(task); m != "" {
			loop.Set(LoopVarSyntaxFlowScanSessionMode, m)
		}
	}
	mode := strings.ToLower(strings.TrimSpace(loop.Get(LoopVarSyntaxFlowScanSessionMode)))
	if strings.TrimSpace(loop.Get(LoopVarSFRuleFullQuality)) == "" && readIrifyRuleFullQualityFromTask(task) {
		loop.Set(LoopVarSFRuleFullQuality, "true")
	}
	if mode == SessionModeStart {
		return
	}
	if strings.TrimSpace(loop.Get(LoopVarSyntaxFlowTaskID)) == "" {
		if id, ok := ReadIrifySyntaxFlowTaskIDFromTask(task); ok && id != "" {
			loop.Set(LoopVarSyntaxFlowTaskID, id)
		}
	}
}

// SyncSSARiskIDFromIrifyToLoop sets ssa_risk_id on the loop from irify_ssa_risk when the loop value is empty.
func SyncSSARiskIDFromIrifyToLoop(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) {
	if loop == nil || task == nil {
		return
	}
	if strings.TrimSpace(loop.Get(LoopVarSSARiskID)) != "" {
		return
	}
	if id, ok := ReadIrifySSARiskIDFromTask(task); ok {
		loop.Set(LoopVarSSARiskID, fmt.Sprintf("%d", id))
	}
}

// SyncSSARisksFilterFromIrifyToLoop materializes ssa_risks_filter_json on the loop from irify_ssa_risks_filter
// attachments when the loop has no prior filter json.
func SyncSSARisksFilterFromIrifyToLoop(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) {
	if loop == nil {
		return
	}
	if strings.TrimSpace(loop.Get(LoopVarSSARisksFilterJSON)) != "" {
		return
	}
	f, ok := buildSSARisksFilterFromTaskAttachments(task)
	if !ok || f == nil {
		return
	}
	b, err := protojson.Marshal(f)
	if err != nil {
		return
	}
	loop.Set(LoopVarSSARisksFilterJSON, string(b))
}
