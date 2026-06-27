package loop_ssa_api_discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	defaultCsrfParamName      = "_csrf"
	publicCMSAdminCookieName  = "PUBLICCMS_ADMIN"
)

// AuthAwareHTTPActionConfig optional behavior for buildAuthAwareHTTPAction.
type AuthAwareHTTPActionConfig struct {
	CalibrationRealm string
	PinnedTarget     *HttpProbeTarget
}

var (
	reCsrfInputNameFirst  = regexp.MustCompile(`(?i)<input[^>]+name\s*=\s*["']([^"']*csrf[^"']*)["'][^>]+value\s*=\s*["']([^"']+)["']`)
	reCsrfInputValueFirst = regexp.MustCompile(`(?i)<input[^>]+value\s*=\s*["']([^"']+)["'][^>]+name\s*=\s*["']([^"']*csrf[^"']*)["']`)
	reCsrfQueryInHTML     = regexp.MustCompile(`(?i)[?&]_csrf=([A-Za-z0-9._-]+)`)
)

// AuthCsrfEntry stores a captured CSRF token for one auth credential.
type AuthCsrfEntry struct {
	CredentialID uint   `json:"credential_id"`
	AuthRealm    string `json:"auth_realm,omitempty"`
	ParamName    string `json:"param_name"`
	Token        string `json:"token"`
	CapturedAt   string `json:"captured_at"`
	SourceURL    string `json:"source_url,omitempty"`
}

type authCsrfCacheV1 struct {
	SchemaVersion int             `json:"schema_version"`
	Entries       []AuthCsrfEntry `json:"entries"`
}

func loadAuthCsrfCache(workDir string) (*authCsrfCacheV1, error) {
	path := store.AuthCsrfTokensPath(workDir)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &authCsrfCacheV1{SchemaVersion: 1}, nil
		}
		return nil, err
	}
	var c authCsrfCacheV1
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.SchemaVersion == 0 {
		c.SchemaVersion = 1
	}
	return &c, nil
}

