package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type Progress struct {
	gorm.Model
	RuntimeId            string
	CurrentProgress      float64
	YakScriptOnlineGroup string
	// 记录指针
	LastRecordPtr int64
	TaskName      string
	// 额外信息
	ExtraInfo string

	ProgressSource string

	// 任务记录的参数
	ProgressTaskParam []byte

	// 目标 大部分的progress都应该有制定目标，所以尝试提取出来作为单独的数据使用
	Target string
}

func CreateOrUpdateProgress(db *gorm.DB, runtimeId string, i interface{}) error {
	db = db.Model(&Progress{})

	if db := db.Where("runtime_id = ?", runtimeId).Assign(i).FirstOrCreate(&Progress{}); db.Error != nil {
		return utils.Errorf("create/update Progress failed: %s", db.Error)
	}

	return nil
}

func FilterProgress(db *gorm.DB, filter *ypb.UnfinishedTaskFilter) *gorm.DB {
	db = bizhelper.FuzzQueryLike(db, "task_name", filter.GetTaskName())
	db = bizhelper.FuzzQueryLike(db, "target", filter.GetTarget())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "progress_source", filter.GetProgressSource())
	db = bizhelper.ExactQueryString(db, "runtime_id", filter.GetRuntimeId())
	return db
}

func QueryProgress(db *gorm.DB, paging *ypb.Paging, filter *ypb.UnfinishedTaskFilter) (*bizhelper.Paginator, []*Progress, error) {
	var ProgressList []*Progress
	db = FilterProgress(db, filter)
	db = bizhelper.QueryOrder(db, paging.GetOrderBy(), paging.GetOrder())
	p, db := bizhelper.Paging(db, int(paging.GetPage()), int(paging.GetLimit()), &ProgressList)
	if db.Error != nil {
		return nil, nil, db.Error
	}
	return p, ProgressList, nil
}

func DeleteProgress(db *gorm.DB, filter *ypb.UnfinishedTaskFilter) ([]*Progress, error) {
	var ProgressList []*Progress
	db = FilterProgress(db, filter)
	db.Delete(&ProgressList)
	if db.Error != nil {
		return nil, db.Error
	}
	return ProgressList, nil
}

func GetProgressByRuntimeId(db *gorm.DB, runtimeId string) (*Progress, error) {
	var p Progress
	if db := db.Where("runtime_id = ?", runtimeId).First(&p); db.Error != nil {
		return nil, utils.Errorf("get Progress by runtimdId failed: %s", db.Error)
	}
	return &p, nil
}

func DeleteProgressByRuntimeId(db *gorm.DB, runtimeId string) (*Progress, error) {
	var p Progress
	if db := db.Where("runtime_id = ?", runtimeId).First(&p); db.Error != nil {
		return nil, utils.Errorf("get Progress by runtimdId failed: %s", db.Error)
	}
	db.Delete(&p)
	return &p, nil
}
