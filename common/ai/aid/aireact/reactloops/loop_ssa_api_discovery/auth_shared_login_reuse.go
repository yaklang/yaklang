package loop_ssa_api_discovery

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
)

// loginEndpointIdentity groups auth realms that share the same login HTTP surface.
func loginEndpointIdentity(method, path string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "POST"
	}
	return method + "|" + normURLPath(path)
}

func loginEndpointIdentityFromCredential(c *store.AuthCredential) string {
	if c == nil {
		return ""
	}
	return loginEndpointIdentity("POST", c.LoginPath)
}

func loginEndpointIdentityFromEndpoint(ep AuthLoginEndpoint) string {
	return loginEndpointIdentity(ep.Method, ep.Path)
}

func collectLoginEndpointIdentities(ev *AuthEvidenceRecord) map[string][]string {
	out := map[string][]string{}
	if ev == nil {
		return out
	}
	add := func(identity, realm string) {
		realm = NormalizeAuthRealm(realm)
		if identity == "" || realm == "" {
			return
		}
		for _, existing := range out[identity] {
			if existing == realm {
				return
			}
		}
		out[identity] = append(out[identity], realm)
	}
	for _, ep := range ev.LoginEndpoints {
		identity := loginEndpointIdentityFromEndpoint(ep)
		realm := NormalizeAuthRealm(ep.AuthRealm)
		if realm == "" {
			realm = InferAuthRealmFromLoginPath(ep.Path, ep.FullURL)
		}
		add(identity, realm)
	}
	return out
}

func endpointForRealmOnIdentity(ev *AuthEvidenceRecord, identity, authRealm string) *AuthLoginEndpoint {
	if ev == nil {
		return nil
	}
	authRealm = NormalizeAuthRealm(authRealm)
	for i := range ev.LoginEndpoints {
		ep := &ev.LoginEndpoints[i]
		if loginEndpointIdentityFromEndpoint(*ep) != identity {
			continue
		}
		r := NormalizeAuthRealm(ep.AuthRealm)
		if r == "" {
			r = InferAuthRealmFromLoginPath(ep.Path, ep.FullURL)
		}
		if r == authRealm {
			return ep
		}
	}
	return nil
}

func hasDirectVerifiedCredentialForRealm(rt *Runtime, authRealm string) bool {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return false
	}
	authRealm = NormalizeAuthRealm(authRealm)
	creds, err := rt.Repo.ListAuthCredentials(rt.Session.ID)
	if err != nil {
		return false
	}
	for _, c := range creds {
		if !c.Verified || strings.TrimSpace(c.HeadersJSON) == "" {
			continue
		}
		cRealm := NormalizeAuthRealm(c.AuthRealm)
		if cRealm == "" {
			cRealm = InferAuthRealmFromLoginPath(c.LoginPath, c.VerifyURL)
		}
		if authRealm != "" && cRealm == authRealm {
			return true
		}
	}
	return false
}

func anyVerifiedCredentialForLoginIdentity(creds []store.AuthCredential, identity string) *store.AuthCredential {
	if identity == "" {
		return nil
	}
	var best *store.AuthCredential
	for i := range creds {
		c := &creds[i]
		if !c.Verified || strings.TrimSpace(c.HeadersJSON) == "" {
			continue
		}
		if loginEndpointIdentityFromCredential(c) != identity {
			continue
		}
		if best == nil || c.ID > best.ID {
			best = c
		}
	}
	return best
}

// realmSatisfiedViaSharedLogin is true when another realm's verified credential uses the same login endpoint.
func realmSatisfiedViaSharedLogin(rt *Runtime, ev *AuthEvidenceRecord, authRealm string) bool {
	if rt == nil || ev == nil || rt.Repo == nil || rt.Session == nil {
		return false
	}
	authRealm = NormalizeAuthRealm(authRealm)
	if authRealm == "" {
		return false
	}
	creds, err := rt.Repo.ListAuthCredentials(rt.Session.ID)
	if err != nil {
		return false
	}
	groups := collectLoginEndpointIdentities(ev)
	for identity, realms := range groups {
		hasTarget := false
		for _, r := range realms {
			if r == authRealm {
				hasTarget = true
				break
			}
		}
		if !hasTarget {
			continue
		}
		if len(realms) < 2 {
			continue
		}
		if src := anyVerifiedCredentialForLoginIdentity(creds, identity); src != nil {
			return true
		}
	}
	return false
}

