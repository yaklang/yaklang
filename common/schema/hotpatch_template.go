package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var HotPatchTemplateTableName = "hot_patch_template"

type HotPatchTemplate struct {
	gorm.Model
	Name    string `json:"name"`
	Content string `json:"content"`
	Type    string `json:"type"`
}

// TableName overrides the table name used by User to `profiles`
func (*HotPatchTemplate) TableName() string {
	return HotPatchTemplateTableName
}

func (t *HotPatchTemplate) ToGRPCModel() *ypb.HotPatchTemplate {
	return &ypb.HotPatchTemplate{
		Name:    t.Name,
		Content: t.Content,
		Type:    t.Type,
	}
}
