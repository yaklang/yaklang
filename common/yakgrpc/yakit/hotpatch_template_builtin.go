package yakit

import (
	_ "embed"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed hotpatch_templates/http_flow_test_tags.yak
var builtinHTTPFlowTestTags string

// EnsureBuiltinHotPatchTemplates installs missing defaults without overwriting
// user edits or enabling global hotpatch. It runs before resolving saved config.
func EnsureBuiltinHotPatchTemplates(db *gorm.DB) error {
	if db == nil {
		return utils.Error("no database set")
	}
	template := NewHotPatchTemplate("HTTP流量测试项打标", builtinHTTPFlowTestTags, "global",
		[]string{"HTTP流量", "测试项打标"})
	return db.Where("name = ? AND type = ?", template.Name, template.Type).
		Attrs(template).FirstOrCreate(&schema.HotPatchTemplate{}).Error
}
