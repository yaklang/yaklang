package syntaxflow_utils

import (
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/encoding/protojson"
)

// ReadIrifySyntaxFlowTaskIDFromTask returns the SyntaxFlow scan task_id from irify_syntaxflow attachments (no loop).
func ReadIrifySyntaxFlowTaskIDFromTask(task aicommon.AIStatefulTask) (string, bool) {
	if task == nil {
		return "", false
	}
	for _, a := range task.GetAttachedDatas() {
		if a == nil || a.Type != IrifyTypeSyntaxFlow || a.Key != IrifyKeyTaskID {
			continue
		}
		if s := strings.TrimSpace(a.Value); s != "" {
			return s, true
		}
	}
	return "", false
}

// ReadIrifySSARiskIDFromTask returns SSA risk id from irify_ssa_risk / risk_id attachment (no loop).
func ReadIrifySSARiskIDFromTask(task aicommon.AIStatefulTask) (int64, bool) {
	if task == nil {
		return 0, false
	}
	for _, a := range task.GetAttachedDatas() {
		if a == nil || a.Type != IrifyTypeSSARisk || a.Key != IrifyKeyRiskID {
			continue
		}
		if id, err := strconv.ParseInt(strings.TrimSpace(a.Value), 10, 64); err == nil && id > 0 {
			return id, true
		}
	}
	return 0, false
}

// readIrifySessionModeFromTask returns attach/start from irify_syntaxflow#session_mode, or "".
func readIrifySessionModeFromTask(task aicommon.AIStatefulTask) string {
	if task == nil {
		return ""
	}
	for _, a := range task.GetAttachedDatas() {
		if a == nil || a.Type != IrifyTypeSyntaxFlow || a.Key != IrifyKeySessionMode {
			continue
		}
		m := strings.ToLower(strings.TrimSpace(a.Value))
		if m == SessionModeAttach || m == SessionModeStart {
			return m
		}
	}
	return ""
}

// readIrifyRuleFullQualityFromTask returns true if irify_syntaxflow_rule#full_quality is truthy.
func readIrifyRuleFullQualityFromTask(task aicommon.AIStatefulTask) bool {
	if task == nil {
		return false
	}
	for _, a := range task.GetAttachedDatas() {
		if a == nil || a.Type != IrifyTypeSyntaxFlowRule || a.Key != IrifyKeyFullQuality {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(a.Value)) {
		case "true", "1", "yes", "on":
			return true
		}
	}
	return false
}

// buildSSARisksFilterFromTaskAttachments assembles a filter from irify_ssa_risks_filter attachments only.
// Returns (nil, false) when the task has no filter attachments with usable data.
func buildSSARisksFilterFromTaskAttachments(task aicommon.AIStatefulTask) (*ypb.SSARisksFilter, bool) {
	if task == nil {
		return nil, false
	}
	for _, a := range task.GetAttachedDatas() {
		if a == nil || a.Type != IrifyTypeSSARisksFilter {
			continue
		}
		if a.Key != IrifyKeyFilterJSON {
			continue
		}
		raw := strings.TrimSpace(a.Value)
		if raw == "" {
			continue
		}
		f := &ypb.SSARisksFilter{}
		if err := protojson.Unmarshal([]byte(raw), f); err == nil {
			return f, true
		}
	}
	f := &ypb.SSARisksFilter{}
	any := false
	for _, a := range task.GetAttachedDatas() {
		if a == nil || a.Type != IrifyTypeSSARisksFilter {
			continue
		}
		switch a.Key {
		case IrifyKeyRuntimeID:
			if s := strings.TrimSpace(a.Value); s != "" {
				f.RuntimeID = append(f.RuntimeID, s)
				any = true
			}
		case IrifyKeyProgramName:
			if s := strings.TrimSpace(a.Value); s != "" {
				f.ProgramName = append(f.ProgramName, s)
				any = true
			}
		case IrifyKeyPrograms:
			for _, p := range strings.Split(a.Value, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					f.ProgramName = append(f.ProgramName, p)
					any = true
				}
			}
		}
	}
	if !any {
		return nil, false
	}
	return f, true
}

// IrifyProgramNamesFromTask returns program name hints from irify_syntaxflow#programs (comma-separated).
func IrifyProgramNamesFromTask(task aicommon.AIStatefulTask) []string {
	if task == nil {
		return nil
	}
	var out []string
	for _, a := range task.GetAttachedDatas() {
		if a == nil || a.Type != IrifyTypeSyntaxFlow || a.Key != IrifyKeyPrograms {
			continue
		}
		for _, p := range strings.Split(a.Value, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}
