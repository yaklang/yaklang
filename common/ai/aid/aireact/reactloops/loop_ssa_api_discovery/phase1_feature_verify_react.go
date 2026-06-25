package loop_ssa_api_discovery

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/utils"
)

// runPhase1FeatureVerifyChain runs the unified feature work dispatcher after Phase Auth gate.
func runPhase1FeatureVerifyChain(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime) error {
	return runPhase1FeatureApiChain(ctx, r, task, rt)
}

func featureAlreadyProcessed(m *FeatureApiMapV1, featureID string) bool {
	if m == nil {
		return false
	}
	for _, f := range m.Features {
		if f.FeatureID == featureID && f.Processed {
			return true
		}
	}
	return false
}

// phase1FeatureVerifyHTTPActionOptions exposes direct HTTP verdict writers without nested probe sub-loop.
func phase1FeatureVerifyHTTPActionOptions() []reactloops.ReActLoopOption {
	return []reactloops.ReActLoopOption{
		buildDiscoveryUpsertVerifiedHttpApi(),
		buildDiscoveryMarkApiRejected(),
		buildDiscoveryLinkHandlerCode(),
	}
}

func validateFeatureVerifyEntry(rt *Runtime, entry *FeatureApiMapEntry) error {
	if entry == nil {
		return utils.Error("nil feature entry")
	}
	if strings.TrimSpace(entry.FeatureID) == "" {
		return utils.Error("feature_id required")
	}
	if len(entry.Apis) == 0 {
		if strings.TrimSpace(entry.NoApiReason) == "" {
			return utils.Error("no_api_reason required when apis is empty")
		}
		return nil
	}
	liveTarget := rt != nil && rt.Session != nil && rt.Session.TargetReachable
	for i, a := range entry.Apis {
		if a.Method == "" || a.PathPattern == "" {
			return utils.Errorf("apis[%d] method and path_pattern required", i)
		}
		if !strings.HasPrefix(strings.TrimSpace(a.PathPattern), "/") {
			return utils.Errorf("apis[%d] path_pattern must be absolute (start with /), got %q", i, a.PathPattern)
		}
		if liveTarget && a.Verified && strings.TrimSpace(a.FullSampleURL) == "" {
			return utils.Errorf("apis[%d] verified=true requires full_sample_url after live HTTP probe", i)
		}
		if liveTarget && a.Verified && strings.TrimSpace(a.VerdictReason) == "" {
			return utils.Errorf("apis[%d] verified=true requires verdict_reason from HTTP probe", i)
		}
	}
	return nil
}

func handlerMatchesFeature(handlerClass, fileRel string, feat FeatureInventoryEntry) bool {
	hc := strings.ToLower(handlerClass)
	fr := strings.ToLower(fileRel)
	for _, ef := range EntryFilesForFeature(feat) {
		if strings.Contains(fr, strings.ToLower(ef)) {
			return true
		}
	}
	for _, pat := range feat.PackagePatterns {
		pat = strings.ToLower(strings.TrimSpace(pat))
		pat = strings.TrimPrefix(pat, "*.")
		pat = strings.ReplaceAll(pat, "*", "")
		if pat != "" && strings.Contains(hc, pat) {
			return true
		}
	}
	return false
}

func syncFeatureApisToEndpoints(rt *Runtime, entry FeatureApiMapEntry) error {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil
	}
	for _, a := range entry.Apis {
		ep := &store.HttpEndpoint{
			SessionID:     rt.Session.ID,
			Method:        a.Method,
			PathPattern:   a.PathPattern,
			HandlerClass:  a.HandlerClass,
			HandlerMethod: a.HandlerSymbol,
			Source:        SourceAICodeRead,
			Status:        "pending_validation",
		}
		_, _ = EndpointInsertionGateway(rt, ep)
	}
	return nil
}
