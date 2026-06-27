package loop_ssa_api_discovery

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/phase1_auth_realm_playbook.txt
var phase1AuthRealmPlaybook string

//go:embed prompts/phase1_auth_mechanism_playbook.txt
var phase1AuthMechanismPlaybook string

//go:embed prompts/phase1_auth_surface_playbook.txt
var phase1AuthSurfacePlaybook string

func runPhase1AuthChain(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime) error {
	if rt == nil || rt.Session == nil || !rt.Session.CodePathOK {
		return nil
	}
	_ = ctx
	if err := runPhase1AuthRealmReAct(r, task, rt); err != nil {
		return err
	}
	realmInv, err := loadAuthRealmInventory(rt.WorkDir)
	if err != nil || realmInv == nil || len(realmInv.Realms) == 0 {
		return utils.Error("auth_realm_inventory missing after A1")
	}
	mechanisms := map[string]AuthMechanismDetailV1{}
	for _, realm := range realmInv.Realms {
		detail, err := runPhase1AuthMechanismReAct(r, task, rt, realm.AuthRealm)
		if err != nil {
			log.Warnf("ssa_api_discovery: auth mechanism %s: %v", realm.AuthRealm, err)
			continue
		}
		if detail != nil {
			mechanisms[realm.AuthRealm] = *detail
			if err := persistAuthMechanismDetail(rt, detail); err != nil {
				log.Warnf("ssa_api_discovery: persist auth mechanism %s: %v", realm.AuthRealm, err)
			}
		}
	}
	return runPhase1AuthSurfaceReAct(r, task, rt, realmInv, mechanisms)
}

func authStepSafeName(s string) string {
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "/", "_")
	return s
}

func runPhase1AuthRealmReAct(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime) error {
	step := "phase1.auth_realm"
	started := time.Now()
	rt.execStepStart(step, "ai")
	extra := embeddedArtifactsForAgent(rt,
		store.RoutingProfilePath(rt.WorkDir),
		store.ComponentPackageMapPath(rt.WorkDir),
		store.BackendScopePath(rt.WorkDir),
	)
	loop, err := buildPhase1AuthRealmLoop(r, rt, extra)
	if err != nil {
		rt.execStepError(step, "ai", started, err, nil)
		return err
	}
	if err := runPhase1ReActLoop(task, "phase1_auth_realm", loop); err != nil {
		log.Warnf("ssa_api_discovery: auth realm react: %v; bootstrap", err)
		bootErr := bootstrapAuthRealmInventory(rt)
		if bootErr != nil {
			rt.execStepError(step, "ai", started, bootErr, nil)
		} else {
			rt.execStepEnd(step, "ai", started, []string{store.AuthRealmInventoryPath(rt.WorkDir)})
		}
		return bootErr
	}
	raw := strings.TrimSpace(loop.Get("auth_realm_inventory_committed"))
	if raw == "" {
		bootErr := bootstrapAuthRealmInventory(rt)
		if bootErr != nil {
			rt.execStepError(step, "ai", started, bootErr, nil)
		} else {
			rt.execStepEnd(step, "ai", started, []string{store.AuthRealmInventoryPath(rt.WorkDir)})
		}
		return bootErr
	}
	var inv AuthRealmInventoryV1
	if err := json.Unmarshal([]byte(raw), &inv); err != nil {
		rt.execStepError(step, "ai", started, err, nil)
		return err
	}
	if err := persistAuthRealmInventory(rt, &inv); err != nil {
		rt.execStepError(step, "ai", started, err, nil)
		return err
	}
	rt.execStepEnd(step, "ai", started, []string{store.AuthRealmInventoryPath(rt.WorkDir)})
	return nil
}

func buildPhase1AuthRealmLoop(r aicommon.AIInvokeRuntime, rt *Runtime, extra string) (*reactloops.ReActLoop, error) {
	preset := phase1AgentBaseOptions(r, rt, phase1AuthRealmPlaybook, extra)
	preset = append(preset, phase1AgentSearchOptions()...)
	preset = append(preset, buildFinalizeAuthRealmInventory(rt), buildBlockedDirectlyAnswer("finalize_auth_realm_inventory"))
	return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY_AUTH_REALM, r, preset...)
}

