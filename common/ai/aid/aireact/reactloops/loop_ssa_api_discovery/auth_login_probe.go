package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

const loopKeyLastLoginProbeJSON = "ssa_last_login_probe_json"

// LoginProbeOutcome is stored on the ReAct loop after a login POST is analyzed.
type LoginProbeOutcome struct {
	Success     bool   `json:"success"`
	HeadersJSON string `json:"headers_json,omitempty"`
	LoginURL    string `json:"login_url,omitempty"`
	LoginPath   string `json:"login_path,omitempty"`
	Username    string `json:"username,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

func redirectFollowLoginSuccessful(content string) bool {
	if !loginResponseHasRedirect(content) {
		return false
	}
	if !strings.Contains(strings.ToLower(content), "redirect #") {
		return false
	}
	return len(validSessionCookiePairsFromHTTPOutput(content)) > 0
}

func validSessionCookiePairsFromHTTPOutput(content string) []string {
	setPairs := filterValidSessionCookiePairs(extractAllSetCookiePairsFromResponse(content))
	reqPairs := extractCookieHeaderPairsFromHTTPOutput(content)
	merged := mergeCookiePairs(setPairs, reqPairs)
	return merged
}

func filterValidSessionCookiePairs(pairs []string) []string {
	var out []string
	for _, pair := range pairs {
		name, value, ok := splitCookiePair(pair)
		if !ok || isClearedSessionCookie(name, value) {
			continue
		}
		out = append(out, name+"="+value)
	}
	return out
}

func mergeCookiePairs(primary, secondary []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, list := range [][]string{primary, secondary} {
		for _, pair := range list {
			name, value, ok := splitCookiePair(pair)
			if !ok || isClearedSessionCookie(name, value) {
				continue
			}
			norm := name + "=" + value
			if _, dup := seen[norm]; dup {
				continue
			}
			seen[norm] = struct{}{}
			out = append(out, norm)
		}
	}
	return out
}

func splitCookiePair(pair string) (name, value string, ok bool) {
	pair = strings.TrimSpace(pair)
	idx := strings.Index(pair, "=")
	if idx <= 0 {
		return "", "", false
	}
	name = strings.TrimSpace(pair[:idx])
	value = strings.TrimSpace(pair[idx+1:])
	return name, value, name != ""
}

func isClearedSessionCookie(name, value string) bool {
	if value == "" {
		return true
	}
	lowerName := strings.ToLower(name)
	if strings.HasPrefix(lowerName, "remember-me") && value == "deleteMe" {
		return true
	}
	return false
}

func extractCookieHeaderPairsFromHTTPOutput(content string) []string {
	if pairs := extractCookieHeaderPairsFromLines(content); len(pairs) > 0 {
		return pairs
	}
	if unescaped := unescapeLiteralNewlines(content); unescaped != content {
		if pairs := extractCookieHeaderPairsFromLines(unescaped); len(pairs) > 0 {
			return pairs
		}
	}
	return nil
}

func unescapeLiteralNewlines(content string) string {
	return strings.ReplaceAll(content, `\n`, "\n")
}

func extractCookieHeaderPairsFromLines(content string) []string {
	var pairs []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(trimmed), "cookie:") {
			continue
		}
		val := strings.TrimSpace(trimmed[len("cookie:"):])
		for _, part := range strings.Split(val, ";") {
			part = strings.TrimSpace(part)
			name, value, ok := splitCookiePair(part)
			if !ok || isClearedSessionCookie(name, value) {
				continue
			}
			pairs = append(pairs, name+"="+value)
		}
	}
	return pairs
}

func analyzeHTTPOutputForLogin(method, requestURL, postParams, body, content string) *LoginProbeOutcome {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method != "POST" && method != "PUT" {
		return nil
	}
	if !looksLikeLoginRequestURL(requestURL) {
		return nil
	}
	if !loginProbeSuccessful(content) {
		return nil
	}
	headersJSON := buildAuthHeadersJSONFromLoginResponse(content)
	if headersJSON == "" {
		return nil
	}
	loginPath := loginPathFromURL(requestURL)
	return &LoginProbeOutcome{
		Success:     true,
		HeadersJSON: headersJSON,
		LoginURL:    strings.TrimSpace(requestURL),
		LoginPath:   loginPath,
		Username:    extractUsernameFromLoginRequest(postParams, body),
		Notes:       "programmatic: login POST returned redirect + session cookie(s); redirect target 404 does not invalidate auth",
	}
}

func looksLikeLoginRequestURL(rawURL string) bool {
	rawURL = strings.ToLower(strings.TrimSpace(rawURL))
	if rawURL == "" {
		return false
	}
	for _, seg := range []string{"/login", "/signin", "/sign-in", "/auth/login", "/authenticate"} {
		if strings.Contains(rawURL, seg) {
			return true
		}
	}
	return false
}

func loginPathFromURL(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Path == "" {
		return normURLPath(rawURL)
	}
	return normURLPath(u.Path)
}

func extractUsernameFromLoginRequest(postParams, body string) string {
	for _, src := range []string{postParams, body} {
		if src == "" {
			continue
		}
		vals, err := url.ParseQuery(strings.ReplaceAll(src, ";", "&"))
		if err != nil {
			continue
		}
		for _, key := range []string{"username", "user", "email", "login", "account"} {
			if v := strings.TrimSpace(vals.Get(key)); v != "" {
				return v
			}
		}
	}
	return ""
}

func storeLoginProbeOutcome(loop *reactloops.ReActLoop, outcome *LoginProbeOutcome) {
	if loop == nil || outcome == nil || !outcome.Success {
		return
	}
	b, err := json.Marshal(outcome)
	if err != nil {
		return
	}
	loop.Set(loopKeyLastLoginProbeJSON, string(b))
}

func loadLoginProbeOutcome(loop *reactloops.ReActLoop) *LoginProbeOutcome {
	if loop == nil {
		return nil
	}
	raw := strings.TrimSpace(loop.Get(loopKeyLastLoginProbeJSON))
	if raw == "" {
		return nil
	}
	var out LoginProbeOutcome
	if json.Unmarshal([]byte(raw), &out) != nil || !out.Success {
		return nil
	}
	return &out
}

func applyLoginProbeOutcomeToCredential(row *store.AuthCredential, outcome *LoginProbeOutcome) bool {
	if row == nil || outcome == nil || !outcome.Success {
		return false
	}
	changed := false
	if !row.Verified {
		row.Verified = true
		changed = true
	}
	if strings.TrimSpace(row.HeadersJSON) == "" || row.HeadersJSON == `{"Cookie":""}` {
		row.HeadersJSON = outcome.HeadersJSON
		changed = true
	}
	if strings.TrimSpace(row.LoginPath) == "" && outcome.LoginPath != "" {
		row.LoginPath = outcome.LoginPath
		changed = true
	}
	if strings.TrimSpace(row.Username) == "" && outcome.Username != "" {
		row.Username = outcome.Username
		changed = true
	}
	if strings.TrimSpace(row.VerifyURL) == "" && outcome.LoginPath != "" {
		row.VerifyURL = suggestVerifyURLFromLoginPath(outcome.LoginPath, row.MountPrefix)
		changed = true
	}
	if changed {
		SyncCredentialHeaderFields(row)
	}
	return changed
}

func suggestVerifyURLFromLoginPath(loginPath, mountPrefix string) string {
	loginPath = normURLPath(loginPath)
	mountPrefix = normURLPath(mountPrefix)
	for _, p := range SuggestPostLoginVerifyPaths(loginPath, "index.html", mountPrefix) {
		if p != "" {
			return p
		}
	}
	return loginPath
}

func formatLoginProbeProgrammaticHint(outcome *LoginProbeOutcome) string {
	if outcome == nil || !outcome.Success {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## programmatic_login_probe (302 + session cookie)\n")
	b.WriteString("Login POST is treated as **SUCCESS** even if redirect follow returned 404.\n")
	b.WriteString("- Rule: redirect (302/301/303/307) + session Cookie issuance => authenticated\n")
	b.WriteString("- Do **not** mark failed because the final response is 404.\n")
	if outcome.HeadersJSON != "" {
		b.WriteString("- Suggested headers_json:\n```json\n")
		b.WriteString(outcome.HeadersJSON)
		b.WriteString("\n```\n")
	}
	if outcome.Username != "" {
		b.WriteString(fmt.Sprintf("- Detected username=%q\n", outcome.Username))
	}
	if outcome.LoginPath != "" {
		b.WriteString(fmt.Sprintf("- login_path=%q\n", outcome.LoginPath))
		if hint := postLoginVerifyURLHint(outcome.LoginPath, suggestVerifyURLFromLoginPath(outcome.LoginPath, ""), ""); hint != "" {
			b.WriteString("\n")
			b.WriteString(hint)
		}
	}
	b.WriteString("- Next: call discovery_select_auth_credential with the auto-saved credential id, then use auth_credential_id for all probes.\n")
	return b.String()
}

