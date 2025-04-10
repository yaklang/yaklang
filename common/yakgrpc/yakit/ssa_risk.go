package yakit

import (
	"context"
	"path"
	"strings"
	"time"

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

type SsaRiskDatas struct {
	Data string
}

func GetSSARiskByFuzzy(db *gorm.DB, programName, sourceUrl, search string, level string) []string {
	var datas []*SsaRiskDatas
	var ret []string

	switch level {
	case "program":
		if db := db.Model(&schema.SSARisk{}).
			Where("program_name LIKE ?", "%"+search+"%").
			Select("`program_name` AS data").
			Group("`program_name`").
			Scan(&datas); db.Error != nil {
			utils.Errorf("get Risk by fuzzy search failed: %s", db.Error)
			return []string{}
		}
		for _, d := range datas {
			ret = append(ret, d.Data)
		}
	case "source":
		if db := db.Model(&schema.SSARisk{}).
			Where("program_name = ?", programName).
			Where("code_source_url LIKE ?", "%"+search+"%").
			Select("`code_source_url` AS data").
			Group("`code_source_url`").
			Scan(&datas); db.Error != nil {
			utils.Errorf("get Risk by fuzzy search failed: %s", db.Error)
			return []string{}
		}
		for _, d := range datas {
			ret = append(ret, d.Data)
		}
	case "function":
		if db := db.Model(&schema.SSARisk{}).
			Where("program_name = ?", programName).
			Where("code_source_url LIKE ?", "%"+sourceUrl+"%").
			Where("function_name LIKE ?", "%"+search+"%").
			Select("`function_name` AS data").
			Group("`function_name`").
			Scan(&datas); db.Error != nil {
			utils.Errorf("get Risk by fuzzy search failed: %s", db.Error)
			return []string{}
		}
		for _, d := range datas {
			ret = append(ret, d.Data)
		}
	}

	return ret
}

type SsaRiskCount struct {
	Prog   string
	Source string
	Func   string
	Count  int64
}

// 请求function无获取
func GetSSARiskByFuncName(db *gorm.DB, programName, sourceUrl, funcName string) ([]*SsaRiskCount, error) {
	var ret []*SsaRiskCount

	fullPath := path.Join("/", programName, sourceUrl)
	if db := db.Model(&schema.SSARisk{}).
		Where("program_name = ?", programName).
		Where("code_source_url LIKE ?", fullPath+"%").
		Where("function_name = ?", funcName).
		Select("`program_name` AS prog, `code_source_url` AS source, `function_name` AS func, COUNT(*) AS count").
		Group("`function_name`").
		Scan(&ret); db.Error != nil {
		return ret, utils.Errorf("get Risk failed: %s", db.Error)
	}
	return ret, nil
}

// 请求path获取function
func GetSSARiskBySourceUrl(db *gorm.DB, programName, sourceUrl string) ([]*SsaRiskCount, error) {
	var ret []*SsaRiskCount

	fullPath := path.Join("/", programName, sourceUrl)
	if db := db.Model(&schema.SSARisk{}).
		Where("program_name = ?", programName).
		Where("code_source_url LIKE ?", fullPath+"%").
		Select("`program_name` AS prog, `code_source_url` AS source, `function_name` AS func, COUNT(*) AS count").
		Group("`function_name`").
		Scan(&ret); db.Error != nil {
		return ret, utils.Errorf("get Risk failed: %s", db.Error)
	}

	return ret, nil
}

// 请求program获取path
func GetSSARiskByProgram(db *gorm.DB, programName string) ([]*SsaRiskCount, error) {
	var ret []*SsaRiskCount

	if db := db.Model(&schema.SSARisk{}).
		Where("program_name = ?", programName).
		Select("`program_name` AS prog, `code_source_url` AS source, `function_name` AS func, COUNT(*) AS count").
		Group("`code_source_url`").
		Scan(&ret); db.Error != nil {
		return ret, utils.Errorf("get Risk failed: %s", db.Error)
	}

	return ret, nil
}

// 请求root获取项目
func GetSSARiskByRoot(db *gorm.DB) ([]*SsaRiskCount, error) {
	var ret []*SsaRiskCount

	if db := db.Model(&schema.SSARisk{}).
		Select("`program_name` AS prog, `code_source_url` AS source, `function_name` AS func, COUNT(*) AS count").
		Group("`program_name`").
		Scan(&ret); db.Error != nil {
		return ret, utils.Errorf("get Risk failed: %s", db.Error)
	}

	return ret, nil
}

func FilterSSARisk(db *gorm.DB, filter *ypb.SSARisksFilter) *gorm.DB {
	db = bizhelper.ExactQueryInt64ArrayOr(db, "id", filter.GetID())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "program_name", filter.GetProgramName())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "function_name", filter.GetFunctionName())
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
		"id", "hash", // for exact query
		"program_name", "code_source_url",
		"risk_type", "severity", "from_rule", "tags",
	}, filter.GetSearch(), false)
	if filter.GetIsRead() != 0 {
		db = bizhelper.QueryByBool(db, "is_read", filter.GetIsRead() > 0)
	}
	if filter.GetAfterCreatedAt() > 0 {
		db = bizhelper.QueryByTimeRangeWithTimestamp(db, "created_at", filter.GetAfterCreatedAt(), time.Now().Add(10*time.Minute).Unix())
	}

	if filter.GetBeforeCreatedAt() > 0 {
		db = bizhelper.QueryByTimeRangeWithTimestamp(db, "created_at", 0, filter.GetBeforeCreatedAt())
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
