package syntaxflow_utils

import (
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/encoding/protojson"
)

// VarSource is loop-scoped configuration (e.g. *reactloops.ReActLoop from WithVar).
type VarSource interface {
	GetVariable(key string) any
}

// SyntaxFlowTaskID returns the SyntaxFlow scan task id (SSA runtime id) from loop vars, then task attachments.
// vars may be nil (e.g. orchestrator reading parent task only).
func SyntaxFlowTaskID(task aicommon.AIStatefulTask, vars VarSource) (string, bool) {
	if vars != nil {
		if v := vars.GetVariable(LoopVarSyntaxFlowTaskID); v != nil {
			if s := strings.TrimSpace(utils.InterfaceToString(v)); s != "" {
				return s, true
			}
		}
	}
	if task == nil {
		return "", false
	}
	for _, a := range task.GetAttachedDatas() {
		if a == nil {
			continue
		}
		if a.Type != AttachedTypeSyntaxFlow {
			continue
		}
		if a.Key != AttachedKeyTaskID {
			continue
		}
		if s := strings.TrimSpace(a.Value); s != "" {
			return s, true
		}
	}
	return "", false
}

// SSARiskID returns the SSA risk primary key from loop vars, then task attachments.
func SSARiskID(task aicommon.AIStatefulTask, vars VarSource) (int64, bool) {
	if vars != nil {
		if v := vars.GetVariable(LoopVarSSARiskID); v != nil {
			if s := strings.TrimSpace(utils.InterfaceToString(v)); s != "" {
				if id, err := strconv.ParseInt(s, 10, 64); err == nil && id > 0 {
					return id, true
				}
			}
		}
	}
	if task == nil {
		return 0, false
	}
	for _, a := range task.GetAttachedDatas() {
		if a == nil || a.Type != AttachedTypeSSARisk || a.Key != AttachedKeyRiskID {
			continue
		}
		if id, err := strconv.ParseInt(strings.TrimSpace(a.Value), 10, 64); err == nil && id > 0 {
			return id, true
		}
	}
	return 0, false
}

// SSARisksFilterForOverview builds ypb.SSARisksFilter from loop var JSON, attachments, or free-text Search only.
func SSARisksFilterForOverview(task aicommon.AIStatefulTask, vars VarSource, userQuery string) *ypb.SSARisksFilter {
	if vars != nil {
		if v := vars.GetVariable(LoopVarSSARisksFilterJSON); v != nil {
			raw := strings.TrimSpace(utils.InterfaceToString(v))
			if raw != "" {
				f := &ypb.SSARisksFilter{}
				if err := protojson.Unmarshal([]byte(raw), f); err == nil {
					return f
				}
			}
		}
	}

	if task != nil {
		for _, a := range task.GetAttachedDatas() {
			if a == nil || a.Type != AttachedTypeSSARisksFilter {
				continue
			}
			if a.Key != AttachedKeyFilterJSON {
				continue
			}
			raw := strings.TrimSpace(a.Value)
			if raw == "" {
				continue
			}
			f := &ypb.SSARisksFilter{}
			if err := protojson.Unmarshal([]byte(raw), f); err == nil {
				return f
			}
		}
	}

	f := &ypb.SSARisksFilter{}
	if task != nil {
		for _, a := range task.GetAttachedDatas() {
			if a == nil || a.Type != AttachedTypeSSARisksFilter {
				continue
			}
			switch a.Key {
			case AttachedKeyRuntimeID:
				if s := strings.TrimSpace(a.Value); s != "" {
					f.RuntimeID = append(f.RuntimeID, s)
				}
			case AttachedKeyProgramName:
				if s := strings.TrimSpace(a.Value); s != "" {
					f.ProgramName = append(f.ProgramName, s)
				}
			case AttachedKeyPrograms:
				for _, p := range strings.Split(a.Value, ",") {
					p = strings.TrimSpace(p)
					if p != "" {
						f.ProgramName = append(f.ProgramName, p)
					}
				}
			}
		}
	}

	trim := strings.TrimSpace(userQuery)
	if trim != "" && f.Search == "" && len(f.ProgramName) == 0 && len(f.RuntimeID) == 0 {
		if len([]rune(trim)) <= 256 {
			f.Search = trim
		} else {
			f.Search = string([]rune(trim)[:256])
		}
	}
	return f
}

// SyntaxFlowScanSessionMode returns SessionModeAttach, SessionModeStart, or "" if unspecified.
func SyntaxFlowScanSessionMode(task aicommon.AIStatefulTask, vars VarSource) string {
	if vars != nil {
		if v := vars.GetVariable(LoopVarSyntaxFlowScanSessionMode); v != nil {
			m := strings.ToLower(strings.TrimSpace(utils.InterfaceToString(v)))
			if m == SessionModeAttach || m == SessionModeStart {
				return m
			}
		}
	}
	if task == nil {
		return ""
	}
	for _, a := range task.GetAttachedDatas() {
		if a == nil || a.Type != AttachedTypeSyntaxFlow {
			continue
		}
		if a.Key != AttachedKeySessionMode {
			continue
		}
		m := strings.ToLower(strings.TrimSpace(a.Value))
		if m == SessionModeAttach || m == SessionModeStart {
			return m
		}
	}
	return ""
}

// ProgramNamesHint returns optional program names from irify_syntaxflow attachment (comma-separated list).
func ProgramNamesHint(task aicommon.AIStatefulTask) []string {
	if task == nil {
		return nil
	}
	var out []string
	for _, a := range task.GetAttachedDatas() {
		if a == nil || a.Type != AttachedTypeSyntaxFlow || a.Key != AttachedKeyPrograms {
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

// SyntaxFlowRuleFullQuality is true when loop var sf_rule_full_quality or irify_syntaxflow_rule attachment says so.
func SyntaxFlowRuleFullQuality(task aicommon.AIStatefulTask, vars VarSource) bool {
	if vars != nil && utils.InterfaceToBoolean(vars.GetVariable(LoopVarSFRuleFullQuality)) {
		return true
	}
	if task == nil {
		return false
	}
	for _, a := range task.GetAttachedDatas() {
		if a == nil || a.Type != AttachedTypeSyntaxFlowRule || a.Key != AttachedKeyFullQuality {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(a.Value)) {
		case "true", "1", "yes", "on":
			return true
		}
	}
	return false
}
