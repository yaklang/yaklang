package yakit

import (
	"context"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
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
	VideoName string
	Cover string `gorm:"type:longtext"`
	Duration string
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

	if db := db.Debug().Where("hash = ?", hash).Assign(i).FirstOrCreate(&ScreenRecorder{}); db.Error != nil {
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
	db = bizhelper.FuzzSearchEx(db, []string{
		"video_name", "note_info",
	}, req.Keywords, false)
	if len(req.Ids) > 0 {
		db = db.Where("id in (?)", req.Ids)
	}
	var ret []*ScreenRecorder
	paging, db := bizhelper.Paging(db, int(params.GetPage()), int(params.GetLimit()), &ret)
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

func BatchScreenRecorder(db *gorm.DB, ctx context.Context) chan *ScreenRecorder {
	outC := make(chan *ScreenRecorder)
	db = db.Model(&ScreenRecorder{})
	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*ScreenRecorder
			if _, b := bizhelper.NewPagination(&bizhelper.Param{
				DB:    db,
				Page:  page,
				Limit: 1000,
			}, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 1000 {
				return
			}
		}
	}()
	return outC
}


func GetOneScreenRecorder(db *gorm.DB, req *ypb.GetOneScreenRecorderRequest) (*ScreenRecorder, error) {
	db = db.Model(&ScreenRecorder{})
	if req.Order == "desc" { // 上一条
		db = db.Where("id < ?", req.Id).Order("id desc").Limit(1)
	} else { // 下一条
		db = db.Where("id > ?", req.Id).Order("id asc").Limit(1)
	}
	var ret ScreenRecorder
	db = db.Find(&ret)
	if db.Error != nil {
		return nil, utils.Errorf("GetOneScreenRecorder failed: %s", db.Error)
	}
	return  &ret, nil
}

func IsExitScreenRecorder(db *gorm.DB, id int64, order string) (*ScreenRecorder, error) {
	db = db.Model(&ScreenRecorder{})
	if order == "desc" { // 上一条
		db = db.Where("id < ?", id).Order("id desc").Limit(1)
	} else { // 下一条
		db = db.Where("id > ?", id).Order("id asc").Limit(1)
	}
	var ret ScreenRecorder
	db = db.Find(&ret)
	if db.Error != nil {
		return nil, utils.Errorf("IsExitScreenRecorder failed: %s", db.Error)
	}
	return  &ret, nil
}

func DeleteScreenRecorder(db *gorm.DB, id int64) error {
	db = db.Model(&ScreenRecorder{})
	db = db.Where("id = ?", id)
	db = db.Unscoped().Delete(&ScreenRecorder{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}
