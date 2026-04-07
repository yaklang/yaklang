package loop_http_fuzz

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

const (
	stateUserGoal               = "user_goal"
	stateFuzzRequest            = "fuzz_request"
	stateOriginalRequest        = "original_request"
	stateIsHTTPS                = "is_https"
	stateRequestProfile         = "request_profile"
	stateParameterInventory     = "parameter_inventory"
	stateHighValueTargets       = "high_value_targets"
	stateBaselineRequest        = "baseline_request"
	stateBaselineResponse       = "baseline_response"
	stateBaselineFingerprint    = "baseline_fingerprint"
	stateTestPlan               = "test_plan"
	stateCoverageMap            = "coverage_map"
	stateAttemptHistory         = "attempt_history"
	stateAnomalyCandidates      = "anomaly_candidates"
	stateConfirmedFindings      = "confirmed_findings"
	stateInterestingResponses   = "interesting_responses"
	stateNextRecommendedActions = "next_recommended_actions"
	stateLastMutation           = "last_mutation"
	stateLastBatchResults       = "last_batch_results"
	stateLastRequest            = "last_request"
	stateLastResponse           = "last_response"
)

type requestProfile struct {
	Scheme           string   `json:"scheme"`
	Host             string   `json:"host"`
	Method           string   `json:"method"`
	Path             string   `json:"path"`
	ContentType      string   `json:"content_type"`
	HasCookie        bool     `json:"has_cookie"`
	HasAuthorization bool     `json:"has_authorization"`
	IsMultipart      bool     `json:"is_multipart"`
	IsJSONBody       bool     `json:"is_json_body"`
	IsXMLBody        bool     `json:"is_xml_body"`
	BusinessGuess    string   `json:"business_guess"`
	RiskHints        []string `json:"risk_hints"`
}

type parameterInventoryItem struct {
	TargetRef          string   `json:"target_ref"`
	Position           string   `json:"position"`
	Name               string   `json:"name"`
	Path               string   `json:"path,omitempty"`
	ValuePreview       string   `json:"value_preview"`
	ValueType          string   `json:"value_type"`
	Encoding           string   `json:"encoding"`
	HighValueTags      []string `json:"high_value_tags,omitempty"`
	SupportedMutations []string `json:"supported_mutations"`
}

type scenarioPlan struct {
	Scenario string   `json:"scenario"`
	Priority int      `json:"priority"`
	Targets  []string `json:"targets"`
	Profiles []string `json:"profiles"`
	Depth    string   `json:"depth"`
	StopWhen []string `json:"stop_when"`
}

type budgetState struct {
	MaxRequests    int `json:"max_requests"`
	RemainingCount int `json:"remaining_count"`
	MaxBatches     int `json:"max_batches"`
	RemainingBatch int `json:"remaining_batch"`
}

type testPlan struct {
	UserGoal        string         `json:"user_goal"`
	ActiveScenarios []scenarioPlan `json:"active_scenarios"`
	RemainingBudget budgetState    `json:"remaining_budget"`
}

type mutationSpec struct {
	TargetRef         string   `json:"target_ref"`
	MutationMode      string   `json:"mutation_mode"`
	Payloads          []string `json:"payloads"`
	UseFuzztag        bool     `json:"use_fuzztag"`
	EncodingPolicy    string   `json:"encoding_policy"`
	DisableAutoEncode bool     `json:"disable_auto_encode"`
	Reason            string   `json:"reason"`
}

type responseFingerprint struct {
	StatusCode    int            `json:"status_code"`
	ContentType   string         `json:"content_type"`
	ContentLength int            `json:"content_length"`
	HeaderDigest  map[string]any `json:"header_digest"`
	BodyDigest    map[string]any `json:"body_digest"`
	Timing        map[string]any `json:"timing"`
	Signals       []string       `json:"signals,omitempty"`
	Summary       string         `json:"summary,omitempty"`
}

type batchResult struct {
	BatchID     string              `json:"batch_id"`
	Scenario    string              `json:"scenario"`
	TargetRef   string              `json:"target_ref"`
	Payload     string              `json:"payload"`
	Signals     []string            `json:"signals"`
	Summary     string              `json:"summary"`
	RequestRaw  string              `json:"request_raw,omitempty"`
	ResponseRaw string              `json:"response_raw,omitempty"`
	Fingerprint responseFingerprint `json:"fingerprint"`
	DurationMs  int64               `json:"duration_ms"`
	Error       string              `json:"error,omitempty"`
	CandidateID string              `json:"candidate_id,omitempty"`
}

type attemptRecord struct {
	BatchID      string   `json:"batch_id"`
	Scenario     string   `json:"scenario"`
	TargetRefs   []string `json:"target_refs"`
	Profile      string   `json:"profile"`
	RequestCount int      `json:"request_count"`
	AnomalyCount int      `json:"anomaly_count"`
	Summary      string   `json:"summary"`
}

type anomalyCandidate struct {
	CandidateID string   `json:"candidate_id"`
	Scenario    string   `json:"scenario"`
	TargetRef   string   `json:"target_ref"`
	Payload     string   `json:"payload"`
	Signals     []string `json:"signals"`
	Confidence  string   `json:"confidence"`
	NeedsRetest bool     `json:"needs_retest"`
	Summary     string   `json:"summary"`
}

type confirmedFinding struct {
	FindingID  string   `json:"finding_id"`
	Category   string   `json:"category"`
	Severity   string   `json:"severity"`
	TargetRefs []string `json:"target_refs"`
	Evidence   []string `json:"evidence"`
	Conclusion string   `json:"conclusion"`
	NextStep   string   `json:"next_step"`
}

type targetSpec struct {
	Raw        string
	Kind       string
	Name       string
	Path       string
	BlockIndex int
}

