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
)

//go:embed prompts/phase1_frontend_api_playbook.txt
var phase1FrontendAPIPlaybook string

func runPhase1FrontendAPIReAct(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime) error {
	if rt == nil || rt.Session == nil || !rt.Session.CodePathOK {
		return nil
	}
	ok, reason := shouldRunFrontendAPIAnalysis(rt)
	if !ok {
		log.Infof("ssa_api_discovery: frontend_api_inventory skipped (%s)", reason)
		return nil
	}
	_ = ctx
	harvest, err := loadFrontendAPIHarvest(rt.WorkDir)
	if err != nil || harvest == nil || len(harvest.Calls) == 0 {
		if _, herr := RunFrontendAPIHarvest(rt); herr != nil {
			log.Warnf("ssa_api_discovery: frontend harvest before react: %v", herr)
		}
		harvest, _ = loadFrontendAPIHarvest(rt.WorkDir)
	}
	extra := embeddedArtifactsForAgent(rt,
		store.FrontendAPIHarvestPath(rt.WorkDir),
		store.ServletRoutingMapPath(rt.WorkDir),
		store.RoutingProfilePath(rt.WorkDir),
		store.BackendScopePath(rt.WorkDir),
	)
	if harvest != nil && len(harvest.Candidates) > 0 {
		b, _ := json.MarshalIndent(harvest.Candidates[:min(20, len(harvest.Candidates))], "", "  ")
		extra += "\n\n## harvest_candidates (top priority)\n```json\n" + string(b) + "\n```"
	}
	loop, err := buildPhase1FrontendAPILoop(r, rt, extra)
	if err != nil {
		return err
	}
	if err := runPhase1ReActLoop(task, "phase1_frontend_api", loop); err != nil {
		log.Warnf("ssa_api_discovery: frontend_api react: %v; bootstrap from harvest", err)
		if harvest != nil {
			return bootstrapFrontendAPIInventoryFromHarvest(rt, harvest)
		}
		return err
	}
	raw := strings.TrimSpace(loop.Get("frontend_api_inventory_committed"))
	if raw == "" {
		if harvest != nil {
			return bootstrapFrontendAPIInventoryFromHarvest(rt, harvest)
		}
		return nil
	}
	var inv FrontendAPIInventory
	if err := json.Unmarshal([]byte(raw), &inv); err != nil {
		return err
	}
	return persistFrontendAPIInventory(rt, &inv)
}

func buildPhase1FrontendAPILoop(r aicommon.AIInvokeRuntime, rt *Runtime, extra string) (*reactloops.ReActLoop, error) {
	preset := phase1AgentBaseOptions(r, rt, phase1FrontendAPIPlaybook, extra)
	preset = append(preset, phase1AgentSearchOptions()...)
	preset = append(preset,
		buildFinalizeFrontendAPIInventory(rt),
		buildBlockedDirectlyAnswer("finalize_frontend_api_inventory"),
	)
	return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY_FRONTEND_API, r, preset...)
}

func buildFinalizeFrontendAPIInventory(rt *Runtime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"finalize_frontend_api_inventory",
		"Commit frontend_api_inventory.json and exit.",
		[]aitool.ToolOption{
			aitool.WithStringParam("inventory_json", aitool.WithParam_Required(true)),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			raw := strings.TrimSpace(action.GetString("inventory_json"))
			var inv FrontendAPIInventory
			if err := parseAgentJSONObject(raw, &inv); err != nil {
				op.Feedback("invalid inventory_json: " + err.Error())
				op.Continue()
				return
			}
			if len(inv.Calls) == 0 {
				op.Feedback("calls must not be empty; merge harvest candidates or read more files")
				op.Continue()
				return
			}
			if err := persistFrontendAPIInventory(rt, &inv); err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			b, _ := json.MarshalIndent(inv, "", "  ")
			loop.Set("frontend_api_inventory_committed", string(b))
			op.Feedback("frontend_api_inventory persisted")
			op.Exit()
		},
	)
}
