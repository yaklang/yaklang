package schema

import (
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Reserved SyntaxFlow rule-group names (stored in SyntaxFlowRule.RuleGroup).
const (
	SyntaxFlowGroupBuiltin = "builtin"
	SyntaxFlowGroupAgent   = "agent"
	SyntaxFlowGroupCustom  = "custom"
)

// SyntaxFlowGroup is the catalog of known rule groups.
// Rules bind via SyntaxFlowRule.RuleGroup (scalar), not many2many.
// The legacy join table syntax_flow_rule_and_group is left unused for soft DB compat.
type SyntaxFlowGroup struct {
	gorm.Model
	GroupName string `json:"group_name" gorm:"unique_index"`
	IsBuildIn bool   `json:"is_build_in" gorm:"index"`
}

func (s *SyntaxFlowGroup) ToGRPCModel() *ypb.SyntaxFlowGroup {
	return &ypb.SyntaxFlowGroup{
		GroupName: s.GroupName,
		IsBuildIn: s.IsBuildIn,
		Count:     0, // filled by caller when known
	}
}

func (s *SyntaxFlowGroup) ToGRPCModelWithCount(count int32) *ypb.SyntaxFlowGroup {
	return &ypb.SyntaxFlowGroup{
		GroupName: s.GroupName,
		IsBuildIn: s.IsBuildIn,
		Count:     count,
	}
}
