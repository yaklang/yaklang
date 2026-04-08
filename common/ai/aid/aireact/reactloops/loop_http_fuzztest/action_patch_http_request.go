package loop_http_fuzztest

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

type httpRequestPatchSpec struct {
	Location      string `json:"location"`
	Operation     string `json:"operation"`
	FieldName     string `json:"field_name,omitempty"`
	FieldValue    string `json:"field_value,omitempty"`
	Reason        string `json:"reason,omitempty"`
	RepairProfile string `json:"repair_profile,omitempty"`
}

var patchHTTPRequestAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"patch_http_request",
		"对当前生效的 HTTP 数据包做细粒度补丁。适用于给 header/query/cookie/body/path/method 增删改字段、做格式转换、改认证信息，或对当前请求做基础修复。复杂整包重写仍使用 modify_http_request。",
		[]aitool.ToolOption{
			aitool.WithStringParam("location", aitool.WithParam_Description("补丁位置：method / path / header / query / cookie / body.form / body.json / body.raw / body.format / auth.basic / auth.bearer.jwt / request")),
			aitool.WithStringParam("operation", aitool.WithParam_Description("补丁动作：add / replace / remove / repair / transform")),
			aitool.WithStringParam("field_name", aitool.WithParam_Description("字段名。header/query/cookie/body.form/body.json 场景常用；body.format 可用作 XML 根节点；auth.basic 可用作 username。")),
			aitool.WithStringParam("field_value", aitool.WithParam_Description("字段值。body.json 支持 JSON 字面量；body.format 用于目标格式；auth.basic 可传 password 或 {\"username\":\"...\",\"password\":\"...\"}；auth.bearer.jwt 可传 claims JSON。")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("请用中文说明为什么要这样改包、希望验证什么，以及需要遵守的安全边界。"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("repair_profile", aitool.WithParam_Description("仅 operation=repair 时使用。可选 basic 或 browser_like，默认 basic。")),
		},
		[]*reactloops.LoopStreamField{
			{FieldName: "reason", AINodeId: "thought"},
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if strings.TrimSpace(getCurrentRequestRaw(loop)) == "" {
				return fmt.Errorf("patch_http_request requires an existing current HTTP request; call set_http_request or restore session first")
			}
			_, err := parseHTTPRequestPatchSpec(action)
			return err
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			spec, err := parseHTTPRequestPatchSpec(action)
			if err != nil {
				operator.Fail(err)
				return
			}

			previousRequest := strings.TrimSpace(getCurrentRequestRaw(loop))
			if previousRequest == "" {
				operator.Fail("current request is empty")
				return
			}

			isHTTPS := strings.EqualFold(loop.Get("is_https"), "true")
			patchedPacket, err := applyHTTPRequestPatchPlan([]byte(previousRequest), spec)
			if err != nil {
				operator.Fail(err)
				return
			}
			patchedPacket = lowhttp.FixHTTPRequest(patchedPacket)

			paramSummary := buildHTTPRequestPatchParamSummary(spec)
			diffSummary := compareRequests(previousRequest, string(patchedPacket))

			log.Infof("patch_http_request action: %s", paramSummary)

			fuzzReq, err := newLoopFuzzRequest(getLoopTaskContext(loop), r, patchedPacket, isHTTPS)
			if err != nil {
				operator.Fail(fmt.Errorf("failed to create patched FuzzHTTPRequest: %v", err))
				return
			}

			previousSummary := getCurrentRequestSummary(loop)
			setLoopCurrentRequestState(loop, fuzzReq, patchedPacket, isHTTPS)
			loop.Set("previous_request", previousRequest)
			loop.Set("previous_request_summary", previousSummary)
			loop.Set("request_change_summary", diffSummary)
			loop.Set("request_modification_reason", spec.Reason)
			loop.Set("request_review_decision", buildReviewDecisionLabel("auto_applied"))
			loop.Set("bootstrap_source", "patch_http_request")

			emitLoopHTTPFuzzEditablePacket(loop, operator.GetTask(), string(patchedPacket))
			feedback := buildHTTPRequestPatchAppliedFeedback(previousRequest, patchedPacket, isHTTPS, spec, diffSummary)
			record := recordLoopHTTPFuzzMetaAction(
				loop,
				"patch_http_request",
				paramSummary,
				utils.ShrinkTextBlock(diffSummary, 240),
			)
			persistLoopHTTPFuzzSessionContext(loop, "patch_http_request")
			r.AddToTimeline("patch_http_request", fmt.Sprintf("Patched current HTTP request: %s\n%s", summarizeHTTPRequestPatchSpec(spec), buildFuzzTimelineSummary(diffSummary)))
			operator.Feedback(buildLoopHTTPFuzzActionFeedback(record) + "\n\n" + feedback)
		},
	)
}

