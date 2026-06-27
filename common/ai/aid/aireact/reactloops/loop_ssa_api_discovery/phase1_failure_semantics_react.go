package loop_ssa_api_discovery

import (
	"context"
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/phase1_failure_semantics_playbook.txt
var phase1FailureSemanticsPlaybook string

func runPhase1FailureSemanticsReAct(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime) error {
	if rt == nil || rt.Session == nil || !rt.Session.CodePathOK {
		return nil
	}
	_ = ctx
	extra := embeddedArtifactsForAgent(rt,
		store.AuthSurfaceMapPath(rt.WorkDir),
		store.BackendScopePath(rt.WorkDir),
	)
	loop, err := buildPhase1FailureSemanticsLoop(r, rt, extra)
	if err != nil {
		return err
	}
	if err := runPhase1ReActLoop(task, "phase1_failure_semantics", loop); err != nil {
		log.Warnf("ssa_api_discovery: failure semantics react: %v; default", err)
		return persistFailureSemantics(rt, DefaultFailureSemantics())
	}
	raw := strings.TrimSpace(loop.Get("failure_semantics_committed"))
	if raw == "" {
		return persistFailureSemantics(rt, DefaultFailureSemantics())
	}
	var fs FailureSemanticsV1
	if err := json.Unmarshal([]byte(raw), &fs); err != nil {
		return err
	}
	if err := validateFailureSemantics(&fs); err != nil {
		log.Warnf("ssa_api_discovery: failure semantics validation: %v; merge default", err)
		def := DefaultFailureSemantics()
		fs.Categories = append(fs.Categories, def.Categories...)
	}
	return persistFailureSemantics(rt, &fs)
}

func buildPhase1FailureSemanticsLoop(r aicommon.AIInvokeRuntime, rt *Runtime, extra string) (*reactloops.ReActLoop, error) {
	preset := phase1AgentBaseOptions(r, rt, phase1FailureSemanticsPlaybook, extra)
	preset = append(preset, phase1AgentSearchOptions()...)
	preset = append(preset, buildFinalizeFailureSemantics(rt), buildBlockedDirectlyAnswer("finalize_failure_semantics"))
	return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY_FAILURE_SEMANTICS, r, preset...)
}

func buildFinalizeFailureSemantics(rt *Runtime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"finalize_failure_semantics",
		"Commit failure_semantics.json and exit.",
		[]aitool.ToolOption{aitool.WithStringParam("semantics_json", aitool.WithParam_Required(true))},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			raw := strings.TrimSpace(action.GetString("semantics_json"))
			var fs FailureSemanticsV1
			if err := parseAgentJSONObject(raw, &fs); err != nil {
				op.Feedback("invalid semantics_json: " + err.Error())
				op.Continue()
				return
			}
			if err := validateFailureSemantics(&fs); err != nil {
				op.Feedback("validation: " + err.Error())
				op.Continue()
				return
			}
			if err := persistFailureSemantics(rt, &fs); err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			b, _ := json.MarshalIndent(fs, "", "  ")
			loop.Set("failure_semantics_committed", string(b))
			op.Exit()
		},
	)
}

func verifyFailureSemanticsGate(rt *Runtime) error {
	if rt == nil {
		return utils.Error("nil runtime")
	}
	if !failureSemanticsExists(rt.WorkDir) {
		return utils.Error("failure_semantics.json missing")
	}
	fs, err := loadFailureSemantics(rt.WorkDir)
	if err != nil {
		return err
	}
	return validateFailureSemantics(fs)
}
