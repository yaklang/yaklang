package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// AuthLoginEndpoint documents one login/auth HTTP entry discovered and optionally probed during code reading.
type AuthLoginEndpoint struct {
	Method                    string            `json:"method"`
	Path                      string            `json:"path"`
	FullURL                   string            `json:"full_url,omitempty"`
	ContentType               string            `json:"content_type"`
	FormFields                map[string]string `json:"form_fields,omitempty"`
	PasswordTransform         string            `json:"password_transform,omitempty"`
	PasswordTransformEvidence string            `json:"password_transform_evidence,omitempty"`
	SuccessIndicators         []string          `json:"success_indicators,omitempty"`
	ProbeAttempted            bool              `json:"probe_attempted,omitempty"`
	ProbeSucceeded              bool              `json:"probe_succeeded,omitempty"`
	ProbeResponseExcerpt        string            `json:"probe_response_excerpt,omitempty"`
	CodeEvidence                string            `json:"code_evidence,omitempty"`
	URLSpace                    string            `json:"url_space,omitempty"`
	AuthRealm                   string            `json:"auth_realm,omitempty"`
	MountPrefix                 string            `json:"mount_prefix,omitempty"`
	Label                       string            `json:"label,omitempty"`
	CredentialID                uint              `json:"credential_id,omitempty"`
}

// AuthCredentialBinding links a stored credential to a URL space / auth realm.
type AuthCredentialBinding struct {
	URLSpace     string `json:"url_space,omitempty"`
	AuthRealm    string `json:"auth_realm,omitempty"`
	MountPrefix  string `json:"mount_prefix,omitempty"`
	LoginPath    string `json:"login_path,omitempty"`
	CredentialID uint   `json:"credential_id"`
	Verified     bool   `json:"verified,omitempty"`
	Label        string `json:"label,omitempty"`
}

// AuthEvidenceRecord is structured auth output from an auth_entry code-reading stage.
type AuthEvidenceRecord struct {
	LoginEndpoints       []AuthLoginEndpoint     `json:"login_endpoints,omitempty"`
	SessionMechanism     string                  `json:"session_mechanism,omitempty"`
	Verified             bool                    `json:"verified"`
	VerificationDetail   string                  `json:"verification_detail,omitempty"`
	CredentialID         uint                    `json:"credential_id,omitempty"`
	MultiAuth            bool                    `json:"multi_auth,omitempty"`
	CredentialBindings   []AuthCredentialBinding `json:"credential_bindings,omitempty"`
	CodeEvidence         []string                `json:"code_evidence,omitempty"`
}

func batchHasAuthEntry(batch []WorklistSeedItem) bool {
	for _, item := range batch {
		if strings.TrimSpace(item.Category) == worklistCategoryAuthEntry {
			return true
		}
		if isAuthEntryPath(item.RelPath) {
			return true
		}
	}
	return false
}

func worklistHasAuthEntry(seed []WorklistSeedItem) bool {
	for _, item := range seed {
		if strings.TrimSpace(item.Category) == worklistCategoryAuthEntry {
			return true
		}
		if item.Priority == 2 && isAuthEntryPath(item.RelPath) {
			return true
		}
	}
	return false
}

