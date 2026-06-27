package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// Phase1AuthFailedError stops Phase1 when live auth is required but not achieved.
type Phase1AuthFailedError struct {
	Reason string
}

func (e *Phase1AuthFailedError) Error() string {
	if e == nil {
		return "phase1 auth failed"
	}
	return "Phase1 鉴权未通过: " + e.Reason
}

// IsPhase1AuthFailed reports whether err is a deliberate auth-failure stop.
func IsPhase1AuthFailed(err error) bool {
	_, ok := err.(*Phase1AuthFailedError)
	return ok
}

// Phase1AuthFailureReport is structured output when auth blocks full verify.
type Phase1AuthFailureReport struct {
	Version      int                 `json:"version"`
	GeneratedAt  string              `json:"generated_at"`
	SessionUUID  string              `json:"session_uuid"`
	Reason       string              `json:"reason"`
	AuthState    string              `json:"auth_state"`
	AuthEvidence *AuthEvidenceRecord `json:"auth_evidence,omitempty"`
	Learnings    []string            `json:"learnings,omitempty"`
	NextSteps    []string            `json:"next_steps,omitempty"`
}

func phase1AuthRequired(rt *Runtime) bool {
	if rt == nil || rt.Session == nil || !rt.Session.TargetReachable {
		return false
	}
	profile, _ := loadProjectProfile(rt.WorkDir)
	if hasSecurityConfigEntry(profile) {
		return true
	}
	if strings.TrimSpace(rt.UserAuthPassword) != "" {
		return true
	}
	scope, _ := loadBackendScope(rt.WorkDir)
	if scope != nil {
		for _, c := range scope.ControllerFileCandidates {
			rel := strings.ToLower(c.RelPath)
			if isAuthEntryPath(c.RelPath) || strings.Contains(rel, "login") {
				return true
			}
		}
	}
	if ev, _ := loadAuthEvidenceFromWorkDir(rt.WorkDir); ev != nil && len(ev.LoginEndpoints) > 0 {
		return true
	}
	return false
}

func hasVerifiedAuthCredential(rt *Runtime) bool {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return false
	}
	creds, err := rt.Repo.ListAuthCredentials(rt.Session.ID)
	if err != nil {
		return false
	}
	for _, c := range creds {
		if c.Verified && strings.TrimSpace(c.HeadersJSON) != "" {
			return true
		}
	}
	return false
}

func authStateIsFailed(rt *Runtime) bool {
	if rt == nil {
		return false
	}
	b, err := os.ReadFile(store.AuthStatePath(rt.WorkDir))
	if err != nil {
		return false
	}
	var rec authStateRecord
	if json.Unmarshal(b, &rec) != nil {
		return false
	}
	return rec.State == authStateFailed
}

// EvaluatePhase1AuthReadiness returns whether full API verify may proceed.
func EvaluatePhase1AuthReadiness(rt *Runtime) (ready bool, reason string) {
	if rt == nil || rt.Session == nil {
		return false, "nil runtime"
	}
	if !rt.Session.TargetReachable {
		return true, "target unreachable"
	}
	if authStateIsNoAuthNeeded(rt) {
		return true, "no auth required"
	}
	if !phase1AuthRequired(rt) {
		if !authVerifiedFromRuntime(rt) && !authStateIsFailed(rt) {
			_, _ = writeAuthState(rt, authStateNoAuthNeeded, "no security config or login entry detected")
		}
		return true, "no auth required"
	}
	if !authReadyForDownstream(rt) {
		if authPartialAuthEnabled(rt) && hasVerifiedAuthCredential(rt) && !authStateIsPartial(rt) && !authVerifiedFromRuntime(rt) {
			return false, "auth_state is not success or partial"
		}
		return false, "auth_state is not success"
	}
	ev, _ := loadAuthEvidenceFromWorkDir(rt.WorkDir)
	if authVerifiedFromRuntime(rt) {
		if !HasAuthCredentialsSatisfied(rt, ev) {
			return false, "no verified auth_credentials with headers_json in DB"
		}
		return true, ""
	}
	// partial auth: at least one verified realm; downstream filters by realm.
	if len(VerifiedAuthRealmsList(rt, ev)) == 0 {
		return false, "partial auth enabled but no verified credentials"
	}
	return true, "partial auth: limited to verified realms"
}