func saveAuthCsrfCache(workDir string, c *authCsrfCacheV1) error {
	if c == nil {
		return utils.Error("nil csrf cache")
	}
	if c.SchemaVersion == 0 {
		c.SchemaVersion = 1
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeJSONFile(store.AuthCsrfTokensPath(workDir), b)
}

func getCsrfTokenForCredential(workDir string, credID uint) (paramName, token string, ok bool) {
	if credID == 0 || workDir == "" {
		return "", "", false
	}
	c, err := loadAuthCsrfCache(workDir)
	if err != nil || c == nil {
		return "", "", false
	}
	for i := len(c.Entries) - 1; i >= 0; i-- {
		e := c.Entries[i]
		if e.CredentialID == credID && strings.TrimSpace(e.Token) != "" {
			pn := strings.TrimSpace(e.ParamName)
			if pn == "" {
				pn = defaultCsrfParamName
			}
			return pn, strings.TrimSpace(e.Token), true
		}
	}
	return "", "", false
}

func setCsrfTokenForCredential(rt *Runtime, cred *store.AuthCredential, paramName, token, sourceURL string) error {
	if rt == nil || cred == nil || strings.TrimSpace(token) == "" {
		return nil
	}
	if strings.TrimSpace(paramName) == "" {
		paramName = defaultCsrfParamName
	}
	c, err := loadAuthCsrfCache(rt.WorkDir)
	if err != nil {
		return err
	}
	entry := AuthCsrfEntry{
		CredentialID: cred.ID,
		AuthRealm:    cred.AuthRealm,
		ParamName:    paramName,
		Token:        token,
		CapturedAt:   time.Now().UTC().Format(time.RFC3339),
		SourceURL:    sourceURL,
	}
	replaced := false
	for i := range c.Entries {
		if c.Entries[i].CredentialID == cred.ID {
			c.Entries[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		c.Entries = append(c.Entries, entry)
	}
	if err := saveAuthCsrfCache(rt.WorkDir, c); err != nil {
		return err
	}
	note := fmt.Sprintf("csrf_meta: param=%s token=%s captured_at=%s", paramName, shrinkCsrfForLog(token), entry.CapturedAt)
	if strings.TrimSpace(cred.Notes) == "" {
		cred.Notes = note
	} else if !strings.Contains(cred.Notes, "csrf_meta:") {
		cred.Notes = strings.TrimSpace(cred.Notes) + "\n" + note
	}
	if rt.Repo != nil {
		return rt.Repo.UpdateAuthCredential(cred)
	}
	return nil
}

func shrinkCsrfForLog(token string) string {
	token = strings.TrimSpace(token)
	if len(token) <= 12 {
		return token
	}
	return token[:6] + "…" + token[len(token)-4:]
}

func extractCsrfFromHTML(html string) (paramName, token string) {
	html = strings.TrimSpace(html)
	if html == "" {
		return "", ""
	}
	if m := reCsrfInputNameFirst.FindStringSubmatch(html); len(m) > 2 {
		return strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
	}
	if m := reCsrfInputValueFirst.FindStringSubmatch(html); len(m) > 2 {
		return strings.TrimSpace(m[2]), strings.TrimSpace(m[1])
	}
	if m := reCsrfQueryInHTML.FindStringSubmatch(html); len(m) > 1 {
		return defaultCsrfParamName, strings.TrimSpace(m[1])
	}
	return "", ""
}

func captureCsrfFromHTTPResponse(rt *Runtime, cred *store.AuthCredential, sourceURL, content string) (string, error) {
	if rt == nil || cred == nil {
		return "", nil
	}
	paramName, token := extractCsrfFromHTML(content)
	if token == "" {
		return "", nil
	}
	if err := setCsrfTokenForCredential(rt, cred, paramName, token, sourceURL); err != nil {
		return "", err
	}
	return fmt.Sprintf("\n\n## csrf_auto_capture\n- param=%s token=%s\n- stored for auth_credential_id=%d; subsequent do_http_request will auto-inject\n",
		paramName, shrinkCsrfForLog(token), cred.ID), nil
}

func paramMapHasKey(params string, key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	for _, part := range strings.Split(params, "&") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if k, _, ok := strings.Cut(part, "="); ok && strings.EqualFold(strings.TrimSpace(k), key) {
			return true
		}
		if strings.EqualFold(part, key) {
			return true
		}
	}
	return false
}

func appendParamPair(params, key, value string) string {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return params
	}
	enc := url.QueryEscape(value)
	pair := key + "=" + enc
	if strings.TrimSpace(params) == "" {
		return pair
	}
	return strings.TrimSpace(params) + "&" + pair
}

func extractCsrfTokenFromPublicCMSAdminCookie(headersJSON string) (paramName, token string, ok bool) {
	if strings.TrimSpace(headersJSON) == "" {
		return "", "", false
	}
	var headers map[string]string
	if err := json.Unmarshal([]byte(headersJSON), &headers); err != nil {
		return "", "", false
	}
	cookieLine := ""
	for k, v := range headers {
		if strings.EqualFold(k, "Cookie") {
			cookieLine = v
			break
		}
	}
	if cookieLine == "" {
		return "", "", false
	}
	for _, part := range strings.Split(cookieLine, ";") {
		part = strings.TrimSpace(part)
		name, value, found := splitCookiePair(part)
		if !found || !strings.EqualFold(name, publicCMSAdminCookieName) {
			continue
		}
		userID, csrfTok, found := strings.Cut(value, "_")
		if !found || strings.TrimSpace(userID) == "" || strings.TrimSpace(csrfTok) == "" {
			return "", "", false
		}
		return defaultCsrfParamName, strings.TrimSpace(csrfTok), true
	}
	return "", "", false
}

// syncCsrfFromCredentialCookie stores CSRF from PUBLICCMS_ADMIN=userId_token when present.
func syncCsrfFromCredentialCookie(rt *Runtime, cred *store.AuthCredential) (paramName, token string, ok bool) {
	if rt == nil || cred == nil {
		return "", "", false
	}
	paramName, token, ok = extractCsrfTokenFromPublicCMSAdminCookie(cred.HeadersJSON)
	if !ok {
		return "", "", false
	}
	_ = setCsrfTokenForCredential(rt, cred, paramName, token, "PUBLICCMS_ADMIN cookie")
	return paramName, token, true
}

func stripManualCsrfFromParams(params aitool.InvokeParams, paramName string) []string {
	if params == nil {
		return nil
	}
	if strings.TrimSpace(paramName) == "" {
		paramName = defaultCsrfParamName
	}
	var notes []string
	for _, key := range []string{"post-params", "query-params", "body"} {
		raw, _ := params[key].(string)
		if raw == "" || !paramMapHasKey(raw, paramName) {
			continue
		}
		cleaned := removeParamKey(raw, paramName)
		if cleaned == "" {
			delete(params, key)
		} else {
			params[key] = cleaned
		}
		notes = append(notes, "removed manual "+paramName+" from "+key+" (engine will inject canonical token)")
	}
	return notes
}

func removeParamKey(params, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return strings.TrimSpace(params)
	}
	var kept []string
	for _, part := range strings.Split(params, "&") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, _, found := strings.Cut(part, "=")
		if found && strings.EqualFold(strings.TrimSpace(k), key) {
			continue
		}
		if !found && strings.EqualFold(part, key) {
			continue
		}
		kept = append(kept, part)
	}
	return strings.Join(kept, "&")
}

