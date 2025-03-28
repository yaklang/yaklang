package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type AnalyzedHTTPFlow struct {
	gorm.Model
	ResultId        string          `json:"result_id" gorm:"index"`
	Rule            string          `json:"rule"`
	RuleVerboseName string          `json:"rule_verbose_name"`
	HTTPFlowId      int64           `json:"http_flow_id"`
	ExtractedData   []ExtractedData `json:"extracted_data"`
}

func (h *AnalyzedHTTPFlow) ToGRPCModel() *ypb.HTTPFlowRuleData {
	if h == nil {
		return nil
	}
	result := &ypb.HTTPFlowRuleData{
		Id:              int64(h.ID),
		HTTPFlowId:      h.HTTPFlowId,
		RuleVerboseName: h.RuleVerboseName,
		Rule:            h.Rule,
	}
	return result
}
