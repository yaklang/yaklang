package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//// SyntaxFlowRuleGroup Syntax Flow规则和组的关系表
//type SyntaxFlowRuleGroup struct {
//	gorm.Model
//	RuleName  string `json:"rule_name" gorm:"index"`
//	GroupName string `json:"group_name" gorm:"index"`
//	IsBuildIn bool   `json:"is_build_in"`
//	Hash      string `json:"hash" gorm:"unique_index"`
//}
//
//func (s *SyntaxFlowRuleGroup) BeforeSave() error {
//	s.CalcHash()
//	return nil
//}
//
//func (s *SyntaxFlowRuleGroup) CalcHash() string {
//	s.Hash = utils.CalcSha256(s.RuleName, s.GroupName)
//	return s.Hash
//}

type SyntaxFlowGroup struct {
	gorm.Model
	GroupName string            `json:"group_name" gorm:"unique_index"`
	IsBuildIn bool              `json:"is_build_in" gorm:"index"`
	Rules     []*SyntaxFlowRule `gorm:"many2many:syntax_flow_rule_and_group;"`
}

func (s *SyntaxFlowGroup) ToGRPCModel() *ypb.SyntaxFlowGroup {
	return &ypb.SyntaxFlowGroup{
		GroupName: s.GroupName,
		IsBuildIn: s.IsBuildIn,
		Count:     int32(len(s.Rules)),
	}
}
