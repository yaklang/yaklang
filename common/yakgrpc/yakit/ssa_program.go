package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"time"
)

func CreateSsaProgram(db *gorm.DB, program *schema.SSAProgram) error {
	result := db.Model(&schema.SSAProgram{}).Create(program)
	return result.Error
}
func UpdateSsaProgram(db *gorm.DB, program *schema.SSAProgram) error {
	result := db.Model(&schema.SSAProgram{}).UpdateColumn("description", program.Description)
	return result.Error
}
func DeleteSsaProgramWithName(db *gorm.DB, name string) error {
	ssadb.DeleteProgram(ssadb.GetDB(), name)
	result := db.Model(&schema.SSAProgram{}).Where("name=?", name).Unscoped().Delete(&schema.SSAProgram{})
	return result.Error
}
func DeleteSsaProgram(db *gorm.DB, request *ypb.DeleteSsaProgramRequest) error {
	var programs []*schema.SSAProgram
	db = db.Model(&schema.SSAProgram{})
	if !request.IsAll {
		ids := request.GetId()
		raw := make([]interface{}, len(ids))
		for index, id := range ids {
			raw[index] = id
		}
		db = bizhelper.ExactQueryArrayOr(db.Model(&schema.SSAProgram{}), "id", raw)
		db = FilterSsaProgram(db, request.Filter)
	}
	queryResult := db.Select("name").Table("ssa_programs").Find(&programs)
	if queryResult.Error != nil {
		log.Errorf("query ssa program fail: %s", queryResult.Error)
	}
	result := db.Unscoped().Delete(&schema.SSAProgram{})
	for _, prog := range programs {
		ssadb.DeleteProgram(ssadb.GetDB(), prog.Name)
	}
	return result.Error
}
func QuerySsaProgram(db *gorm.DB, request *ypb.QuerySsaProgramRequest) (*bizhelper.Paginator, []*schema.SSAProgram, error) {
	defer func() {
		if msg := recover(); msg != nil {
			log.Errorf("query ssa program fail: %s", msg)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	db = BuildQuerySsaProgram(db.Model(&schema.SSAProgram{}), request)
	var programs []*schema.SSAProgram
	if request.Offset != 0 {
		if request.Paging.Order == "desc" {
			db = db.Where("id < ?", request.Offset)
		} else {
			db = db.Where("id > ?", request.Offset)
		}
	}
	paging, dbx := bizhelper.Paging(db, int(request.Paging.Page), int(request.Paging.Limit), &programs)
	if dbx.Error != nil {
		return nil, nil, utils.Errorf("select ssa program fail: %s", dbx.Error)
	}
	return paging, programs, nil
}
func BuildQuerySsaProgram(db *gorm.DB, request *ypb.QuerySsaProgramRequest) *gorm.DB {
	if request.Paging == nil {
		request.Paging = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	p := request.Paging
	if p.OrderBy == "" {
		p.OrderBy = "id"
	}
	db = bizhelper.QueryOrder(db, p.OrderBy, p.Order)
	if request.GetIsAll() {
		return db
	}
	if request.Id != 0 {
		db = db.Where("id=?", request.Id)
	}
	db = FilterSsaProgram(db, request.Filter)
	return db
}
func FilterSsaProgram(db *gorm.DB, params *ypb.SsaProgramFilter) *gorm.DB {
	db = db.Model(&schema.SSAProgram{})
	if params == nil {
		params = &ypb.SsaProgramFilter{}
	}
	if params.GetBeforeUpdateAt() > 0 {
		db = bizhelper.QueryByTimeRangeWithTimestamp(db, "updated_at", 0, params.GetBeforeUpdateAt())
	}
	if params.GetAfterUpdateAt() > 0 {
		db = bizhelper.QueryByTimeRangeWithTimestamp(db, "updated_at", params.GetAfterUpdateAt(), time.Now().Add(10*time.Minute).Unix())
	}
	if params.GetLanguage() != "" {
		db = db.Where("language=?", params.GetLanguage())
	}
	if params.GetEngineVersion() != "" {
		db = db.Where("engine_version=?", params.GetEngineVersion())
	}
	db = bizhelper.FuzzSearchEx(db, []string{"description"}, params.GetKeyword(), false)
	if params.GetProgramName() != "" {
		db = db.Where("name=?", params.GetProgramName())
	}
	return db
}
