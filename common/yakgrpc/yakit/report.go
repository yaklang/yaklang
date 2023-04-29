package yakit

import (
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"yaklang/common/consts"
	"yaklang/common/go-funk"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/bizhelper"
	"yaklang/common/yakgrpc/ypb"
	"strconv"
	"time"
)

type ReportRecord struct {
	gorm.Model

	Title       string
	PublishedAt time.Time `json:"published_at"`
	Hash        string    `json:"hash" gorm:"unique_index"`
	Owner       string    `json:"owner"`
	From        string    `json:"from"`
	QuotedJson  string    `json:"quoted_json"`
}

func (i *ReportRecord) ToGRPCModel() *ypb.Report {
	unquoted, err := strconv.Unquote(i.QuotedJson)
	if err != nil {
		unquoted = i.QuotedJson
	}
	return &ypb.Report{
		Title:       i.Title,
		PublishedAt: uint64(i.PublishedAt.Unix()),
		Hash:        i.Hash,
		Id:          uint64(i.ID),
		Owner:       i.Owner,
		From:        i.From,
		JsonRaw:     unquoted,
	}
}

func (r *ReportRecord) CalcHash() string {
	return utils.CalcSha1(r.Title, r.PublishedAt.Format(utils.DefaultTimeFormat))
}

func (r *ReportRecord) BeforeSave() {
	r.Hash = r.CalcHash()
}

func CreateOrUpdateReportRecord(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&ReportRecord{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&ReportRecord{}); db.Error != nil {
		return utils.Errorf("create/update ReportRecord failed: %s", db.Error)
	}

	return nil
}

func GetReportRecord(db *gorm.DB, id int64) (*ReportRecord, error) {
	var req ReportRecord
	if db := db.Model(&ReportRecord{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get ReportRecord failed: %s", db.Error)
	}

	return &req, nil
}

func GetReportRecordByHash(db *gorm.DB, id string) (*ReportRecord, error) {
	var req ReportRecord
	if db := db.Model(&ReportRecord{}).Where("hash = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get ReportRecord failed: %s", db.Error)
	}
	return &req, nil
}

func DeleteReportRecordByID(db *gorm.DB, id int64) error {
	if db := db.Model(&ReportRecord{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&ReportRecord{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteReportRecordByIDs(db *gorm.DB, ids ...int64) error {
	if len(ids) == 1 {
		id := ids[0]
		if db := db.Model(&ReportRecord{}).Where(
			"id = ?", id,
		).Unscoped().Delete(&ReportRecord{}); db.Error != nil {
			return db.Error
		}
		return nil
	}

	if db = bizhelper.ExactQueryInt64ArrayOr(db, "id", ids).Unscoped().Delete(&ReportRecord{}); db.Error != nil {
		return utils.Errorf("delete id(s) failed: %v", db.Error)
	}

	return nil
}

func DeleteReportRecordByHash(db *gorm.DB, id string) error {
	if db := db.Model(&ReportRecord{}).Where(
		"hash = ?", id,
	).Unscoped().Delete(&ReportRecord{}); db.Error != nil {
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

func QueryReportRecord(db *gorm.DB, params *ypb.QueryReportsRequest) (*bizhelper.Paginator, []*ReportRecord, error) {
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
	var ret []*ReportRecord
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

const (
	REPORT_ITEM_TYPE_MARKDOWN             = "markdown"
	REPORT_ITEM_TYPE_DIVIDER              = "divider"
	REPORT_ITEM_TYPE_TABLE                = "json-table"
	REPORT_ITEM_TYPE_PIE_GRAPH            = "pie-graph"
	REPORT_ITEM_TYPE_VERTICAL_BAR_GRAPH   = "vertical-bar-graph"
	REPORT_ITEM_TYPE_HORIZONTAL_BAR_GRAPH = "horizontal-bar-graph"
	REPORT_ITEM_TYPE_RAW                  = "raw"
	REPORT_ITEM_TYPE_CODE                 = "code"
	REPORT_ITEM_TYPE_WORDCLOUD            = "wordcloud"
)

type ReportItem struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

type Report struct {
	TitleValue string        `json:"title"`
	OwnerValue string        `json:"owner"`
	FromValue  string        `json:"from"`
	Items      []*ReportItem `json:"items"`
}

func NewReport() *Report {
	return &Report{}
}

func safeStr(i interface{}, items ...interface{}) string {
	s := utils.ParseStringToVisible(utils.InterfaceToString(i))
	return fmt.Sprintf(s, items...)
}

func (r *Report) Title(i interface{}, items ...interface{}) {
	r.TitleValue = safeStr(i, items...)
}

func (r *Report) Owner(i interface{}, items ...interface{}) {
	r.OwnerValue = safeStr(i, items...)
}

func (r *Report) From(i interface{}, items ...interface{}) {
	r.FromValue = safeStr(i, items...)
}

func (r *Report) append(item *ReportItem) {
	r.Items = append(r.Items, item)
}

func (r *Report) Markdown(i string) {
	r.append(&ReportItem{
		Type:    REPORT_ITEM_TYPE_MARKDOWN,
		Content: i,
	})
}

func (r *Report) Divider() {
	r.append(&ReportItem{Type: REPORT_ITEM_TYPE_DIVIDER})
}

type graphKVPair struct {
	Key   string `json:"key"`
	Value int64  `json:"value"`
}

func (r *Report) basicGraph(typeStr string, items ...interface{}) {
	pairs := r.baseGeneralKVPair(items...)
	if len(pairs) <= 0 {
		return
	}
	raw, _ := json.Marshal(pairs)
	r.append(&ReportItem{
		Type:    typeStr,
		Content: string(raw),
	})
}

func (r *Report) PieGraph(items ...interface{}) {
	r.basicGraph(REPORT_ITEM_TYPE_PIE_GRAPH, items...)
}

func (r *Report) WordCloud(items ...interface{}) {
	r.basicGraph(REPORT_ITEM_TYPE_WORDCLOUD, items...)
}

func (r *Report) BarGraphVertical(items ...interface{}) {
	r.basicGraph(REPORT_ITEM_TYPE_VERTICAL_BAR_GRAPH, items...)
}

func (r *Report) BarGraphHorizontal(items ...interface{}) {
	r.basicGraph(REPORT_ITEM_TYPE_HORIZONTAL_BAR_GRAPH, items...)
}

func (r *Report) Raw(items interface{}) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("marshal interface{} failed: %s", err)
		}
	}()

	raw, _ := json.Marshal(items)
	if len(raw) <= 0 {
		return
	}

	r.append(&ReportItem{Type: REPORT_ITEM_TYPE_RAW, Content: utils.EscapeInvalidUTF8Byte(raw)})
}

func (r *Report) Code(items interface{}) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("marshal interface{} failed: %s", err)
		}
	}()

	raw := utils.InterfaceToBytes(items)
	r.append(&ReportItem{Type: REPORT_ITEM_TYPE_CODE, Content: utils.EscapeInvalidUTF8Byte(raw)})
}

func (r *Report) baseGeneralKVPair(items ...interface{}) []*graphKVPair {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("read kvpair for report failed: %s", err)
			return
		}
	}()
	pairs := funk.Map(items, func(i interface{}) *graphKVPair {
		rawMap := utils.InterfaceToGeneralMap(i)
		if rawMap == nil {
			return nil
		}
		key := utils.MapGetFirstRaw(rawMap, "key", "Key", "name", "Name")
		value := utils.MapGetFirstRaw(rawMap, "value", "Value", "data", "Data")
		num, _ := strconv.ParseInt(fmt.Sprint(value), 10, 64)
		return &graphKVPair{
			Key:   fmt.Sprint(key),
			Value: num,
		}
	}).([]*graphKVPair)
	return funk.Filter(pairs, func(pair *graphKVPair) bool { return pair != nil }).([]*graphKVPair)
}

