package yakit

import (
	"context"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"net/http"
	"strings"

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
type MatchInfo struct {
	Raw    []byte
	Offset int
}
type MatchResult struct {
	*regexp2.Match
	IsMatchRequest bool
	MatchInfo      *MatchInfo
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

func ExtractedDataFromHTTPFlow(hiddenIndex string, ruleName string, matchResult *MatchResult, data string, regexpStr ...string) *schema.ExtractedData {
	var r string
	if len(regexpStr) > 0 {
		r = strings.Join(regexpStr, ", ")
	}

	extractData := &schema.ExtractedData{
		SourceType:     "httpflow",
		TraceId:        hiddenIndex,
		Regexp:         r,
		RuleVerbose:    ruleName,
		Data:           data,
		DataIndex:      matchResult.Index + matchResult.MatchInfo.Offset,
		Length:         matchResult.Length,
		IsMatchRequest: matchResult.IsMatchRequest,
	}
	return extractData
}

func BatchExtractedData(db *gorm.DB, ctx context.Context) chan *schema.ExtractedData {
	outC := make(chan *schema.ExtractedData)
	go func() {
		defer close(outC)

		page := 1
		for {
			var items []*schema.ExtractedData
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
