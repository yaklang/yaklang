package syntaxflow_utils

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/encoding/protojson"
)

// LoopStringGetter is implemented by *reactloops.ReActLoop (see Get).
type LoopStringGetter interface {
	Get(k string) string
}

// BuildSSARisksFilterFromLoop builds an SSARisksFilter from loop var ssa_risks_filter_json (protojson)
// and optionally fills Search from a short free-text user query.
func BuildSSARisksFilterFromLoop(loop LoopStringGetter, userQuery string) *ypb.SSARisksFilter {
	f := &ypb.SSARisksFilter{}
	if loop != nil {
		if raw := strings.TrimSpace(loop.Get(LoopVarSSARisksFilterJSON)); raw != "" {
			_ = protojson.Unmarshal([]byte(raw), f)
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

// SSARisksFilterHasConstraints reports whether the filter narrows the SSA risk set beyond an empty filter.
func SSARisksFilterHasConstraints(f *ypb.SSARisksFilter) bool {
	if f == nil {
		return false
	}
	if strings.TrimSpace(f.Search) != "" {
		return true
	}
	if len(f.GetRuntimeID()) > 0 || len(f.GetProgramName()) > 0 || len(f.GetRiskType()) > 0 ||
		len(f.GetSeverity()) > 0 || len(f.GetFromRule()) > 0 || strings.TrimSpace(f.GetTitle()) != "" ||
		len(f.GetID()) > 0 || len(f.GetTags()) > 0 {
		return true
	}
	return f.GetIsRead() != 0
}

// FormatSSARisksFilterHuman renders filter fields for timeline / thought stream (user-visible).
func FormatSSARisksFilterHuman(f *ypb.SSARisksFilter) string {
	if f == nil || !SSARisksFilterHasConstraints(f) {
		return "(无附加过滤 — 查询 SSA 工程库全部风险)"
	}
	var parts []string
	if s := strings.TrimSpace(f.Search); s != "" {
		parts = append(parts, fmt.Sprintf("search=%q", s))
	}
	if ids := f.GetRuntimeID(); len(ids) > 0 {
		parts = append(parts, "runtime_id="+strings.Join(ids, ","))
	}
	if names := f.GetProgramName(); len(names) > 0 {
		parts = append(parts, "program_name="+strings.Join(names, ","))
	}
	if types := f.GetRiskType(); len(types) > 0 {
		parts = append(parts, "risk_type="+strings.Join(types, ","))
	}
	if sev := f.GetSeverity(); len(sev) > 0 {
		parts = append(parts, "severity="+strings.Join(sev, ","))
	}
	if rules := f.GetFromRule(); len(rules) > 0 {
		parts = append(parts, "from_rule="+strings.Join(rules, ","))
	}
	if t := strings.TrimSpace(f.GetTitle()); t != "" {
		parts = append(parts, "title="+t)
	}
	if f.GetIsRead() != 0 {
		parts = append(parts, fmt.Sprintf("is_read=%v", f.GetIsRead() > 0))
	}
	if len(parts) == 0 {
		return "(filter_json 已设置但无可读字段)"
	}
	return strings.Join(parts, "; ")
}
