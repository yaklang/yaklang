package syntaxflow_utils

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"google.golang.org/protobuf/encoding/protojson"
)

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
