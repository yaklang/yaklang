package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/utils"
)

// Auth realm labels (generic; not project-specific).
const (
	AuthRealmAdmin  = "admin"
	AuthRealmWeb    = "web"
	AuthRealmAPI    = "api"
	AuthRealmOAuth  = "oauth"
	AuthRealmMember = "member"
	AuthRealmPublic = "public"
)

// AuthCredentialSummary is injected into probe/verify prompts for AI credential selection.
type AuthCredentialSummary struct {
	ID                uint     `json:"id"`
	AuthType          string   `json:"auth_type,omitempty"`
	URLSpace          string   `json:"url_space,omitempty"`
	AuthRealm         string   `json:"auth_realm,omitempty"`
	CredentialGroupID string   `json:"credential_group_id,omitempty"`
	MountPrefix       string   `json:"mount_prefix,omitempty"`
	LoginPath         string   `json:"login_path,omitempty"`
	VerifyURL         string   `json:"verify_url,omitempty"`
	Verified          bool     `json:"verified"`
	Username          string   `json:"username,omitempty"`
	Label             string   `json:"label,omitempty"`
	HeaderKeys        []string `json:"header_keys,omitempty"`
}

// ProbeAuthSelectionHint helps the probe agent pick auth_credential_id.
type ProbeAuthSelectionHint struct {
	MultiAuth                 bool                    `json:"multi_auth"`
	HandlerAuthRealm          string                  `json:"handler_auth_realm,omitempty"`
	HandlerURLSpace           string                  `json:"handler_url_space,omitempty"`
	SuggestedAuthCredentialID uint                    `json:"suggested_auth_credential_id,omitempty"`
	SelectionReason           string                  `json:"selection_reason,omitempty"`
	AvailableCredentials      []AuthCredentialSummary `json:"available_credentials,omitempty"`
}

// DetectMultiAuth reports whether the project likely has multiple independent auth surfaces.
func DetectMultiAuth(rt *Runtime, ev *AuthEvidenceRecord) bool {
	if ev != nil && ev.MultiAuth {
		return true
	}
	if ev != nil && len(ev.LoginEndpoints) >= 2 {
		realms := map[string]struct{}{}
		for _, ep := range ev.LoginEndpoints {
			r := NormalizeAuthRealm(ep.AuthRealm)
			if r == "" {
				r = InferAuthRealmFromLoginPath(ep.Path, ep.FullURL)
			}
			if r != "" && r != AuthRealmPublic {
				realms[r] = struct{}{}
			}
		}
		if len(realms) >= 2 {
			return true
		}
		prefixes := map[string]struct{}{}
		for _, ep := range ev.LoginEndpoints {
			p := strings.TrimSpace(ep.MountPrefix)
			if p == "" {
				p = extractMountPrefixFromLoginPath(ep.Path, ep.FullURL)
			}
			if p != "" && p != "/" {
				prefixes[p] = struct{}{}
			}
		}
		if len(prefixes) >= 2 {
			return true
		}
	}
	if rt != nil {
		if multi := detectMultiAuthFromRouting(rt); multi {
			return true
		}
		if multi := detectMultiAuthFromPlan(rt); multi {
			return true
		}
	}
	return false
}

func detectMultiAuthFromRouting(rt *Runtime) bool {
	rp, err := loadRoutingProfileFromWorkDir(rt.WorkDir)
	if err != nil || rp == nil || len(rp.URLSpaces) < 2 {
		return false
	}
	mounts := map[string]struct{}{}
	hasRoot := false
	for _, sp := range rp.URLSpaces {
		m := normURLPath(strings.TrimSpace(sp.MountPrefix))
		if m == "" || isUnknownMountPrefix(m) {
			continue
		}
		if m == "/" {
			hasRoot = true
			continue
		}
		mounts[m] = struct{}{}
	}
	if len(mounts) >= 2 {
		return true
	}
	return len(mounts) >= 1 && hasRoot
}

func detectMultiAuthFromPlan(rt *Runtime) bool {
	plan, err := LoadCodeReadingPlanForRuntime(rt)
	if err != nil || plan == nil {
		return false
	}
	adminLogin, webLogin := false, false
	for _, ep := range plan.DiscoveredAPIs {
		seg := normalizePathSegment(lastURLPathSegment(ep.PathPattern))
		if seg != normalizePathSegment("login") && seg != normalizePathSegment("logindialog") {
			continue
		}
		hc := strings.ToLower(ep.HandlerClass)
		if strings.Contains(hc, ".controller.admin") || strings.Contains(hc, ".admin.") {
			adminLogin = true
		}
		if strings.Contains(hc, ".controller.web") || strings.Contains(hc, ".web.") {
			webLogin = true
		}
	}
	return adminLogin && webLogin
}

