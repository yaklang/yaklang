package loop_ssa_api_discovery

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

const loopKeySelectedAuthCredentialID = "selected_auth_credential_id"

var authHeaderNames = map[string]struct{}{
	"cookie":        {},
	"authorization": {},
	"x-api-key":     {},
	"x-auth-token":  {},
	"x-csrf-token":  {},
}

// applyAuthCredentialToHTTPParams injects DB credential headers and strips manual auth headers when credID>0.
func applyAuthCredentialToHTTPParams(params aitool.InvokeParams, cred *store.AuthCredential) (notes []string) {
	if params == nil || cred == nil {
		return nil
	}
	SyncCredentialHeaderFields(cred)
	if _, hadManual := stripManualAuthHeadersFromParams(params); hadManual {
		notes = append(notes, "removed manual auth headers; using auth_credential_id injection only")
	}
	authText := credentialHeadersText(cred)
	if authText == "" {
		notes = append(notes, "auth_credential has no injectable headers")
		return notes
	}
	existing, _ := params["headers"].(string)
	if strings.TrimSpace(existing) != "" {
		params["headers"] = existing + "\n" + authText
	} else {
		params["headers"] = authText
	}
	return notes
}

func credentialHeadersText(cred *store.AuthCredential) string {
	if cred == nil {
		return ""
	}
	if strings.TrimSpace(cred.HeadersText) != "" {
		return cred.HeadersText
	}
	if cred.HeaderName != "" && cred.HeaderValue != "" {
		return fmt.Sprintf("%s: %s", cred.HeaderName, cred.HeaderValue)
	}
	return ""
}

func stripManualAuthHeadersFromParams(params aitool.InvokeParams) (strippedNotes []string, stripped bool) {
	if params == nil {
		return nil, false
	}
	raw, ok := params["headers"].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, false
	}
	var kept []string
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			kept = append(kept, line)
			continue
		}
		name := strings.ToLower(strings.TrimSpace(parts[0]))
		if _, isAuth := authHeaderNames[name]; isAuth {
			stripped = true
			strippedNotes = append(strippedNotes, "stripped manual header "+strings.TrimSpace(parts[0]))
			continue
		}
		kept = append(kept, line)
	}
	if stripped {
		if len(kept) == 0 {
			delete(params, "headers")
		} else {
			params["headers"] = strings.Join(kept, "\n")
		}
	}
	return strippedNotes, stripped
}

func resolveHTTPAuthCredentialID(loopCredID int, loop *reactloops.ReActLoop) int {
	if loopCredID > 0 {
		return loopCredID
	}
	if loop == nil {
		return 0
	}
	for _, key := range []string{loopKeySelectedAuthCredentialID, "probe_suggested_auth_credential_id"} {
		if v := strings.TrimSpace(loop.Get(key)); v != "" {
			var id int
			if _, err := fmt.Sscanf(v, "%d", &id); err == nil && id > 0 {
				return id
			}
		}
	}
	return 0
}

func formatAuthInjectNotes(notes []string) string {
	if len(notes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## auth_credential injection\n")
	for _, n := range notes {
		b.WriteString("- ")
		b.WriteString(n)
		b.WriteString("\n")
	}
	b.WriteString("- Use **auth_credential_id** only; do not copy Cookie/headers from DB manually.\n")
	return b.String()
}
