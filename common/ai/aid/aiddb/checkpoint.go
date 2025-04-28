package aiddb

import (
	"context"
	"encoding/json"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func CreateOrUpdateCheckpoint(db *gorm.DB, checkpoint *schema.AiCheckpoint) error {
	if checkpoint.Hash == "" {
		checkpoint.Hash = checkpoint.CalcHash()
	}

	var existingCheckpoint schema.AiCheckpoint
	if err := db.Where("hash = ?", checkpoint.Hash).First(&existingCheckpoint).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return db.Create(checkpoint).Error
		}
		return err
	}

	return db.Model(&existingCheckpoint).Updates(checkpoint).Error
}

func YieldCheckpoint(ctx context.Context, db *gorm.DB, uuid string) chan *schema.AiCheckpoint {
	db = db.Model(&schema.AiCheckpoint{}).Where("coordinator_uuid = ?", uuid)
	return bizhelper.YieldModel[*schema.AiCheckpoint](ctx, db, bizhelper.WithYieldModel_PageSize(100))
}

func GetAIInteractiveCheckpoint(db *gorm.DB, coordinatorUuid string, seq int64) (*schema.AiCheckpoint, bool) {
	var checkpoint schema.AiCheckpoint
	if err := db.Where("coordinator_uuid = ? AND seq = ?", coordinatorUuid, seq).First(&checkpoint).Error; err != nil {
		return nil, false
	}

	if checkpoint.Type != schema.AiCheckpointType_AIInteractive {
		return &checkpoint, false
	}

	return &checkpoint, true
}

func GetToolCallCheckpoint(db *gorm.DB, coordinatorUuid string, seq int64) (*schema.AiCheckpoint, bool) {
	var checkpoint schema.AiCheckpoint
	if err := db.Where("coordinator_uuid = ? AND seq = ?", coordinatorUuid, seq).First(&checkpoint).Error; err != nil {
		return nil, false
	}

	if checkpoint.Type != schema.AiCheckpointType_ToolCall {
		return &checkpoint, false
	}

	return &checkpoint, true
}

func GetReviewCheckpoint(db *gorm.DB, coordinatorUuid string, seq int64) (*schema.AiCheckpoint, bool) {
	var checkpoint schema.AiCheckpoint
	if err := db.Where("coordinator_uuid = ? AND seq = ?", coordinatorUuid, seq).First(&checkpoint).Error; err != nil {
		return nil, false
	}

	if checkpoint.Type != schema.AiCheckpointType_Review {
		return &checkpoint, false
	}

	return &checkpoint, true
}

func AiCheckPointGetRequestParams(c *schema.AiCheckpoint) aitool.InvokeParams {
	var params = make(aitool.InvokeParams)
	result, err := codec.StrConvUnquote(c.RequestQuotedJson)
	if err != nil {
		log.Warnf("unquote response params failed: %v", err)
		return params
	}
	if err := json.Unmarshal([]byte(result), &params); err != nil {
		log.Warnf("unmarshal response params failed: %v", err)
		return params
	}
	return params
}

func AiCheckPointGetResponseParams(c *schema.AiCheckpoint) aitool.InvokeParams {
	var params = make(aitool.InvokeParams)
	result, err := codec.StrConvUnquote(c.ResponseQuotedJson)
	if err != nil {
		log.Warnf("unquote response params failed: %v", err)
		return params
	}
	if err := json.Unmarshal([]byte(result), &params); err != nil {
		log.Warnf("unmarshal response params failed: %v", err)
		return params
	}
	return params
}

func AiCheckPointGetToolResult(c *schema.AiCheckpoint) *aitool.ToolResult {
	var res aitool.ToolResult
	result, err := codec.StrConvUnquote(c.ResponseQuotedJson)
	if err != nil {
		log.Warnf("unquote request params failed: %v", err)
		return nil
	}
	if err := json.Unmarshal([]byte(result), &res); err != nil {
		log.Warnf("unmarshal request params failed: %v", err)
		return nil
	}
	return &res
}
