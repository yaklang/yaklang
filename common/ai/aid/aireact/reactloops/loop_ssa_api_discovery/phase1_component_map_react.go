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

//go:embed prompts/phase1_component_map_playbook.txt
var phase1ComponentMapPlaybook string

func runPhase1ComponentMapReAct(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime) error {
	if rt == nil || rt.Session == nil || !rt.Session.CodePathOK {
		return nil
	}
	_ = ctx
	extra := embeddedArtifactsForAgent(rt,
		store.TechArchitecturePath(rt.WorkDir),
		store.BackendScopePath(rt.WorkDir),
		store.JavaBusinessScopeInventoryPath(rt.WorkDir),
		store.RoutingProfilePath(rt.WorkDir),
	)
	loop, err := buildPhase1ComponentMapLoop(r, rt, extra)
	if err != nil {
		return err
	}
	if err := runPhase1ReActLoop(task, "phase1_component_map", loop); err != nil {
		log.Warnf("ssa_api_discovery: component map react: %v; programmatic fallback", err)
		return bootstrapComponentPackageMap(rt)
	}
	raw := strings.TrimSpace(loop.Get("component_package_map_committed"))
	if raw == "" {
		return bootstrapComponentPackageMap(rt)
	}
	var m ComponentPackageMapV1
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return err
	}
	return persistComponentPackageMap(rt, &m)
}

func buildPhase1ComponentMapLoop(r aicommon.AIInvokeRuntime, rt *Runtime, extra string) (*reactloops.ReActLoop, error) {
	preset := phase1AgentBaseOptions(r, rt, phase1ComponentMapPlaybook, extra)
	preset = append(preset, phase1AgentSearchOptions()...)
	preset = append(preset, buildFinalizeComponentPackageMap(rt), buildBlockedDirectlyAnswer("finalize_component_package_map"))
	return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY_COMPONENT_MAP, r, preset...)
}

func buildFinalizeComponentPackageMap(rt *Runtime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"finalize_component_package_map",
		"Commit component_package_map.json and exit.",
		[]aitool.ToolOption{
			aitool.WithStringParam("map_json", aitool.WithParam_Required(true)),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			raw := strings.TrimSpace(action.GetString("map_json"))
			var m ComponentPackageMapV1
			if err := parseAgentJSONObject(raw, &m); err != nil {
				op.Feedback("invalid map_json: " + err.Error())
				op.Continue()
				return
			}
			if len(m.Components) == 0 {
				op.Feedback("components required")
				op.Continue()
				return
			}
			if err := persistComponentPackageMap(rt, &m); err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			b, _ := json.MarshalIndent(m, "", "  ")
			loop.Set("component_package_map_committed", string(b))
			op.Feedback("component_package_map persisted")
			op.Exit()
		},
	)
}

func bootstrapComponentPackageMap(rt *Runtime) error {
	inv, _ := loadJavaBusinessScopeInventory(rt.WorkDir)
	m := &ComponentPackageMapV1{
		SchemaVersion: artifactV2SchemaVersion,
		Language:      rt.Session.Language,
		Components:    []ComponentPackageEntry{},
	}
	if inv != nil {
		layers := map[string][]string{}
		for _, u := range collectRequiredScopeUnits(inv) {
			layer := inferControllerLayerFromPath(u.Path)
			layers[layer] = append(layers[layer], u.Path)
		}
		for layer, paths := range layers {
			m.Components = append(m.Components, ComponentPackageEntry{
				ID:              "bootstrap_" + layer,
				Label:           layer + " controllers",
				PackagePatterns: paths,
				ControllerLayer: layer,
				EvidenceRefs:    []string{"java_business_scope_inventory"},
			})
		}
	}
	if len(m.Components) == 0 {
		m.Components = append(m.Components, ComponentPackageEntry{
			ID: "default", PackagePatterns: []string{"*"}, ControllerLayer: "unknown",
		})
	}
	return persistComponentPackageMap(rt, m)
}

func inferControllerLayerFromPath(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.Contains(lower, ".controller.admin."):
		return "admin"
	case strings.Contains(lower, ".controller.web."):
		return "web"
	case strings.Contains(lower, ".controller.api."):
		return "api"
	default:
		return "unknown"
	}
}