func realmHasVerifiedCredentialWithEvidence(rt *Runtime, ev *AuthEvidenceRecord, authRealm string) bool {
	if hasDirectVerifiedCredentialForRealm(rt, authRealm) {
		return true
	}
	if ev == nil && rt != nil && strings.TrimSpace(rt.WorkDir) != "" {
		ev, _ = loadAuthEvidenceFromWorkDir(rt.WorkDir)
	}
	return realmSatisfiedViaSharedLogin(rt, ev, authRealm)
}

// EnsureSharedLoginCredentialAliases creates verified credential rows for realms that share a login
// endpoint with an already-verified credential (e.g. api reuses admin session from POST /admin/login).
func EnsureSharedLoginCredentialAliases(rt *Runtime, ev *AuthEvidenceRecord) int {
	if rt == nil || ev == nil || rt.Repo == nil || rt.Session == nil {
		return 0
	}
	creds, err := rt.Repo.ListAuthCredentials(rt.Session.ID)
	if err != nil {
		return 0
	}
	created := 0
	for identity, realms := range collectLoginEndpointIdentities(ev) {
		if len(realms) < 2 {
			continue
		}
		src := anyVerifiedCredentialForLoginIdentity(creds, identity)
		if src == nil {
			continue
		}
		for _, targetRealm := range realms {
			if hasDirectVerifiedCredentialForRealm(rt, targetRealm) {
				continue
			}
			ep := endpointForRealmOnIdentity(ev, identity, targetRealm)
			row := cloneCredentialForSharedLoginRealm(rt, src, targetRealm, ep)
			if row == nil {
				continue
			}
			if err := rt.Repo.CreateAuthCredential(row); err != nil {
				log.Warnf("ssa_api_discovery: shared_login alias realm=%s from id=%d: %v", targetRealm, src.ID, err)
				continue
			}
			created++
			log.Infof("ssa_api_discovery: shared_login_reuse created credential id=%d realm=%s from id=%d login=%s",
				row.ID, targetRealm, src.ID, identity)
			creds = append(creds, *row)
		}
	}
	return created
}

func cloneCredentialForSharedLoginRealm(rt *Runtime, src *store.AuthCredential, targetRealm string, ep *AuthLoginEndpoint) *store.AuthCredential {
	if src == nil || !src.Verified || strings.TrimSpace(src.HeadersJSON) == "" {
		return nil
	}
	targetRealm = NormalizeAuthRealm(targetRealm)
	if targetRealm == "" {
		return nil
	}
	srcRealm := NormalizeAuthRealm(src.AuthRealm)
	identity := loginEndpointIdentityFromCredential(src)
	row := &store.AuthCredential{
		SessionID:         rt.Session.ID,
		AuthType:          src.AuthType,
		Username:          src.Username,
		TokenValue:        src.TokenValue,
		HeaderName:        src.HeaderName,
		HeaderValue:       src.HeaderValue,
		HeadersJSON:       src.HeadersJSON,
		HeadersText:       src.HeadersText,
		URLSpace:          src.URLSpace,
		AuthRealm:         targetRealm,
		CredentialGroupID: src.CredentialGroupID,
		MountPrefix:       src.MountPrefix,
		LoginPath:         src.LoginPath,
		LoginEvidenceJSON: src.LoginEvidenceJSON,
		Verified:          true,
		VerifyURL:         src.VerifyURL,
		LastAcquiredAt:    src.LastAcquiredAt,
		LastVerifiedAt:    src.LastVerifiedAt,
		Notes: fmt.Sprintf("shared_login_reuse from credential id=%d realm=%s login=%s",
			src.ID, srcRealm, identity),
	}
	if ep != nil {
		if mp := normURLPath(ep.MountPrefix); mp != "" {
			row.MountPrefix = mp
		}
		if lp := strings.TrimSpace(ep.Path); lp != "" {
			row.LoginPath = lp
		}
		if us := strings.TrimSpace(ep.URLSpace); us != "" {
			row.URLSpace = us
		}
	}
	SyncCredentialHeaderFields(row)
	return row
}

func propagateSharedLoginEndpointCredentialIDs(ev *AuthEvidenceRecord, creds []store.AuthCredential) {
	if ev == nil || len(creds) == 0 {
		return
	}
	for i := range ev.LoginEndpoints {
		ep := &ev.LoginEndpoints[i]
		if ep.CredentialID > 0 {
			continue
		}
		identity := loginEndpointIdentityFromEndpoint(*ep)
		if c := anyVerifiedCredentialForLoginIdentity(creds, identity); c != nil {
			ep.CredentialID = c.ID
		}
	}
}
