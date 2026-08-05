package schema

import (
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// SyntaxFlowGroup is the catalog of known rule groups (package buckets).
// Rules bind via SyntaxFlowRule.RuleGroup (scalar), not many2many.
// The legacy join table syntax_flow_rule_and_group is left unused for soft DB compat.
type SyntaxFlowGroup struct {
	gorm.Model
	GroupName   string `json:"group_name" gorm:"unique_index"`
	IsBuildIn   bool   `json:"is_build_in" gorm:"index"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Source      string `json:"source" gorm:"index"`
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

// ToPackageGRPCModel maps a group catalog row to the soft-compat Package RPC shape.
func (s *SyntaxFlowGroup) ToPackageGRPCModel(ruleCount int32) *ypb.SyntaxFlowPackage {
	if s == nil {
		return nil
	}
	return &ypb.SyntaxFlowPackage{
		Name:        s.GroupName,
		Version:     s.Version,
		Description: s.Description,
		Source:      s.Source,
		IsBuiltin:   s.IsBuildIn,
		RuleCount:   ruleCount,
	}
}
