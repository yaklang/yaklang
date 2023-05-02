package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type ScreenRecorder struct {
	gorm.Model

	// 保存到本地的路径
	Filename string
	NoteInfo string
	Project  string

	Hash string `json:"hash" gorm:"unique_index"`
}

func (s *ScreenRecorder) CalcHash() string {
	s.Hash = utils.CalcSha1(s.Filename, s.Project)
	return s.Hash
}

func (s *ScreenRecorder) BeforeSave() error {
	s.Hash = s.CalcHash()
	return nil
}

func CreateOrUpdateScreenRecorder(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&ScreenRecorder{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&ScreenRecorder{}); db.Error != nil {
		return utils.Errorf("create/update ScreenRecorder failed: %s", db.Error)
	}

	return nil
}

func GetScreenRecorder(db *gorm.DB, id int64) (*ScreenRecorder, error) {
	var req ScreenRecorder
	if db := db.Model(&ScreenRecorder{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get ScreenRecorder failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteScreenRecorderByID(db *gorm.DB, id int64) error {
	if db := db.Model(&ScreenRecorder{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&ScreenRecorder{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func QueryScreenRecorder(db *gorm.DB, req *ypb.QueryScreenRecorderRequest) (*bizhelper.Paginator, []*ScreenRecorder, error) {
	db = db.Model(&ScreenRecorder{})

	params := req.GetPagination()

	db = bizhelper.ExactQueryString(db, "project", req.GetProject())
	db = bizhelper.QueryOrder(db, params.GetOrderBy(), params.GetOrder())

	var ret []*ScreenRecorder
	paging, db := bizhelper.Paging(db, int(params.GetPage()), int(params.GetLimit()), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}