// MergeFullVerifyPlanAfterAuth expands code_reading_plan with static hints and DB routes after auth OK.
func MergeFullVerifyPlanAfterAuth(rt *Runtime) error {
	if rt == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	plan, _ := LoadCodeReadingPlan(rt.WorkDir)
	if plan == nil {
		plan = &CodeReadingPlan{}
	}
	plan = mergeAuthEndpointsIntoPlan(rt, plan)
	plan = mergeStaticHintsIntoPlan(rt, plan)
	plan = mergeHttpEndpointsIntoPlan(rt, plan)
	if len(plan.DiscoveredAPIs) == 0 {
		if fb, err := BuildFallbackCodeReadingPlanFromStaticHints(rt); err == nil && fb != nil {
			plan = fb
		}
	}
	if len(plan.DiscoveredAPIs) == 0 {
		return utils.Error("full verify plan empty after auth merge")
	}
	plan.HintDiff = strings.TrimSpace(plan.HintDiff)
	if plan.HintDiff == "" {
		plan.HintDiff = "merged after auth: static_hints + http_endpoints + login routes"
	}
	if err := PersistCodeReadingPlan(rt, plan); err != nil {
		return err
	}
	if _, err := SyncAICodeReadingRoutesToEndpoints(rt); err != nil {
		return err
	}
	log.Infof("ssa_api_discovery: full verify plan merged discovered_apis=%d", len(plan.DiscoveredAPIs))
	return nil
}

func mergeAuthEndpointsIntoPlan(rt *Runtime, plan *CodeReadingPlan) *CodeReadingPlan {
	if plan == nil {
		plan = &CodeReadingPlan{}
	}
	ev, err := loadAuthEvidenceFromWorkDir(rt.WorkDir)
	if err != nil || ev == nil {
		return plan
	}
	seen := map[string]struct{}{}
	for _, a := range plan.DiscoveredAPIs {
		seen[routeKey(a.Method, a.PathPattern)] = struct{}{}
	}
	for _, ep := range ev.LoginEndpoints {
		path := strings.TrimSpace(ep.Path)
		if path == "" && strings.TrimSpace(ep.FullURL) != "" {
			path = ep.FullURL
		}
		key := routeKey(ep.Method, path)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		plan.DiscoveredAPIs = append(plan.DiscoveredAPIs, DiscoveredAPI{
			Method:       strings.ToUpper(strings.TrimSpace(ep.Method)),
			PathPattern:  normURLPath(path),
			CodeEvidence: "auth_evidence:" + authEvidenceSummary(ev),
		})
	}
	if strings.TrimSpace(plan.AuthNotes) == "" && ev.VerificationDetail != "" {
		plan.AuthNotes = ev.VerificationDetail
	}
	plan.AuthEvidence = ev
	return plan
}

func mergeHttpEndpointsIntoPlan(rt *Runtime, plan *CodeReadingPlan) *CodeReadingPlan {
	if plan == nil || rt == nil || rt.Repo == nil || rt.Session == nil {
		return plan
	}
	dbPlan, err := BuildCodeReadingPlanFromDB(rt)
	if err != nil || dbPlan == nil {
		return plan
	}
	seen := map[string]struct{}{}
	for _, a := range plan.DiscoveredAPIs {
		seen[routeKey(a.Method, a.PathPattern)] = struct{}{}
	}
	for _, a := range dbPlan.DiscoveredAPIs {
		key := routeKey(a.Method, a.PathPattern)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		plan.DiscoveredAPIs = append(plan.DiscoveredAPIs, a)
	}
	return plan
}

