package loop_http_flow_analyze

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		datas := reactloops.RunAttachedExtraResourcesInit(r, loop, task.GetAttachedDatas())
		var flowIds []int64
		for _, data := range datas {
			switch data.(type) {
			case *aicommon.AttachedHTTPFlowResourceData:
				flowData := data.(*aicommon.AttachedHTTPFlowResourceData)
				flowIds = append(flowIds, flowData.IDs...)
			}
		}
		flowIds = lo.Uniq(flowIds)

		if len(flowIds) > 0 {
			loop.Set(attachedHTTPFlowIDsKey, flowIds)

			// Format detailed HTTP flow information for the loop
			db := consts.GetGormProjectDatabase()
			if cfg := r.GetConfig(); cfg != nil && cfg.GetDB() != nil {
				db = cfg.GetDB()
			}

			if db != nil {
				detailedInfo := formatAttachedHTTPFlowsDetailed(db, flowIds, loop)
				loop.Set(attachedHTTPFlowDetailsKey, detailedInfo)
			} else {
				log.Warn("database not available for formatting HTTP flow details")
			}
		}
	}
}

// formatAttachedHTTPFlowsDetailed formats HTTP flows with full request/response details
// This is specific to loop_http_flow_analyze and enforces length limits
func formatAttachedHTTPFlowsDetailed(db *gorm.DB, flowIDs []int64, loop *reactloops.ReActLoop) string {
	if len(flowIDs) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("## Detailed HTTP Flow Information\n\n")
	builder.WriteString(fmt.Sprintf("Total flows: %d\n\n", len(flowIDs)))

	var sections []string
	var loadErrors []string

	emitter := loop.GetEmitter()

	for _, flowID := range flowIDs {
		flow, err := yakit.GetHTTPFlow(db, flowID)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Sprintf("- ID %d: load failed: %v", flowID, err))
			continue
		}
		// Use the local formatting function
		sections = append(sections, formatAttachedHTTPFlow(flow, emitter))
	}

	if len(loadErrors) > 0 {
		builder.WriteString("### Load Errors\n")
		builder.WriteString(strings.Join(loadErrors, "\n"))
		builder.WriteString("\n\n")
	}

	if len(sections) == 0 {
		builder.WriteString("_Error: no HTTP flows could be loaded_\n")
		return strings.TrimSpace(builder.String())
	}

	builder.WriteString(strings.Join(sections, "\n\n---\n\n"))
	full := strings.TrimSpace(builder.String())

	// Apply centralized length limit to avoid overwhelming the context
	// This prevents multiple flows from bypassing the limit
	if len(full) <= aicommon.AttachedHTTPFlowListInlineLimit {
		return full
	}

	// Content is too large, save to file
	if loopDataDir := loop.GetLoopContentDir("data"); loopDataDir != "" {
		filename := fmt.Sprintf("%s/attached_http_flows_detailed.md", loopDataDir)
		if err := os.WriteFile(filename, []byte(full), 0644); err != nil {
			log.Warnf("failed to write detailed HTTP flows to file: %v", err)
		} else {
			if emitter != nil {
				emitter.EmitPinFilename(filename)
			}
			// Return reference to file with inline preview
			inline := full
			if len(inline) > aicommon.AttachedHTTPFlowListInlineLimit {
				inline = inline[:aicommon.AttachedHTTPFlowListInlineLimit]
			}
			return fmt.Sprintf("## Detailed HTTP Flow Information\n\nContent length %d exceeds inline limit %d.\nFull content saved to file: %s\n\nInline preview:\n%s",
				len(full), aicommon.AttachedHTTPFlowListInlineLimit, filename, inline)
		}
	}

	// Fallback: truncate inline
	inline := full
	if len(inline) > aicommon.AttachedHTTPFlowListInlineLimit {
		inline = inline[:aicommon.AttachedHTTPFlowListInlineLimit]
	}
	return fmt.Sprintf("## Detailed HTTP Flow Information\n\nContent length %d exceeds inline limit %d.\n\nTruncated preview:\n%s",
		len(full), aicommon.AttachedHTTPFlowListInlineLimit, inline)
}

