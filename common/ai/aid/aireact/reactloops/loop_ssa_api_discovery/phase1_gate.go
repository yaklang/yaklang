package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func verifyPhase1ApiVerificationGate(rt *Runtime) error {
	return verifyPhase1GranularGate(rt)
}

// verifyPhase1GranularGate checks that ReAct-driven coverage assessment has completed,
// feature_api_map has processed all F1-assigned features, and probe evidence is sufficient.
//
// KEY CHANGE: We no longer require all code_unit_registry units to be completed.
// The registry may include third-party library files, generated code, and other
// non-business units. Coverage is measured against F1 feature_inventory jobs instead.
func verifyPhase1GranularGate(rt *Runtime) error {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil
	}
	inv, err := loadFeatureInventory(rt.WorkDir)
	if err != nil {
		return utils.Errorf("Phase1 验证闸门未通过：feature_inventory 缺失（%v）", err)
	}

	// NEW: Trust CoverageSignalReAct verdict as the authoritative coverage decision.
	// If verdict is empty (ReAct didn't run or failed), gate fails with actionable advice.
	if ok, reason := verifyCoverageSignalVerdict(rt); !ok {
		return utils.Errorf("Phase1 验证闸门未通过：%s", reason)
	}

	// NEW: Code-only coverage check uses feature_work_progress done-set, not registry.
	if ok, reason := allCodeOnlyUnitsPresent(rt, inv); !ok {
		return utils.Errorf("Phase1 验证闸门未通过：%s", reason)
	}

	apiMap, err := loadFeatureApiMap(rt.WorkDir)
	if err != nil {
		return utils.Errorf("Phase1 验证闸门未通过：feature_api_map 缺失（%v）", err)
	}
	httpFeatures := map[string]bool{}
	for _, feat := range inv.Features {
		if strings.TrimSpace(feat.SurfaceKind) == SurfaceKindHTTPAPI {
			httpFeatures[feat.FeatureID] = true
		}
	}
	processed := map[string]bool{}
	for _, f := range apiMap.Features {
		if f.Processed {
			processed[f.FeatureID] = true
		}
	}
	for featID := range httpFeatures {
		if !processed[featID] {
			return utils.Errorf("Phase1 验证闸门未通过：http_api 功能项 %s 未完成验证", featID)
		}
	}
	for _, f := range apiMap.Features {
		if !httpFeatures[f.FeatureID] {
			continue
		}
		if len(f.Apis) == 0 && strings.TrimSpace(f.NoApiReason) == "" {
			return utils.Errorf("Phase1 验证闸门未通过：功能项 %s 无 API 且缺少 no_api_reason", f.FeatureID)
		}
	}
	if !rt.Session.TargetReachable {
		return verifyFeatureInventoryGateOffline(rt)
	}
	if phase1AuthRequired(rt) {
		if ready, reason := EvaluatePhase1AuthCalibrationReadiness(rt); !ready {
			return utils.Errorf("Phase1 验证闸门未通过：鉴权校准未就绪（%s）", reason)
		}
	}
	routes := collectHttpApiRoutesFromFeatureMap(apiMap, httpFeatures)
	if len(routes) == 0 {
		if rt.Session.TargetReachable && len(httpFeatures) > 0 {
			if n := countSessionVerifiedHttpApisTrue(rt); n == 0 {
				return utils.Errorf("Phase1 验证闸门未通过：靶机可达但 verified_http_apis 中无 verified=true 记录（http_api features=%d）", len(httpFeatures))
			}
		}
		return nil
	}
	vha, err := rt.Repo.ListVerifiedHttpApis(rt.Session.ID)
	if err != nil {
		return err
	}
	covered := countVerifiedRoutesWithProbeEvidence(routes, vha)
	if covered < len(routes) {
		return utils.Errorf("Phase1 验证闸门未通过：http_api 路由=%d 但 verified_http_apis 探测覆盖仅 %d 条（每条须有 probe 证据）", len(routes), covered)
	}
	return nil
}