// applyCsrfTokenToHTTPParams injects cached CSRF when missing.
// Mutating methods use post-params/body; GET/HEAD @Csrf endpoints use query-params (PublicCMS ajaxTodo).
func applyCsrfTokenToHTTPParams(params aitool.InvokeParams, paramName, token string) []string {
	if params == nil || strings.TrimSpace(token) == "" {
		return nil
	}
	if strings.TrimSpace(paramName) == "" {
		paramName = defaultCsrfParamName
	}
	var notes []string
	method := strings.ToUpper(strings.TrimSpace(fmt.Sprint(params["method"])))
	if method == "" {
		method = "GET"
	}
	switch method {
	case "POST", "PUT", "PATCH", "DELETE":
		if post, _ := params["post-params"].(string); !paramMapHasKey(post, paramName) {
			params["post-params"] = appendParamPair(post, paramName, token)
			notes = append(notes, "auto-injected "+paramName+" into post-params")
		}
		if body, _ := params["body"].(string); body != "" && !paramMapHasKey(body, paramName) {
			ct := strings.ToLower(strings.TrimSpace(fmt.Sprint(params["content-type"])))
			if ct == "" || strings.Contains(ct, "x-www-form-urlencoded") || looksLikeFormBody(body) {
				params["body"] = appendParamPair(body, paramName, token)
				if len(notes) == 0 {
					notes = append(notes, "auto-injected "+paramName+" into body")
				}
			}
		}
	case "GET", "HEAD":
		if q, _ := params["query-params"].(string); !paramMapHasKey(q, paramName) {
			params["query-params"] = appendParamPair(q, paramName, token)
			notes = append(notes, "auto-injected "+paramName+" into query-params")
		}
	}
	return notes
}

func applyCachedCsrfForCredential(rt *Runtime, credID uint, params aitool.InvokeParams) []string {
	return applyCachedCsrfForCredentialIfRequired(rt, credID, params, true)
}

func applyCachedCsrfForCredentialIfRequired(rt *Runtime, credID uint, params aitool.InvokeParams, required bool) []string {
	if !required || rt == nil || credID == 0 || params == nil {
		return nil
	}
	paramName, token, ok := getCsrfTokenForCredential(rt.WorkDir, uint(credID))
	if !ok {
		return nil
	}
	return applyCsrfTokenToHTTPParams(params, paramName, token)
}

func httpRequestPathFromParams(params aitool.InvokeParams) string {
	if params == nil {
		return ""
	}
	raw := strings.TrimSpace(fmt.Sprint(params["url"]))
	if raw == "" {
		return ""
	}
	if u, err := url.Parse(raw); err == nil && strings.TrimSpace(u.Path) != "" {
		return normURLPath(u.Path)
	}
	return normURLPath(raw)
}

