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

//go:embed prompts/phase1_framework_toolkit_router_playbook.txt
var phase1FrameworkToolkitRouterPlaybook string

const frameworkToolkitSelectionCommittedKey = "framework_toolkit_selection_committed"

func runFrameworkToolkitRouterReAct(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime) (*FrameworkToolkitSelectionV1, error) {
	if rt == nil {
		return nil, utils.Error("nil runtime")
	}
	if sel, err := loadFrameworkToolkitSelection(rt.WorkDir); err == nil && sel != nil && strings.TrimSpace(sel.FrameworkID) != "" {
		sel.Source = "resume"
		rt.SelectedFrameworkID = sel.FrameworkID
		return sel, nil
	}
	extra := embeddedArtifactsForAgent(rt,
		store.ProjectProfilePath(rt.WorkDir),
		store.JavaBusinessScopeInventoryPath(rt.WorkDir),
		store.BackendScopePath(rt.WorkDir),
	)
	extra += "\n\n## registered_toolkits\n" + strings.Join(ListFrameworkToolkitIDs(), ", ")
	loop, err := buildFrameworkToolkitRouterLoop(r, rt, extra)
	if err != nil {
		return fallbackFrameworkToolkitSelection(rt)
	}
	if err := runPhase1ReActLoop(task, "phase1_framework_toolkit_router", loop); err != nil {
		log.Warnf("ssa_api_discovery: framework toolkit router react: %v; detect fallback", err)
		return fallbackFrameworkToolkitSelection(rt)
	}
	raw := strings.TrimSpace(loop.Get(frameworkToolkitSelectionCommittedKey))
	if raw == "" {
		return fallbackFrameworkToolkitSelection(rt)
	}
	var sel FrameworkToolkitSelectionV1
	if err := json.Unmarshal([]byte(raw), &sel); err != nil {
		return fallbackFrameworkToolkitSelection(rt)
	}
	sel.Source = "react"
	if err := persistFrameworkToolkitSelection(rt, &sel); err != nil {
		return nil, err
	}
	return &sel, nil
}

func fallbackFrameworkToolkitSelection(rt *Runtime) (*FrameworkToolkitSelectionV1, error) {
	if sel, ok := DetectFrameworkToolkit(rt); ok {
		if err := persistFrameworkToolkitSelection(rt, sel); err != nil {
			return nil, err
		}
		return sel, nil
	}
	other := &FrameworkToolkitSelectionV1{
		SchemaVersion: frameworkToolkitSelectionSchemaVersion,
		FrameworkID:   FrameworkToolkitIDOther,
		Confidence:    0,
		Rationale:     "no programmatic detect match",
		Source:        "detect_fallback",
	}
	if err := persistFrameworkToolkitSelection(rt, other); err != nil {
		return nil, err
	}
	return other, nil
}

func buildFrameworkToolkitRouterLoop(r aicommon.AIInvokeRuntime, rt *Runtime, extra string) (*reactloops.ReActLoop, error) {
	preset := phase1AgentBaseOptions(r, rt, phase1FrameworkToolkitRouterPlaybook, extra)
	preset = append(preset, phase1AgentSearchOptions()...)
	preset = append(preset, buildFinalizeFrameworkToolkitSelection(rt), buildBlockedDirectlyAnswer("finalize_framework_toolkit_selection"))
	return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY_FRAMEWORK_TOOLKIT_ROUTER, r, preset...)
}

func buildFinalizeFrameworkToolkitSelection(rt *Runtime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"finalize_framework_toolkit_selection",
		"Commit framework_toolkit_selection.json and exit.",
		[]aitool.ToolOption{
			aitool.WithStringParam("selection_json", aitool.WithParam_Required(true)),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			raw := strings.TrimSpace(action.GetString("selection_json"))
			var sel FrameworkToolkitSelectionV1
			if err := parseAgentJSONObject(raw, &sel); err != nil {
				op.Feedback("invalid selection_json: " + err.Error())
				op.Continue()
				return
			}
			if strings.TrimSpace(sel.FrameworkID) == "" {
				op.Feedback("framework_id required")
				op.Continue()
				return
			}
			sel.FrameworkID = normalizeFrameworkToolkitID(sel.FrameworkID)
			if err := persistFrameworkToolkitSelection(rt, &sel); err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			b, _ := json.MarshalIndent(sel, "", "  ")
			loop.Set(frameworkToolkitSelectionCommittedKey, string(b))
			op.Feedback("framework_toolkit_selection persisted: " + sel.FrameworkID)
			op.Exit()
		},
	)
}

// runPhase1WithFrameworkToolkit handles toolkit-enabled Phase1 branching.
func runPhase1WithFrameworkToolkit(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime) error {
	if rt == nil || !rt.FrameworkToolkitEnabled {
		return runPhase1Redesigned(ctx, r, task, rt)
	}
	if err := runMinimalProgrammaticPrep(ctx, r, rt); err != nil {
		log.Warnf("ssa_api_discovery: toolkit prep: %v", err)
	}
	sel, err := runFrameworkToolkitRouterReAct(ctx, r, task, rt)
	if err != nil {
		return err
	}
	fwID := normalizeFrameworkToolkitID(sel.FrameworkID)
	rt.SelectedFrameworkID = fwID
	if fwID != FrameworkToolkitIDOther && GetFrameworkToolkit(fwID) != nil {
		rt.FrameworkToolkitMode = FrameworkToolkitModeFast
		if err := RunFrameworkToolkit(ctx, r, rt, fwID); err != nil {
			return err
		}
		if _, err := writeRouteCandidatesFromUnifiedEndpoints(rt); err != nil {
			log.Warnf("ssa_api_discovery: route_candidates: %v", err)
		}
		if err := WritePhase1DiscoveryReport(rt); err != nil {
			log.Warnf("ssa_api_discovery: discovery report: %v", err)
		}
		return nil
	}
	rt.FrameworkToolkitMode = FrameworkToolkitModeFallbackAI
	if _, err := RunGenericProgrammaticExtract(rt); err != nil {
		log.Warnf("ssa_api_discovery: generic programmatic extract: %v", err)
	}
	return runPhase1Redesigned(ctx, r, task, rt)
}
