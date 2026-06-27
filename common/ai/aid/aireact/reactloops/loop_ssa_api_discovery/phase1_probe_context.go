package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/utils"
)

// Fields marked "auto" are populated by enrichProbeCandidateContext — the agent must infer/verify from raw hints.
type ProbeCandidateContext struct {
	CandidateID        string   `json:"candidate_id"`
	Method             string   `json:"method"`
	PathPattern        string   `json:"path_pattern"`
	HandlerClass       string   `json:"handler_class,omitempty"`
	HandlerFile        string   `json:"handler_file"`
	HandlerSymbol      string   `json:"handler_symbol"`
	CodeSnippet        string   `json:"code_snippet"`
	EffectiveBases     []string `json:"effective_bases"`
	URLSpace           string   `json:"url_space"`
	MaxProbeIterations int      `json:"max_probe_iterations"`

	// Auto-injected probe hints (raw signals only; agent must infer/verify).
	AuthSelectionJSON         string `json:"auth_selection_json,omitempty"`
	AuthEvidenceJSON          string `json:"auth_evidence_json,omitempty"`
	RoutingProfileExcerpt     string `json:"routing_profile_excerpt,omitempty"`
	ForwardingProfileExcerpt  string `json:"forwarding_profile_excerpt,omitempty"`
	VerifiedSamplesJSON       string `json:"verified_success_samples_json,omitempty"`
	AuthNotes                 string `json:"auth_notes,omitempty"`
	FailureSemanticsJSON      string `json:"failure_semantics_json,omitempty"`
	CalibrationSamplesJSON    string `json:"auth_calibration_json,omitempty"`
	AuthSurfaceJSON           string `json:"auth_surface_json,omitempty"`
	AuthSurfaceMapJSON        string `json:"auth_surface_map_json,omitempty"`
	ForwardChainsJSON         string `json:"forward_chains_json,omitempty"`
}

// probeDestructivePathSegments are generic session-invalidating endpoints — skip live probe.
var probeDestructivePathSegments = []string{
	"logout", "signout", "sign-out", "log-out",
	"clearcache", "clear-cache", "flushcache", "flush-cache",
	"changepassword", "change-password", "resetpassword", "reset-password",
	"deleteaccount", "revokeall", "invalidate",
}

func isProbeDestructivePath(pathPattern string) (bool, string) {
	seg := normalizePathSegment(lastURLPathSegment(pathPattern))
	if seg == "" {
		return false, ""
	}
	for _, d := range probeDestructivePathSegments {
		if seg == normalizePathSegment(d) {
			return true, fmt.Sprintf("skip destructive/session-invalidating endpoint %s (mark route_known_via_static_only if needed)", normURLPath(pathPattern))
		}
	}
	return false, ""
}

func normalizePathSegment(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, ".html")
	return strings.ReplaceAll(s, "-", "")
}

func lastURLPathSegment(path string) string {
	path = strings.TrimSpace(path)
	path = strings.Split(path, "?")[0]
	path = strings.TrimSuffix(path, "/")
	if path == "" || path == "/" {
		return ""
	}
	if i := strings.LastIndex(path, "/"); i >= 0 && i+1 < len(path) {
		return path[i+1:]
	}
	return path
}

func isWeakCodeEvidence(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	lower := strings.ToLower(s)
	if strings.Contains(lower, "static_hint fallback") {
		return true
	}
	if strings.HasPrefix(lower, "auth_evidence:") && len(s) < 160 {
		return true
	}
	return len(s) < 80
}

