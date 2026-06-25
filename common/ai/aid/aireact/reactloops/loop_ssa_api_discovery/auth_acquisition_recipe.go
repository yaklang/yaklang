package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
)

// HeadersJSONToText converts a JSON map of headers to "Header: value\r\n..." format.
func HeadersJSONToText(headersJSON string) string {
	if strings.TrimSpace(headersJSON) == "" {
		return ""
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(headersJSON), &m); err != nil {
		return ""
	}
	return HeadersMapToText(m)
}

// HeadersMapToText converts a map of headers to "Header: value\r\n..." format.
func HeadersMapToText(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	var parts []string
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%s: %s", k, v))
	}
	return strings.Join(parts, "\r\n")
}

// HeadersTextToJSON converts "Header: value\r\n..." format back to JSON.
func HeadersTextToJSON(text string) string {
	m := HeadersTextToMap(text)
	if len(m) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// HeadersTextToMap parses "Header: value\r\n..." text into a map.
func HeadersTextToMap(text string) map[string]string {
	m := make(map[string]string)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			m[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return m
}

// SyncCredentialHeaderFields ensures HeadersText, HeaderName, HeaderValue are
// derived from HeadersJSON (the canonical source).
func SyncCredentialHeaderFields(cred *store.AuthCredential) {
	if cred == nil {
		return
	}
	if cred.HeadersJSON != "" {
		cred.HeadersText = HeadersJSONToText(cred.HeadersJSON)
		var m map[string]string
		if err := json.Unmarshal([]byte(cred.HeadersJSON), &m); err == nil && len(m) > 0 {
			for k, v := range m {
				cred.HeaderName = k
				cred.HeaderValue = v
				break
			}
		}
	} else if cred.HeaderName != "" && cred.HeaderValue != "" {
		m := map[string]string{cred.HeaderName: cred.HeaderValue}
		b, _ := json.Marshal(m)
		cred.HeadersJSON = string(b)
		cred.HeadersText = HeadersMapToText(m)
	}
}

// pickPrimaryAuthHeaderFromMap 从 headers_json 中选一条用于 --auth-header CLI；多键时优先 Cookie、Authorization 等（批量探测仍以 auth-headers-file 为准）。
func pickPrimaryAuthHeaderFromMap(m map[string]string) (headerName, headerValue string) {
	if len(m) == 0 {
		return "", ""
	}
	prio := []string{"Cookie", "Authorization", "X-API-Key", "X-CSRF-Token", "X-Auth-Token"}
	for _, want := range prio {
		for k, v := range m {
			if strings.EqualFold(strings.TrimSpace(k), want) {
				v = strings.TrimSpace(v)
				if v != "" {
					return strings.TrimSpace(k), v
				}
			}
		}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	k := keys[0]
	return strings.TrimSpace(k), strings.TrimSpace(m[k])
}

// BuildAuthHeaderCLIArg builds the --auth-header CLI argument from a credential.
// Returns empty string if no headers available.
func BuildAuthHeaderCLIArg(cred *store.AuthCredential) string {
	if cred == nil {
		return ""
	}
	if cred.HeaderName != "" && cred.HeaderValue != "" {
		return fmt.Sprintf("%s: %s", cred.HeaderName, cred.HeaderValue)
	}
	if cred.HeadersJSON != "" {
		var m map[string]string
		if err := json.Unmarshal([]byte(cred.HeadersJSON), &m); err == nil && len(m) > 0 {
			k, v := pickPrimaryAuthHeaderFromMap(m)
			if k != "" && v != "" {
				return fmt.Sprintf("%s: %s", k, v)
			}
		}
	}
	return ""
}

// GetDefaultCredentialForSession returns the best available credential for scanning.
func GetDefaultCredentialForSession(rt *Runtime) *store.AuthCredential {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil
	}
	cred, err := rt.Repo.GetFreshestVerifiedCredential(rt.Session.ID)
	if err != nil {
		creds, lerr := rt.Repo.ListVerifiedAuthCredentials(rt.Session.ID)
		if lerr != nil || len(creds) == 0 {
			return nil
		}
		cred = &creds[len(creds)-1]
	}
	if cred != nil {
		SyncCredentialHeaderFields(cred)
		log.Infof("auth: default credential id=%d type=%s verified=%v", cred.ID, cred.AuthType, cred.Verified)
	}
	return cred
}
