package loop_http_fuzztest

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type generateRiskSpec struct {
	Target       string
	Title        string
	TitleVerbose string
	RiskType     string
	Severity     string
	Description  string
	Details      string
	Payload      string
}

var generateRiskAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"generate_risk",
		"Generate and save one or more Yakit risk records after the HTTP fuzzing evidence is strong enough. "+
			"Use this only when the current test has clear vulnerability signals or defensible potential risks.",
		[]aitool.ToolOption{
			aitool.WithStructArrayParam("risks",
				[]aitool.PropertyOption{aitool.WithParam_Description("Batch risk records to generate. Use this when multiple independent risks should be saved in one action. If this array is provided, the top-level single-risk fields are ignored.")},
				[]aitool.PropertyOption{},
				aitool.WithStringParam("target", aitool.WithParam_Description("Risk target URL/IP/host. If empty, infer from representative request or current effective request.")),
				aitool.WithStringParam("title", aitool.WithParam_Description("Risk title, e.g. '订单接口疑似 IDOR 越权读取'."), aitool.WithParam_Required(true)),
				aitool.WithStringParam("title_verbose", aitool.WithParam_Description("Human-readable detailed title. If empty, title is reused.")),
				aitool.WithStringParam("risk_type", aitool.WithParam_Description("Risk type, e.g. sqli, xss, path-traversal, ssrf, unauth-access, auth-bypass, privilege-escalation, info-exposure, weak-pass, logic."), aitool.WithParam_Required(true)),
				aitool.WithStringParam("severity", aitool.WithParam_Description("Risk severity: critical, high, warning/medium, low, info."), aitool.WithParam_EnumString("critical", "high", "warning", "medium", "low", "info"), aitool.WithParam_Required(true)),
				aitool.WithStringParam("description", aitool.WithParam_Description("Chinese risk description: evidence, impact, reproduction summary, and testing boundary."), aitool.WithParam_Required(true)),
				aitool.WithStringParam("details", aitool.WithParam_Description("Risk details as JSON object string when possible. Plain text is accepted and stored as summary.")),
				aitool.WithStringParam("payload", aitool.WithParam_Description("Representative payload or mutated value that triggered the signal.")),
			),
			aitool.WithStringParam("target", aitool.WithParam_Description("Single-risk target URL/IP/host. If empty, infer from representative request or current effective request.")),
			aitool.WithStringParam("title", aitool.WithParam_Description("Single-risk title. Required only when risks array is not used.")),
			aitool.WithStringParam("title_verbose", aitool.WithParam_Description("Single-risk detailed title. If empty, title is reused.")),
			aitool.WithStringParam("risk_type", aitool.WithParam_Description("Single-risk type. Required only when risks array is not used.")),
			aitool.WithStringParam("severity", aitool.WithParam_Description("Single-risk severity. Required only when risks array is not used."), aitool.WithParam_EnumString("critical", "high", "warning", "medium", "low", "info")),
			aitool.WithStringParam("description", aitool.WithParam_Description("Single-risk Chinese description. Required only when risks array is not used.")),
			aitool.WithStringParam("details", aitool.WithParam_Description("Risk details as JSON object string when possible. Plain text is accepted and stored as summary.")),
			aitool.WithStringParam("payload", aitool.WithParam_Description("Representative payload or mutated value that triggered the signal.")),
		},
		[]*reactloops.LoopStreamField{
			{FieldName: "target", AINodeId: "thought"},
			{FieldName: "title", AINodeId: "thought"},
			{FieldName: "risk_type", AINodeId: "thought"},
			{FieldName: "severity", AINodeId: "thought"},
			{FieldName: "description", AINodeId: "thought"},
			{FieldName: "payload", AINodeId: "thought"},
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			specs := collectGenerateRiskSpecs(action)
			if len(specs) == 0 {
				return fmt.Errorf("generate_risk requires either risks array or single-risk fields")
			}
			for idx, spec := range specs {
				if err := validateGenerateRiskSpec(l, spec, idx); err != nil {
					return err
				}
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			specs := collectGenerateRiskSpecs(action)
			riskIDs := make([]string, 0, len(specs))
			summaries := make([]string, 0, len(specs))
			for idx, spec := range specs {
				riskID, summary, err := saveGeneratedRisk(loop, spec)
				if err != nil {
					operator.Fail(fmt.Errorf("generate_risk: save risk #%d failed: %w", idx+1, err))
					return
				}
				riskIDs = append(riskIDs, riskID)
				summaries = append(summaries, summary)
			}

			summary := fmt.Sprintf("已生成 %d 个 Risk:\n%s", len(summaries), strings.Join(summaries, "\n"))
			if len(riskIDs) == 1 {
				loop.Set("generated_risk_id", riskIDs[0])
			} else {
				loop.Delete("generated_risk_id")
			}
			loop.Set("generated_risk_ids", strings.Join(riskIDs, ","))
			loop.Set("generated_risk_summary", summary)
			recordLoopHTTPFuzzMetaAction(loop, "generate_risk", fmt.Sprintf("count=%d; risk_ids=%s", len(riskIDs), strings.Join(riskIDs, ",")), summary)
			r.AddToTimeline("generate_risk", summary)
			operator.Feedback(summary)
			operator.Continue()
		},
	)
}

