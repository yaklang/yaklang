package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

// mergeResponseCookiesIntoCredential merges Set-Cookie pairs from an HTTP tool response
// into the stored credential, preserving unrelated cookies and skipping cleared/expired ones.
func mergeResponseCookiesIntoCredential(cred *store.AuthCredential, httpContent string) (updated bool, notes []string) {
	if cred == nil {
		return false, nil
	}
	newPairs := filterValidSessionCookiePairs(extractAllSetCookiePairsFromResponse(httpContent))
	if len(newPairs) == 0 {
		return false, nil
	}
	existing := map[string]string{}
	if strings.TrimSpace(cred.HeadersJSON) != "" {
		var m map[string]string
		if err := json.Unmarshal([]byte(cred.HeadersJSON), &m); err == nil {
			for k, v := range m {
				if strings.EqualFold(k, "Cookie") {
					for _, pair := range strings.Split(v, ";") {
						name, val, ok := splitCookiePair(strings.TrimSpace(pair))
						if ok && !isClearedSessionCookie(name, val) {
							existing[name] = val
						}
					}
				} else if strings.TrimSpace(v) != "" {
					existing[k] = v
				}
			}
		}
	}
	changed := false
	for _, pair := range newPairs {
		name, val, ok := splitCookiePair(pair)
		if !ok {
			continue
		}
		if old, ok := existing[name]; !ok || old != val {
			existing[name] = val
			changed = true
			notes = append(notes, fmt.Sprintf("refreshed cookie %s from response Set-Cookie", name))
		}
	}
	if !changed {
		return false, notes
	}
	var cookieParts []string
	for name, val := range existing {
		cookieParts = append(cookieParts, name+"="+val)
	}
	headers := map[string]string{"Cookie": strings.Join(cookieParts, "; ")}
	b, err := json.Marshal(headers)
	if err != nil {
		return false, notes
	}
	cred.HeadersJSON = string(b)
	SyncCredentialHeaderFields(cred)
	return true, notes
}

func refreshAuthCredentialFromHTTPResponse(rt *Runtime, cred *store.AuthCredential, httpContent string) []string {
	if rt == nil || cred == nil || rt.Repo == nil {
		return nil
	}
	updated, notes := mergeResponseCookiesIntoCredential(cred, httpContent)
	if !updated {
		return notes
	}
	if err := rt.Repo.UpdateAuthCredential(cred); err != nil {
		notes = append(notes, "credential cookie refresh persist failed: "+err.Error())
		return notes
	}
	if _, _, ok := syncCsrfFromCredentialCookie(rt, cred); ok {
		notes = append(notes, "csrf re-synced from refreshed PUBLICCMS_ADMIN cookie")
	}
	return notes
}
