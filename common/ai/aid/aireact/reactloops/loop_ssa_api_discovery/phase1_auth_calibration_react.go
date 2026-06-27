package loop_ssa_api_discovery

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/phase1_auth_calibration_playbook.txt
var phase1AuthCalibrationPlaybook string

func runPhase1AuthCalibrationChain(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime) error {
	if rt == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	_ = ctx
	if !rt.Session.TargetReachable {
		log.Infof("ssa_api_discovery: skip auth calibration (target unreachable)")
		c := &AuthCalibrationV1{SchemaVersion: artifactV2SchemaVersion, AllCalibrated: true}
		return persistAuthCalibration(rt, c)
	}
	surface, err := loadAuthSurfaceMap(rt.WorkDir)
	if err != nil {
		return err
	}
	cal := &AuthCalibrationV1{
		SchemaVersion: artifactV2SchemaVersion,
		Realms:        []AuthCalibrationRealm{},
	}
	completedLoginIdentities := map[string]struct{}{}
	ev, _ := loadAuthEvidenceFromWorkDir(rt.WorkDir)
	for _, s := range surface.Surfaces {
		realm := NormalizeAuthRealm(s.AuthRealm)
		if hasDirectVerifiedCredentialForRealm(rt, realm) {
			log.Infof("ssa_api_discovery: skip auth calibration react for realm %s (verified credential exists)", realm)
			continue
		}
		identity := loginEndpointIdentity(s.LoginMethod, s.LoginPostPath)
		if identity != "" {
			if _, dup := completedLoginIdentities[identity]; dup {
				n := EnsureSharedLoginCredentialAliases(rt, ev)
				if n > 0 {
					ev, _ = loadAuthEvidenceFromWorkDir(rt.WorkDir)
				}
				if hasDirectVerifiedCredentialForRealm(rt, realm) || realmSatisfiedViaSharedLogin(rt, ev, realm) {
					log.Infof("ssa_api_discovery: skip auth calibration react for realm %s (shared login_post_path)", realm)
					continue
				}
			}
		}
		realmCal, err := runPhase1AuthCalibrationReAct(r, task, rt, s.AuthRealm)
		if err != nil {
			log.Warnf("ssa_api_discovery: auth calibration %s: %v", s.AuthRealm, err)
			continue
		}
		if realmCal != nil {
			cal.Realms = append(cal.Realms, *realmCal)
			if realmCal.Calibrated && identity != "" {
				completedLoginIdentities[identity] = struct{}{}
				ev, _ = loadAuthEvidenceFromWorkDir(rt.WorkDir)
				EnsureSharedLoginCredentialAliases(rt, ev)
			}
		}
	}
	calErr := validateAuthCalibration(cal, surface)
	if calErr != nil {
		if hasVerifiedAuthCredential(rt) {
			log.Warnf("ssa_api_discovery: auth calibration validation relaxed (verified credentials in DB): %v", calErr)
			_ = persistAuthCalibration(rt, cal)
			ev, _ := loadAuthEvidenceFromWorkDir(rt.WorkDir)
			if err := writeAuthStateAfterCalibration(rt, ev, cal); err != nil && !authPartialAuthEnabled(rt) {
				return &Phase1AuthFailedError{Reason: err.Error()}
			}
			_ = RefreshAuthEvidenceFromDB(rt)
			_ = MergeAuthSurfaceIntoRoutingProfile(rt)
			return nil
		}
		if authPartialAuthEnabled(rt) && hasVerifiedAuthCredential(rt) {
			log.Warnf("ssa_api_discovery: partial auth calibration (partial_auth): %v", calErr)
			_ = persistAuthCalibration(rt, cal)
			_, _ = writeAuthState(rt, authStatePartial, "partial auth calibration: "+calErr.Error())
			_ = RefreshAuthEvidenceFromDB(rt)
			return nil
		}
		return &Phase1AuthFailedError{Reason: calErr.Error()}
	}
	if err := persistAuthCalibration(rt, cal); err != nil {
		return err
	}
	if err := RefreshAuthEvidenceFromDB(rt); err != nil {
		log.Warnf("ssa_api_discovery: refresh auth_evidence before gate: %v", err)
	}
	ev, _ = loadAuthEvidenceFromWorkDir(rt.WorkDir)
	if !AuthGateSatisfied(rt, ev) {
		if authPartialAuthEnabled(rt) && !hasVerifiedAuthCredential(rt) {
			return &Phase1AuthFailedError{Reason: "partial_auth enabled but no verified credentials in DB (need at least one successful login)"}
		}
		return &Phase1AuthFailedError{Reason: "auth calibration ok but verified credentials missing for required realms"}
	}
	if err := writeAuthStateAfterCalibration(rt, ev, cal); err != nil {
		return &Phase1AuthFailedError{Reason: err.Error()}
	}
	_ = RefreshAuthEvidenceFromDB(rt)
	_ = MergeAuthSurfaceIntoRoutingProfile(rt)
	return nil
}

func countCalibratedRealms(cal *AuthCalibrationV1) int {
	if cal == nil {
		return 0
	}
	n := 0
	for _, r := range cal.Realms {
		if r.Calibrated {
			n++
		}
	}
	return n
}

