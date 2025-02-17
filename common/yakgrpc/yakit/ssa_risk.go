package yakit

import (
	"context"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//func DeleteRiskByProgram(DB *gorm.DB, programNames []string) error {
//	db := DB.Model(&schema.Risk{})
//	db = bizhelper.ExactOrQueryStringArrayOr(db, "program_name", programNames)
//	if db := db.Unscoped().Delete(&schema.Risk{}); db.Error != nil {
//		return db.Error
//	}
//	return nil
//}

func DeleteSSARiskBySFResult(DB *gorm.DB, resultIDs []int64) error {
	db := DB.Model(&schema.SSARisk{})
	db = bizhelper.ExactQueryInt64ArrayOr(db, "result_id", resultIDs)
	if db := db.Unscoped().Delete(&schema.SSARisk{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func CreateSSARisk(DB *gorm.DB, r *schema.SSARisk) error {
	if db := DB.Create(r); db.Error != nil {
		return db.Error
	}
	return nil
}

func GetSSARiskByID(db *gorm.DB, id int64) (*schema.SSARisk, error) {
	var r schema.SSARisk
	if db := db.Model(&schema.SSARisk{}).Where("id = ?", id).First(&r); db.Error != nil {
		return nil, utils.Errorf("get Risk failed: %s", db.Error)
	}
	return &r, nil
}

func GetSSARiskByHash(db *gorm.DB, hash string) (*schema.SSARisk, error) {
	var r schema.SSARisk
	db = FilterSSARisk(db, &ypb.SSARisksFilter{
		Hash: []string{hash},
	})
	if db := db.First(&r); db.Error != nil {
		return nil, utils.Errorf("get Risk failed: %s", db.Error)
	}
	return &r, nil
}

func GetSSARiskByFuncName(db *gorm.DB, programName, path, funcName string) ([]*schema.SSARisk, error) {
	var r []*schema.SSARisk
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	path = "/" + programName + path

	if db := db.Model(&schema.SSARisk{}).
		Where("program_name = ?", programName).
		Where("code_source_url LIKE ?", path+"%").
		Where("function_name = ?", funcName).Find(&r); db.Error != nil {
		return nil, utils.Errorf("get Risk failed: %s", db.Error)
	}
	return r, nil
}

func GetSSARiskByPath(db *gorm.DB, programName, path string) ([]*schema.SSARisk, error) {
	var r []*schema.SSARisk
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	path = "/" + programName + path

	if db := db.Model(&schema.SSARisk{}).
		Where("program_name = ?", programName).
		Where("code_source_url LIKE ?", path+"%").Find(&r); db.Error != nil {
		return nil, utils.Errorf("get Risk failed: %s", db.Error)
	}
	return r, nil
}

func GetSSARiskByProgram(db *gorm.DB, programName string) ([]*schema.SSARisk, error) {
	var r []*schema.SSARisk
	if db := db.Model(&schema.SSARisk{}).
		Where("program_name = ?", programName).Find(&r); db.Error != nil {
		return nil, utils.Errorf("get Risk failed: %s", db.Error)
	}
	return r, nil
}
func GetSSARisk(db *gorm.DB, programName, path, funcName string) ([]*schema.SSARisk, error) {
	var r []*schema.SSARisk
	var err error
	if funcName != "" {
		r, err = GetSSARiskByFuncName(db, programName, path, funcName)
		if err != nil {
			return nil, err
		}
	} else if path != "" {
		r, err = GetSSARiskByPath(db, programName, path)
		if err != nil {
			return nil, err
		}
	} else if programName != "" {
		r, err = GetSSARiskByProgram(db, programName)
		if err != nil {
			return nil, err
		}
	} else {
		if db := db.Model(&schema.SSARisk{}).Find(&r); db.Error != nil {
			return nil, utils.Errorf("get Risk failed: %s", db.Error)
		}
	}

	return r, nil
}

func GetCount(db *gorm.DB, source, funcName string) (int, error) {
	var c int
	if db := db.Model(&schema.SSARisk{}).Where("code_source_url = ?", source).Where("function_name = ?", funcName).Count(&c); db.Error != nil {
		return -1, utils.Errorf("get Risk failed: %s", db.Error)
	}
	return c, nil
}

func FilterSSARisk(db *gorm.DB, filter *ypb.SSARisksFilter) *gorm.DB {
	db = bizhelper.ExactQueryInt64ArrayOr(db, "id", filter.GetID())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "program_name", filter.GetProgramName())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "code_source_url", filter.GetCodeSourceUrl())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "risk_type", filter.GetRiskType())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "severity", filter.GetSeverity())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "from_rule", filter.GetFromRule())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "runtime_id", filter.GetRuntimeID())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "hash", filter.GetHash())
	db = bizhelper.ExactQueryUint64ArrayOr(db, "result_id", filter.GetResultID())
	db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{"tags"}, filter.GetTags(), false)
	db = bizhelper.FuzzSearchEx(db, []string{"title", "title_verbose"}, filter.GetTitle(), false)
	db = bizhelper.FuzzSearchEx(db, []string{
		"program_name", "code_source_url",
		"risk_type", "severity", "from_rule", "tags",
	}, filter.GetSearch(), false)
	if filter.GetIsRead() != 0 {
		db = bizhelper.QueryByBool(db, "is_read", filter.GetIsRead() > 0)
	}
	return db
}

func QuerySSARisk(db *gorm.DB, filter *ypb.SSARisksFilter, paging *ypb.Paging) (*bizhelper.Paginator, []*schema.SSARisk, error) {
	if filter == nil {
		return nil, nil, utils.Errorf("empty filter")
	}
	if paging == nil {
		paging = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	var risks []*schema.SSARisk

	db = db.Model(&schema.SSARisk{})
	db = FilterSSARisk(db, filter)
	queryPaging, queryDb := bizhelper.YakitPagingQuery(db, paging, &risks)
	if queryDb.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return queryPaging, risks, nil
}

func DeleteSSARisks(DB *gorm.DB, filter *ypb.SSARisksFilter) error {
	db := DB.Model(&schema.SSARisk{})
	db = FilterSSARisk(db, filter)
	if db := db.Unscoped().Delete(&schema.SSARisk{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func UpdateSSARiskTags(DB *gorm.DB, id int64, tags []string) error {
	db := DB.Model(&schema.SSARisk{})
	if db := db.Where("id = ?", id).Update("tags", strings.Join(tags, "|")); db.Error != nil {
		return db.Error
	}
	return nil
}

func SSARiskColumnGroupCount(db *gorm.DB, column string) []*ypb.FieldGroup {
	return bizhelper.GroupCount(db, "ssa_risks", column)
}

func NewSSARiskReadRequest(db *gorm.DB, filter *ypb.SSARisksFilter) error {
	db = db.Model(&schema.SSARisk{})
	if filter != nil {
		db = FilterSSARisk(db, filter)
	}
	db = db.UpdateColumn("is_read", true)
	if db.Error != nil {
		return utils.Errorf("NewSSARiskReadRequest failed %s", db.Error)
	}
	return nil
}

func QuerySSARiskCount(DB *gorm.DB, filter *ypb.SSARisksFilter) (int, error) {
	db := DB.Model(&schema.SSARisk{})
	db = FilterSSARisk(db, filter)
	var count int
	db = db.Count(&count)
	return count, db.Error
}

func YieldSSARisk(db *gorm.DB, ctx context.Context) chan *schema.SSARisk {
	return bizhelper.YieldModel[*schema.SSARisk](ctx, db, bizhelper.WithYieldModel_PageSize(100))
}
