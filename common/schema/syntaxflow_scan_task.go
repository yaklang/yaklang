package schema

import (
	"encoding/json"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	SYNTAXFLOWSCAN_EXECUTING     = "executing"
	SYNTAXFLOWSCAN_PAUSED        = "paused"
	SYNTAXFLOWSCAN_DONE          = "done"
	SYNTAXFLOWSCAN_ERROR         = "error"
	SYNTAXFLOWSCAN_PROGRAM_SPLIT = ","
)

type SyntaxFlowScanTask struct {
	gorm.Model
	TaskId   string `gorm:"unique_index"`
	Programs string
	// rules
	RulesCount int64

	Status string // executing / done / paused / error
	Reason string // user cancel / finished / recover failed so on

	// query execute
	FailedQuery  int64 // query failed
	SkipQuery    int64 // language not match, skip this rule
	SuccessQuery int64
	// risk
	RiskCount int64
	// query process
	TotalQuery int64

	// config
	Config []byte `gorm:"type:text"` // new data
}

func (s *SyntaxFlowScanTask) ToGRPCModel() *ypb.SyntaxFlowScanTask {
	res := &ypb.SyntaxFlowScanTask{
		Id:           uint64(s.ID),
		CreatedAt:    s.CreatedAt.Unix(),
		UpdatedAt:    s.CreatedAt.Unix(),
		TaskId:       s.TaskId,
		Programs:     strings.Split(s.Programs, SYNTAXFLOWSCAN_PROGRAM_SPLIT),
		RuleCount:    s.RulesCount,
		Status:       s.Status,
		Reason:       s.Reason,
		FailedQuery:  s.FailedQuery,
		SkipQuery:    s.SkipQuery,
		SuccessQuery: s.SuccessQuery,
		RiskCount:    s.RiskCount,
		TotalQuery:   s.TotalQuery,
	}
	if len(s.Config) != 0 {
		_ = json.Unmarshal(s.Config, &res.Config)
	}
	return res
}

func SaveSyntaxFlowScanTask(db *gorm.DB, task *SyntaxFlowScanTask) error {
	return db.Save(task).Error
}

func GetSyntaxFlowScanTaskById(db *gorm.DB, taskId string) (*SyntaxFlowScanTask, error) {
	task := &SyntaxFlowScanTask{}
	err := db.Where("task_id = ?", taskId).First(task).Error
	if err != nil {
		return nil, err
	}
	return task, nil
}

func DeleteSyntaxFlowScanTask(db *gorm.DB, taskId string) error {
	return db.Where("task_id = ?", taskId).Unscoped().Delete(&SyntaxFlowScanTask{}).Error
}
