package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
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

func QueryAnalyzedHTTPFlowRule(db *gorm.DB, req *ypb.QueryAnalyzedHTTPFlowRuleRequest) (*bizhelper.Paginator, []*schema.AnalyzedHTTPFlow, error) {
	if req == nil {
		return nil, nil, utils.Error("QueryAnalyzedHTTPFlowRule request is nil")
	}
	p := req.GetPagination()
	if p == nil {
		p = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	db = db.Model(&schema.AnalyzedHTTPFlow{}).Preload("HTTPFlow")
	db = FilterAnalyzedHTTPFlowRule(db, req.GetFilter())
	var ret []*schema.AnalyzedHTTPFlow
	paging, db := bizhelper.YakitPagingQuery(db, p, &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("QueryAnalyzedHTTPFlowRule paging failed: %s", db.Error)
	}
	return paging, ret, nil
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