// NormalizeAuthRealm canonicalizes realm strings.
func NormalizeAuthRealm(realm string) string {
	r := strings.ToLower(strings.TrimSpace(realm))
	switch r {
	case AuthRealmAdmin, AuthRealmWeb, AuthRealmAPI, AuthRealmOAuth, AuthRealmMember, AuthRealmPublic:
		return r
	case "backend", "management", "backstage":
		return AuthRealmAdmin
	case "frontend", "front", "user", "portal":
		return AuthRealmWeb
	}
	return r
}

// InferAuthRealmFromHandlerClass maps handler package to auth realm.
func InferAuthRealmFromHandlerClass(handlerClass string) string {
	hc := strings.ToLower(strings.TrimSpace(handlerClass))
	switch {
	case strings.Contains(hc, ".controller.admin"), strings.Contains(hc, ".admin."):
		return AuthRealmAdmin
	case strings.Contains(hc, ".controller.api"), strings.Contains(hc, ".api."):
		return AuthRealmAPI
	case strings.Contains(hc, ".controller.web"), strings.Contains(hc, ".web."):
		return AuthRealmWeb
	case strings.Contains(hc, ".oauth"):
		return AuthRealmOAuth
	}
	return ""
}

// InferAuthRealmFromLoginPath infers realm from login URL path prefix.
func InferAuthRealmFromLoginPath(path, fullURL string) string {
	p := strings.TrimSpace(path)
	if p == "" && fullURL != "" {
		if u, err := url.Parse(fullURL); err == nil {
			p = u.Path
		}
	}
	p = strings.ToLower(normURLPath(p))
	switch {
	case strings.HasPrefix(p, "/admin"), strings.Contains(p, "/admin/"):
		return AuthRealmAdmin
	case strings.HasPrefix(p, "/api"), strings.Contains(p, "/api/"):
		return AuthRealmAPI
	case strings.Contains(p, "/oauth"):
		return AuthRealmOAuth
	case p == "/login" || strings.HasSuffix(p, "/login"):
		return AuthRealmWeb
	}
	return ""
}

func extractMountPrefixFromLoginPath(path, fullURL string) string {
	p := strings.TrimSpace(path)
	if p == "" && fullURL != "" {
		if u, err := url.Parse(fullURL); err == nil {
			p = u.Path
		}
	}
	p = normURLPath(p)
	if i := strings.LastIndex(p, "/"); i > 0 {
		return p[:i]
	}
	return ""
}

