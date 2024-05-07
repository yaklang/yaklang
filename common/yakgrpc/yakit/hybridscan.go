package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	HYBRIDSCAN_EXECUTING = "executing"
	HYBRIDSCAN_PAUSED    = "paused"
	HYBRIDSCAN_DONE      = "done"
	HYBRIDSCAN_ERROR     = "error"
)

type HybridScanTask struct {
	gorm.Model

	TaskId string `gorm:"unique_index"`
	// executing
	// paused
	// done
	Status              string
	Reason              string // user cancel / finished / recover failed so on
	SurvivalTaskIndexes string // 暂停的时候正在执行的任务

	// struct{ https bool; request bytes }[]
	Targets string
	// string[]
	Plugins         string
	TotalTargets    int64
	TotalPlugins    int64
	TotalTasks      int64
	FinishedTasks   int64
	FinishedTargets int64

	ScanConfig []byte
}

func GetHybridScanByTaskId(db *gorm.DB, taskId string) (*HybridScanTask, error) {
	var task HybridScanTask
	err := db.Where("task_id = ?", taskId).First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func SaveHybridScanTask(db *gorm.DB, task *HybridScanTask) error {
	return db.Save(task).Error
}

func QueryHybridScan(db *gorm.DB, query *ypb.QueryHybridScanTaskRequest) (*bizhelper.Paginator, []*HybridScanTask, error) {
	db = db.Model(&HybridScanTask{})
	db = FilterHybridScan(db, query.GetFilter())
	var data []*HybridScanTask
	paging := query.GetPagination()
	db = bizhelper.QueryOrder(db, paging.GetOrderBy(), paging.GetOrder())
	p, db := bizhelper.Paging(db, int(paging.GetPage()), int(paging.GetLimit()), &data)
	if db.Error != nil {
		return nil, nil, db.Error
	}
	return p, data, nil
}

func FilterHybridScan(db *gorm.DB, filter *ypb.HybridScanTaskFilter) *gorm.DB {
	db = bizhelper.ExactQueryString(db, "status", filter.GetStatus())
	db = bizhelper.FuzzQuery(db, "target", filter.GetTarget())
	db = bizhelper.ExactQueryStringArrayOr(db, "task_id", filter.GetTaskId())
	if filter.GetFromId() > 0 {
		db = db.Where("id > ?", filter.GetFromId())
	}
	if filter.GetUntilId() > 0 {
		db = db.Where("id <= ?", filter.GetUntilId())
	}
	return db
}
