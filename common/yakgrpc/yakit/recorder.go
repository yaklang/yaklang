package yakit

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func CreateOrUpdateScreenRecorder(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&schema.ScreenRecorder{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&schema.ScreenRecorder{}); db.Error != nil {
		return utils.Errorf("create/update ScreenRecorder failed: %s", db.Error)
	}

	return nil
}

func GetScreenRecorder(db *gorm.DB, id int64) (*schema.ScreenRecorder, error) {
	var req schema.ScreenRecorder
	if db := db.Model(&schema.ScreenRecorder{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get ScreenRecorder failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteScreenRecorderByID(db *gorm.DB, id int64) error {
	if db := db.Model(&schema.ScreenRecorder{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&schema.ScreenRecorder{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func QueryScreenRecorder(db *gorm.DB, req *ypb.QueryScreenRecorderRequest) (*bizhelper.Paginator, []*schema.ScreenRecorder, error) {
	db = db.Model(&schema.ScreenRecorder{})
	if req == nil {
		return nil, nil, utils.Errorf("QueryScreenRecorderRequest is nil")
	}
	p := req.GetPagination()
	if p == nil {
		p = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	db = bizhelper.ExactQueryString(db, "project", req.GetProject())
	db = bizhelper.QueryOrder(db, p.GetOrderBy(), p.GetOrder())
	db = bizhelper.FuzzSearchEx(db, []string{
		"video_name", "note_info",
	}, req.Keywords, false)
	if len(req.Ids) > 0 {
		db = db.Where("id in (?)", req.Ids)
	}
	var ret []*schema.ScreenRecorder
	paging, db := bizhelper.Paging(db, int(p.GetPage()), int(p.GetLimit()), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return paging, ret, nil
}

/*func DeleteScreenRecorder(db *gorm.DB, req *ypb.QueryScreenRecorderRequest) error {
	db = db.Model(&ScreenRecorder{})
	db = bizhelper.ExactQueryString(db, "project", req.GetProject())
	db = bizhelper.FuzzSearchEx(db, []string{
		"video_name", "note_info",
	}, req.Keywords, false)
	if len(req.Ids) > 0 {
		db = db.Where("id in (?)", req.Ids)
	}
	db = db.Unscoped().Delete(&ScreenRecorder{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}*/

func BatchScreenRecorder(db *gorm.DB, ctx context.Context) chan *schema.ScreenRecorder {
	return bizhelper.YieldModel[*schema.ScreenRecorder](ctx, db)
}

func GetOneScreenRecorder(db *gorm.DB, req *ypb.GetOneScreenRecorderRequest) (*schema.ScreenRecorder, error) {
	db = db.Model(&schema.ScreenRecorder{})
	if req.Order == "desc" { // 上一条
		db = db.Where("id < ?", req.Id).Order("id desc").Limit(1)
	} else { // 下一条
		db = db.Where("id > ?", req.Id).Order("id asc").Limit(1)
	}
	var ret schema.ScreenRecorder
	db = db.Find(&ret)
	if db.Error != nil {
		return nil, utils.Errorf("GetOneScreenRecorder failed: %s", db.Error)
	}
	return &ret, nil
}

func IsExitScreenRecorder(db *gorm.DB, id int64, order string) (*schema.ScreenRecorder, error) {
	db = db.Model(&schema.ScreenRecorder{})
	if order == "desc" { // 上一条
		db = db.Where("id < ?", id).Order("id desc").Limit(1)
	} else { // 下一条
		db = db.Where("id > ?", id).Order("id asc").Limit(1)
	}
	var ret schema.ScreenRecorder
	db = db.Find(&ret)
	if db.Error != nil {
		return nil, utils.Errorf("IsExitScreenRecorder failed: %s", db.Error)
	}
	return &ret, nil
}

func DeleteScreenRecorder(db *gorm.DB, id int64) error {
	db = db.Model(&schema.ScreenRecorder{})
	db = db.Where("id = ?", id)
	db = db.Unscoped().Delete(&schema.ScreenRecorder{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}