func tryAutoSaveLoginCredentialFromProbe(loop *reactloops.ReActLoop, rt *Runtime, authRealm string, action *aicommon.Action, outcome *LoginProbeOutcome) string {
	_, msg := saveLoginCredentialFromProbe(rt, loop, authRealm, action, outcome)
	return msg
}

func inferMountPrefixForAuthRealm(rt *Runtime, authRealm string) string {
	if rt != nil {
		if surface, err := loadAuthSurfaceMap(rt.WorkDir); err == nil {
			for _, s := range surface.Surfaces {
				if NormalizeAuthRealm(s.AuthRealm) == NormalizeAuthRealm(authRealm) {
					if mp := normURLPath(s.MountPrefix); mp != "" {
						return mp
					}
				}
			}
		}
	}
	return "/"
}

func enrichHTTPFeedbackWithLoginProbe(loop *reactloops.ReActLoop, rt *Runtime, authRealm string, action *aicommon.Action, content string) string {
	if action == nil {
		return ""
	}
	method := action.GetString("method")
	if method == "" {
		method = "GET"
	}
	outcome := analyzeHTTPOutputForLogin(method, action.GetString("url"), action.GetString("post-params"), action.GetString("body"), content)
	if outcome == nil {
		return ""
	}
	storeLoginProbeOutcome(loop, outcome)
	msg := formatLoginProbeProgrammaticHint(outcome)
	msg += tryAutoSaveLoginCredentialFromProbe(loop, rt, authRealm, action, outcome)
	return msg
}

func applyStoredLoginProbeToUpsert(loop *reactloops.ReActLoop, row *store.AuthCredential) string {
	outcome := loadLoginProbeOutcome(loop)
	if outcome == nil {
		return ""
	}
	if !applyLoginProbeOutcomeToCredential(row, outcome) {
		return ""
	}
	return "\nprogrammatic_login_probe: upgraded credential (verified=true, headers_json filled from 302+Set-Cookie rule)"
}
