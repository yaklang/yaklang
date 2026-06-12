package aicommon

import (
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
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
	result, err := RenderAttachedHTTPFlowResource(db, &AttachedResource{Value: d.Raw}, emitter)
	if err != nil {
		return fmt.Sprintf("## Attached HTTP Flows\n\n_Error: %v_", err)
	}
	return result
}

func (d *AttachedHTTPFlowResourceData) renderSummary(db *gorm.DB) string {
	return d.render(db, nil)
}
