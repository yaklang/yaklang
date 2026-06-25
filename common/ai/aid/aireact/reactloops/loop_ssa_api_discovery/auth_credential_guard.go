package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
)

func credentialGroupKey(authRealm, groupID string) string {
	return NormalizeAuthRealm(authRealm) + "|" + normalizeCredentialGroupID(groupID)
}

func loopKeyGroupSatisfied(authRealm, groupID string) string {
	return "auth_group_satisfied_" + credentialGroupKey(authRealm, groupID)
}

// inferCredentialGroupID maps username to user input credential group.
func inferCredentialGroupID(rt *Runtime, username string) string {
	if rt == nil || strings.TrimSpace(username) == "" {
		return ""
	}
	for _, g := range rt.UserCredentialGroups() {
		for _, a := range g.Accounts {
			if a.Username == username {
				return g.GroupID
			}
		}
	}
	return ""
}

func findVerifiedCredentialForGroup(rt *Runtime, authRealm, groupID string) *store.AuthCredential {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil
	}
	authRealm = NormalizeAuthRealm(authRealm)
	groupID = normalizeCredentialGroupID(groupID)
	if authRealm == "" || groupID == "" {
		return nil
	}
	creds, err := rt.Repo.ListAuthCredentials(rt.Session.ID)
	if err != nil {
		return nil
	}
	var best *store.AuthCredential
	for i := range creds {
		c := &creds[i]
		if !c.Verified || strings.TrimSpace(c.HeadersJSON) == "" {
			continue
		}
		if NormalizeAuthRealm(c.AuthRealm) != authRealm {
			continue
		}
		gid := strings.TrimSpace(c.CredentialGroupID)
		if gid == "" {
			gid = inferCredentialGroupID(rt, c.Username)
		}
		if normalizeCredentialGroupID(gid) != groupID {
			continue
		}
		if best == nil || c.ID > best.ID {
			best = c
		}
	}
	return best
}

func groupAlreadySatisfied(rt *Runtime, authRealm, groupID string) bool {
	return findVerifiedCredentialForGroup(rt, authRealm, groupID) != nil
}

// checkLoginPOSTBlocked returns non-empty feedback when login POST should not run.
func checkLoginPOSTBlocked(rt *Runtime, authRealm string, action *aicommon.Action) string {
	if rt == nil || action == nil {
		return ""
	}
	method := strings.ToUpper(strings.TrimSpace(action.GetString("method")))
	if method != "POST" && method != "PUT" {
		return ""
	}
	if !looksLikeLoginRequestURL(action.GetString("url")) {
		return ""
	}
	authRealm = NormalizeAuthRealm(authRealm)
	if authRealm == "" {
		return ""
	}
	username := extractUsernameFromLoginRequest(action.GetString("post-params"), action.GetString("body"))
	groupID := inferCredentialGroupID(rt, username)
	if groupID == "" {
		for _, gid := range CredentialGroupIDsForAuthRealm(authRealm) {
			if groupAlreadySatisfied(rt, authRealm, gid) {
				groupID = gid
				break
			}
		}
	}
	if groupID == "" {
		return ""
	}
	if cred := findVerifiedCredentialForGroup(rt, authRealm, groupID); cred != nil {
		return fmt.Sprintf(
			"login POST blocked: auth_realm=%q group=%q already has verified credential id=%d username=%q. "+
				"Use discovery_select_auth_credential + do_http_request with auth_credential_id=%d; do not login again with other accounts in this group.",
			authRealm, groupID, cred.ID, cred.Username, cred.ID,
		)
	}
	return ""
}

func validateSessionCookieHeaders(headersJSON, username string) error {
	if strings.TrimSpace(headersJSON) == "" {
		return fmt.Errorf("headers_json required for verified credential")
	}
	var m map[string]string
	if json.Unmarshal([]byte(headersJSON), &m) != nil {
		return fmt.Errorf("headers_json must be a JSON object")
	}
	cookie := strings.TrimSpace(m["Cookie"])
	if cookie == "" {
		return nil
	}
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		name, value, ok := splitCookiePair(part)
		if !ok {
			continue
		}
		if strings.EqualFold(name, "PUBLICCMS_ADMIN") && username != "" {
			if value == username {
				return fmt.Errorf("PUBLICCMS_ADMIN cookie value looks like username %q, not a session token", username)
			}
			if !strings.Contains(value, "_") && len(value) < 16 {
				return fmt.Errorf("PUBLICCMS_ADMIN cookie value %q is too short to be a session token", value)
			}
		}
	}
	return nil
}

