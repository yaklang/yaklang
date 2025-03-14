package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

func HandleAnalyzedHTTPFlowsColorAndTag(db *gorm.DB, flows []*schema.HTTPFlow, color string, extraTag ...string) error {
	for _, flow := range flows {
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
	}
	return UpdateHTTPFlowsTags(db, flows)
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
	db = bizhelper.OrderByPaging(db, p)
	db = db.Model(&schema.AnalyzedHTTPFlow{})
	db = db.Preload("HTTPFlows")
	db = bizhelper.ExactQueryStringArrayOr(db, "result_id", req.GetAnalyzeIds())

	var ret []*schema.AnalyzedHTTPFlow
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("QueryAnalyzedHTTPFlowRule paging failed: %s", db.Error)
	}
	return paging, ret, nil
}
