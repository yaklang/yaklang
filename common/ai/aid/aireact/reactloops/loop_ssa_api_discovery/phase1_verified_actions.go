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

// ProbeResult is the JSON payload from finalize_probe_result / discovery_probe_api_candidate.
type ProbeResult struct {
	Verified          bool              `json:"verified"`
	Confidence        int               `json:"confidence"`
	Method            string            `json:"method"`
	PathPattern       string            `json:"path_pattern"`
	FullSampleURL     string            `json:"full_sample_url"`
	EffectiveBase     string            `json:"effective_base"`
	ProbeStatusCode   int               `json:"probe_status_code"`
	ContentType       string            `json:"content_type"`
	ResponseExcerpt   string            `json:"response_excerpt"`
	QueryParamsJSON   string            `json:"query_params_json"`
	BodyHintJSON      string            `json:"body_hint_json"`
	AuthHeadersJSON   string            `json:"auth_headers_json"`
	VerdictReason     string            `json:"verdict_reason"`
	ProbeAttempts     []json.RawMessage `json:"probe_attempts"`
	RejectReason      string            `json:"reject_reason"`
	HandlerFile       string            `json:"handler_file"`
	HandlerSymbol     string            `json:"handler_symbol"`
	CodeSnippet       string            `json:"code_snippet"`
	URLSpace          string            `json:"url_space"`
	AuthAccess        string            `json:"auth_access,omitempty"`
	Source            string            `json:"source"`
}

func phase1VerifiedActionOptions(r aicommon.AIInvokeRuntime) []reactloops.ReActLoopOption {
	return []reactloops.ReActLoopOption{
		buildDiscoveryUpsertVerifiedHttpApi(),
		buildDiscoveryMarkApiRejected(),
		buildDiscoveryProbeApiCandidate(r),
		buildDiscoveryLinkHandlerCode(),
	}
}

func buildDiscoveryUpsertVerifiedHttpApi() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_upsert_verified_http_api",
		"Upsert a verified_http_apis row from probe_result JSON or explicit fields.",
		[]aitool.ToolOption{
			aitool.WithStringParam("probe_result_json", aitool.WithParam_Description("JSON from finalize_probe_result")),
			aitool.WithStringParam("method"),
			aitool.WithStringParam("path_pattern"),
			aitool.WithBoolParam("verified"),
			aitool.WithIntegerParam("confidence"),
			aitool.WithStringParam("source", aitool.WithParam_Default("ai_probe")),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			pr, err := probeResultFromAction(action)
			if err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			if !sess.TargetReachable && pr.Verified {
				op.Feedback("target_reachable=false: cannot set verified=true")
				op.Continue()
				return
			}
			attemptsJSON, _ := json.Marshal(pr.ProbeAttempts)
			row := &store.VerifiedHttpApi{
				SessionID:         sess.ID,
				Method:            pr.Method,
				PathPattern:       pr.PathPattern,
				FullSampleURL:     pr.FullSampleURL,
				EffectiveBase:     pr.EffectiveBase,
				URLSpace:          pr.URLSpace,
				QueryParamsJSON:   pr.QueryParamsJSON,
				BodyHintJSON:      pr.BodyHintJSON,
				AuthHeadersJSON:   pr.AuthHeadersJSON,
				HandlerFile:       pr.HandlerFile,
				HandlerSymbol:     pr.HandlerSymbol,
				CodeSnippet:       pr.CodeSnippet,
				ProbeStatusCode:   pr.ProbeStatusCode,
				ContentType:       pr.ContentType,
				ResponseExcerpt:   utils.ShrinkString(pr.ResponseExcerpt, 8000),
				ProbeAttemptsJSON: string(attemptsJSON),
				VerdictReason:     pr.VerdictReason,
				Verified:          pr.Verified,
				Confidence:        pr.Confidence,
				Source:            pr.Source,
				RejectReason:      pr.RejectReason,
			}
			if row.Source == "" {
				row.Source = "ai_probe"
			}
			if row.Method == "" || row.PathPattern == "" {
				op.Feedback("method and path_pattern required")
				op.Continue()
				return
			}
			if err := rt.Repo.UpsertVerifiedHttpApi(row); err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			if err := ApplyVerifiedHttpApiProbeBackfill(rt, row); err != nil {
				log.Warnf("ssa_api_discovery: probe backfill after upsert: %v", err)
			}
			op.Feedback(fmt.Sprintf("verified_http_api upserted id=%d verified=%v", row.ID, row.Verified))
			op.Continue()
		},
	)
}

