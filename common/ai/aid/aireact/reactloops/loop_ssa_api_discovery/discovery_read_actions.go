package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// ssaDiscoveryReportDataReadHint 报告子循环读取会话库时的统一指引（勿用 bash/sqlite3/read_file 读二进制 DB）。
const ssaDiscoveryReportDataReadHint = `
## 读取会话 SQLite（统一入口，禁止绕行）
- **只读**：使用 **discovery_read_session_data**（按 entity 拉取表数据）或 **discovery_get_status**（会话摘要与计数）。
- **禁止**：对 session.sqlite3 使用 bash、sqlite3 命令行、python 手写 SQL，或用 read_file 读取二进制库文件。
- 全量 JSON 快照：entity=**snapshot**（refresh_snapshot=true 时先刷新 discovery_snapshot.json）。
- 漏洞验证记录：entity=**vuln_verifications**（列名以 entity=**schema** 为准，勿臆造 http_endpoint_id 等列）。
`

func bindDiscoveryRuntimeInLoop(loop *reactloops.ReActLoop, rt *Runtime) {
	if loop == nil || rt == nil || rt.Session == nil {
		return
	}
	setRuntime(loop, rt)
	loop.Set("discovery_session_uuid", rt.Session.UUID)
	loop.Set("discovery_sqlite_path", rt.SQLitePath)
}

// discoveryReportReadActionOptions registers read-only discovery DB actions on report_generating sub-loops.
func discoveryReportReadActionOptions(rt *Runtime) []reactloops.ReActLoopOption {
	if rt == nil || rt.Session == nil {
		return nil
	}
	return []reactloops.ReActLoopOption{
		buildDiscoveryReadSessionData(),
		buildDiscoveryGetStatus(),
	}
}