func enrichProbeCandidateContext(rt *Runtime, ctx *ProbeCandidateContext) {
	if rt == nil || ctx == nil {
		return
	}
	plan, _ := LoadCodeReadingPlanForRuntime(rt)
	enrichProbeContextFromPlan(rt, ctx)

	if ctx.HandlerClass == "" && plan != nil {
		if api := LookupDiscoveredAPI(plan, ctx.Method, ctx.PathPattern); api != nil {
			ctx.HandlerClass = strings.TrimSpace(api.HandlerClass)
		}
	}
	if ctx.HandlerFile == "" && ctx.HandlerClass != "" {
		ctx.HandlerFile = guessFileFromHandlerClass(ctx.HandlerClass)
	}
	if isWeakCodeEvidence(ctx.CodeSnippet) && strings.TrimSpace(ctx.HandlerFile) != "" {
		if snip := loadHandlerSourceSnippet(rt, ctx.HandlerFile, ctx.HandlerSymbol, 12000); snip != "" {
			ctx.CodeSnippet = snip
		}
	}

	rp, _ := loadRoutingProfileFromWorkDir(rt.WorkDir)
	if len(ctx.EffectiveBases) == 0 {
		ctx.EffectiveBases = parseEffectiveBasesFromSession(rt)
	}
	if ctx.URLSpace == "" && rp != nil {
		ctx.URLSpace = PickURLSpaceIDForHandler(rp, ctx.HandlerClass)
	}

	authSel := BuildProbeAuthSelectionHint(rt, ctx.HandlerClass, ctx.URLSpace, "", false)
	if b, err := json.Marshal(authSel); err == nil {
		ctx.AuthSelectionJSON = string(b)
	}

	if plan != nil {
		ctx.AuthNotes = strings.TrimSpace(plan.AuthNotes)
		if len(plan.ForwardChains) > 0 {
			b, _ := json.Marshal(plan.ForwardChains)
			ctx.ForwardChainsJSON = utils.ShrinkString(string(b), 2500)
		}
	}

	if ev, err := loadAuthEvidenceFromWorkDir(rt.WorkDir); err == nil && ev != nil {
		redacted := redactAuthEvidenceForPrompt(ev)
		b, _ := json.Marshal(redacted)
		ctx.AuthEvidenceJSON = utils.ShrinkString(string(b), 3500)
	}
	if rt.Session != nil && strings.TrimSpace(rt.Session.RoutingProfileJSON) != "" {
		ctx.RoutingProfileExcerpt = utils.ShrinkString(rt.Session.RoutingProfileJSON, 2500)
	} else if b, err := osReadFileShrink(store.RoutingProfilePath(rt.WorkDir), 2500); err == nil {
		ctx.RoutingProfileExcerpt = b
	}
	if b, err := osReadFileShrink(store.ForwardingProfilePath(rt.WorkDir), 2000); err == nil {
		ctx.ForwardingProfileExcerpt = b
	}
	if samples := loadGapFillVerifiedSamples(rt); len(samples) > 0 {
		b, _ := json.Marshal(samples)
		ctx.VerifiedSamplesJSON = utils.ShrinkString(string(b), 2000)
	}
	if ctx.AuthSurfaceJSON == "" {
		if b, err := osReadFileShrink(store.AuthSurfacePath(rt.WorkDir), 3000); err == nil {
			ctx.AuthSurfaceJSON = b
		}
	}
	if b, err := osReadFileShrink(store.AuthSurfaceMapPath(rt.WorkDir), 3500); err == nil {
		ctx.AuthSurfaceMapJSON = b
	}
	ctx.FailureSemanticsJSON = utils.ShrinkString(failureSemanticsJSONForProbe(rt), 4000)
	ctx.CalibrationSamplesJSON = utils.ShrinkString(authCalibrationJSONForProbe(rt), 3000)
}

func buildProbePolicyLine(ctx *ProbeCandidateContext) string {
	var parts []string
	parts = append(parts, "use candidate_ctx fields as hints; do not read_file or search for handler/routing/auth")
	parts = append(parts, "infer URL/method/auth from code_snippet + routing_profile_excerpt + auth_selection_json, then verify with probe")
	parts = append(parts, "verdict requires: reachable URL, correct method, valid params, expected business response")
	if ctx.EffectiveBases != nil && len(ctx.EffectiveBases) > 0 {
		parts = append(parts, fmt.Sprintf("effective_bases=%v (pick one as base URL)", ctx.EffectiveBases))
	}
	if ctx.URLSpace != "" {
		parts = append(parts, fmt.Sprintf("url_space=%s", ctx.URLSpace))
	}
	if ctx.RoutingProfileExcerpt != "" {
		parts = append(parts, "routing_profile_excerpt contains mount_prefix hints for url_spaces")
	}
	if ctx.CodeSnippet != "" {
		parts = append(parts, "code_snippet contains @RequestMapping/@GetMapping/@PostMapping etc. — infer path and method from annotations")
	}
	if ctx.AuthSelectionJSON != "" {
		parts = append(parts, "auth_selection_json contains available_credentials — select matching auth_credential_id based on handler package/path; engine suggestion may be overridden")
	}
	if ctx.AuthEvidenceJSON != "" {
		parts = append(parts, "auth_evidence shows known login endpoints and realms")
	}
	if ctx.VerifiedSamplesJSON != "" {
		parts = append(parts, "verified_success_samples shows similar verified routes — use as URL/method pattern reference")
	}
	if ctx.FailureSemanticsJSON != "" {
		parts = append(parts, "failure_semantics describes expected error responses for this project")
	}
	return strings.Join(parts, "; ")
}


func resolveSuggestedAuthCredentialID(rt *Runtime) uint {
	if rt == nil {
		return 0
	}
	id, _ := ResolveCredentialIDForProbe(rt, "", "", "", "")
	return id
}

func loadHandlerSourceSnippet(rt *Runtime, handlerFile, handlerSymbol string, maxLen int) string {
	path := resolveHandlerFilePath(rt, handlerFile)
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	content := string(b)
	if sym := strings.TrimSpace(handlerSymbol); sym != "" {
		if snip := extractHandlerMethodSnippet(content, sym); snip != "" {
			return utils.ShrinkString(snip, maxLen)
		}
	}
	return utils.ShrinkString(content, maxLen)
}