func buildDiscoveryMarkApiRejected() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_mark_api_rejected",
		"Record a rejected API candidate in verified_http_apis (verified=false).",
		[]aitool.ToolOption{
			aitool.WithStringParam("method", aitool.WithParam_Required(true)),
			aitool.WithStringParam("path_pattern", aitool.WithParam_Required(true)),
			aitool.WithStringParam("reject_reason", aitool.WithParam_Required(true)),
			aitool.WithStringParam("probe_attempts_json"),
			aitool.WithStringParam("handler_file"),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			row := &store.VerifiedHttpApi{
				SessionID:         sess.ID,
				Method:            action.GetString("method"),
				PathPattern:       action.GetString("path_pattern"),
				Verified:          false,
				RejectReason:      action.GetString("reject_reason"),
				ProbeAttemptsJSON: action.GetString("probe_attempts_json"),
				HandlerFile:       action.GetString("handler_file"),
				Source:            "rejected",
			}
			if err := rt.Repo.UpsertVerifiedHttpApi(row); err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			op.Feedback(fmt.Sprintf("rejected api recorded id=%d", row.ID))
			op.Continue()
		},
	)
}

func buildDiscoveryLinkHandlerCode() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_link_handler_code",
		"Attach handler code snippet metadata (stored in loop for next probe).",
		[]aitool.ToolOption{
			aitool.WithStringParam("handler_file"),
			aitool.WithStringParam("handler_symbol"),
			aitool.WithStringParam("code_snippet"),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			loop.Set("phase1_handler_file", action.GetString("handler_file"))
			loop.Set("phase1_handler_symbol", action.GetString("handler_symbol"))
			loop.Set("phase1_code_snippet", utils.ShrinkString(action.GetString("code_snippet"), 12000))
			op.Feedback("handler code linked for current candidate")
			op.Continue()
		},
	)
}

func probeResultFromAction(action *aicommon.Action) (*ProbeResult, error) {
	raw := strings.TrimSpace(action.GetString("probe_result_json"))
	if raw != "" {
		var pr ProbeResult
		if err := json.Unmarshal([]byte(raw), &pr); err != nil {
			return nil, err
		}
		return &pr, nil
	}
	pr := &ProbeResult{
		Method:          action.GetString("method"),
		PathPattern:     action.GetString("path_pattern"),
		Verified:        action.GetBool("verified"),
		Confidence:      action.GetInt("confidence"),
		Source:          action.GetString("source"),
		FullSampleURL:   action.GetString("full_sample_url"),
		VerdictReason:   action.GetString("verdict_reason"),
		ResponseExcerpt: action.GetString("response_excerpt"),
	}
	if pr.Method == "" || pr.PathPattern == "" {
		return nil, utils.Error("probe_result_json or method+path_pattern required")
	}
	return pr, nil
}

// UpsertVerifiedHttpApiFromProbeResult persists probe verdict into verified_http_apis.
func UpsertVerifiedHttpApiFromProbeResult(rt *Runtime, pr *ProbeResult) (*store.VerifiedHttpApi, error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil || pr == nil {
		return nil, utils.Error("nil runtime or probe result")
	}
	if !rt.Session.TargetReachable && pr.Verified {
		return nil, utils.Error("target_reachable=false: cannot set verified=true")
	}
	if pr.Method == "" || pr.PathPattern == "" {
		return nil, utils.Error("method and path_pattern required")
	}
	attemptsJSON, _ := json.Marshal(pr.ProbeAttempts)
	row := &store.VerifiedHttpApi{
		SessionID:         rt.Session.ID,
		Method:            pr.Method,
		PathPattern:       pr.PathPattern,
		FullSampleURL:     pr.FullSampleURL,
		EffectiveBase:     pr.EffectiveBase,
		URLSpace:          pr.URLSpace,
		QueryParamsJSON:   pr.QueryParamsJSON,
		BodyHintJSON:      pr.BodyHintJSON,
		AuthHeadersJSON:   pr.AuthHeadersJSON,
		HandlerFile:       pr.HandlerFile,
		HandlerSymbol:     pr.HandlerSymbol,
		CodeSnippet:       pr.CodeSnippet,
		ProbeStatusCode:   pr.ProbeStatusCode,
		ContentType:       pr.ContentType,
		ResponseExcerpt:   utils.ShrinkString(pr.ResponseExcerpt, 8000),
		ProbeAttemptsJSON: string(attemptsJSON),
		VerdictReason:     pr.VerdictReason,
		Verified:          pr.Verified,
		Confidence:        pr.Confidence,
		Source:            pr.Source,
		RejectReason:      pr.RejectReason,
	}
	if row.Source == "" {
		row.Source = "ai_probe"
	}
	if err := rt.Repo.UpsertVerifiedHttpApi(row); err != nil {
		return nil, err
	}
	if err := ApplyVerifiedHttpApiProbeBackfill(rt, row); err != nil {
		log.Warnf("ssa_api_discovery: probe backfill: %v", err)
	}
	return row, nil
}