func analyzeAndStoreRequest(loop *reactloops.ReActLoop, httpRequest string, isHTTPS bool) (*requestProfile, []parameterInventoryItem, error) {
	fuzzReq, err := mutate.NewFuzzHTTPRequest([]byte(httpRequest), mutate.OptHTTPS(isHTTPS))
	if err != nil {
		return nil, nil, utils.Wrap(err, "create fuzz request")
	}

	profile := buildRequestProfile(fuzzReq, isHTTPS)
	inventory := buildParameterInventory(fuzzReq)
	highValue := extractHighValueTargets(inventory)
	plan := buildTestPlan(currentUserGoal(loop), profile, inventory)

	loop.Set(stateFuzzRequest, fuzzReq)
	loop.Set(stateOriginalRequest, httpRequest)
	loop.Set(stateIsHTTPS, strconv.FormatBool(isHTTPS))
	loop.Set(stateRequestProfile, profile)
	loop.Set(stateParameterInventory, inventory)
	loop.Set(stateHighValueTargets, highValue)
	loop.Set(stateTestPlan, plan)
	loop.Set(stateCoverageMap, map[string][]string{})
	loop.Set(stateAttemptHistory, []attemptRecord{})
	loop.Set(stateAnomalyCandidates, []anomalyCandidate{})
	loop.Set(stateConfirmedFindings, []confirmedFinding{})
	loop.Set(stateInterestingResponses, []batchResult{})
	loop.Set(stateNextRecommendedActions, suggestNextActions(plan))
	loop.Set(stateLastMutation, mutationSpec{})
	loop.Set(stateLastBatchResults, []batchResult{})
	loop.Set(stateLastRequest, "")
	loop.Set(stateLastResponse, "")

	baselineReq := ""
	baselineResp := ""
	var baselineFP responseFingerprint
	if result, execErr := fuzzReq.ExecFirst(mutate.WithPoolOpt_Https(isHTTPS)); execErr == nil && result != nil {
		baselineReq = string(result.RequestRaw)
		baselineResp = string(result.ResponseRaw)
		baselineFP = fingerprintResponse(result.ResponseRaw, "", result.DurationMs)
	}
	loop.Set(stateBaselineRequest, baselineReq)
	loop.Set(stateBaselineResponse, baselineResp)
	loop.Set(stateBaselineFingerprint, baselineFP)

	return profile, inventory, nil
}

func buildRequestProfile(fuzzReq *mutate.FuzzHTTPRequest, isHTTPS bool) *requestProfile {
	scheme := "http"
	if isHTTPS {
		scheme = "https"
	}
	path := fuzzReq.GetPathWithoutQuery()
	contentType := strings.ToLower(fuzzReq.GetContentType())
	profile := &requestProfile{
		Scheme:           scheme,
		Host:             fuzzReq.GetHeader("Host"),
		Method:           fuzzReq.GetMethod(),
		Path:             path,
		ContentType:      contentType,
		HasCookie:        fuzzReq.GetHeader("Cookie") != "",
		HasAuthorization: fuzzReq.GetHeader("Authorization") != "",
		IsMultipart:      strings.Contains(contentType, "multipart/form-data"),
		IsJSONBody:       strings.Contains(contentType, "application/json"),
		IsXMLBody:        strings.Contains(contentType, "xml"),
	}
	profile.BusinessGuess = guessBusiness(profile, fuzzReq)
	profile.RiskHints = dedupeStrings(append(
		guessRiskHints(profile, fuzzReq),
		guessRiskHintsByPath(profile.Path)...,
	))
	return profile
}

func buildParameterInventory(fuzzReq *mutate.FuzzHTTPRequest) []parameterInventoryItem {
	items := make([]parameterInventoryItem, 0)
	for _, param := range fuzzReq.GetAllParams() {
		if param == nil {
			continue
		}
		targetRef, ok := targetRefFromParam(param)
		if !ok || targetRef == "" {
			continue
		}
		value := utils.InterfaceToString(param.GetFirstValue())
		item := parameterInventoryItem{
			TargetRef:          targetRef,
			Position:           param.Position(),
			Name:               param.Name(),
			Path:               normalizePathString(param.Path()),
			ValuePreview:       safePreview(targetRef, param.Name(), value),
			ValueType:          guessValueType(param.GetFirstValue()),
			Encoding:           guessEncoding(targetRef),
			HighValueTags:      classifyHighValueTags(param.Name(), targetRef, value),
			SupportedMutations: supportedMutationModes(targetRef),
		}
		items = append(items, item)
	}

	items = append(items, pathBlockInventoryItems(fuzzReq.GetPathWithoutQuery())...)
	items = append(items, rawSyntheticTargets(fuzzReq)...)

	sort.Slice(items, func(i, j int) bool {
		return items[i].TargetRef < items[j].TargetRef
	})
	return dedupeInventory(items)
}

func pathBlockInventoryItems(path string) []parameterInventoryItem {
	if path == "" {
		return nil
	}
	var items []parameterInventoryItem
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for idx, seg := range segments {
		if seg == "" {
			continue
		}
		ref := fmt.Sprintf("path:block:%d", idx+1)
		items = append(items, parameterInventoryItem{
			TargetRef:          ref,
			Position:           "path_block",
			Name:               strconv.Itoa(idx + 1),
			ValuePreview:       seg,
			ValueType:          guessValueType(seg),
			Encoding:           "plain",
			HighValueTags:      classifyHighValueTags(seg, ref, seg),
			SupportedMutations: []string{"replace", "prefix", "suffix"},
		})
	}
	return items
}

func rawSyntheticTargets(fuzzReq *mutate.FuzzHTTPRequest) []parameterInventoryItem {
	var items []parameterInventoryItem
	if rawQuery := fuzzReq.GetQueryRaw(); rawQuery != "" {
		items = append(items, parameterInventoryItem{
			TargetRef:          "query_raw",
			Position:           "query_raw",
			Name:               "query_raw",
			ValuePreview:       safePreview("query_raw", "query_raw", rawQuery),
			ValueType:          "string",
			Encoding:           "plain",
			SupportedMutations: []string{"replace", "append", "raw_replace"},
		})
	}
	items = append(items, parameterInventoryItem{
		TargetRef:          "path",
		Position:           "path",
		Name:               "path",
		ValuePreview:       fuzzReq.GetPathWithoutQuery(),
		ValueType:          "string",
		Encoding:           "plain",
		SupportedMutations: []string{"replace", "append", "prefix", "suffix"},
	})
	items = append(items, parameterInventoryItem{
		TargetRef:          "method",
		Position:           "method",
		Name:               "method",
		ValuePreview:       fuzzReq.GetMethod(),
		ValueType:          "string",
		Encoding:           "plain",
		SupportedMutations: []string{"replace"},
	})
	if len(fuzzReq.GetBody()) > 0 {
		items = append(items, parameterInventoryItem{
			TargetRef:          "body_raw",
			Position:           "body_raw",
			Name:               "body_raw",
			ValuePreview:       safePreview("body_raw", "body_raw", string(fuzzReq.GetBody())),
			ValueType:          "string",
			Encoding:           "plain",
			SupportedMutations: []string{"replace", "append", "prefix", "suffix", "raw_replace"},
		})
	}
	return items
}

