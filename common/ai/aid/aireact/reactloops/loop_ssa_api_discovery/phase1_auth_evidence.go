package loop_ssa_api_discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// AuthEvidence from code reading / project profile for evidence-driven login.
type AuthEvidence struct {
	LoginPaths    []string `json:"login_paths"`
	UsernameField string   `json:"username_field"`
	PasswordField string   `json:"password_field"`
	HashAlgorithm string   `json:"hash_algorithm,omitempty"`
	HashEvidence  string   `json:"hash_evidence,omitempty"`
	ContentType   string   `json:"content_type,omitempty"`
}

type authStateRecord struct {
	State     string `json:"state"`
	UpdatedAt string `json:"updated_at"`
	Detail    string `json:"detail,omitempty"`
}

const (
	authStateSuccess      = "success"
	authStateFailed       = "failed"
	authStateNoAuthNeeded = "no_auth_needed"
)

// RunPhase1EvidenceAuth attempts programmatic login using evidence from catalog/plan/profile.
func RunPhase1EvidenceAuth(ctx context.Context, invoker aicommon.AIInvokeRuntime, rt *Runtime) (string, error) {
	if rt == nil || rt.Session == nil || rt.Repo == nil {
		return "", nil
	}
	if !rt.Session.TargetReachable {
		return writeAuthState(rt, authStateNoAuthNeeded, "target unreachable")
	}

	evidence := collectAuthEvidence(rt)
	profile, _ := loadProjectProfile(rt.WorkDir)
	if len(evidence.LoginPaths) == 0 && !hasSecurityConfigEntry(profile) {
		return writeAuthState(rt, authStateNoAuthNeeded, "no security config or login paths in evidence")
	}

	username, password := credentialsForRuntime(rt)
	pwd := password
	if algo := strings.TrimSpace(evidence.HashAlgorithm); algo != "" {
		if res, err := transformCredentialGoParams(algo, password, "", "", "", "hex", false); err == nil && strings.TrimSpace(res.Output) != "" {
			pwd = res.Output
		}
	}

	if invoker != nil {
		for _, loginPath := range evidence.LoginPaths {
			msg, err := tryEvidenceLogin(ctx, invoker, rt, loginPath, evidence, username, pwd)
			if err == nil && msg != "" {
				if invoker != nil {
					invoker.AddToTimeline("[ssa_phase1_auth]", "auth_state=success "+msg)
				}
				return writeAuthState(rt, authStateSuccess, msg)
			}
		}
	}

	if invoker != nil {
		invoker.AddToTimeline("[ssa_phase1_auth]", "auth_state=failed")
	}
	return writeAuthState(rt, authStateFailed, "evidence login attempts failed")
}

func writeAuthState(rt *Runtime, state, detail string) (string, error) {
	rec := authStateRecord{State: state, UpdatedAt: time.Now().UTC().Format(time.RFC3339), Detail: detail}
	b, _ := json.MarshalIndent(rec, "", "  ")
	_ = writeJSONFile(store.AuthStatePath(rt.WorkDir), b)
	return state, nil
}

func collectAuthEvidence(rt *Runtime) AuthEvidence {
	ev := AuthEvidence{
		UsernameField: "username",
		PasswordField: "password",
		ContentType:   "application/json",
	}
	if plan, err := LoadCodeReadingPlan(rt.WorkDir); err == nil {
		if notes := strings.TrimSpace(plan.AuthNotes); notes != "" {
			ev = parseAuthNotesIntoEvidence(notes, ev)
		}
	}
	if extra := scanAuthNotesFromLoginFiles(rt); extra != "" {
		ev = parseAuthNotesIntoEvidence(extra, ev)
	}
	if catalog, err := loadApiCatalog(rt.WorkDir); err == nil {
		for _, e := range catalog.Entries {
			lower := strings.ToLower(e.PathPattern + " " + e.HandlerSymbol)
			if strings.Contains(lower, "login") || strings.Contains(lower, "signin") || strings.Contains(lower, "auth") {
				ev.LoginPaths = append(ev.LoginPaths, e.PathPattern)
			}
		}
	}
	ev.LoginPaths = dedupeStrings(ev.LoginPaths)
	return ev
}

