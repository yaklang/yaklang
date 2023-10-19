package yakit

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/ReneKroon/ttlcache"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var (
	_WebFuzzerTaskTTLCache     = ttlcache.NewCache()
	_WebFuzzerResponseTTLCache = ttlcache.NewCache()
)

func init() {
	ttl := 30 * time.Minute
	_WebFuzzerResponseTTLCache.SetTTL(ttl)
	_WebFuzzerTaskTTLCache.SetTTL(ttl)
}

/*
这个结构用于保存当前测试的结果

包含：基本参数+请求数据

耗时+执行结果

执行结果包含，失败原因与执行成功的原因。

总共有多少个请求
*/
type WebFuzzerTask struct {
	gorm.Model

	// 原始请求 json+quote
	RawFuzzTaskRequest string `json:"raw_fuzz_task_request"`

	// 对应前端的组织形式
	FuzzerIndex    string `json:"fuzzer_index"`
	FuzzerTabIndex string `json:"fuzzer_tab_index"`

	// HTTP 数据流总量
	HTTPFlowTotal        int    `json:"http_flow_total"`
	HTTPFlowSuccessCount int    `json:"http_flow_success_count"`
	HTTPFlowFailedCount  int    `json:"http_flow_failed_count"`
	Ok                   bool   `json:"ok"`
	Reason               string `json:"reason"` // if not ok
	Host                 string `json:"host"`
	Port                 int    `json:"port"`
	// retry 相关
	RetryRootID uint `json:"retry_root_id"`
}

func (w *WebFuzzerTask) CalcCacheHash() string {
	return utils.CalcSha1(w.ID, w.FuzzerIndex, w.FuzzerTabIndex, w.HTTPFlowTotal, w.HTTPFlowFailedCount, w.HTTPFlowSuccessCount, w.Ok, w.Reason, w.Host, w.Port, w.RetryRootID)
}

func (w *WebFuzzerTask) getCacheGRPCModel() *ypb.HistoryHTTPFuzzerTask {
	i, ok := _WebFuzzerTaskTTLCache.Get(w.CalcCacheHash())
	if ok {
		return i.(*ypb.HistoryHTTPFuzzerTask)
	}
	return nil
}

func (w *WebFuzzerTask) setCacheGRPCModel(t *ypb.HistoryHTTPFuzzerTask) {
	_WebFuzzerTaskTTLCache.Set(w.CalcCacheHash(), t)
}

func (w *WebFuzzerTask) ToGRPCModel() *ypb.HistoryHTTPFuzzerTask {
	var t *ypb.HistoryHTTPFuzzerTask

	if t = w.getCacheGRPCModel(); t != nil {
		return t
	}

	t = &ypb.HistoryHTTPFuzzerTask{
		Id:                   int32(w.ID),
		CreatedAt:            w.CreatedAt.Unix(),
		HTTPFlowTotal:        int32(w.HTTPFlowTotal),
		HTTPFlowSuccessCount: int32(w.HTTPFlowSuccessCount),
		HTTPFlowFailedCount:  int32(w.HTTPFlowFailedCount),
		Host:                 w.Host,
		Port:                 int32(w.Port),
	}
	w.setCacheGRPCModel(t)
	return t
}

func (w *WebFuzzerTask) ToGRPCModelDetail() *ypb.HistoryHTTPFuzzerTaskDetail {
	var reqRaw ypb.FuzzerRequest
	_ = json.Unmarshal([]byte(w.RawFuzzTaskRequest), &reqRaw)
	return &ypb.HistoryHTTPFuzzerTaskDetail{
		BasicInfo:     w.ToGRPCModel(),
		OriginRequest: &reqRaw,
	}
}

func QueryFirst50WebFuzzerTask(db *gorm.DB) []*ypb.HistoryHTTPFuzzerTask {
	var task []*WebFuzzerTask
	if db := db.Model(&WebFuzzerTask{}).Order("created_at desc").Find(&task); db.Error != nil {
		log.Errorf("query web fuzzer task failed: %s", db.Error)
		return nil
	} else {
		return funk.Map(task, func(i *WebFuzzerTask) *ypb.HistoryHTTPFuzzerTask {
			return i.ToGRPCModel()
		}).([]*ypb.HistoryHTTPFuzzerTask)
	}
}

