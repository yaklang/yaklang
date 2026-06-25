package loop_ssa_api_discovery

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

// EnsureAuthReadyBeforeFeatureWork verifies Phase Auth completed before concurrent feature API work.
func EnsureAuthReadyBeforeFeatureWork(rt *Runtime) error {
	if rt == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	if !rt.Session.TargetReachable {
		return nil
	}
	if !phase1AuthRequired(rt) {
		return nil
	}
	ready, reason := EvaluatePhase1AuthCalibrationReadiness(rt)
	if !ready {
		return &Phase1AuthFailedError{Reason: "Phase Feature blocked: auth not ready — " + reason}
	}
	if !hasVerifiedAuthCredential(rt) && !authPartialAuthEnabled(rt) {
		return &Phase1AuthFailedError{Reason: "Phase Feature blocked: no verified auth_credentials in DB"}
	}
	return nil
}

func runPhase1FeatureApiChain(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime) error {
	_ = ctx
	if err := EnsureAuthReadyBeforeFeatureWork(rt); err != nil {
		return err
	}
	return runPhase1FeatureWorkChain(r, task, rt)
}
