package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func buildUpsertAuthCredentialAction() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_upsert_auth_credential",
		"Create or update an auth credential. headers_json is the canonical source for all auth headers. Merge all Set-Cookie pairs into one Cookie value when saving login response.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("id", aitool.WithParam_Description("existing row id; 0=create")),
			aitool.WithStringParam("auth_type", aitool.WithParam_Required(true), aitool.WithParam_Description("cookie_session|jwt_bearer|basic_auth|api_key")),
			aitool.WithStringParam("username"),
			aitool.WithStringParam("token_value", aitool.WithParam_Description("Cookie / JWT / API Key value")),
			aitool.WithStringParam("header_name", aitool.WithParam_Description("e.g. Authorization, Cookie, X-API-Key")),
			aitool.WithStringParam("header_value", aitool.WithParam_Description("full header value, e.g. Bearer xxx")),
			aitool.WithStringParam("headers_json", aitool.WithParam_Description("JSON map of all auth headers: {\"Cookie\":\"...\",\"X-CSRF-Token\":\"...\"}")),
			aitool.WithStringParam("url_space", aitool.WithParam_Description("routing_profile url_spaces.id for this credential (e.g. stage_0_admin, public)")),
			aitool.WithStringParam("auth_realm", aitool.WithParam_Description("admin|web|api|oauth|member — which auth surface this credential belongs to")),
			aitool.WithStringParam("credential_group_id", aitool.WithParam_Description("user credential group: admin|user|web|api")),
			aitool.WithStringParam("mount_prefix", aitool.WithParam_Description("URL mount prefix for this realm, e.g. /admin or /api")),
			aitool.WithStringParam("login_path", aitool.WithParam_Description("login endpoint path used to obtain this credential")),
			aitool.WithBoolParam("verified"),
			aitool.WithStringParam("verify_url"),
			aitool.WithStringParam("notes"),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			at := strings.ToLower(strings.TrimSpace(action.GetString("auth_type")))
			valid := map[string]bool{"cookie_session": true, "jwt_bearer": true, "basic_auth": true, "api_key": true}
			if !valid[at] {
				return utils.Errorf("invalid auth_type %q", action.GetString("auth_type"))
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			row := &store.AuthCredential{
				SessionID:   sess.ID,
				AuthType:    strings.ToLower(strings.TrimSpace(action.GetString("auth_type"))),
				Username:    action.GetString("username"),
				TokenValue:  action.GetString("token_value"),
				HeaderName:  action.GetString("header_name"),
				HeaderValue: action.GetString("header_value"),
				HeadersJSON: action.GetString("headers_json"),
				URLSpace:    strings.TrimSpace(action.GetString("url_space")),
				AuthRealm:         NormalizeAuthRealm(action.GetString("auth_realm")),
				CredentialGroupID: normalizeCredentialGroupID(action.GetString("credential_group_id")),
				MountPrefix:       normURLPath(action.GetString("mount_prefix")),
				LoginPath:   strings.TrimSpace(action.GetString("login_path")),
				Verified:    action.GetBool("verified", false),
				VerifyURL:   action.GetString("verify_url"),
				Notes:       action.GetString("notes"),
			}
			SyncCredentialHeaderFields(row)
			if row.CredentialGroupID == "" {
				row.CredentialGroupID = inferCredentialGroupID(rt, row.Username)
			}
			probeMsg := applyStoredLoginProbeToUpsert(loop, row)
			if probeMsg != "" {
				log.Infof("ssa_api_discovery: upsert login probe upgrade realm=%s verified=%v", row.AuthRealm, row.Verified)
			}
			blocked, enforceMsg := enforceVerifiedCredentialFromProbe(row, loop, rt)
			if blocked {
				op.Feedback(strings.TrimSpace(enforceMsg))
				op.Continue()
				return
			}
			if enforceMsg != "" {
				probeMsg += enforceMsg
			}

			id := action.GetInt("id")
			if id > 0 {
				existing, err := rt.Repo.GetAuthCredential(sess.ID, uint(id))
				if err != nil {
					op.Feedback(err.Error())
					op.Continue()
					return
				}
				row.ID = existing.ID
				row.CreatedAt = existing.CreatedAt
				row.ReacquireCount = existing.ReacquireCount
				row.AcquireRecipeID = existing.AcquireRecipeID
				if err := rt.Repo.UpdateAuthCredential(row); err != nil {
					op.Feedback(err.Error())
					op.Continue()
					return
				}
			} else {
				if err := rt.Repo.CreateAuthCredential(row); err != nil {
					op.Feedback(err.Error())
					op.Continue()
					return
				}
			}
			var fb string
			if id > 0 {
				fb = fmt.Sprintf("updated auth_credential id=%d headers_json=%v", row.ID, row.HeadersJSON != "")
			} else {
				fb = fmt.Sprintf("created auth_credential id=%d type=%s realm=%s url_space=%s verified=%v headers_json=%v",
					row.ID, row.AuthType, row.AuthRealm, row.URLSpace, row.Verified, row.HeadersJSON != "")
			}
			if row.Verified && strings.TrimSpace(row.HeadersJSON) != "" {
				if row.CredentialGroupID != "" {
					supersedeOlderCredentialsInGroup(rt, row.AuthRealm, row.CredentialGroupID, row.ID)
					markGroupSatisfied(loop, row.AuthRealm, row.CredentialGroupID, row.ID)
				}
				if err := RefreshAuthEvidenceFromDB(rt); err != nil {
					log.Warnf("ssa_api_discovery: refresh auth_evidence after upsert: %v", err)
				}
			}
			if hint := postLoginVerifyURLHint(row.LoginPath, row.VerifyURL, row.MountPrefix); hint != "" {
				fb += "\n\n" + hint
			}
			fb += probeMsg
			op.Feedback(fb)
			op.Continue()
		},
	)
}