func buildFinalizeAuthRealmInventory(rt *Runtime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"finalize_auth_realm_inventory",
		"Commit auth_realm_inventory.json and exit.",
		[]aitool.ToolOption{aitool.WithStringParam("inventory_json", aitool.WithParam_Required(true))},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			raw := strings.TrimSpace(action.GetString("inventory_json"))
			var inv AuthRealmInventoryV1
			if err := parseAgentJSONObject(raw, &inv); err != nil {
				op.Feedback("invalid inventory_json: " + err.Error())
				op.Continue()
				return
			}
			if len(inv.Realms) == 0 {
				op.Feedback("realms required")
				op.Continue()
				return
			}
			if err := persistAuthRealmInventory(rt, &inv); err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			b, _ := json.MarshalIndent(inv, "", "  ")
			loop.Set("auth_realm_inventory_committed", string(b))
			op.Exit()
		},
	)
}

func bootstrapAuthRealmInventory(rt *Runtime) error {
	inv := &AuthRealmInventoryV1{
		SchemaVersion: artifactV2SchemaVersion,
		MultiAuth:     false,
		Realms: []AuthRealmSummary{
			{AuthRealm: "public", MountPrefix: "/", Label: "default public", Evidence: "bootstrap"},
		},
	}
	if phase1AuthRequired(rt) {
		inv.Realms = []AuthRealmSummary{
			{AuthRealm: "admin", MountPrefix: "/admin", Evidence: "bootstrap"},
			{AuthRealm: "web", MountPrefix: "/", Evidence: "bootstrap"},
		}
		inv.MultiAuth = true
	}
	return persistAuthRealmInventory(rt, inv)
}

func runPhase1AuthMechanismReAct(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime, authRealm string) (*AuthMechanismDetailV1, error) {
	step := fmt.Sprintf("phase1.auth_mechanism.%s", authStepSafeName(authRealm))
	started := time.Now()
	rt.execStepStart(step, "ai")
	extra := buildAuthMechanismExtraContext(rt, authRealm)
	loop, err := buildPhase1AuthMechanismLoop(r, rt, authRealm, extra)
	if err != nil {
		rt.execStepError(step, "ai", started, err, nil)
		return nil, err
	}
	subName := "phase1_auth_mechanism_" + authRealm
	if err := runPhase1ReActLoop(task, subName, loop); err != nil {
		rt.execStepEnd(step, "ai", started, nil)
		return bootstrapAuthMechanism(rt, authRealm), nil
	}
	raw := strings.TrimSpace(loop.Get("auth_mechanism_committed"))
	if raw == "" {
		rt.execStepEnd(step, "ai", started, nil)
		return bootstrapAuthMechanism(rt, authRealm), nil
	}
	var d AuthMechanismDetailV1
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		rt.execStepError(step, "ai", started, err, nil)
		return nil, err
	}
	if err := persistAuthMechanismDetail(rt, &d); err != nil {
		log.Warnf("ssa_api_discovery: persist auth mechanism %s: %v", authRealm, err)
	}
	rt.execStepEnd(step, "ai", started, []string{store.AuthMechanismMapPath(rt.WorkDir)})
	return &d, nil
}

func buildPhase1AuthMechanismLoop(r aicommon.AIInvokeRuntime, rt *Runtime, authRealm, extra string) (*reactloops.ReActLoop, error) {
	preset := phase1AgentBaseOptions(r, rt, phase1AuthMechanismPlaybook, extra)
	preset = append(preset, phase1AgentSearchOptions()...)
	preset = append(preset,
		buildAuthAwareHTTPAction(r, rt, nil),
		buildFinalizeAuthMechanism(authRealm),
		buildBlockedDirectlyAnswer("finalize_auth_mechanism"),
	)
	return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY_AUTH_MECHANISM, r, preset...)
}