// requiresCsrfForHTTPParams returns true when combined_api_catalog marks the target with csrf_token.
func requiresCsrfForHTTPParams(rt *Runtime, params aitool.InvokeParams) bool {
	if rt == nil || params == nil || rt.WorkDir == "" {
		return false
	}
	catalog, err := loadCombinedAPICatalog(rt.WorkDir)
	if err != nil || catalog == nil {
		return false
	}
	method := strings.ToUpper(strings.TrimSpace(fmt.Sprint(params["method"])))
	if method == "" {
		method = "GET"
	}
	path := httpRequestPathFromParams(params)
	if path == "" {
		return false
	}
	rec := findCombinedAPIRecordExact(catalog, method, path)
	if rec == nil {
		rec = findCombinedAPIRecordExact(catalog, "", path)
	}
	return rec != nil && rec.RequiresCsrf()
}

func defaultCsrfFetchURL(rt *Runtime, cred *store.AuthCredential) string {
	if rt == nil || rt.Session == nil || cred == nil {
		return ""
	}
	base := strings.TrimSuffix(EffectiveTargetBaseURL(rt.Session), "/")
	mp := normURLPath(cred.MountPrefix)
	if mp == "" || mp == "/" {
		if realm := NormalizeAuthRealm(cred.AuthRealm); realm == AuthRealmAdmin {
			mp = "/admin"
		}
	}
	path := normURLPath(cred.VerifyURL)
	if path == "" || path == "/" {
		switch NormalizeAuthRealm(cred.AuthRealm) {
		case AuthRealmAdmin:
			path = "/admin/"
		default:
			path = "/"
		}
	}
	if mp != "" && mp != "/" && !strings.HasPrefix(path, mp+"/") && path != mp {
		path = joinURLPath(mp, strings.TrimPrefix(path, "/"))
	}
	return base + path
}

func csrfFetchURLsForCredential(rt *Runtime, cred *store.AuthCredential) []string {
	if rt == nil || cred == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var urls []string
	add := func(u string) {
		u = strings.TrimSpace(u)
		if u == "" {
			return
		}
		if _, ok := seen[u]; ok {
			return
		}
		seen[u] = struct{}{}
		urls = append(urls, u)
	}
	add(defaultCsrfFetchURL(rt, cred))
	base := strings.TrimSuffix(EffectiveTargetBaseURL(rt.Session), "/")
	switch NormalizeAuthRealm(cred.AuthRealm) {
	case AuthRealmAdmin:
		add(base + "/admin/")
		add(base + "/admin/index")
	case AuthRealmWeb:
		add(base + "/")
		add(base + "/login.html")
	case AuthRealmAPI:
		add(base + "/api/")
	}
	return urls
}

