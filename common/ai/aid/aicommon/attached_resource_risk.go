package aicommon

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	RegisterAttachedResourceDataFactory(
		AttachedResourceTypeRisk,
		func() AttachedResourceData { return &AttachedRiskResourceData{} },
	)
}

// AttachedRiskResourceData is the task-scoped set of existing risks the model
// may inspect. The resource carries IDs only; risk records remain authoritative
// in the current project database.
type AttachedRiskResourceData struct {
	IDs []int64
}

func (d *AttachedRiskResourceData) Type() string {
	return AttachedResourceTypeRisk
}

func (d *AttachedRiskResourceData) Unmarshal(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return utils.Error("attached risk IDs are empty")
	}
	if !strings.HasPrefix(raw, "[") {
		return utils.Error("attached risk value must be a JSON array of positive risk IDs")
	}

	var ids []int64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return fmt.Errorf("parse attached risk IDs: %w", err)
	}
	if len(ids) == 0 {
		return utils.Error("attached risk ID list is empty")
	}
	if len(ids) > AttachedRiskResourceMaxIDs {
		return fmt.Errorf("attached risk ID count %d exceeds limit %d", len(ids), AttachedRiskResourceMaxIDs)
	}

	seen := make(map[int64]struct{}, len(ids))
	normalized := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return fmt.Errorf("attached risk ID must be positive: %d", id)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	d.IDs = normalized
	return nil
}

func (d *AttachedRiskResourceData) BindLoopData(ReActLoopIF) error {
	return nil
}

func (d *AttachedRiskResourceData) ToAttachData(loop ReActLoopIF) string {
	if d == nil || len(d.IDs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Attached Risks\n\n")
	b.WriteString(fmt.Sprintf("- Count: %d\n", len(d.IDs)))
	b.WriteString("- Scope: all listed existing project risks\n")
	b.WriteString("- Usage: query_cybersecurity_risk defaults to this attached scope; triage_cybersecurity_risks can modify only risks inside it.\n\n")
	if len(d.IDs) <= AttachedRiskResourcePreviewIDs {
		idList := make([]string, 0, len(d.IDs))
		for _, id := range d.IDs {
			idList = append(idList, strconv.FormatInt(id, 10))
		}
		b.WriteString("### Risk IDs\n```\n")
		b.WriteString(strings.Join(idList, ", "))
		b.WriteString("\n```\n")
	} else {
		b.WriteString(fmt.Sprintf(
			"IDs are omitted because %d risks are attached; use query_cybersecurity_risk to inspect the selection.\n",
			len(d.IDs),
		))
	}
	return strings.TrimSpace(b.String())
}

// AttachedRiskSelectionScope derives the task-scoped mutation boundary from
// explicit risk attachments. An invalid risk attachment still requires a scope
// but contributes no allowed IDs, causing triage to fail closed.
func AttachedRiskSelectionScope(task AIStatefulTask) (required bool, ids []int64) {
	if task == nil {
		return false, nil
	}
	seen := make(map[int64]struct{})
	for _, data := range task.GetAttachedDatas() {
		if data == nil || !data.HasType(AttachedResourceTypeRisk) {
			continue
		}
		required = true
		parsed, err := ParseAttachedResourceData(data)
		if err != nil {
			continue
		}
		riskData, ok := parsed.(*AttachedRiskResourceData)
		if !ok {
			continue
		}
		for _, id := range riskData.IDs {
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return required, ids
}
