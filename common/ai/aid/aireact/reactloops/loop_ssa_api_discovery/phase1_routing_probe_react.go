package loop_ssa_api_discovery

import (
	"context"
	_ "embed"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/phase1_routing_probe_playbook.txt
var phase1RoutingProbePlaybook string

func runPhase1RoutingProbeReAct(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime) error {
	if rt == nil || rt.Session == nil || !rt.Session.CodePathOK {
		return nil
	}
	_ = ctx
	extra := embeddedArtifactsForAgent(rt,
		store.ProjectProfilePath(rt.WorkDir),
		store.BackendScopePath(rt.WorkDir),
	)
	loop, err := buildPhase1RoutingProbeLoop(r, rt, extra)
	if err != nil {
		return err
	}
	if err := runPhase1ReActLoop(task, "phase1_routing_probe", loop); err != nil {
		log.Warnf("ssa_api_discovery: routing probe react: %v; programmatic fallback", err)
		return bootstrapRoutingProfileFromComponentMap(rt)
	}
	if strings.TrimSpace(loop.Get("routing_profile_committed")) == "" {
		return bootstrapRoutingProfileFromComponentMap(rt)
	}
	return nil
}

func buildPhase1RoutingProbeLoop(r aicommon.AIInvokeRuntime, rt *Runtime, extra string) (*reactloops.ReActLoop, error) {
	preset := phase1AgentBaseOptions(r, rt, phase1RoutingProbePlaybook, extra)
	preset = append(preset, phase1AgentSearchOptions()...)
	preset = append(preset,
		buildRoutingSaveDraft(),
		buildRoutingCommitProfile(),
		buildBlockedDirectlyAnswer("routing_commit_profile"),
	)
	return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY_ROUTING_PROBE, r, preset...)
}

func bootstrapRoutingProfileFromConfig(rt *Runtime) error {
	if rt == nil {
		return utils.Error("nil runtime")
	}
	profile, _ := loadProjectProfile(rt.WorkDir)
	stages, _ := loadAllCodeReadingStages(rt.WorkDir)
	rp := &RoutingProfileV1{
		SchemaVersion:    routingProfileSchemaVersion,
		ValidationStatus: "provisional",
		Target: RoutingProfileTarget{
			Raw:             rt.Session.TargetRaw,
			EffectiveOrigin: EffectiveTargetBaseURL(rt.Session),
			ContextPath:     "/",
		},
	}
	if profile != nil && profile.ContextPath != "" && profile.ContextPath != "unknown" {
		rp.Target.ContextPath = normURLPath(profile.ContextPath)
	}
	seen := map[string]struct{}{}
	for _, st := range stages {
		for _, rf := range st.RoutingFacts {
			mp := normURLPath(rf.MountPrefix)
			if mp == "" {
				continue
			}
			if _, ok := seen[mp]; ok {
				continue
			}
			seen[mp] = struct{}{}
			rp.URLSpaces = append(rp.URLSpaces, RoutingURLSpace{
				ID:          "bootstrap_" + strings.TrimPrefix(mp, "/"),
				MountPrefix: mp,
				Confidence:  rf.Confidence,
			})
		}
	}
	if len(rp.URLSpaces) == 0 {
		rp.URLSpaces = append(rp.URLSpaces, RoutingURLSpace{
			ID: "default", MountPrefix: "/", Confidence: 0.4,
		})
	}
	base := EffectiveTargetBaseURL(rt.Session)
	for _, sp := range rp.URLSpaces {
		rp.EffectiveBases = append(rp.EffectiveBases, RoutingEffectiveBase{
			SpaceID: sp.ID,
			BaseURL: strings.TrimSuffix(base, "/") + sp.MountPrefix,
		})
	}
	canonical, err := CanonicalRoutingProfileJSON(rp)
	if err != nil {
		return err
	}
	_ = rt.Repo.UpdateSessionFields(rt.Session.UUID, map[string]interface{}{
		"routing_profile_json": canonical,
	})
	return WriteRoutingProfileFile(rt.WorkDir, canonical)
}

func verifyRoutingProbeGate(rt *Runtime) error {
	if rt == nil {
		return utils.Error("nil runtime")
	}
	rp, err := loadRoutingProfileFromWorkDir(rt.WorkDir)
	if err != nil {
		return utils.Wrap(err, "routing_profile missing")
	}
	return validateRoutingProfileMountPrefixes(rp)
}