func parseHTTPRequestPatchSpec(action *aicommon.Action) (*httpRequestPatchSpec, error) {
	if action == nil {
		return nil, fmt.Errorf("patch action is nil")
	}
	spec := &httpRequestPatchSpec{
		Location:      strings.TrimSpace(strings.ToLower(action.GetString("location"))),
		Operation:     strings.TrimSpace(strings.ToLower(action.GetString("operation"))),
		FieldName:     strings.TrimSpace(action.GetString("field_name")),
		FieldValue:    action.GetString("field_value"),
		Reason:        strings.TrimSpace(action.GetString("reason")),
		RepairProfile: strings.TrimSpace(strings.ToLower(action.GetString("repair_profile"))),
	}

	if strings.TrimSpace(spec.Reason) == "" {
		return nil, fmt.Errorf("reason is required")
	}
	if err := validateHTTPRequestPatchSpec(spec); err != nil {
		return nil, err
	}
	return spec, nil
}

func validateHTTPRequestPatchSpec(spec *httpRequestPatchSpec) error {
	if spec == nil {
		return fmt.Errorf("patch spec is nil")
	}
	if spec.Reason == "" {
		return fmt.Errorf("reason is required")
	}
	if spec.Location == "" {
		return fmt.Errorf("location is required")
	}
	if spec.Operation == "" {
		return fmt.Errorf("operation is required")
	}

	validLocations := map[string]struct{}{
		"method":          {},
		"path":            {},
		"header":          {},
		"query":           {},
		"cookie":          {},
		"body.form":       {},
		"body.json":       {},
		"body.raw":        {},
		"body.format":     {},
		"auth.basic":      {},
		"auth.bearer.jwt": {},
		"request":         {},
	}
	if _, ok := validLocations[spec.Location]; !ok {
		return fmt.Errorf("location must be one of: method, path, header, query, cookie, body.form, body.json, body.raw, body.format, auth.basic, auth.bearer.jwt, request")
	}

	validOperations := map[string]struct{}{
		"add":       {},
		"replace":   {},
		"remove":    {},
		"repair":    {},
		"transform": {},
	}
	if _, ok := validOperations[spec.Operation]; !ok {
		return fmt.Errorf("operation must be one of: add, replace, remove, repair, transform")
	}

	if spec.Operation == "repair" {
		if spec.Location != "request" {
			return fmt.Errorf("operation=repair currently only supports location=request")
		}
		if spec.RepairProfile == "" {
			spec.RepairProfile = "basic"
		}
		if spec.RepairProfile != "basic" && spec.RepairProfile != "browser_like" {
			return fmt.Errorf("repair_profile must be one of: basic, browser_like")
		}
		return nil
	}

	switch spec.Location {
	case "body.format":
		if spec.Operation != "transform" {
			return fmt.Errorf("location=body.format requires operation=transform")
		}
		if strings.TrimSpace(spec.FieldValue) == "" {
			return fmt.Errorf("field_value is required for body.format transform")
		}
	case "auth.basic":
		if spec.Operation != "replace" && spec.Operation != "transform" {
			return fmt.Errorf("location=auth.basic only supports replace/transform")
		}
		if strings.TrimSpace(spec.FieldValue) == "" && strings.TrimSpace(spec.FieldName) == "" {
			return fmt.Errorf("auth.basic requires field_value or field_name")
		}
	case "auth.bearer.jwt":
		if spec.Operation != "replace" && spec.Operation != "transform" {
			return fmt.Errorf("location=auth.bearer.jwt only supports replace/transform")
		}
		if strings.TrimSpace(spec.FieldValue) == "" {
			return fmt.Errorf("auth.bearer.jwt requires field_value JSON claims patch")
		}
	case "method", "path", "body.raw":
		if spec.Operation != "remove" && strings.TrimSpace(spec.FieldValue) == "" {
			return fmt.Errorf("field_value is required for %s %s", spec.Operation, spec.Location)
		}
	case "request":
		return fmt.Errorf("location=request currently only supports operation=repair")
	default:
		if spec.Operation == "transform" {
			return fmt.Errorf("operation=transform is not supported for location=%s", spec.Location)
		}
		if spec.FieldName == "" {
			return fmt.Errorf("field_name is required for location=%s", spec.Location)
		}
		if spec.Operation != "remove" && strings.TrimSpace(spec.FieldValue) == "" {
			return fmt.Errorf("field_value is required for %s %s", spec.Operation, spec.Location)
		}
	}

	return nil
}

