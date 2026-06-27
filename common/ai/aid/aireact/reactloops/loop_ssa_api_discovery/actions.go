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

func mustRT(loop *reactloops.ReActLoop, op *reactloops.LoopActionHandlerOperator) (*Runtime, *store.DiscoverySession, bool) {
	rt := getRuntime(loop)
	if rt == nil || rt.Repo == nil {
		op.Feedback("discovery store not initialized")
		op.Continue()
		return nil, nil, false
	}
	uuid := strings.TrimSpace(loop.Get("discovery_session_uuid"))
	if uuid == "" {
		op.Feedback("missing discovery_session_uuid")
		op.Continue()
		return nil, nil, false
	}
	sess, err := rt.Repo.GetSessionByUUID(uuid)
	if err != nil {
		op.Feedback(fmt.Sprintf("load session: %v", err))
		op.Continue()
		return nil, nil, false
	}
	rt.Session = sess
	return rt, sess, true
}

func buildDiscoveryGetStatus() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_get_status",
		"Read-only: session, SSA summary, target (raw, host, port, scheme, target_base_url, target_url_hints), reachability probe, SQLite path, entity counts, endpoint_harvest / api_preanalysis / api_spec_import / api_base_calibration summaries.",
		[]aitool.ToolOption{},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error { return nil },
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			payload, err := buildDiscoveryStatusPayload(rt, sess)
			if err != nil {
				op.Feedback(fmt.Sprintf("status: %v", err))
				op.Continue()
				return
			}
			raw, _ := json.MarshalIndent(payload, "", "  ")
			feedback := string(raw)
			if strings.TrimSpace(loop.Get("discovery_phase")) == "phase1_verify" {
				refreshPhase1VerifyLoopVars(loop, rt)
				if pending := strings.TrimSpace(loop.Get("phase1_pending_candidates")); pending != "" {
					feedback += "\n\nphase1_pending_candidates: " + pending
				}
				if err := verifyPhase1ApiVerificationGate(rt); err != nil {
					feedback += "\n\nphase1_gate: " + err.Error() + " — finish pending routes via discovery_probe_api_candidate then discovery_upsert_verified_http_api / discovery_mark_api_rejected; do not re-probe the same route."
				} else {
					feedback += "\n\nphase1_gate: ok — you may call directly_answer to finish Phase1C."
				}
			}
			op.Feedback(feedback)
			counts, _ := payload["counts"].(map[string]int)
			loop.Set("discovery_counts_line", fmt.Sprintf("components=%d configs=%d deps=%d endpoints=%d security=%d biz=%d verified_http_apis=%d/%d legacy_verified=%d sf=%d vuln_ver=%d",
				counts["components"], counts["config_artifacts"], counts["dependencies"],
				counts["http_endpoints"], counts["security_mechanisms"], counts["business_capabilities"],
				counts["verified_http_apis_verified"], counts["verified_http_apis_total"],
				counts["verified_endpoints"], counts["syntaxflow_findings"], counts["vuln_verifications"]))
			log.Infof("ssa_api_discovery: discovery_get_status session=%s phase=%s", sess.UUID, sess.Phase)
			op.Continue()
		},
	)
}

func buildUpsertComponent() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_upsert_component",
		"Create or update an architecture component row. Omit id to insert.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("id", aitool.WithParam_Description("Existing row id for update; omit or 0 for create")),
			aitool.WithStringParam("name", aitool.WithParam_Required(true)),
			aitool.WithStringParam("kind", aitool.WithParam_Description("service|lib|adapter|batch|unknown")),
			aitool.WithStringParam("summary", aitool.WithParam_Description("Short summary")),
			aitool.WithStringParam("path_hints_json", aitool.WithParam_Description(`JSON array of paths, e.g. ["src/main/api"]`)),
			aitool.WithIntegerParam("confidence", aitool.WithParam_Description("0-100")),
			aitool.WithStringParam("source", aitool.WithParam_Description("ai or manual"), aitool.WithParam_Default("ai")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			if strings.TrimSpace(action.GetString("name")) == "" {
				return utils.Error("name required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			id := action.GetInt("id")
			row := &store.ArchitectureComponent{
				SessionID:     sess.ID,
				Name:          action.GetString("name"),
				Kind:          action.GetString("kind"),
				Summary:       action.GetString("summary"),
				PathHintsJSON: action.GetString("path_hints_json"),
				Confidence:    action.GetInt("confidence"),
				Source:        action.GetString("source"),
			}
			if row.Source == "" {
				row.Source = "ai"
			}
			if id > 0 {
				existing, err := rt.Repo.GetComponent(sess.ID, uint(id))
				if err != nil {
					op.Feedback(err.Error())
					op.Continue()
					return
				}
				row.ID = existing.ID
				row.CreatedAt = existing.CreatedAt
				if err := rt.Repo.UpdateComponent(row); err != nil {
					op.Feedback(err.Error())
					op.Continue()
					return
				}
				op.Feedback(fmt.Sprintf("updated component id=%d", row.ID))
			} else {
				if err := rt.Repo.CreateComponent(row); err != nil {
					op.Feedback(err.Error())
					op.Continue()
					return
				}
				op.Feedback(fmt.Sprintf("created component id=%d", row.ID))
			}
			op.Continue()
		},
	)
}

