package loop_ssa_api_discovery

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// runMinimalProgrammaticPrep runs Stage0–2.6 programmatic steps required before toolkit extract.
func runMinimalProgrammaticPrep(ctx context.Context, r aicommon.AIInvokeRuntime, rt *Runtime) error {
	if rt == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	_ = ctx
	steps := []struct {
		name string
		fn   func() error
		out  []string
	}{
		{"project_profile", func() error { _, err := RunBuildProjectProfile(rt); return err }, []string{store.ProjectProfilePath(rt.WorkDir)}},
		{"backend_scope", func() error { _, err := RunBuildBackendScope(ctx, r, rt); return err }, []string{store.BackendScopePath(rt.WorkDir)}},
		{"unified_endpoint_extraction", func() error { _, err := RunFullEndpointExtraction(rt); return err }, []string{store.UnifiedEndpointsPath(rt.WorkDir)}},
		{"servlet_routing_map", func() error { _, err := RunBuildServletRoutingMap(rt); return err }, []string{store.ServletRoutingMapPath(rt.WorkDir)}},
		{"java_business_scope", func() error { _, err := BuildJavaBusinessScopeInventory(rt); return err }, []string{store.JavaBusinessScopeInventoryPath(rt.WorkDir)}},
		{"code_unit_registry", func() error { _, err := BuildCodeUnitRegistry(rt); return err }, []string{store.CodeUnitRegistryPath(rt.WorkDir)}},
	}
	for _, step := range steps {
		started := step.name
		rt.execStepStart("framework_toolkit.prep."+started, "programmatic")
		if err := step.fn(); err != nil {
			rt.execStepError("framework_toolkit.prep."+started, "programmatic", startedTime(), err, nil)
			log.Warnf("ssa_api_discovery: toolkit prep %s: %v", started, err)
		} else {
			rt.execStepEnd("framework_toolkit.prep."+started, "programmatic", startedTime(), step.out)
		}
	}
	if ok, _ := shouldRunFrontendAPIAnalysis(rt); ok {
		rt.execStepStart("framework_toolkit.prep.frontend_api_harvest", "programmatic")
		if _, err := RunFrontendAPIHarvest(rt); err != nil {
			rt.execStepError("framework_toolkit.prep.frontend_api_harvest", "programmatic", startedTime(), err, nil)
			log.Warnf("ssa_api_discovery: toolkit frontend harvest: %v", err)
		} else {
			rt.execStepEnd("framework_toolkit.prep.frontend_api_harvest", "programmatic", startedTime(), []string{store.FrontendAPIHarvestPath(rt.WorkDir)})
		}
	}
	return nil
}

func startedTime() time.Time {
	return time.Now()
}