func collectGenerateRiskSpecs(action *aicommon.Action) []generateRiskSpec {
	if action == nil {
		return nil
	}
	riskParams := action.GetInvokeParamsArray("risks")
	if len(riskParams) > 0 {
		specs := make([]generateRiskSpec, 0, len(riskParams))
		for _, params := range riskParams {
			specs = append(specs, generateRiskSpecFromParams(params))
		}
		return specs
	}
	return []generateRiskSpec{{
		Target:       strings.TrimSpace(action.GetString("target")),
		Title:        strings.TrimSpace(action.GetString("title")),
		TitleVerbose: strings.TrimSpace(action.GetString("title_verbose")),
		RiskType:     strings.TrimSpace(action.GetString("risk_type")),
		Severity:     strings.TrimSpace(action.GetString("severity")),
		Description:  strings.TrimSpace(action.GetString("description")),
		Details:      strings.TrimSpace(action.GetString("details")),
		Payload:      strings.TrimSpace(action.GetString("payload")),
	}}
}

func generateRiskSpecFromParams(params aitool.InvokeParams) generateRiskSpec {
	return generateRiskSpec{
		Target:       getGenerateRiskParamString(params, "target"),
		Title:        getGenerateRiskParamString(params, "title"),
		TitleVerbose: getGenerateRiskParamString(params, "title_verbose"),
		RiskType:     getGenerateRiskParamString(params, "risk_type"),
		Severity:     getGenerateRiskParamString(params, "severity"),
		Description:  getGenerateRiskParamString(params, "description"),
		Details:      getGenerateRiskParamString(params, "details"),
		Payload:      getGenerateRiskParamString(params, "payload"),
	}
}

func getGenerateRiskParamString(params aitool.InvokeParams, key string) string {
	if params == nil {
		return ""
	}
	raw, ok := params[key]
	if !ok || utils.IsNil(raw) {
		return ""
	}
	if s, ok := raw.(string); ok {
		return strings.TrimSpace(s)
	}
	if data, err := json.Marshal(raw); err == nil {
		return strings.TrimSpace(string(data))
	}
	return strings.TrimSpace(utils.InterfaceToString(raw))
}

func validateGenerateRiskSpec(loop *reactloops.ReActLoop, spec generateRiskSpec, index int) error {
	prefix := "single risk"
	if index >= 0 {
		prefix = fmt.Sprintf("risk #%d", index+1)
	}
	if strings.TrimSpace(spec.Title) == "" {
		return fmt.Errorf("%s: title is required", prefix)
	}
	if strings.TrimSpace(spec.RiskType) == "" {
		return fmt.Errorf("%s: risk_type is required", prefix)
	}
	if !isValidGenerateRiskSeverity(spec.Severity) {
		return fmt.Errorf("%s: severity must be one of: critical, high, warning, medium, low, info", prefix)
	}
	if strings.TrimSpace(spec.Description) == "" {
		return fmt.Errorf("%s: description is required", prefix)
	}
	if inferGenerateRiskTarget(loop, spec.Target) == "" {
		return fmt.Errorf("%s: target is required when no representative/current HTTP request URL can be inferred", prefix)
	}
	return nil
}

