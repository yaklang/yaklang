package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type AnalyzedHTTPFlow struct {
	gorm.Model
	ResultId        string      `json:"result_id" gorm:"index"`
	Rule            string      `json:"rule"`
	RuleVerboseName string      `json:"rule_verbose_name"`
	HTTPFlows       []*HTTPFlow `json:"http_flows"`
}

func (h *AnalyzedHTTPFlow) ToGRPCModel() *ypb.HTTPFlowRuleData {
	ids := lo.Map(h.HTTPFlows, func(flow *HTTPFlow, _ int) int64 {
		return int64(flow.ID)
	})
	result := &ypb.HTTPFlowRuleData{
		RuleVerboseName: h.RuleVerboseName,
		Rule:            h.Rule,
		HTTPFlowIds:     ids,
	}
	return result
}
