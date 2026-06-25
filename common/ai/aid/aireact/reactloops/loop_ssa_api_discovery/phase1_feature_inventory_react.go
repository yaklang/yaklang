package loop_ssa_api_discovery

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/phase1_feature_inventory_playbook.txt
var phase1FeatureInventoryPlaybook string

func runPhase1FeatureInventoryReAct(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime) error {
	if rt == nil || rt.Session == nil || !rt.Session.CodePathOK {
		return utils.Error("nil runtime")
	}
	_ = ctx
	extra := embeddedArtifactsForAgent(rt,
		store.ComponentPackageMapPath(rt.WorkDir),
		store.BusinessFunctionMapPath(rt.WorkDir),
		store.JavaBusinessScopeInventoryPath(rt.WorkDir),
		// NOTE: code_unit_registry.json is NOT embedded here. The AI reads it via
		// read_file (automatic chunking/pagination) on demand. The registry may be
		// large (hundreds of KB for big projects) and embedding it directly in the
		// prompt would waste tokens; read_file handles pagination efficiently.
	)
	loop, err := buildPhase1FeatureInventoryLoop(r, rt, extra)
	if err != nil {
		return err
	}
	if err := runPhase1ReActLoop(task, "phase1_feature_inventory", loop); err != nil {
		return &Phase1BusinessCoverageError{Reason: "feature inventory react failed: " + err.Error()}
	}
	raw := strings.TrimSpace(loop.Get("feature_inventory_committed"))
	if raw == "" {
		return &Phase1BusinessCoverageError{Reason: "feature inventory not committed"}
	}
	var inv FeatureInventoryV1
	if err := parseAgentJSONObject(raw, &inv); err != nil {
		return err
	}
	if err := validateFeatureInventory(&inv, rt); err != nil {
		return &Phase1BusinessCoverageError{Reason: err.Error()}
	}
	return persistFeatureInventory(rt, &inv)
}

func buildPhase1FeatureInventoryLoop(r aicommon.AIInvokeRuntime, rt *Runtime, extra string) (*reactloops.ReActLoop, error) {
	preset := phase1AgentBaseOptions(r, rt, phase1FeatureInventoryPlaybook, extra)
	preset = append(preset, phase1AgentSearchOptions()...)
	preset = append(preset,
		buildFinalizeFeatureInventory(rt),
		buildBlockedDirectlyAnswer("finalize_feature_inventory"),
		buildFeatureInventoryFinishOverride(),
		// read_file with chunked registry reading can take 15+ consecutive calls.
		// Increase spin thresholds to avoid false positives: 20 for simple detection,
		// 20 for AI detection, 5 for force-exit (was 8/8/3).
		reactloops.WithSameActionTypeSpinThreshold(20),
		reactloops.WithSameLogicSpinThreshold(20),
		reactloops.WithMaxConsecutiveSpinWarnings(5),
	)
	return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY_FEATURE_INVENTORY, r, preset...)
}

func buildFeatureInventoryFinishOverride() reactloops.ReActLoopOption {
	return reactloops.WithOverrideLoopAction(&reactloops.LoopAction{
		ActionType:  "finish",
		Description: "Blocked until finalize_feature_inventory succeeds.",
		ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			if strings.TrimSpace(loop.Get("feature_inventory_committed")) != "" {
				op.Exit()
				return
			}
			op.Feedback("use finalize_feature_inventory to commit feature_inventory.json; do not finish early")
			op.Continue()
		},
	})
}

func buildFinalizeFeatureInventory(rt *Runtime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"finalize_feature_inventory",
		"Commit feature_inventory.json and exit.",
		[]aitool.ToolOption{
			aitool.WithStringParam("inventory_json", aitool.WithParam_Required(true)),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			raw := strings.TrimSpace(action.GetString("inventory_json"))
			var inv FeatureInventoryV1
			if err := parseAgentJSONObject(raw, &inv); err != nil {
				op.Feedback("invalid inventory_json: " + err.Error() + " (submit raw JSON without TAG wrappers)")
				op.Continue()
				return
			}
			if err := validateFeatureInventory(&inv, rt); err != nil {
				op.Feedback("validation: " + err.Error())
				op.Continue()
				return
			}
			if err := persistFeatureInventory(rt, &inv); err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			b, _ := json.MarshalIndent(inv, "", "  ")
			loop.Set("feature_inventory_committed", string(b))
			op.Feedback(fmt.Sprintf("feature_inventory persisted features=%d", len(inv.Features)))
			op.Exit()
		},
	)
}

func sanitizeFeatureID(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}