func saveGeneratedRisk(loop *reactloops.ReActLoop, spec generateRiskSpec) (string, string, error) {
	target := inferGenerateRiskTarget(loop, spec.Target)
	title := strings.TrimSpace(spec.Title)
	titleVerbose := strings.TrimSpace(spec.TitleVerbose)
	if titleVerbose == "" {
		titleVerbose = title
	}
	riskType := strings.TrimSpace(spec.RiskType)
	severity := strings.TrimSpace(spec.Severity)
	description := strings.TrimSpace(spec.Description)
	details := buildGenerateRiskDetails(spec.Details, loop)
	payload := strings.TrimSpace(spec.Payload)
	requestRaw := ""
	responseRaw := ""
	if loop != nil {
		requestRaw = firstNonEmptyString(
			strings.TrimSpace(loop.Get("representative_request")),
			strings.TrimSpace(getCurrentRequestRaw(loop)),
		)
		responseRaw = strings.TrimSpace(loop.Get("representative_response"))
	}

	opts := []yakit.RiskParamsOpt{
		yakit.WithRiskParam_Title(title),
		yakit.WithRiskParam_TitleVerbose(titleVerbose),
		yakit.WithRiskParam_RiskType(riskType),
		yakit.WithRiskParam_Severity(severity),
		yakit.WithRiskParam_Description(description),
		yakit.WithRiskParam_Details(details),
		yakit.WithRiskParam_FromScript(LoopHTTPFuzztestName),
	}
	if payload != "" {
		opts = append(opts, yakit.WithRiskParam_Payload(payload))
	}
	if requestRaw != "" {
		opts = append(opts, yakit.WithRiskParam_Request(requestRaw))
	}
	if responseRaw != "" {
		opts = append(opts, yakit.WithRiskParam_Response(responseRaw))
	}

	risk, err := yakit.NewRisk(target, opts...)
	if err != nil {
		return "", "", err
	}
	if risk == nil {
		return "", "", fmt.Errorf("saved risk is nil")
	}

	riskID := fmt.Sprintf("%d", risk.ID)
	summary := fmt.Sprintf("- id=%d severity=%s type=%s target=%s title=%s", risk.ID, risk.Severity, risk.RiskType, target, title)
	return riskID, summary, nil
}

func isValidGenerateRiskSeverity(severity string) bool {
	switch strings.TrimSpace(strings.ToLower(severity)) {
	case "critical", "high", "warning", "warn", "medium", "middle", "low", "info":
		return true
	default:
		return false
	}
}

func inferGenerateRiskTarget(loop *reactloops.ReActLoop, target string) string {
	target = strings.TrimSpace(target)
	if target != "" {
		return target
	}
	if loop == nil {
		return ""
	}
	isHTTPS := strings.EqualFold(loop.Get("is_https"), "true")
	for _, raw := range []string{
		loop.Get("representative_request"),
		getCurrentRequestRaw(loop),
		loop.Get("original_request"),
	} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if url := extractRequestURL(raw, isHTTPS); strings.TrimSpace(url) != "" {
			return strings.TrimSpace(url)
		}
	}
	return ""
}

func buildGenerateRiskDetails(detailsRaw string, loop *reactloops.ReActLoop) map[string]any {
	details := parseGenerateRiskDetails(detailsRaw)
	details["source_loop"] = LoopHTTPFuzztestName
	details["source_action"] = "generate_risk"
	if loop == nil {
		return details
	}
	if hiddenIndex := strings.TrimSpace(loop.Get("representative_httpflow_hidden_index")); hiddenIndex != "" {
		details["representative_httpflow_hidden_index"] = hiddenIndex
	}
	if diffResult := strings.TrimSpace(firstNonEmptyString(loop.Get("diff_result_analysis"), loop.Get("diff_result_compressed"), loop.Get("diff_result"))); diffResult != "" {
		if _, exists := details["analysis_summary"]; !exists {
			details["analysis_summary"] = utils.ShrinkTextBlock(diffResult, 2000)
		}
	}
	if verification := strings.TrimSpace(loop.Get("verification_result")); verification != "" {
		if _, exists := details["verification_result"]; !exists {
			details["verification_result"] = utils.ShrinkTextBlock(verification, 1200)
		}
	}
	return details
}

func parseGenerateRiskDetails(detailsRaw string) map[string]any {
	detailsRaw = strings.TrimSpace(detailsRaw)
	if detailsRaw == "" {
		return map[string]any{}
	}
	var parsed any
	if err := json.Unmarshal([]byte(detailsRaw), &parsed); err == nil {
		if parsedMap, ok := parsed.(map[string]any); ok {
			return parsedMap
		}
		return map[string]any{"details": parsed}
	}
	return map[string]any{"summary": detailsRaw}
}