func applyHTTPRequestPatchPlan(packet []byte, spec *httpRequestPatchSpec) ([]byte, error) {
	if spec == nil {
		return nil, fmt.Errorf("patch spec is nil")
	}
	return applySingleHTTPRequestPatch(packet, spec)
}

func applySingleHTTPRequestPatch(packet []byte, spec *httpRequestPatchSpec) ([]byte, error) {
	if spec == nil {
		return nil, fmt.Errorf("patch spec is nil")
	}

	switch spec.Operation {
	case "repair":
		return lowhttp.RepairHTTPRequestPacket(packet, spec.RepairProfile)
	case "add", "replace", "remove", "transform":
	default:
		return nil, fmt.Errorf("unsupported patch operation: %s", spec.Operation)
	}

	switch spec.Location {
	case "method":
		if spec.Operation == "remove" {
			return nil, fmt.Errorf("method does not support remove")
		}
		return lowhttp.ReplaceHTTPPacketMethod(packet, strings.ToUpper(strings.TrimSpace(spec.FieldValue))), nil
	case "path":
		switch spec.Operation {
		case "add":
			return lowhttp.AppendHTTPPacketPath(packet, strings.TrimSpace(spec.FieldValue)), nil
		case "replace":
			return lowhttp.ReplaceHTTPPacketPath(packet, strings.TrimSpace(spec.FieldValue)), nil
		case "remove":
			return lowhttp.ReplaceHTTPPacketPath(packet, "/"), nil
		}
	case "header":
		switch spec.Operation {
		case "add":
			return lowhttp.AppendHTTPPacketHeader(packet, spec.FieldName, spec.FieldValue), nil
		case "replace":
			return lowhttp.ReplaceHTTPPacketHeader(packet, spec.FieldName, spec.FieldValue), nil
		case "remove":
			return lowhttp.DeleteHTTPPacketHeader(packet, spec.FieldName), nil
		}
	case "query":
		switch spec.Operation {
		case "add":
			return lowhttp.AppendHTTPPacketQueryParam(packet, spec.FieldName, spec.FieldValue), nil
		case "replace":
			return lowhttp.ReplaceHTTPPacketQueryParam(packet, spec.FieldName, spec.FieldValue), nil
		case "remove":
			return lowhttp.DeleteHTTPPacketQueryParam(packet, spec.FieldName), nil
		}
	case "cookie":
		switch spec.Operation {
		case "add":
			return lowhttp.AppendHTTPPacketCookie(packet, spec.FieldName, spec.FieldValue), nil
		case "replace":
			return lowhttp.ReplaceHTTPPacketCookie(packet, spec.FieldName, spec.FieldValue), nil
		case "remove":
			return lowhttp.DeleteHTTPPacketCookie(packet, spec.FieldName), nil
		}
	case "body.form":
		switch spec.Operation {
		case "add":
			return lowhttp.AppendHTTPPacketPostParam(packet, spec.FieldName, spec.FieldValue), nil
		case "replace":
			return lowhttp.ReplaceHTTPPacketPostParam(packet, spec.FieldName, spec.FieldValue), nil
		case "remove":
			return lowhttp.DeleteHTTPPacketPostParam(packet, spec.FieldName), nil
		}
	case "body.json":
		return lowhttp.PatchHTTPPacketJSONField(packet, spec.Operation, spec.FieldName, spec.FieldValue)
	case "body.raw":
		_, currentBody := lowhttp.SplitHTTPPacketFast(packet)
		switch spec.Operation {
		case "add":
			return lowhttp.ReplaceHTTPPacketBodyFast(packet, append(currentBody, []byte(spec.FieldValue)...)), nil
		case "replace":
			return lowhttp.ReplaceHTTPPacketBodyFast(packet, []byte(spec.FieldValue)), nil
		case "remove":
			return lowhttp.ReplaceHTTPPacketBodyFast(packet, nil), nil
		}
	case "body.format":
		return lowhttp.TransformHTTPPacketBodyFormat(packet, spec.FieldValue, spec.FieldName)
	case "auth.basic":
		return lowhttp.ReplaceHTTPPacketBasicAuthByPatch(packet, spec.FieldName, spec.FieldValue)
	case "auth.bearer.jwt":
		return lowhttp.RewriteHTTPPacketBearerJWTClaims(packet, spec.FieldValue)
	}

	return nil, fmt.Errorf("unsupported location=%s operation=%s", spec.Location, spec.Operation)
}