func buildListAuthCredentialsAction() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_list_auth_credentials",
		"List auth credentials for this session (redacted: no Cookie/header values). Optional auth_realm filter.",
		[]aitool.ToolOption{
			aitool.WithStringParam("auth_realm", aitool.WithParam_Description("optional filter: admin|web|api|oauth|member")),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			rows, err := rt.Repo.ListAuthCredentials(sess.ID)
			if err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			filterRealm := NormalizeAuthRealm(action.GetString("auth_realm"))
			out := make([]AuthCredentialSummary, 0, len(rows))
			for i := range rows {
				c := &rows[i]
				if filterRealm != "" && NormalizeAuthRealm(c.AuthRealm) != filterRealm {
					continue
				}
				out = append(out, credentialToSummary(c))
			}
			b, _ := json.MarshalIndent(out, "", "  ")
			op.Feedback(string(b) + "\n\nUse discovery_select_auth_credential to pick an id; do NOT copy headers manually.")
			op.Continue()
		},
	)
}

func buildSelectAuthCredentialAction() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_select_auth_credential",
		"Select a verified auth credential for subsequent HTTP probes. Sets selected_auth_credential_id on the loop.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("credential_id", aitool.WithParam_Required(true)),
			aitool.WithStringParam("auth_realm", aitool.WithParam_Description("optional realm check: must match credential auth_realm")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			if action.GetInt("credential_id") <= 0 {
				return utils.Error("credential_id required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			cid := uint(action.GetInt("credential_id"))
			cred, err := rt.Repo.GetAuthCredential(sess.ID, cid)
			if err != nil {
				op.Feedback(fmt.Sprintf("credential not found: %v", err))
				op.Continue()
				return
			}
			if !cred.Verified || strings.TrimSpace(cred.HeadersJSON) == "" {
				op.Feedback(fmt.Sprintf("credential id=%d is not verified or has empty headers_json; login first or pick another id", cid))
				op.Continue()
				return
			}
			wantRealm := NormalizeAuthRealm(action.GetString("auth_realm"))
			if wantRealm != "" && NormalizeAuthRealm(cred.AuthRealm) != wantRealm {
				op.Feedback(fmt.Sprintf("credential id=%d auth_realm=%q does not match requested %q", cid, cred.AuthRealm, wantRealm))
				op.Continue()
				return
			}
			groupID := strings.TrimSpace(cred.CredentialGroupID)
			if groupID == "" {
				groupID = inferCredentialGroupID(rt, cred.Username)
			}
			markGroupSatisfied(loop, cred.AuthRealm, groupID, cred.ID)
			op.Feedback(fmt.Sprintf(
				"selected auth_credential id=%d username=%q realm=%s group=%s. "+
					"Use do_http_request with auth_credential_id=%d only — do NOT pass headers/Cookie manually.",
				cid, cred.Username, cred.AuthRealm, groupID, cid,
			))
			op.Continue()
		},
	)
}

func buildVerifyAuthCredentialAction() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_verify_auth_credential",
		"Verify an existing auth credential by sending a request to a protected endpoint. Marks verified=true on success.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("credential_id", aitool.WithParam_Required(true)),
			aitool.WithStringParam("verify_url", aitool.WithParam_Required(true), aitool.WithParam_Description("URL that requires auth; expects non-401/403 on success")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			if action.GetInt("credential_id") <= 0 {
				return utils.Error("credential_id required")
			}
			if strings.TrimSpace(action.GetString("verify_url")) == "" {
				return utils.Error("verify_url required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			cid := uint(action.GetInt("credential_id"))
			cred, err := rt.Repo.GetAuthCredential(sess.ID, cid)
			if err != nil {
				op.Feedback(fmt.Sprintf("credential not found: %v", err))
				op.Continue()
				return
			}

			verifyURL := strings.TrimSpace(action.GetString("verify_url"))

			invoker := loop.GetInvoker()
			ctx := loop.GetConfig().GetContext()
			task := loop.GetCurrentTask()
			if task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}

			SyncCredentialHeaderFields(cred)
			params := aitool.InvokeParams{
				"url": verifyURL,
			}
			if cred.HeadersText != "" {
				params["headers"] = cred.HeadersText
			} else if cred.HeaderName != "" && cred.HeaderValue != "" {
				params["headers"] = fmt.Sprintf("%s: %s", cred.HeaderName, cred.HeaderValue)
			}

			result, _, rerr := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, "do_http_request", params)
			if rerr != nil {
				op.Feedback(fmt.Sprintf("verify request failed: %v", rerr))
				op.Continue()
				return
			}

			content := toolResultTextContent(result)

			verified := !strings.Contains(content, "401") && !strings.Contains(content, "403 Forbidden")
			cred.Verified = verified
			cred.VerifyURL = verifyURL
			if err := rt.Repo.UpdateAuthCredential(cred); err != nil {
				log.Warnf("update credential verify: %v", err)
			}

			op.Feedback(fmt.Sprintf("credential id=%d verified=%v response_preview=%s", cid, verified, utils.ShrinkString(content, 2000)))
			op.Continue()
		},
	)
}