// RequiredAuthRealms returns distinct realms that need verified credentials when multi_auth.
func RequiredAuthRealms(rt *Runtime, ev *AuthEvidenceRecord) []string {
	if ev == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(r string) {
		r = NormalizeAuthRealm(r)
		if r == "" || r == AuthRealmPublic {
			return
		}
		if _, ok := seen[r]; ok {
			return
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	for _, ep := range ev.LoginEndpoints {
		add(ep.AuthRealm)
		if ep.AuthRealm == "" {
			add(InferAuthRealmFromLoginPath(ep.Path, ep.FullURL))
		}
	}
	if len(out) == 0 && DetectMultiAuth(rt, ev) {
		add(AuthRealmAdmin)
		add(AuthRealmWeb)
	}
	return out
}

func credentialBindingKey(urlSpace, authRealm, mountPrefix string) string {
	us := strings.TrimSpace(urlSpace)
	ar := NormalizeAuthRealm(authRealm)
	mp := normURLPath(mountPrefix)
	if us != "" {
		return "space:" + us
	}
	if ar != "" {
		return "realm:" + ar
	}
	if mp != "" && mp != "/" {
		return "mount:" + mp
	}
	return ""
}

// HasAuthCredentialsSatisfied checks whether required auth mechanisms are ready.
func HasAuthCredentialsSatisfied(rt *Runtime, ev *AuthEvidenceRecord) bool {
	if !hasVerifiedAuthCredential(rt) {
		return false
	}
	if !DetectMultiAuth(rt, ev) {
		return true
	}
	realms := RequiredAuthRealms(rt, ev)
	if len(realms) == 0 {
		return true
	}
	return allRequiredRealmsHaveVerifiedCredentials(rt, realms)
}

// SyncCredentialBindingsFromDB rebuilds auth_evidence.credential_bindings from auth_credentials rows.
func SyncCredentialBindingsFromDB(rt *Runtime, ev *AuthEvidenceRecord) {
	if rt == nil || ev == nil || rt.Repo == nil || rt.Session == nil {
		return
	}
	creds, err := rt.Repo.ListAuthCredentials(rt.Session.ID)
	if err != nil {
		return
	}
	var bindings []AuthCredentialBinding
	for _, c := range creds {
		if !c.Verified || strings.TrimSpace(c.HeadersJSON) == "" {
			continue
		}
		bindings = append(bindings, AuthCredentialBinding{
			URLSpace:     strings.TrimSpace(c.URLSpace),
			AuthRealm:    NormalizeAuthRealm(c.AuthRealm),
			MountPrefix:  normURLPath(c.MountPrefix),
			LoginPath:    strings.TrimSpace(c.LoginPath),
			CredentialID: c.ID,
			Verified:     c.Verified,
			Label:        credentialLabel(c.URLSpace, c.AuthRealm, c.MountPrefix),
		})
	}
	ev.CredentialBindings = bindings
	ev.MultiAuth = DetectMultiAuth(rt, ev)
	if len(bindings) == 1 {
		ev.CredentialID = bindings[0].CredentialID
	}
	for i := range ev.LoginEndpoints {
		ep := &ev.LoginEndpoints[i]
		for _, b := range bindings {
			if credentialMatchesEndpoint(b, *ep) {
				ep.CredentialID = b.CredentialID
				break
			}
		}
	}
	if len(creds) > 0 {
		propagateSharedLoginEndpointCredentialIDs(ev, creds)
	}
}

func credentialMatchesEndpoint(b AuthCredentialBinding, ep AuthLoginEndpoint) bool {
	if b.CredentialID == 0 {
		return false
	}
	epRealm := NormalizeAuthRealm(ep.AuthRealm)
	if epRealm == "" {
		epRealm = InferAuthRealmFromLoginPath(ep.Path, ep.FullURL)
	}
	if b.AuthRealm != "" && epRealm != "" && b.AuthRealm == epRealm {
		return true
	}
	if b.URLSpace != "" && ep.URLSpace != "" && b.URLSpace == ep.URLSpace {
		return true
	}
	epMount := normURLPath(ep.MountPrefix)
	if epMount == "" {
		epMount = extractMountPrefixFromLoginPath(ep.Path, ep.FullURL)
	}
	if b.MountPrefix != "" && epMount != "" && b.MountPrefix == epMount {
		return true
	}
	return false
}

func credentialLabel(urlSpace, authRealm, mountPrefix string) string {
	if l := strings.TrimSpace(urlSpace); l != "" {
		return l
	}
	if r := NormalizeAuthRealm(authRealm); r != "" {
		return r
	}
	return strings.TrimSpace(mountPrefix)
}

func listVerifiedCredentialSummaries(rt *Runtime) ([]AuthCredentialSummary, error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil, utils.Error("nil runtime")
	}
	rows, err := rt.Repo.ListAuthCredentials(rt.Session.ID)
	if err != nil {
		return nil, err
	}
	out := make([]AuthCredentialSummary, 0, len(rows))
	for _, c := range rows {
		if !c.Verified || strings.TrimSpace(c.HeadersJSON) == "" {
			continue
		}
		out = append(out, credentialToSummary(&c))
	}
	return out, nil
}

func credentialToSummary(c *store.AuthCredential) AuthCredentialSummary {
	if c == nil {
		return AuthCredentialSummary{}
	}
	var keys []string
	if strings.TrimSpace(c.HeadersJSON) != "" {
		var hm map[string]string
		if json.Unmarshal([]byte(c.HeadersJSON), &hm) == nil {
			for k := range hm {
				keys = append(keys, k)
			}
		}
	}
	return AuthCredentialSummary{
		ID:                c.ID,
		AuthType:          c.AuthType,
		URLSpace:          c.URLSpace,
		AuthRealm:         NormalizeAuthRealm(c.AuthRealm),
		CredentialGroupID: strings.TrimSpace(c.CredentialGroupID),
		MountPrefix:       normURLPath(c.MountPrefix),
		LoginPath:   c.LoginPath,
		VerifyURL:   c.VerifyURL,
		Verified:    c.Verified,
		Username:    c.Username,
		Label:       credentialLabel(c.URLSpace, c.AuthRealm, c.MountPrefix),
		HeaderKeys:  keys,
	}
}

// BuildProbeAuthSelectionHint assembles credential selection context for API probe.
func BuildProbeAuthSelectionHint(rt *Runtime, handlerClass, urlSpace, mountPrefix string, requiresAuth bool) ProbeAuthSelectionHint {
	hint := ProbeAuthSelectionHint{MultiAuth: false}
	if rt == nil {
		return hint
	}
	ev, _ := loadAuthEvidenceFromWorkDir(rt.WorkDir)
	hint.MultiAuth = DetectMultiAuth(rt, ev)
	hint.HandlerURLSpace = strings.TrimSpace(urlSpace)
	hint.HandlerAuthRealm = InferAuthRealmFromHandlerClass(handlerClass)
	if hint.HandlerAuthRealm == "" && mountPrefix != "" {
		switch {
		case strings.Contains(strings.ToLower(mountPrefix), "admin"):
			hint.HandlerAuthRealm = AuthRealmAdmin
		case strings.Contains(strings.ToLower(mountPrefix), "api"):
			hint.HandlerAuthRealm = AuthRealmAPI
		}
	}
	creds, _ := listVerifiedCredentialSummaries(rt)
	hint.AvailableCredentials = creds
	if !requiresAuth {
		hint.SelectionReason = "endpoint marked public/no-auth; auth_credential_id optional"
		return hint
	}
	id, reason := ResolveCredentialIDForProbe(rt, handlerClass, urlSpace, mountPrefix, hint.HandlerAuthRealm)
	hint.SuggestedAuthCredentialID = id
	hint.SelectionReason = reason
	return hint
}

// ResolveCredentialIDForProbe picks the best matching verified credential id.
func ResolveCredentialIDForProbe(rt *Runtime, handlerClass, urlSpace, mountPrefix, authRealm string) (uint, string) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return 0, "no runtime"
	}
	authRealm = NormalizeAuthRealm(authRealm)
	if authRealm == "" {
		authRealm = InferAuthRealmFromHandlerClass(handlerClass)
	}
	urlSpace = strings.TrimSpace(urlSpace)
	mountPrefix = normURLPath(mountPrefix)

	creds, err := rt.Repo.ListVerifiedAuthCredentials(rt.Session.ID)
	if err != nil || len(creds) == 0 {
		creds, _ = rt.Repo.ListAuthCredentials(rt.Session.ID)
	}
	var verified []store.AuthCredential
	for _, c := range creds {
		if c.Verified && strings.TrimSpace(c.HeadersJSON) != "" {
			verified = append(verified, c)
		}
	}
	if len(verified) == 0 {
		return 0, "no verified credentials for auth_realm=" + authRealm + "; obtain auth via login calibration first; if endpoint still returns 401/403 without credential, mark auth_required_skipped"
	}
	if len(verified) == 1 {
		return verified[0].ID, "single verified credential"
	}

	bestScore := -1
	var best *store.AuthCredential
	for i := range verified {
		c := &verified[i]
		score := scoreCredentialMatch(c, urlSpace, mountPrefix, authRealm)
		if score > bestScore {
			bestScore = score
			best = c
		}
	}
	if best != nil && bestScore > 0 {
		return best.ID, fmt.Sprintf("matched credential id=%d by url_space=%s auth_realm=%s (score=%d); AI may override if response indicates wrong session",
			best.ID, best.URLSpace, best.AuthRealm, bestScore)
	}
	return verified[len(verified)-1].ID, "fallback to latest verified credential; AI should pick correct id from available_credentials if probe gets auth_required"
}

