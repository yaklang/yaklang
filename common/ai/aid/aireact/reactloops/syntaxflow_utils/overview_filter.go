package syntaxflow_utils

import (
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
