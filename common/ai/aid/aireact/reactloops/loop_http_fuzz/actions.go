package loop_http_fuzz

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
)

var loadHTTPRequestAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"load_http_request",
		"Load the raw HTTP request, analyze the request surface, and establish a baseline response fingerprint.",
		[]aitool.ToolOption{
			aitool.WithStringParam("http_request", aitool.WithParam_Description("Raw HTTP request packet to test."), aitool.WithParam_Required(true)),
			aitool.WithBoolParam("is_https", aitool.WithParam_Description("Whether the request should be sent via HTTPS.")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("Why this request should be loaded.")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if action.GetString("http_request") == "" {
				return utils.Error("http_request is required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			raw := action.GetString("http_request")
			isHTTPS := action.GetBool("is_https")
			if reason := strings.TrimSpace(action.GetString("reason")); reason != "" {
				loop.Set(stateUserGoal, reason)
			} else if currentUserGoal(loop) != "" {
				loop.Set(stateUserGoal, currentUserGoal(loop))
			}

			profile, inventory, err := analyzeAndStoreRequest(loop, raw, isHTTPS)
			if err != nil {
				op.Fail(err)
				return
			}

			r.AddToTimeline("load_http_request", fmt.Sprintf("loaded %s %s with %d targets", profile.Method, profile.Path, len(inventory)))
			op.Feedback(fmt.Sprintf(
				"Request loaded.\nBusiness guess: %s\nHigh-value targets: %d\nBaseline status: %d",
				profile.BusinessGuess,
				len(extractHighValueTargets(inventory)),
				getBaselineFingerprint(loop).StatusCode,
			))
		},
	)
}

var inspectRequestSurfaceAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"inspect_request_surface",
		"Summarize the structured request surface and highlight high-value fuzz targets.",
		[]aitool.ToolOption{
			aitool.WithStringArrayParam("focus", aitool.WithParam_Description("Optional focus tags such as credential, identifier, url_like, file_like.")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("Why the request surface is being inspected.")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if loop.Get(stateOriginalRequest) == "" {
				return utils.Error("load_http_request must be called first")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			focus := action.GetStringSlice("focus")
			items := getParameterInventory(loop)
			if len(focus) > 0 {
				var filtered []parameterInventoryItem
				for _, item := range items {
					if containsAny(strings.Join(item.HighValueTags, ","), focus) {
						filtered = append(filtered, item)
					}
				}
				items = filtered
			}
			profile, _ := loop.GetVariable(stateRequestProfile).(*requestProfile)
			if profile == nil {
				op.Fail("request profile missing")
				return
			}
			r.AddToTimeline("inspect_request_surface", fmt.Sprintf("summarized %d targets", len(items)))
			op.Feedback(fmt.Sprintf(
				"Request surface ready.\nBusiness guess: %s\nTargets:\n%s",
				profile.BusinessGuess,
				toPrettyJSON(items),
			))
		},
	)
}

var mutateTargetAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"mutate_target",
		"Prepare a unified target mutation for later batch execution.",
		[]aitool.ToolOption{
			aitool.WithStringParam("target_ref", aitool.WithParam_Description("Structured target_ref such as query:id, json:$.username, path:block:2."), aitool.WithParam_Required(true)),
			aitool.WithStringParam("mutation_mode", aitool.WithParam_Description("replace / append / prefix / suffix / raw_replace / base64_wrap"), aitool.WithParam_Required(true)),
			aitool.WithStringArrayParam("payloads", aitool.WithParam_Description("Payloads or fuzztag templates to apply."), aitool.WithParam_Required(true)),
			aitool.WithBoolParam("use_fuzztag", aitool.WithParam_Description("Whether payloads intentionally contain Yak fuzztag templates.")),
			aitool.WithStringParam("encoding_policy", aitool.WithParam_Description("preserve / force_url / force_base64 / no_encode / inherit")),
			aitool.WithBoolParam("disable_auto_encode", aitool.WithParam_Description("Disable automatic encoding in the underlying fuzz request.")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("Why this target should be mutated.")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if loop.Get(stateOriginalRequest) == "" {
				return utils.Error("load_http_request must be called first")
			}
			if _, err := parseTargetRef(action.GetString("target_ref")); err != nil {
				return err
			}
			if len(action.GetStringSlice("payloads")) == 0 {
				return utils.Error("payloads is required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			spec := mutationSpec{
				TargetRef:         action.GetString("target_ref"),
				MutationMode:      action.GetString("mutation_mode"),
				Payloads:          action.GetStringSlice("payloads"),
				UseFuzztag:        action.GetBool("use_fuzztag"),
				EncodingPolicy:    action.GetString("encoding_policy"),
				DisableAutoEncode: action.GetBool("disable_auto_encode"),
				Reason:            action.GetString("reason"),
			}
			if spec.EncodingPolicy == "" {
				spec.EncodingPolicy = "preserve"
			}
			loop.Set(stateLastMutation, spec)
			r.AddToTimeline("mutate_target", fmt.Sprintf("%s via %s", spec.TargetRef, spec.MutationMode))
			op.Feedback(fmt.Sprintf("Prepared mutation for %s with %d payloads.", spec.TargetRef, len(spec.Payloads)))
		},
	)
}

var executeTestBatchAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"execute_test_batch",
		"Execute a prepared mutation or a named profile as a batch and collect anomalies.",
		[]aitool.ToolOption{
			aitool.WithStringParam("scenario", aitool.WithParam_Description("Scenario label such as sqli, xss, weak_password."), aitool.WithParam_Required(true)),
			aitool.WithStringArrayParam("target_refs", aitool.WithParam_Description("Targets to execute when not using last_mutation.")),
			aitool.WithStringParam("profile", aitool.WithParam_Description("Optional payload profile name.")),
			aitool.WithStringParam("variant_source", aitool.WithParam_Description("Use last_mutation to execute the last prepared mutate_target.")),
			aitool.WithIntegerParam("max_requests", aitool.WithParam_Description("Maximum number of requests to retain from the batch.")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("Why this batch is executed.")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if loop.Get(stateOriginalRequest) == "" {
				return utils.Error("load_http_request must be called first")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			scenario := action.GetString("scenario")
			maxRequests := action.GetInt("max_requests", 12)
			if maxRequests <= 0 {
				maxRequests = 12
			}
			var (
				results []batchResult
				err     error
			)
			if action.GetString("variant_source") == "last_mutation" {
				results, err = applyPreparedMutation(loop, getPreparedMutation(loop), scenario, maxRequests)
			} else {
				targetRefs := action.GetStringSlice("target_refs")
				if len(targetRefs) == 0 {
					err = utils.Error("target_refs is required unless variant_source=last_mutation")
				} else {
					results, err = executeProfileBatch(loop, scenario, targetRefs, action.GetString("profile"), maxRequests, action.GetString("reason"))
				}
			}
			if err != nil {
				op.Fail(err)
				return
			}
			r.AddToTimeline("execute_test_batch", fmt.Sprintf("%s executed with %d results", scenario, len(results)))
			op.Feedback(fmt.Sprintf(
				"Executed %s batch.\nResults: %d\nCandidates: %d",
				scenario,
				len(results),
				len(getAnomalyCandidates(loop)),
			))
		},
	)
}

var runGenericVulnTestAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"run_generic_vuln_test",
		"Run a named vulnerability profile against one or more targets.",
		[]aitool.ToolOption{
			aitool.WithStringParam("scenario", aitool.WithParam_Description("sqli / xss / ssti / cmdi / traversal / ssrf / crlf / auth_bypass"), aitool.WithParam_Required(true)),
			aitool.WithStringArrayParam("target_refs", aitool.WithParam_Description("Targets to test."), aitool.WithParam_Required(true)),
			aitool.WithStringParam("profile", aitool.WithParam_Description("Payload profile name."), aitool.WithParam_Required(true)),
			aitool.WithStringParam("depth", aitool.WithParam_Description("low / medium / high")),
			aitool.WithBoolParam("prefer_fuzztag", aitool.WithParam_Description("Prefer built-in fuzztag-based payloads.")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("Why this test is being run.")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if loop.Get(stateOriginalRequest) == "" {
				return utils.Error("load_http_request must be called first")
			}
			if len(action.GetStringSlice("target_refs")) == 0 {
				return utils.Error("target_refs is required")
			}
			if action.GetString("profile") == "" {
				return utils.Error("profile is required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			depth := action.GetString("depth", "medium")
			maxRequests := depthToMaxRequests(depth)
			results, err := executeProfileBatch(loop, action.GetString("scenario"), action.GetStringSlice("target_refs"), action.GetString("profile"), maxRequests, action.GetString("reason"))
			if err != nil {
				op.Fail(err)
				return
			}
			op.Feedback(fmt.Sprintf("Executed %s on %d targets and collected %d results.", action.GetString("scenario"), len(action.GetStringSlice("target_refs")), len(results)))
		},
	)
}

var runWeakPasswordTestAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"run_weak_password_test",
		"Run weak-password and simple auth bypass checks for credential-bearing requests.",
		[]aitool.ToolOption{
			aitool.WithStringArrayParam("username_targets", aitool.WithParam_Description("Username-like targets.")),
			aitool.WithStringArrayParam("password_targets", aitool.WithParam_Description("Password-like targets.")),
			aitool.WithStringParam("username_strategy", aitool.WithParam_Description("dictionary")),
			aitool.WithStringParam("password_strategy", aitool.WithParam_Description("top_weak")),
			aitool.WithIntegerParam("max_pairs", aitool.WithParam_Description("Max credential pairs to execute.")),
			aitool.WithBoolParam("prefer_fuzztag", aitool.WithParam_Description("Prefer fuzztag dictionaries where suitable.")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("Why the test is run.")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if loop.Get(stateOriginalRequest) == "" {
				return utils.Error("load_http_request must be called first")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			userTargets := action.GetStringSlice("username_targets")
			passTargets := action.GetStringSlice("password_targets")
			if len(userTargets) == 0 || len(passTargets) == 0 {
				for _, item := range getParameterInventory(loop) {
					if containsAny(strings.Join(item.HighValueTags, ","), []string{"credential"}) {
						if containsAny(strings.ToLower(item.Name+" "+item.TargetRef), []string{"user", "account"}) {
							userTargets = append(userTargets, item.TargetRef)
						}
						if containsAny(strings.ToLower(item.Name+" "+item.TargetRef), []string{"password", "passwd", "pwd"}) {
							passTargets = append(passTargets, item.TargetRef)
						}
					}
				}
				userTargets = dedupeStrings(userTargets)
				passTargets = dedupeStrings(passTargets)
			}
			if len(userTargets) == 0 || len(passTargets) == 0 {
				op.Fail("unable to infer username_targets/password_targets")
				return
			}

			usernames := []string{"admin", "root", "test", "guest"}
			passwords := []string{"admin", "admin123", "123456", "password", "test123"}
			maxPairs := action.GetInt("max_pairs", 20)
			isHTTPS := loop.Get(stateIsHTTPS) == "true"
			baseline := getBaselineFingerprint(loop)
			var results []batchResult
			candidates := getAnomalyCandidates(loop)
			pairs := 0

			for _, username := range usernames {
				for _, password := range passwords {
					if pairs >= maxPairs {
						break
					}
					current, err := buildFreshFuzzRequest(loop)
					if err != nil {
						op.Fail(err)
						return
					}
					mutated := current
					for _, target := range userTargets {
						mutated, _, err = applyMutation(mutated, target, "replace", []string{username}, "preserve", false)
						if err != nil {
							op.Fail(err)
							return
						}
					}
					for _, target := range passTargets {
						mutated, _, err = applyMutation(mutated, target, "replace", []string{password}, "preserve", false)
						if err != nil {
							op.Fail(err)
							return
						}
					}
					result, execErr := mutated.ExecFirst(mutate.WithPoolOpt_Https(isHTTPS))
					if execErr != nil {
						log.Warnf("run_weak_password_test exec failed: %v", execErr)
						continue
					}
					payload := username + "/" + password
					fp := fingerprintResponse(result.ResponseRaw, username, result.DurationMs)
					signals, summary := diffResponse(baseline, fp, username)
					item := batchResult{
						BatchID:     newID("b"),
						Scenario:    "weak_password",
						TargetRef:   strings.Join(append(userTargets, passTargets...), ","),
						Payload:     payload,
						Signals:     signals,
						Summary:     summary,
						RequestRaw:  string(result.RequestRaw),
						ResponseRaw: string(result.ResponseRaw),
						Fingerprint: fp,
						DurationMs:  result.DurationMs,
					}
					if len(signals) > 0 {
						candidate := anomalyCandidate{
							CandidateID: newID("a"),
							Scenario:    "weak_password",
							TargetRef:   item.TargetRef,
							Payload:     payload,
							Signals:     signals,
							Confidence:  candidateConfidence(signals),
							NeedsRetest: true,
							Summary:     summary,
						}
						item.CandidateID = candidate.CandidateID
						candidates = append(candidates, candidate)
					}
					results = append(results, item)
					pairs++
				}
			}

			loop.Set(stateAnomalyCandidates, candidates)
			loop.Set(stateLastBatchResults, tailBatchResults(results, 12))
			updateNextActions(loop, "weak_password", action.GetString("reason"))
			op.Feedback(fmt.Sprintf("Weak password batch executed with %d pairs, %d anomalies.", len(results), len(candidates)))
		},
	)
}

var runIdentifierEnumerationAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"run_identifier_enumeration",
		"Run identifier enumeration against a single target using numeric or zero-padded profiles.",
		[]aitool.ToolOption{
			aitool.WithStringParam("target_ref", aitool.WithParam_Description("Identifier-like target_ref."), aitool.WithParam_Required(true)),
			aitool.WithStringParam("strategy", aitool.WithParam_Description("numeric_range / zero_padded / dictionary")),
			aitool.WithStringParam("range_template", aitool.WithParam_Description("Optional fuzztag range template such as {{int(1-50|4)}}.")),
			aitool.WithIntegerParam("max_requests", aitool.WithParam_Description("Max requests.")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("Why the enumeration is being run.")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if loop.Get(stateOriginalRequest) == "" {
				return utils.Error("load_http_request must be called first")
			}
			if _, err := parseTargetRef(action.GetString("target_ref")); err != nil {
				return err
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			profile := "id_enum_numeric"
			switch action.GetString("strategy") {
			case "zero_padded":
				profile = "id_enum_zero_padded"
			}
			if tpl := action.GetString("range_template"); tpl != "" {
				loop.Set(stateLastMutation, mutationSpec{
					TargetRef:      action.GetString("target_ref"),
					MutationMode:   "replace",
					Payloads:       []string{tpl},
					UseFuzztag:     true,
					EncodingPolicy: "preserve",
					Reason:         action.GetString("reason"),
				})
				results, err := applyPreparedMutation(loop, getPreparedMutation(loop), "identifier_enumeration", action.GetInt("max_requests", 12))
				if err != nil {
					op.Fail(err)
					return
				}
				op.Feedback(fmt.Sprintf("Identifier enumeration executed with %d results.", len(results)))
				return
			}
			results, err := executeProfileBatch(loop, "identifier_enumeration", []string{action.GetString("target_ref")}, profile, action.GetInt("max_requests", 12), action.GetString("reason"))
			if err != nil {
				op.Fail(err)
				return
			}
			op.Feedback(fmt.Sprintf("Identifier enumeration executed with %d results.", len(results)))
		},
	)
}

var runSensitiveInfoExposureTestAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"run_sensitive_info_exposure_test",
		"Probe common debug, backup, and metadata paths for sensitive exposure.",
		[]aitool.ToolOption{
			aitool.WithStringParam("mode", aitool.WithParam_Description("path_probe")),
			aitool.WithStringParam("path_profile", aitool.WithParam_Description("Profile name, default debug_leak_probe.")),
			aitool.WithIntegerParam("max_requests", aitool.WithParam_Description("Max requests.")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("Why this test is run.")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if loop.Get(stateOriginalRequest) == "" {
				return utils.Error("load_http_request must be called first")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			profile := action.GetString("path_profile", "debug_leak_probe")
			results, err := executeProfileBatch(loop, "sensitive_info_exposure", []string{"path"}, profile, action.GetInt("max_requests", 12), action.GetString("reason"))
			if err != nil {
				op.Fail(err)
				return
			}
			op.Feedback(fmt.Sprintf("Sensitive exposure probe executed with %d results.", len(results)))
		},
	)
}

var runEncodingBypassTestAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"run_encoding_bypass_test",
		"Generate encoded variants from a base payload and execute them against one target.",
		[]aitool.ToolOption{
			aitool.WithStringParam("target_ref", aitool.WithParam_Description("Target to mutate."), aitool.WithParam_Required(true)),
			aitool.WithStringParam("base_payload", aitool.WithParam_Description("Original semantic payload."), aitool.WithParam_Required(true)),
			aitool.WithStringArrayParam("encodings", aitool.WithParam_Description("url / double_url / unicode / hex / base64")),
			aitool.WithBoolParam("mixed_case", aitool.WithParam_Description("Whether to add mixed-case variant.")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("Why this encoding bypass test is run.")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if loop.Get(stateOriginalRequest) == "" {
				return utils.Error("load_http_request must be called first")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			payloads := []string{action.GetString("base_payload")}
			base := action.GetString("base_payload")
			for _, enc := range action.GetStringSlice("encodings") {
				switch enc {
				case "url":
					payloads = append(payloads, "{{urlenc("+base+")}}")
				case "double_url":
					payloads = append(payloads, "{{doubleurlenc("+base+")}}")
				case "unicode":
					payloads = append(payloads, "{{unicode("+base+")}}")
				case "hex":
					payloads = append(payloads, "{{hex("+base+")}}")
				case "base64":
					payloads = append(payloads, "{{base64("+base+")}}")
				}
			}
			if action.GetBool("mixed_case") {
				payloads = append(payloads, "{{randomupper("+base+")}}")
			}
			loop.Set(stateLastMutation, mutationSpec{
				TargetRef:      action.GetString("target_ref"),
				MutationMode:   "replace",
				Payloads:       dedupeStrings(payloads),
				UseFuzztag:     true,
				EncodingPolicy: "preserve",
				Reason:         action.GetString("reason"),
			})
			results, err := applyPreparedMutation(loop, getPreparedMutation(loop), "encoding_bypass", depthToMaxRequests("medium"))
			if err != nil {
				op.Fail(err)
				return
			}
			op.Feedback(fmt.Sprintf("Encoding bypass batch executed with %d results.", len(results)))
		},
	)
}

var commitFindingAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"commit_finding",
		"Convert anomaly candidates into explicit evidence-backed findings.",
		[]aitool.ToolOption{
			aitool.WithStringArrayParam("candidate_ids", aitool.WithParam_Description("Candidate IDs to commit. Empty means all current candidates.")),
			aitool.WithStringParam("category", aitool.WithParam_Description("Finding category."), aitool.WithParam_Required(true)),
			aitool.WithStringParam("severity", aitool.WithParam_Description("high / medium / low"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("reason", aitool.WithParam_Description("Why the candidate should be committed.")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if loop.Get(stateOriginalRequest) == "" {
				return utils.Error("load_http_request must be called first")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			candidateIDs := dedupeStrings(action.GetStringSlice("candidate_ids"))
			candidates := getAnomalyCandidates(loop)
			if len(candidates) == 0 {
				op.Fail("no anomaly candidates available")
				return
			}
			selected := make([]anomalyCandidate, 0)
			for _, candidate := range candidates {
				if len(candidateIDs) == 0 || containsAny(candidate.CandidateID, candidateIDs) {
					selected = append(selected, candidate)
				}
			}
			if len(selected) == 0 {
				op.Fail("no matching candidate_ids found")
				return
			}

			findings := getConfirmedFindings(loop)
			for _, candidate := range selected {
				findings = append(findings, confirmedFinding{
					FindingID:  newID("f"),
					Category:   action.GetString("category"),
					Severity:   action.GetString("severity"),
					TargetRefs: []string{candidate.TargetRef},
					Evidence: []string{
						fmt.Sprintf("payload=%s", candidate.Payload),
						fmt.Sprintf("signals=%s", strings.Join(candidate.Signals, ",")),
						candidate.Summary,
					},
					Conclusion: fmt.Sprintf("%s candidate on %s shows %s", action.GetString("category"), candidate.TargetRef, strings.Join(candidate.Signals, ", ")),
					NextStep:   action.GetString("reason"),
				})
			}
			loop.Set(stateConfirmedFindings, findings)
			loop.Set(stateNextRecommendedActions, []string{"summarize findings", "stop when budget is low"})
			r.AddToTimeline("commit_finding", fmt.Sprintf("committed %d findings", len(selected)))
			op.Feedback(fmt.Sprintf("Committed %d findings.", len(selected)))
		},
	)
}

func depthToMaxRequests(depth string) int {
	switch depth {
	case "low":
		return 6
	case "high":
		return 18
	default:
		return 12
	}
}