func buildHTTPRequestPatchParamSummary(spec *httpRequestPatchSpec) string {
	if spec == nil {
		return ""
	}
	return fmt.Sprintf("location=%s; operation=%s; field_name=%s; repair_profile=%s; reason=%s",
		spec.Location, spec.Operation, spec.FieldName, spec.RepairProfile, spec.Reason)
}

func summarizeHTTPRequestPatchSpec(spec *httpRequestPatchSpec) string {
	if spec == nil {
		return ""
	}
	if spec.Operation == "repair" {
		return fmt.Sprintf("%s %s", spec.Operation, spec.RepairProfile)
	}
	if spec.FieldName == "" {
		return fmt.Sprintf("%s %s", spec.Operation, spec.Location)
	}
	return fmt.Sprintf("%s %s.%s", spec.Operation, spec.Location, spec.FieldName)
}

func buildHTTPRequestPatchAppliedFeedback(previousRequest string, patchedPacket []byte, isHTTPS bool, spec *httpRequestPatchSpec, diffSummary string) string {
	var out strings.Builder
	out.WriteString("HTTP 数据包补丁已应用。\n\n")
	out.WriteString("=== 补丁意图 ===\n")
	out.WriteString(summarizeHTTPRequestPatchSpec(spec))
	out.WriteString("\n\n")
	if strings.TrimSpace(spec.Reason) != "" {
		out.WriteString("=== 修改原因 ===\n")
		out.WriteString(spec.Reason)
		out.WriteString("\n\n")
	}
	out.WriteString("=== 修改前摘要 ===\n")
	out.WriteString(getRequestSummaryWithFallback(previousRequest, isHTTPS))
	out.WriteString("\n\n")
	out.WriteString("=== 修改后摘要 ===\n")
	out.WriteString(getRequestSummaryWithFallback(string(patchedPacket), isHTTPS))
	out.WriteString("\n\n")
	out.WriteString("=== Merge 变化 ===\n")
	out.WriteString(diffSummary)
	out.WriteString("\n\n")
	out.WriteString("=== 当前生效数据包 ===\n")
	out.WriteString(string(patchedPacket))
	out.WriteString("\n")
	return out.String()
}

func getRequestSummaryWithFallback(request string, isHTTPS bool) string {
	request = strings.TrimSpace(request)
	if request == "" {
		return "(none)"
	}
	_, summary := buildHTTPRequestStreamSummary(request, isHTTPS)
	return summary
}

func emitLoopHTTPFuzzEditablePacket(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, rawPacket string) {
	if loop == nil || task == nil || loop.GetEmitter() == nil || strings.TrimSpace(rawPacket) == "" {
		return
	}
	taskID := task.GetId()
	if taskID == "" {
		taskID = utils.InterfaceToString(task.GetIndex())
	}
	if taskID == "" {
		return
	}
	_, _ = loop.GetEmitter().EmitHTTPRequestStreamEvent("http_flow", strings.NewReader(rawPacket), taskID)
}
