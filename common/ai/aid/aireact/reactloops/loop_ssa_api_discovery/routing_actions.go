package loop_ssa_api_discovery

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func buildRoutingSaveDraft() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"routing_save_draft",
		"Cache intermediate routing profile JSON in loop state (key routing_profile_draft). Does not persist to SQLite.",
		[]aitool.ToolOption{
			aitool.WithStringParam("profile_json", aitool.WithParam_Required(true), aitool.WithParam_Description("Full routing profile v1 JSON object as string")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			if strings.TrimSpace(action.GetString("profile_json")) == "" {
				return utils.Error("profile_json required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			loop.Set("routing_profile_draft", action.GetString("profile_json"))
			op.Feedback("draft saved to loop var routing_profile_draft")
			op.Continue()
		},
	)
}

func buildRoutingCommitProfile() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"routing_commit_profile",
		"Validate routing profile v1 JSON, write discovery_sessions.routing_profile_json, workdir/ssa_discovery/routing_profile.json, and timeline event. Sets routing_profile_committed.",
		[]aitool.ToolOption{
			aitool.WithStringParam("profile_json", aitool.WithParam_Required(false), aitool.WithParam_Description("JSON string; if empty uses loop var routing_profile_draft")),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, _, ok := mustRT(loop, op)
			if !ok {
				return
			}
			raw := strings.TrimSpace(action.GetString("profile_json"))
			if raw == "" {
				raw = strings.TrimSpace(loop.Get("routing_profile_draft"))
			}
			if raw == "" {
				op.Feedback("profile_json empty and no routing_profile_draft; provide profile_json in action")
				op.Continue()
				return
			}
			p, err := parseRoutingProfileFromAgentJSON(raw, rt)
			if err != nil {
				op.Feedback(fmt.Sprintf("invalid routing profile: %v", err))
				op.Continue()
				return
			}
			if err := ValidateRoutingProfileForCommit(p); err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			canonical, err := CanonicalRoutingProfileJSON(p)
			if err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			sess := rt.Session
			if sess == nil {
				op.Feedback("nil session")
				op.Continue()
				return
			}
			if err := rt.Repo.UpdateSessionFields(sess.UUID, map[string]interface{}{
				"routing_profile_json": canonical,
			}); err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			if err := WriteRoutingProfileFile(rt.WorkDir, canonical); err != nil {
				log.Warnf("ssa_api_discovery: write routing_profile.json: %v", err)
			}
			_ = rt.Repo.AppendEvent(sess.ID, "info", "routing_commit_profile", string(utils.Jsonify(map[string]any{
				"validation_status": p.ValidationStatus,
				"effective_bases":   p.EffectiveBases,
				"url_spaces":        len(p.URLSpaces),
			})))
			reload2, err := rt.Repo.GetSessionByUUID(sess.UUID)
			if err == nil && reload2 != nil {
				rt.Session = reload2
			} else {
				sess.RoutingProfileJSON = canonical
			}
			loop.Set("routing_profile_committed", "1")
			op.Feedback("routing profile committed; session and routing_profile.json updated")
			if inv := loop.GetInvoker(); inv != nil {
				inv.AddToTimeline("[ssa_routing]", fmt.Sprintf("routing_profile committed status=%s", p.ValidationStatus))
			}
			op.Continue()
		},
	)
}
