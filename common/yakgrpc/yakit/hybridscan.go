package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	HYBRIDSCAN_EXECUTING = "executing"
	HYBRIDSCAN_PAUSED    = "paused"
	HYBRIDSCAN_DONE      = "done"
	HYBRIDSCAN_ERROR     = "error"
)

func GetHybridScanByTaskId(db *gorm.DB, taskId string) (*schema.HybridScanTask, error) {
	var task schema.HybridScanTask
	err := db.Where("task_id = ?", taskId).First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func SaveHybridScanTask(db *gorm.DB, task *schema.HybridScanTask) error {
	return db.Save(task).Error
}

func QueryHybridScan(db *gorm.DB, query *ypb.QueryHybridScanTaskRequest) (*bizhelper.Paginator, []*schema.HybridScanTask, error) {
	db = db.Model(&schema.HybridScanTask{})
	db = FilterHybridScan(db, query.GetFilter())
	var data []*schema.HybridScanTask
	paging := query.GetPagination()
	db = bizhelper.QueryOrder(db, paging.GetOrderBy(), paging.GetOrder())
	p, db := bizhelper.Paging(db, int(paging.GetPage()), int(paging.GetLimit()), &data)
	if db.Error != nil {
		return nil, nil, db.Error
	}
	return p, data, nil
}

func FilterHybridScan(db *gorm.DB, filter *ypb.HybridScanTaskFilter) *gorm.DB {
	db = bizhelper.FuzzQueryLike(db, "targets", filter.GetTarget())
	db = bizhelper.ExactQueryStringArrayOr(db, "task_id", filter.GetTaskId())
	db = bizhelper.ExactQueryStringArrayOr(db, "status", filter.GetStatus())
	db = bizhelper.ExactQueryStringArrayOr(db, "hybrid_scan_task_source", filter.GetHybridScanTaskSource())
	if filter.GetFromId() > 0 {
		db = db.Where("id > ?", filter.GetFromId())
	}
	if filter.GetUntilId() > 0 {
		db = db.Where("id <= ?", filter.GetUntilId())
	}
	return db
}
