package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

func HandleAnalyzedHTTPFlowsColorAndTag(db *gorm.DB, flow *schema.HTTPFlow, color string, extraTag ...string) error {
	switch strings.ToLower(color) {
	case "red":
		flow.Red()
	case "green":
		flow.Green()
	case "blue":
		flow.Blue()
	case "yellow":
		flow.Yellow()
	case "orange":
		flow.Orange()
	case "purple":
		flow.Purple()
	case "cyan":
		flow.Cyan()
	case "grey":
		flow.Grey()
	}
	flow.AddTag(extraTag...)
	return UpdateHTTPFlowTags(db, flow)
}

func FilterAnalyzedHTTPFlowRule(db *gorm.DB, params *ypb.AnalyzedHTTPFlowFilter) *gorm.DB {
	if params == nil {
		return db
	}

	db = db.Model(&schema.AnalyzedHTTPFlow{})
	if len(params.GetResultIds()) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "result_id", params.GetResultIds())
	}
	if len(params.GetRuleVerboseNames()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOr(db, "rule_verbose_name", params.GetRuleVerboseNames())
	}
	if len(params.GetRule()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOr(db, "rule", params.GetRule())
	}
	return db
}

func QueryAnalyzedHTTPFlowRule(db *gorm.DB, resultIds []string) []*schema.AnalyzedHTTPFlow {
	var analyzed []*schema.AnalyzedHTTPFlow
	db = db.Model(&schema.AnalyzedHTTPFlow{})
	db.Where("result_id IN (?)", resultIds).Find(&analyzed)
	return analyzed
}
