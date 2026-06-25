package loop_ssa_api_discovery

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func refreshPhase1VerifyLoopVars(loop *reactloops.ReActLoop, rt *Runtime) {
	if loop == nil || rt == nil || rt.Repo == nil || rt.Session == nil {
		return
	}
	eps, _ := rt.Repo.ListHttpEndpoints(rt.Session.ID)
	vha, _ := rt.Repo.ListVerifiedHttpApis(rt.Session.ID)
	done := map[string]struct{}{}
	for _, v := range vha {
		key := strings.ToUpper(strings.TrimSpace(v.Method)) + " " + strings.TrimSpace(v.PathPattern)
		done[key] = struct{}{}
	}
	var pending []string
	var pendingOther []string
	for _, e := range eps {
		key := strings.ToUpper(strings.TrimSpace(e.Method)) + " " + strings.TrimSpace(e.PathPattern)
		if _, ok := done[key]; !ok {
			line := fmt.Sprintf("id=%d %s %s source=%s", e.ID, e.Method, e.PathPattern, e.Source)
			if e.Source == SourceAICodeRead {
				pending = append(pending, line)
			} else {
				pendingOther = append(pendingOther, line)
			}
		}
	}
	pending = append(pending, pendingOther...)
	if len(pending) > 30 {
		pending = append(pending[:30], fmt.Sprintf("... +%d more", len(eps)-30))
	}
	loop.Set("phase1_pending_candidates", strings.Join(pending, "; "))
	total, verified, _ := rt.Repo.CountVerifiedHttpApis(rt.Session.ID)
	loop.Set("discovery_counts_line", fmt.Sprintf("verified_http_apis total=%d verified=%d http_endpoints=%d", total, verified, len(eps)))
}