func buildDiscoveryReadSessionData() reactloops.ReActLoopOption {
	entityHelp := strings.Join(store.AllSessionReadEntities(), "|")
	return reactloops.WithRegisterLoopAction(
		"discovery_read_session_data",
		"Read-only: load structured rows from the current discovery session SQLite (unified DB read entry). "+
			"Do not use bash/sqlite3/read_file on session.sqlite3. "+
			"entity: "+entityHelp+". "+
			"Optional limit (default 200, max 2000) truncates large arrays. "+
			"In feature-verify sub-loops, http_endpoints/verified_http_apis/verified_endpoints are auto-filtered to the current feature package_patterns. "+
			"entity=snapshot: refresh_snapshot=true rewrites discovery_snapshot.json first; include_body=true embeds file excerpt.",
		[]aitool.ToolOption{
			aitool.WithStringParam("entity",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Entity/table bundle to read: "+entityHelp),
			),
			aitool.WithIntegerParam("limit", aitool.WithParam_Description("Max rows for list entities (default 200, max 2000)")),
			aitool.WithBoolParam("refresh_snapshot", aitool.WithParam_Description("When entity=snapshot, export JSON before read")),
			aitool.WithBoolParam("include_body", aitool.WithParam_Description("When entity=snapshot, include truncated JSON file body")),
			aitool.WithStringParam("status_filter", aitool.WithParam_Description("For dynamic_vuln_findings: confirmed|uncertain|false_positive; empty=all")),
			aitool.WithStringParam("kind", aitool.WithParam_Description("For phase_artifacts: filter by artifact kind")),
			aitool.WithStringParam("stage_filter", aitool.WithParam_Description("For file_operations: filter by pipeline_stage")),
			aitool.WithStringParam("operation_filter", aitool.WithParam_Description("For file_operations: filter by operation")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			ent := strings.ToLower(strings.TrimSpace(action.GetString("entity")))
			if ent == "" {
				return utils.Error("entity required")
			}
			ok := false
			for _, e := range store.AllSessionReadEntities() {
				if ent == e {
					ok = true
					break
				}
			}
			if !ok {
				return utils.Errorf("unknown entity %q", action.GetString("entity"))
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			ent := strings.ToLower(strings.TrimSpace(action.GetString("entity")))
			limit := action.GetInt("limit")
			if limit <= 0 {
				limit = 200
			}
			if limit > 2000 {
				limit = 2000
			}
			feat, featRouteKeys, featureScoped := loopFeatureScope(loop)
			ctrlScope, ctrlRouteKeys, controllerScoped := loopControllerScope(loop)

			var payload any
			var err error
			switch ent {
			case store.SessionEntityStatus:
				payload, err = buildDiscoveryStatusPayload(rt, sess)
			case store.SessionEntitySchema:
				payload = map[string]any{
					"sqlite_path": rt.SQLitePath,
					"tables":      store.DocumentedSessionTableColumns,
					"note":        "Use these column names only; do not invent columns for custom SQL.",
				}
			case store.SessionEntitySnapshot:
				payload, err = readDiscoverySnapshotPayload(rt, action.GetBool("refresh_snapshot"), action.GetBool("include_body"))
			case store.SessionEntityHTTPEndpoints:
				rows, e := rt.Repo.ListHttpEndpoints(sess.ID)
				if controllerScoped {
					total := len(rows)
					rows = filterHttpEndpointsByController(rows, ctrlScope, ctrlRouteKeys)
					payload, err = sessionRowsPayload(rows, e, limit)
					if err == nil {
						payload = attachControllerScopeToPayload(payload, ctrlScope, total, len(rows))
					}
				} else if featureScoped {
					total := len(rows)
					rows = filterHttpEndpointsByFeature(rows, feat, featRouteKeys)
					payload, err = sessionRowsPayload(rows, e, limit)
					if err == nil {
						payload = attachFeatureScopeToPayload(payload, feat, total, len(rows))
					}
				} else {
					payload, err = sessionRowsPayload(rows, e, limit)
				}
			case store.SessionEntityVerifiedEndpoints:
				rows, e := rt.Repo.ListVerifiedEndpoints(sess.ID)
				if controllerScoped {
					eps, e2 := rt.Repo.ListHttpEndpoints(sess.ID)
					if e2 != nil {
						err = e2
						break
					}
					eps = filterHttpEndpointsByController(eps, ctrlScope, ctrlRouteKeys)
					allowed := map[uint]struct{}{}
					for i := range eps {
						allowed[eps[i].ID] = struct{}{}
					}
					total := len(rows)
					rows = filterVerifiedEndpointsByFeature(rows, allowed, FeatureInventoryEntry{}, nil)
					payload, err = sessionRowsPayload(rows, e, limit)
					if err == nil {
						payload = attachControllerScopeToPayload(payload, ctrlScope, total, len(rows))
					}
				} else if featureScoped {
					eps, e2 := rt.Repo.ListHttpEndpoints(sess.ID)
					if e2 != nil {
						err = e2
						break
					}
					allowed := featureScopedEndpointIDs(eps, feat, featRouteKeys)
					total := len(rows)
					rows = filterVerifiedEndpointsByFeature(rows, allowed, feat, featRouteKeys)
					payload, err = sessionRowsPayload(rows, e, limit)
					if err == nil {
						payload = attachFeatureScopeToPayload(payload, feat, total, len(rows))
					}
				} else {
					payload, err = sessionRowsPayload(rows, e, limit)
				}
			case store.SessionEntityVerifiedHTTPApis:
				rows, e := rt.Repo.ListVerifiedHttpApis(sess.ID)
				if controllerScoped {
					total := len(rows)
					rows = filterVerifiedHttpApisByController(rows, ctrlScope, ctrlRouteKeys)
					payload, err = sessionRowsPayload(rows, e, limit)
					if err == nil {
						payload = attachControllerScopeToPayload(payload, ctrlScope, total, len(rows))
					}
				} else if featureScoped {
					total := len(rows)
					rows = filterVerifiedHttpApisByFeature(rows, feat, featRouteKeys)
					payload, err = sessionRowsPayload(rows, e, limit)
					if err == nil {
						payload = attachFeatureScopeToPayload(payload, feat, total, len(rows))
					}
				} else {
					payload, err = sessionRowsPayload(rows, e, limit)
				}
			case store.SessionEntitySyntaxflowFindings:
				rows, e := rt.Repo.ListDiscoverySyntaxFlowFindings(sess.ID)
				payload, err = sessionRowsPayload(rows, e, limit)
			case store.SessionEntityVulnVerifications:
				rows, e := rt.Repo.ListVulnVerifications(sess.ID)
				payload, err = sessionRowsPayload(rows, e, limit)
			case store.SessionEntityDynamicVulnFindings:
				sf := strings.TrimSpace(action.GetString("status_filter"))
				var rows []store.DynamicVulnFinding
				var e error
				if sf != "" {
					rows, e = rt.Repo.ListDynamicVulnFindingsByStatus(sess.ID, sf)
				} else {
					rows, e = rt.Repo.ListDynamicVulnFindings(sess.ID)
				}
				payload, err = sessionRowsPayload(rows, e, limit)
			case store.SessionEntityAuthCredentials:
				rows, e := rt.Repo.ListAuthCredentials(sess.ID)
				payload, err = sessionRowsPayload(rows, e, limit)
			case store.SessionEntityComponents:
				rows, e := rt.Repo.ListComponents(sess.ID)
				payload, err = sessionRowsPayload(rows, e, limit)
			case store.SessionEntityConfigArtifacts:
				rows, e := rt.Repo.ListConfigArtifacts(sess.ID)
				payload, err = sessionRowsPayload(rows, e, limit)
			case store.SessionEntityDependencies:
				rows, e := rt.Repo.ListDependencies(sess.ID)
				payload, err = sessionRowsPayload(rows, e, limit)
			case store.SessionEntitySecurityMechanisms:
				rows, e := rt.Repo.ListSecurityMechanisms(sess.ID)
				payload, err = sessionRowsPayload(rows, e, limit)
			case store.SessionEntityBusinessCapabilities:
				rows, e := rt.Repo.ListBusinessCapabilities(sess.ID)
				payload, err = sessionRowsPayload(rows, e, limit)
			case store.SessionEntityVulnChecklistItems:
				rows, e := rt.Repo.ListVulnChecklistItems(sess.ID)
				payload, err = sessionRowsPayload(rows, e, limit)
			case store.SessionEntityPhaseArtifacts:
				kind := strings.TrimSpace(action.GetString("kind"))
				rows, e := rt.Repo.ListPhaseArtifacts(sess.ID, kind, limit)
				payload, err = sessionRowsPayload(rows, e, limit)
			case store.SessionEntityCoverageWorkItems:
				rows, e := rt.Repo.ListCoverageWorkItems(sess.ID, store.CoverageKindHttpEndpoint, limit)
				if featureScoped {
					eps, e2 := rt.Repo.ListHttpEndpoints(sess.ID)
					if e2 != nil {
						err = e2
						break
					}
					allowed := featureScopedEndpointIDs(eps, feat, featRouteKeys)
					total := len(rows)
					rows = filterCoverageWorkItemsByFeature(rows, allowed, feat)
					payload, err = sessionRowsPayload(rows, e, limit)
					if err == nil {
						payload = attachFeatureScopeToPayload(payload, feat, total, len(rows))
					}
				} else {
					payload, err = sessionRowsPayload(rows, e, limit)
				}
			case store.SessionEntityDiscoveryEvents:
				rows, e := rt.Repo.ListEvents(sess.ID, limit)
				payload, err = sessionRowsPayload(rows, e, limit)
			case store.SessionEntityEndpointValidationAttempts:
				rows, e := rt.Repo.ListEndpointValidationAttemptsBySession(sess.ID, limit)
				if featureScoped {
					eps, e2 := rt.Repo.ListHttpEndpoints(sess.ID)
					if e2 != nil {
						err = e2
						break
					}
					allowed := featureScopedEndpointIDs(eps, feat, featRouteKeys)
					total := len(rows)
					rows = filterEndpointValidationAttemptsByFeature(rows, allowed, feat)
					payload, err = sessionRowsPayload(rows, e, limit)
					if err == nil {
						payload = attachFeatureScopeToPayload(payload, feat, total, len(rows))
					}
				} else {
					payload, err = sessionRowsPayload(rows, e, limit)
				}
			case store.SessionEntityFileOperations:
				stage := strings.TrimSpace(action.GetString("stage_filter"))
				opFilter := strings.TrimSpace(action.GetString("operation_filter"))
				rows, e := rt.Repo.ListFileOperations(sess.ID, stage, opFilter, limit)
				payload, err = sessionRowsPayload(rows, e, limit)
			default:
				err = utils.Errorf("unsupported entity %q", ent)
			}
			if err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			wrap := map[string]any{
				"entity":       ent,
				"session_uuid": sess.UUID,
				"sqlite_path":  rt.SQLitePath,
				"data":         payload,
			}
			if controllerScoped {
				wrap["controller_scope_active"] = true
				wrap["controller_file"] = ctrlScope.ControllerFile
				wrap["feature_id"] = ctrlScope.FeatureID
			} else if featureScoped {
				wrap["feature_scope_active"] = true
				wrap["feature_id"] = feat.FeatureID
			}
			b, _ := json.MarshalIndent(wrap, "", "  ")
			op.Feedback(utils.ShrinkString(string(b), 24000))
			op.Continue()
		},
	)
}

func sessionRowsPayload(rows any, err error, limit int) (map[string]any, error) {
	if err != nil {
		return nil, err
	}
	total := 0
	switch v := rows.(type) {
	case []store.HttpEndpoint:
		total = len(v)
		if limit > 0 && len(v) > limit {
			v = v[:limit]
		}
		rows = v
	case []store.VerifiedEndpoint:
		total = len(v)
		if limit > 0 && len(v) > limit {
			v = v[:limit]
		}
		rows = v
	case []store.VerifiedHttpApi:
		total = len(v)
		if limit > 0 && len(v) > limit {
			v = v[:limit]
		}
		rows = v
	case []store.DiscoverySyntaxFlowFinding:
		total = len(v)
		if limit > 0 && len(v) > limit {
			v = v[:limit]
		}
		rows = v
	case []store.VulnVerification:
		total = len(v)
		if limit > 0 && len(v) > limit {
			v = v[:limit]
		}
		rows = v
	case []store.DynamicVulnFinding:
		total = len(v)
		if limit > 0 && len(v) > limit {
			v = v[:limit]
		}
		rows = v
	case []store.AuthCredential:
		total = len(v)
		if limit > 0 && len(v) > limit {
			v = v[:limit]
		}
		rows = v
	case []store.ArchitectureComponent:
		total = len(v)
		if limit > 0 && len(v) > limit {
			v = v[:limit]
		}
		rows = v
	case []store.ConfigArtifact:
		total = len(v)
		if limit > 0 && len(v) > limit {
			v = v[:limit]
		}
		rows = v
	case []store.DependencyRef:
		total = len(v)
		if limit > 0 && len(v) > limit {
			v = v[:limit]
		}
		rows = v
	case []store.SecurityMechanism:
		total = len(v)
		if limit > 0 && len(v) > limit {
			v = v[:limit]
		}
		rows = v
	case []store.BusinessCapability:
		total = len(v)
		if limit > 0 && len(v) > limit {
			v = v[:limit]
		}
		rows = v
	case []store.VulnChecklistItem:
		total = len(v)
		if limit > 0 && len(v) > limit {
			v = v[:limit]
		}
		rows = v
	case []store.PhaseArtifact:
		total = len(v)
		if limit > 0 && len(v) > limit {
			v = v[:limit]
		}
		rows = v
	case []store.CoverageWorkItem:
		total = len(v)
		if limit > 0 && len(v) > limit {
			v = v[:limit]
		}
		rows = v
	case []store.DiscoveryEvent:
		total = len(v)
		if limit > 0 && len(v) > limit {
			v = v[:limit]
		}
		rows = v
	case []store.EndpointValidationAttempt:
		total = len(v)
		if limit > 0 && len(v) > limit {
			v = v[:limit]
		}
		rows = v
	case []store.DiscoveryFileOperation:
		total = len(v)
		if limit > 0 && len(v) > limit {
			v = v[:limit]
		}
		rows = v
	default:
		return nil, utils.Error("internal: unsupported row type for sessionRowsPayload")
	}
	shown := total
	if limit > 0 && shown > limit {
		shown = limit
	}
	return map[string]any{
		"total":     total,
		"limit":     limit,
		"truncated": total > shown,
		"rows":      rows,
	}, nil
}

func readDiscoverySnapshotPayload(rt *Runtime, refresh, includeBody bool) (map[string]any, error) {
	if rt == nil {
		return nil, utils.Error("nil runtime")
	}
	path := store.DiscoverySnapshotPath(rt.WorkDir)
	if refresh {
		p, err := ExportDiscoverySnapshotJSON(rt)
		if err != nil {
			return nil, err
		}
		path = p
	} else if _, err := os.Stat(path); err != nil {
		p, err2 := ExportDiscoverySnapshotJSON(rt)
		if err2 != nil {
			return nil, err2
		}
		path = p
	}
	out := map[string]any{
		"snapshot_path": path,
		"refresh":       refresh,
	}
	if includeBody {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		out["body_excerpt"] = utils.ShrinkString(string(b), 20000)
	}
	return out, nil
}

// buildDiscoveryStatusPayload mirrors discovery_get_status JSON (shared read path).
func buildDiscoveryStatusPayload(rt *Runtime, sess *store.DiscoverySession) (map[string]any, error) {
	if rt == nil || rt.Repo == nil || sess == nil {
		return nil, utils.Error("nil runtime or session")
	}
	c1, c2, d, e, se, biz, ver, sf, vv, err := rt.Repo.CountsBySession(sess.ID)
	if err != nil {
		return nil, err
	}
	covTotal, _ := rt.Repo.CountCoverageWorkItems(sess.ID, store.CoverageKindHttpEndpoint)
	covPending, _ := rt.Repo.CountCoverageWorkItemsByStatus(sess.ID, store.CoverageKindHttpEndpoint, store.CoverageStatusPending)

	payload := map[string]any{
		"session_uuid":            sess.UUID,
		"sqlite_path":             rt.SQLitePath,
		"code_root_path":          sess.CodeRootPath,
		"code_path_ok":            sess.CodePathOK,
		"target_raw":              sess.TargetRaw,
		"target_host":             sess.TargetHost,
		"target_port":             sess.TargetPort,
		"target_scheme":           sess.TargetScheme,
		"target_base_url":         EffectiveTargetBaseURL(sess),
		"effective_bases":         FormatEffectiveBasesForPrompt(sess),
		"routing_profile_excerpt": utils.ShrinkString(sess.RoutingProfileJSON, 800),
		"target_url_hints":        targetURLHints(sess),
		"target_reachable":        sess.TargetReachable,
		"target_probe_method":     sess.TargetProbeMethod,
		"target_probe_detail":     sess.TargetProbeDetail,
		"language":                sess.Language,
		"ssa_ok":                  sess.SSACompileOK,
		"ssa_program_name":        sess.SSAProgramName,
		"ssa_file_count":          sess.SSAFileCount,
		"ssa_compile_error":       sess.SSACompileError,
		"phase":                   sess.Phase,
		"notes":                   sess.Notes,
		"counts": map[string]int{
			"components": c1, "config_artifacts": c2, "dependencies": d,
			"http_endpoints": e, "security_mechanisms": se, "business_capabilities": biz,
			"verified_endpoints": ver, "syntaxflow_findings": sf, "vuln_verifications": vv,
		},
		"coverage_http": map[string]int64{"total": covTotal, "pending": covPending},
	}
	var endpointHarvest, apiPreanalysis, apiSpecImport, apiBaseCalibration map[string]any
	if strings.TrimSpace(sess.EndpointHarvestMetaJSON) != "" {
		_ = json.Unmarshal([]byte(sess.EndpointHarvestMetaJSON), &endpointHarvest)
	}
	if strings.TrimSpace(sess.ApiPreanalysisMetaJSON) != "" {
		_ = json.Unmarshal([]byte(sess.ApiPreanalysisMetaJSON), &apiPreanalysis)
	}
	if strings.TrimSpace(sess.ApiSpecImportMetaJSON) != "" {
		_ = json.Unmarshal([]byte(sess.ApiSpecImportMetaJSON), &apiSpecImport)
	}
	if strings.TrimSpace(sess.ApiBaseCalibrationMetaJSON) != "" {
		_ = json.Unmarshal([]byte(sess.ApiBaseCalibrationMetaJSON), &apiBaseCalibration)
	}
	payload["endpoint_harvest"] = endpointHarvest
	payload["api_preanalysis"] = apiPreanalysis
	payload["api_spec_import"] = apiSpecImport
	payload["api_base_calibration"] = apiBaseCalibration
	if total, verified, err := rt.Repo.CountVerifiedHttpApis(sess.ID); err == nil {
		rejected := total - verified
		payload["verified_http_apis"] = map[string]int{"total": total, "verified": verified, "rejected": rejected}
		if c, ok := payload["counts"].(map[string]int); ok {
			c["verified_http_apis_total"] = total
			c["verified_http_apis_verified"] = verified
			c["verified_http_apis_rejected"] = rejected
		}
	}
	if n, err := rt.Repo.ListVulnChecklistItems(sess.ID); err == nil {
		if c, ok := payload["counts"].(map[string]int); ok {
			c["vuln_checklist_items"] = len(n)
		}
		high, medium, low, none, _ := rt.Repo.CountVulnChecklistByConfidence(sess.ID)
		payload["vuln_checklist_assoc"] = map[string]int64{
			"high": high, "medium": medium, "low": low, "none": none,
		}
	}
	if n, err := rt.Repo.CountPhaseArtifacts(sess.ID); err == nil {
		if c, ok := payload["counts"].(map[string]int); ok {
			c["phase_artifacts"] = int(n)
		}
	}
	if evN, err := rt.Repo.CountEvents(sess.ID); err == nil {
		if c, ok := payload["counts"].(map[string]int); ok {
			c["discovery_events"] = int(evN)
		}
	}
	payload["route_candidates_count"] = countRouteCandidates(rt)
	return payload, nil
}
