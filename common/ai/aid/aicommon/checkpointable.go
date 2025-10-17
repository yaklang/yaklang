package aicommon

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// CheckpointableStorage 检查点存储接口
type CheckpointableStorage interface {
	CreateReviewCheckpoint(id int64) *schema.AiCheckpoint
	CreateToolCallCheckpoint(id int64) *schema.AiCheckpoint
	CreateAIInteractiveCheckpoint(id int64) *schema.AiCheckpoint
	SubmitCheckpointRequest(checkpoint *schema.AiCheckpoint, req any) error
	SubmitCheckpointResponse(checkpoint *schema.AiCheckpoint, rsp any) error
	GetDB() *gorm.DB
}

var _ CheckpointableStorage = &BaseCheckpointableStorage{}

// BaseCheckpointableStorage 基础检查点存储实现
type BaseCheckpointableStorage struct {
	runtimeId string // coordinator runtime ID
	db        *gorm.DB
}

// NewBaseCheckpointableStorage 创建新的基础检查点存储
func NewBaseCheckpointableStorage() *BaseCheckpointableStorage {
	return &BaseCheckpointableStorage{
		db: consts.GetGormProjectDatabase(),
	}
}

// NewCheckpointableStorageWithDB 使用指定数据库创建检查点存储
func NewCheckpointableStorageWithDB(runtimeUuid string, db *gorm.DB) *BaseCheckpointableStorage {
	if db == nil {
		db = consts.GetGormProjectDatabase()
	}
	return &BaseCheckpointableStorage{
		runtimeId: runtimeUuid,
		db:        db,
	}
}

// GetDB 获取数据库连接
func (s *BaseCheckpointableStorage) GetDB() *gorm.DB {
	return s.db
}

// createCheckpoint 创建检查点的通用方法
func (s *BaseCheckpointableStorage) createCheckpoint(runtimeId string, typeName schema.AiCheckpointType, id int64) *schema.AiCheckpoint {
	checkpoint := &schema.AiCheckpoint{
		CoordinatorUuid: runtimeId,
		Seq:             id,
		Type:            typeName,
		Finished:        false,
	}

	db := s.GetDB()
	if db == nil {
		log.Error("database connection is nil")
		return checkpoint
	}

	if err := yakit.CreateOrUpdateCheckpoint(db, checkpoint); err != nil {
		log.Errorf("failed to create checkpoint: %v", err)
	}

	log.Debugf("created checkpoint: runtime=%s, type=%s, seq=%d", runtimeId, typeName, id)
	return checkpoint
}

// CreateReviewCheckpoint 创建审核检查点
func (s *BaseCheckpointableStorage) CreateReviewCheckpoint(id int64) *schema.AiCheckpoint {
	return s.createCheckpoint(s.runtimeId, schema.AiCheckpointType_Review, id)
}

// CreateToolCallCheckpoint 创建工具调用检查点
func (s *BaseCheckpointableStorage) CreateToolCallCheckpoint(id int64) *schema.AiCheckpoint {
	return s.createCheckpoint(s.runtimeId, schema.AiCheckpointType_ToolCall, id)
}

// CreateAIInteractiveCheckpoint 创建AI交互检查点
func (s *BaseCheckpointableStorage) CreateAIInteractiveCheckpoint(id int64) *schema.AiCheckpoint {
	return s.createCheckpoint(s.runtimeId, schema.AiCheckpointType_AIInteractive, id)
}

// SubmitCheckpointRequest 提交检查点请求
func (s *BaseCheckpointableStorage) SubmitCheckpointRequest(checkpoint *schema.AiCheckpoint, req any) error {
	if checkpoint == nil {
		return utils.Error("checkpoint is nil")
	}

	checkpoint.RequestQuotedJson = codec.StrConvQuote(string(utils.Jsonify(req)))

	db := s.GetDB()
	if db == nil {
		return utils.Error("database connection is nil")
	}

	if err := yakit.CreateOrUpdateCheckpoint(db, checkpoint); err != nil {
		log.Errorf("failed to submit checkpoint request: %v", err)
		return err
	}

	log.Debugf("submitted checkpoint request: runtime=%s, seq=%d", checkpoint.CoordinatorUuid, checkpoint.Seq)
	return nil
}

// SubmitCheckpointResponse 提交检查点响应
func (s *BaseCheckpointableStorage) SubmitCheckpointResponse(checkpoint *schema.AiCheckpoint, rsp any) error {
	if checkpoint == nil {
		return utils.Error("checkpoint is nil")
	}

	checkpoint.ResponseQuotedJson = codec.StrConvQuote(string(utils.Jsonify(rsp)))
	checkpoint.Finished = true

	db := s.GetDB()
	if db == nil {
		return utils.Error("database connection is nil")
	}

	if err := yakit.CreateOrUpdateCheckpoint(db, checkpoint); err != nil {
		log.Errorf("failed to submit checkpoint response: %v", err)
		return err
	}

	log.Debugf("submitted checkpoint response: runtime=%s, seq=%d", checkpoint.CoordinatorUuid, checkpoint.Seq)
	return nil
}