func buildFinalizeAuthMechanism(authRealm string) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"finalize_auth_mechanism",
		"Commit auth mechanism detail for current realm and exit.",
		[]aitool.ToolOption{aitool.WithStringParam("mechanism_json", aitool.WithParam_Required(true))},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			raw := strings.TrimSpace(action.GetString("mechanism_json"))
			var d AuthMechanismDetailV1
			if err := parseAgentJSONObject(raw, &d); err != nil {
				op.Feedback("invalid mechanism_json: " + err.Error())
				op.Continue()
				return
			}
			if strings.TrimSpace(d.AuthRealm) == "" {
				d.AuthRealm = authRealm
			}
			b, _ := json.MarshalIndent(d, "", "  ")
			loop.Set("auth_mechanism_committed", string(b))
			op.Exit()
		},
	)
}

func bootstrapAuthMechanism(rt *Runtime, authRealm string) *AuthMechanismDetailV1 {
	mp := "/"
	if authRealm == "admin" {
		mp = "/admin"
	}
	kind := "unknown"
	if authRealm == "admin" {
		kind = "backend"
	} else if authRealm == "web" {
		kind = "frontend"
	}
	return &AuthMechanismDetailV1{
		AuthRealm:         authRealm,
		SessionMechanism:  "cookie_session",
		LoginMethod:       "POST",
		LoginPath:         mp + "/login",
		LoginPostPath:     mp + "/login",
		LoginPagePath:     mp + "/login",
		LoginPageKind:     kind,
		ContentType:       "application/x-www-form-urlencoded",
		MechanismDetail:   "bootstrap",
	}
}

func runPhase1AuthSurfaceReAct(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime, realmInv *AuthRealmInventoryV1, mechanisms map[string]AuthMechanismDetailV1) error {
	step := "phase1.auth_surface"
	started := time.Now()
	rt.execStepStart(step, "ai")
	mechJSON, _ := json.MarshalIndent(mechanisms, "", "  ")
	extra := fmt.Sprintf("## Mechanism details\n```json\n%s\n```\n\n", string(mechJSON)) + embeddedArtifactsForAgent(rt,
		store.AuthRealmInventoryPath(rt.WorkDir),
		store.AuthMechanismMapPath(rt.WorkDir),
		store.ComponentPackageMapPath(rt.WorkDir),
		store.RoutingProfilePath(rt.WorkDir),
	)
	loop, err := buildPhase1AuthSurfaceLoop(r, rt, extra)
	if err != nil {
		rt.execStepError(step, "ai", started, err, nil)
		return err
	}
	if err := runPhase1ReActLoop(task, "phase1_auth_surface", loop); err != nil {
		log.Warnf("ssa_api_discovery: auth surface react: %v; bootstrap", err)
		bootErr := bootstrapAuthSurfaceMap(rt, realmInv, mechanisms)
		if bootErr != nil {
			rt.execStepError(step, "ai", started, bootErr, []string{store.AuthSurfacePath(rt.WorkDir), store.AuthEvidencePath(rt.WorkDir)})
		} else {
			rt.execStepEnd(step, "ai", started, []string{store.AuthSurfacePath(rt.WorkDir), store.AuthEvidencePath(rt.WorkDir)})
		}
		return bootErr
	}
	raw := strings.TrimSpace(loop.Get("auth_surface_map_committed"))
	if raw == "" {
		bootErr := bootstrapAuthSurfaceMap(rt, realmInv, mechanisms)
		if bootErr != nil {
			rt.execStepError(step, "ai", started, bootErr, []string{store.AuthSurfacePath(rt.WorkDir), store.AuthEvidencePath(rt.WorkDir)})
		} else {
			rt.execStepEnd(step, "ai", started, []string{store.AuthSurfacePath(rt.WorkDir), store.AuthEvidencePath(rt.WorkDir)})
		}
		return bootErr
	}
	var m AuthSurfaceMapV1
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		rt.execStepError(step, "ai", started, err, nil)
		return err
	}
	if err := validateAuthSurfaceMap(&m); err != nil {
		rt.execStepError(step, "ai", started, err, nil)
		return err
	}
	if err := persistAuthSurfaceMap(rt, &m); err != nil {
		rt.execStepError(step, "ai", started, err, nil)
		return err
	}
	rt.execStepEnd(step, "ai", started, []string{store.AuthSurfacePath(rt.WorkDir), store.AuthEvidencePath(rt.WorkDir)})
	return nil
}