// formatAttachedHTTPFlow formats a single HTTP flow with full details
func formatAttachedHTTPFlow(flow *schema.HTTPFlow, emitter *aicommon.Emitter) string {
	if flow == nil {
		return "HTTP flow not found"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Attached HTTP Flow #%d\n\n", flow.ID))
	b.WriteString(formatAttachedHTTPFlowMetadata(flow))
	b.WriteString("\n")

	req := attachedHTTPFlowRequest(flow)
	inlineReq, reqSpill := inlineOrSpillAttachedText("request", req, aicommon.AttachedHTTPFlowRequestInlineLimit, emitter)
	b.WriteString("### Request\n")
	if reqSpill != "" {
		b.WriteString(reqSpill)
		b.WriteString("\n\nInline preview:\n```\n")
		b.WriteString(inlineReq)
		b.WriteString("\n```\n\n")
	} else {
		b.WriteString("```\n")
		b.WriteString(inlineReq)
		b.WriteString("\n```\n\n")
	}

	rsp := attachedHTTPFlowResponse(flow)
	inlineRsp, rspSpill := inlineOrSpillAttachedText("response", rsp, aicommon.AttachedHTTPFlowResponseInlineLimit, emitter)
	b.WriteString("### Response\n")
	if rspSpill != "" {
		b.WriteString(rspSpill)
		b.WriteString("\n\nInline preview:\n```\n")
		b.WriteString(inlineRsp)
		b.WriteString("\n```\n")
	} else {
		b.WriteString("```\n")
		b.WriteString(inlineRsp)
		b.WriteString("\n```\n")
	}

	return strings.TrimSpace(b.String())
}

// attachedHTTPFlowRequest extracts the request content from flow
func attachedHTTPFlowRequest(flow *schema.HTTPFlow) string {
	if flow == nil {
		return ""
	}
	if req := flow.GetRequest(); req != "" {
		return req
	}
	return flow.Request
}

// attachedHTTPFlowResponse extracts the response content from flow
func attachedHTTPFlowResponse(flow *schema.HTTPFlow) string {
	if flow == nil {
		return ""
	}
	if rsp := flow.GetResponse(); rsp != "" {
		return rsp
	}
	if flow.TooLargeResponseHeaderFile != "" || flow.TooLargeResponseBodyFile != "" {
		var parts []string
		if flow.TooLargeResponseHeaderFile != "" {
			if data, err := os.ReadFile(flow.TooLargeResponseHeaderFile); err == nil && len(data) > 0 {
				parts = append(parts, string(data))
			}
		}
		if flow.TooLargeResponseBodyFile != "" {
			if data, err := os.ReadFile(flow.TooLargeResponseBodyFile); err == nil && len(data) > 0 {
				parts = append(parts, string(data))
			}
		}
		return strings.Join(parts, "")
	}
	return flow.Response
}

// formatAttachedNullableString formats nullable string fields
func formatAttachedNullableString(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return v
}

// formatAttachedProcessName formats the process name field
func formatAttachedProcessName(flow *schema.HTTPFlow) string {
	if flow == nil || !flow.ProcessName.Valid {
		return "-"
	}
	return formatAttachedNullableString(flow.ProcessName.String)
}

// formatAttachedHTTPFlowMetadata formats all metadata fields of a flow
func formatAttachedHTTPFlowMetadata(flow *schema.HTTPFlow) string {
	if flow == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Metadata\n")
	b.WriteString(fmt.Sprintf("- ID: %d\n", flow.ID))
	b.WriteString(fmt.Sprintf("- HiddenIndex: %s\n", formatAttachedNullableString(flow.HiddenIndex)))
	b.WriteString(fmt.Sprintf("- Hash: %s\n", formatAttachedNullableString(flow.Hash)))
	b.WriteString(fmt.Sprintf("- IsHTTPS: %t\n", flow.IsHTTPS))
	b.WriteString(fmt.Sprintf("- URL: %s\n", formatAttachedNullableString(flow.Url)))
	b.WriteString(fmt.Sprintf("- Path: %s\n", formatAttachedNullableString(flow.Path)))
	b.WriteString(fmt.Sprintf("- PathSuffix: %s\n", formatAttachedNullableString(flow.PathSuffix)))
	b.WriteString(fmt.Sprintf("- Method: %s\n", formatAttachedNullableString(flow.Method)))
	b.WriteString(fmt.Sprintf("- RequestLength: %d\n", flow.RequestLength))
	b.WriteString(fmt.Sprintf("- BodyLength: %d\n", flow.BodyLength))
	b.WriteString(fmt.Sprintf("- ContentType: %s\n", formatAttachedNullableString(flow.ContentType)))
	b.WriteString(fmt.Sprintf("- StatusCode: %d\n", flow.StatusCode))
	b.WriteString(fmt.Sprintf("- SourceType: %s\n", formatAttachedNullableString(flow.SourceType)))
	b.WriteString(fmt.Sprintf("- DurationMs: %d\n", flow.Duration/int64(time.Millisecond)))
	b.WriteString(fmt.Sprintf("- GetParamsTotal: %d\n", flow.GetParamsTotal))
	b.WriteString(fmt.Sprintf("- PostParamsTotal: %d\n", flow.PostParamsTotal))
	b.WriteString(fmt.Sprintf("- CookieParamsTotal: %d\n", flow.CookieParamsTotal))
	b.WriteString(fmt.Sprintf("- IPAddress: %s\n", formatAttachedNullableString(flow.IPAddress)))
	b.WriteString(fmt.Sprintf("- RemoteAddr: %s\n", formatAttachedNullableString(flow.RemoteAddr)))
	b.WriteString(fmt.Sprintf("- Host: %s\n", formatAttachedNullableString(flow.Host)))
	b.WriteString(fmt.Sprintf("- Tags: %s\n", formatAttachedNullableString(flow.Tags)))
	b.WriteString(fmt.Sprintf("- Payload: %s\n", formatAttachedNullableString(flow.Payload)))
	b.WriteString(fmt.Sprintf("- IsWebsocket: %t\n", flow.IsWebsocket))
	b.WriteString(fmt.Sprintf("- WebsocketHash: %s\n", formatAttachedNullableString(flow.WebsocketHash)))
	b.WriteString(fmt.Sprintf("- RuntimeId: %s\n", formatAttachedNullableString(flow.RuntimeId)))
	b.WriteString(fmt.Sprintf("- FromPlugin: %s\n", formatAttachedNullableString(flow.FromPlugin)))
	b.WriteString(fmt.Sprintf("- ProcessName: %s\n", formatAttachedProcessName(flow)))
	b.WriteString(fmt.Sprintf("- IsTooLargeResponse: %t\n", flow.IsTooLargeResponse))
	b.WriteString(fmt.Sprintf("- IsReadTooSlowResponse: %t\n", flow.IsReadTooSlowResponse))
	b.WriteString(fmt.Sprintf("- TooLargeResponseHeaderFile: %s\n", formatAttachedNullableString(flow.TooLargeResponseHeaderFile)))
	b.WriteString(fmt.Sprintf("- TooLargeResponseBodyFile: %s\n", formatAttachedNullableString(flow.TooLargeResponseBodyFile)))
	if !flow.CreatedAt.IsZero() {
		b.WriteString(fmt.Sprintf("- CreatedAt: %s\n", flow.CreatedAt.Format(time.RFC3339)))
	}
	if !flow.UpdatedAt.IsZero() {
		b.WriteString(fmt.Sprintf("- UpdatedAt: %s\n", flow.UpdatedAt.Format(time.RFC3339)))
	}
	return b.String()
}

// inlineOrSpillAttachedText handles content that may exceed inline limits
func inlineOrSpillAttachedText(label, content string, limit int, emitter *aicommon.Emitter) (inline string, spillNote string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "(empty)", ""
	}
	if len(content) <= limit {
		return content, ""
	}

	filePath := consts.TempAIFileFast(fmt.Sprintf("attached-%s-*.txt", label), content)
	if filePath != "" && emitter != nil {
		_, _ = emitter.EmitPinFilename(filePath)
	}

	inline = content[:limit]
	spillNote = fmt.Sprintf(
		"%s length %d exceeds inline limit %d; full content saved to file: %s\nUse file-reading tools to load the complete content.",
		label, len(content), limit, filePath,
	)
	return inline, spillNote
}