func targetRefFromParam(param *mutate.FuzzHTTPRequestParam) (string, bool) {
	switch param.Position() {
	case "method":
		return "method", true
	case "header":
		return "header:" + param.Name(), true
	case "get-query":
		return "query:" + param.Name(), true
	case "get-query-base64":
		return "query_base64:" + param.Name(), true
	case "get-query-json":
		return "query_json:" + param.Name() + ":" + normalizeJSONPath(param.Path()), true
	case "get-query-base64-json":
		return "query_base64_json:" + param.Name() + ":" + normalizeJSONPath(param.Path()), true
	case "post-query":
		return "form:" + param.Name(), true
	case "post-query-base64":
		return "form_base64:" + param.Name(), true
	case "post-query-json":
		return "form_json:" + param.Name() + ":" + normalizeJSONPath(param.Path()), true
	case "post-query-base64-json":
		return "form_base64_json:" + param.Name() + ":" + normalizeJSONPath(param.Path()), true
	case "post-json":
		return "json:" + normalizeJSONPath(param.Path()), true
	case "post-xml":
		return "xml:" + normalizeXMLPath(param.Path()), true
	case "cookie":
		return "cookie:" + param.Name(), true
	case "cookie-base64":
		return "cookie_base64:" + param.Name(), true
	case "cookie-json":
		return "cookie_json:" + param.Name() + ":" + normalizeJSONPath(param.Path()), true
	case "cookie-base64-json":
		return "cookie_base64_json:" + param.Name() + ":" + normalizeJSONPath(param.Path()), true
	case "body":
		return "body_raw", true
	default:
		return "", false
	}
}

func parseTargetRef(raw string) (targetSpec, error) {
	switch {
	case raw == "method":
		return targetSpec{Raw: raw, Kind: "method"}, nil
	case raw == "path":
		return targetSpec{Raw: raw, Kind: "path"}, nil
	case raw == "query_raw":
		return targetSpec{Raw: raw, Kind: "query_raw"}, nil
	case raw == "body_raw":
		return targetSpec{Raw: raw, Kind: "body_raw"}, nil
	case strings.HasPrefix(raw, "path:block:"):
		i, err := strconv.Atoi(strings.TrimPrefix(raw, "path:block:"))
		if err != nil || i <= 0 {
			return targetSpec{}, utils.Errorf("invalid path block target_ref: %s", raw)
		}
		return targetSpec{Raw: raw, Kind: "path_block", BlockIndex: i}, nil
	case strings.HasPrefix(raw, "header:"):
		return targetSpec{Raw: raw, Kind: "header", Name: strings.TrimPrefix(raw, "header:")}, nil
	case strings.HasPrefix(raw, "query:"):
		return targetSpec{Raw: raw, Kind: "query", Name: strings.TrimPrefix(raw, "query:")}, nil
	case strings.HasPrefix(raw, "query_base64_json:"):
		rest := strings.TrimPrefix(raw, "query_base64_json:")
		name, path, ok := strings.Cut(rest, ":")
		if !ok {
			return targetSpec{}, utils.Errorf("invalid query_base64_json target_ref: %s", raw)
		}
		return targetSpec{Raw: raw, Kind: "query_base64_json", Name: name, Path: normalizeJSONPath(path)}, nil
	case strings.HasPrefix(raw, "query_base64:"):
		return targetSpec{Raw: raw, Kind: "query_base64", Name: strings.TrimPrefix(raw, "query_base64:")}, nil
	case strings.HasPrefix(raw, "query_json:"):
		rest := strings.TrimPrefix(raw, "query_json:")
		name, path, ok := strings.Cut(rest, ":")
		if !ok {
			return targetSpec{}, utils.Errorf("invalid query_json target_ref: %s", raw)
		}
		return targetSpec{Raw: raw, Kind: "query_json", Name: name, Path: normalizeJSONPath(path)}, nil
	case strings.HasPrefix(raw, "cookie_base64_json:"):
		rest := strings.TrimPrefix(raw, "cookie_base64_json:")
		name, path, ok := strings.Cut(rest, ":")
		if !ok {
			return targetSpec{}, utils.Errorf("invalid cookie_base64_json target_ref: %s", raw)
		}
		return targetSpec{Raw: raw, Kind: "cookie_base64_json", Name: name, Path: normalizeJSONPath(path)}, nil
	case strings.HasPrefix(raw, "cookie_base64:"):
		return targetSpec{Raw: raw, Kind: "cookie_base64", Name: strings.TrimPrefix(raw, "cookie_base64:")}, nil
	case strings.HasPrefix(raw, "cookie_json:"):
		rest := strings.TrimPrefix(raw, "cookie_json:")
		name, path, ok := strings.Cut(rest, ":")
		if !ok {
			return targetSpec{}, utils.Errorf("invalid cookie_json target_ref: %s", raw)
		}
		return targetSpec{Raw: raw, Kind: "cookie_json", Name: name, Path: normalizeJSONPath(path)}, nil
	case strings.HasPrefix(raw, "cookie:"):
		return targetSpec{Raw: raw, Kind: "cookie", Name: strings.TrimPrefix(raw, "cookie:")}, nil
	case strings.HasPrefix(raw, "form_base64_json:"):
		rest := strings.TrimPrefix(raw, "form_base64_json:")
		name, path, ok := strings.Cut(rest, ":")
		if !ok {
			return targetSpec{}, utils.Errorf("invalid form_base64_json target_ref: %s", raw)
		}
		return targetSpec{Raw: raw, Kind: "form_base64_json", Name: name, Path: normalizeJSONPath(path)}, nil
	case strings.HasPrefix(raw, "form_base64:"):
		return targetSpec{Raw: raw, Kind: "form_base64", Name: strings.TrimPrefix(raw, "form_base64:")}, nil
	case strings.HasPrefix(raw, "form_json:"):
		rest := strings.TrimPrefix(raw, "form_json:")
		name, path, ok := strings.Cut(rest, ":")
		if !ok {
			return targetSpec{}, utils.Errorf("invalid form_json target_ref: %s", raw)
		}
		return targetSpec{Raw: raw, Kind: "form_json", Name: name, Path: normalizeJSONPath(path)}, nil
	case strings.HasPrefix(raw, "form:"):
		return targetSpec{Raw: raw, Kind: "form", Name: strings.TrimPrefix(raw, "form:")}, nil
	case strings.HasPrefix(raw, "json:"):
		return targetSpec{Raw: raw, Kind: "json", Path: normalizeJSONPath(strings.TrimPrefix(raw, "json:"))}, nil
	case strings.HasPrefix(raw, "xml:"):
		return targetSpec{Raw: raw, Kind: "xml", Path: normalizeXMLPath(strings.TrimPrefix(raw, "xml:"))}, nil
	case strings.HasPrefix(raw, "upload_filename:"):
		return targetSpec{Raw: raw, Kind: "upload_filename", Name: strings.TrimPrefix(raw, "upload_filename:")}, nil
	case strings.HasPrefix(raw, "upload_content:"):
		return targetSpec{Raw: raw, Kind: "upload_content", Name: strings.TrimPrefix(raw, "upload_content:")}, nil
	default:
		return targetSpec{}, utils.Errorf("unsupported target_ref: %s", raw)
	}
}

