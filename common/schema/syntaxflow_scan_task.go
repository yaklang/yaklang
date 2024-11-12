package schema

import (
	"github.com/jinzhu/gorm"
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
	RuleFilter []byte `gorm:"type:text"`

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