// verifyCoverageSignalVerdict checks that CoverageSignalReAct has run and produced a verdict.
// This replaces the mechanical allRegistryUnitsCompleted check.
// The verdict is the authoritative coverage decision — if ReAct said "finish" or "continue"
// and produced reasoning, we trust it. Only fail if verdict is empty (ReAct didn't run).
func verifyCoverageSignalVerdict(rt *Runtime) (bool, string) {
	if rt == nil {
		return false, "nil runtime"
	}
	decision, err := loadCoverageSignalDecision(rt)
	if err != nil || decision == nil {
		return false, "CoverageSignalReAct 未运行或决策缺失（需至少一次 CoverageSignalReAct 调用）"
	}
	verdict := strings.TrimSpace(string(decision.Verdict))
	if verdict == "" {
		// ReAct ran but produced no verdict — this is a failure condition.
		// Provide actionable advice about what to do.
		return false, fmt.Sprintf("CoverageSignalReAct verdict 为空（reasoning: %q）；请确保 job 调度结束后 CoverageSignalReAct 正常输出 finish/continue/reprioritize",
			strings.TrimSpace(decision.Reasoning))
	}
	// Verdict is present — trust it. Valid verdicts are: finish, continue, reprioritize.
	validVerdicts := map[string]bool{"finish": true, "continue": true, "reprioritize": true}
	if !validVerdicts[verdict] {
		return false, fmt.Sprintf("CoverageSignalReAct verdict 无效: %q（期望 finish|continue|reprioritize）", verdict)
	}
	return true, ""
}

// loadCoverageSignalDecision loads the persisted coverage_signal_decision artifact.
func loadCoverageSignalDecision(rt *Runtime) (*CoverageSignalDecision, error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil, utils.Error("nil runtime")
	}
	row, err := rt.Repo.GetPhaseArtifact(rt.Session.ID, "coverage_signal_decision")
	if err != nil || row == nil {
		return nil, utils.Error("coverage_signal_decision artifact not found")
	}
	payload := strings.TrimSpace(row.PayloadJSON)
	if payload == "" {
		return nil, utils.Error("coverage_signal_decision payload empty")
	}
	var decision CoverageSignalDecision
	if err := json.Unmarshal([]byte(payload), &decision); err != nil {
		return nil, err
	}
	return &decision, nil
}

func verifyFeatureInventoryGateOffline(rt *Runtime) error {
	inv, err := loadFeatureInventory(rt.WorkDir)
	if err != nil {
		return err
	}
	apiMap, err := loadFeatureApiMap(rt.WorkDir)
	if err != nil {
		return err
	}
	httpCount := 0
	for _, feat := range inv.Features {
		if strings.TrimSpace(feat.SurfaceKind) == SurfaceKindHTTPAPI {
			httpCount++
		}
	}
	if len(apiMap.Features) < httpCount {
		return utils.Errorf("Phase1 离线闸门：feature_api_map 条目 %d < http_api features %d", len(apiMap.Features), httpCount)
	}
	return nil
}

func countSessionVerifiedHttpApisTrue(rt *Runtime) int {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return 0
	}
	rows, err := rt.Repo.ListVerifiedHttpApis(rt.Session.ID)
	if err != nil {
		return 0
	}
	n := 0
	for i := range rows {
		if rows[i].Verified {
			n++
		}
	}
	return n
}

func collectHttpApiRoutesFromFeatureMap(apiMap *FeatureApiMapV1, httpFeatures map[string]bool) []FeatureApiEntry {
	if apiMap == nil {
		return nil
	}
	var out []FeatureApiEntry
	for _, f := range apiMap.Features {
		if !httpFeatures[f.FeatureID] {
			continue
		}
		for _, a := range f.Apis {
			if a.Verified {
				out = append(out, a)
			}
		}
	}
	return out
}

func countVerifiedRoutesWithProbeEvidence(routes []FeatureApiEntry, vha []store.VerifiedHttpApi) int {
	done := map[string]struct{}{}
	for i := range vha {
		v := &vha[i]
		if !store.VerifiedHttpApiHasProbeEvidence(v) {
			continue
		}
		done[routeKey(v.Method, v.PathPattern)] = struct{}{}
	}
	covered := 0
	for _, a := range routes {
		if _, ok := done[routeKey(a.Method, a.PathPattern)]; ok {
			covered++
		}
	}
	return covered
}

// countPhase1CanonicalRoutes counts verified routes from feature_api_map (http_api features only).
func countPhase1CanonicalRoutes(rt *Runtime) int {
	if rt == nil {
		return 0
	}
	inv, err := loadFeatureInventory(rt.WorkDir)
	if err != nil {
		return 0
	}
	apiMap, err := loadFeatureApiMap(rt.WorkDir)
	if err != nil {
		return 0
	}
	httpFeatures := map[string]bool{}
	for _, feat := range inv.Features {
		if strings.TrimSpace(feat.SurfaceKind) == SurfaceKindHTTPAPI {
			httpFeatures[feat.FeatureID] = true
		}
	}
	return len(collectHttpApiRoutesFromFeatureMap(apiMap, httpFeatures))
}