func isLoginTemplateRelPath(rel string) bool {
	lower := strings.ToLower(strings.ReplaceAll(rel, `\`, `/`))
	if !strings.HasSuffix(lower, ".html") && !strings.HasSuffix(lower, ".ftl") && !strings.HasSuffix(lower, ".jsp") {
		return false
	}
	for _, tok := range []string{"login", "signin", "sign-in", "authenticate", "auth"} {
		if strings.Contains(lower, tok) {
			return true
		}
	}
	return false
}

func isAuthAnalysisPath(rel string) bool {
	if isBackendCodeRelPath(rel) {
		return true
	}
	return isLoginTemplateRelPath(rel)
}

func worklistBatchAllowedPathsForBatch(rt *Runtime, batch []WorklistSeedItem) map[string]struct{} {
	out := worklistBatchAllowedPaths(rt, batch)
	if !batchHasAuthEntry(batch) {
		return out
	}
	for _, item := range batch {
		rel := normalizePlanFileRef(rt, item.RelPath)
		if rel != "" {
			out[rel] = struct{}{}
		}
	}
	return out
}

func validateAuthStageOutput(out *CodeReadingStageOutput, batch []WorklistSeedItem, rt *Runtime) error {
	if out == nil || out.AuthEvidence == nil {
		return utils.Error("auth_entry batch requires auth_evidence in finalize_code_reading_stage")
	}
	ev := out.AuthEvidence
	if len(ev.LoginEndpoints) == 0 {
		return utils.Error("auth_evidence.login_endpoints required")
	}
	for i, ep := range ev.LoginEndpoints {
		if strings.TrimSpace(ep.Method) == "" {
			return utils.Errorf("auth_evidence.login_endpoints[%d].method required", i)
		}
		if strings.TrimSpace(ep.Path) == "" && strings.TrimSpace(ep.FullURL) == "" {
			return utils.Errorf("auth_evidence.login_endpoints[%d].path or full_url required", i)
		}
		if strings.TrimSpace(ep.ContentType) == "" {
			return utils.Errorf("auth_evidence.login_endpoints[%d].content_type required", i)
		}
	}
	reachable := rt != nil && rt.Session != nil && rt.Session.TargetReachable
	if reachable {
		multi := DetectMultiAuth(rt, ev)
		if multi {
			ev.MultiAuth = true
			attemptedRealms := map[string]struct{}{}
			for _, ep := range ev.LoginEndpoints {
				if !ep.ProbeAttempted {
					continue
				}
				r := NormalizeAuthRealm(ep.AuthRealm)
				if r == "" {
					r = InferAuthRealmFromLoginPath(ep.Path, ep.FullURL)
				}
				if r != "" {
					attemptedRealms[r] = struct{}{}
				}
			}
			required := RequiredAuthRealms(rt, ev)
			for _, r := range required {
				if _, ok := attemptedRealms[r]; !ok {
					return utils.Errorf("multi_auth: must probe login for auth_realm=%s (separate do_http_request + discovery_upsert_auth_credential with url_space/auth_realm)", r)
				}
			}
		} else {
			attempted := false
			for _, ep := range ev.LoginEndpoints {
				if ep.ProbeAttempted {
					attempted = true
					break
				}
			}
			if !attempted {
				return utils.Error("target reachable: auth stage must probe at least one login endpoint via do_http_request")
			}
		}
	}
	_ = batch
	return nil
}

func mergeAuthEvidenceFromStages(stages []CodeReadingStageOutput) *AuthEvidenceRecord {
	var best *AuthEvidenceRecord
	for i := range stages {
		ev := stages[i].AuthEvidence
		if ev == nil {
			continue
		}
		if best == nil || ev.Verified && !best.Verified {
			copy := *ev
			best = &copy
		}
	}
	return best
}

// SyncAuthFromCodeReadingStages writes auth_state.json from staged auth_evidence (no programmatic login).
func SyncAuthFromCodeReadingStages(rt *Runtime) (string, error) {
	if rt == nil || rt.Session == nil {
		return "", nil
	}
	stages, err := loadAllCodeReadingStages(rt.WorkDir)
	if err != nil {
		return "", err
	}
	ev := mergeAuthEvidenceFromStages(stages)
	if ev == nil {
		if !rt.Session.TargetReachable {
			return writeAuthState(rt, authStateNoAuthNeeded, "target unreachable; auth analysis deferred")
		}
		if hasSecurityConfigInStages(stages) {
			return writeAuthState(rt, authStateFailed, "auth_entry stage missing auth_evidence")
		}
		return writeAuthState(rt, authStateNoAuthNeeded, "no auth evidence from code reading stages")
	}

	b, _ := json.MarshalIndent(ev, "", "  ")
	_ = writeJSONFile(store.AuthEvidencePath(rt.WorkDir), b)
	if rt.Repo != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactAuthEvidence, string(b))
	}

	if ev.Verified {
		creds, _ := rt.Repo.ListAuthCredentials(rt.Session.ID)
		if len(creds) == 0 {
			log.Warnf("ssa_api_discovery: auth verified in stage but no auth_credentials row; agent should call discovery_upsert_auth_credential")
		}
		return writeAuthState(rt, authStateSuccess, ev.VerificationDetail)
	}
	if !rt.Session.TargetReachable {
		return writeAuthState(rt, authStateNoAuthNeeded, "auth documented; target unreachable for live verify")
	}
	detail := strings.TrimSpace(ev.VerificationDetail)
	if detail == "" {
		detail = "auth analysis completed but login probe did not succeed"
	}
	return writeAuthState(rt, authStateFailed, detail)
}

func hasSecurityConfigInStages(stages []CodeReadingStageOutput) bool {
	for _, st := range stages {
		for _, rf := range st.RoutingFacts {
			if strings.Contains(strings.ToLower(rf.Kind), "security") {
				return true
			}
		}
	}
	return false
}

func authVerifiedFromRuntime(rt *Runtime) bool {
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
	return rec.State == authStateSuccess
}

func authStateIsNoAuthNeeded(rt *Runtime) bool {
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
	return rec.State == authStateNoAuthNeeded
}

func authStageCompleted(stages []CodeReadingStageOutput) bool {
	for _, st := range stages {
		if st.AuthEvidence != nil && len(st.AuthEvidence.LoginEndpoints) > 0 {
			return true
		}
	}
	return false
}

func loadAuthEvidenceFromWorkDir(workDir string) (*AuthEvidenceRecord, error) {
	b, err := os.ReadFile(store.AuthEvidencePath(workDir))
	if err != nil {
		return nil, err
	}
	var ev AuthEvidenceRecord
	if err := json.Unmarshal(b, &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

func authEvidenceSummary(ev *AuthEvidenceRecord) string {
	if ev == nil {
		return ""
	}
	var parts []string
	for _, ep := range ev.LoginEndpoints {
		parts = append(parts, ep.Method+" "+ep.Path+" ct="+ep.ContentType)
	}
	if ev.Verified {
		parts = append(parts, "verified=true")
	}
	return strings.Join(parts, "; ")
}

func touchAuthEvidenceGeneratedAt(workDir string) {
	path := store.AuthEvidencePath(workDir)
	var ev AuthEvidenceRecord
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &ev)
	}
	if ev.VerificationDetail == "" {
		ev.VerificationDetail = "generated_at=" + time.Now().UTC().Format(time.RFC3339)
	}
	b, _ := json.MarshalIndent(ev, "", "  ")
	_ = writeJSONFile(path, b)
}
