package loop_ssa_api_discovery

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

func countSessionHttpEndpoints(rt *Runtime) (int, error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return 0, nil
	}
	eps, err := rt.Repo.ListHttpEndpoints(rt.Session.ID)
	if err != nil {
		return 0, err
	}
	return len(eps), nil
}

// mergeStaticEndpointHarvest runs optional Yak harvest, then Go static harvest when endpoints are still empty.
func mergeStaticEndpointHarvest(
	ctx context.Context,
	invoker aicommon.AIInvokeRuntime,
	rt *Runtime,
	yakTool string,
	extra map[string]any,
) (yakErr error, goUsed bool, goErr error) {
	if invoker != nil && yakTool != "" {
		_, yakErr = executeYakTool(invoker, ctx, yakTool, rt, extra)
	}
	n, err := countSessionHttpEndpoints(rt)
	if err != nil {
		return yakErr, false, err
	}
	if n > 0 {
		return yakErr, false, nil
	}
	if rt == nil || rt.Session == nil || !LanguageHasStaticHarvester(rt.Session.Language) {
		return yakErr, false, nil
	}
	_, goErr = RunEndpointHarvestForSession(rt)
	return yakErr, true, goErr
}

func ensureStaticEndpointsFromGo(rt *Runtime) error {
	n, err := countSessionHttpEndpoints(rt)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if rt == nil || rt.Session == nil || !LanguageHasStaticHarvester(rt.Session.Language) {
		return nil
	}
	_, err = RunEndpointHarvestForSession(rt)
	return err
}

func prepTaskGoFallbackWarning(taskID string, yakErr error) string {
	if yakErr == nil {
		return ""
	}
	return taskID + ": yak failed, go_fallback: " + utils.ShrinkString(yakErr.Error(), 400)
}
