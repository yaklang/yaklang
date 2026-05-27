package aicommon

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func normalizeAttachedResourceType(typ string) string {
	return strings.ToLower(strings.TrimSpace(typ))
}

func IsAttachedHTTPFlowResource(data *AttachedResource) bool {
	if data == nil {
		return false
	}
	switch normalizeAttachedResourceType(data.Type) {
	case AttachedResourceTypeHTTPFlowID, "httpflowid", "http_flow":
		return true
	default:
		return false
	}
}

func IsAttachedSelectedResource(data *AttachedResource) bool {
	if data == nil {
		return false
	}
	return normalizeAttachedResourceType(data.Type) == AttachedResourceTypeSelected
}

func attachedHTTPFlowIDFromResource(data *AttachedResource) (int64, error) {
	if data == nil {
		return 0, utils.Error("attached resource is nil")
	}
	raw := strings.TrimSpace(data.Value)
	if raw == "" {
		return 0, utils.Error("http flow id is empty")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, utils.Errorf("invalid http flow id: %q", raw)
	}
	return id, nil
}

func attachedSelectedTextFromResource(data *AttachedResource) string {
	if data == nil {
		return ""
	}
	return data.Value
}

func attachedHTTPFlowRequest(flow *schema.HTTPFlow) string {
	if flow == nil {
		return ""
	}
	if req := flow.GetRequest(); req != "" {
		return req
	}
	return flow.Request
}

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

func formatAttachedNullableString(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return v
}

func formatAttachedProcessName(flow *schema.HTTPFlow) string {
	if flow == nil || !flow.ProcessName.Valid {
		return "-"
	}
	return formatAttachedNullableString(flow.ProcessName.String)
}

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

func inlineOrSpillAttachedText(label, content string, limit int, emitter *Emitter) (inline string, spillNote string) {
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

func FormatAttachedHTTPFlow(flow *schema.HTTPFlow, emitter *Emitter) string {
	if flow == nil {
		return "HTTP flow not found"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Attached HTTP Flow #%d\n\n", flow.ID))
	b.WriteString(formatAttachedHTTPFlowMetadata(flow))
	b.WriteString("\n")

	req := attachedHTTPFlowRequest(flow)
	inlineReq, reqSpill := inlineOrSpillAttachedText("request", req, AttachedHTTPFlowRequestInlineLimit, emitter)
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
	inlineRsp, rspSpill := inlineOrSpillAttachedText("response", rsp, AttachedHTTPFlowResponseInlineLimit, emitter)
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

func FormatAttachedSelectedText(content string, emitter *Emitter) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return "## Attached Selected Text\n\n(empty selection)"
	}

	inline, spillNote := inlineOrSpillAttachedText("selected_text", content, AttachedSelectedTextInlineLimit, emitter)
	var b strings.Builder
	b.WriteString("## Attached Selected Text\n\n")
	if spillNote != "" {
		b.WriteString(spillNote)
		b.WriteString("\n\nInline preview:\n```\n")
		b.WriteString(inline)
		b.WriteString("\n```\n")
	} else {
		b.WriteString("```\n")
		b.WriteString(inline)
		b.WriteString("\n```\n")
	}
	return strings.TrimSpace(b.String())
}

func RenderAttachedHTTPFlowResource(db *gorm.DB, data *AttachedResource, emitter *Emitter) (string, error) {
	if db == nil {
		return "", utils.Error("project database is not available")
	}
	flowID, err := attachedHTTPFlowIDFromResource(data)
	if err != nil {
		return "", err
	}
	flow, err := yakit.GetHTTPFlow(db, flowID)
	if err != nil {
		return "", utils.Wrap(err, "load attached http flow failed")
	}
	return FormatAttachedHTTPFlow(flow, emitter), nil
}

func RenderAttachedSelectedResource(data *AttachedResource, emitter *Emitter) string {
	return FormatAttachedSelectedText(attachedSelectedTextFromResource(data), emitter)
}