func buildTestPlan(userGoal string, profile *requestProfile, inventory []parameterInventoryItem) testPlan {
	goalLower := strings.ToLower(userGoal)
	credentialTargets := findTargetsByTag(inventory, "credential")
	identifierTargets := findTargetsByTag(inventory, "identifier_candidate")
	urlTargets := findTargetsByTag(inventory, "url_like")
	fileTargets := findTargetsByTag(inventory, "file_like")
	searchTargets := findTargetsByTag(inventory, "search_input")

	var scenarios []scenarioPlan
	addScenario := func(s scenarioPlan) {
		if len(s.Targets) == 0 && s.Scenario != "sensitive_info_exposure" {
			return
		}
		scenarios = append(scenarios, s)
	}

	if len(credentialTargets) > 0 || containsAny(goalLower, []string{"弱口令", "login", "password", "认证", "auth"}) {
		addScenario(scenarioPlan{
			Scenario: "weak_password",
			Priority: 10,
			Targets:  credentialTargets,
			Profiles: []string{"weakpass_basic"},
			Depth:    "medium",
			StopWhen: []string{"auth_state_changed", "budget_exhausted"},
		})
		addScenario(scenarioPlan{
			Scenario: "auth_bypass",
			Priority: 9,
			Targets:  append([]string{}, credentialTargets...),
			Profiles: []string{"auth_bypass_basic"},
			Depth:    "low",
			StopWhen: []string{"auth_state_changed", "budget_exhausted"},
		})
	}
	if len(identifierTargets) > 0 || containsAny(goalLower, []string{"idor", "枚举", "identifier", "id"}) {
		addScenario(scenarioPlan{
			Scenario: "identifier_enumeration",
			Priority: 8,
			Targets:  identifierTargets,
			Profiles: []string{"id_enum_numeric"},
			Depth:    "medium",
			StopWhen: []string{"record_count_changed", "budget_exhausted"},
		})
		addScenario(scenarioPlan{
			Scenario: "sqli",
			Priority: 7,
			Targets:  identifierTargets,
			Profiles: []string{"sqli_basic"},
			Depth:    "low",
			StopWhen: []string{"error_signature_detected", "time_delay_detected", "budget_exhausted"},
		})
	}
	if len(searchTargets) > 0 || containsAny(goalLower, []string{"xss", "sql", "注入", "sqli"}) {
		addScenario(scenarioPlan{
			Scenario: "xss",
			Priority: 7,
			Targets:  searchTargets,
			Profiles: []string{"xss_html"},
			Depth:    "medium",
			StopWhen: []string{"payload_reflected", "budget_exhausted"},
		})
		addScenario(scenarioPlan{
			Scenario: "sqli",
			Priority: 7,
			Targets:  searchTargets,
			Profiles: []string{"sqli_basic"},
			Depth:    "medium",
			StopWhen: []string{"error_signature_detected", "budget_exhausted"},
		})
	}
	if len(urlTargets) > 0 || containsAny(goalLower, []string{"ssrf", "redirect", "callback", "return"}) {
		addScenario(scenarioPlan{
			Scenario: "ssrf",
			Priority: 7,
			Targets:  urlTargets,
			Profiles: []string{"ssrf_basic"},
			Depth:    "low",
			StopWhen: []string{"status_changed", "redirect_changed", "budget_exhausted"},
		})
	}
	if len(fileTargets) > 0 || containsAny(goalLower, []string{"traversal", "path", "文件", "上传"}) {
		addScenario(scenarioPlan{
			Scenario: "traversal",
			Priority: 7,
			Targets:  fileTargets,
			Profiles: []string{"traversal_basic"},
			Depth:    "medium",
			StopWhen: []string{"sensitive_keyword_detected", "budget_exhausted"},
		})
	}
	addScenario(scenarioPlan{
		Scenario: "sensitive_info_exposure",
		Priority: 6,
		Targets:  []string{"path"},
		Profiles: []string{"debug_leak_probe"},
		Depth:    "low",
		StopWhen: []string{"sensitive_keyword_detected", "budget_exhausted"},
	})

	sort.SliceStable(scenarios, func(i, j int) bool {
		return scenarios[i].Priority > scenarios[j].Priority
	})

	return testPlan{
		UserGoal:        userGoal,
		ActiveScenarios: scenarios,
		RemainingBudget: budgetState{
			MaxRequests:    80,
			RemainingCount: 80,
			MaxBatches:     12,
			RemainingBatch: 12,
		},
	}
}

func fingerprintResponse(raw []byte, payload string, durationMs int64) responseFingerprint {
	fp := responseFingerprint{
		StatusCode:    lowhttp.GetStatusCodeFromResponse(raw),
		ContentType:   lowhttp.GetHTTPPacketHeader(raw, "Content-Type"),
		ContentLength: len(bodyFromRaw(raw)),
		HeaderDigest: map[string]any{
			"location":         lowhttp.GetHTTPPacketHeader(raw, "Location"),
			"set_cookie_count": len(lowhttp.GetHTTPPacketHeadersFull(raw)["Set-Cookie"]),
			"server_hint":      lowhttp.GetHTTPPacketHeader(raw, "Server"),
		},
		BodyDigest: map[string]any{},
		Timing: map[string]any{
			"duration_ms": durationMs,
		},
	}
	body := string(bodyFromRaw(raw))
	title := titleRegexp.FindStringSubmatch(body)
	errorHits := matchKeywords(body, errorSignatureKeywords)
	sensitiveHits := matchKeywords(body, sensitiveInfoKeywords)
	reflected := ""
	if payload != "" && strings.Contains(body, payload) {
		reflected = payload
	}
	fp.BodyDigest["title"] = ""
	if len(title) > 1 {
		fp.BodyDigest["title"] = strings.TrimSpace(title[1])
	}
	fp.BodyDigest["preview"] = utils.ShrinkTextBlock(strings.ReplaceAll(body, "\n", " "), 220)
	fp.BodyDigest["keyword_hits"] = matchKeywords(body, authSuccessKeywords)
	fp.BodyDigest["error_signatures"] = errorHits
	fp.BodyDigest["reflection_hits"] = []string{}
	if reflected != "" {
		fp.BodyDigest["reflection_hits"] = []string{reflected}
	}
	fp.BodyDigest["sensitive_hits"] = sensitiveHits
	return fp
}