func parseAuthNotesIntoEvidence(notes string, ev AuthEvidence) AuthEvidence {
	lower := strings.ToLower(notes)
	for _, algo := range []string{"sha512", "sha256", "sha1", "md5", "base64"} {
		if strings.Contains(lower, algo) {
			ev.HashAlgorithm = algo
			ev.HashEvidence = notes
			break
		}
	}
	for _, line := range strings.Split(notes, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "login_path:") {
			p := strings.TrimSpace(line[len("login_path:"):])
			if p != "" {
				ev.LoginPaths = append(ev.LoginPaths, p)
			}
		}
	}
	return ev
}

func tryEvidenceLogin(ctx context.Context, invoker aicommon.AIInvokeRuntime, rt *Runtime, path string, ev AuthEvidence, username, password string) (string, error) {
	if invoker == nil {
		return "", nil
	}
	base := EffectiveTargetBaseURL(rt.Session)
	if base == "" {
		return "", nil
	}
	userField := ev.UsernameField
	if userField == "" {
		userField = "username"
	}
	passField := ev.PasswordField
	if passField == "" {
		passField = "password"
	}
	jsonBody, _ := json.Marshal(map[string]string{userField: username, passField: password})
	ct := ev.ContentType
	if ct == "" {
		ct = "application/json"
	}
	loginURL := strings.TrimRight(base, "/") + normURLPath(path)
	params := aitool.InvokeParams{
		"url": loginURL, "method": "POST", "content-type": ct, "body": string(jsonBody),
	}
	result, _, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, "do_http_request", params)
	if err != nil {
		return "", err
	}
	content := toolResultTextContent(result)
	if !loginProbeSuccessful(content) {
		return "", utils.Error("login rejected")
	}
	return saveLoginCredential(rt, loginURL, username, content)
}

func saveLoginCredential(rt *Runtime, loginURL, username, content string) (string, error) {
	cred := &store.AuthCredential{
		SessionID: rt.Session.ID,
		AuthType:  "cookie_session",
		Username:  username,
		Verified:  true,
		VerifyURL: loginURL,
		Notes:     "phase1 evidence-driven login",
	}
	if headersJSON := buildAuthHeadersJSONFromLoginResponse(content); headersJSON != "" {
		cred.HeadersJSON = headersJSON
		SyncCredentialHeaderFields(cred)
	} else if token := extractJSONTokenFromResponse(content); token != "" {
		cred.HeadersJSON = fmt.Sprintf(`{"Authorization":"Bearer %s"}`, token)
		SyncCredentialHeaderFields(cred)
	}
	if cred.HeadersJSON == "" {
		return "", utils.Error("no auth headers in response")
	}
	if err := rt.Repo.CreateAuthCredential(cred); err != nil {
		return "", err
	}
	return fmt.Sprintf("credential_id=%d url=%s", cred.ID, loginURL), nil
}

func defaultAuthCredentials() (string, string) {
	return "admin", "Admin@2024!"
}

func scanAuthNotesFromLoginFiles(rt *Runtime) string {
	if rt == nil || rt.Session == nil || !rt.Session.CodePathOK {
		return ""
	}
	root := rt.Session.CodeRootPath
	var notes []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		base := strings.ToLower(info.Name())
		if !strings.Contains(base, "login") && !strings.Contains(base, "auth") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		head, _ := readFileHead(path, 4096)
		if len(head) == 0 {
			return nil
		}
		snippet := string(head)
		lower := strings.ToLower(snippet)
		for _, algo := range []string{"sha512", "sha256", "md5", "cryptojs", "securelogin"} {
			if strings.Contains(lower, algo) {
				notes = append(notes, fmt.Sprintf("hash_hint:%s in %s", algo, filepath.ToSlash(rel)))
			}
		}
		return nil
	})
	return strings.Join(notes, "; ")
}