func scoreCredentialMatch(c *store.AuthCredential, urlSpace, mountPrefix, authRealm string) int {
	if c == nil {
		return 0
	}
	score := 0
	cRealm := NormalizeAuthRealm(c.AuthRealm)
	if cRealm == "" {
		cRealm = InferAuthRealmFromLoginPath(c.LoginPath, c.VerifyURL)
	}
	if authRealm != "" && cRealm != "" && authRealm == cRealm {
		score += 10
	}
	if urlSpace != "" && strings.TrimSpace(c.URLSpace) == urlSpace {
		score += 8
	}
	cMount := normURLPath(c.MountPrefix)
	if mountPrefix != "" && cMount != "" && mountPrefix == cMount {
		score += 6
	}
	if authRealm == AuthRealmAdmin && strings.Contains(strings.ToLower(c.LoginPath+c.VerifyURL), "/admin") {
		score += 3
	}
	if authRealm == AuthRealmWeb && cRealm == AuthRealmWeb {
		score += 3
	}
	return score
}

// CollectMultiAuthHintsForAuthLoop returns reactive hints for the auth ReAct loop.
func CollectMultiAuthHintsForAuthLoop(rt *Runtime) string {
	if rt == nil {
		return ""
	}
	var lines []string
	if detectMultiAuthFromRouting(rt) {
		lines = append(lines, "routing_profile has multiple url_spaces — expect multi_auth; login and upsert credential per space/realm")
	}
	if detectMultiAuthFromPlan(rt) {
		lines = append(lines, "code plan shows both admin and web login controllers — multi_auth likely")
	}
	rp, _ := loadRoutingProfileFromWorkDir(rt.WorkDir)
	if rp != nil {
		for _, sp := range rp.URLSpaces {
			if mp := strings.TrimSpace(sp.MountPrefix); mp != "" && mp != "/" {
				lines = append(lines, fmt.Sprintf("url_space id=%s mount_prefix=%s", sp.ID, mp))
			}
		}
	}
	if len(lines) == 0 {
		return "multi_auth: unknown yet — search for multiple Login*Controller / SecurityFilterChain / servlet mappings"
	}
	return strings.Join(lines, "\n")
}
