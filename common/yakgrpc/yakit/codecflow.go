package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type CodecFlow struct {
	gorm.Model
	FlowName string
	WorkFlow []*ypb.CodecWork
}

func (cf *CodecFlow) ToGRPC() *ypb.CustomizeCodecFlow {
	return &ypb.CustomizeCodecFlow{
		FlowName: cf.FlowName,
		WorkFlow: cf.WorkFlow,
	}
}

func CreateOrUpdateCodecFlow(db *gorm.DB, flow *CodecFlow) error {
	db = db.Model(&CodecFlow{})
	if db := db.Where("flow_name= ?", flow.FlowName).Assign(flow).FirstOrCreate(&CodecFlow{}); db.Error != nil {
		return utils.Errorf("create/update Codec Flow failed: %s", db.Error)
	}
	return nil
}

func DeleteCodecFlow(db *gorm.DB, flowName string) error {
	db = db.Model(&CodecFlow{})
	if db := db.Where("flow_name = ?", flowName).Delete(&CodecFlow{}); db.Error != nil {
		return utils.Errorf("delete Codec Flow failed: %s", db.Error)
	}
	return nil
}

func ClearCodecFlow(db *gorm.DB) error {
	db = db.Model(&CodecFlow{})
	if db := db.Unscoped().Delete(&CodecFlow{}); db.Error != nil {
		return utils.Errorf("clear Codec Flow failed: %s", db.Error)
	}
	return nil
}

func GetCodecFlowByName(db *gorm.DB, flowName string) (*CodecFlow, error) {
	var flow CodecFlow
	if db := db.Model(&CodecFlow{}).Where("flow_name = ?", flowName).First(&flow); db.Error != nil {
		return nil, db.Error
	}
	return &flow, nil
}

func GetAllCodecFlow(db *gorm.DB) ([]*CodecFlow, error) {
	var flows []*CodecFlow
	if db := db.Model(&CodecFlow{}).Find(&flows); db.Error != nil {
		return nil, db.Error
	}
	return flows, nil
}
