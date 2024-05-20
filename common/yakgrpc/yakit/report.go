package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func CreateOrUpdateReportRecord(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&schema.ReportRecord{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&schema.ReportRecord{}); db.Error != nil {
		return utils.Errorf("create/update ReportRecord failed: %s", db.Error)
	}

	return nil
}

func GetReportRecord(db *gorm.DB, id int64) (*schema.ReportRecord, error) {
	var req schema.ReportRecord
	if db := db.Model(&schema.ReportRecord{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get ReportRecord failed: %s", db.Error)
	}

	return &req, nil
}

func GetReportRecordByHash(db *gorm.DB, id string) (*schema.ReportRecord, error) {
	var req schema.ReportRecord
	if db := db.Model(&schema.ReportRecord{}).Where("hash = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get ReportRecord failed: %s", db.Error)
	}
	return &req, nil
}

func DeleteReportRecordByID(db *gorm.DB, id int64) error {
	if db := db.Model(&schema.ReportRecord{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&schema.ReportRecord{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteReportRecordByIDs(db *gorm.DB, ids ...int64) error {
	if len(ids) == 1 {
		id := ids[0]
		if db := db.Model(&schema.ReportRecord{}).Where(
			"id = ?", id,
		).Unscoped().Delete(&schema.ReportRecord{}); db.Error != nil {
			return db.Error
		}
		return nil
	}

	if db = bizhelper.ExactQueryInt64ArrayOr(db, "id", ids).Unscoped().Delete(&schema.ReportRecord{}); db.Error != nil {
		return utils.Errorf("delete id(s) failed: %v", db.Error)
	}

	return nil
}

func DeleteReportRecordByHash(db *gorm.DB, id string) error {
	if db := db.Model(&schema.ReportRecord{}).Where(
		"hash = ?", id,
	).Unscoped().Delete(&schema.ReportRecord{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func FilterReportRecord(db *gorm.DB, params *ypb.QueryReportsRequest) *gorm.DB {
	db = bizhelper.FuzzSearchEx(db, []string{
		"title", "owner", "`from`", `quoted_json`,
	}, params.GetKeyword(), false)
	db = bizhelper.FuzzQueryStringArrayOrPrefixLike(
		db, "owner",
		utils.PrettifyListFromStringSplitEx(params.GetOwner()),
	)
	db = bizhelper.FuzzQueryStringArrayOrPrefixLike(
		db, "`from`",
		utils.PrettifyListFromStringSplitEx(params.GetFrom()),
	)
	db = bizhelper.FuzzQueryStringArrayOrPrefixLike(db, "title", utils.PrettifyListFromStringSplitEx(params.GetTitle()))
	return db
}

func QueryReportRecord(db *gorm.DB, params *ypb.QueryReportsRequest) (*bizhelper.Paginator, []*schema.ReportRecord, error) {
	db = db.Table("report_records").Select("id,created_at,updated_at,deleted_at,title,published_at,hash,owner,`from`")
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

	db = FilterReportRecord(db, params)
	var ret []*schema.ReportRecord
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func NewReport() *schema.Report {
	return &schema.Report{}
}

var ReportExports = map[string]interface{}{
	"New": NewReport,
}