func runPhase1AuthCalibrationReAct(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime, authRealm string) (*AuthCalibrationRealm, error) {
	extra := buildAuthCalibrationExtraContext(rt, authRealm)
	loop, err := buildPhase1AuthCalibrationLoop(r, rt, authRealm, extra)
	if err != nil {
		return nil, err
	}
	subName := "phase1_auth_calibration_" + authRealm
	if err := runPhase1ReActLoop(task, subName, loop); err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(loop.Get("auth_calibration_realm_committed"))
	if raw == "" {
		return nil, utils.Errorf("auth calibration not committed for realm %s", authRealm)
	}
	var realm AuthCalibrationRealm
	if err := json.Unmarshal([]byte(raw), &realm); err != nil {
		return nil, err
	}
	return &realm, nil
}

func buildPhase1AuthCalibrationLoop(r aicommon.AIInvokeRuntime, rt *Runtime, authRealm, extra string) (*reactloops.ReActLoop, error) {
	preset := phase1AgentBaseOptions(r, rt, phase1AuthCalibrationPlaybook, extra)
	preset = append(preset, phase1AgentSearchOptions()...)
	preset = append(preset,
		buildAuthAwareHTTPAction(r, rt, &AuthAwareHTTPActionConfig{CalibrationRealm: authRealm}),
		buildDiscoveryFetchCsrfToken(r, rt),
		buildDiscoveryTransformCredential(),
		buildListAuthCredentialsAction(),
		buildSelectAuthCredentialAction(),
		buildUpsertAuthCredentialAction(),
		buildFinalizeAuthCalibrationRealm(authRealm),
		buildBlockedDirectlyAnswer("finalize_auth_calibration_realm"),
	)
	return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY_AUTH_CALIBRATION, r, preset...)
}

func buildFinalizeAuthCalibrationRealm(authRealm string) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"finalize_auth_calibration_realm",
		"Commit calibration result for current auth realm (2 passed probes required).",
		[]aitool.ToolOption{aitool.WithStringParam("calibration_json", aitool.WithParam_Required(true))},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, _, ok := mustRT(loop, op)
			if !ok {
				return
			}
			raw := strings.TrimSpace(action.GetString("calibration_json"))
			var realm AuthCalibrationRealm
			if err := parseAgentJSONObject(raw, &realm); err != nil {
				op.Feedback("invalid calibration_json: " + err.Error())
				op.Continue()
				return
			}
			if strings.TrimSpace(realm.AuthRealm) == "" {
				realm.AuthRealm = authRealm
			}
			if len(realm.Probes) < 2 {
				op.Feedback("at least 2 calibration probes required")
				op.Continue()
				return
			}
			passed := 0
			for i, p := range realm.Probes {
				if strings.TrimSpace(p.Path) == "" {
					op.Feedback(fmt.Sprintf("probes[%d].path required (non-empty URL path)", i))
					op.Continue()
					return
				}
				if strings.TrimSpace(p.Method) == "" {
					realm.Probes[i].Method = "GET"
				}
				if p.Passed {
					passed++
				}
			}
			if passed < 2 {
				op.Feedback(fmt.Sprintf("calibration failed: only %d/2 probes passed; revise auth_surface_map and retry", passed))
				op.Continue()
				return
			}
			if !realmHasVerifiedCredential(rt, authRealm) {
				op.Feedback(fmt.Sprintf(
					"calibration blocked: auth_realm=%q has no verified auth_credential in DB (need headers_json). "+
						"After login POST (302+Set-Cookie), engine programmatic_auto_save should create a row — call discovery_list_auth_credentials. "+
						"If list is still empty, call discovery_upsert_auth_credential (verified=true, headers_json from login response); do not finalize with session cookies in JSON only.",
					authRealm,
				))
				op.Continue()
				return
			}
			realm.Calibrated = true
			b, _ := json.MarshalIndent(realm, "", "  ")
			loop.Set("auth_calibration_realm_committed", string(b))
			op.Exit()
		},
	)
}

func verifyAuthCalibrationGate(rt *Runtime) error {
	if rt == nil || !rt.Session.TargetReachable {
		return nil
	}
	surface, err := loadAuthSurfaceMap(rt.WorkDir)
	if err != nil {
		return err
	}
	cal, err := loadAuthCalibration(rt.WorkDir)
	if err != nil {
		return &Phase1AuthFailedError{Reason: "auth_calibration.json missing: " + err.Error()}
	}
	return validateAuthCalibration(cal, surface)
}

func EvaluatePhase1AuthCalibrationReadiness(rt *Runtime) (ready bool, reason string) {
	if rt == nil || !rt.Session.TargetReachable {
		return true, "target unreachable"
	}
	ev, _ := loadAuthEvidenceFromWorkDir(rt.WorkDir)
	if authPartialAuthEnabled(rt) && len(VerifiedAuthRealmsList(rt, ev)) > 0 {
		if err := verifyAuthCalibrationGate(rt); err != nil {
			if AuthGateSatisfied(rt, ev) {
				missing := missingAuthRealms(rt, ev, RequiredAuthRealms(rt, ev))
				return true, fmt.Sprintf("partial auth (YAK_SSA_AUTH_PARTIAL_OK): verified=%v missing=%v; %v",
					VerifiedAuthRealmsList(rt, ev), missing, err.Error())
			}
		}
	}
	if err := verifyAuthCalibrationGate(rt); err != nil {
		return false, err.Error()
	}
	if !AuthGateSatisfied(rt, ev) {
		if authPartialAuthEnabled(rt) && !hasVerifiedAuthCredential(rt) {
			return false, "partial_auth enabled but no verified auth_credentials in DB"
		}
		return false, "no verified auth_credentials with headers_json for all required auth realms"
	}
	if authStateIsPartial(rt) {
		return true, "partial auth: probing verified realms only"
	}
	return true, "auth_calibration ok"
}