func countVerifiedCoverageForCanonicalRoutes(rt *Runtime, vha []store.VerifiedHttpApi) int {
	if rt == nil {
		return countVerifiedRoutesWithProbeEvidence(nil, vha)
	}
	inv, _ := loadFeatureInventory(rt.WorkDir)
	apiMap, _ := loadFeatureApiMap(rt.WorkDir)
	httpFeatures := map[string]bool{}
	if inv != nil {
		for _, feat := range inv.Features {
			if strings.TrimSpace(feat.SurfaceKind) == SurfaceKindHTTPAPI {
				httpFeatures[feat.FeatureID] = true
			}
		}
	}
	routes := collectHttpApiRoutesFromFeatureMap(apiMap, httpFeatures)
	return countVerifiedRoutesWithProbeEvidence(routes, vha)
}

func countCanonicalRouteCoverage(rt *Runtime, vha []store.VerifiedHttpApi, requireProbeEvidence bool) int {
	if !requireProbeEvidence {
		return len(vha)
	}
	return countVerifiedCoverageForCanonicalRoutes(rt, vha)
}

func countRouteCandidates(rt *Runtime) int {
	return countPhase1CanonicalRoutes(rt)
}

// Phase1VerificationGateError stops Phase1 when unit artifacts or probe evidence are incomplete.
type Phase1VerificationGateError struct {
	Reason string
}

func (e *Phase1VerificationGateError) Error() string {
	if e == nil {
		return "phase1 verification gate failed"
	}
	return "Phase1 API 验证闸门未通过: " + e.Reason
}

func IsPhase1VerificationGateFailed(err error) bool {
	_, ok := err.(*Phase1VerificationGateError)
	return ok
}

func buildPhase1VerifyDirectlyAnswerOverride() reactloops.ReActLoopOption {
	return reactloops.WithOverrideLoopAction(&reactloops.LoopAction{
		ActionType: "directly_answer",
		Description: "Phase1C：全部候选已写入 verified_http_apis 后简短收尾。",
		Options: []aitool.ToolOption{
			aitool.WithStringParam("answer_payload"),
		},
		AITagStreamFields: []*reactloops.LoopAITagField{
			{TagName: "FINAL_ANSWER", VariableName: "tag_final_answer", AINodeId: "re-act-loop-answer-payload", ContentType: aicommon.TypeTextMarkdown},
		},
		StreamFields: []*reactloops.LoopStreamField{
			{FieldName: "answer_payload", AINodeId: "re-act-loop-answer-payload", ContentType: aicommon.TypeTextMarkdown},
		},
		ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			rt := getRuntime(loop)
			if rt != nil {
				if err := verifyPhase1ApiVerificationGate(rt); err != nil {
					return err
				}
			}
			payload := action.GetString("answer_payload")
			if payload == "" {
				payload = loop.Get("tag_final_answer")
			}
			if payload == "" {
				return utils.Error("answer_payload required")
			}
			loop.Set("directly_answer_payload", payload)
			return nil
		},
		ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			invoker := loop.GetInvoker()
			payload := loop.Get("directly_answer_payload")
			if payload == "" {
				payload = loop.Get("tag_final_answer")
			}
			invoker.EmitFileArtifactWithExt("directly_answer", ".md", payload)
			invoker.EmitResultAfterStream(payload)
			invoker.AddToTimeline("directly_answer", "phase1_verify_gate: "+utils.ShrinkString(payload, 800))
			op.Exit()
		},
	})
}

func phase1GateBlockedFeedback(loop *reactloops.ReActLoop, op *reactloops.LoopActionHandlerOperator) bool {
	rt := getRuntime(loop)
	if rt == nil {
		return false
	}
	if err := verifyPhase1ApiVerificationGate(rt); err != nil {
		refreshPhase1VerifyLoopVars(loop, rt)
		op.Feedback(err.Error())
		op.Continue()
		return true
	}
	return false
}

func refreshPhase1GateStatusLine(rt *Runtime) string {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return ""
	}
	total, verified, _ := rt.Repo.CountVerifiedHttpApis(rt.Session.ID)
	vha, _ := rt.Repo.ListVerifiedHttpApis(rt.Session.ID)
	probed := countVerifiedCoverageForCanonicalRoutes(rt, vha)
	return fmt.Sprintf("verified_http_apis total=%d verified=%d probed_routes=%d canonical_routes=%d", total, verified, probed, countPhase1CanonicalRoutes(rt))
}
