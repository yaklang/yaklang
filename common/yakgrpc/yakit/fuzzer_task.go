package yakit

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var (
	WebFuzzerTaskTTLCache     = utils.NewTTLCache[*ypb.HistoryHTTPFuzzerTask](30 * time.Minute)
	WebFuzzerResponseTTLCache = utils.NewTTLCache[*ypb.FuzzerResponse](30 * time.Minute)
)

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
	t, ok := WebFuzzerTaskTTLCache.Get(w.CalcCacheHash())
	if ok {
		return t
	}
	return nil
}

func (w *WebFuzzerTask) setCacheGRPCModel(t *ypb.HistoryHTTPFuzzerTask) {
	WebFuzzerTaskTTLCache.Set(w.CalcCacheHash(), t)
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

// Deprecated
func QueryFirst50WebFuzzerTask(db *gorm.DB) []*ypb.HistoryHTTPFuzzerTask {
	var task []*WebFuzzerTask
	if db := db.Model(&WebFuzzerTask{}).Where("id = retry_root_id or retry_root_id is null or retry_root_id = 0").Order("created_at desc").Find(&task); db.Error != nil {
		log.Errorf("query web fuzzer task failed: %s", db.Error)
		return nil
	} else {
		return funk.Map(task, func(i *WebFuzzerTask) *ypb.HistoryHTTPFuzzerTask {
			return i.ToGRPCModel()
		}).([]*ypb.HistoryHTTPFuzzerTask)
	}
}

func QueryFuzzerHistoryTasks(db *gorm.DB, req *ypb.QueryHistoryHTTPFuzzerTaskExParams) (*bizhelper.Paginator, []*WebFuzzerTask, error) {
	oldDB := db

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

	// 返回的任务跳过重试的任务
	db = db.Where("id = retry_root_id or retry_root_id is null or retry_root_id = 0")

	var returnTasks, tasks []*WebFuzzerTask

	db = bizhelper.QueryOrder(db, orderby, order)
	paging, db := bizhelper.Paging(db, int(pagination.GetPage()), int(pagination.GetLimit()), &returnTasks)
	if db.Error != nil {
		return nil, nil, utils.Errorf("pagination failed: %s", db.Error)
	}

	// 对重试任务进行处理

	// 先获取所有task的id
	ids := lo.Map(returnTasks, func(i *WebFuzzerTask, _ int) int64 {
		return int64(i.ID)
	})
	db = oldDB.Model(&WebFuzzerTask{}).Select([]string{"id", "retry_root_id", "http_flow_total", "http_flow_success_count"})
	db = bizhelper.ExactQueryInt64ArrayOr(db, "retry_root_id", ids)
	// 找到重试任务，计算总共成功的数量
	if db.Find(&tasks); db.Error != nil {
		return nil, nil, utils.Errorf("search by retry_root_id failed: %s", db.Error)
	}
	successCountMap := make(map[uint]int)
	for _, task := range tasks {
		if _, ok := successCountMap[task.RetryRootID]; !ok {
			successCountMap[task.RetryRootID] = 0
		}
		successCountMap[task.RetryRootID] += task.HTTPFlowSuccessCount
	}
	// 更新返回的任务
	for _, task := range returnTasks {
		if successCount, ok := successCountMap[uint(task.ID)]; ok {
			task.HTTPFlowSuccessCount = successCount
			task.HTTPFlowFailedCount = task.HTTPFlowTotal - successCount
		}
	}

	return paging, returnTasks, nil
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
		return nil, db.Error
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
	if db := db.Model(&WebFuzzerTask{}).Where("fuzzer_tab_index = ?", index).Unscoped().Delete(&WebFuzzerTask{}); db.Error != nil {
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
	HiddenIndex     string `json:"hidden_index"`
}

func (w *WebFuzzerResponse) CalcCacheHash() string {
	return utils.CalcSha1(w.ID, w.WebFuzzerTaskId, w.OK, w.Request, w.Content, w.Payload, w.Url, w.StatusCode, w.DurationMs, w.Timestamp)
}

func (w *WebFuzzerResponse) getCacheGRPCModel() *ypb.FuzzerResponse {
	rsp, ok := WebFuzzerResponseTTLCache.Get(w.CalcCacheHash())
	if ok {
		return rsp
	}
	return nil
}

func (w *WebFuzzerResponse) setCacheGRPCModel(r *ypb.FuzzerResponse) {
	WebFuzzerResponseTTLCache.Set(w.CalcCacheHash(), r)
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

func queryWebFuzzerResponsePayloadsFromHTTPFlow(db *gorm.DB, httpflows []*HTTPFlow) (hiddenIndexToPayloadsMap map[string][]string, err error) {
	var responses []*WebFuzzerResponse
	hiddenIndexs := lo.Map(httpflows, func(i *HTTPFlow, _ int) string {
		return i.HiddenIndex
	})
	db = db.Model(&WebFuzzerResponse{})
	db = bizhelper.ExactOrQueryStringArrayOr(db, "hidden_index", hiddenIndexs)
	err = db.Select("payload, hidden_index").Find(&responses).Error
	if err != nil {
		return nil, err
	}

	hiddenIndexToPayloadsMap = make(map[string][]string)
	for _, r := range responses {
		hiddenIndexToPayloadsMap[r.HiddenIndex] = strings.Split(r.Payload, ",")
	}
	return hiddenIndexToPayloadsMap, nil
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

func YieldWebFuzzerResponseByTaskIDs(db *gorm.DB, ctx context.Context, taskIDs []uint, oks ...bool) chan *WebFuzzerResponse {
	int64TaskIDs := lo.Map(taskIDs, func(i uint, _ int) int64 { return int64(i) })

	db = db.Model(&WebFuzzerResponse{})
	db = bizhelper.ExactQueryInt64ArrayOr(db, "web_fuzzer_task_id", int64TaskIDs)
	if len(oks) > 0 {
		db = db.Where("ok = ?", oks[0])
	}
	outC := make(chan *WebFuzzerResponse)
	yieldWebFuzzerResponsesToChan(outC, db, ctx)
	return outC
}

func SaveWebFuzzerResponse(db *gorm.DB, taskId int, hiddenIndex string, rsp *ypb.FuzzerResponse) {
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
		HiddenIndex:     hiddenIndex,
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

		page := 1
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
