package schema

import (
	"encoding/json"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"time"
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
	HiddenIndex     string `json:"hidden_index" gorm:"index"`
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

var WebFuzzerTaskTTLCache = utils.NewTTLCache[*ypb.HistoryHTTPFuzzerTask](30 * time.Minute)

var (
	WebFuzzerResponseTTLCache = utils.NewTTLCache[*ypb.FuzzerResponse](30 * time.Minute)
)
