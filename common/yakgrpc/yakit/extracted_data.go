package yakit

import (
	"context"
	"net/http"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/dlclark/regexp2"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type PacketInfo struct {
	IsRequest     bool
	GzipHeader    string
	ChunkedHeader string
	Method        string
	RequestURI    string
	Proto         string
	Headers       [][2]string
	Cookies       []*http.Cookie
	HeaderRaw     string
	BodyRaw       []byte
	Raw           []byte
}
type MatchMetaInfo struct {
	Raw    []byte
	Offset int
}
type MatchResult struct {
	*regexp2.Match
	IsMatchRequest bool
	MatchResult    string
	MetaInfo       *MatchMetaInfo
}

func CreateOrUpdateExtractedData(db *gorm.DB, mainId int64, i interface{}) error {
	if mainId <= 0 {
		if db := db.Model(&schema.ExtractedData{}).Save(i); db.Error != nil {
			return db.Error
		}
		return nil
	}
	db = db.Model(&schema.ExtractedData{})

	if db := db.Where("id = ?", mainId).Assign(i).FirstOrCreate(&schema.ExtractedData{}); db.Error != nil {
		return utils.Errorf("create/update ExtractedData failed: %s", db.Error)
	}

	return nil
}

func CreateOrUpdateExtractedDataEx(mainId int64, i interface{}) error {
	if consts.GLOBAL_DB_SAVE_SYNC.IsSet() {
		return CreateOrUpdateExtractedData(consts.GetGormProjectDatabase(), mainId, i)
	} else {
		DBSaveAsyncChannel <- func(db *gorm.DB) error {
			return CreateOrUpdateExtractedData(db, mainId, i)
		}
		return nil
	}
}

func GetExtractedData(db *gorm.DB, id int64) (*schema.ExtractedData, error) {
	var req schema.ExtractedData
	if db := db.Model(&schema.ExtractedData{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get ExtractedData failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteExtractedDataByID(db *gorm.DB, id int64) error {
	if db := db.Model(&schema.ExtractedData{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&schema.ExtractedData{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func FilterExtractedData(db *gorm.DB, filter *ypb.ExtractedDataFilter) *gorm.DB {
	if filter == nil {
		return db
	}
	db = bizhelper.ExactQueryStringArrayOr(db, "trace_id", filter.GetTraceID())
	db = bizhelper.ExactQueryStringArrayOr(db, "rule_verbose", filter.GetRuleVerbose())
	return db
}
func QueryExtractedDataOnlyName(db *gorm.DB) ([]*schema.ExtractedData, error) {
	var result []*schema.ExtractedData
	db = db.Select("rule_verbose,trace_id").Find(&result)
	if db.Error != nil {
		return nil, utils.Errorf("select rule_verbose fail: %s", db.Error)
	}
	return result, nil
}
func QueryExtractedDataPagination(db *gorm.DB, req *ypb.QueryMITMRuleExtractedDataRequest) (*bizhelper.Paginator, []*schema.ExtractedData, error) {
	db = db.Model(&schema.ExtractedData{})
	filter := req.GetFilter()
	if filter == nil {
		if req.GetHTTPFlowHiddenIndex() != "" {
			filter = &ypb.ExtractedDataFilter{
				TraceID: []string{req.GetHTTPFlowHiddenIndex()},
			}
		} else if req.GetHTTPFlowHash() != "" {
			filter = &ypb.ExtractedDataFilter{
				TraceID: []string{req.GetHTTPFlowHash()},
			}
		}
	}

	db = FilterExtractedData(db, filter)
	if req.OnlyName {
		result, err := QueryExtractedDataOnlyName(db)
		if err != nil {
			return nil, nil, err
		}
		return &bizhelper.Paginator{
			TotalRecord: len(result),
		}, result, nil
	}
	params := req.GetPagination()
	if params == nil {
		params = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	db = bizhelper.QueryOrder(db, params.OrderBy, params.Order)
	var ret []*schema.ExtractedData
	paging, db := bizhelper.Paging(db, int(params.GetPage()), int(params.GetLimit()), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return paging, ret, nil
}

func CountExtractedData(db *gorm.DB, filter *ypb.ExtractedDataFilter) (float64, error) {
	db = db.Model(&schema.ExtractedData{})
	db = FilterExtractedData(db, filter)
	var count float64
	if db := db.Count(&count); db.Error != nil {
		return 0, db.Error
	}
	return count, nil
}

func ExtractedDataFromHTTPFlow(hiddenIndex string, ruleName string, res *MatchResult, regexpStr ...string) *schema.ExtractedData {
	var r string
	if len(regexpStr) > 0 {
		r = strings.Join(regexpStr, ", ")
	}

	extractData := &schema.ExtractedData{
		SourceType:     "httpflow",
		TraceId:        hiddenIndex,
		Regexp:         r,
		RuleVerbose:    ruleName,
		Data:           res.MatchResult,
		DataIndex:      res.Index + res.MetaInfo.Offset,
		Length:         res.Length,
		IsMatchRequest: res.IsMatchRequest,
	}
	return extractData
}

func BatchExtractedData(db *gorm.DB, ctx context.Context) chan *schema.ExtractedData {
	return bizhelper.YieldModel[*schema.ExtractedData](ctx, db)
}

func DeleteExtractedDataByTraceIds(db *gorm.DB, hiddenIndex []string) error {
	db = db.Model(&schema.ExtractedData{}).Where("source_type == 'httpflow' ")
	db = bizhelper.ExactQueryStringArrayOr(db, "trace_id", hiddenIndex)
	db = db.Unscoped().Delete(&schema.ExtractedData{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteExtractedDataByTraceId(db *gorm.DB, hiddenIndex string) error {
	if internalDb := db.Model(&schema.ExtractedData{}).Where("trace_id = ?", hiddenIndex).Unscoped().Delete(&schema.ExtractedData{}); internalDb.Error != nil {
		return db.Error
	}
	return nil
}

func DropExtractedDataTable(db *gorm.DB) {
	db.DropTableIfExists(&schema.ExtractedData{})
	if db := db.Exec(`UPDATE SQLITE_SEQUENCE SET SEQ=0 WHERE NAME='extracted_data';`); db.Error != nil {
		log.Errorf("update sqlite sequence failed: %s", db.Error)
	}
	db.AutoMigrate(&schema.ExtractedData{})
}