func buildUpsertConfigArtifact() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_upsert_config_artifact",
		"Create or update a config artifact row. Do not store secret values—only paths and key kinds.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("id"),
			aitool.WithStringParam("rel_path", aitool.WithParam_Required(true)),
			aitool.WithStringParam("format"),
			aitool.WithStringParam("summary"),
			aitool.WithStringParam("sensitive_key_kinds_json", aitool.WithParam_Description(`JSON array of kinds, e.g. ["jwt_secret"]`)),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			if strings.TrimSpace(action.GetString("rel_path")) == "" {
				return utils.Error("rel_path required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			id := action.GetInt("id")
			row := &store.ConfigArtifact{
				SessionID:             sess.ID,
				RelPath:               action.GetString("rel_path"),
				Format:                action.GetString("format"),
				Summary:               action.GetString("summary"),
				SensitiveKeyKindsJSON: action.GetString("sensitive_key_kinds_json"),
			}
			if id > 0 {
				existing, err := rt.Repo.GetConfigArtifact(sess.ID, uint(id))
				if err != nil {
					op.Feedback(err.Error())
					op.Continue()
					return
				}
				row.ID = existing.ID
				row.CreatedAt = existing.CreatedAt
				if err := rt.Repo.UpdateConfigArtifact(row); err != nil {
					op.Feedback(err.Error())
					op.Continue()
					return
				}
				op.Feedback(fmt.Sprintf("updated config id=%d", row.ID))
			} else {
				if err := rt.Repo.CreateConfigArtifact(row); err != nil {
					op.Feedback(err.Error())
					op.Continue()
					return
				}
				op.Feedback(fmt.Sprintf("created config id=%d", row.ID))
			}
			op.Continue()
		},
	)
}

// dependency batch: JSON array of {"name","version","ecosystem"}
func buildDependencyBatch() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_dependency_batch",
		"Replace all dependency rows with a JSON array: [{\"name\":\"\",\"version\":\"\",\"ecosystem\":\"maven|npm|go|pip\"},...].",
		[]aitool.ToolOption{
			aitool.WithStringParam("deps_json", aitool.WithParam_Required(true)),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			if strings.TrimSpace(action.GetString("deps_json")) == "" {
				return utils.Error("deps_json required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			var items []struct {
				Name      string `json:"name"`
				Version   string `json:"version"`
				Ecosystem string `json:"ecosystem"`
			}
			if err := json.Unmarshal([]byte(action.GetString("deps_json")), &items); err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			if err := rt.Repo.DeleteDependenciesBySession(sess.ID); err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			for _, it := range items {
				if it.Name == "" {
					continue
				}
				_ = rt.Repo.CreateDependency(&store.DependencyRef{
					SessionID: sess.ID, Name: it.Name, Version: it.Version, Ecosystem: it.Ecosystem,
				})
			}
			op.Feedback(fmt.Sprintf("replaced %d dependencies", len(items)))
			op.Continue()
		},
	)
}

func buildUpsertHttpEndpoint() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_upsert_http_endpoint",
		"Create or update an HTTP endpoint row.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("id"),
			aitool.WithStringParam("method", aitool.WithParam_Required(true)),
			aitool.WithStringParam("path_pattern", aitool.WithParam_Required(true)),
			aitool.WithStringParam("handler_class"),
			aitool.WithStringParam("handler_method"),
			aitool.WithStringParam("authz_hint"),
			aitool.WithStringParam("source", aitool.WithParam_Default("ai")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			if action.GetString("method") == "" || action.GetString("path_pattern") == "" {
				return utils.Error("method and path_pattern required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			id := action.GetInt("id")
			row := &store.HttpEndpoint{
				SessionID:     sess.ID,
				Method:        action.GetString("method"),
				PathPattern:   action.GetString("path_pattern"),
				HandlerClass:  action.GetString("handler_class"),
				HandlerMethod: action.GetString("handler_method"),
				AuthzHint:     action.GetString("authz_hint"),
				Source:        action.GetString("source"),
			}
			if row.Source == "" {
				row.Source = "ai"
			}
			if id > 0 {
				existing, err := rt.Repo.GetHttpEndpoint(sess.ID, uint(id))
				if err != nil {
					op.Feedback(err.Error())
					op.Continue()
					return
				}
				row.ID = existing.ID
				row.CreatedAt = existing.CreatedAt
				row.Status = existing.Status
				if reason := NormalizeAndValidateEndpoint(row); reason != "" {
					op.Feedback(fmt.Sprintf("rejected: %s", reason))
					op.Continue()
					return
				}
				if err := rt.Repo.UpdateHttpEndpoint(row); err != nil {
					op.Feedback(err.Error())
					op.Continue()
					return
				}
				op.Feedback(fmt.Sprintf("updated endpoint id=%d", row.ID))
			} else {
				res, err := EndpointInsertionGateway(rt, row)
				if err != nil {
					op.Feedback(err.Error())
					op.Continue()
					return
				}
				if res.Status == "rejected_at_gate" {
					op.Feedback(fmt.Sprintf("rejected at gate: %s — please fix method/path and retry", res.Reason))
					op.Continue()
					return
				}
				if res.Merged {
					op.Feedback(fmt.Sprintf("merged with existing endpoint id=%d", res.EndpointID))
				} else {
					op.Feedback(fmt.Sprintf("created endpoint id=%d (status=pending_validation)", res.EndpointID))
				}
			}
			op.Continue()
		},
	)
}