// prefetchCsrfTokenForCredential GETs realm pages until a CSRF token is cached.
func prefetchCsrfTokenForCredential(ctx context.Context, invoker aicommon.AIInvokeRuntime, rt *Runtime, cred *store.AuthCredential) error {
	if rt == nil || cred == nil {
		return utils.Error("nil prefetch context")
	}
	if _, _, ok := getCsrfTokenForCredential(rt.WorkDir, cred.ID); ok {
		return nil
	}
	urls := csrfFetchURLsForCredential(rt, cred)
	if len(urls) == 0 {
		return utils.Error("no csrf fetch url candidates")
	}
	var lastErr error
	for _, fetchURL := range urls {
		if _, _, ok := getCsrfTokenForCredential(rt.WorkDir, cred.ID); ok {
			return nil
		}
		_, _, _, err := fetchCsrfTokenWithCredential(ctx, invoker, rt, cred, fetchURL)
		if err == nil {
			return nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = utils.Error("csrf token not found")
	}
	return lastErr
}

// PrefetchCsrfTokensForSession fetches CSRF tokens for verified credentials missing cache entries.
func PrefetchCsrfTokensForSession(ctx context.Context, invoker aicommon.AIInvokeRuntime, rt *Runtime) (prefetched int, warnings []string) {
	if rt == nil || rt.Repo == nil || rt.Session == nil || !rt.Session.TargetReachable {
		return 0, nil
	}
	creds, err := rt.Repo.ListAuthCredentials(rt.Session.ID)
	if err != nil {
		return 0, []string{err.Error()}
	}
	for i := range creds {
		cred := creds[i]
		if !cred.Verified || strings.TrimSpace(cred.HeadersJSON) == "" {
			continue
		}
		if _, _, ok := syncCsrfFromCredentialCookie(rt, &cred); ok {
			prefetched++
			continue
		}
		if _, _, ok := getCsrfTokenForCredential(rt.WorkDir, cred.ID); ok {
			continue
		}
		if err := prefetchCsrfTokenForCredential(ctx, invoker, rt, &cred); err != nil {
			warnings = append(warnings, fmt.Sprintf("credential_id=%d realm=%s: %v", cred.ID, cred.AuthRealm, err))
			continue
		}
		prefetched++
	}
	return prefetched, warnings
}

func fetchCsrfTokenWithCredential(ctx context.Context, invoker aicommon.AIInvokeRuntime, rt *Runtime, cred *store.AuthCredential, fetchURL string) (paramName, token string, rawFeedback string, err error) {
	if invoker == nil || rt == nil || cred == nil {
		return "", "", "", utils.Error("nil fetch context")
	}
	if strings.TrimSpace(fetchURL) == "" {
		fetchURL = defaultCsrfFetchURL(rt, cred)
	}
	if fetchURL == "" {
		return "", "", "", utils.Error("empty csrf fetch url")
	}
	params := aitool.InvokeParams{
		"url":    fetchURL,
		"method": "GET",
	}
	applyAuthCredentialToHTTPParams(params, cred)
	result, _, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, "do_http_request", params)
	if err != nil {
		return "", "", "", err
	}
	content := toolResultTextContent(result)
	paramName, token = extractCsrfFromHTML(content)
	if token == "" {
		return "", "", utils.ShrinkString(content, 4000), utils.Error("csrf token not found in fetch response")
	}
	if err := setCsrfTokenForCredential(rt, cred, paramName, token, fetchURL); err != nil {
		return paramName, token, "", err
	}
	return paramName, token, utils.ShrinkString(content, 2000), nil
}

func buildDiscoveryFetchCsrfToken(r aicommon.AIInvokeRuntime, rt *Runtime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_fetch_csrf_token",
		"GET a page with auth_credential_id, extract CSRF token (_csrf), store and return for auto-injection on later requests.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("auth_credential_id", aitool.WithParam_Required(true)),
			aitool.WithStringParam("fetch_url", aitool.WithParam_Description("optional; default admin verify/index URL for credential realm")),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			if rt == nil || rt.Repo == nil || rt.Session == nil {
				op.Feedback("nil runtime")
				op.Continue()
				return
			}
			credID := action.GetInt("auth_credential_id")
			if credID <= 0 {
				credID = resolveHTTPAuthCredentialID(0, loop)
			}
			if credID <= 0 {
				op.Feedback("auth_credential_id required")
				op.Continue()
				return
			}
			cred, err := rt.Repo.GetAuthCredential(rt.Session.ID, uint(credID))
			if err != nil || cred == nil {
				op.Feedback(fmt.Sprintf("credential %d not found: %v", credID, err))
				op.Continue()
				return
			}
			ctx := loop.GetConfig().GetContext()
			fetchURL := strings.TrimSpace(action.GetString("fetch_url"))
			var paramName, token, raw string
			var ferr error
			if fetchURL != "" {
				paramName, token, raw, ferr = fetchCsrfTokenWithCredential(ctx, r, rt, cred, fetchURL)
			} else {
				ferr = prefetchCsrfTokenForCredential(ctx, r, rt, cred)
				if ferr == nil {
					paramName, token, _ = getCsrfTokenForCredential(rt.WorkDir, cred.ID)
				}
			}
			if ferr != nil {
				op.Feedback(fmt.Sprintf("discovery_fetch_csrf_token failed: %v\n%s", ferr, raw))
				op.Continue()
				return
			}
			op.Feedback(fmt.Sprintf("csrf captured: param=%s token=%s credential_id=%d\nSubsequent do_http_request with auth_credential_id will auto-inject this token.",
				paramName, shrinkCsrfForLog(token), credID))
			op.Continue()
		},
	)
}

func formatCsrfInjectNotes(notes []string) string {
	if len(notes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## csrf_auto_inject\n")
	for _, n := range notes {
		b.WriteString("- ")
		b.WriteString(n)
		b.WriteString("\n")
	}
	return b.String()
}
