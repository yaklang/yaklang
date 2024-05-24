package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func CreateOrUpdateProgress(db *gorm.DB, runtimeId string, i interface{}) error {
	db = db.Model(&schema.Progress{})

	if db := db.Where("runtime_id = ?", runtimeId).Assign(i).FirstOrCreate(&schema.Progress{}); db.Error != nil {
		return utils.Errorf("create/update Progress failed: %s", db.Error)
	}

	return nil
}

func FilterProgress(db *gorm.DB, filter *ypb.UnfinishedTaskFilter) *gorm.DB {
	db = bizhelper.FuzzQueryLike(db, "task_name", filter.GetTaskName())
	db = bizhelper.FuzzQueryLike(db, "target", filter.GetTarget())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "progress_source", filter.GetProgressSource())
	db = bizhelper.ExactQueryStringArrayOr(db, "runtime_id", filter.GetRuntimeId())
	return db
}

func QueryProgress(db *gorm.DB, paging *ypb.Paging, filter *ypb.UnfinishedTaskFilter) (*bizhelper.Paginator, []*schema.Progress, error) {
	var ProgressList []*schema.Progress
	db = FilterProgress(db, filter)
	db = bizhelper.QueryOrder(db, paging.GetOrderBy(), paging.GetOrder())
	p, db := bizhelper.Paging(db, int(paging.GetPage()), int(paging.GetLimit()), &ProgressList)
	if db.Error != nil {
		return nil, nil, db.Error
	}
	return p, ProgressList, nil
}

func DeleteProgress(db *gorm.DB, filter *ypb.UnfinishedTaskFilter) ([]*schema.Progress, error) {
	var ProgressList []*schema.Progress
	db = FilterProgress(db, filter)
	db.Delete(&ProgressList)
	if db.Error != nil {
		return nil, db.Error
	}
	return ProgressList, nil
}

func GetProgressByRuntimeId(db *gorm.DB, runtimeId string) (*schema.Progress, error) {
	var p schema.Progress
	if db := db.Where("runtime_id = ?", runtimeId).First(&p); db.Error != nil {
		return nil, utils.Errorf("get Progress by runtimdId failed: %s", db.Error)
	}
	return &p, nil
}

func DeleteProgressByRuntimeId(db *gorm.DB, runtimeId string) (*schema.Progress, error) {
	var p schema.Progress
	if db := db.Where("runtime_id = ?", runtimeId).First(&p); db.Error != nil {
		return nil, utils.Errorf("get Progress by runtimdId failed: %s", db.Error)
	}
	db.Delete(&p)
	return &p, nil
}