func diffResponse(baseline responseFingerprint, current responseFingerprint, payload string) ([]string, string) {
	signals := make([]string, 0)
	if current.StatusCode != 0 && baseline.StatusCode != 0 && baseline.StatusCode != current.StatusCode {
		signals = append(signals, "status_changed")
	}
	if baseline.ContentLength > 0 {
		delta := absInt(current.ContentLength - baseline.ContentLength)
		if delta >= 32 && delta*100/maxInt(baseline.ContentLength, 1) >= 20 {
			signals = append(signals, "length_delta_large")
		}
	}
	if duration, ok := current.Timing["duration_ms"].(int64); ok {
		if baseDuration, okBase := baseline.Timing["duration_ms"].(int64); okBase {
			if duration >= 3000 || (baseDuration > 0 && duration > baseDuration*3 && duration-baseDuration > 1200) {
				signals = append(signals, "time_delay_detected")
			}
		}
	}
	if reflected, _ := current.BodyDigest["reflection_hits"].([]string); len(reflected) > 0 && payload != "" {
		signals = append(signals, "payload_reflected")
	}
	if hits, _ := current.BodyDigest["error_signatures"].([]string); len(hits) > 0 {
		signals = append(signals, "error_signature_detected")
	}
	if hits, _ := current.BodyDigest["sensitive_hits"].([]string); len(hits) > 0 {
		signals = append(signals, "sensitive_keyword_detected")
	}
	if current.HeaderDigest["location"] != baseline.HeaderDigest["location"] && current.HeaderDigest["location"] != "" {
		signals = append(signals, "redirect_changed")
	}
	if current.HeaderDigest["set_cookie_count"] != baseline.HeaderDigest["set_cookie_count"] {
		signals = append(signals, "set_cookie_changed")
	}
	if containsAny(strings.ToLower(utils.InterfaceToString(current.BodyDigest["preview"])), []string{"welcome", "logout", "dashboard", "profile"}) &&
		!containsAny(strings.ToLower(utils.InterfaceToString(baseline.BodyDigest["preview"])), []string{"welcome", "logout", "dashboard", "profile"}) {
		signals = append(signals, "auth_state_changed")
	}

	if len(signals) == 0 {
		return signals, "no obvious abnormal signal"
	}
	return dedupeStrings(signals), strings.Join(signals, ", ")
}

func getPreparedMutation(loop *reactloops.ReActLoop) mutationSpec {
	if ret, ok := loop.GetVariable(stateLastMutation).(mutationSpec); ok {
		return ret
	}
	return mutationSpec{}
}

func getParameterInventory(loop *reactloops.ReActLoop) []parameterInventoryItem {
	if ret, ok := loop.GetVariable(stateParameterInventory).([]parameterInventoryItem); ok {
		return ret
	}
	return nil
}

func getTestPlan(loop *reactloops.ReActLoop) testPlan {
	if ret, ok := loop.GetVariable(stateTestPlan).(testPlan); ok {
		return ret
	}
	return testPlan{}
}

func saveTestPlan(loop *reactloops.ReActLoop, plan testPlan) {
	loop.Set(stateTestPlan, plan)
}

func getBaselineFingerprint(loop *reactloops.ReActLoop) responseFingerprint {
	if ret, ok := loop.GetVariable(stateBaselineFingerprint).(responseFingerprint); ok {
		return ret
	}
	return responseFingerprint{}
}

func getAttemptHistory(loop *reactloops.ReActLoop) []attemptRecord {
	if ret, ok := loop.GetVariable(stateAttemptHistory).([]attemptRecord); ok {
		return ret
	}
	return nil
}

func getAnomalyCandidates(loop *reactloops.ReActLoop) []anomalyCandidate {
	if ret, ok := loop.GetVariable(stateAnomalyCandidates).([]anomalyCandidate); ok {
		return ret
	}
	return nil
}

func getConfirmedFindings(loop *reactloops.ReActLoop) []confirmedFinding {
	if ret, ok := loop.GetVariable(stateConfirmedFindings).([]confirmedFinding); ok {
		return ret
	}
	return nil
}

func getCoverageMap(loop *reactloops.ReActLoop) map[string][]string {
	if ret, ok := loop.GetVariable(stateCoverageMap).(map[string][]string); ok {
		return ret
	}
	return map[string][]string{}
}

func applyPreparedMutation(loop *reactloops.ReActLoop, spec mutationSpec, scenario string, maxRequests int) ([]batchResult, error) {
	if spec.TargetRef == "" {
		return nil, utils.Error("no prepared mutation found")
	}
	current, err := buildFreshFuzzRequest(loop)
	if err != nil {
		return nil, err
	}
	mutated, payloads, err := applyMutation(current, spec.TargetRef, spec.MutationMode, spec.Payloads, spec.EncodingPolicy, spec.DisableAutoEncode)
	if err != nil {
		return nil, err
	}
	return executeMutatedBatch(loop, mutated, payloads, scenario, spec.TargetRef, maxRequests, "")
}

func executeProfileBatch(loop *reactloops.ReActLoop, scenario string, targetRefs []string, profile string, maxRequests int, reason string) ([]batchResult, error) {
	payloads := payloadsForProfile(profile)
	if len(payloads) == 0 {
		return nil, utils.Errorf("profile not found: %s", profile)
	}
	var all []batchResult
	for _, targetRef := range targetRefs {
		current, err := buildFreshFuzzRequest(loop)
		if err != nil {
			return nil, err
		}
		mutated, effective, err := applyMutation(current, targetRef, "replace", payloads, "preserve", false)
		if err != nil {
			return nil, err
		}
		limit := maxRequests
		if limit > 0 {
			limit -= len(all)
		}
		results, execErr := executeMutatedBatch(loop, mutated, effective, scenario, targetRef, limit, profile)
		if execErr != nil {
			return all, execErr
		}
		all = append(all, results...)
		if maxRequests > 0 && len(all) >= maxRequests {
			break
		}
	}
	updateNextActions(loop, scenario, reason)
	return all, nil
}

