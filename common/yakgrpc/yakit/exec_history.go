package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
)

var (
	updateExecHistoryLock = new(sync.Mutex)
)

func CreateOrUpdateExecHistory(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&schema.ExecHistory{})

	updateExecHistoryLock.Lock()
	defer updateExecHistoryLock.Unlock()
	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&schema.ExecHistory{}); db.Error != nil {
		return utils.Errorf("create/update ExecHistory failed: %s", db.Error)
	}

	return nil
}

func GetExecHistory(db *gorm.DB, id int64) (*schema.ExecHistory, error) {
	var req schema.ExecHistory
	if db := db.Model(&schema.ExecHistory{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get ExecHistory failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteExecHistoryByID(db *gorm.DB, id int64) error {
	if db := db.Model(&schema.ExecHistory{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&schema.ExecHistory{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteExecHistoryAll(db *gorm.DB) error {
	if db := db.Model(&schema.ExecHistory{}).Where("id > 0").Unscoped().Delete(&schema.ExecHistory{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func QueryExecHistory(db *gorm.DB, params *ypb.ExecHistoryRequest) (*bizhelper.Paginator, []*schema.ExecHistory, error) {
	if params == nil {
		params = &ypb.ExecHistoryRequest{}
	}

	originDB := db

	db = db.Select("id,created_at,updated_at,deleted_at,script,script_id,timestamp_nano," +
		"duration_ms,params,ok,reason,runtime_id,from_yak_module," +
		"length(stdout) as stdout_len,length(stderr) as stderr_len").Table("exec_histories")

	if params.Pagination == nil {
		params.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	p := params.Pagination
	db = bizhelper.QueryOrder(db, p.OrderBy, p.Order)

	var scriptName = params.GetYakScriptName()
	if scriptName == "" && params.GetYakScriptId() > 0 {
		s, _ := GetYakScript(originDB, params.GetYakScriptId())
		if s != nil {
			scriptName = s.ScriptName
		}
	}
	db = bizhelper.ExactQueryString(db, "from_yak_module", scriptName)

	var ret []*schema.ExecHistory
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}
