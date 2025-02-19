package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type GeneralRuleGroup struct {
	gorm.Model
	GroupName string         `json:"group_name" gorm:"unique_index"`
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