func executeMutatedBatch(loop *reactloops.ReActLoop, mutated mutate.FuzzHTTPRequestIf, payloads []string, scenario string, targetRef string, maxRequests int, profile string) ([]batchResult, error) {
	isHTTPS := loop.Get(stateIsHTTPS) == "true"
	resultCh, err := mutated.Exec(mutate.WithPoolOpt_Https(isHTTPS))
	if err != nil {
		return nil, utils.Wrap(err, "execute fuzz batch")
	}

	plan := getTestPlan(loop)
	if plan.RemainingBudget.RemainingBatch > 0 {
		plan.RemainingBudget.RemainingBatch--
	}
	saveTestPlan(loop, plan)

	batchID := newID("b")
	baseline := getBaselineFingerprint(loop)
	var results []batchResult
	anomalies := getAnomalyCandidates(loop)
	interesting := getInterestingResponses(loop)
	coverage := getCoverageMap(loop)
	anomalyCount := 0
	requestCount := 0

	for result := range resultCh {
		if maxRequests > 0 && requestCount >= maxRequests {
			continue
		}
		requestCount++
		fp := fingerprintResponse(result.ResponseRaw, firstPayload(result, payloads), result.DurationMs)
		signals, summary := diffResponse(baseline, fp, firstPayload(result, payloads))
		item := batchResult{
			BatchID:     batchID,
			Scenario:    scenario,
			TargetRef:   targetRef,
			Payload:     firstPayload(result, payloads),
			Signals:     signals,
			Summary:     summary,
			RequestRaw:  string(result.RequestRaw),
			ResponseRaw: string(result.ResponseRaw),
			Fingerprint: fp,
			DurationMs:  result.DurationMs,
		}
		if result.Error != nil {
			item.Error = result.Error.Error()
			item.Summary = result.Error.Error()
		}
		if len(signals) > 0 {
			anomaly := anomalyCandidate{
				CandidateID: newID("a"),
				Scenario:    scenario,
				TargetRef:   targetRef,
				Payload:     item.Payload,
				Signals:     signals,
				Confidence:  candidateConfidence(signals),
				NeedsRetest: true,
				Summary:     summary,
			}
			item.CandidateID = anomaly.CandidateID
			anomalies = append(anomalies, anomaly)
			anomalyCount++
			interesting = append(interesting, item)
		}
		results = append(results, item)
		coverage[targetRef] = dedupeStrings(append(coverage[targetRef], scenario))
		plan.RemainingBudget.RemainingCount = maxInt(plan.RemainingBudget.RemainingCount-1, 0)
		loop.Set(stateLastRequest, item.RequestRaw)
		loop.Set(stateLastResponse, item.ResponseRaw)
	}

	attempts := getAttemptHistory(loop)
	attempts = append(attempts, attemptRecord{
		BatchID:      batchID,
		Scenario:     scenario,
		TargetRefs:   []string{targetRef},
		Profile:      profile,
		RequestCount: requestCount,
		AnomalyCount: anomalyCount,
		Summary:      fmt.Sprintf("%s on %s: %d requests, %d anomalies", scenario, targetRef, requestCount, anomalyCount),
	})

	saveTestPlan(loop, plan)
	loop.Set(stateAttemptHistory, attempts)
	loop.Set(stateAnomalyCandidates, anomalies)
	loop.Set(stateInterestingResponses, tailBatchResults(interesting, 12))
	loop.Set(stateCoverageMap, coverage)
	loop.Set(stateLastBatchResults, tailBatchResults(results, 12))
	return results, nil
}

func buildFreshFuzzRequest(loop *reactloops.ReActLoop) (mutate.FuzzHTTPRequestIf, error) {
	raw := loop.Get(stateOriginalRequest)
	if raw == "" {
		return nil, utils.Error("no original_request found")
	}
	return mutate.NewFuzzHTTPRequest([]byte(raw), mutate.OptHTTPS(loop.Get(stateIsHTTPS) == "true"))
}

func applyMutation(current mutate.FuzzHTTPRequestIf, targetRef, mode string, payloads []string, encodingPolicy string, disableAutoEncode bool) (mutate.FuzzHTTPRequestIf, []string, error) {
	spec, err := parseTargetRef(targetRef)
	if err != nil {
		return nil, nil, err
	}
	baseValue := resolveTargetCurrentValue(current.FirstFuzzHTTPRequest(), spec)
	finalPayloads := mutatePayloads(baseValue, payloads, mode, encodingPolicy)
	if len(finalPayloads) == 0 {
		return nil, nil, utils.Errorf("no payloads generated for %s", targetRef)
	}
	if disableAutoEncode {
		current = current.DisableAutoEncode(true)
	}
	switch spec.Kind {
	case "method":
		return current.FuzzMethod(finalPayloads...), finalPayloads, nil
	case "path":
		if mode == "append" {
			return current.FuzzPathAppend(finalPayloads...), finalPayloads, nil
		}
		return current.FuzzPath(finalPayloads...), finalPayloads, nil
	case "path_block":
		var paths []string
		for _, payload := range finalPayloads {
			paths = append(paths, replacePathBlock(current.FirstFuzzHTTPRequest().GetPathWithoutQuery(), spec.BlockIndex, payload))
		}
		return current.FuzzPath(paths...), finalPayloads, nil
	case "header":
		return current.FuzzHTTPHeader(spec.Name, finalPayloads), finalPayloads, nil
	case "query_raw":
		return current.FuzzGetParamsRaw(finalPayloads...), finalPayloads, nil
	case "query":
		return current.FuzzGetParams(spec.Name, finalPayloads), finalPayloads, nil
	case "query_base64":
		return current.FuzzGetBase64Params(spec.Name, finalPayloads), finalPayloads, nil
	case "query_json":
		return current.FuzzGetJsonPathParams(spec.Name, trimJSONPathPrefix(spec.Path), finalPayloads), finalPayloads, nil
	case "query_base64_json":
		return current.FuzzGetBase64JsonPath(spec.Name, trimJSONPathPrefix(spec.Path), finalPayloads), finalPayloads, nil
	case "form":
		return current.FuzzPostParams(spec.Name, finalPayloads), finalPayloads, nil
	case "form_base64":
		return current.FuzzPostBase64Params(spec.Name, finalPayloads), finalPayloads, nil
	case "form_json":
		return current.FuzzPostJsonPathParams(spec.Name, trimJSONPathPrefix(spec.Path), finalPayloads), finalPayloads, nil
	case "form_base64_json":
		return current.FuzzPostBase64JsonPath(spec.Name, trimJSONPathPrefix(spec.Path), finalPayloads), finalPayloads, nil
	case "json":
		key := jsonFieldKey(spec.Path)
		if key == "" {
			key = trimJSONPathPrefix(spec.Path)
		}
		if strings.Contains(trimJSONPathPrefix(spec.Path), ".") {
			key = strings.Split(trimJSONPathPrefix(spec.Path), ".")[0]
			return current.FuzzPostJsonPathParams(key, trimJSONPathPrefix(spec.Path), finalPayloads), finalPayloads, nil
		}
		return current.FuzzPostJsonParams(key, finalPayloads), finalPayloads, nil
	case "xml":
		return current.FuzzPostXMLParams(spec.Path, finalPayloads), finalPayloads, nil
	case "cookie":
		return current.FuzzCookie(spec.Name, finalPayloads), finalPayloads, nil
	case "cookie_base64":
		return current.FuzzCookieBase64(spec.Name, finalPayloads), finalPayloads, nil
	case "cookie_json":
		return current.FuzzCookieJsonPath(spec.Name, trimJSONPathPrefix(spec.Path), finalPayloads), finalPayloads, nil
	case "cookie_base64_json":
		return current.FuzzCookieBase64JsonPath(spec.Name, trimJSONPathPrefix(spec.Path), finalPayloads), finalPayloads, nil
	case "body_raw":
		return current.FuzzPostRaw(finalPayloads...), finalPayloads, nil
	case "upload_filename":
		return current.FuzzUploadFileName(spec.Name, finalPayloads), finalPayloads, nil
	case "upload_content":
		if len(finalPayloads) == 1 {
			return current.FuzzUploadFile(spec.Name, spec.Name, []byte(finalPayloads[0])), finalPayloads, nil
		}
		next := current
		for _, payload := range finalPayloads {
			next = next.FuzzUploadFile(spec.Name, spec.Name, []byte(payload))
		}
		return next, finalPayloads, nil
	default:
		return nil, nil, utils.Errorf("unsupported mutation target: %s", targetRef)
	}
}

