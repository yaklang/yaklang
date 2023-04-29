package yakit

import (
	"github.com/jinzhu/gorm"
	"yaklang/common/utils"
	"yaklang/common/utils/bizhelper"
	"yaklang/common/yakgrpc/ypb"
	"strings"
)

type ExtractedData struct {
	gorm.Model

	// sourcetype 一般来说是标注数据来源
	SourceType string `gorm:"index"`

	// trace id 表示数据源的 ID
	TraceId string `gorm:"index"`

	// 提取数据的正则数据
	Regexp string

	// 规则 Verbose
	RuleVerbose string

	// UTF8 safe escape
	Data string
}

func CreateOrUpdateExtractedData(db *gorm.DB, mainId int64, i interface{}) error {
	if mainId <= 0 {
		if db := db.Model(&ExtractedData{}).Save(i); db.Error != nil {
			return db.Error
		}
		return nil
	}
	db = db.Model(&ExtractedData{})

	if db := db.Where("id = ?", mainId).Assign(i).FirstOrCreate(&ExtractedData{}); db.Error != nil {
		return utils.Errorf("create/update ExtractedData failed: %s", db.Error)
	}

	return nil
}

func GetExtractedData(db *gorm.DB, id int64) (*ExtractedData, error) {
	var req ExtractedData
	if db := db.Model(&ExtractedData{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get ExtractedData failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteExtractedDataByID(db *gorm.DB, id int64) error {
	if db := db.Model(&ExtractedData{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&ExtractedData{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func QueryExtractedData(db *gorm.DB, req *ypb.QueryMITMRuleExtractedDataRequest) (*bizhelper.Paginator, []*ExtractedData, error) {
	db = db.Model(&ExtractedData{})

	params := req.GetPagination()

	db = bizhelper.QueryOrder(db, params.OrderBy, params.Order)

	var ret []*ExtractedData
	paging, db := bizhelper.Paging(db, int(params.GetPage()), int(params.GetLimit()), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func SaveExtractedDataFromHTTPFlow(db *gorm.DB, flowHash string, ruleName string, data string, regexpStr ...string) error {
	var r string
	if len(regexpStr) > 0 {
		r = strings.Join(regexpStr, ", ")
	}
	extractData := &ExtractedData{
		SourceType:  "httpflow",
		TraceId:     flowHash,
		Regexp:      r,
		RuleVerbose: ruleName,
		Data:        data,
	}
	return CreateOrUpdateExtractedData(db, -1, extractData)
}
