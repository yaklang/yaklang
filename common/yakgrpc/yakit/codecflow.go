package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func CreateOrUpdateCodecFlow(db *gorm.DB, flow *schema.CodecFlow) error {
	db = db.Model(&schema.CodecFlow{})
	if db := db.Where("flow_name= ?", flow.FlowName).Assign(flow).FirstOrCreate(&schema.CodecFlow{}); db.Error != nil {
		return utils.Errorf("create/update Codec Flow failed: %s", db.Error)
	}
	return nil
}

func DeleteCodecFlow(db *gorm.DB, flowName string) error {
	db = db.Model(&schema.CodecFlow{})
	if db := db.Where("flow_name = ?", flowName).Delete(&schema.CodecFlow{}); db.Error != nil {
		return utils.Errorf("delete Codec Flow failed: %s", db.Error)
	}
	return nil
}

func ClearCodecFlow(db *gorm.DB) error {
	db = db.Model(&schema.CodecFlow{})
	if db := db.Unscoped().Delete(&schema.CodecFlow{}); db.Error != nil {
		return utils.Errorf("clear Codec Flow failed: %s", db.Error)
	}
	return nil
}

func GetCodecFlowByName(db *gorm.DB, flowName string) (*schema.CodecFlow, error) {
	var flow schema.CodecFlow
	if db := db.Model(&schema.CodecFlow{}).Where("flow_name = ?", flowName).First(&flow); db.Error != nil {
		return nil, db.Error
	}
	return &flow, nil
}

func GetAllCodecFlow(db *gorm.DB) ([]*schema.CodecFlow, error) {
	var flows []*schema.CodecFlow
	if db := db.Model(&schema.CodecFlow{}).Find(&flows); db.Error != nil {
		return nil, db.Error
	}
	return flows, nil
}