func resolveTargetCurrentValue(fuzzReq *mutate.FuzzHTTPRequest, spec targetSpec) string {
	if fuzzReq == nil {
		return ""
	}
	switch spec.Kind {
	case "method":
		return fuzzReq.GetMethod()
	case "path":
		return fuzzReq.GetPathWithoutQuery()
	case "path_block":
		return currentPathBlockValue(fuzzReq.GetPathWithoutQuery(), spec.BlockIndex)
	case "header":
		return fuzzReq.GetHeader(spec.Name)
	case "query":
		return fuzzReq.GetQueryValue(spec.Name)
	case "query_raw":
		return fuzzReq.GetQueryRaw()
	case "form":
		return fuzzReq.GetPostQueryValue(spec.Name)
	case "body_raw":
		return string(fuzzReq.GetBody())
	default:
		for _, item := range buildParameterInventory(fuzzReq) {
			if item.TargetRef == spec.Raw {
				return item.ValuePreview
			}
		}
	}
	return ""
}

func mutatePayloads(baseValue string, payloads []string, mode string, encodingPolicy string) []string {
	var final []string
	for _, payload := range payloads {
		if payload == "" {
			continue
		}
		switch mode {
		case "prefix":
			payload = payload + baseValue
		case "suffix", "append":
			payload = baseValue + payload
		case "base64_wrap":
			payload = base64.StdEncoding.EncodeToString([]byte(payload))
		default:
		}
		payload = applyEncodingPolicy(payload, encodingPolicy)
		final = append(final, payload)
	}
	return dedupeStrings(final)
}

func applyEncodingPolicy(payload string, policy string) string {
	switch policy {
	case "force_url":
		return url.QueryEscape(payload)
	case "force_base64":
		return base64.StdEncoding.EncodeToString([]byte(payload))
	case "no_encode", "inherit", "preserve", "":
		return payload
	default:
		return payload
	}
}

func extractHighValueTargets(items []parameterInventoryItem) []parameterInventoryItem {
	var ret []parameterInventoryItem
	for _, item := range items {
		if len(item.HighValueTags) > 0 {
			ret = append(ret, item)
		}
	}
	sort.SliceStable(ret, func(i, j int) bool {
		return ret[i].TargetRef < ret[j].TargetRef
	})
	return ret
}

func classifyHighValueTags(name, targetRef, value string) []string {
	nameLower := strings.ToLower(name)
	targetLower := strings.ToLower(targetRef)
	var tags []string
	if containsAny(nameLower+" "+targetLower, credentialFieldHints) {
		tags = append(tags, "credential")
	}
	if containsAny(nameLower+" "+targetLower, identifierFieldHints) {
		tags = append(tags, "identifier_candidate")
	}
	if containsAny(nameLower+" "+targetLower, urlFieldHints) {
		tags = append(tags, "url_like")
	}
	if containsAny(nameLower+" "+targetLower, fileFieldHints) {
		tags = append(tags, "file_like")
	}
	if containsAny(nameLower+" "+targetLower, searchFieldHints) {
		tags = append(tags, "search_input")
	}
	if containsAny(strings.ToLower(value), []string{"eyj", "jwt", "bearer"}) {
		tags = append(tags, "token_like")
	}
	return dedupeStrings(tags)
}

func guessBusiness(profile *requestProfile, fuzzReq *mutate.FuzzHTTPRequest) string {
	nameSpace := strings.ToLower(profile.Path + " " + strings.Join(fuzzReq.GetQueryKeys(), " ") + " " + strings.Join(fuzzReq.GetPostQueryKeys(), " "))
	switch {
	case containsAny(nameSpace, []string{"login", "signin", "auth", "password", "username"}):
		return "login"
	case containsAny(nameSpace, []string{"search", "query", "filter", "keyword"}):
		return "search"
	case containsAny(nameSpace, []string{"upload", "file", "avatar"}):
		return "upload"
	case containsAny(nameSpace, []string{"detail", "view", "order", "doc", "user", "id"}):
		return "detail"
	default:
		return "generic"
	}
}

func guessRiskHints(profile *requestProfile, fuzzReq *mutate.FuzzHTTPRequest) []string {
	var hints []string
	if profile.IsJSONBody {
		hints = append(hints, "json")
	}
	if profile.IsXMLBody {
		hints = append(hints, "xml")
	}
	if profile.IsMultipart {
		hints = append(hints, "multipart")
	}
	if profile.HasAuthorization || profile.HasCookie {
		hints = append(hints, "auth")
	}
	inventory := buildParameterInventory(fuzzReq)
	for _, item := range inventory {
		hints = append(hints, item.HighValueTags...)
	}
	return dedupeStrings(hints)
}

func guessRiskHintsByPath(path string) []string {
	path = strings.ToLower(path)
	var hints []string
	if containsAny(path, []string{"admin", "debug", "actuator", "swagger"}) {
		hints = append(hints, "sensitive_surface")
	}
	if containsAny(path, []string{"login", "signin"}) {
		hints = append(hints, "credential_input")
	}
	return hints
}

func supportedMutationModes(targetRef string) []string {
	switch {
	case targetRef == "method":
		return []string{"replace"}
	case strings.HasPrefix(targetRef, "upload_content:"):
		return []string{"replace"}
	case targetRef == "body_raw" || targetRef == "query_raw":
		return []string{"replace", "append", "prefix", "suffix", "raw_replace"}
	default:
		return []string{"replace", "append", "prefix", "suffix"}
	}
}

