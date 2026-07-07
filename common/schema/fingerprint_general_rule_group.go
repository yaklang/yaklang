package schema

import (
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"gorm.io/gorm"
)

type GeneralRuleGroup struct {
	gorm.Model
	GroupName string         `json:"group_name" gorm:"uniqueIndex"`
	Rules     []*GeneralRule `gorm:"many2many:general_rule_and_group;"`
}

func (s *GeneralRuleGroup) ToGRPCModel() *ypb.FingerprintGroup {
	return &ypb.FingerprintGroup{
		GroupName: s.GroupName,
		Count:     int64(len(s.Rules)),
	}
}

func GRPCFingerprintGroupToSchemaGeneralRuleGroup(grpcGroup *ypb.FingerprintGroup) *GeneralRuleGroup {
	return &GeneralRuleGroup{
		GroupName: grpcGroup.GroupName,
	}
}
