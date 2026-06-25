package loop_ssa_api_discovery

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
)

// EnsureHttpEndpointsIfEmpty ensures http_endpoints exist after Phase1, supplementing static hints without overwriting AI routes.
func EnsureHttpEndpointsIfEmpty(invoker aicommon.AIInvokeRuntime, ctx context.Context, rt *Runtime, auditTag string) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return
	}
	sess := rt.Session
	if !sess.CodePathOK || sess.CodeRootPath == "" {
		log.Infof("ssa_api_discovery: skip endpoint guard (%s): invalid code path", auditTag)
		return
	}

	aiCount, err := CountAICodeReadEndpoints(rt)
	if err != nil {
		log.Warnf("ssa_api_discovery: count ai_code_read endpoints: %v", err)
	}
	if aiCount > 0 {
		log.Infof("ssa_api_discovery: endpoint guard (%s): ai_code_read=%d supplementing static hints only", auditTag, aiCount)
		if _, _, serr := SupplementStaticRouteHints(ctx, invoker, rt); serr != nil {
			log.Warnf("ssa_api_discovery: supplement hints (%s): %v", auditTag, serr)
		}
		if s2, gerr := rt.Repo.GetSessionByUUID(sess.UUID); gerr == nil && s2 != nil {
			rt.Session = s2
		}
		return
	}

	if _, err := LoadCodeReadingPlan(rt.WorkDir); err == nil {
		log.Warnf("ssa_api_discovery: endpoint guard (%s): code_reading_plan exists but no ai_code_read endpoints synced", auditTag)
		_ = rt.Repo.AppendEvent(sess.ID, "warn", "http_endpoint_auto_harvest", fmt.Sprintf(`{"tag":%q,"warning":"ai_code_read_missing"}`, auditTag))
		return
	}

	var n int64
	if err := rt.Repo.DB().Model(&store.HttpEndpoint{}).Where("session_id = ?", sess.ID).Count(&n).Error; err != nil {
		log.Warnf("ssa_api_discovery: count http_endpoints: %v", err)
		return
	}
	if n > 0 {
		return
	}
	if !LanguageHasStaticHarvester(sess.Language) {
		return
	}
	if _, _, serr := SupplementStaticRouteHints(ctx, invoker, rt); serr != nil {
		log.Warnf("ssa_api_discovery: legacy supplement (%s): %v", auditTag, serr)
	}
}