func safePreview(targetRef, name, value string) string {
	if containsAny(strings.ToLower(targetRef+" "+name), []string{"password", "passwd", "secret", "authorization", "token"}) {
		if value == "" {
			return ""
		}
		return "***"
	}
	return utils.ShrinkTextBlock(value, 64)
}

func guessValueType(v any) string {
	if v == nil {
		return "null"
	}
	switch ret := v.(type) {
	case bool:
		return "bool"
	case int, int32, int64, uint, uint32, uint64, float32, float64:
		return "number"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		if _, err := strconv.ParseFloat(ret, 64); err == nil {
			return "number"
		}
		if ret == "true" || ret == "false" {
			return "bool"
		}
		return "string"
	default:
		return "string"
	}
}

func guessEncoding(targetRef string) string {
	switch {
	case strings.Contains(targetRef, "_base64"):
		return "base64"
	case strings.HasPrefix(targetRef, "json:") || strings.Contains(targetRef, "_json:"):
		return "json"
	case strings.HasPrefix(targetRef, "xml:"):
		return "xml"
	default:
		return "plain"
	}
}

func normalizeJSONPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "$.")
	path = strings.TrimPrefix(path, "$")
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return "$"
	}
	return "$." + path
}

func trimJSONPathPrefix(path string) string {
	path = normalizeJSONPath(path)
	path = strings.TrimPrefix(path, "$.")
	if path == "$" {
		return ""
	}
	return path
}

func normalizeXMLPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

func normalizePathString(path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "$.") {
		return path
	}
	return strings.TrimSpace(path)
}

func jsonFieldKey(path string) string {
	path = trimJSONPathPrefix(path)
	if path == "" {
		return ""
	}
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func pathSegments(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(strings.Trim(path, "/"), "/")
}

func currentPathBlockValue(path string, index int) string {
	parts := pathSegments(path)
	if index <= 0 || index > len(parts) {
		return ""
	}
	return parts[index-1]
}

func replacePathBlock(path string, index int, payload string) string {
	parts := pathSegments(path)
	if index <= 0 || index > len(parts) {
		return path
	}
	parts[index-1] = payload
	return "/" + strings.Join(parts, "/")
}

func newID(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, utils.TimestampMs())
}

func findTargetsByTag(items []parameterInventoryItem, tag string) []string {
	var refs []string
	for _, item := range items {
		if containsAny(strings.Join(item.HighValueTags, " "), []string{tag}) {
			refs = append(refs, item.TargetRef)
		}
	}
	return dedupeStrings(refs)
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]struct{})
	var ret []string
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		ret = append(ret, item)
	}
	return ret
}

func dedupeInventory(items []parameterInventoryItem) []parameterInventoryItem {
	seen := make(map[string]struct{})
	var ret []parameterInventoryItem
	for _, item := range items {
		if item.TargetRef == "" {
			continue
		}
		if _, ok := seen[item.TargetRef]; ok {
			continue
		}
		seen[item.TargetRef] = struct{}{}
		ret = append(ret, item)
	}
	return ret
}

func containsAny(target string, keywords []string) bool {
	target = strings.ToLower(target)
	for _, keyword := range keywords {
		if strings.Contains(target, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func suggestNextActions(plan testPlan) []string {
	if len(plan.ActiveScenarios) == 0 {
		return []string{"load_http_request", "inspect_request_surface"}
	}
	first := plan.ActiveScenarios[0]
	return []string{
		"inspect_request_surface",
		fmt.Sprintf("run_%s_test", first.Scenario),
		"commit_finding",
	}
}

func updateNextActions(loop *reactloops.ReActLoop, scenario, reason string) {
	candidates := getAnomalyCandidates(loop)
	if len(candidates) > 0 {
		loop.Set(stateNextRecommendedActions, []string{
			"retest anomaly with execute_test_batch",
			"commit_finding",
		})
		return
	}
	if scenario != "" {
		loop.Set(stateNextRecommendedActions, []string{
			fmt.Sprintf("try another %s target", scenario),
			"inspect_request_surface",
		})
		return
	}
	loop.Set(stateNextRecommendedActions, []string{
		"inspect_request_surface",
		reason,
	})
}

func getInterestingResponses(loop *reactloops.ReActLoop) []batchResult {
	if ret, ok := loop.GetVariable(stateInterestingResponses).([]batchResult); ok {
		return ret
	}
	return nil
}

func tailBatchResults(items []batchResult, max int) []batchResult {
	if len(items) <= max {
		return items
	}
	return append([]batchResult(nil), items[len(items)-max:]...)
}

func firstPayload(result *mutate.HttpResult, fallback []string) string {
	if result != nil && len(result.Payloads) > 0 {
		return result.Payloads[0]
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return ""
}

func candidateConfidence(signals []string) string {
	switch {
	case containsAny(strings.Join(signals, " "), []string{"auth_state_changed", "time_delay_detected", "sensitive_keyword_detected"}):
		return "high"
	case len(signals) >= 2:
		return "medium"
	default:
		return "low"
	}
}

func bodyFromRaw(raw []byte) []byte {
	_, body := lowhttp.SplitHTTPPacketFast(raw)
	return body
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var (
	titleRegexp            = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	credentialFieldHints   = []string{"username", "user", "account", "password", "passwd", "pwd", "token", "auth", "session"}
	identifierFieldHints   = []string{"id", "uid", "userid", "orderid", "docid", "itemid", "number", "code"}
	urlFieldHints          = []string{"url", "redirect", "callback", "return", "next", "target"}
	fileFieldHints         = []string{"file", "path", "filename", "dir", "filepath"}
	searchFieldHints       = []string{"q", "query", "search", "keyword", "name", "title", "content"}
	errorSignatureKeywords = []string{"sql syntax", "mysql", "psql", "postgres", "sqlite", "sqlstate", "exception", "traceback", "stack trace", "syntax error", "warning:"}
	sensitiveInfoKeywords  = []string{"swagger", "openapi", "password", "secret", "token", "access_key", "private key", "/etc/passwd", ".env", "set-cookie"}
	authSuccessKeywords    = []string{"welcome", "logout", "dashboard", "profile", "token", "set-cookie"}
)

func matchKeywords(text string, keywords []string) []string {
	text = strings.ToLower(text)
	var hits []string
	for _, keyword := range keywords {
		if strings.Contains(text, strings.ToLower(keyword)) {
			hits = append(hits, keyword)
		}
	}
	return dedupeStrings(hits)
}

func marshalSummary(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}