func buildPhase1AuthSurfaceLoop(r aicommon.AIInvokeRuntime, rt *Runtime, extra string) (*reactloops.ReActLoop, error) {
	preset := phase1AgentBaseOptions(r, rt, phase1AuthSurfacePlaybook, extra)
	preset = append(preset, phase1AgentSearchOptions()...)
	preset = append(preset, buildFinalizeAuthSurfaceMap(rt), buildBlockedDirectlyAnswer("finalize_auth_surface_map"))
	return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY_AUTH_SURFACE, r, preset...)
}

func buildFinalizeAuthSurfaceMap(rt *Runtime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"finalize_auth_surface_map",
		"Commit auth_surface_map.json and exit.",
		[]aitool.ToolOption{aitool.WithStringParam("surface_map_json", aitool.WithParam_Required(true))},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			raw := strings.TrimSpace(action.GetString("surface_map_json"))
			var m AuthSurfaceMapV1
			if err := parseAgentJSONObject(raw, &m); err != nil {
				op.Feedback("invalid surface_map_json: " + err.Error())
				op.Continue()
				return
			}
			if err := validateAuthSurfaceMap(&m); err != nil {
				op.Feedback("validation: " + err.Error())
				op.Continue()
				return
			}
			if err := persistAuthSurfaceMap(rt, &m); err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			b, _ := json.MarshalIndent(m, "", "  ")
			loop.Set("auth_surface_map_committed", string(b))
			op.Exit()
		},
	)
}

func bootstrapAuthSurfaceMap(rt *Runtime, realmInv *AuthRealmInventoryV1, mechanisms map[string]AuthMechanismDetailV1) error {
	comp, _ := loadComponentPackageMap(rt.WorkDir)
	m := &AuthSurfaceMapV1{
		SchemaVersion: artifactV2SchemaVersion,
		MultiAuth:     realmInv != nil && realmInv.MultiAuth,
		Surfaces:      []AuthSurfaceEntry{},
	}
	if realmInv != nil {
		for _, r := range realmInv.Realms {
			s := AuthSurfaceEntry{
				AuthRealm:   r.AuthRealm,
				URLSpace:    r.URLSpace,
				MountPrefix: r.MountPrefix,
			}
			if mech, ok := mechanisms[r.AuthRealm]; ok {
				s.SessionMechanism = mech.SessionMechanism
				s.PasswordTransform = mech.PasswordTransform
				s.LoginPath = mech.LoginPath
				s.LoginMethod = mech.LoginMethod
				s.LoginPageKind = mech.LoginPageKind
				s.LoginPagePath = mech.LoginPagePath
				s.LoginPostPath = mech.LoginPostPath
				s.LoginFormFields = mech.LoginFormFields
				s.ContentType = mech.ContentType
				s.MechanismDetail = mech.MechanismDetail
				s.FilterChain = mech.FilterChain
				s.CodeEvidence = mech.CodeEvidence
			}
			if comp != nil {
				for _, c := range comp.Components {
					if c.ControllerLayer == r.AuthRealm || (r.AuthRealm == "web" && c.ControllerLayer == "web") {
						s.PackagePatterns = append(s.PackagePatterns, c.PackagePatterns...)
					}
				}
			}
			if len(s.PathPrefixes) == 0 && s.MountPrefix != "" {
				s.PathPrefixes = []string{s.MountPrefix}
			}
			m.Surfaces = append(m.Surfaces, s)
		}
	}
	return persistAuthSurfaceMap(rt, m)
}
