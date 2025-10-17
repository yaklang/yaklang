package yakit

import (
	"errors"

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

func CreateCodecFlow(db *gorm.DB, flow *schema.CodecFlow) error {
	var existingFlow schema.CodecFlow
	result := db.Model(&schema.CodecFlow{}).
		Where("flow_name = ?", flow.FlowName).
		First(&existingFlow)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			if err := db.Create(flow).Error; err != nil {
				return utils.Errorf("create Codec Flow failed: %s", err)
			}
			return nil
		}
		return utils.Errorf("query Codec Flow failed: %s", result.Error)
	}

	return utils.Errorf("Codec Flow: %s already exists", flow.FlowName)
}

func UpdateCodecFlow(db *gorm.DB, flow *schema.CodecFlow) error {
	var existingFlow schema.CodecFlow
	result := db.Model(&schema.CodecFlow{}).
		Where("flow_name = ?", flow.FlowName).
		First(&existingFlow)

	if result.Error == nil {
		if err := db.Model(&schema.CodecFlow{}).
			Where("flow_name = ?", flow.FlowName).
			Updates(flow).Error; err != nil {
			return utils.Errorf("update Codec Flow failed: %s", err)
		}
		return nil
	}

	return utils.Errorf("Codec Flow: %s not find", flow.FlowName)
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

// func GetCodecFlowByID(db *gorm.DB, flowID string) (*schema.CodecFlow, error) {
// 	var flow schema.CodecFlow
// 	if db := db.Model(&schema.CodecFlow{}).Where("flow_id = ?", flowID).First(&flow); db.Error != nil {
// 		return nil, db.Error
// 	}
// 	return &flow, nil
// }

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