func (r *Report) Table(i interface{}, raw ...interface{}) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("create table failed: %s", err)
		}
	}()
	headers := funk.Map(i, func(result interface{}) string {
		return utils.InterfaceToString(result)
	}).([]string)
	var data = make([][]string, len(raw))
	for index, rawIns := range raw {
		dataRow := funk.Map(rawIns, func(row interface{}) string {
			return utils.InterfaceToString(row)
		}).([]string)
		data[index] = dataRow
	}
	rawBytes, err := json.Marshal(map[string]interface{}{
		"header": headers,
		"data":   data,
	})
	if err != nil {
		log.Errorf("marshal bytes failed: %s", err)
		return
	}
	r.append(&ReportItem{
		Type:    REPORT_ITEM_TYPE_TABLE,
		Content: string(rawBytes),
	})
}

func (r *Report) ToRecord() (*ReportRecord, error) {
	raw, err := json.Marshal(r.Items)
	if err != nil {
		return nil, utils.Errorf("marshal report item failed: %s", err)
	}
	var owner = r.OwnerValue
	var from = r.FromValue
	if owner == "" {
		owner = "default"
	}
	if from == "" {
		from = "default"
	}
	return &ReportRecord{
		Title:       r.TitleValue,
		PublishedAt: time.Now(),
		Owner:       owner,
		From:        from,
		QuotedJson:  strconv.Quote(string(raw)),
	}, nil
}

func (r *ReportRecord) ToReport() (*Report, error) {
	jsonStr, err := strconv.Unquote(r.QuotedJson)
	if err != nil {
		return nil, utils.Errorf("unquote json body failed: %s", err)
	}
	var items []*ReportItem
	_ = json.Unmarshal([]byte(jsonStr), &items)
	reportIns := &Report{
		TitleValue: r.Title,
		OwnerValue: r.Owner,
		FromValue:  r.From,
		Items:      items,
	}
	return reportIns, nil
}

func (r *Report) Save() int {
	record, err := r.ToRecord()
	if err != nil {
		return 0
	}
	db := consts.GetGormProjectDatabase()
	if db != nil {
		db.Save(record)
	}
	return int(record.ID)
}

var ReportExports = map[string]interface{}{
	"New": NewReport,
}
