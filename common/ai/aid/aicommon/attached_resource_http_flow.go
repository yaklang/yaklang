package aicommon

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

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
	if reactloop != nil {
		if cfg := reactloop.GetConfig(); cfg != nil && cfg.GetDB() != nil {
			db = cfg.GetDB()
		}
	}
	return d.renderSummary(db)
}

func (d *AttachedHTTPFlowResourceData) renderSummary(db *gorm.DB) string {
	if db == nil {
		return "## Attached HTTP Flows\n\n_Error: project database is not available_"
	}

	var builder strings.Builder
	builder.WriteString("## Attached HTTP Flows\n")
	var loadErrors []string
	var summaries []string
	for _, flowID := range d.IDs {
		flow, loadErr := yakit.GetHTTPFlow(db, flowID)
		if loadErr != nil {
			loadErrors = append(loadErrors, fmt.Sprintf("- ID %d: load failed: %v", flowID, loadErr))
			continue
		}
		summaries = append(summaries, formatHTTPFlowSummary(flow))
	}

	if len(loadErrors) > 0 {
		builder.WriteString("### Load Errors\n")
		builder.WriteString(strings.Join(loadErrors, "\n"))
		builder.WriteString("\n\n")
	}

	if len(summaries) == 0 {
		if len(loadErrors) > 0 {
			return strings.TrimSpace(builder.String())
		}
		builder.WriteString("_Error: no attached HTTP flows could be loaded_")
		return strings.TrimSpace(builder.String())
	}

	for i, summary := range summaries {
		if i > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(summary)
	}

	return strings.TrimSpace(builder.String())
}

func formatHTTPFlowSummary(flow *schema.HTTPFlow) string {
	if flow == nil {
		return "Flow not found"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("**ID %d**: ", flow.ID))
	b.WriteString(fmt.Sprintf("%s %s ", formatAttachedNullableString(flow.Method), formatAttachedNullableString(flow.Url)))
	b.WriteString(fmt.Sprintf("→ %d ", flow.StatusCode))
	b.WriteString(fmt.Sprintf("(Req: %d bytes, Resp: %d bytes)", flow.RequestLength, flow.BodyLength))
	if flow.Tags != "" {
		b.WriteString(fmt.Sprintf(" [%s]", formatAttachedNullableString(flow.Tags)))
	}
	return b.String()
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

func formatAttachedNullableString(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return v
}
