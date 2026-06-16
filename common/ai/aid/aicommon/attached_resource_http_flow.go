package aicommon

import (
	"encoding/json"
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

func init() {
	RegisterAttachedResourceDataFactory(
		AttachedResourceTypeHTTPFlowID,
		func() AttachedResourceData { return &AttachedHTTPFlowResourceData{} },
		"httpflowid",
		"http_flow",
	)
}

type AttachedHTTPFlowResourceData struct {
	Raw string
	IDs []int64
}

func (d *AttachedHTTPFlowResourceData) Type() string {
	return AttachedResourceTypeHTTPFlowID
}

func (d *AttachedHTTPFlowResourceData) Unmarshal(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return utils.Error("http flow id list is empty")
	}
	ids, err := parseAttachedHTTPFlowIDsJSON(raw)
	if err != nil {
		return err
	}
	ids, err = normalizeAttachedHTTPFlowIDs(ids)
	if err != nil {
		return err
	}
	d.Raw = raw
	d.IDs = ids
	return nil
}

func (d *AttachedHTTPFlowResourceData) BindLoopData(reactloop ReActLoopIF) error {
	return nil
}

func (d *AttachedHTTPFlowResourceData) ToAttachData(reactloop ReActLoopIF) string {
	db := consts.GetGormProjectDatabase()
	var emitter *Emitter
	if reactloop != nil {
		emitter = reactloop.GetEmitter()
		if cfg := reactloop.GetConfig(); cfg != nil && cfg.GetDB() != nil {
			db = cfg.GetDB()
		}
	}
	return d.render(db, emitter)
}

func (d *AttachedHTTPFlowResourceData) render(db *gorm.DB, emitter *Emitter) string {
	if db == nil {
		return "## Attached HTTP Flows\n\n_Error: project database is not available_"
	}

	var sections []string
	var loadErrors []string
	for _, flowID := range d.IDs {
		flow, loadErr := yakit.GetHTTPFlow(db, flowID)
		if loadErr != nil {
			loadErrors = append(loadErrors, fmt.Sprintf("- ID %d: load failed: %v", flowID, loadErr))
			continue
		}
		sections = append(sections, FormatAttachedHTTPFlow(flow, emitter))
	}

	var builder strings.Builder
	builder.WriteString("## Attached HTTP Flows\n\n")
	builder.WriteString(fmt.Sprintf("Requested IDs: %s\n\n", FormatAttachedHTTPFlowIDsSummary(d.Raw)))
	if len(loadErrors) > 0 {
		builder.WriteString("### Load Errors\n")
		builder.WriteString(strings.Join(loadErrors, "\n"))
		builder.WriteString("\n\n")
	}
	if len(sections) == 0 {
		if len(loadErrors) > 0 {
			return strings.TrimSpace(builder.String())
		}
		builder.WriteString("_Error: no attached HTTP flows could be loaded_")
		return strings.TrimSpace(builder.String())
	}
	builder.WriteString(strings.Join(sections, "\n\n---\n\n"))

	full := strings.TrimSpace(builder.String())
	inline, spillNote := inlineOrSpillAttachedText("attached_http_flow_list", full, AttachedHTTPFlowListInlineLimit, emitter)
	if spillNote != "" {
		return strings.TrimSpace(spillNote + "\n\nInline preview:\n" + inline)
	}
	return full
}

func parseAttachedHTTPFlowIDsJSON(raw string) ([]int64, error) {
	var directItems []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &directItems); err == nil {
		return parseAttachedHTTPFlowIDItems(directItems)
	}

	var payload struct {
		IDs json.RawMessage `json:"ids"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err == nil && len(payload.IDs) > 0 {
		var idItems []json.RawMessage
		if err := json.Unmarshal(payload.IDs, &idItems); err == nil {
			return parseAttachedHTTPFlowIDItems(idItems)
		}
	}

	// Try parsing as a single ID (number or string)
	var singleID int64
	if err := json.Unmarshal([]byte(raw), &singleID); err == nil {
		return []int64{singleID}, nil
	}

	var singleIDStr string
	if err := json.Unmarshal([]byte(raw), &singleIDStr); err == nil {
		parsed, err := strconv.ParseInt(strings.TrimSpace(singleIDStr), 10, 64)
		if err != nil {
			return nil, utils.Errorf("invalid http flow id string: %q", singleIDStr)
		}
		return []int64{parsed}, nil
	}

	return nil, utils.Errorf("invalid http flow id list json: %q", raw)
}

func parseAttachedHTTPFlowIDItems(items []json.RawMessage) ([]int64, error) {
	if len(items) == 0 {
		return nil, utils.Error("http flow id list is empty")
	}
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		item = json.RawMessage(strings.TrimSpace(string(item)))
		var id int64
		if err := json.Unmarshal(item, &id); err == nil {
			ids = append(ids, id)
			continue
		}
		var idStr string
		if err := json.Unmarshal(item, &idStr); err == nil {
			parsed, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err != nil {
				return nil, utils.Errorf("invalid http flow id string: %q", idStr)
			}
			ids = append(ids, parsed)
			continue
		}
		return nil, utils.Errorf("invalid http flow id element: %s", string(item))
	}
	return ids, nil
}

func normalizeAttachedHTTPFlowIDs(ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, utils.Error("http flow id list is empty")
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, utils.Errorf("invalid http flow id: %d", id)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, utils.Error("http flow id list is empty")
	}
	return out, nil
}

func FormatAttachedHTTPFlowIDsSummary(value string) string {
	ids, err := parseAttachedHTTPFlowIDsJSON(strings.TrimSpace(value))
	if err == nil {
		ids, err = normalizeAttachedHTTPFlowIDs(ids)
	}
	if err != nil {
		return strings.TrimSpace(value)
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ", ")
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