func QueryFuzzerHistoryTasks(db *gorm.DB, req *ypb.QueryHistoryHTTPFuzzerTaskExParams) (*bizhelper.Paginator, []*WebFuzzerTask, error) {
	var keywords []string
	if req.GetKeyword() != "" {
		keywords = append(keywords, req.GetKeyword())
		keywords = append(keywords, strings.Trim(strconv.Quote(req.GetKeyword()), `" \r\n`))
		db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
			"id", "raw_fuzz_task_request", "host",
		}, keywords, false)
	}

	if req.GetFuzzerTabIndex() != "" {
		db = db.Where("fuzzer_tab_index = ?", req.GetFuzzerTabIndex())
	}
	pagination := req.GetPagination()
	order, orderby := pagination.Order, pagination.OrderBy
	if order == "" {
		order = "asc"
	}
	if orderby == "" {
		orderby = "id"
	}

	var task []*WebFuzzerTask

	db = bizhelper.QueryOrder(db, orderby, order)
	paging, db := bizhelper.Paging(db, int(pagination.GetPage()), int(pagination.GetLimit()), &task)
	if db.Error != nil {
		return nil, nil, utils.Errorf("pagination failed: %s", db.Error)
	}
	return paging, task, nil
}

func SaveWebFuzzerTask(db *gorm.DB, req *ypb.FuzzerRequest, total int, ok bool, reason string) (*WebFuzzerTask, error) {
	if req.Verbose == "" {
		if req.Request == "" && req.RequestRaw != nil {
			req.Verbose = utils.EscapeInvalidUTF8Byte(req.RequestRaw)
		}
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, utils.Errorf("marshal fuzzer request failed: %s", err)
	}

	t := &WebFuzzerTask{
		RawFuzzTaskRequest: string(raw),
		HTTPFlowTotal:      total,
		Ok:                 ok,
		Reason:             reason,
	}
	if db := db.Save(t); db.Error != nil {
		return nil, err
	}
	return t, nil
}

func DeleteWebFuzzerTaskAll(db *gorm.DB) error {
	if db := db.Model(&WebFuzzerTask{}).Where("true").Unscoped().Delete(&WebFuzzerTask{}); db.Error != nil {
		return utils.Errorf("delete web fuzzer all failed: %s", db.Error)
	}
	return nil
}

func DeleteWebFuzzerTask(db *gorm.DB, id int64) error {
	if db := db.Model(&WebFuzzerTask{}).Where("id = ?", id).Unscoped().Delete(&WebFuzzerTask{}); db.Error != nil {
		return utils.Errorf("delete web fuzzer by id failed: %s", db.Error)
	}
	return nil
}

func DeleteWebFuzzerTaskByWebFuzzerIndex(db *gorm.DB, index string) error {
	if db := db.Debug().Model(&WebFuzzerTask{}).Where("fuzzer_tab_index = ?", index).Unscoped().Delete(&WebFuzzerTask{}); db.Error != nil {
		return utils.Errorf("delete web fuzzer by fuzzer_tab_index failed: %s", db.Error)
	}
	return nil
}

func GetWebFuzzerTaskById(db *gorm.DB, id int) (*WebFuzzerTask, error) {
	var t WebFuzzerTask
	if db := db.Model(&WebFuzzerTask{}).Where("id = ?", id).First(&t); db.Error != nil {
		return nil, utils.Errorf("get web fuzzer task failed: %s", db.Error)
	}
	return &t, nil
}

func GetWebFuzzerRetryRootID(db *gorm.DB, id uint) (uint, error) {
	var t WebFuzzerTask
	if db := db.Model(&WebFuzzerTask{}).Select("retry_root_id").Where("id = ?", id).First(&t); db.Error != nil {
		return 0, utils.Errorf("get web fuzzer task retry_root_id failed: %s", db.Error)
	}
	return t.RetryRootID, nil
}

func GetWebFuzzerTasksIDByRetryRootID(db *gorm.DB, root_id uint) ([]uint, error) {
	var ids []uint
	if db := db.Model(&WebFuzzerTask{}).Where("retry_root_id = ?", root_id).Pluck("id", &ids); db.Error != nil {
		return nil, utils.Errorf("get web fuzzer task id by retry_root_id failed: %s", db.Error)
	}
	return ids, nil
}

