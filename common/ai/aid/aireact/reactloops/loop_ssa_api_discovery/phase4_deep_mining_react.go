package loop_ssa_api_discovery

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/phase4_deep_mining_playbook.txt
var phase4DeepMiningPlaybook string

func buildPhase4DeepMiningLoop(r aicommon.AIInvokeRuntime, rt *Runtime, pl *PipelineState, target HttpProbeTarget) (*reactloops.ReActLoop, error) {
	targetJSON, _ := json.MarshalIndent(target, "", "  ")
	extra := fmt.Sprintf("## 当前探测接口\n```json\n%s\n```\n\n", string(targetJSON)) +
		FormatUserCredentialGroupsInstruction(rt) + "\n\n" +
		"## 漏洞类型注册表\n" + FormatVulnTypeRegistryForPrompt() + "\n\n" +
		embeddedArtifactsForAgent(rt,
			store.RoutingProfilePath(rt.WorkDir),
			store.FailureSemanticsPath(rt.WorkDir),
			store.AuthCalibrationPath(rt.WorkDir),
		)
	playbook := strings.TrimSpace(phase4DeepMiningPlaybook) + "\n\n" + FormatVulnTypeRegistryForPrompt()
	preset := phase1AgentBaseOptions(r, rt, playbook, extra)
	preset = append(preset, phase1AgentSearchOptions()...)
	preset = append(preset,
		buildListAuthCredentialsAction(),
		buildDiscoveryFetchCsrfToken(r, rt),
		buildAuthAwareHTTPAction(r, rt, &AuthAwareHTTPActionConfig{PinnedTarget: &target}),
		buildDiscoveryReadSessionData(),
		buildRecordVulnProbeAction(rt, target),
		buildFinalizeEndpointDeepMining(rt, pl, target),
		buildBlockedDirectlyAnswer("finalize_endpoint_deep_mining"),
		buildDeepMiningFinishGate(rt, target),
	)
	loopName := fmt.Sprintf("ssa_api_discovery_phase4_deep_mining_%d", target.VerifiedHttpApiID)
	return reactloops.NewReActLoop(loopName, r, preset...)
}

func buildDeepMiningFinishGate(rt *Runtime, target HttpProbeTarget) reactloops.ReActLoopOption {
	return reactloops.WithOverrideLoopAction(&reactloops.LoopAction{
		ActionType:  "finish",
		Description: "Finish only via finalize_endpoint_deep_mining after all vuln types recorded.",
		ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			if strings.TrimSpace(loop.Get("deep_mining_finalized")) != "1" {
				op.Feedback("call finalize_endpoint_deep_mining after discovery_record_vuln_probe for every vuln_type")
				op.Continue()
				return
			}
			op.Exit()
		},
	})
}

func buildRecordVulnProbeAction(rt *Runtime, target HttpProbeTarget) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_record_vuln_probe",
		"Record deep-mining outcome for one vuln_type on the current endpoint.",
		[]aitool.ToolOption{
			aitool.WithStringParam("vuln_type", aitool.WithParam_Required(true)),
			aitool.WithStringParam("status", aitool.WithParam_Required(true), aitool.WithParam_Description("confirmed|safe|uncertain|skipped")),
			aitool.WithStringParam("skip_reason"),
			aitool.WithStringParam("payload"),
			aitool.WithStringParam("request_url"),
			aitool.WithStringParam("response_excerpt"),
			aitool.WithStringParam("ai_analysis"),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt2, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			vulnType := strings.TrimSpace(action.GetString("vuln_type"))
			status := strings.TrimSpace(action.GetString("status"))
			if vulnType == "" || status == "" {
				op.Feedback("vuln_type and status required")
				op.Continue()
				return
			}
			if _, ok := VulnTypeDefByID(vulnType); !ok {
				op.Feedback("unknown vuln_type: " + vulnType)
				op.Continue()
				return
			}
			if status == "skipped" && strings.TrimSpace(action.GetString("skip_reason")) == "" {
				op.Feedback("skip_reason required when status=skipped")
				op.Continue()
				return
			}
			apiID := target.VerifiedHttpApiID
			if apiID == 0 {
				apiID = target.ID
			}
			row := &store.EndpointVulnProbe{
				SessionID:         sess.ID,
				VerifiedHttpApiID: apiID,
				VulnType:          vulnType,
				Status:            status,
				SkipReason:        action.GetString("skip_reason"),
				Payload:           action.GetString("payload"),
				RequestURL:        action.GetString("request_url"),
				ResponseExcerpt:   action.GetString("response_excerpt"),
				AIAnalysis:        action.GetString("ai_analysis"),
				Source:            "deep_mining",
			}
			if err := rt2.Repo.UpsertEndpointVulnProbe(row); err != nil {
				op.Feedback("upsert probe: " + err.Error())
				op.Continue()
				return
			}
			if status == "confirmed" {
				_ = syncConfirmedProbeToDynamicFinding(rt2, target, *row)
			}
			op.Feedback(fmt.Sprintf("saved probe api=%d vuln_type=%s status=%s", apiID, vulnType, status))
			op.Continue()
		},
	)
}

func buildFinalizeEndpointDeepMining(rt *Runtime, pl *PipelineState, target HttpProbeTarget) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"finalize_endpoint_deep_mining",
		"Validate full vuln_type coverage for current endpoint and exit.",
		[]aitool.ToolOption{
			aitool.WithStringParam("summary", aitool.WithParam_Description("optional one-line summary")),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt2, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			apiID := target.VerifiedHttpApiID
			if apiID == 0 {
				apiID = target.ID
			}
			probes, err := rt2.Repo.ListEndpointVulnProbes(sess.ID, apiID)
			if err != nil {
				op.Feedback("list probes: " + err.Error())
				op.Continue()
				return
			}
			if err := validateEndpointDeepMiningCoverage(sess.ID, apiID, probes); err != nil {
				op.Feedback("finalize blocked: " + err.Error())
				op.Continue()
				return
			}
			if pl != nil {
				pl.MarkDeepMiningDone(apiID)
			}
			loop.Set("deep_mining_finalized", "1")
			op.Feedback(fmt.Sprintf("endpoint %d deep mining complete (%d probe records)", apiID, len(probes)))
			op.Exit()
		},
	)
}

func runPhase4DeepMiningReAct(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime, pl *PipelineState, target HttpProbeTarget) error {
	apiID := target.VerifiedHttpApiID
	if apiID == 0 {
		apiID = target.ID
	}
	step := fmt.Sprintf("phase4.step3.deep_mining.target_%d", apiID)
	started := time.Now()
	rt.execStepStart(step, "ai")
	if pl != nil && pl.IsDeepMiningDone(apiID) {
		rt.execInfo(step, "ai", "skipped — already finalized")
		rt.execStepEnd(step, "ai", started, nil)
		return nil
	}
	loop, err := buildPhase4DeepMiningLoop(r, rt, pl, target)
	if err != nil {
		rt.execStepError(step, "ai", started, err, nil)
		return err
	}
	subName := fmt.Sprintf("phase4_deep_mining_%d", apiID)
	if err := runPhase1ReActLoop(task, subName, loop); err != nil {
		rt.execStepError(step, "ai", started, err, nil)
		return err
	}
	if pl != nil && !pl.IsDeepMiningDone(apiID) {
		err := utils.Errorf("endpoint %d deep mining did not finalize", apiID)
		rt.execStepError(step, "ai", started, err, nil)
		return err
	}
	rt.execStepEnd(step, "ai", started, nil)
	return nil
}