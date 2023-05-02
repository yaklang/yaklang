package yakit

import (
	"context"
	"encoding/json"
	"github.com/jinzhu/gorm"
	"strconv"
	"strings"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/bizhelper"
	"yaklang/common/yakgrpc/ypb"
)

import "yaklang/common/go-funk"

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

	// HTTP 数据流总量
	HTTPFlowTotal        int    `json:"http_flow_total"`
	HTTPFlowSuccessCount int    `json:"http_flow_success_count"`
	HTTPFlowFailedCount  int    `json:"http_flow_failed_count"`
	Ok                   bool   `json:"ok"`
	Reason               string `json:"reason"` // if not ok
	Host                 string `json:"host"`
	Port                 int    `json:"port"`
}

func (w *WebFuzzerTask) ToSwaggerModel() *ypb.HistoryHTTPFuzzerTask {
	return &ypb.HistoryHTTPFuzzerTask{
		Id:                   int32(w.ID),
		CreatedAt:            w.CreatedAt.Unix(),
		HTTPFlowTotal:        int32(w.HTTPFlowTotal),
		HTTPFlowSuccessCount: int32(w.HTTPFlowSuccessCount),
		HTTPFlowFailedCount:  int32(w.HTTPFlowFailedCount),
		Host:                 w.Host,
		Port:                 int32(w.Port),
	}
}

func (w *WebFuzzerTask) ToSwaggerModelDetail() *ypb.HistoryHTTPFuzzerTaskDetail {
	var reqRaw ypb.FuzzerRequest
	_ = json.Unmarshal([]byte(w.RawFuzzTaskRequest), &reqRaw)
	return &ypb.HistoryHTTPFuzzerTaskDetail{
		BasicInfo:     w.ToSwaggerModel(),
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
			return i.ToSwaggerModel()
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

	var task []*WebFuzzerTask
	db = bizhelper.QueryOrder(db, "id", "desc")
	paging, db := bizhelper.Paging(db, int(req.GetPagination().GetPage()), int(req.GetPagination().GetLimit()), &task)
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

func GetWebFuzzerTaskById(db *gorm.DB, id int) (*WebFuzzerTask, error) {
	var t WebFuzzerTask
	if db := db.Model(&WebFuzzerTask{}).Where("id = ?", id).First(&t); db.Error != nil {
		return nil, utils.Errorf("get web fuzzer task failed: %s", db.Error)
	}
	return &t, nil
}

type WebFuzzerResponse struct {
	gorm.Model

	WebFuzzerTaskId int    `json:"web_fuzzer_task_id" gorm:"index"`
	Content         string `json:"content"`
	Payload         string `json:"payload"`
	Url             string `json:"url"`
	StatusCode      int    `json:"status_code"`
	DurationMs      int    `json:"duration_ms"`
	Timestamp       int64  `json:"timestamp"`
}

func (w *WebFuzzerResponse) ToGRPCModel() (*ypb.FuzzerResponse, error) {
	var rsp ypb.FuzzerResponse
	err := json.Unmarshal([]byte(w.Content), &rsp)
	if err != nil {
		log.Errorf("unmarshal fuzzer failed: %s", err)
		return nil, err
	}
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

func SaveWebFuzzerResponse(db *gorm.DB, taskId int, rsp *ypb.FuzzerResponse) {
	raw, err := json.Marshal(rsp)
	if err != nil {
		log.Errorf("marshal FuzzerResponse failed: %s", err)
		return
	}
	r := &WebFuzzerResponse{
		WebFuzzerTaskId: taskId,
		Content:         string(raw),
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
	return outC
}
