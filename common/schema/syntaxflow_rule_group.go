package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type SyntaxFlowRuleGroup struct {
	gorm.Model
	RuleName      string `gorm:"index"`
	GroupName     string `gorm:"index"`
	Hash          string `gorm:"unique_index"`
	IsBuildInRule bool
}

func (s *SyntaxFlowRuleGroup) BeforeSave() error {
	s.CalcHash()
	return nil
}

func (s *SyntaxFlowRuleGroup) CalcHash() string {
	s.Hash = utils.CalcSha256(s.RuleName, s.GroupName)
	return s.Hash
}
