package aicommon

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	RegisterAttachedResourceDataFactory(AttachedResourceTypeRiskID,
		func() AttachedResourceData { return &AttachedRiskResourceData{} }, "risk")
}

// AttachedRiskResourceData identifies an existing project risk. Resolving the
// record and its session/runtime boundary belongs to the consuming focus loop.
type AttachedRiskResourceData struct {
	ID int64
}

func (d *AttachedRiskResourceData) Type() string { return AttachedResourceTypeRiskID }

func (d *AttachedRiskResourceData) Unmarshal(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return utils.Error("risk id is empty")
	}
	if strings.HasPrefix(raw, "{") {
		var payload struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return fmt.Errorf("invalid risk resource: %w", err)
		}
		raw = strings.TrimSpace(string(payload.ID))
	}
	if strings.HasPrefix(raw, `"`) {
		var id string
		if err := json.Unmarshal([]byte(raw), &id); err != nil {
			return fmt.Errorf("invalid risk id: %w", err)
		}
		raw = strings.TrimSpace(id)
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("invalid positive risk id: %q", raw)
	}
	d.ID = id
	return nil
}

func (d *AttachedRiskResourceData) BindLoopData(ReActLoopIF) error { return nil }

func (d *AttachedRiskResourceData) ToAttachData(ReActLoopIF) string {
	return fmt.Sprintf("Attached risk ID: %d (the focus loop resolves this record at initialization)", d.ID)
}