func supersedeOlderCredentialsInGroup(rt *Runtime, authRealm, groupID string, keepID uint) {
	if rt == nil || rt.Repo == nil || rt.Session == nil || keepID == 0 {
		return
	}
	authRealm = NormalizeAuthRealm(authRealm)
	groupID = normalizeCredentialGroupID(groupID)
	creds, err := rt.Repo.ListAuthCredentials(rt.Session.ID)
	if err != nil {
		return
	}
	for i := range creds {
		c := &creds[i]
		if c.ID == keepID || !c.Verified {
			continue
		}
		if NormalizeAuthRealm(c.AuthRealm) != authRealm {
			continue
		}
		gid := strings.TrimSpace(c.CredentialGroupID)
		if gid == "" {
			gid = inferCredentialGroupID(rt, c.Username)
		}
		if normalizeCredentialGroupID(gid) != groupID {
			continue
		}
		c.Verified = false
		c.Notes = strings.TrimSpace(c.Notes + fmt.Sprintf("; superseded by credential id=%d", keepID))
		if err := rt.Repo.UpdateAuthCredential(c); err != nil {
			log.Warnf("ssa_api_discovery: supersede credential id=%d: %v", c.ID, err)
		}
	}
}

func markGroupSatisfied(loop *reactloops.ReActLoop, authRealm, groupID string, credID uint) {
	if loop == nil || credID == 0 {
		return
	}
	loop.Set(loopKeyGroupSatisfied(authRealm, groupID), fmt.Sprintf("%d", credID))
	loop.Set(loopKeySelectedAuthCredentialID, fmt.Sprintf("%d", credID))
}

func saveLoginCredentialFromProbe(rt *Runtime, loop *reactloops.ReActLoop, authRealm string, action *aicommon.Action, outcome *LoginProbeOutcome) (*store.AuthCredential, string) {
	if rt == nil || rt.Repo == nil || rt.Session == nil || outcome == nil || !outcome.Success {
		return nil, ""
	}
	authRealm = NormalizeAuthRealm(authRealm)
	if authRealm == "" {
		return nil, ""
	}
	if err := validateSessionCookieHeaders(outcome.HeadersJSON, outcome.Username); err != nil {
		return nil, "\nprogrammatic_auto_save rejected: " + err.Error()
	}
	groupID := inferCredentialGroupID(rt, outcome.Username)
	row := &store.AuthCredential{
		SessionID:         rt.Session.ID,
		AuthType:          "cookie_session",
		Username:          outcome.Username,
		HeadersJSON:       outcome.HeadersJSON,
		AuthRealm:         authRealm,
		CredentialGroupID: groupID,
		MountPrefix:       inferMountPrefixForAuthRealm(rt, authRealm),
		LoginPath:         outcome.LoginPath,
		Verified:          true,
		VerifyURL:         suggestVerifyURLFromLoginPath(outcome.LoginPath, inferMountPrefixForAuthRealm(rt, authRealm)),
		Notes:             outcome.Notes,
	}
	if row.LoginPath == "" {
		row.LoginPath = "/login"
	}
	now := time.Now().UTC()
	row.LastAcquiredAt = &now
	row.LastVerifiedAt = &now
	if action != nil {
		if ev, err := json.Marshal(map[string]string{
			"login_url":    action.GetString("url"),
			"method":       action.GetString("method"),
			"content_type": action.GetString("content-type"),
		}); err == nil {
			row.LoginEvidenceJSON = string(ev)
		}
	}
	SyncCredentialHeaderFields(row)
	if err := rt.Repo.CreateAuthCredential(row); err != nil {
		log.Warnf("ssa_api_discovery: auto-save login credential: %v", err)
		return nil, ""
	}
	if groupID != "" {
		supersedeOlderCredentialsInGroup(rt, authRealm, groupID, row.ID)
	}
	if loop != nil {
		markGroupSatisfied(loop, authRealm, groupID, row.ID)
	}
	if err := RefreshAuthEvidenceFromDB(rt); err != nil {
		log.Warnf("ssa_api_discovery: refresh auth_evidence after auto-save: %v", err)
	}
	msg := fmt.Sprintf("\n\nprogrammatic_auto_save: created auth_credential id=%d realm=%s group=%s verified=true (302+Set-Cookie). "+
		"Use discovery_select_auth_credential credential_id=%d then auth_credential_id=%d for probes; do not login same group again.",
		row.ID, row.AuthRealm, groupID, row.ID, row.ID)
	return row, msg
}

func enforceVerifiedCredentialFromProbe(row *store.AuthCredential, loop *reactloops.ReActLoop, rt *Runtime) (blocked bool, msg string) {
	if row == nil || !row.Verified {
		return false, ""
	}
	outcome := loadLoginProbeOutcome(loop)
	if outcome != nil && outcome.Success {
		row.HeadersJSON = outcome.HeadersJSON
		if row.Username == "" {
			row.Username = outcome.Username
		}
		if row.LoginPath == "" {
			row.LoginPath = outcome.LoginPath
		}
		SyncCredentialHeaderFields(row)
		return false, "\nprogrammatic_login_probe: headers_json taken from last login POST (302+Set-Cookie)"
	}
	if strings.TrimSpace(row.HeadersJSON) != "" {
		if err := validateSessionCookieHeaders(row.HeadersJSON, row.Username); err != nil {
			row.Verified = false
			return true, "upsert rejected: " + err.Error() + " — headers must come from successful login POST"
		}
	}
	if row.Verified && strings.TrimSpace(row.HeadersJSON) == "" {
		row.Verified = false
		return true, "upsert rejected: verified=true requires headers from login probe; perform login POST first"
	}
	return false, ""
}