type WebFuzzerResponse struct {
	gorm.Model

	WebFuzzerTaskId int    `json:"web_fuzzer_task_id" gorm:"index"`
	OK              bool   `json:"ok"`
	Request         string `json:"request"`
	Content         string `json:"content"`
	Payload         string `json:"payload"`
	Url             string `json:"url"`
	StatusCode      int    `json:"status_code"`
	DurationMs      int    `json:"duration_ms"`
	Timestamp       int64  `json:"timestamp"`
}

func (w *WebFuzzerResponse) CalcCacheHash() string {
	return utils.CalcSha1(w.ID, w.WebFuzzerTaskId, w.OK, w.Request, w.Content, w.Payload, w.Url, w.StatusCode, w.DurationMs, w.Timestamp)
}

func (w *WebFuzzerResponse) getCacheGRPCModel() *ypb.FuzzerResponse {
	i, ok := _WebFuzzerResponseTTLCache.Get(w.CalcCacheHash())
	if ok {
		return i.(*ypb.FuzzerResponse)
	}
	return nil
}

func (w *WebFuzzerResponse) setCacheGRPCModel(r *ypb.FuzzerResponse) {
	_WebFuzzerResponseTTLCache.Set(w.CalcCacheHash(), r)
}

func (w *WebFuzzerResponse) ToGRPCModel() (*ypb.FuzzerResponse, error) {
	var rsp ypb.FuzzerResponse
	if r := w.getCacheGRPCModel(); r != nil {
		return r, nil
	}

	err := json.Unmarshal([]byte(w.Content), &rsp)
	if err != nil {
		log.Errorf("unmarshal fuzzer failed: %s", err)
		return nil, err
	}
	w.setCacheGRPCModel(&rsp)
	return &rsp, nil
}

func DeleteWebFuzzerResponseByTaskID(db *gorm.DB, id int64) error {
	if db := db.Model(&WebFuzzerResponse{}).Where(
		"web_fuzzer_task_id = ?", id,
	).Unscoped().Delete(&WebFuzzerResponse{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func QueryWebFuzzerResponse(db *gorm.DB, params *ypb.QueryHTTPFuzzerResponseByTaskIdRequest) (*bizhelper.Paginator, []*WebFuzzerResponse, error) {
	db = db.Model(&WebFuzzerResponse{})

	db = db.Where("web_fuzzer_task_id = ?", params.GetTaskId())

	p := params.GetPagination()
	db = bizhelper.QueryOrder(db, "created_at", "desc")

	var ret []*WebFuzzerResponse
	paging, db := bizhelper.Paging(db, int(p.GetPage()), int(p.GetLimit()), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func YieldWebFuzzerResponseByTaskIDsWithOk(db *gorm.DB, ctx context.Context, taskIDs []uint) chan *WebFuzzerResponse {
	db = db.Model(&WebFuzzerResponse{})
	db = db.Where("ok = true").Where("web_fuzzer_task_id IN (?)", taskIDs)
	outC := make(chan *WebFuzzerResponse)
	yieldWebFuzzerResponsesToChan(outC, db, ctx)
	return outC
}

func SaveWebFuzzerResponse(db *gorm.DB, taskId int, rsp *ypb.FuzzerResponse) {
	raw, err := json.Marshal(rsp)
	if err != nil {
		log.Errorf("marshal FuzzerResponse failed: %s", err)
		return
	}
	r := &WebFuzzerResponse{
		WebFuzzerTaskId: taskId,
		OK:              rsp.Ok,
		Request:         utils.UnsafeBytesToString(rsp.RequestRaw),
		Content:         utils.UnsafeBytesToString(raw),
		Payload:         strings.Join(rsp.Payloads, ","),
		Url:             rsp.Url,
		StatusCode:      int(rsp.StatusCode),
		DurationMs:      int(rsp.DurationMs),
		Timestamp:       rsp.GetTimestamp(),
	}
	if db := db.Save(r); db.Error != nil {
		log.Errorf("save web fuzzer response to database failed: %s", db.Error)
		return
	}
}

func YieldWebFuzzerResponses(db *gorm.DB, ctx context.Context, id int) chan *WebFuzzerResponse {
	db = db.Model(&WebFuzzerResponse{}).Where("web_fuzzer_task_id = ?", id)
	outC := make(chan *WebFuzzerResponse)
	yieldWebFuzzerResponsesToChan(outC, db, ctx)
	return outC
}

func yieldWebFuzzerResponsesToChan(outC chan *WebFuzzerResponse, db *gorm.DB, ctx context.Context) {
	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*WebFuzzerResponse
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
}
