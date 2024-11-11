package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"sync"
)

const (
	SYNTAXFLOWSCAN_EXECUTING = "executing"
	SYNTAXFLOWSCAN_PAUSED    = "paused"
	SYNTAXFLOWSCAN_DONE      = "done"
	SYNTAXFLOWSCAN_ERROR     = "error"
)

var a = sync.Mutex{}

func SaveSyntaxFlowScanTask(db *gorm.DB, task *schema.SyntaxFlowScanTask) error {
	return db.Save(task).Error
}

func GetSyntaxFlowScanTaskById(db *gorm.DB, taskId string) (*schema.SyntaxFlowScanTask, error) {
	task := &schema.SyntaxFlowScanTask{}
	err := db.Where("task_id = ?", taskId).First(task).Error
	if err != nil {
		return nil, err
	}
	return task, nil
}

func DeleteSyntaxFlowScanTask(db *gorm.DB, taskId string) error {
	return db.Where("task_id = ?", taskId).Unscoped().Delete(&schema.SyntaxFlowScanTask{}).Error
}