// WritePhase1AuthFailureReport writes structured JSON + markdown summary and syncs DB artifact.
func WritePhase1AuthFailureReport(rt *Runtime, reason string) error {
	if rt == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	ev, _ := loadAuthEvidenceFromWorkDir(rt.WorkDir)
	state := authStateFailed
	if b, err := os.ReadFile(store.AuthStatePath(rt.WorkDir)); err == nil {
		var rec authStateRecord
		if json.Unmarshal(b, &rec) == nil && rec.State != "" {
			state = rec.State
		}
	}
	report := Phase1AuthFailureReport{
		Version:      1,
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		SessionUUID:  rt.Session.UUID,
		Reason:       reason,
		AuthState:    state,
		AuthEvidence: ev,
		Learnings:    buildAuthFailureLearnings(rt, ev, reason),
		NextSteps: []string{
			"检查凭证与页面 evidence 中的客户端变换是否匹配",
			"确认 mount_prefix 与 login_post_path 一致",
			"使用 discovery_upsert_auth_credential 写入 Set-Cookie / CSRF headers",
		},
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	jsonPath := store.Phase1AuthFailureReportPath(rt.WorkDir)
	if err := writeJSONFile(jsonPath, raw); err != nil {
		return err
	}
	if rt.Repo != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactPhase1AuthFailure, string(raw))
	}
	md := renderPhase1AuthFailureMarkdown(&report)
	mdPath := store.Phase1AuthFailureReportMDPath(rt.WorkDir)
	if err := os.WriteFile(mdPath, []byte(md), 0o644); err != nil {
		return err
	}
	log.Infof("ssa_api_discovery: phase1 auth failure report written %s", jsonPath)
	return nil
}

func buildAuthFailureLearnings(rt *Runtime, ev *AuthEvidenceRecord, reason string) []string {
	var out []string
	out = append(out, "gate_reason: "+reason)
	if ev != nil {
		for _, ep := range ev.LoginEndpoints {
			line := fmt.Sprintf("login %s %s ct=%s transform=%s probe_ok=%v",
				ep.Method, ep.Path, ep.ContentType, ep.PasswordTransform, ep.ProbeSucceeded)
			out = append(out, line)
		}
		if ev.VerificationDetail != "" {
			out = append(out, "verification_detail: "+ev.VerificationDetail)
		}
		for _, ce := range ev.CodeEvidence {
			out = append(out, "code: "+ce)
		}
	}
	if rt != nil && rt.Repo != nil && rt.Session != nil {
		creds, _ := rt.Repo.ListAuthCredentials(rt.Session.ID)
		out = append(out, fmt.Sprintf("auth_credentials_rows=%d verified_with_headers=%v", len(creds), hasVerifiedAuthCredential(rt)))
	}
	return out
}

func renderPhase1AuthFailureMarkdown(report *Phase1AuthFailureReport) string {
	if report == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Phase1 鉴权失败报告\n\n")
	b.WriteString(fmt.Sprintf("生成时间: %s\n\n", report.GeneratedAt))
	b.WriteString(fmt.Sprintf("Session: `%s`\n\n", report.SessionUUID))
	b.WriteString(fmt.Sprintf("**状态**: `%s`\n\n", report.AuthState))
	b.WriteString(fmt.Sprintf("**原因**: %s\n\n", report.Reason))
	if report.AuthEvidence != nil {
		b.WriteString("## 鉴权分析 (auth_evidence)\n\n")
		b.WriteString(fmt.Sprintf("- verified: %v\n", report.AuthEvidence.Verified))
		if report.AuthEvidence.SessionMechanism != "" {
			b.WriteString(fmt.Sprintf("- session: %s\n", report.AuthEvidence.SessionMechanism))
		}
		if report.AuthEvidence.VerificationDetail != "" {
			b.WriteString(fmt.Sprintf("- detail: %s\n", report.AuthEvidence.VerificationDetail))
		}
		for i, ep := range report.AuthEvidence.LoginEndpoints {
			b.WriteString(fmt.Sprintf("- endpoint[%d]: %s %s (%s) probe_ok=%v\n",
				i, ep.Method, ep.Path, ep.ContentType, ep.ProbeSucceeded))
		}
		b.WriteString("\n")
	}
	if len(report.Learnings) > 0 {
		b.WriteString("## 已确认信息\n\n")
		for _, l := range report.Learnings {
			b.WriteString("- " + l + "\n")
		}
		b.WriteString("\n")
	}
	if len(report.NextSteps) > 0 {
		b.WriteString("## 建议下一步\n\n")
		for _, s := range report.NextSteps {
			b.WriteString("- " + s + "\n")
		}
	}
	b.WriteString("\n\n> 全量 API 验证已跳过：需至少一套鉴权机制验证成功（auth_state=success 且 DB 中存在 verified auth_credentials）。\n")
	return b.String()
}