func resolveHandlerFilePath(rt *Runtime, handlerFile string) string {
	ref := strings.TrimSpace(handlerFile)
	if ref == "" || rt == nil || rt.Session == nil {
		return ""
	}
	ref = filepath.ToSlash(ref)
	root := strings.TrimSpace(rt.Session.CodeRootPath)
	candidates := []string{ref}
	if root != "" {
		candidates = append(candidates, filepath.Join(root, ref))
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

func extractHandlerMethodSnippet(content, symbol string) string {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return ""
	}
	re := regexp.MustCompile(`(?ms)(?:@[\w.]+(?:\([^)]*\))?\s*)*(?:public|protected|private)?\s*[\w.<>,\s\[\]]+\s+` + regexp.QuoteMeta(symbol) + `\s*\([^)]*\)\s*\{`)
	loc := re.FindStringIndex(content)
	if loc == nil {
		return ""
	}
	start := loc[0]
	// include preceding annotations / class-level @RequestMapping context
	if start > 800 {
		start -= 800
	} else {
		start = 0
	}
	end := loc[1] + 1200
	if end > len(content) {
		end = len(content)
	}
	return content[start:end]
}

// buildPhase1VerifyEmbeddedContext injects auth/routing/credential summaries into the verify loop prompt.
func buildPhase1VerifyEmbeddedContext(rt *Runtime) string {
	if rt == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## 预注入探测上下文（引擎自动装配，禁止为探测再 read_file/grep 找路由或鉴权）\n\n")
	sb.WriteString("- 调用 **discovery_probe_api_candidate** 时只需传 `method`+`path_pattern` 或 `http_endpoint_id`；handler 源码、mount 前缀、凭证 id、URL 提示由引擎注入子循环 candidate_ctx。\n")
	sb.WriteString("- 破坏性端点（logout/clearCache/changePassword 等）会自动 skip，勿手动探测。\n\n")

	if ev, err := loadAuthEvidenceFromWorkDir(rt.WorkDir); err == nil && ev != nil {
		redacted := redactAuthEvidenceForPrompt(ev)
		b, _ := json.MarshalIndent(redacted, "", "  ")
		sb.WriteString("### auth_evidence（已脱敏）\n```json\n")
		sb.WriteString(utils.ShrinkString(string(b), 3500))
		sb.WriteString("\n```\n\n")
	}
	if rt.Session != nil && strings.TrimSpace(rt.Session.RoutingProfileJSON) != "" {
		sb.WriteString("### routing_profile_excerpt\n```json\n")
		sb.WriteString(utils.ShrinkString(rt.Session.RoutingProfileJSON, 2500))
		sb.WriteString("\n```\n\n")
	}
	if summaries, err := listVerifiedCredentialSummaries(rt); err == nil && len(summaries) > 0 {
		b, _ := json.MarshalIndent(summaries, "", "  ")
		sb.WriteString("### auth_credentials（按 url_space / auth_realm 分套存储）\n```json\n")
		sb.WriteString(utils.ShrinkString(string(b), 4000))
		sb.WriteString("\n```\n\n")
	}
	ev, _ := loadAuthEvidenceFromWorkDir(rt.WorkDir)
	if DetectMultiAuth(rt, ev) {
		sb.WriteString("### multi_auth\n项目存在多套鉴权；API 探测须根据 handler 的 auth_realm/url_space 从上方列表选择 **auth_credential_id**，禁止混用后台 Cookie 探测前台接口或反之。\n\n")
	} else if credID := resolveSuggestedAuthCredentialID(rt); credID > 0 {
		sb.WriteString(fmt.Sprintf("### single_auth\n- default auth_credential_id=%d\n\n", credID))
	}
	if b, err := osReadFileShrink(store.ForwardingProfilePath(rt.WorkDir), 2000); err == nil {
		sb.WriteString("### forwarding_profile_excerpt\n```json\n")
		sb.WriteString(b)
		sb.WriteString("\n```\n\n")
	}
	sb.WriteString(phase1ArtifactHints(rt))
	return sb.String()
}

func phase1ArtifactHints(rt *Runtime) string {
	if rt == nil {
		return ""
	}
	paths := []string{
		store.Phase1PrepBundlePath(rt.WorkDir),
		store.CodeReadingPlanPath(rt.WorkDir),
		store.StaticRouteHintsPath(rt.WorkDir),
		store.RouteCandidatesPath(rt.WorkDir),
		store.AuthSurfacePath(rt.WorkDir),
		store.ForwardingProfilePath(rt.WorkDir),
	}
	var lines []string
	lines = append(lines, "## Phase1 工件路径")
	for _, p := range paths {
		_, err := os.Stat(p)
		st := "missing"
		if err == nil {
			st = "ready"
		}
		lines = append(lines, fmt.Sprintf("- %s (%s)", p, st))
	}
	return strings.Join(lines, "\n")
}
